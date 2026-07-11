package team

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mewcode/internal/tool"
)

func TestCreateTeamPersistsExpectedFiles(t *testing.T) {
	m := NewManager(t.TempDir(), Options{DefaultBackend: BackendInProcess, DefaultMemberApproval: true})
	created, err := m.Create("alpha")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	for _, name := range []string{"team.json", "tasks.json", "members.json", "events.jsonl", "mailboxes/lead.jsonl"} {
		if _, err := os.Stat(filepath.Join(created.Root, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	loaded, err := m.Load("alpha")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Name != "alpha" || loaded.Lead != "lead" || loaded.Backend != BackendInProcess {
		t.Fatalf("loaded team = %#v", loaded)
	}
	members, err := m.Members("alpha")
	if err != nil {
		t.Fatalf("Members returned error: %v", err)
	}
	if len(members) != 1 || !members[0].RequiresApproval || members[0].InstanceID != "lead" {
		t.Fatalf("members = %#v", members)
	}
}

func TestTasksSaveDependsOnAndUpdate(t *testing.T) {
	m := NewManager(t.TempDir(), Options{})
	if _, err := m.Create("alpha"); err != nil {
		t.Fatal(err)
	}
	first, err := m.CreateTask("alpha", "探索", "", "dev", nil)
	if err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	second, err := m.CreateTask("alpha", "实现", "写代码", "dev", []string{first.ID})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}
	if len(second.DependsOn) != 1 || second.DependsOn[0] != first.ID {
		t.Fatalf("depends_on = %#v", second.DependsOn)
	}
	updated, err := m.UpdateTask("alpha", second.ID, TaskStatusDone, "ok", "")
	if err != nil {
		t.Fatalf("UpdateTask returned error: %v", err)
	}
	if updated.Status != TaskStatusDone || updated.Result != "ok" {
		t.Fatalf("updated = %#v", updated)
	}
	tasks, err := m.ListTasks("alpha")
	if err != nil || len(tasks) != 2 {
		t.Fatalf("ListTasks = %#v %v", tasks, err)
	}
}

func TestMailboxSendBroadcastAndBadLineWarning(t *testing.T) {
	m := NewManager(t.TempDir(), Options{})
	created, err := m.Create("alpha")
	if err != nil {
		t.Fatal(err)
	}
	members, _ := m.Members("alpha")
	members = append(members, Member{Name: "dev", Role: "coder", InstanceID: "dev-1", Backend: BackendInProcess, Status: MemberStatusIdle})
	if err := m.SaveMembers("alpha", members); err != nil {
		t.Fatal(err)
	}
	sent, err := m.SendMessage("alpha", "lead", "dev", MessageSummary, "hello", "short", nil)
	if err != nil || len(sent) != 1 {
		t.Fatalf("SendMessage = %#v %v", sent, err)
	}
	broadcast, err := m.SendMessage("alpha", "lead", "all", MessageLifecycle, "wake", "", nil)
	if err != nil || len(broadcast) != 1 {
		t.Fatalf("Broadcast = %#v %v", broadcast, err)
	}
	path := filepath.Join(created.Root, "mailboxes", "dev-1.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString("{bad json\n")
	raw, _ := json.Marshal(newMailMessage("lead", "dev", MessageApprovalReply, "yes", "", nil))
	_, _ = file.Write(append(raw, '\n'))
	_ = file.Close()
	items, err := m.Mailbox("alpha", "dev-1")
	if err != nil {
		t.Fatalf("Mailbox returned error: %v", err)
	}
	if len(items) != 3 || m.WarningCount() == 0 {
		t.Fatalf("items=%d warnings=%d", len(items), m.WarningCount())
	}
	if _, err := m.SendMessage("alpha", "lead", "missing", MessageText, "x", "", nil); err == nil {
		t.Fatalf("expected unknown member error")
	}
}

func TestBackendsStartStopAndTerminalPaneNoFallback(t *testing.T) {
	m := NewManager(t.TempDir(), Options{})
	if _, err := m.Create("alpha"); err != nil {
		t.Fatal(err)
	}
	members, _ := m.Members("alpha")
	members = append(members, Member{Name: "dev", Role: "coder", InstanceID: "dev-1", Backend: BackendInProcess, Status: MemberStatusIdle})
	if err := m.SaveMembers("alpha", members); err != nil {
		t.Fatal(err)
	}
	m.Runner = func(ctx context.Context, req RunRequest) RunResult {
		return RunResult{OK: true, Status: MemberStatusIdle}
	}
	if result := m.StartMember(context.Background(), "alpha", "dev", "task"); !result.OK || result.Status != MemberStatusRunning {
		t.Fatalf("StartMember = %#v", result)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		dev, _ := m.ResolveMember("alpha", "dev")
		if dev.Status == MemberStatusIdle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("member did not become idle")
}

func TestTerminalPaneBackendReturnsExplicitError(t *testing.T) {
	m := NewManager(t.TempDir(), Options{})
	if _, err := m.Create("alpha"); err != nil {
		t.Fatal(err)
	}
	members, _ := m.Members("alpha")
	members = append(members, Member{Name: "pane", Role: "coder", InstanceID: "pane-1", Backend: BackendTerminalPane, Status: MemberStatusIdle})
	if err := m.SaveMembers("alpha", members); err != nil {
		t.Fatal(err)
	}
	result := m.StartMember(context.Background(), "alpha", "pane", "task")
	if result.OK || !strings.Contains(result.Error, "terminal_pane") {
		t.Fatalf("terminal pane result = %#v", result)
	}
}

func TestReloadMembersFromMarkdownFrontmatter(t *testing.T) {
	m := NewManager(t.TempDir(), Options{DefaultBackend: BackendInProcess})
	created, err := m.Create("alpha")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(created.Root, "members")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.md"), []byte(`---
name: dev
role: coder
instance_id: dev-1
backend: terminal_pane
requires_approval: true
---
Dev member.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err := m.ReloadMembersFromConfig("alpha")
	if err != nil {
		t.Fatalf("ReloadMembersFromConfig returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	dev, err := m.ResolveMember("alpha", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Role != "coder" || dev.InstanceID != "dev-1" || dev.Backend != BackendTerminalPane || !dev.RequiresApproval {
		t.Fatalf("dev = %#v", dev)
	}
}

func TestSchedulerFilteringAndContext(t *testing.T) {
	m := NewManager(t.TempDir(), Options{SchedulerAllowed: true})
	if err := m.SetSchedulerEnabled(true); err != nil {
		t.Fatal(err)
	}
	defs := []tool.Definition{
		{Name: "read_file"},
		{Name: "run_command"},
		{Name: ToolTaskCreate},
		{Name: ToolMessageSend},
	}
	main := m.FilterDefinitions(defs, Actor{})
	if len(main) != 2 || main[0].Name != "read_file" {
		t.Fatalf("main defs = %#v", main)
	}
	lead := m.FilterDefinitions(defs, Actor{Team: "alpha", Name: "lead", Kind: ActorLead})
	if len(lead) != 2 || lead[0].Name != ToolTaskCreate || lead[1].Name != ToolMessageSend {
		t.Fatalf("lead defs = %#v", lead)
	}
	msgs := m.ContextMessages(Actor{Team: "alpha", Name: "lead", Kind: ActorLead})
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "理解目标") || !strings.Contains(msgs[0].Content, "合并/上报") {
		t.Fatalf("context = %#v", msgs)
	}
	locked := NewManager(t.TempDir(), Options{SchedulerAllowed: false})
	if err := locked.SetSchedulerEnabled(true); err == nil {
		t.Fatalf("expected scheduler lock error")
	}
}

func TestRegisterToolsAndVisibilityHelpers(t *testing.T) {
	reg := tool.NewRegistry()
	m := NewManager(t.TempDir(), Options{})
	if err := RegisterTools(reg, m); err != nil {
		t.Fatalf("RegisterTools returned error: %v", err)
	}
	defs := reg.Definitions()
	if len(defs) != 7 {
		t.Fatalf("defs = %#v", defs)
	}
	for _, def := range defs {
		if !IsTeamTool(def.Name) {
			t.Fatalf("expected team tool: %s", def.Name)
		}
	}
}

func TestConservativeMergeCleanAndConflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "file.txt")
	runGit(t, repo, "commit", "-m", "base")
	baseBranch := strings.TrimSpace(runGitOutput(t, repo, "branch", "--show-current"))
	runGit(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "new.txt")
	runGit(t, repo, "commit", "-m", "feature")
	runGit(t, repo, "checkout", baseBranch)
	if result := ConservativeMerge(context.Background(), repo, "feature"); !result.OK {
		t.Fatalf("clean merge = %#v", result)
	}

	conflict := t.TempDir()
	runGit(t, conflict, "init")
	runGit(t, conflict, "config", "user.email", "test@example.com")
	runGit(t, conflict, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(conflict, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, conflict, "add", "file.txt")
	runGit(t, conflict, "commit", "-m", "base")
	baseBranch = strings.TrimSpace(runGitOutput(t, conflict, "branch", "--show-current"))
	runGit(t, conflict, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(conflict, "file.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, conflict, "commit", "-am", "feature")
	runGit(t, conflict, "checkout", baseBranch)
	if err := os.WriteFile(filepath.Join(conflict, "file.txt"), []byte("master\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, conflict, "commit", "-am", "master")
	result := ConservativeMerge(context.Background(), conflict, "feature")
	if result.OK || !result.Rolled {
		t.Fatalf("conflict merge = %#v", result)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

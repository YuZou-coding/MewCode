package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/chat"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
	"mewcode/internal/sessionstore"
	"mewcode/internal/tool"
	"mewcode/internal/tui"
	"mewcode/internal/tuiapp"
)

type appProvider struct{}

func (p appProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent, 1)
	errs := make(chan error, 1)
	events <- provider.StreamEvent{Kind: provider.EventText, Text: "ok"}
	close(events)
	errs <- nil
	return events, errs
}

func TestAppRunConnectsLoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out strings.Builder
	err := App{
		Input:       strings.NewReader("hi\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Provider:    appProvider{},
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "MewCode > thinking...") {
		t.Fatalf("output missing thinking status: %q", out.String())
	}
	if !strings.Contains(out.String(), "MewCode > first token in") {
		t.Fatalf("output missing first token timing: %q", out.String())
	}
	if !strings.Contains(out.String(), "MewCode > ok") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCloneCheckerInheritsModeWithoutSessionRules(t *testing.T) {
	checker := &permissions.Checker{
		Root:        t.TempDir(),
		Mode:        permissions.ModeYOLO,
		DefaultMode: permissions.ModeDefault,
		Session:     permissions.NewSessionStore(),
		Project:     []permissions.Rule{{Effect: permissions.EffectDeny, Tool: "edit_file"}},
	}
	checker.Session.Add(permissions.Rule{Effect: permissions.EffectAllow, Tool: "read_file"})
	clone := cloneChecker(checker)
	if clone.CurrentMode() != permissions.ModeYOLO || clone.InitialMode() != permissions.ModeDefault {
		t.Fatalf("clone modes = current:%s default:%s", clone.CurrentMode(), clone.InitialMode())
	}
	if len(clone.Session.Rules()) != 0 || len(clone.Project) != 1 {
		t.Fatalf("clone rules = session:%#v project:%#v", clone.Session.Rules(), clone.Project)
	}
}

type scriptedAppProvider struct {
	requests [][]chat.Message
	series   [][]provider.StreamEvent
}

func (p *scriptedAppProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	copied := make([]chat.Message, len(messages))
	copy(copied, messages)
	p.requests = append(p.requests, copied)
	selected := []provider.StreamEvent{}
	if len(p.series) > 0 {
		selected = p.series[0]
		p.series = p.series[1:]
	}
	events := make(chan provider.StreamEvent, len(selected))
	errs := make(chan error, 1)
	for _, event := range selected {
		events <- event
	}
	close(events)
	errs <- nil
	return events, errs
}

func TestWorkerIsolationCreatesWorktreeAndInjectsPathTranslation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initAppRepo(t)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".mewcode", "workers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".mewcode", "workers", "iso.md"), []byte("---\nname: iso\ndescription: isolated\nisolation: worktree\n---\nUse isolation.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := &scriptedAppProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "run_worker", Arguments: []byte(`{"task":"inspect","role":"iso"}`)}}},
		{{Kind: provider.EventText, Text: "child done"}},
		{{Kind: provider.EventText, Text: "parent done"}},
	}}
	var out strings.Builder
	err := App{
		Input:       strings.NewReader("delegate\ny\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Provider:    fp,
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v\nout=%s", err, out.String())
	}
	if len(fp.requests) < 3 {
		t.Fatalf("requests = %d out=%s reqs=%s", len(fp.requests), out.String(), appMessagesText(fp.requests[len(fp.requests)-1]))
	}
	child := appMessagesText(fp.requests[1])
	if !strings.Contains(child, "mewcode-worktree-isolation") || !strings.Contains(child, filepath.Join(repo, ".mewcode", "worktrees", "worker", "worker_1")) {
		t.Fatalf("child request missing isolation: %s", child)
	}
	if _, err := os.Stat(filepath.Join(repo, ".mewcode", "worktrees", "worker", "worker_1")); !os.IsNotExist(err) {
		t.Fatalf("isolated worktree should be cleaned, stat err=%v", err)
	}
}

func TestRunAgentForTUIPermissionPromptAllowsTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp_tool_test.txt")
	if err := os.WriteFile(path, []byte("hello tool"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &scriptedAppProvider{series: [][]provider.StreamEvent{
		{
			{Kind: provider.EventThinking, Text: "hidden reasoning"},
			{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"` + path + `","old_text":"hello tool","new_text":"hello MewCode"}`)}},
		},
		{{Kind: provider.EventText, Text: "edited"}},
	}}
	loop := testLoopForApp(registry, fp, dir)
	choiceCount := 0
	events := drainTUIEvents(runAgentForTUI(context.Background(), loop, "edit", func(ctx context.Context, request permissions.Request, decision permissions.Decision) permissions.HITLChoice {
		choiceCount++
		if request.Tool != "edit_file" || !strings.Contains(string(request.Arguments), path) {
			t.Fatalf("unexpected permission request: %#v", request)
		}
		return permissions.HITLAllowOnce
	}))
	if choiceCount != 1 {
		t.Fatalf("choiceCount = %d", choiceCount)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "hello MewCode" {
		t.Fatalf("file content = %q", raw)
	}
	if !hasTUIEvent(events, tuiapp.StreamThinking, "hidden reasoning") {
		t.Fatalf("events missing thinking: %#v", events)
	}
	if !hasTUIToolEvent(events, tuiapp.StreamToolStart, "edit_file") {
		t.Fatalf("events missing tool start: %#v", events)
	}
	if !hasTUIToolEvent(events, tuiapp.StreamToolResult, "edit_file") {
		t.Fatalf("events missing tool result: %#v", events)
	}
	foundTarget := false
	for _, event := range events {
		if event.Kind == tuiapp.StreamToolStart && event.CallID == "call_1" && event.Target == path {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Fatalf("events missing tool target/call id: %#v", events)
	}
	if !hasTUIEvent(events, tuiapp.StreamTextDelta, "edited") {
		t.Fatalf("events = %#v", events)
	}
}

func TestFullscreenAgentPathAppendsSessionStore(t *testing.T) {
	dir := t.TempDir()
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	store, err := (sessionstore.Store{ProjectRoot: dir}).Create()
	if err != nil {
		t.Fatalf("Create session store: %v", err)
	}
	session := chat.NewSession()
	bindSessionStore(session, store, io.Discard)
	fp := &scriptedAppProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventText, Text: "hello"}},
	}}
	loop := tui.Loop{
		Session:  session,
		Provider: fp,
		Registry: registry,
		Tools:    registry.Definitions(),
	}
	events := drainTUIEvents(runAgentForTUI(context.Background(), loop, "你好", nil))
	if !hasTUIEvent(events, tuiapp.StreamTextDelta, "hello") {
		t.Fatalf("events = %#v", events)
	}
	meta, err := store.Meta()
	if err != nil {
		t.Fatalf("Meta returned error: %v", err)
	}
	if meta.MessageCount != 2 || meta.Title != "你好" {
		t.Fatalf("meta = %#v", meta)
	}
	raw, err := os.ReadFile(filepath.Join(sessionstore.SessionDir(dir, store.ID), sessionstore.MessagesFile))
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if strings.Count(string(raw), "\n") != 2 || !strings.Contains(string(raw), "hello") {
		t.Fatalf("messages jsonl = %s", raw)
	}
}

func testLoopForApp(registry *tool.Registry, fp provider.Provider, root string) tui.Loop {
	return tui.Loop{
		Session:           chat.NewSession(),
		Provider:          fp,
		Registry:          registry,
		Tools:             registry.Definitions(),
		PermissionChecker: &permissions.Checker{Root: root, Session: permissions.NewSessionStore()},
		NoTypeDelay:       true,
	}
}

func appMessagesText(messages []chat.Message) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Content)
	}
	return b.String()
}

func initAppRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runAppGit(t, dir, "init")
	runAppGit(t, dir, "config", "user.email", "test@example.com")
	runAppGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".mewcode/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAppGit(t, dir, "add", ".gitignore", "README.md")
	runAppGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runAppGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func drainTUIEvents(events <-chan tuiapp.StreamEvent) []tuiapp.StreamEvent {
	var out []tuiapp.StreamEvent
	for event := range events {
		out = append(out, event)
	}
	return out
}

func hasTUIEvent(events []tuiapp.StreamEvent, kind tuiapp.StreamKind, text string) bool {
	for _, event := range events {
		if event.Kind == kind && strings.Contains(event.Text, text) {
			return true
		}
	}
	return false
}

func hasTUIToolEvent(events []tuiapp.StreamEvent, kind tuiapp.StreamKind, name string) bool {
	for _, event := range events {
		if event.Kind == kind && event.ToolName == name {
			return true
		}
	}
	return false
}

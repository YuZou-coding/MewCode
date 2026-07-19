package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

type fakeTool struct{ name string }

func (f fakeTool) Definition() tool.Definition {
	return tool.Definition{Name: f.name, Description: f.name, Schema: tool.ObjectSchema(nil, map[string]any{})}
}

func (f fakeTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	return tool.OK(map[string]any{"name": f.name})
}

func TestLoadDiscoversRolesAndAppliesPriority(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	writeRole(t, filepath.Join(home, ".mewcode", UserDirName, "general.md"), "general", "user general", "")
	writeRole(t, filepath.Join(project, ProjectDir, "general.md"), "general", "project general", "")
	writeRole(t, filepath.Join(project, ProjectDir, "plan", EntryFileName), "planx", "directory plan", "tools_allow:\n  - read_file\n")
	writeFile(t, filepath.Join(project, ProjectDir, "bad.md"), "---\nname:\n---\nbody")

	loaded := Load(project, home, Options{})
	if len(loaded.Warnings) == 0 {
		t.Fatalf("expected warning for bad file")
	}
	general, ok := loaded.Roles["general"]
	if !ok || general.Description != "project general" || general.Source != SourceProject {
		t.Fatalf("general = %#v ok=%v", general, ok)
	}
	plan, ok := loaded.Roles["planx"]
	if !ok || plan.Path == "" || !strings.HasSuffix(plan.Path, EntryFileName) || strings.Join(plan.ToolsAllow, ",") != "read_file" {
		t.Fatalf("directory role = %#v ok=%v", plan, ok)
	}
	if _, ok := loaded.Roles["explore"]; !ok {
		t.Fatalf("missing builtin explore")
	}
	if _, ok := loaded.Roles["verify"]; ok {
		t.Fatalf("verify should be disabled by default")
	}
	loaded = Load(project, home, Options{EnableVerify: true})
	if _, ok := loaded.Roles["verify"]; !ok {
		t.Fatalf("verify should be enabled")
	}
}

func TestParseFrontmatterFields(t *testing.T) {
	role, err := ParseMarkdown("worker.md", SourceProject, []byte(`---
name: Explore
description: Explore code
tools_allow: [read_file, search_code]
tools_deny:
  - run_command
model: worker-model
max_iterations: 3
permission_mode: strict
background_tools: [read_file]
isolation: worktree
---
Use tools.
`))
	if err != nil {
		t.Fatalf("ParseMarkdown returned error: %v", err)
	}
	if role.Name != "explore" || role.Model != "worker-model" || role.MaxIterations != 3 || role.PermissionMode != PermissionStrict || role.Isolation != IsolationWorktree {
		t.Fatalf("role = %#v", role)
	}
	if strings.Join(role.ToolsAllow, ",") != "read_file,search_code" || strings.Join(role.ToolsDeny, ",") != "run_command" || strings.Join(role.BackgroundTools, ",") != "read_file" {
		t.Fatalf("lists = %#v %#v %#v", role.ToolsAllow, role.ToolsDeny, role.BackgroundTools)
	}
	if !strings.Contains(role.Body, "Use tools.") {
		t.Fatalf("body = %q", role.Body)
	}
}

func TestFilterDefinitionsRemovesRunWorkerAndAppliesAllowDenyBackground(t *testing.T) {
	defs := []tool.Definition{{Name: "read_file"}, {Name: "write_file"}, {Name: RunWorkerToolName}, {Name: "search_code"}}
	role := Role{ToolsAllow: []string{"read_file", "write_file", "search_code"}, ToolsDeny: []string{"write_file"}, BackgroundTools: []string{"search_code"}}
	if got := names(FilterDefinitions(defs, role, false)); got != "read_file,search_code" {
		t.Fatalf("foreground tools = %s", got)
	}
	if got := names(FilterDefinitions(defs, role, true)); got != "search_code" {
		t.Fatalf("background tools = %s", got)
	}
}

func TestRunWorkerDefinitionListsAvailableRoles(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{
		"explore": {Name: "explore", Description: "Explore", Source: SourceBuiltin},
		"custom":  {Name: "custom", Description: "Custom review", Source: SourceProject, ToolsAllow: []string{"read_file"}},
	}}, Options{})
	schema := RunWorkerTool{Manager: manager}.Definition().Schema
	role := schema["properties"].(map[string]any)["role"].(map[string]any)
	if got := role["enum"]; got == nil {
		t.Fatal("role schema does not expose available roles")
	}
	if !strings.Contains(role["description"].(string), "custom") || !strings.Contains(role["description"].(string), "explore") {
		t.Fatalf("role description = %q", role["description"])
	}
}

func TestContextMessagesIncludeAvailableRoles(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{
		"custom": {Name: "custom", Description: "Custom review", Source: SourceProject},
	}}, Options{})
	messages := manager.ContextMessages()
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "custom") || !strings.Contains(messages[0].Content, "project") {
		t.Fatalf("context messages = %#v", messages)
	}
}

func TestManagerRunsForegroundBackgroundCancelAndNotifications(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{"explore": {Name: "explore", Description: "Explore"}}}, Options{BackgroundThreshold: time.Hour})
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		if req.RoleName != "explore" || req.Task != "inspect" {
			t.Fatalf("request = %#v", req)
		}
		return RunResult{Text: "done", Usage: provider.Usage{InputTokens: 2, OutputTokens: 3}}
	}
	result := manager.Run(context.Background(), RunRequest{Task: "inspect", RoleName: "explore"})
	if !result.OK || result.TaskID == "" || result.Result != "done" {
		t.Fatalf("result = %#v", result)
	}
	task, ok := manager.Task(result.TaskID)
	if !ok || task.Status != StatusCompleted || task.Usage.InputTokens != 2 {
		t.Fatalf("task = %#v ok=%v", task, ok)
	}
	if len(manager.DrainNotifications()) != 1 {
		t.Fatalf("expected completion notification")
	}

	block := make(chan struct{})
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		<-block
		return RunResult{Text: "later"}
	}
	bg := manager.Run(context.Background(), RunRequest{Task: "slow", RoleName: "explore", Background: true})
	if !bg.OK || !bg.Background {
		t.Fatalf("background result = %#v", bg)
	}
	if ok := manager.Cancel(bg.TaskID); !ok {
		t.Fatalf("cancel returned false")
	}
	task, _ = manager.Task(bg.TaskID)
	if task.Status != StatusCanceled {
		t.Fatalf("status = %s", task.Status)
	}
	close(block)
}

func TestManagerMovesSlowForegroundWorkerToBackgroundWithoutRestart(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{"explore": {Name: "explore"}}}, Options{BackgroundThreshold: time.Millisecond})
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	runs := 0
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		runs++
		started <- struct{}{}
		<-release
		return RunResult{Text: "finished"}
	}

	result := manager.Run(context.Background(), RunRequest{Task: "slow", RoleName: "explore"})
	<-started
	if !result.OK || !result.Background || result.Status != StatusRunning || result.TaskID == "" {
		t.Fatalf("result = %#v", result)
	}
	if runs != 1 {
		t.Fatalf("worker restarted: runs=%d", runs)
	}
	close(release)
	deadline := time.After(time.Second)
	for {
		task, _ := manager.Task(result.TaskID)
		if task.Status == StatusCompleted {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("task did not complete: %#v", task)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if runs != 1 {
		t.Fatalf("worker restarted after background handoff: runs=%d", runs)
	}
}

func TestWaitForRunningWorkersBlocksUntilCompletion(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{"explore": {Name: "explore"}}}, Options{})
	started := make(chan struct{})
	finish := make(chan struct{})
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		close(started)
		select {
		case <-finish:
			return RunResult{Text: "done"}
		case <-ctx.Done():
			return RunResult{Error: ctx.Err()}
		}
	}
	result := manager.Run(context.Background(), RunRequest{Task: "inspect", RoleName: "explore", Background: true})
	if !result.OK {
		t.Fatalf("background run = %#v", result)
	}
	<-started
	done := make(chan struct{})
	go func() { manager.WaitForRunning(context.Background()); close(done) }()
	select {
	case <-done:
		t.Fatal("wait returned before worker completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(finish)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("wait did not return after worker completed")
	}
}

func TestRunWorkerToolUsesRoleAndForkDefaults(t *testing.T) {
	manager := NewManager(LoadResult{Roles: map[string]Role{"explore": {Name: "explore"}}}, Options{})
	var requests []RunRequest
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		requests = append(requests, req)
		return RunResult{Text: "ok"}
	}
	raw, _ := json.Marshal(map[string]any{"task": "inspect", "role": "explore", "model": "m2", "max_iterations": 4, "isolation": "worktree"})
	result := RunWorkerTool{Manager: manager}.Execute(context.Background(), tool.Input{Arguments: raw})
	if !result.OK || requests[0].Fork || requests[0].Model != "m2" || requests[0].MaxIterations != 4 || requests[0].Isolation != IsolationWorktree {
		t.Fatalf("result=%#v requests=%#v", result, requests)
	}

	manager.ParentMessages = []chat.Message{{Role: chat.RoleUser, Content: "parent"}}
	called := make(chan struct{}, 1)
	manager.Runner = func(ctx context.Context, req RunRequest) RunResult {
		requests = append(requests, req)
		called <- struct{}{}
		return RunResult{Text: "ok"}
	}
	raw, _ = json.Marshal(map[string]any{"task": "fork task", "background": false})
	result = RunWorkerTool{Manager: manager}.Execute(context.Background(), tool.Input{Arguments: raw})
	<-called
	if !result.OK || !requests[1].Fork || !requests[1].Background || len(requests[1].ParentMessages) != 1 {
		t.Fatalf("fork result=%#v request=%#v", result, requests[1])
	}
	if !strings.Contains(ForkInstruction("x"), "不能再 fork") || !strings.Contains(ForkInstruction("x"), "不要请求确认") || !strings.Contains(ForkInstruction("x"), "结构化字段输出") {
		t.Fatalf("fork instruction missing constraints")
	}
}

func writeRole(t *testing.T, path, name, description, extra string) {
	t.Helper()
	writeFile(t, path, "---\nname: "+name+"\ndescription: "+description+"\n"+extra+"---\nBody for "+name+"\n")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(defs []tool.Definition) string {
	items := make([]string, 0, len(defs))
	for _, def := range defs {
		items = append(items, def.Name)
	}
	return strings.Join(items, ",")
}

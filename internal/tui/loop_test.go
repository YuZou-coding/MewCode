package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/compact"
	"mewcode/internal/hooks"
	"mewcode/internal/memory"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
	"mewcode/internal/skill"
	"mewcode/internal/tool"
	"mewcode/internal/worktree"
)

type fakeProvider struct {
	requests [][]chat.Message
	tools    [][]tool.Definition
	events   []provider.StreamEvent
	series   [][]provider.StreamEvent
	err      error
}

func messagesText(messages []chat.Message) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Content)
	}
	return b.String()
}

func (p *fakeProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	copied := make([]chat.Message, len(messages))
	copy(copied, messages)
	p.requests = append(p.requests, copied)
	copiedTools := make([]tool.Definition, len(tools))
	copy(copiedTools, tools)
	p.tools = append(p.tools, copiedTools)

	selected := p.events
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
	errs <- p.err
	return events, errs
}

func TestLoopHandlesVersionWithoutProviderRequest(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{}
	var out strings.Builder

	err = Loop{
		Input:       strings.NewReader("/version\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "MewCode dev") {
		t.Fatalf("output missing version: %q", out.String())
	}
	if len(fp.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(fp.requests))
	}
}

func TestLoopExecutesOneToolAndFeedsResultBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("hello README"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "read_file", Arguments: []byte(`{"path":"` + path + `"}`)}}},
		{{Kind: provider.EventText, Text: "README loaded"}},
	}}
	session := chat.NewSession()
	var out strings.Builder

	err = Loop{
		Input:       strings.NewReader("read readme\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     session,
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(fp.requests))
	}
	second := fp.requests[1]
	toolResult := findToolResult(second)
	if !hasAssistantToolCall(second) || toolResult == nil {
		t.Fatalf("second request missing tool history: %#v", second)
	}
	if !strings.Contains(string(toolResult.Content), "hello README") {
		t.Fatalf("tool result = %s", toolResult.Content)
	}
	if !strings.Contains(out.String(), "README loaded") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLoopAsksBeforeRunningCommand(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "run_command", Arguments: []byte(`{"command":"echo should-not-run"}`)}}},
		{{Kind: provider.EventText, Text: "command denied"}},
	}}
	var out strings.Builder

	err = Loop{
		Input:       strings.NewReader("run it\nn\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Allow command?") {
		t.Fatalf("output missing confirmation: %q", out.String())
	}
	toolResult := findToolResult(fp.requests[1])
	if toolResult == nil || !strings.Contains(string(toolResult.Content), "command_denied") {
		t.Fatalf("tool result = %#v", toolResult)
	}
}

func TestLoopAsksBeforeEditingFileAndDenies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp_tool_test.txt")
	if err := os.WriteFile(path, []byte("hello tool"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"` + path + `","old_text":"hello tool","new_text":"hello MewCode"}`)}}},
		{{Kind: provider.EventText, Text: "edit denied"}},
	}}
	var out strings.Builder

	err = Loop{
		Input:       strings.NewReader("edit file\nn\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Allow tool? edit_file") {
		t.Fatalf("output missing tool confirmation: %q", out.String())
	}
	assertDiskContent(t, path, "hello tool")
	toolResult := findToolResult(fp.requests[1])
	if toolResult == nil || !strings.Contains(string(toolResult.Content), "permission_denied") {
		t.Fatalf("tool result = %#v", toolResult)
	}
}

func TestLoopAsksBeforeEditingFileAndAllows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp_tool_test.txt")
	if err := os.WriteFile(path, []byte("hello tool"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"` + path + `","old_text":"hello tool","new_text":"hello MewCode"}`)}}},
		{{Kind: provider.EventText, Text: "edit done"}},
	}}

	err = Loop{
		Input:       strings.NewReader("edit file\ny\n/exit\n"),
		Output:      io.Discard,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	assertDiskContent(t, path, "hello MewCode")
}

func TestLoopPermissionPromptShowsRepresentativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tmp_tool_test.txt"), []byte("hello tool"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"tmp_tool_test.txt","old_text":"hello tool","new_text":"hello MewCode"}`)}}},
		{{Kind: provider.EventText, Text: "edit denied"}},
	}}
	var out strings.Builder
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	err = Loop{
		Input:       strings.NewReader("edit file\nn\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
		PermissionChecker: &permissions.Checker{
			Root:    dir,
			Session: permissions.NewSessionStore(),
		},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "permission required for edit_file path=tmp_tool_test.txt") {
		t.Fatalf("output missing permission path: %q", out.String())
	}
	if !strings.Contains(out.String(), "Allow edit_file? [n] deny [y] allow once [s] allow session [a] allow always") {
		t.Fatalf("output missing HITL choices: %q", out.String())
	}
}

func TestLoopPermissionPromptShowsCommandSummary(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "run_command", Arguments: []byte(`{"command":"echo hello"}`)}}},
		{{Kind: provider.EventText, Text: "command denied"}},
	}}
	var out strings.Builder

	err = Loop{
		Input:       strings.NewReader("run command\nn\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
		PermissionChecker: &permissions.Checker{
			Root:    t.TempDir(),
			Session: permissions.NewSessionStore(),
		},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "permission required for run_command command=echo hello") {
		t.Fatalf("output missing permission command: %q", out.String())
	}
}

func TestLoopGuardsFinalAnswerAfterReadOnlyToolForModificationRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp_tool_test.txt")
	if err := os.WriteFile(path, []byte("hello tool"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "read_file", Arguments: []byte(`{"path":"` + path + `"}`)}}},
		{{Kind: provider.EventText, Text: "未修改文件"}},
	}}

	err = Loop{
		Input:       strings.NewReader("把 tmp_tool_test.txt 里的 hello tool 改成 hello zy\n/exit\n"),
		Output:      io.Discard,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(fp.requests))
	}
	if len(fp.tools[0]) == 0 {
		t.Fatalf("first request missing tools")
	}
	if len(fp.tools[1]) == 0 {
		t.Fatalf("second request missing tools for agent loop")
	}
	if findToolResult(fp.requests[1]) == nil {
		t.Fatalf("second request missing read_file result: %#v", fp.requests[1])
	}
	assertDiskContent(t, path, "hello tool")
}

func TestLoopStreamsAndStoresConversation(t *testing.T) {
	fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: "hello"}}}
	session := chat.NewSession()
	var out strings.Builder

	err := Loop{
		Input:       strings.NewReader("hi\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     session,
		Provider:    fp,
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !strings.Contains(out.String(), "MewCode > thinking...") {
		t.Fatalf("output missing thinking time status: %q", out.String())
	}
	if !strings.Contains(out.String(), "MewCode > first token in") {
		t.Fatalf("output missing first token timing: %q", out.String())
	}
	if !strings.Contains(out.String(), "MewCode > hello") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "MewCode") || !strings.Contains(out.String(), "/\\_/\\") {
		t.Fatalf("output missing banner: %q", out.String())
	}
	messages := session.Messages()
	if len(messages) != 2 || messages[0].Content != "hi" || messages[1].Content != "hello" {
		t.Fatalf("unexpected session messages: %#v", messages)
	}
}

func TestLoopManualCompactDoesNotStoreCommandAndPrintsStats(t *testing.T) {
	session := chat.NewSession()
	for i := 0; i < 7; i++ {
		session.AddUser("user-" + string(rune('0'+i)))
		session.AddAssistant(strings.Repeat("x", 12000))
	}
	fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: compactSummaryText()}}}
	var out strings.Builder

	err := Loop{
		Input:       strings.NewReader("/compact\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     session,
		Provider:    fp,
		NoTypeDelay: true,
		Compact:     &compact.Manager{Root: t.TempDir(), Provider: fp},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	output := out.String()
	for _, want := range []string{"compacted messages", "chars", "artifacts", "You >"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %q", want, output)
		}
	}
	for _, message := range session.Messages() {
		if message.Role == chat.RoleUser && message.Content == "/compact" {
			t.Fatalf("/compact was stored as user message")
		}
	}
	if len(fp.tools) != 1 || len(fp.tools[0]) != 0 {
		t.Fatalf("manual compact summary got tools: %#v", fp.tools)
	}
}

func TestLoopCommandRegistryHandlesHelpStatusPlanAndUnknown(t *testing.T) {
	fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: "ok"}}}
	session := chat.NewSession()
	engine := hooks.NewEngine([]hooks.Rule{{Name: "status-hook", Event: hooks.EventSessionStart, Action: hooks.Action{Type: hooks.ActionSubAgent, Prompt: "later"}}}, nil)
	_ = engine.Fire(context.Background(), hooks.Context{Event: hooks.EventSessionStart})
	var out strings.Builder
	err := Loop{
		Input:       strings.NewReader("/HELP compact\n/plan\n/status\n/do\n/unknown\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     session,
		Provider:    fp,
		NoTypeDelay: true,
		HookEngine:  engine,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	text := out.String()
	for _, want := range []string{"usage: /compact", "mode=plan", "mode=execute", "messages=0", "hooks(rules=1 warnings=1)", "unknown command /unknown; type /help"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q: %q", want, text)
		}
	}
	if len(fp.requests) != 0 {
		t.Fatalf("local commands triggered provider: %d", len(fp.requests))
	}
	if len(session.Messages()) != 0 {
		t.Fatalf("local commands stored messages: %#v", session.Messages())
	}
}

func TestLoopReviewCommandSendsVisibleUserMessage(t *testing.T) {
	fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: "reviewed"}}}
	session := chat.NewSession()
	var out strings.Builder
	err := Loop{
		Input:        strings.NewReader("/review internal/tui\n/exit\n"),
		Output:       &out,
		Errors:       io.Discard,
		Session:      session,
		Provider:     fp,
		NoTypeDelay:  true,
		SkillManager: skill.NewManager("", "", skill.LoadResult{Skills: map[string]skill.Skill{"review": {Name: "review", Description: "review skill", Mode: skill.ModeShared, Context: skill.ContextRecent, Body: "REVIEW SOP"}}}),
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("provider requests = %d", len(fp.requests))
	}
	messages := session.Messages()
	if len(messages) != 2 || messages[0].Role != chat.RoleUser || !strings.Contains(messages[0].Content, "internal/tui") || !strings.Contains(messages[0].Content, "review Skill") {
		t.Fatalf("review not stored as visible user message: %#v", messages)
	}
	if !strings.Contains(out.String(), "reviewed") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestControllerSkillsShowRunAndClearActive(t *testing.T) {
	session := chat.NewSession()
	manager := skill.NewManager("", "", skill.LoadResult{Skills: map[string]skill.Skill{
		"review": {Name: "review", Description: "review skill", Mode: skill.ModeShared, Context: skill.ContextRecent, Model: "gpt-skill", Tools: []string{"read_file"}, Body: "REVIEW SOP"},
	}})
	loop := Loop{Session: session, Provider: &fakeProvider{}, SkillManager: manager}
	controller := NewController(&loop, context.Background(), io.Discard, io.Discard, session)
	show := controller.Skills(context.Background(), "show review")
	for _, want := range []string{"description=review skill", "mode=shared", "model=gpt-skill", "context=recent", "tools=read_file"} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q: %s", want, show)
		}
	}
	prompt, err := controller.RunSkill(context.Background(), "review", "internal/agent")
	if err != nil {
		t.Fatalf("RunSkill: %v", err)
	}
	if !strings.Contains(prompt, "review Skill") || !strings.Contains(prompt, "internal/agent") || manager.ActiveCount() != 1 {
		t.Fatalf("prompt=%q active=%d", prompt, manager.ActiveCount())
	}
	if err := controller.ClearConversation(); err != nil {
		t.Fatalf("ClearConversation: %v", err)
	}
	if manager.ActiveCount() != 0 {
		t.Fatalf("active skills = %d", manager.ActiveCount())
	}
}

func TestControllerRunsIsolatedSkillWithContextStrategies(t *testing.T) {
	for _, tc := range []struct {
		name        string
		strategy    skill.ContextStrategy
		want        string
		notWant     string
		historySize int
	}{
		{name: "empty", strategy: skill.ContextEmpty, notWant: "old-0", historySize: 8},
		{name: "recent", strategy: skill.ContextRecent, want: "old-7", notWant: "old-0", historySize: 8},
		{name: "summary", strategy: skill.ContextFullSummary, want: "主会话摘要", historySize: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: "isolated done"}}}
			session := chat.NewSession()
			for i := 0; i < tc.historySize; i++ {
				session.AddUser(fmt.Sprintf("old-%d", i))
			}
			manager := skill.NewManager("", "", skill.LoadResult{Skills: map[string]skill.Skill{
				"test": {Name: "test", Description: "test skill", Mode: skill.ModeIsolated, Context: tc.strategy, Body: "TEST SOP"},
			}})
			loop := Loop{Session: session, Provider: fp, Registry: tool.NewRegistry(), SkillManager: manager}
			controller := NewController(&loop, context.Background(), io.Discard, io.Discard, session)
			result, err := controller.RunSkill(context.Background(), "test", "run tests")
			if err != nil {
				t.Fatalf("RunSkill: %v", err)
			}
			if !strings.Contains(result, "isolated skill test summary: isolated done") {
				t.Fatalf("result = %q", result)
			}
			joined := messagesText(fp.requests[0])
			if tc.want != "" && !strings.Contains(joined, tc.want) {
				t.Fatalf("request missing %q: %s", tc.want, joined)
			}
			if tc.notWant != "" && strings.Contains(joined, tc.notWant) {
				t.Fatalf("request contains %q: %s", tc.notWant, joined)
			}
		})
	}
}

func TestLoopPermissionsCommandAndClearSession(t *testing.T) {
	checker := &permissions.Checker{
		Mode:        permissions.ModeDefault,
		DefaultMode: permissions.ModeDefault,
		Session:     permissions.NewSessionStore(),
		Project:     []permissions.Rule{{Effect: permissions.EffectDeny, Tool: "edit_file"}},
		User:        []permissions.Rule{{Effect: permissions.EffectAllow, Tool: "read_file"}},
	}
	checker.Session.Add(permissions.Rule{Effect: permissions.EffectAllow, Tool: "run_command"})
	fp := &fakeProvider{events: []provider.StreamEvent{{Kind: provider.EventText, Text: "ok"}}}
	var out strings.Builder
	err := Loop{
		Input:             strings.NewReader("/permissions\n/permissions mode yolo\n/permissions mode strict\n/permissions mode reset\n/permissions clear-session\n/permissions\n/exit\n"),
		Output:            &out,
		Errors:            io.Discard,
		Session:           chat.NewSession(),
		Provider:          fp,
		PermissionChecker: checker,
		NoTypeDelay:       true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "permissions mode=default default_mode=default user=1 project=1 session=1") || !strings.Contains(text, "permission mode=yolo") || !strings.Contains(text, "permission mode=strict") || !strings.Contains(text, "cleared session permissions") || !strings.Contains(text, "permissions mode=default default_mode=default user=1 project=1 session=0") {
		t.Fatalf("permissions output = %q", text)
	}
	if len(checker.Session.Rules()) != 0 {
		t.Fatalf("session rules not cleared")
	}
}

func TestLoopStrictPermissionPromptOnlyOffersOnce(t *testing.T) {
	registry := tool.NewRegistry()
	called := false
	if err := registry.Register(permissionCountingTool{name: "edit_file", called: &called}); err != nil {
		t.Fatalf("register: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{}`)}}}, {{Kind: provider.EventText, Text: "done"}}}}
	var out strings.Builder
	err := Loop{
		Input:       strings.NewReader("edit\ny\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		Registry:    registry,
		Tools:       registry.Definitions(),
		NoTypeDelay: true,
		PermissionChecker: &permissions.Checker{
			Root:        t.TempDir(),
			Mode:        permissions.ModeStrict,
			DefaultMode: permissions.ModeStrict,
			Session:     permissions.NewSessionStore(),
		},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !called {
		t.Fatal("edit tool was not executed after once approval")
	}
	if text := out.String(); !strings.Contains(text, "[n] deny [y] allow once") || strings.Contains(text, "allow session") || strings.Contains(text, "allow always") {
		t.Fatalf("strict prompt = %q", text)
	}
}

type permissionCountingTool struct {
	name   string
	called *bool
}

func (t permissionCountingTool) Definition() tool.Definition {
	return tool.Definition{Name: t.name, Schema: tool.ObjectSchema(nil, nil)}
}

func (t permissionCountingTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	*t.called = true
	return tool.OK(nil)
}

func assertDiskContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func hasAssistantToolCall(messages []chat.Message) bool {
	for _, message := range messages {
		if message.ToolCall != nil || len(message.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func findToolResult(messages []chat.Message) *chat.ToolResult {
	for _, message := range messages {
		if message.ToolResult != nil {
			return message.ToolResult
		}
	}
	return nil
}

func TestLoopDoesNotStorePartialAssistantOnError(t *testing.T) {
	fp := &fakeProvider{
		events: []provider.StreamEvent{{Kind: provider.EventText, Text: "partial"}},
		err:    errors.New("malformed SSE event"),
	}
	session := chat.NewSession()
	var errs strings.Builder

	err := Loop{
		Input:       strings.NewReader("hi\n/exit\n"),
		Output:      io.Discard,
		Errors:      &errs,
		Session:     session,
		Provider:    fp,
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	messages := session.Messages()
	if len(messages) != 1 || messages[0].Role != chat.RoleUser {
		t.Fatalf("unexpected session messages: %#v", messages)
	}
	if !strings.Contains(errs.String(), "malformed SSE event") {
		t.Fatalf("errors = %q", errs.String())
	}
}

func TestLoopShowsThinkingStatus(t *testing.T) {
	fp := &fakeProvider{events: []provider.StreamEvent{
		{Kind: provider.EventThinking, Text: "hidden"},
		{Kind: provider.EventText, Text: "answer"},
	}}
	var out strings.Builder

	err := Loop{
		Input:       strings.NewReader("hi\n/exit\n"),
		Output:      &out,
		Errors:      io.Discard,
		Session:     chat.NewSession(),
		Provider:    fp,
		NoTypeDelay: true,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "MewCode > thinking...") {
		t.Fatalf("output missing thinking status: %q", out.String())
	}
	if strings.Contains(out.String(), "hidden") {
		t.Fatalf("raw thinking leaked: %q", out.String())
	}
}

func TestLoopWorktreesEnterExitRefreshesCWD(t *testing.T) {
	repo := initLoopRepo(t)
	realRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventText, Text: "inside"}},
		{{Kind: provider.EventText, Text: "outside"}},
	}}
	manager := worktree.NewManager(repo, worktree.Config{})
	var out strings.Builder

	err = Loop{
		Input:           strings.NewReader("/worktrees create feature/foo\n/worktrees enter feature/foo\nhi\n/worktrees exit\nhi again\n/exit\n"),
		Output:          &out,
		Errors:          io.Discard,
		Session:         chat.NewSession(),
		Provider:        fp,
		NoTypeDelay:     true,
		WorktreeManager: manager,
		Compact:         &compact.Manager{Root: repo, Provider: fp},
		Notes:           &memory.Notes{HomeDir: t.TempDir(), ProjectRoot: repo},
		PermissionChecker: &permissions.Checker{
			Root:    repo,
			Session: permissions.NewSessionStore(),
		},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v\noutput=%s", err, out.String())
	}
	if len(fp.requests) != 2 {
		t.Fatalf("requests = %d output=%s", len(fp.requests), out.String())
	}
	inside := messagesText(fp.requests[0])
	outside := messagesText(fp.requests[1])
	if !strings.Contains(inside, filepath.Join(repo, ".mewcode", "worktrees", "feature", "foo")) {
		t.Fatalf("inside request missing worktree cwd: %s", inside)
	}
	if !strings.Contains(outside, "cwd: "+realRepo) {
		t.Fatalf("outside request missing repo cwd: %s", outside)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(250 * time.Millisecond); got != "250ms" {
		t.Fatalf("formatDuration = %q, want 250ms", got)
	}
	if got := formatDuration(1500 * time.Millisecond); got != "1.5s" {
		t.Fatalf("formatDuration = %q, want 1.5s", got)
	}
}

func TestWriteTypewriter(t *testing.T) {
	var out strings.Builder
	written, err := writeTypewriter(context.Background(), &out, "你好", 0)
	if err != nil {
		t.Fatalf("writeTypewriter returned error: %v", err)
	}
	if out.String() != "你好" {
		t.Fatalf("output = %q", out.String())
	}
	if written != len("你好") {
		t.Fatalf("written = %d, want %d", written, len("你好"))
	}
}

func compactSummaryText() string {
	return "【分析草稿】draft\n【正式摘要】\n主要请求: req\n关键概念: concept\n文件代码: code\n错误修复: fix\n解决过程: process\n用户原话: quote\n待办: todo\n当前工作: current\n下一步: next"
}

func TestPrintBanner(t *testing.T) {
	var out strings.Builder
	if err := PrintBanner(&out); err != nil {
		t.Fatalf("PrintBanner returned error: %v", err)
	}
	if !strings.Contains(out.String(), "MewCode") {
		t.Fatalf("banner missing name: %q", out.String())
	}
	if !strings.Contains(out.String(), "Type /exit to quit") {
		t.Fatalf("banner missing exit hint: %q", out.String())
	}
}

func initLoopRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runLoopGit(t, dir, "init")
	runLoopGit(t, dir, "config", "user.email", "test@example.com")
	runLoopGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runLoopGit(t, dir, "add", "README.md")
	runLoopGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runLoopGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/compact"
	"mewcode/internal/hooks"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
	"mewcode/internal/skill"
	"mewcode/internal/tool"
	"mewcode/internal/worker"
)

type fakeProvider struct {
	mu       sync.Mutex
	requests [][]chat.Message
	tools    [][]tool.Definition
	series   [][]provider.StreamEvent
	err      error
}

func (p *fakeProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	p.mu.Lock()
	copied := make([]chat.Message, len(messages))
	copy(copied, messages)
	p.requests = append(p.requests, copied)
	copiedTools := make([]tool.Definition, len(tools))
	copy(copiedTools, tools)
	p.tools = append(p.tools, copiedTools)
	selected := []provider.StreamEvent{}
	if len(p.series) > 0 {
		selected = p.series[0]
		p.series = p.series[1:]
	}
	p.mu.Unlock()

	events := make(chan provider.StreamEvent, len(selected))
	errs := make(chan error, 1)
	for _, event := range selected {
		events <- event
	}
	close(events)
	errs <- p.err
	return events, errs
}

func (p *fakeProvider) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

type sleepTool struct {
	name     string
	duration time.Duration
	mu       *sync.Mutex
	order    *[]string
}

type countingTool struct {
	name   string
	called *bool
}

func (c countingTool) Definition() tool.Definition {
	return tool.Definition{Name: c.name, Description: c.name, Schema: tool.ObjectSchema(nil, map[string]any{})}
}

func (c countingTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	*c.called = true
	return tool.OK(map[string]any{"called": true})
}

func (s sleepTool) Definition() tool.Definition {
	return tool.Definition{Name: s.name, Description: s.name, Schema: tool.ObjectSchema(nil, map[string]any{})}
}

func (s sleepTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	if s.mu != nil {
		s.mu.Lock()
		*s.order = append(*s.order, s.name)
		s.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		return tool.Fail("cancelled", ctx.Err().Error())
	case <-time.After(s.duration):
		return tool.OK(map[string]any{"name": s.name})
	}
}

func TestRunReturnsBufferedEventChannelQuickly(t *testing.T) {
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventText, Text: "done"}}}}
	agent := &Agent{Provider: fp, Session: chat.NewSession(), MaxIterations: 1}
	start := time.Now()
	events := agent.Run(context.Background(), "hi")
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("Run blocked too long")
	}
	if cap(events) != EventBufferSize {
		t.Fatalf("event channel cap = %d, want %d", cap(events), EventBufferSize)
	}
	drain(events)
}

func TestReactLoopRunsUntilNoToolCalls(t *testing.T) {
	registry := tool.NewRegistry()
	_ = registry.Register(sleepTool{name: "read_file", duration: 1 * time.Millisecond})
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: call("call_1", "read_file")}},
		{{Kind: provider.EventText, Text: "done"}},
	}}
	session := chat.NewSession()
	agent := &Agent{Provider: fp, Registry: registry, Session: session, Tools: registry.Definitions()}

	events := collect(agent.Run(context.Background(), "read"))
	if fp.requestCount() != 2 {
		t.Fatalf("requests = %d, want 2", fp.requestCount())
	}
	if !hasEvent(events, EventFinalResponse) {
		t.Fatalf("missing final response: %#v", events)
	}
	if len(session.Messages()) != 4 {
		t.Fatalf("session messages = %#v", session.Messages())
	}
}

func TestAgentInjectsEnvironmentAndPlanOnlyReminder(t *testing.T) {
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventText, Text: "计划"}}}}
	agent := &Agent{Provider: fp, Session: chat.NewSession(), PlanOnly: true}

	drain(agent.Run(context.Background(), "plan"))
	if fp.requestCount() != 1 {
		t.Fatalf("requests = %d, want 1", fp.requestCount())
	}
	request := fp.requests[0]
	if len(request) < 3 {
		t.Fatalf("request too short: %#v", request)
	}
	if request[0].Role != chat.RoleSystem || !strings.Contains(request[0].Content, "cwd") {
		t.Fatalf("missing environment message: %#v", request)
	}
	if request[1].Role != chat.RoleSystem || !strings.Contains(request[1].Content, "只允许读类工具") {
		t.Fatalf("missing plan-only reminder: %#v", request)
	}
}

func TestAgentInjectsSkillSummaryThenActiveSOPAndFiltersTools(t *testing.T) {
	registry := tool.NewRegistry()
	manager := skill.NewManager("", "", skill.LoadResult{Skills: map[string]skill.Skill{
		"review": {Name: "review", Description: "Review code", Tools: []string{"read_file"}, Body: "REVIEW-SOP"},
	}})
	if err := registry.Register(skill.LoadTool{Manager: manager}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(countingTool{name: "read_file", called: new(bool)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(countingTool{name: "write_file", called: new(bool)}); err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "load_1", Name: skill.LoadToolName, Arguments: []byte(`{"name":"review"}`)}}},
		{{Kind: provider.EventText, Text: "done"}},
	}}
	agent := &Agent{Provider: fp, Registry: registry, Session: chat.NewSession(), Tools: registry.Definitions(), SkillManager: manager}

	drain(agent.Run(context.Background(), "review this"))
	if len(fp.requests) != 2 {
		t.Fatalf("requests = %d", len(fp.requests))
	}
	first := messagesText(fp.requests[0])
	if !strings.Contains(first, "review: Review code") || strings.Contains(first, "REVIEW-SOP") {
		t.Fatalf("first request = %s", first)
	}
	second := messagesText(fp.requests[1])
	if !strings.Contains(second, "REVIEW-SOP") {
		t.Fatalf("second request missing SOP: %s", second)
	}
	if strings.Contains(messagesText(agent.Session.Messages()), "REVIEW-SOP") {
		t.Fatalf("SOP leaked into session: %#v", agent.Session.Messages())
	}
	if toolNames(fp.tools[1]) != "load_skill,read_file" {
		t.Fatalf("tools = %s", toolNames(fp.tools[1]))
	}
}

func TestAgentEmitsUsageEvent(t *testing.T) {
	fp := &fakeProvider{series: [][]provider.StreamEvent{{
		{Kind: provider.EventUsage, Usage: provider.Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 128, CacheWriteTokens: 256}},
		{Kind: provider.EventText, Text: "done"},
	}}}
	agent := &Agent{Provider: fp, Session: chat.NewSession()}

	events := collect(agent.Run(context.Background(), "hi"))
	var usage *Usage
	for _, event := range events {
		if event.Kind == EventUsage {
			u := event.Usage
			usage = &u
		}
	}
	if usage == nil {
		t.Fatalf("missing UsageEvent: %#v", events)
	}
	if usage.InputTokens != 1 || usage.OutputTokens != 2 || usage.CacheReadTokens != 128 || usage.CacheWriteTokens != 256 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestAgentCompactsBeforeProviderRequest(t *testing.T) {
	session := chat.NewSession()
	for i := 0; i < 7; i++ {
		session.AddUser("user-" + string(rune('0'+i)))
		session.AddAssistant(strings.Repeat("x", 12000))
	}
	session.AddToolResult(chat.ToolResult{CallID: "call_big", Name: "read_file", Content: json.RawMessage(strings.Repeat("b", compact.SingleToolResultThreshold+1))})
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventText, Text: compactSummaryText()}},
		{{Kind: provider.EventText, Text: "done"}},
	}}
	agent := &Agent{
		Provider: fp,
		Session:  session,
		Tools:    []tool.Definition{{Name: "read_file"}},
		Compact:  &compact.Manager{Root: t.TempDir(), Provider: fp},
	}

	events := collect(agent.Run(context.Background(), "latest user text"))
	if !hasEvent(events, EventFinalResponse) {
		t.Fatalf("events = %#v", events)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("requests = %d, want summary + business", len(fp.requests))
	}
	if len(fp.requests[0]) != 1 || !strings.Contains(fp.requests[0][0].Content, "禁止调用工具") {
		t.Fatalf("summary request = %#v", fp.requests[0])
	}
	business := fp.requests[1]
	joined := messagesText(business)
	if strings.Contains(joined, strings.Repeat("b", compact.SingleToolResultThreshold+1)) {
		t.Fatalf("business request contains full large tool result")
	}
	if !strings.Contains(joined, compact.ArtifactDir) || !strings.Contains(joined, "上下文压缩摘要") || !strings.Contains(joined, "不要根据摘要脑补代码") || !strings.Contains(joined, "latest user text") {
		t.Fatalf("business request missing compacted context: %.500s", joined)
	}
}

func TestMaxIterationsGeneratesToolFreeFinalReport(t *testing.T) {
	registry := tool.NewRegistry()
	_ = registry.Register(sleepTool{name: "read_file"})
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: call("call_1", "read_file")}},
		{{Kind: provider.EventToolCall, ToolCall: call("call_2", "read_file")}},
		{{Kind: provider.EventToolCall, ToolCall: call("ignored", "read_file")}, {Kind: provider.EventText, Text: "completed: partial; next: continue"}},
	}}
	agent := &Agent{Provider: fp, Registry: registry, Session: chat.NewSession(), Tools: registry.Definitions(), MaxIterations: 2}

	events := collect(agent.Run(context.Background(), "loop"))
	last := events[len(events)-1]
	if last.Kind != EventFinalResponse || last.Text != "completed: partial; next: continue" {
		t.Fatalf("last event = %#v", last)
	}
	if len(fp.tools) != 3 || len(fp.tools[2]) != 0 {
		t.Fatalf("final report tools = %#v", fp.tools)
	}
	if !strings.Contains(messagesText(fp.requests[2]), "Do not call any tools") {
		t.Fatalf("final report prompt = %#v", fp.requests[2])
	}
	for _, event := range events {
		if event.Kind == EventToolCallStart && event.ToolCallID == "ignored" {
			t.Fatalf("final report tool call leaked: %#v", event)
		}
	}
}

func TestReadToolsRunConcurrentlyAndWriteToolsSequentially(t *testing.T) {
	registry := tool.NewRegistry()
	var mu sync.Mutex
	var order []string
	_ = registry.Register(sleepTool{name: "read_file", duration: 100 * time.Millisecond})
	_ = registry.Register(sleepTool{name: "search_code", duration: 100 * time.Millisecond})
	_ = registry.Register(sleepTool{name: "write_file", duration: 100 * time.Millisecond, mu: &mu, order: &order})
	_ = registry.Register(sleepTool{name: "edit_file", duration: 100 * time.Millisecond, mu: &mu, order: &order})

	agent := &Agent{Registry: registry}
	readStart := time.Now()
	agent.executeToolBatch(context.Background(), []provider.ToolCall{*call("r1", "read_file"), *call("r2", "search_code")}, make(chan Event, EventBufferSize))
	if time.Since(readStart) >= 250*time.Millisecond {
		t.Fatalf("read tools did not run concurrently")
	}

	writeStart := time.Now()
	agent.executeToolBatch(context.Background(), []provider.ToolCall{*call("w1", "write_file"), *call("w2", "edit_file")}, make(chan Event, EventBufferSize))
	if time.Since(writeStart) < 200*time.Millisecond {
		t.Fatalf("write tools did not run sequentially")
	}
	if len(order) != 2 || order[0] != "write_file" || order[1] != "edit_file" {
		t.Fatalf("order = %#v", order)
	}
}

func TestPlanOnlyBlocksWriteTools(t *testing.T) {
	registry := tool.NewRegistry()
	_ = registry.Register(sleepTool{name: "edit_file"})
	agent := &Agent{Registry: registry, PlanOnly: true}

	result := agent.executeSingleTool(context.Background(), *call("call_1", "edit_file"), make(chan Event, EventBufferSize))
	if result.OK || result.Error == nil || result.Error.Code != "plan_only_blocked" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHookBeforeToolBlockFeedsReasonBackToModel(t *testing.T) {
	registry := tool.NewRegistry()
	called := false
	_ = registry.Register(countingTool{name: "edit_file", called: &called})
	engine := hooks.NewEngine([]hooks.Rule{{
		Name:  "block-secret",
		Event: hooks.EventToolBeforeExecute,
		Conditions: hooks.Conditions{All: []hooks.Clause{
			{Field: "tool.name", Op: hooks.OpEq, Value: "edit_file"},
			{Field: "tool.args.path", Op: hooks.OpGlob, Value: "*.secret"},
		}},
		Block:  "blocked path {{tool.args.path}}",
		Action: hooks.Action{Type: hooks.ActionInjectPrompt, Prompt: "unused"},
	}}, nil)
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"tmp.secret"}`)}}},
		{{Kind: provider.EventText, Text: "adjusted"}},
	}}
	agent := &Agent{Provider: fp, Registry: registry, Session: chat.NewSession(), Tools: registry.Definitions(), HookEngine: engine}

	events := collect(agent.Run(context.Background(), "edit"))
	if called {
		t.Fatalf("tool executed despite hook block")
	}
	if !hasEvent(events, EventFinalResponse) {
		t.Fatalf("events = %#v", events)
	}
	if len(fp.requests) != 2 || !strings.Contains(messagesText(fp.requests[1]), "hook_blocked") || !strings.Contains(messagesText(fp.requests[1]), "blocked path tmp.secret") {
		t.Fatalf("second request missing hook result: %#v", fp.requests)
	}
}

func TestHookInjectPromptBeforeSendDoesNotEnterSessionHistory(t *testing.T) {
	engine := hooks.NewEngine([]hooks.Rule{{
		Name:   "inject",
		Event:  hooks.EventMessageBeforeSend,
		Action: hooks.Action{Type: hooks.ActionInjectPrompt, Prompt: "HOOK {{message.content}}"},
	}}, nil)
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventText, Text: "ok"}}}}
	session := chat.NewSession()
	agent := &Agent{Provider: fp, Session: session, HookEngine: engine}

	drain(agent.Run(context.Background(), "hello"))
	if len(fp.requests) != 1 || !strings.Contains(messagesText(fp.requests[0]), "HOOK hello") {
		t.Fatalf("request missing injected prompt: %#v", fp.requests)
	}
	if strings.Contains(messagesText(session.Messages()), "HOOK hello") {
		t.Fatalf("hook prompt leaked into session: %#v", session.Messages())
	}
}

func TestAgentInjectsWorkerNotificationsWithoutSessionLeak(t *testing.T) {
	manager := worker.NewManager(worker.LoadResult{Roles: map[string]worker.Role{}}, worker.Options{})
	manager.EnqueueNotification(worker.Notification{TaskID: "worker_1", Role: "fork", Status: string(worker.StatusCompleted), Result: "background result"})
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventText, Text: "ok"}}}}
	session := chat.NewSession()
	agent := &Agent{Provider: fp, Session: session, WorkerManager: manager}

	drain(agent.Run(context.Background(), "hello"))
	if len(fp.requests) != 1 || !strings.Contains(messagesText(fp.requests[0]), "worker_1") || !strings.Contains(messagesText(fp.requests[0]), "background result") {
		t.Fatalf("request missing worker notification: %#v", fp.requests)
	}
	if strings.Contains(messagesText(session.Messages()), "worker_1") {
		t.Fatalf("worker notification leaked into session: %#v", session.Messages())
	}
	if len(manager.DrainNotifications()) != 0 {
		t.Fatalf("notification should have been drained")
	}
}

func TestAgentRunWorkerToolReturnsForegroundResult(t *testing.T) {
	manager := worker.NewManager(worker.LoadResult{Roles: map[string]worker.Role{
		"explore": {Name: "explore", Description: "Explore"},
	}}, worker.Options{BackgroundThreshold: time.Hour})
	manager.Runner = func(ctx context.Context, req worker.RunRequest) worker.RunResult {
		if req.RoleName != "explore" || req.Task != "inspect" || req.Fork {
			t.Fatalf("request = %#v", req)
		}
		return worker.RunResult{Text: "worker final"}
	}
	registry := tool.NewRegistry()
	if err := worker.RegisterTools(registry, manager); err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "worker_call", Name: worker.RunWorkerToolName, Arguments: []byte(`{"task":"inspect","role":"explore"}`)}}},
		{{Kind: provider.EventText, Text: "parent done"}},
	}}
	session := chat.NewSession()
	agent := &Agent{Provider: fp, Registry: registry, Session: session, Tools: registry.Definitions(), WorkerManager: manager}

	drain(agent.Run(context.Background(), "delegate"))
	if len(fp.requests) != 2 || !strings.Contains(messagesText(fp.requests[1]), "worker final") {
		t.Fatalf("second request missing worker result: %#v", fp.requests)
	}
}

func TestHookActionFailureDoesNotInterruptConversation(t *testing.T) {
	engine := hooks.NewEngine([]hooks.Rule{{
		Name:   "bad-shell",
		Event:  hooks.EventMessageBeforeSend,
		Action: hooks.Action{Type: hooks.ActionShell, Command: "exit 2"},
	}}, nil)
	fp := &fakeProvider{series: [][]provider.StreamEvent{{{Kind: provider.EventText, Text: "ok"}}}}
	agent := &Agent{Provider: fp, Session: chat.NewSession(), HookEngine: engine}

	events := collect(agent.Run(context.Background(), "hello"))
	if !hasEvent(events, EventFinalResponse) || engine.WarningCount() == 0 {
		t.Fatalf("events=%#v warnings=%#v", events, engine.Warnings())
	}
}

func TestToolTimeoutAndHooks(t *testing.T) {
	registry := tool.NewRegistry()
	_ = registry.Register(sleepTool{name: "read_file", duration: 200 * time.Millisecond})
	agent := &Agent{Registry: registry, ToolTimeout: 50 * time.Millisecond}

	result := agent.executeSingleTool(context.Background(), *call("call_1", "read_file"), make(chan Event, EventBufferSize))
	if result.OK || result.Error == nil || result.Error.Code != "tool_timeout" {
		t.Fatalf("timeout result = %#v", result)
	}

	agent.PreToolHook = func(ctx context.Context, call provider.ToolCall) (bool, string) {
		return false, "blocked"
	}
	result = agent.executeSingleTool(context.Background(), *call("call_2", "read_file"), make(chan Event, EventBufferSize))
	if result.OK || result.Error == nil || result.Error.Code != "hook_blocked" {
		t.Fatalf("hook result = %#v", result)
	}

	postCalled := false
	agent.PreToolHook = nil
	agent.ToolTimeout = time.Second
	agent.PostToolHook = func(ctx context.Context, call provider.ToolCall, result tool.Result) error {
		postCalled = true
		return errors.New("ignored")
	}
	result = agent.executeSingleTool(context.Background(), *call("call_3", "read_file"), make(chan Event, EventBufferSize))
	if !result.OK || !postCalled {
		t.Fatalf("result = %#v postCalled=%v", result, postCalled)
	}
}

func TestPermissionDenySkipsToolExecute(t *testing.T) {
	registry := tool.NewRegistry()
	called := false
	_ = registry.Register(countingTool{name: "edit_file", called: &called})
	agent := &Agent{
		Registry: registry,
		PermissionChecker: &permissions.Checker{
			Root:    t.TempDir(),
			Session: permissions.NewSessionStore(),
			Project: []permissions.Rule{{
				Effect: permissions.EffectDeny,
				Tool:   "edit_file",
				Source: permissions.SourceProject,
			}},
		},
	}

	result := agent.executeSingleTool(context.Background(), *call("call_1", "edit_file"), make(chan Event, EventBufferSize))
	if result.OK || result.Error == nil || result.Error.Code != "permission_denied" {
		t.Fatalf("result = %#v", result)
	}
	if called {
		t.Fatalf("tool Execute was called after permission denial")
	}
}

func TestSandboxDenialSkipsToolExecute(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "read_file", args: map[string]any{"path": "../outside.txt"}},
		{name: "write_file", args: map[string]any{"path": "../outside.txt", "content": "nope"}},
		{name: "edit_file", args: map[string]any{"path": "../outside.txt", "old_text": "a", "new_text": "b"}},
		{name: "search_code", args: map[string]any{"root": "..", "pattern": "MewCode"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := tool.NewRegistry()
			called := false
			_ = registry.Register(countingTool{name: tc.name, called: &called})
			raw, _ := json.Marshal(tc.args)
			agent := &Agent{
				Registry: registry,
				PermissionChecker: &permissions.Checker{
					Root:    t.TempDir(),
					Session: permissions.NewSessionStore(),
					User: []permissions.Rule{{
						Effect: permissions.EffectAllow,
						Tool:   tc.name,
						Source: permissions.SourceUser,
					}},
				},
			}

			result := agent.executeSingleTool(context.Background(), provider.ToolCall{ID: "call_1", Name: tc.name, Arguments: raw}, make(chan Event, EventBufferSize))
			if result.OK || result.Error == nil || result.Error.Code != "path_outside_sandbox" {
				t.Fatalf("result = %#v", result)
			}
			if called {
				t.Fatalf("%s Execute was called after sandbox denial", tc.name)
			}
		})
	}
}

func TestPermissionCheckerReplacesCommandConfirmation(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	raw, _ := json.Marshal(map[string]any{"command": "echo permission-ok"})
	agent := &Agent{
		Registry: registry,
		PermissionChecker: &permissions.Checker{
			Root:    t.TempDir(),
			Session: permissions.NewSessionStore(),
			Project: []permissions.Rule{{
				Effect:         permissions.EffectAllow,
				Tool:           "run_command",
				CommandPattern: "echo *",
				Source:         permissions.SourceProject,
			}},
		},
	}

	result := agent.executeSingleTool(context.Background(), provider.ToolCall{ID: "call_1", Name: "run_command", Arguments: raw}, make(chan Event, EventBufferSize))
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
}

func call(id string, name string) *provider.ToolCall {
	raw, _ := json.Marshal(map[string]any{})
	return &provider.ToolCall{ID: id, Name: name, Arguments: raw}
}

func collect(events <-chan Event) []Event {
	var result []Event
	for event := range events {
		result = append(result, event)
	}
	return result
}

func drain(events <-chan Event) {
	for range events {
	}
}

func hasEvent(events []Event, kind EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func messagesText(messages []chat.Message) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Content)
		if message.ToolResult != nil {
			b.WriteString(string(message.ToolResult.Content))
		}
	}
	return b.String()
}

func toolNames(defs []tool.Definition) string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return strings.Join(names, ",")
}

func compactSummaryText() string {
	return "【分析草稿】draft\n【正式摘要】\n主要请求: req\n关键概念: concept\n文件代码: code\n错误修复: fix\n解决过程: process\n用户原话: quote\n待办: todo\n当前工作: current\n下一步: next"
}

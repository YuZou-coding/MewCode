package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadProjectAndUserRulesValidateAndMerge(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	writeHookFile(t, filepath.Join(home, ".mewcode", "hooks.yaml"), `rules:
- name: user-start
  event: session.start
  action:
    type: inject_prompt
    prompt: user
`)
	writeHookFile(t, filepath.Join(project, ".mewcode", "hooks.yaml"), `rules:
- name: project-start
  event: session.start
  action:
    type: inject_prompt
    prompt: project
`)

	loaded, err := Load(project, home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Rules) != 2 || loaded.Rules[0].Name != "user-start" || loaded.Rules[1].Name != "project-start" {
		t.Fatalf("rules = %#v", loaded.Rules)
	}
}

func TestLoadValidationIncludesRuleName(t *testing.T) {
	project := t.TempDir()
	writeHookFile(t, filepath.Join(project, ".mewcode", "hooks.yaml"), `rules:
- name: bad-block
  event: message.before_send
  block: nope
  action:
    type: inject_prompt
    prompt: x
`)
	_, err := Load(project, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "bad-block") || !strings.Contains(err.Error(), "block") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadValidationRejectsMissingAndInvalidFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "missing-event", body: "rules:\n- name: missing-event\n  action:\n    type: inject_prompt\n    prompt: x\n", want: "missing event"},
		{name: "missing-action", body: "rules:\n- name: missing-action\n  event: session.start\n", want: "missing action.type"},
		{name: "bad-event", body: "rules:\n- name: bad-event\n  event: nope\n  action:\n    type: inject_prompt\n    prompt: x\n", want: "nope"},
		{name: "bad-action", body: "rules:\n- name: bad-action\n  event: session.start\n  action:\n    type: nope\n", want: "nope"},
		{name: "async-before", body: "rules:\n- name: async-before\n  event: tool.before_execute\n  async: true\n  action:\n    type: inject_prompt\n    prompt: x\n", want: "async"},
		{name: "mixed", body: "rules:\n- name: mixed\n  event: session.start\n  all:\n  - field: event\n    op: eq\n    value: session.start\n  any:\n  - field: event\n    op: eq\n    value: session.start\n  action:\n    type: inject_prompt\n    prompt: x\n", want: "cannot mix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			writeHookFile(t, filepath.Join(project, ".mewcode", "hooks.yaml"), tc.body)
			_, err := Load(project, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tc.name) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConditionMatchingOperatorsAndTemplate(t *testing.T) {
	ctx := Context{
		Event:          EventToolBeforeExecute,
		ToolName:       "edit_file",
		ToolArgs:       map[string]any{"path": "internal/agent/agent.go"},
		Path:           "internal/agent/agent.go",
		MessageContent: "hello world",
		Error:          "boom",
	}
	rule := Rule{
		Name:  "match",
		Event: EventToolBeforeExecute,
		Conditions: Conditions{All: []Clause{
			{Field: "tool.name", Op: OpEq, Value: "edit_file"},
			{Field: "tool.args.path", Op: OpGlob, Value: "internal/**/*.go"},
			{Field: "message.content", Op: OpRegex, Value: "hello"},
			{Field: "error", Op: OpNot, Value: "nope"},
		}},
		Action: Action{Type: ActionInjectPrompt, Prompt: "{{event}} {{tool.name}} {{tool.args.path}} {{missing}}"},
	}
	if !rule.Matches(ctx) {
		t.Fatalf("rule did not match")
	}
	if got := Render(rule.Action.Prompt, ctx); got != "tool.before_execute edit_file internal/agent/agent.go " {
		t.Fatalf("render = %q", got)
	}
}

func TestEngineOnceInjectAndBlock(t *testing.T) {
	engine := NewEngine([]Rule{
		{Name: "once", Event: EventMessageBeforeSend, Once: true, Action: Action{Type: ActionInjectPrompt, Prompt: "remember {{message.content}}"}},
		{Name: "block-edit", Event: EventToolBeforeExecute, Block: "blocked {{path}}", Action: Action{Type: ActionInjectPrompt, Prompt: "ignored"}},
	}, nil)
	_ = engine.Fire(context.Background(), Context{Event: EventMessageBeforeSend, MessageContent: "A"})
	_ = engine.Fire(context.Background(), Context{Event: EventMessageBeforeSend, MessageContent: "B"})
	messages := engine.DrainPrompts()
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "remember A") {
		t.Fatalf("messages = %#v", messages)
	}
	result := engine.BeforeTool(context.Background(), Context{Event: EventToolBeforeExecute, ToolName: "edit_file", Path: "tmp.txt"})
	if !result.Blocked || !strings.Contains(result.Reason, "tmp.txt") {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPAndAsyncShellWarningsDoNotInterrupt(t *testing.T) {
	called := make(chan bool, 1)
	engine := NewEngine([]Rule{
		{Name: "http", Event: EventSessionStart, Action: Action{Type: ActionHTTP, URL: "https://example.test/hook", Method: "POST", Body: `{"event":"{{event}}"}`}},
		{Name: "shell", Event: EventSessionStart, Async: true, TimeoutMS: 1000, Action: Action{Type: ActionShell, Command: "echo hook-ok"}},
		{Name: "sub", Event: EventSessionStart, Action: Action{Type: ActionSubAgent, Prompt: "later"}},
	}, nil)
	engine.client = fakeHTTPDoer{called: called}
	if err := engine.Fire(context.Background(), Context{Event: EventSessionStart}); err != nil {
		t.Fatalf("Fire: %v", err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatalf("http action not called")
	}
	if len(engine.Warnings()) == 0 {
		t.Fatalf("expected sub_agent placeholder warning")
	}
}

func TestTimeoutMSLimitsAction(t *testing.T) {
	engine := NewEngine([]Rule{{
		Name:      "slow",
		Event:     EventSessionStart,
		TimeoutMS: 10,
		Action:    Action{Type: ActionShell, Command: "ignored"},
	}}, blockingRunner{})
	_ = engine.Fire(context.Background(), Context{Event: EventSessionStart})
	if engine.WarningCount() == 0 {
		t.Fatalf("expected timeout warning")
	}
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, command string, timeoutMS int) error {
	<-ctx.Done()
	return ctx.Err()
}

type fakeHTTPDoer struct {
	called chan bool
}

func (f fakeHTTPDoer) Do(ctx context.Context, method string, url string, headers map[string]string, body string, timeoutMS int) error {
	f.called <- method == "POST" && strings.Contains(url, "example.test") && strings.Contains(body, "session.start")
	return nil
}

func writeHookFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

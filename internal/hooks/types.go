package hooks

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"mewcode/internal/chat"
)

const (
	UserHooksPath    = ".mewcode/hooks.yaml"
	ProjectHooksPath = ".mewcode/hooks.yaml"
)

type EventName string

const (
	EventSystemStart       EventName = "system.start"
	EventSystemExit        EventName = "system.exit"
	EventSystemError       EventName = "system.error"
	EventCompactBefore     EventName = "compact.before"
	EventCompactAfter      EventName = "compact.after"
	EventSessionStart      EventName = "session.start"
	EventSessionEnd        EventName = "session.end"
	EventTurnStart         EventName = "turn.start"
	EventTurnEnd           EventName = "turn.end"
	EventMessageBeforeSend EventName = "message.before_send"
	EventMessageAfterRecv  EventName = "message.after_receive"
	EventToolBeforeExecute EventName = "tool.before_execute"
	EventToolAfterExecute  EventName = "tool.after_execute"
)

type Op string

const (
	OpEq    Op = "eq"
	OpNot   Op = "not"
	OpRegex Op = "regex"
	OpGlob  Op = "glob"
)

type ActionType string

const (
	ActionShell        ActionType = "shell"
	ActionInjectPrompt ActionType = "inject_prompt"
	ActionHTTP         ActionType = "http"
	ActionSubAgent     ActionType = "sub_agent"
)

type Rule struct {
	Name       string
	Event      EventName
	Conditions Conditions
	Action     Action
	Once       bool
	Async      bool
	TimeoutMS  int
	Block      string
	Source     string
}

type Conditions struct {
	All []Clause
	Any []Clause
}

type Clause struct {
	Field string
	Op    Op
	Value string
}

type Action struct {
	Type    ActionType
	Command string
	Prompt  string
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

type Context struct {
	Event          EventName
	ToolName       string
	ToolArgs       map[string]any
	Path           string
	Command        string
	MessageContent string
	Error          string
	SessionID      string
	ToolResult     string
}

type Loaded struct {
	Rules    []Rule
	Warnings []string
}

type BlockResult struct {
	Blocked bool
	Reason  string
}

type Engine struct {
	rules    []Rule
	warnings []string
	prompts  []chat.Message
	once     map[string]bool
	runner   CommandRunner
	client   HTTPDoer
	mu       sync.Mutex
}

type CommandRunner interface {
	Run(ctx context.Context, command string, timeoutMS int) error
}

type HTTPDoer interface {
	Do(ctx context.Context, method string, url string, headers map[string]string, body string, timeoutMS int) error
}

func NewEngine(rules []Rule, runner CommandRunner) *Engine {
	return &Engine{rules: rules, once: map[string]bool{}, runner: runner, client: DefaultHTTPClient{}}
}

func (e *Engine) RuleCount() int {
	if e == nil {
		return 0
	}
	return len(e.rules)
}

func (e *Engine) WarningCount() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.warnings)
}

func (e *Engine) Warnings() []string {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	warnings := make([]string, len(e.warnings))
	copy(warnings, e.warnings)
	return warnings
}

func (e *Engine) addWarning(format string, args ...any) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.warnings = append(e.warnings, fmt.Sprintf(format, args...))
}

func (e *Engine) DrainPrompts() []chat.Message {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	prompts := make([]chat.Message, len(e.prompts))
	copy(prompts, e.prompts)
	e.prompts = nil
	return prompts
}

func (e *Engine) ContextMessages() []chat.Message {
	return e.DrainPrompts()
}

func normalizeRuleName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "(unnamed)"
	}
	return name
}

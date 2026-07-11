package agent

import "mewcode/internal/tool"

type EventKind string

const (
	EventUserMessage       EventKind = "UserMessage"
	EventThinkingText      EventKind = "ThinkingText"
	EventStreamText        EventKind = "StreamText"
	EventToolCallStart     EventKind = "ToolCallStart"
	EventToolResult        EventKind = "ToolResult"
	EventFinalResponse     EventKind = "FinalResponse"
	EventUsage             EventKind = "UsageEvent"
	EventPermissionRequest EventKind = "PermissionRequestEvent"
	EventError             EventKind = "ErrorEvent"
	EventIteration         EventKind = "IterationEvent"
)

type Event struct {
	Kind              EventKind
	Text              string
	ToolCallID        string
	ToolName          string
	ToolArguments     []byte
	PermissionReason  string
	PermissionPath    string
	PermissionCommand string
	Result            *tool.Result
	Usage             Usage
	Error             error
	Iteration         int
	MaxIterations     int
}

type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

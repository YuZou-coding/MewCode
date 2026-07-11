package chat

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCall   *ToolCall
	ToolCalls  []ToolCall
	ToolResult *ToolResult
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	CallID  string
	Name    string
	Content json.RawMessage
}

type Session struct {
	messages []Message
	onAppend func(Message)
}

func NewSession() *Session {
	return &Session{}
}

func (s *Session) AddUser(content string) {
	s.append(Message{Role: RoleUser, Content: content})
}

func (s *Session) AddAssistant(content string) {
	s.append(Message{Role: RoleAssistant, Content: content})
}

func (s *Session) AddSystem(content string) {
	s.append(Message{Role: RoleSystem, Content: content})
}

func (s *Session) AddAssistantToolCall(call ToolCall) {
	s.append(Message{Role: RoleAssistant, ToolCall: &call})
}

func (s *Session) AddAssistantToolCalls(content string, calls []ToolCall) {
	copied := make([]ToolCall, len(calls))
	copy(copied, calls)
	message := Message{Role: RoleAssistant, Content: content, ToolCalls: copied}
	if len(copied) == 1 {
		message.ToolCall = &message.ToolCalls[0]
	}
	s.append(message)
}

func (s *Session) AddToolResult(result ToolResult) {
	s.append(Message{Role: RoleTool, ToolResult: &result})
}

func (s *Session) Messages() []Message {
	messages := make([]Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

func (s *Session) ReplaceMessages(messages []Message) {
	copied := make([]Message, len(messages))
	copy(copied, messages)
	s.messages = copied
}

func (s *Session) SetAppendHook(hook func(Message)) {
	s.onAppend = hook
}

func (s *Session) append(message Message) {
	s.messages = append(s.messages, message)
	if s.onAppend != nil {
		s.onAppend(message)
	}
}

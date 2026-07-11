package sessionstore

import (
	"path/filepath"
	"time"

	"mewcode/internal/chat"
)

const (
	SessionsDirName = ".mewcode/sessions"
	MessagesFile    = "messages.jsonl"
	MetaFile        = "meta.json"
	TimeGap         = 2 * time.Hour
)

type Record struct {
	Role       chat.Role        `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCall   *chat.ToolCall   `json:"tool_call,omitempty"`
	ToolCalls  []chat.ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *chat.ToolResult `json:"tool_result,omitempty"`
	CreatedAt  string           `json:"created_at"`
}

type Meta struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	MessageCount int    `json:"message_count"`
	UpdatedAt    string `json:"updated_at"`
}

type RestoreResult struct {
	Messages []chat.Message
	Meta     Meta
	Warnings []string
}

func Root(projectRoot string) string {
	return filepath.Join(projectRoot, SessionsDirName)
}

func SessionDir(projectRoot, id string) string {
	return filepath.Join(Root(projectRoot), id)
}

package sessionstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mewcode/internal/chat"
)

func TestStoreAppendsJSONLAndMeta(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	store, err := (Store{ProjectRoot: root, Now: func() time.Time { return now }}).Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(chat.Message{Role: chat.RoleUser, Content: "帮我看 README"}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := store.Append(chat.Message{Role: chat.RoleAssistant, Content: "好的"}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(SessionDir(root, store.ID), MessagesFile))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	if got := strings.Count(string(raw), "\n"); got != 2 {
		t.Fatalf("jsonl lines = %d\n%s", got, raw)
	}
	meta, err := store.Meta()
	if err != nil {
		t.Fatalf("Meta: %v", err)
	}
	if meta.ID != store.ID || meta.MessageCount != 2 || !strings.Contains(meta.Title, "帮我看 README") || meta.UpdatedAt == "" {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestRestoreSkipsBadLinesAndTruncatesIncompleteToolUse(t *testing.T) {
	root := t.TempDir()
	store, err := (Store{ProjectRoot: root}).Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	good := Record{Role: chat.RoleUser, Content: "hello", CreatedAt: time.Now().Format(time.RFC3339)}
	call := Record{
		Role:      chat.RoleAssistant,
		ToolCalls: []chat.ToolCall{{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	rawGood, _ := json.Marshal(good)
	rawCall, _ := json.Marshal(call)
	body := string(rawGood) + "\n{bad json\n" + string(rawCall) + "\n"
	if err := os.WriteFile(filepath.Join(SessionDir(root, store.ID), MessagesFile), []byte(body), 0600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	result := store.Restore(context.Background(), nil)
	if len(result.Messages) != 1 || result.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v warnings=%#v", result.Messages, result.Warnings)
	}
	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, "bad jsonl") || !strings.Contains(joined, "tool_use") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestTimeGapReminder(t *testing.T) {
	meta := Meta{UpdatedAt: time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)}
	message, ok := TimeGapReminder(meta, time.Date(2026, 7, 6, 13, 0, 0, 0, time.UTC))
	if !ok || !strings.Contains(message.Content, "会话恢复提醒") || !strings.Contains(message.Content, "上次活跃时间") {
		t.Fatalf("message=%#v ok=%v", message, ok)
	}
	_, ok = TimeGapReminder(meta, time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC))
	if ok {
		t.Fatalf("unexpected reminder below threshold")
	}
}

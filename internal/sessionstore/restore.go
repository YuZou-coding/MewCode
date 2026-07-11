package sessionstore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/compact"
	"mewcode/internal/prompt"
)

func (s *SessionStore) Restore(ctx context.Context, manager *compact.Manager) RestoreResult {
	meta, err := s.Meta()
	if err != nil {
		return RestoreResult{Warnings: []string{fmt.Sprintf("restore meta failed: %v", err)}}
	}
	file, err := os.Open(filepath.Join(SessionDir(s.ProjectRoot, s.ID), MessagesFile))
	if err != nil {
		return RestoreResult{Meta: meta, Warnings: []string{fmt.Sprintf("restore messages failed: %v", err)}}
	}
	defer file.Close()

	var messages []chat.Message
	var warnings []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped bad jsonl line %d", line))
			continue
		}
		messages = append(messages, chat.Message{
			Role:       record.Role,
			Content:    record.Content,
			ToolCall:   record.ToolCall,
			ToolCalls:  record.ToolCalls,
			ToolResult: record.ToolResult,
		})
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("restore scan failed: %v", err))
	}
	messages, truncated := truncateIncompleteToolUse(messages)
	if truncated {
		warnings = append(warnings, "truncated incomplete tool_use without matching tool_result")
	}
	if manager != nil && compact.HistorySize(messages) > compact.HistorySummaryThreshold {
		result := manager.ManualCompact(ctx, messages)
		messages = result.Messages
		for _, err := range result.Stats.Errors {
			warnings = append(warnings, err.Error())
		}
	}
	if reminder, ok := TimeGapReminder(meta, time.Now()); ok {
		messages = append([]chat.Message{reminder}, messages...)
	}
	return RestoreResult{Messages: messages, Meta: meta, Warnings: warnings}
}

func truncateIncompleteToolUse(messages []chat.Message) ([]chat.Message, bool) {
	pending := map[string]bool{}
	lastComplete := len(messages)
	for i, message := range messages {
		for _, call := range callsOf(message) {
			pending[call.ID] = true
		}
		if message.ToolResult != nil {
			delete(pending, message.ToolResult.CallID)
		}
		if len(pending) == 0 {
			lastComplete = i + 1
		}
	}
	if len(pending) == 0 {
		return messages, false
	}
	return messages[:lastComplete], true
}

func callsOf(message chat.Message) []chat.ToolCall {
	if len(message.ToolCalls) > 0 {
		return message.ToolCalls
	}
	if message.ToolCall != nil {
		return []chat.ToolCall{*message.ToolCall}
	}
	return nil
}

func TimeGapReminder(meta Meta, now time.Time) (chat.Message, bool) {
	updated, err := time.Parse(time.RFC3339, meta.UpdatedAt)
	if err != nil || now.Sub(updated) <= TimeGap {
		return chat.Message{}, false
	}
	return prompt.InternalInstruction(fmt.Sprintf("会话恢复提醒：距离上次活跃已过去 %s。请结合历史继续，不要假设中断期间文件没有变化。上次活跃时间：%s，恢复时间：%s。",
		now.Sub(updated).Round(time.Minute),
		updated.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)), true
}

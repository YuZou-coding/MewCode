package compact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"mewcode/internal/chat"
)

type ToolResultCompactor struct {
	Store ArtifactStore
}

type toolCandidate struct {
	Index int
	Size  int
}

func (c ToolResultCompactor) Compact(messages []chat.Message) Result {
	next := CloneMessages(messages)
	stats := Stats{
		BeforeMessages: len(messages),
		BeforeChars:    HistorySize(messages),
	}
	var candidates []toolCandidate
	for i, message := range next {
		if message.ToolResult == nil {
			continue
		}
		size := ToolResultSize(*message.ToolResult)
		if size > SingleToolResultThreshold {
			if c.externalize(&next[i], &stats) {
				continue
			}
		}
		candidates = append(candidates, toolCandidate{Index: i, Size: size})
	}
	total := ToolResultsTotal(next)
	if total > MessageToolResultsLimit {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Size > candidates[j].Size
		})
		for _, candidate := range candidates {
			if total <= MessageToolResultsLimit {
				break
			}
			if next[candidate.Index].ToolResult == nil {
				continue
			}
			if c.externalize(&next[candidate.Index], &stats) {
				total -= candidate.Size
			}
		}
	}
	stats.AfterMessages = len(next)
	stats.AfterChars = HistorySize(next)
	return Result{Messages: next, Stats: stats}
}

func (c ToolResultCompactor) externalize(message *chat.Message, stats *Stats) bool {
	if message.ToolResult == nil {
		return false
	}
	original := *message.ToolResult
	path, err := c.Store.WriteToolResult(original)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Errorf("compact artifact write failed: %w", err))
		return false
	}
	preview := previewSource(original.Content)
	runes := []rune(preview)
	if len(runes) > ToolResultPreviewLength {
		preview = string(runes[:ToolResultPreviewLength])
	}
	replacement := map[string]any{
		"ok": true,
		"data": map[string]any{
			"externalized":  true,
			"artifact_path": path,
			"original_size": ToolResultSize(original),
			"tool_name":     original.Name,
			"call_id":       original.CallID,
			"preview":       preview,
			"notice":        "工具结果已截断，完整内容已写入 artifact 文件。如需细节请重新读取该路径。",
		},
	}
	raw, err := json.Marshal(replacement)
	if err != nil {
		stats.Errors = append(stats.Errors, err)
		return false
	}
	message.ToolResult.Content = raw
	stats.Artifacts++
	return true
}

func previewSource(raw json.RawMessage) string {
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		if content, ok := payload.Data["content"].(string); ok && content != "" {
			return content
		}
		if text, ok := payload.Data["text"].(string); ok && text != "" {
			return text
		}
	}
	return string(raw)
}

func ContainsFullContent(message chat.Message, needle string) bool {
	if message.ToolResult == nil {
		return false
	}
	return strings.Contains(string(message.ToolResult.Content), needle)
}

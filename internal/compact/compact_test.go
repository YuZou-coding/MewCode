package compact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

func TestConstants(t *testing.T) {
	if SingleToolResultThreshold != 12000 || MessageToolResultsLimit != 24000 || HistorySummaryThreshold != 80000 {
		t.Fatalf("thresholds changed")
	}
	if ToolResultPreviewLength != 2000 || RecentRoundsToKeep != 6 || SummaryFailureLimit != 3 {
		t.Fatalf("compact constants changed")
	}
	if ArtifactDir != ".mewcode/artifacts/tool-results" || ManualCommand != "/compact" {
		t.Fatalf("paths or command changed")
	}
}

func TestMeasureMessages(t *testing.T) {
	user := chat.Message{Role: chat.RoleUser, Content: "你好abc"}
	if MessageSize(user) != 5 {
		t.Fatalf("user size = %d", MessageSize(user))
	}
	assistant := chat.Message{Role: chat.RoleAssistant, Content: "hello"}
	if MessageSize(assistant) != 5 {
		t.Fatalf("assistant size = %d", MessageSize(assistant))
	}
	toolMessage := toolResultMessage("call_1", "read_file", strings.Repeat("x", 7))
	if MessageSize(toolMessage) != 7 {
		t.Fatalf("tool size = %d", MessageSize(toolMessage))
	}
	if ToolResultsTotal([]chat.Message{toolMessage, toolResultMessage("call_2", "search_code", strings.Repeat("y", 3))}) != 10 {
		t.Fatalf("tool total mismatch")
	}
	if HistorySize([]chat.Message{user, assistant, toolMessage}) != 17 {
		t.Fatalf("history size = %d", HistorySize([]chat.Message{user, assistant, toolMessage}))
	}
}

func TestArtifactWriteAndToolResultCompaction(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("a", SingleToolResultThreshold+1)
	message := toolResultMessage("call_1", "read_file", long)
	result := ToolResultCompactor{Store: ArtifactStore{Root: root}}.Compact([]chat.Message{{Role: chat.RoleUser, Content: "保持原文"}, message})
	if result.Messages[0].Content != "保持原文" {
		t.Fatalf("user message changed")
	}
	if result.Stats.Artifacts != 1 || len(result.Stats.Errors) != 0 {
		t.Fatalf("stats = %#v", result.Stats)
	}
	replaced := string(result.Messages[1].ToolResult.Content)
	for _, want := range []string{ArtifactDir, "12001", "read_file", "call_1", strings.Repeat("a", ToolResultPreviewLength), "已截断"} {
		if !strings.Contains(replaced, want) {
			t.Fatalf("replacement missing %q: %.200s", want, replaced)
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ArtifactDir, "*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("matches=%#v err=%v", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var record ArtifactRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if record.ToolName != "read_file" || record.CallID != "call_1" || record.OriginalSize != len(long) || record.Content != long || record.CreatedAt == "" {
		t.Fatalf("record = %#v", record)
	}
	if filepath.Ext(matches[0]) != ".json" || !strings.Contains(filepath.Base(matches[0]), "read_file") {
		t.Fatalf("artifact filename = %s", matches[0])
	}
}

func TestToolResultThresholdsAndLargestFirst(t *testing.T) {
	root := t.TempDir()
	atThreshold := toolResultMessage("call_1", "read_file", strings.Repeat("a", SingleToolResultThreshold))
	result := ToolResultCompactor{Store: ArtifactStore{Root: root}}.Compact([]chat.Message{atThreshold})
	if result.Stats.Artifacts != 0 {
		t.Fatalf("threshold result externalized")
	}

	messages := []chat.Message{
		toolResultMessage("small", "tool", strings.Repeat("s", 5000)),
		toolResultMessage("large", "tool", strings.Repeat("l", 11000)),
		toolResultMessage("medium", "tool", strings.Repeat("m", 9001)),
	}
	result = ToolResultCompactor{Store: ArtifactStore{Root: root}}.Compact(messages)
	if result.Stats.Artifacts != 1 {
		t.Fatalf("artifacts = %d", result.Stats.Artifacts)
	}
	if !strings.Contains(string(result.Messages[1].ToolResult.Content), "externalized") {
		t.Fatalf("largest was not externalized: %s", result.Messages[1].ToolResult.Content)
	}
	if strings.Contains(string(result.Messages[0].ToolResult.Content), "externalized") || strings.Contains(string(result.Messages[2].ToolResult.Content), "externalized") {
		t.Fatalf("wrong result externalized")
	}
}

func TestArtifactWriteFailureKeepsOriginal(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(rootFile, []byte("x"), 0600); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	original := strings.Repeat("x", SingleToolResultThreshold+1)
	message := toolResultMessage("call_1", "read_file", original)
	result := ToolResultCompactor{Store: ArtifactStore{Root: rootFile}}.Compact([]chat.Message{message})
	if result.Stats.Artifacts != 0 || len(result.Stats.Errors) == 0 {
		t.Fatalf("stats = %#v", result.Stats)
	}
	if string(result.Messages[0].ToolResult.Content) != original {
		t.Fatalf("original not preserved")
	}
}

func TestSummaryPromptAndExtraction(t *testing.T) {
	prompt := SummaryPrompt([]chat.Message{{Role: chat.RoleUser, Content: "用户原文"}})
	if !strings.HasPrefix(prompt, noToolsInstruction) || !strings.HasSuffix(prompt, "不要调用任何工具。") {
		t.Fatalf("prompt missing no-tools boundary: %s", prompt)
	}
	if !strings.Contains(prompt, "分析草稿") {
		t.Fatalf("prompt missing draft instruction")
	}
	for _, section := range SummarySections {
		if !strings.Contains(prompt, section) {
			t.Fatalf("prompt missing section %s", section)
		}
	}
	text := "【分析草稿】丢弃\n【正式摘要】\n主要请求: a\n关键概念: b\n文件代码: c\n错误修复: d\n解决过程: e\n用户原话: f\n待办: g\n当前工作: h\n下一步: i"
	official := ExtractOfficialSummary(text)
	if strings.Contains(official, "分析草稿") || !HasAllSummarySections(official) {
		t.Fatalf("official = %s", official)
	}
}

func TestSummarizerKeepsRecentRoundsAndAddsBoundary(t *testing.T) {
	messages := longHistory(7, strings.Repeat("x", 12000))
	fp := &fakeSummaryProvider{text: summaryText()}
	breaker := &Breaker{}
	result := Summarizer{Provider: fp, Breaker: breaker}.Compact(context.Background(), messages, false)
	if !result.Stats.Summarized || len(result.Stats.Errors) != 0 {
		t.Fatalf("stats = %#v", result.Stats)
	}
	if breaker.Failures() != 0 {
		t.Fatalf("breaker failures = %d", breaker.Failures())
	}
	if len(fp.toolsSeen) != 1 || len(fp.toolsSeen[0]) != 0 {
		t.Fatalf("summary saw tools: %#v", fp.toolsSeen)
	}
	if len(result.Messages) < 2 || !strings.Contains(result.Messages[0].Content, "上下文压缩摘要") || !strings.Contains(result.Messages[1].Content, "如需文件细节请重新读取") || !strings.Contains(result.Messages[1].Content, "不要根据摘要脑补代码") {
		t.Fatalf("messages = %#v", result.Messages[:2])
	}
	for i := 2; i < len(result.Messages); i++ {
		if result.Messages[i].Role == chat.RoleUser && !strings.HasPrefix(result.Messages[i].Content, "user-") {
			t.Fatalf("recent user changed: %#v", result.Messages[i])
		}
	}
	if !strings.Contains(fp.messagesSeen[0][0].Content, "user-0") {
		t.Fatalf("old user original not sent to summary prompt")
	}
}

func TestSummarizerThresholdAndBreaker(t *testing.T) {
	fp := &fakeSummaryProvider{text: summaryText(), err: errors.New("summary failed")}
	breaker := &Breaker{}
	messages := longHistory(7, strings.Repeat("x", 12000))
	for i := 0; i < SummaryFailureLimit; i++ {
		result := Summarizer{Provider: fp, Breaker: breaker}.Compact(context.Background(), messages, false)
		if len(result.Stats.Errors) == 0 {
			t.Fatalf("expected error on failure %d", i)
		}
		if HistorySize(result.Messages) != HistorySize(messages) {
			t.Fatalf("history changed on failure")
		}
	}
	if !breaker.AutomaticDisabled() {
		t.Fatalf("breaker not disabled")
	}
	callsBefore := len(fp.messagesSeen)
	result := Summarizer{Provider: fp, Breaker: breaker}.Compact(context.Background(), messages, false)
	if len(fp.messagesSeen) != callsBefore || result.Stats.Summarized {
		t.Fatalf("automatic summary not blocked")
	}
	fp.err = nil
	result = Summarizer{Provider: fp, Breaker: breaker}.Compact(context.Background(), messages, true)
	if !result.Stats.Summarized || breaker.Failures() != 0 {
		t.Fatalf("manual summary did not reset breaker")
	}

	short := []chat.Message{{Role: chat.RoleUser, Content: strings.Repeat("x", HistorySummaryThreshold)}}
	fp2 := &fakeSummaryProvider{text: summaryText()}
	result = Summarizer{Provider: fp2, Breaker: &Breaker{}}.Compact(context.Background(), short, false)
	if len(fp2.messagesSeen) != 0 || result.Stats.Summarized {
		t.Fatalf("threshold equal should not summarize")
	}
}

func TestManagerRunsToolThenSummary(t *testing.T) {
	root := t.TempDir()
	fp := &fakeSummaryProvider{text: summaryText()}
	messages := longHistory(7, strings.Repeat("x", 12000))
	messages = append(messages[:2], append([]chat.Message{toolResultMessage("call_big", "read_file", strings.Repeat("b", SingleToolResultThreshold+1))}, messages[2:]...)...)
	manager := &Manager{Root: root, Provider: fp}
	result := manager.CompactBeforeRequest(context.Background(), messages)
	if !result.Stats.Summarized || result.Stats.Artifacts != 1 {
		t.Fatalf("stats = %#v", result.Stats)
	}
	if !strings.Contains(fp.messagesSeen[0][0].Content, ArtifactDir) {
		t.Fatalf("summary did not run after tool externalization: %s", fp.messagesSeen[0][0].Content)
	}
}

type fakeSummaryProvider struct {
	text         string
	err          error
	messagesSeen [][]chat.Message
	toolsSeen    [][]tool.Definition
}

func (f *fakeSummaryProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	copiedMessages := CloneMessages(messages)
	copiedTools := make([]tool.Definition, len(tools))
	copy(copiedTools, tools)
	f.messagesSeen = append(f.messagesSeen, copiedMessages)
	f.toolsSeen = append(f.toolsSeen, copiedTools)
	events := make(chan provider.StreamEvent, 1)
	errs := make(chan error, 1)
	if f.text != "" {
		events <- provider.StreamEvent{Kind: provider.EventText, Text: f.text}
	}
	close(events)
	errs <- f.err
	return events, errs
}

func toolResultMessage(id string, name string, content string) chat.Message {
	return chat.Message{Role: chat.RoleTool, ToolResult: &chat.ToolResult{CallID: id, Name: name, Content: json.RawMessage(content)}}
}

func longHistory(rounds int, payload string) []chat.Message {
	var messages []chat.Message
	for i := 0; i < rounds; i++ {
		messages = append(messages,
			chat.Message{Role: chat.RoleUser, Content: "user-" + string(rune('0'+i))},
			chat.Message{Role: chat.RoleAssistant, Content: payload},
		)
	}
	return messages
}

func summaryText() string {
	return "【分析草稿】draft\n【正式摘要】\n主要请求: req\n关键概念: concept\n文件代码: code\n错误修复: fix\n解决过程: process\n用户原话: quote\n待办: todo\n当前工作: current\n下一步: next"
}

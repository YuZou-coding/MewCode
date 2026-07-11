package compact

import (
	"context"
	"fmt"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

const BoundaryMessage = "压缩边界：以上摘要替代了较早的对话历史。如需文件细节请重新读取相关文件或 artifact 路径；不要根据摘要脑补代码或不存在的实现。"

type Summarizer struct {
	Provider provider.Provider
	Breaker  *Breaker
}

func (s Summarizer) Compact(ctx context.Context, messages []chat.Message, force bool) Result {
	next := CloneMessages(messages)
	stats := Stats{BeforeMessages: len(messages), BeforeChars: HistorySize(messages)}
	if !force && HistorySize(next) <= HistorySummaryThreshold {
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	if !force && s.Breaker != nil && s.Breaker.AutomaticDisabled() {
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	if s.Provider == nil {
		err := fmt.Errorf("compact summarizer provider is not configured")
		stats.Errors = append(stats.Errors, err)
		if s.Breaker != nil {
			s.Breaker.RecordFailure()
		}
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	old, recent := splitHistoryForSummary(next)
	if len(old) == 0 {
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	prompt := SummaryPrompt(old)
	summary, err := s.callSummary(ctx, prompt)
	if err != nil {
		stats.Errors = append(stats.Errors, err)
		if s.Breaker != nil {
			s.Breaker.RecordFailure()
		}
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	official := ExtractOfficialSummary(summary)
	if official == "" {
		err := fmt.Errorf("compact summary is empty")
		stats.Errors = append(stats.Errors, err)
		if s.Breaker != nil {
			s.Breaker.RecordFailure()
		}
		stats.AfterMessages = len(next)
		stats.AfterChars = HistorySize(next)
		return Result{Messages: next, Stats: stats}
	}
	if s.Breaker != nil {
		s.Breaker.RecordSuccess()
	}
	summaryMessage := chat.Message{Role: chat.RoleSystem, Content: "上下文压缩摘要：\n" + official}
	boundaryMessage := chat.Message{Role: chat.RoleSystem, Content: BoundaryMessage}
	compacted := make([]chat.Message, 0, len(recent)+2)
	compacted = append(compacted, summaryMessage, boundaryMessage)
	compacted = append(compacted, recent...)
	stats.Summarized = true
	stats.AfterMessages = len(compacted)
	stats.AfterChars = HistorySize(compacted)
	return Result{Messages: compacted, Stats: stats}
}

func (s Summarizer) callSummary(ctx context.Context, prompt string) (string, error) {
	messages := []chat.Message{{Role: chat.RoleUser, Content: prompt}}
	stream, errs := s.Provider.StreamChat(ctx, messages, []tool.Definition{})
	var text strings.Builder
	for event := range stream {
		if event.Kind == provider.EventText {
			text.WriteString(event.Text)
		}
	}
	if err := <-errs; err != nil {
		return "", err
	}
	return text.String(), nil
}

func splitHistoryForSummary(messages []chat.Message) ([]chat.Message, []chat.Message) {
	userCount := 0
	startRecent := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == chat.RoleUser {
			userCount++
			if userCount == RecentRoundsToKeep {
				startRecent = i
				break
			}
		}
	}
	if userCount < RecentRoundsToKeep {
		return nil, messages
	}
	old := CloneMessages(messages[:startRecent])
	recent := CloneMessages(messages[startRecent:])
	return old, recent
}

type Manager struct {
	Root     string
	Provider provider.Provider
	Breaker  Breaker
}

func (m *Manager) CompactBeforeRequest(ctx context.Context, messages []chat.Message) Result {
	return m.compact(ctx, messages, false)
}

func (m *Manager) ManualCompact(ctx context.Context, messages []chat.Message) Result {
	return m.compact(ctx, messages, true)
}

func (m *Manager) compact(ctx context.Context, messages []chat.Message, force bool) Result {
	toolResult := ToolResultCompactor{Store: ArtifactStore{Root: m.Root}}.Compact(messages)
	summarizer := Summarizer{Provider: m.Provider, Breaker: &m.Breaker}
	summary := summarizer.Compact(ctx, toolResult.Messages, force)
	stats := summary.Stats
	stats.BeforeMessages = len(messages)
	stats.BeforeChars = HistorySize(messages)
	stats.Artifacts += toolResult.Stats.Artifacts
	stats.Errors = append(toolResult.Stats.Errors, summary.Stats.Errors...)
	return Result{Messages: summary.Messages, Stats: stats}
}

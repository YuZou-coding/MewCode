package tuiapp

import (
	"fmt"
	"strings"
	"time"

	"mewcode/internal/command"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
)

func (m *Model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if m.streamingBlock < 0 || m.streamingBlock >= len(m.blocks) || m.blocks[m.streamingBlock].Kind != BlockAssistant {
		m.blocks = append(m.blocks, TranscriptBlock{Kind: BlockAssistant, Text: delta})
		m.streamingBlock = len(m.blocks) - 1
		m.refresh()
		return
	}
	m.blocks[m.streamingBlock].Text += delta
	m.refresh()
}

func (m *Model) appendSystem(text string) {
	m.appendBlock(TranscriptBlock{Kind: BlockSystem, Text: text})
}
func (m *Model) appendUser(text string) { m.appendBlock(TranscriptBlock{Kind: BlockUser, Text: text}) }
func (m *Model) appendThinking() {
	m.appendBlock(TranscriptBlock{Kind: BlockThinking, Text: "thinking..."})
}
func (m *Model) appendUsage(usage provider.Usage) {
	m.appendBlock(TranscriptBlock{Kind: BlockUsage, Usage: usage})
}
func (m *Model) appendError(text string) {
	m.appendBlock(TranscriptBlock{Kind: BlockError, Text: text})
}

func (m *Model) appendBlock(block TranscriptBlock) {
	m.blocks = append(m.blocks, block)
	m.refresh()
}

func (m *Model) appendPermissionDecision(toolName string, choice permissions.HITLChoice) {
	text := "Denied " + toolName
	switch choice {
	case permissions.HITLAllowOnce:
		text = "Allowed " + toolName + " once"
	case permissions.HITLAllowSession:
		text = "Allowed " + toolName + " for this session"
	case permissions.HITLAllowAlways:
		text = "Allowed " + toolName + " always"
	}
	m.appendBlock(TranscriptBlock{Kind: BlockPermission, Text: text})
}

func (m *Model) appendToolStart(callID, name, target string) {
	m.appendBlock(TranscriptBlock{Kind: BlockTool, CallID: callID, ToolName: name, Target: target, ToolState: ToolRunning, StartedAt: time.Now()})
}

func (m *Model) completeTool(callID, name string, state ToolBlockState, detail string) {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		block := &m.blocks[i]
		if block.Kind != BlockTool || (callID != "" && block.CallID != "" && block.CallID != callID) || (name != "" && block.ToolName != "" && block.ToolName != name) {
			continue
		}
		block.ToolState = state
		block.Detail = detail
		if !block.StartedAt.IsZero() {
			block.Duration = time.Since(block.StartedAt)
		}
		m.refresh()
		return
	}
	m.appendBlock(TranscriptBlock{Kind: BlockTool, CallID: callID, ToolName: name, ToolState: state, Detail: detail})
}

func (m *Model) refresh() {
	m.viewport.SetContent(m.renderTranscript(true))
	if m.followOutput {
		m.viewport.GotoBottom()
		m.newOutput = false
	} else {
		m.newOutput = true
	}
}

func (m Model) renderTranscript(styled bool) string {
	lines := make([]string, 0, len(m.blocks))
	for _, block := range m.blocks {
		lines = append(lines, m.renderBlock(block, styled))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderBlock(block TranscriptBlock, styled bool) string {
	style := func(value string, render func(...string) string) string {
		if styled {
			return render(value)
		}
		return value
	}
	switch block.Kind {
	case BlockWelcome:
		return block.Text
	case BlockUser:
		return style("❯ "+block.Text, defaultTheme.brand.Render)
	case BlockAssistant:
		return style("● "+block.Text, defaultTheme.brand.Render)
	case BlockThinking:
		return style("✻ Thinking", defaultTheme.muted.Render)
	case BlockTool:
		return m.renderToolBlock(block, styled)
	case BlockUsage:
		line := fmt.Sprintf("  tokens in=%d out=%d · cache read=%d write=%d", block.Usage.InputTokens, block.Usage.OutputTokens, block.Usage.CacheReadTokens, block.Usage.CacheWriteTokens)
		return style(line, defaultTheme.muted.Render)
	case BlockError:
		return style("× Error: "+block.Text, defaultTheme.danger.Render)
	case BlockPermission:
		return style("  "+block.Text, defaultTheme.warning.Render)
	default:
		return style("  "+block.Text, defaultTheme.muted.Render)
	}
}

func (m Model) renderToolBlock(block TranscriptBlock, styled bool) string {
	name, state := block.ToolName, block.ToolState
	if name == "" {
		name = "tool"
	}
	if state == "" {
		state = ToolRunning
	}
	marker := "●"
	if state == ToolDone {
		marker = "✓"
	} else if state == ToolFailed || state == ToolBlocked {
		marker = "×"
	}
	line := fmt.Sprintf("  %s %s %s", marker, name, state)
	if block.Target != "" {
		line = fmt.Sprintf("  %s %s %s · %s", marker, name, block.Target, state)
	}
	if block.Duration > 0 {
		line += " in " + formatDuration(block.Duration)
	}
	if (state == ToolFailed || state == ToolBlocked) && block.Detail != "" {
		line += "\n    " + truncateLines(block.Detail, 3)
	}
	if !styled {
		return line
	}
	if state == ToolDone {
		return defaultTheme.success.Render(line)
	}
	if state == ToolFailed || state == ToolBlocked {
		return defaultTheme.danger.Render(line)
	}
	return defaultTheme.muted.Render(line)
}

func welcomeText(state command.State) string {
	session := state.SessionID
	if session == "" {
		session = "new"
	}
	return fmt.Sprintf("      /\\_/\\\n /\\  / o o \\  /\\\n//\\\\ \\~(*)~/ //\\\\\n`  /   ^   \\  `\n\nMewCode\nTerminal AI coding agent\n\nmode=%s  session=%s  messages=%d\n\nTry /help for commands, /plan for planning, /compact for context,\n/sessions to resume, /status for details, or /exit when you are done.", state.Mode, session, state.MessageCount)
}

func truncateLines(value string, limit int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if limit > 0 && len(lines) > limit {
		lines = append(lines[:limit], "…")
	}
	return strings.Join(lines, "\n    ")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

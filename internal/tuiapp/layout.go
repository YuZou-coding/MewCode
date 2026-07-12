package tuiapp

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	state := m.controller.Status()
	headerText := "MewCode"
	if state.WorkingDirectory != "" {
		headerText += " · " + filepath.Base(state.WorkingDirectory)
	}
	if state.GitBranch != "" && m.width >= 60 {
		headerText += " · " + state.GitBranch
	}
	header := defaultTheme.brand.Render(truncateDisplay(headerText, max(1, m.width)))
	panel := ""
	if m.pendingPermission != nil {
		panel = "\n" + m.permissionLine()
	} else if commands := m.commandPanel(); commands != "" {
		panel = "\n" + commands
	} else if m.newOutput {
		panel = "\n" + defaultTheme.warning.Render("↓ New output · End to follow")
	}
	panelHeight := strings.Count(panel, "\n")
	m.viewport.Height = max(1, m.height-4-panelHeight)
	return lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), panel, m.inputLine(), m.statusLine())
}

func (m Model) statusLine() string {
	state, width := m.controller.Status(), max(1, m.width)
	parts := []string{state.Mode}
	if width < 52 {
		parts = append(parts, fmt.Sprintf("m:%d", state.MessageCount))
	} else {
		parts = append(parts, fmt.Sprintf("msgs %d", state.MessageCount))
	}
	if state.GitBranch != "" {
		parts = append(parts, state.GitBranch)
	}
	if state.ContextPercent > 0 {
		parts = append(parts, fmt.Sprintf("ctx %d%%", state.ContextPercent))
	}
	if m.busy && !m.phaseStartedAt.IsZero() {
		parts = append(parts, m.phase+" "+formatDuration(time.Since(m.phaseStartedAt)))
	}
	if m.busy && m.maxIterations > 0 {
		parts = append(parts, fmt.Sprintf("iteration %d/%d", m.iteration, m.maxIterations))
	}
	if width >= 60 {
		parts = append(parts, fmt.Sprintf("mcp %d/%d", state.MCPConnected, state.MCPConfigured))
	}
	if width >= 60 && state.SessionID != "" {
		parts = append(parts, "session "+shortMiddle(state.SessionID, 14))
	}
	if width >= 80 {
		parts = append(parts, fmt.Sprintf("tokens %d/%d", state.LastUsage.InputTokens, state.LastUsage.OutputTokens), "/ for commands")
	}
	line := strings.Join(parts, " · ")
	return defaultTheme.muted.Render(truncateDisplay(line, width))
}

func (m Model) inputLine() string {
	if m.busy && m.pendingPermission == nil {
		return defaultTheme.muted.Render("✻ working · " + m.activityText())
	}
	runes, index := []rune(m.input), m.cursor
	if index < 0 {
		index = 0
	}
	if index > len(runes) {
		index = len(runes)
	}
	line := "❯ " + string(runes[:index]) + defaultTheme.cursor.Render("▌") + string(runes[index:])
	return truncateDisplay(line, max(1, m.width))
}

func (m Model) activityText() string {
	phase := m.phase
	if phase == "" {
		phase = "working"
	}
	elapsed := time.Duration(0)
	if !m.phaseStartedAt.IsZero() {
		elapsed = time.Since(m.phaseStartedAt)
	}
	return fmt.Sprintf("%s · %s", phase, formatDuration(elapsed))
}

func shortMiddle(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	head := (limit - 1) / 2
	tail := limit - 1 - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

func truncateDisplay(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	var b strings.Builder
	for _, r := range value {
		next := b.String() + string(r)
		if lipgloss.Width(next) > width {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

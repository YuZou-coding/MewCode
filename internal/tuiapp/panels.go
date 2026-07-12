package tuiapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"mewcode/internal/permissions"
)

func (m *Model) syncCommandPanel() {
	if m.registry == nil || !strings.HasPrefix(strings.TrimSpace(m.input), "/") {
		m.commandItems = nil
		m.commandSelection = 0
		return
	}
	m.commandItems = m.registry.PanelItems(m.input, 8)
	if m.commandSelection >= len(m.commandItems) {
		m.commandSelection = max(0, len(m.commandItems)-1)
	}
}

func (m Model) commandPanelVisible() bool {
	return !m.busy && m.pendingPermission == nil && !m.commandPanelDismissed && len(m.commandItems) > 0
}
func (m *Model) moveCommandSelection(delta int) {
	if len(m.commandItems) > 0 {
		m.commandSelection = (m.commandSelection + delta + len(m.commandItems)) % len(m.commandItems)
	}
}
func (m *Model) applySelectedCommand() {
	if m.commandSelection < 0 || m.commandSelection >= len(m.commandItems) {
		return
	}
	m.input = "/" + m.commandItems[m.commandSelection].Name
	if m.commandItems[m.commandSelection].ArgHint != "" {
		m.input += " "
	}
	m.cursor = len([]rune(m.input))
}

func (m Model) permissionLine() string {
	pending := m.pendingPermission
	if pending == nil {
		return ""
	}
	lines := []string{"Permission required", "tool: " + pending.request.Tool}
	var args map[string]any
	_ = json.Unmarshal(pending.request.Arguments, &args)
	for _, key := range []string{"path", "file_path"} {
		if value, ok := args[key].(string); ok && value != "" {
			lines = append(lines, "path: "+shortMiddle(value, max(8, m.width-10)))
			break
		}
	}
	if value, ok := args["command"].(string); ok && value != "" {
		lines = append(lines, "command: "+shortMiddle(strings.ReplaceAll(value, "\n", " "), max(8, m.width-13)))
	}
	if pending.decision.Reason != "" {
		lines = append(lines, "reason: "+truncateDisplay(pending.decision.Reason, max(8, m.width-9)))
	}
	if pending.decision.Mode == permissions.ModeStrict {
		lines = append(lines, "n deny   y once")
	} else if m.width < 52 {
		lines = append(lines, "n deny · y once", "s session · a always")
	} else {
		lines = append(lines, "n deny   y once   s session   a always")
	}
	contentWidth := max(1, m.width-2)
	for index := range lines {
		lines[index] = truncateDisplay(lines[index], contentWidth)
	}
	return renderPermissionPanel(lines)
}

func renderPermissionPanel(lines []string) string {
	rendered := make([]string, 0, len(lines))
	for index, line := range lines {
		content := line
		if index == 0 {
			content = defaultTheme.permissionTitle.Render(line)
		} else if strings.Contains(line, "deny") || strings.Contains(line, "once") || strings.Contains(line, "session") || strings.Contains(line, "always") {
			content = renderPermissionActions(line)
		}
		rendered = append(rendered, defaultTheme.permissionBorder.Render("│")+" "+content)
	}
	return strings.Join(rendered, "\n")
}

func renderPermissionActions(line string) string {
	line = strings.ReplaceAll(line, "n deny", defaultTheme.danger.Render("n deny"))
	for _, action := range []string{"y once", "s session", "a always"} {
		line = strings.ReplaceAll(line, action, defaultTheme.success.Render(action))
	}
	return line
}

func (m Model) commandPanel() string {
	if !m.commandPanelVisible() {
		return ""
	}
	lines := []string{"Commands  ↑↓ select · tab complete · enter run · esc close"}
	for index, item := range m.commandItems {
		marker := "  "
		if index == m.commandSelection {
			marker = "› "
		}
		line := marker + fmt.Sprintf("/%-11s %s", item.Name, item.Description)
		if m.width >= 88 && item.Usage != "" {
			line += "  " + item.Usage
		}
		line = truncateDisplay(line, max(1, m.width-2))
		lines = append(lines, line)
	}
	return renderCommandPanel(lines, m.commandSelection+1)
}

func renderCommandPanel(lines []string, selectedLine int) string {
	rendered := make([]string, 0, len(lines))
	for index, line := range lines {
		content := line
		if index == 0 {
			content = defaultTheme.commandTitle.Render(line)
		} else if index == selectedLine {
			content = defaultTheme.commandSelected.Render(line)
		} else {
			content = defaultTheme.muted.Render(line)
		}
		rendered = append(rendered, defaultTheme.commandBorder.Render("│")+" "+content)
	}
	return strings.Join(rendered, "\n")
}

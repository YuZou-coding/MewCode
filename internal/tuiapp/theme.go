package tuiapp

import (
	"github.com/charmbracelet/lipgloss"
)

type tuiTheme struct {
	brand            lipgloss.Style
	muted            lipgloss.Style
	success          lipgloss.Style
	warning          lipgloss.Style
	danger           lipgloss.Style
	cursor           lipgloss.Style
	permissionTitle  lipgloss.Style
	permissionBorder lipgloss.Style
	commandTitle     lipgloss.Style
	commandBorder    lipgloss.Style
	commandSelected  lipgloss.Style
}

var defaultTheme = tuiTheme{
	brand:            lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("173")),
	muted:            lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	success:          lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
	warning:          lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	danger:           lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
	cursor:           lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
	permissionTitle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
	permissionBorder: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	commandTitle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("173")),
	commandBorder:    lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
	commandSelected:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("173")),
}

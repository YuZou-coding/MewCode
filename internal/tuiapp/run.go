package tuiapp

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"mewcode/internal/command"
)

func Run(ctx context.Context, input io.Reader, output io.Writer, registry *command.Registry, controller command.Controller, submit SubmitFunc) error {
	model := New(ctx, registry, controller, submit)
	program := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}

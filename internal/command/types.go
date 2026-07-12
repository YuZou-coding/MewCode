package command

import (
	"context"
	"fmt"
	"strings"

	"mewcode/internal/provider"
)

type Type string

const (
	TypeLocal    Type = "local"
	TypeUIState  Type = "ui_state"
	TypeAIPrompt Type = "ai_prompt"
)

type Handler func(context.Context, Controller, Invocation) Result

type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Type        Type
	ArgHint     string
	Hidden      bool
	Subcommands []string
	Handler     Handler
}

type Invocation struct {
	Raw     string
	Name    string
	Args    string
	Command Command
}

type ParseResult struct {
	IsCommand bool
	Name      string
	Args      string
	Raw       string
}

type Result struct {
	Exit        bool
	SendToAgent string
	Messages    []string
	Err         error
}

type State struct {
	Mode              string
	SessionID         string
	MessageCount      int
	LastUsage         provider.Usage
	WorkingDirectory  string
	GitBranch         string
	ContextPercent    int
	CompletionOptions []string
	HookRules         int
	HookWarnings      int
	WorkerRunning     int
	WorkerCompleted   int
	MCPConnected      int
	MCPConfigured     int
	WorktreeName      string
	WorktreeMainRoot  string
	WorktreePath      string
	WorktreeCleaned   int
	TeamActive        string
	TeamLead          string
	TeamRunning       int
	TeamPending       int
	TeamIncomplete    int
	TeamScheduler     bool
}

type Controller interface {
	ShowSystemMessage(message string)
	SendUserMessage(ctx context.Context, text string) error
	SetPlanMode(enabled bool)
	PlanMode() bool
	ClearConversation() error
	Status() State
	Compact(ctx context.Context) string
	ListSessions() string
	ResumeSession(ctx context.Context, id string) string
	Notes(command string, args string) string
	Permissions(command string) string
	Skills(ctx context.Context, args string) string
	Workers(ctx context.Context, args string) string
	Worktrees(ctx context.Context, args string) string
	Teams(ctx context.Context, args string) string
	RunSkill(ctx context.Context, name string, args string) (string, error)
}

func (r Result) Emit(controller Controller) Result {
	for _, message := range r.Messages {
		controller.ShowSystemMessage(message)
	}
	if r.Err != nil {
		controller.ShowSystemMessage("command error: " + r.Err.Error())
	}
	return r
}

func Message(text string) Result {
	return Result{Messages: []string{text}}
}

func Messagef(format string, args ...any) Result {
	return Result{Messages: []string{fmt.Sprintf(format, args...)}}
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "/"))
}

package command

import (
	"context"
	"strings"
	"testing"

	"mewcode/internal/provider"
)

func TestRegistryRejectsNameAndAliasConflicts(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Command{Name: "Help", Aliases: []string{"h"}}); err != nil {
		t.Fatalf("register help: %v", err)
	}
	for _, cmd := range []Command{
		{Name: "help"},
		{Name: "status", Aliases: []string{"h"}},
		{Name: "h"},
		{Name: "other", Aliases: []string{"help"}},
	} {
		if err := r.Register(cmd); err == nil {
			t.Fatalf("expected conflict for %#v", cmd)
		}
	}
	if _, ok := r.Lookup("/H"); !ok {
		t.Fatalf("alias lookup failed")
	}
}

func TestParseCommandsAndPlainText(t *testing.T) {
	plain := Parse("hello")
	if plain.IsCommand || plain.Raw != "hello" {
		t.Fatalf("plain parse = %#v", plain)
	}
	parsed := Parse("/HELP compact")
	if !parsed.IsCommand || parsed.Name != "help" || parsed.Args != "compact" {
		t.Fatalf("command parse = %#v", parsed)
	}
	parsed = Parse("/resume abc 123")
	if parsed.Name != "resume" || parsed.Args != "abc 123" {
		t.Fatalf("resume parse = %#v", parsed)
	}
}

func TestCompletionSingleMultipleAndHidden(t *testing.T) {
	r := NewRegistry()
	for _, cmd := range []Command{
		{Name: "compact", Aliases: []string{"ctx"}},
		{Name: "clear"},
		{Name: "sessions"},
		{Name: "status"},
		{Name: "secret", Hidden: true},
		{Name: "notes", Subcommands: []string{"path", "clear"}},
		{Name: "permissions", Subcommands: []string{"clear-session"}},
	} {
		if err := r.Register(cmd); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	if got := r.Complete("/co").Replacement; got != "/compact" {
		t.Fatalf("single replacement = %q", got)
	}
	if got := strings.Join(r.Complete("/s").Candidates, ","); got != "sessions,status" {
		t.Fatalf("multi candidates = %q", got)
	}
	if strings.Contains(strings.Join(r.Complete("/se").Candidates, ","), "secret") || r.Complete("/se").Replacement == "/secret" {
		t.Fatalf("hidden command completed")
	}
	if got := r.Complete("/notes c").Replacement; got != "/notes clear" {
		t.Fatalf("notes subcommand = %q", got)
	}
	if got := r.Complete("/permissions c").Replacement; got != "/permissions clear-session" {
		t.Fatalf("permissions subcommand = %q", got)
	}
}

func TestBuiltinsCompletionIncludesWorkers(t *testing.T) {
	r, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins returned error: %v", err)
	}
	if got := r.Complete("/worke").Replacement; got != "/workers" {
		t.Fatalf("workers completion = %q", got)
	}
	if got := r.Complete("/workt").Replacement; got != "/worktrees" {
		t.Fatalf("worktrees completion = %q", got)
	}
	if got := r.Complete("/worktrees cr").Replacement; got != "/worktrees create" {
		t.Fatalf("worktrees subcommand = %q", got)
	}
	if got := r.Complete("/teams sch").Replacement; got != "/teams scheduler" {
		t.Fatalf("teams subcommand = %q", got)
	}
}

func TestBuiltinsVersionCommand(t *testing.T) {
	r, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins returned error: %v", err)
	}
	if got := r.Complete("/ver").Replacement; got != "/version" {
		t.Fatalf("version completion = %q", got)
	}
	if _, ok := r.Lookup("/v"); ok {
		t.Fatal("unexpected /v alias")
	}

	c := &fakeController{}
	result := Dispatch(context.Background(), r, c, "/version")
	if got := strings.Join(result.Messages, "\n"); got != "MewCode dev" {
		t.Fatalf("version output = %q", got)
	}
	if help := r.Help(""); !strings.Contains(help, "/version") {
		t.Fatalf("help output missing /version: %q", help)
	}
}

func TestPanelMatchesCommandsByNameAliasAndGroup(t *testing.T) {
	r, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins returned error: %v", err)
	}
	items := r.PanelItems("/wo", 8)
	if got := panelNames(items); got != "workers,worktrees" {
		t.Fatalf("panel names = %q", got)
	}
	for _, item := range items {
		if item.Description == "" || item.Usage == "" || item.Group == "" {
			t.Fatalf("incomplete panel item: %#v", item)
		}
	}
	items = r.PanelItems("/st", 8)
	if got := panelNames(items); got != "status" {
		t.Fatalf("alias panel names = %q", got)
	}
	items = r.PanelItems("/workers ", 8)
	if got := panelNames(items); got != "workers cancel,workers list,workers show" {
		t.Fatalf("worker subcommand panel names = %q", got)
	}
	items = r.PanelItems("/workers l", 8)
	if got := panelNames(items); got != "workers list" {
		t.Fatalf("worker filtered subcommand panel names = %q", got)
	}
}

func panelNames(items []PanelItem) string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return strings.Join(names, ",")
}

func TestBuiltinsDispatchLocalAndAIPrompt(t *testing.T) {
	r, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins returned error: %v", err)
	}
	if err := RegisterSkillCommands(r, []SkillCommand{{Name: "review", Description: "review skill"}}); err != nil {
		t.Fatalf("RegisterSkillCommands returned error: %v", err)
	}
	c := &fakeController{}
	Dispatch(context.Background(), r, c, "/help")
	if !strings.Contains(strings.Join(c.messages, "\n"), "/compact") {
		t.Fatalf("help output = %#v", c.messages)
	}
	result := Dispatch(context.Background(), r, c, "/review internal/tui")
	if !strings.Contains(result.SendToAgent, "internal/tui") {
		t.Fatalf("review result = %#v", result)
	}
	Dispatch(context.Background(), r, c, "/unknown")
	if !strings.Contains(c.messages[len(c.messages)-1], "/help") {
		t.Fatalf("unknown output = %#v", c.messages)
	}
	result = Dispatch(context.Background(), r, c, "/workers show task_1")
	if !strings.Contains(strings.Join(result.Messages, "\n"), "show task_1") {
		t.Fatalf("workers result = %#v", result)
	}
	result = Dispatch(context.Background(), r, c, "/worktrees status")
	if !strings.Contains(strings.Join(result.Messages, "\n"), "worktree status") {
		t.Fatalf("worktrees result = %#v", result)
	}
	result = Dispatch(context.Background(), r, c, "/teams status")
	if !strings.Contains(strings.Join(result.Messages, "\n"), "team status") {
		t.Fatalf("teams result = %#v", result)
	}
}

func TestRegisterSkillCommandsRejectsBuiltinConflict(t *testing.T) {
	r, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins returned error: %v", err)
	}
	if err := RegisterSkillCommands(r, []SkillCommand{{Name: "help", Description: "conflict"}}); err == nil {
		t.Fatalf("expected conflict")
	}
}

type fakeController struct {
	messages []string
	plan     bool
}

func (f *fakeController) ShowSystemMessage(message string) { f.messages = append(f.messages, message) }
func (f *fakeController) SendUserMessage(ctx context.Context, text string) error {
	f.messages = append(f.messages, "user:"+text)
	return nil
}
func (f *fakeController) SetPlanMode(enabled bool) { f.plan = enabled }
func (f *fakeController) PlanMode() bool           { return f.plan }
func (f *fakeController) ClearConversation() error { return nil }
func (f *fakeController) Status() State {
	return State{Mode: "execute", SessionID: "s1", LastUsage: provider.Usage{InputTokens: 1}}
}
func (f *fakeController) Compact(ctx context.Context) string                  { return "compacted" }
func (f *fakeController) ListSessions() string                                { return "sessions" }
func (f *fakeController) ResumeSession(ctx context.Context, id string) string { return "resumed " + id }
func (f *fakeController) Notes(command string, args string) string            { return "notes " + args }
func (f *fakeController) Permissions(command string) string                   { return "permissions " + command }
func (f *fakeController) Skills(ctx context.Context, args string) string      { return "skills " + args }
func (f *fakeController) Workers(ctx context.Context, args string) string     { return "worker " + args }
func (f *fakeController) Worktrees(ctx context.Context, args string) string {
	return "worktree " + args
}
func (f *fakeController) Teams(ctx context.Context, args string) string {
	return "team " + args
}
func (f *fakeController) RunSkill(ctx context.Context, name string, args string) (string, error) {
	return "skill:" + name + ":" + args, nil
}

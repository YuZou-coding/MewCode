package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"mewcode/internal/agent"
	"mewcode/internal/chat"
	"mewcode/internal/command"
	"mewcode/internal/compact"
	"mewcode/internal/external"
	"mewcode/internal/hooks"
	"mewcode/internal/instructions"
	"mewcode/internal/memory"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
	"mewcode/internal/sessionstore"
	"mewcode/internal/skill"
	"mewcode/internal/team"
	"mewcode/internal/tool"
	"mewcode/internal/worker"
	"mewcode/internal/worktree"
)

type synchronizedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *synchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

type Loop struct {
	Input             io.Reader
	Output            io.Writer
	Errors            io.Writer
	Session           *chat.Session
	Provider          provider.Provider
	Registry          *tool.Registry
	Tools             []tool.Definition
	PermissionChecker *permissions.Checker
	Compact           *compact.Manager
	MaxIterations     int
	PlanOnly          bool
	ToolTimeout       time.Duration
	TypeDelay         time.Duration
	NoTypeDelay       bool
	ContextMessages   []chat.Message
	SessionStore      *sessionstore.SessionStore
	SessionCatalog    sessionstore.Store
	Notes             *memory.Notes
	NoteUpdater       *memory.Updater
	SkillManager      *skill.Manager
	HookEngine        *hooks.Engine
	WorkerManager     *worker.Manager
	ExternalManager   *external.Manager
	TeamManager       *team.Manager
	WorktreeManager   *worktree.Manager
	WorktreeCleaned   int
	HomeDir           string
}

type Controller struct {
	loop             *Loop
	ctx              context.Context
	output           io.Writer
	errorsOut        io.Writer
	session          *chat.Session
	lastUsage        provider.Usage
	workingDirectory string
	gitBranch        string
}

func NewController(loop *Loop, ctx context.Context, output io.Writer, errorsOut io.Writer, session *chat.Session) *Controller {
	controller := &Controller{loop: loop, ctx: ctx, output: output, errorsOut: errorsOut, session: session}
	if loop.PermissionChecker != nil {
		controller.workingDirectory = loop.PermissionChecker.Root
	}
	if controller.workingDirectory != "" {
		cmd := exec.Command("git", "-C", controller.workingDirectory, "branch", "--show-current")
		if value, err := cmd.Output(); err == nil {
			controller.gitBranch = strings.TrimSpace(string(value))
		}
	}
	return controller
}

const banner = `
 /\_/\
( o.o )  MewCode
 > ^ <

Terminal AI coding assistant
Type /exit to quit

`

func (l Loop) Run(ctx context.Context) error {
	input := l.Input
	if input == nil {
		input = strings.NewReader("")
	}
	output := l.Output
	if output == nil {
		output = io.Discard
	}
	output = &synchronizedWriter{writer: output}
	errorsOut := l.Errors
	if errorsOut == nil {
		errorsOut = io.Discard
	}
	errorsOut = &synchronizedWriter{writer: errorsOut}
	session := l.Session
	if session == nil {
		session = chat.NewSession()
	}
	if l.Provider == nil {
		return errors.New("provider is required")
	}
	if l.SessionStore != nil {
		session.SetAppendHook(func(message chat.Message) {
			if err := l.SessionStore.Append(message); err != nil {
				_, _ = fmt.Fprintf(errorsOut, "session append failed: %v\n", err)
			}
		})
	}

	if err := PrintBanner(output); err != nil {
		return err
	}

	scanner := bufio.NewScanner(input)
	turns := 0
	notesCleared := false
	cmdRegistry, err := command.Builtins()
	if err != nil {
		return err
	}
	if l.SkillManager != nil {
		if err := command.RegisterSkillCommands(cmdRegistry, skillCommands(l.SkillManager)); err != nil {
			return err
		}
	}
	controller := NewController(&l, ctx, output, errorsOut, session)
	for {
		if _, err := fmt.Fprint(output, "You > "); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		result := command.Dispatch(ctx, cmdRegistry, controller, text)
		for _, message := range result.Messages {
			if _, err := fmt.Fprintf(output, "MewCode > %s\n", message); err != nil {
				return err
			}
		}
		if result.Err != nil {
			if _, err := fmt.Fprintf(output, "MewCode > command error: %v\n", result.Err); err != nil {
				return err
			}
		}
		if result.Exit {
			if l.HookEngine != nil {
				_ = l.HookEngine.Fire(ctx, hooks.Context{Event: hooks.EventSessionEnd})
				_ = l.HookEngine.Fire(ctx, hooks.Context{Event: hooks.EventSystemExit})
			}
			if l.NoteUpdater != nil && !notesCleared && len(session.Messages()) > 0 {
				if err := l.NoteUpdater.Update(ctx, session.Messages()); err != nil {
					_, _ = fmt.Fprintf(errorsOut, "notes update failed: %v\n", err)
				}
			}
			return nil
		}
		if result.SendToAgent == "" && command.Parse(text).IsCommand {
			if strings.HasPrefix(text, "/notes clear ") {
				notesCleared = true
			}
			continue
		}
		if result.SendToAgent != "" {
			text = result.SendToAgent
		}

		if _, err := fmt.Fprint(output, "MewCode > thinking..."); err != nil {
			return err
		}
		if err := l.runAgentTurn(ctx, scanner, output, errorsOut, session, text, func(usage provider.Usage) { controller.lastUsage = usage }); err != nil {
			return err
		}
		notesCleared = false
		turns++
		if l.NoteUpdater != nil {
			if err := l.NoteUpdater.MaybeUpdate(ctx, turns, session.Messages()); err != nil {
				_, _ = fmt.Fprintf(errorsOut, "notes update failed: %v\n", err)
			}
		}
	}
}

func PrintBanner(w io.Writer) error {
	_, err := io.WriteString(w, banner)
	return err
}

func (c *Controller) ShowSystemMessage(message string) {}

func (c *Controller) SendUserMessage(ctx context.Context, text string) error {
	return c.loop.runAgentTurn(ctx, bufio.NewScanner(strings.NewReader("")), c.output, c.errorsOut, c.session, text, func(usage provider.Usage) { c.lastUsage = usage })
}

func (c *Controller) SetPlanMode(enabled bool) { c.loop.PlanOnly = enabled }

func (c *Controller) PlanMode() bool { return c.loop.PlanOnly }

func (c *Controller) ClearConversation() error {
	c.session.SetAppendHook(nil)
	c.session.ReplaceMessages(nil)
	if c.loop.SkillManager != nil {
		c.loop.SkillManager.ClearActive()
	}
	if c.loop.SessionCatalog.ProjectRoot != "" {
		store, err := c.loop.SessionCatalog.Create()
		if err != nil {
			return err
		}
		c.loop.SessionStore = store
		c.session.SetAppendHook(func(message chat.Message) {
			if err := store.Append(message); err != nil {
				_, _ = fmt.Fprintf(c.errorsOut, "session append failed: %v\n", err)
			}
		})
	}
	return nil
}

func (c *Controller) Status() command.State {
	mode := "execute"
	if c.loop.PlanOnly {
		mode = "plan"
	}
	sessionID := ""
	if c.loop.SessionStore != nil {
		sessionID = c.loop.SessionStore.ID
	}
	state := command.State{Mode: mode, SessionID: sessionID, MessageCount: len(c.session.Messages()), LastUsage: c.lastUsage}
	state.WorkingDirectory = c.workingDirectory
	state.GitBranch = c.gitBranch
	state.ContextPercent = compact.HistorySize(c.session.Messages()) * 100 / compact.HistorySummaryThreshold
	if state.ContextPercent > 100 {
		state.ContextPercent = 100
	}
	if c.loop.HookEngine != nil {
		state.HookRules = c.loop.HookEngine.RuleCount()
		state.HookWarnings = c.loop.HookEngine.WarningCount()
	}
	if c.loop.WorkerManager != nil {
		state.WorkerRunning = c.loop.WorkerManager.RunningCount()
		state.WorkerCompleted = c.loop.WorkerManager.RecentCompletedCount()
	}
	if c.loop.ExternalManager != nil {
		state.MCPConnected = c.loop.ExternalManager.CachedCount()
	}
	if c.loop.WorktreeManager != nil {
		state.WorktreeMainRoot = c.loop.WorktreeManager.MainRoot
		state.WorktreeCleaned = c.loop.WorktreeCleaned
		if wtState, err := c.loop.WorktreeManager.LoadState(); err == nil {
			state.WorktreeName = wtState.ActiveName
			state.WorktreePath = wtState.ActivePath
		}
	}
	if c.loop.TeamManager != nil {
		stats := c.loop.TeamManager.Stats()
		state.TeamActive = stats.ActiveTeam
		state.TeamLead = stats.Lead
		state.TeamRunning = stats.RunningMembers
		state.TeamPending = stats.PendingMessages
		state.TeamIncomplete = stats.IncompleteTasks
		state.TeamScheduler = stats.SchedulerEnabled
	}
	return state
}

func (c *Controller) Compact(ctx context.Context) string {
	var fallback compact.Manager
	result := fallback.ManualCompact(ctx, c.session.Messages())
	if c.loop.Compact != nil {
		result = c.loop.Compact.ManualCompact(ctx, c.session.Messages())
	}
	c.session.ReplaceMessages(result.Messages)
	var b strings.Builder
	fmt.Fprintf(&b, "compacted messages %d -> %d, chars %d -> %d, artifacts %d", result.Stats.BeforeMessages, result.Stats.AfterMessages, result.Stats.BeforeChars, result.Stats.AfterChars, result.Stats.Artifacts)
	for _, compactErr := range result.Stats.Errors {
		fmt.Fprintf(&b, "\ncompact error: %v", compactErr)
	}
	return b.String()
}

func (c *Controller) ListSessions() string {
	var b strings.Builder
	if err := c.loop.printSessions(&b); err != nil {
		return "sessions error: " + err.Error()
	}
	return strings.TrimPrefix(strings.TrimSpace(b.String()), "MewCode > ")
}

func (c *Controller) ResumeSession(ctx context.Context, id string) string {
	var b strings.Builder
	if err := c.loop.resumeSession(ctx, strings.TrimSpace(id), c.session, &b); err != nil {
		return "resume error: " + err.Error()
	}
	return strings.ReplaceAll(strings.TrimSpace(b.String()), "MewCode > ", "")
}

func (c *Controller) Notes(commandName string, args string) string {
	text := "/notes"
	if strings.TrimSpace(args) != "" {
		text += " " + strings.TrimSpace(args)
	}
	var b strings.Builder
	if err := c.loop.handleNotes(text, &b); err != nil {
		return "notes error: " + err.Error()
	}
	return strings.ReplaceAll(strings.TrimSpace(b.String()), "MewCode > ", "")
}

func (c *Controller) Permissions(commandName string) string {
	checker := c.loop.PermissionChecker
	if checker == nil {
		return "permissions are not configured"
	}
	fields := strings.Fields(commandName)
	if len(fields) == 2 && fields[0] == "mode" {
		if fields[1] == "reset" {
			checker.ResetMode()
			return "permission mode=" + string(checker.CurrentMode())
		}
		mode, ok := permissions.ParseMode(strings.ToLower(fields[1]))
		if !ok || !checker.SetMode(mode) {
			return "permissions error: mode must be strict, default, yolo, or reset"
		}
		return "permission mode=" + string(checker.CurrentMode())
	}
	if len(fields) == 1 && fields[0] == "clear-session" {
		if checker.Session != nil {
			checker.Session.Clear()
		}
		return "cleared session permissions"
	}
	sessionRules := 0
	if checker.Session != nil {
		sessionRules = len(checker.Session.Rules())
	}
	return fmt.Sprintf("permissions mode=%s default_mode=%s user=%d project=%d session=%d", checker.CurrentMode(), checker.InitialMode(), len(checker.User), len(checker.Project), sessionRules)
}

func (c *Controller) Skills(ctx context.Context, args string) string {
	manager := c.loop.SkillManager
	if manager == nil {
		return "skills are not configured"
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		var b strings.Builder
		for _, skill := range manager.List() {
			fmt.Fprintf(&b, "%s | %s | mode=%s source=%s\n", skill.Name, skill.Description, skill.Mode, skill.Source)
		}
		if strings.TrimSpace(b.String()) == "" {
			return "no skills"
		}
		return strings.TrimSpace(b.String())
	}
	switch fields[0] {
	case "show":
		if len(fields) < 2 {
			return "skills show error: missing skill name"
		}
		skill, ok := manager.Get(fields[1])
		if !ok {
			return "skills show error: skill not found: " + fields[1]
		}
		return fmt.Sprintf("name=%s\ndescription=%s\nmode=%s\nmodel=%s\ncontext=%s\ntools=%s\nsource=%s\npath=%s",
			skill.Name, skill.Description, skill.Mode, skill.Model, skill.Context, strings.Join(skill.Tools, ","), skill.Source, skill.Path)
	case "run":
		if len(fields) < 2 {
			return "skills run error: missing skill name"
		}
		skillName := fields[1]
		prompt, err := c.RunSkill(ctx, skillName, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args), "run "+skillName)))
		if err != nil {
			return "skills run error: " + err.Error()
		}
		selected, _ := manager.Get(skillName)
		if selected.Mode == skill.ModeIsolated {
			return prompt
		}
		if err := c.SendUserMessage(ctx, prompt); err != nil {
			return "skills run error: " + err.Error()
		}
		return "skill sent: " + skillName
	case "reload":
		loaded, err := skill.Load(manager.ProjectRoot, manager.HomeDir, c.loop.Registry)
		if err != nil {
			return "skills reload error: " + err.Error()
		}
		manager.Skills = loaded.Skills
		manager.Warnings = loaded.Warnings
		manager.ClearActive()
		return fmt.Sprintf("skills reloaded: %d", len(manager.Skills))
	default:
		return "unknown skills command"
	}
}

func (c *Controller) RunSkill(ctx context.Context, name string, args string) (string, error) {
	manager := c.loop.SkillManager
	if manager == nil {
		return "", fmt.Errorf("skills are not configured")
	}
	if _, err := manager.RefreshSkill(name); err != nil {
		return "", err
	}
	selectedSkill, err := manager.Activate(name)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(args)
	if target == "" {
		target = "当前工作区"
	}
	promptText := fmt.Sprintf("请使用 %s Skill 处理以下请求：%s", selectedSkill.Name, target)
	if selectedSkill.Mode != skill.ModeIsolated {
		return promptText, nil
	}
	return c.runIsolatedSkill(ctx, selectedSkill, promptText)
}

func (c *Controller) Workers(ctx context.Context, args string) string {
	manager := c.loop.WorkerManager
	if manager == nil {
		return "workers are not configured"
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		tasks := manager.Tasks()
		if len(tasks) == 0 {
			return "no workers"
		}
		var b strings.Builder
		for _, task := range tasks {
			fmt.Fprintf(&b, "%s | role=%s | fork=%v | status=%s | started=%s | ended=%s | tokens=%d/%d\n",
				task.ID, task.RoleName, task.Fork, task.Status, formatTime(task.StartedAt), formatTime(task.EndedAt), task.Usage.InputTokens, task.Usage.OutputTokens)
		}
		return strings.TrimSpace(b.String())
	}
	switch fields[0] {
	case "show":
		if len(fields) < 2 {
			return "workers show error: missing task id"
		}
		task, ok := manager.Task(fields[1])
		if !ok {
			return "workers show error: task not found: " + fields[1]
		}
		return fmt.Sprintf("id=%s\nrole=%s\nfork=%v\nstatus=%s\ntask=%s\nresult=%s\nerror=%s\nnotification=%s\ntokens=%d/%d",
			task.ID, task.RoleName, task.Fork, task.Status, task.Task, task.Result, task.Error, task.Notification, task.Usage.InputTokens, task.Usage.OutputTokens)
	case "cancel":
		if len(fields) < 2 {
			return "workers cancel error: missing task id"
		}
		if !manager.Cancel(fields[1]) {
			return "workers cancel error: task not running: " + fields[1]
		}
		return "worker canceled: " + fields[1]
	default:
		return "unknown workers command"
	}
}

func (c *Controller) Worktrees(ctx context.Context, args string) string {
	manager := c.loop.WorktreeManager
	if manager == nil {
		return "worktrees are not configured"
	}
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "status" {
		return c.worktreeStatus()
	}
	switch fields[0] {
	case "create":
		if len(fields) < 2 {
			return "worktrees create error: missing name"
		}
		info, err := manager.Create(ctx, fields[1])
		if err != nil {
			return "worktrees create error: " + err.Error()
		}
		return worktreeLine("created", info)
	case "list":
		items, err := manager.List()
		if err != nil {
			return "worktrees list error: " + err.Error()
		}
		if len(items) == 0 {
			return "no worktrees"
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "%s | branch=%s | path=%s | active=%v\n", item.Name, item.Branch, item.Path, item.Active)
		}
		return strings.TrimSpace(b.String())
	case "enter":
		if len(fields) < 2 {
			return "worktrees enter error: missing name"
		}
		info, err := manager.Enter(ctx, fields[1])
		if err != nil {
			return "worktrees enter error: " + err.Error()
		}
		c.loop.refreshProjectRoot(ctx, info.Path)
		return worktreeLine("entered", info)
	case "exit":
		if err := manager.Exit(); err != nil {
			return "worktrees exit error: " + err.Error()
		}
		c.loop.refreshProjectRoot(ctx, manager.MainRoot)
		return "exited worktree: " + manager.MainRoot
	case "delete":
		if len(fields) < 2 {
			return "worktrees delete error: missing name"
		}
		force := false
		for _, field := range fields[2:] {
			if field == "--force" {
				force = true
			}
		}
		if err := manager.Delete(ctx, fields[1], force); err != nil {
			return "worktrees delete error: " + err.Error()
		}
		return "deleted worktree: " + fields[1]
	default:
		return "unknown worktrees command"
	}
}

func (c *Controller) Teams(ctx context.Context, args string) string {
	manager := c.loop.TeamManager
	if manager == nil {
		return "teams are not configured"
	}
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "status" {
		stats := manager.Stats()
		return fmt.Sprintf("active=%s lead=%s running=%d pending=%d incomplete=%d scheduler=%v warnings=%d", stats.ActiveTeam, stats.Lead, stats.RunningMembers, stats.PendingMessages, stats.IncompleteTasks, stats.SchedulerEnabled, stats.Warnings)
	}
	switch fields[0] {
	case "create":
		if len(fields) < 2 {
			return "teams create error: missing name"
		}
		created, err := manager.Create(fields[1])
		if err != nil {
			return "teams create error: " + err.Error()
		}
		return fmt.Sprintf("created team %s backend=%s", created.Name, created.Backend)
	case "list":
		items, err := manager.List()
		if err != nil {
			return "teams list error: " + err.Error()
		}
		if len(items) == 0 {
			return "no teams"
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "%s | lead=%s | backend=%s | status=%s\n", item.Name, item.Lead, item.Backend, item.Status)
		}
		return strings.TrimSpace(b.String())
	case "show":
		if len(fields) < 2 {
			return "teams show error: missing name"
		}
		item, err := manager.Load(fields[1])
		if err != nil {
			return "teams show error: " + err.Error()
		}
		members, _ := manager.Members(item.Name)
		tasks, _ := manager.ListTasks(item.Name)
		return fmt.Sprintf("name=%s\nlead=%s\nbackend=%s\nstatus=%s\nmembers=%d\ntasks=%d\nroot=%s", item.Name, item.Lead, item.Backend, item.Status, len(members), len(tasks), item.Root)
	case "start":
		if len(fields) < 2 {
			return "teams start error: missing name"
		}
		item, err := manager.Start(fields[1])
		if err != nil {
			return "teams start error: " + err.Error()
		}
		return "started team " + item.Name
	case "stop":
		if len(fields) < 2 {
			return "teams stop error: missing name"
		}
		if err := manager.Stop(fields[1]); err != nil {
			return "teams stop error: " + err.Error()
		}
		return "stopped team " + fields[1]
	case "send":
		if len(fields) < 3 {
			return "teams send error: usage /teams send <member> <message>"
		}
		stats := manager.Stats()
		if stats.ActiveTeam == "" {
			return "teams send error: no active team"
		}
		body := strings.TrimSpace(strings.TrimPrefix(args, "send "+fields[1]))
		sent, err := manager.SendMessage(stats.ActiveTeam, stats.Lead, fields[1], team.MessageText, body, "", nil)
		if err != nil {
			return "teams send error: " + err.Error()
		}
		return fmt.Sprintf("sent messages: %d", len(sent))
	case "scheduler":
		if len(fields) < 2 {
			return "teams scheduler error: missing on|off"
		}
		if fields[1] != "on" && fields[1] != "off" {
			return "teams scheduler error: expected on|off"
		}
		enabled := fields[1] == "on"
		if err := manager.SetSchedulerEnabled(enabled); err != nil {
			return "teams scheduler error: " + err.Error()
		}
		return fmt.Sprintf("team scheduler=%v", enabled)
	default:
		return "unknown teams command"
	}
}

func (c *Controller) worktreeStatus() string {
	state, _ := c.loop.WorktreeManager.LoadState()
	return fmt.Sprintf("main=%s cwd=%s active=%s path=%s cleaned=%d", c.loop.WorktreeManager.MainRoot, mustGetwd(), state.ActiveName, state.ActivePath, c.loop.WorktreeCleaned)
}

func (c *Controller) runIsolatedSkill(ctx context.Context, selected skill.Skill, promptText string) (string, error) {
	isolatedManager := skill.NewManager(c.loop.SkillManager.ProjectRoot, c.loop.SkillManager.HomeDir, skill.LoadResult{Skills: c.loop.SkillManager.Skills})
	if _, err := isolatedManager.Activate(selected.Name); err != nil {
		return "", err
	}
	session := chat.NewSession()
	switch selected.Context {
	case skill.ContextEmpty:
	case skill.ContextFullSummary:
		session.AddSystem("主会话摘要：\n" + flattenMessages(c.session.Messages()))
	default:
		messages := c.session.Messages()
		if len(messages) > skill.RecentCount {
			messages = messages[len(messages)-skill.RecentCount:]
		}
		session.ReplaceMessages(messages)
	}
	runner := &agent.Agent{
		Provider:          c.loop.Provider,
		Registry:          c.loop.Registry,
		Session:           session,
		Tools:             c.loop.Tools,
		MaxIterations:     c.loop.MaxIterations,
		PlanOnly:          c.loop.PlanOnly,
		ToolTimeout:       c.loop.ToolTimeout,
		PermissionChecker: c.loop.PermissionChecker,
		Compact:           c.loop.Compact,
		ContextMessages:   c.loop.ContextMessages,
		SkillManager:      isolatedManager,
		HookEngine:        c.loop.HookEngine,
		TeamManager:       c.loop.TeamManager,
		TeamActor:         team.Actor{},
	}
	var final string
	for event := range runner.Run(ctx, promptText) {
		if event.Kind == agent.EventFinalResponse {
			final = event.Text
		}
		if event.Kind == agent.EventError && event.Error != nil {
			return "", event.Error
		}
	}
	if strings.TrimSpace(final) == "" {
		final = "(empty)"
	}
	return fmt.Sprintf("isolated skill %s summary: %s", selected.Name, final), nil
}

func flattenMessages(messages []chat.Message) string {
	var b strings.Builder
	for _, message := range messages {
		if strings.TrimSpace(message.Content) != "" {
			fmt.Fprintf(&b, "%s: %s\n", message.Role, message.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

func skillCommands(manager *skill.Manager) []command.SkillCommand {
	items := manager.List()
	commands := make([]command.SkillCommand, 0, len(items))
	for _, item := range items {
		commands = append(commands, command.SkillCommand{Name: item.Name, Description: item.Description, Mode: string(item.Mode)})
	}
	return commands
}

func (l Loop) runAgentTurn(ctx context.Context, scanner *bufio.Scanner, output io.Writer, errorsOut io.Writer, session *chat.Session, text string, onUsage func(provider.Usage)) error {
	runner := &agent.Agent{
		Provider:          l.Provider,
		Registry:          l.Registry,
		Session:           session,
		Tools:             l.Tools,
		MaxIterations:     l.MaxIterations,
		PlanOnly:          l.PlanOnly,
		ToolTimeout:       l.ToolTimeout,
		ConfirmTool:       confirmTool(scanner.Scan, scanner.Text, output),
		ConfirmCommand:    confirmCommand(scanner.Scan, scanner.Text, output),
		PermissionChecker: l.PermissionChecker,
		PermissionPrompt:  permissionPrompt(scanner.Scan, scanner.Text, output),
		Compact:           l.Compact,
		ContextMessages:   l.ContextMessages,
		SkillManager:      l.SkillManager,
		HookEngine:        l.HookEngine,
		WorkerManager:     l.WorkerManager,
		TeamManager:       l.TeamManager,
	}
	if l.TeamManager != nil {
		runner.TeamActor = l.TeamManager.ActiveActor()
	}
	startedAnswer := false
	startedAt := time.Now()
	delay := l.TypeDelay
	if delay == 0 && !l.NoTypeDelay {
		delay = 12 * time.Millisecond
	}

	for event := range runner.Run(ctx, text) {
		switch event.Kind {
		case agent.EventUserMessage:
		case agent.EventThinkingText:
		case agent.EventStreamText:
			if !startedAnswer {
				if _, err := fmt.Fprintf(output, "\nMewCode > first token in %s\nMewCode > ", formatDuration(time.Since(startedAt))); err != nil {
					return err
				}
				startedAnswer = true
			}
			if _, err := writeTypewriter(ctx, output, event.Text, delay); err != nil {
				return err
			}
		case agent.EventToolCallStart:
			if _, err := fmt.Fprintf(output, "\nMewCode > using tool %s...", event.ToolName); err != nil {
				return err
			}
		case agent.EventToolResult:
			if _, err := fmt.Fprint(output, "\nMewCode > using tool result...\nMewCode > thinking..."); err != nil {
				return err
			}
			startedAnswer = false
			startedAt = time.Now()
		case agent.EventFinalResponse:
		case agent.EventUsage:
			if onUsage != nil {
				onUsage(provider.Usage{
					InputTokens:      event.Usage.InputTokens,
					OutputTokens:     event.Usage.OutputTokens,
					CacheReadTokens:  event.Usage.CacheReadTokens,
					CacheWriteTokens: event.Usage.CacheWriteTokens,
				})
			}
		case agent.EventPermissionRequest:
			if _, err := fmt.Fprintf(output, "\nMewCode > permission required for %s", event.ToolName); err != nil {
				return err
			}
			if event.PermissionPath != "" {
				if _, err := fmt.Fprintf(output, " path=%s", event.PermissionPath); err != nil {
					return err
				}
			}
			if event.PermissionCommand != "" {
				if _, err := fmt.Fprintf(output, " command=%s", event.PermissionCommand); err != nil {
					return err
				}
			}
		case agent.EventError:
			if event.Error != nil {
				_, _ = fmt.Fprintf(errorsOut, "%v\n", event.Error)
			}
		}
	}
	_, err := fmt.Fprintln(output)
	return err
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func worktreeLine(prefix string, info worktree.Info) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s worktree: %s branch=%s path=%s", prefix, info.Name, info.Branch, info.Path)
	if info.FastRestored {
		b.WriteString(" fast_restored=true")
	}
	for _, warning := range info.Warnings {
		fmt.Fprintf(&b, "\nwarning: %s", warning)
	}
	return b.String()
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return cwd
}

func (l *Loop) refreshProjectRoot(ctx context.Context, root string) {
	if l.PermissionChecker != nil {
		l.PermissionChecker.Root = root
		l.PermissionChecker.Session = permissions.NewSessionStore()
	}
	if l.Compact != nil {
		l.Compact.Root = root
	}
	if l.Notes != nil {
		l.Notes.ProjectRoot = root
	}
	l.SessionCatalog.ProjectRoot = root
	if l.SessionStore != nil {
		l.SessionStore.ProjectRoot = root
	}
	home := l.HomeDir
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	loadedInstructions := instructions.Load(root, home)
	l.ContextMessages = instructions.Messages(loadedInstructions.Blocks)
	if loadedHooks, err := hooks.Load(root, home); err == nil {
		l.HookEngine = hooks.NewEngine(loadedHooks.Rules, nil)
	}
	if l.SkillManager != nil {
		if loaded, err := skill.Load(root, home, l.Registry); err == nil {
			l.SkillManager.ProjectRoot = root
			l.SkillManager.HomeDir = home
			l.SkillManager.Skills = loaded.Skills
			l.SkillManager.Warnings = loaded.Warnings
			l.SkillManager.ClearActive()
		}
	}
	if l.WorkerManager != nil {
		l.WorkerManager.SetParentMessages(nil)
	}
	_ = ctx
}

func (l Loop) printSessions(output io.Writer) error {
	metas, err := l.SessionCatalog.List()
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		_, err := fmt.Fprintln(output, "MewCode > no sessions")
		return err
	}
	for _, meta := range metas {
		if _, err := fmt.Fprintf(output, "MewCode > %s | %s | messages=%d | updated=%s\n", meta.ID, meta.Title, meta.MessageCount, meta.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (l Loop) resumeSession(ctx context.Context, id string, session *chat.Session, output io.Writer) error {
	if id == "" {
		_, err := fmt.Fprintln(output, "MewCode > resume error: missing session id")
		return err
	}
	store := l.SessionCatalog.Open(id)
	result := store.Restore(ctx, l.Compact)
	if len(result.Messages) == 0 {
		if _, err := fmt.Fprintf(output, "MewCode > resume error: session %s has no valid messages\n", id); err != nil {
			return err
		}
		return nil
	}
	session.SetAppendHook(nil)
	session.ReplaceMessages(result.Messages)
	l.SessionStore = store
	session.SetAppendHook(func(message chat.Message) {
		_ = store.Append(message)
	})
	if _, err := fmt.Fprintf(output, "MewCode > resumed %s messages=%d\n", id, len(result.Messages)); err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(output, "MewCode > restore warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func (l Loop) handleNotes(text string, output io.Writer) error {
	if l.Notes == nil {
		_, err := fmt.Fprintln(output, "MewCode > notes are not configured")
		return err
	}
	switch {
	case text == "/notes":
		_, err := fmt.Fprintf(output, "MewCode > user notes:\n%s\nMewCode > project notes:\n%s\n", emptyNote(l.Notes.ReadUser()), emptyNote(l.Notes.ReadProject()))
		return err
	case text == "/notes path":
		_, err := fmt.Fprintf(output, "MewCode > user notes path: %s\nMewCode > project notes path: %s\n", memory.UserNotesPath(l.Notes.HomeDir), memory.ProjectNotesPath(l.Notes.ProjectRoot))
		return err
	case strings.HasPrefix(text, "/notes clear "):
		target := strings.TrimSpace(strings.TrimPrefix(text, "/notes clear "))
		if target == "" {
			target = "all"
		}
		if err := l.Notes.Clear(target); err != nil {
			_, err = fmt.Fprintf(output, "MewCode > notes clear error: %v\n", err)
			return err
		}
		_, err := fmt.Fprintf(output, "MewCode > cleared notes: %s\n", target)
		return err
	default:
		_, err := fmt.Fprintln(output, "MewCode > unknown notes command")
		return err
	}
}

func emptyNote(content string) string {
	if strings.TrimSpace(content) == "" {
		return "(empty)"
	}
	return content
}

func permissionPrompt(scan func() bool, text func() string, output io.Writer) agent.PermissionPromptFunc {
	return func(ctx context.Context, request permissions.Request, decision permissions.Decision) permissions.HITLChoice {
		prompt := "\nAllow %s? [n] deny [y] allow once [s] allow session [a] allow always: "
		if decision.Mode == permissions.ModeStrict {
			prompt = "\nAllow %s? [n] deny [y] allow once: "
		}
		if _, err := fmt.Fprintf(output, prompt, request.Tool); err != nil {
			return permissions.HITLDeny
		}
		if !scan() {
			return permissions.HITLDeny
		}
		switch strings.ToLower(strings.TrimSpace(text())) {
		case "y", "yes":
			return permissions.HITLAllowOnce
		case "s", "session":
			if decision.Mode == permissions.ModeStrict {
				return permissions.HITLDeny
			}
			return permissions.HITLAllowSession
		case "a", "always":
			if decision.Mode == permissions.ModeStrict {
				return permissions.HITLDeny
			}
			return permissions.HITLAllowAlways
		default:
			return permissions.HITLDeny
		}
	}
}

func confirmTool(scan func() bool, text func() string, output io.Writer) agent.ConfirmToolFunc {
	return func(ctx context.Context, call provider.ToolCall) bool {
		if _, err := fmt.Fprintf(output, "\nAllow tool? %s %s [y/N]: ", call.Name, string(call.Arguments)); err != nil {
			return false
		}
		if !scan() {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(text()))
		return answer == "y" || answer == "yes"
	}
}

func confirmCommand(scan func() bool, text func() string, output io.Writer) agent.ConfirmCommandFunc {
	return func(ctx context.Context, command string) bool {
		if _, err := fmt.Fprintf(output, "\nAllow command? %s [y/N]: ", command); err != nil {
			return false
		}
		if !scan() {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(text()))
		return answer == "y" || answer == "yes"
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func writeTypewriter(ctx context.Context, w io.Writer, text string, delay time.Duration) (int, error) {
	written := 0
	for _, r := range text {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		n, err := io.WriteString(w, string(r))
		written += n
		if err != nil {
			return written, err
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return written, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return written, nil
}

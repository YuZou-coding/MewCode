package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"mewcode/internal/agent"
	"mewcode/internal/chat"
	"mewcode/internal/command"
	"mewcode/internal/compact"
	"mewcode/internal/config"
	"mewcode/internal/external"
	"mewcode/internal/hooks"
	"mewcode/internal/instructions"
	"mewcode/internal/memory"
	"mewcode/internal/permissions"
	"mewcode/internal/prompt"
	"mewcode/internal/provider"
	"mewcode/internal/sessionstore"
	"mewcode/internal/skill"
	"mewcode/internal/team"
	"mewcode/internal/tool"
	"mewcode/internal/tui"
	"mewcode/internal/tuiapp"
	"mewcode/internal/worker"
	"mewcode/internal/worktree"
)

type App struct {
	Input             io.Reader
	Output            io.Writer
	Errors            io.Writer
	Provider          provider.Provider
	Registry          *tool.Registry
	PermissionChecker *permissions.Checker
	ExternalManager   *external.Manager
	Compact           *compact.Manager
	PromptModules     []prompt.Module
	ContextMessages   []chat.Message
	SessionStore      *sessionstore.SessionStore
	SessionCatalog    *sessionstore.Store
	Notes             *memory.Notes
	NoteUpdater       *memory.Updater
	NoTypeDelay       bool
	ForceFullScreen   bool
	Config            config.Config
	ProviderFactory   provider.Factory
	WorktreeManager   *worktree.Manager
	TeamManager       *team.Manager
	ResumeWorktree    bool
}

func Run(ctx context.Context, input io.Reader, output io.Writer, errorsOut io.Writer) error {
	return RunWithProviderOptions(ctx, input, output, errorsOut)
}

func RunWithResume(ctx context.Context, input io.Reader, output io.Writer, errorsOut io.Writer, resume bool) error {
	return RunWithProviderOptionsAndResume(ctx, input, output, errorsOut, resume)
}

func RunWithProviderOptions(ctx context.Context, input io.Reader, output io.Writer, errorsOut io.Writer, opts ...provider.Option) error {
	return RunWithProviderOptionsAndResume(ctx, input, output, errorsOut, false, opts...)
}

func RunWithProviderOptionsAndResume(ctx context.Context, input io.Reader, output io.Writer, errorsOut io.Writer, resume bool, opts ...provider.Option) error {
	cfg, err := config.LoadProject()
	if err != nil {
		return err
	}
	modelProvider, err := provider.New(cfg, opts...)
	if err != nil {
		return err
	}
	return App{
		Input:           input,
		Output:          output,
		Errors:          errorsOut,
		Provider:        modelProvider,
		Config:          cfg,
		ProviderFactory: provider.NewFactory(opts...),
		ResumeWorktree:  resume,
	}.Run(ctx)
}

func (a App) Run(ctx context.Context) error {
	modules := a.PromptModules
	if len(modules) == 0 {
		modules = prompt.DefaultModules()
	}
	_ = prompt.Build(modules)
	registry := a.Registry
	if registry == nil {
		var err error
		registry, err = tool.DefaultRegistry()
		if err != nil {
			return err
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = cwd
	}
	worktreeManager := a.WorktreeManager
	if worktreeManager == nil {
		worktreeManager = worktree.NewManager(cwd, worktree.Config{
			CopyFiles: a.Config.WorktreeCopyFiles,
			LinkDirs:  a.Config.WorktreeLinkDirs,
			TTL:       worktreeTTL(a.Config.WorktreeTTL),
		})
	}
	cleanup := worktreeManager.Cleanup(ctx)
	if a.ResumeWorktree {
		if info, err := worktreeManager.Resume(ctx); err == nil {
			cwd = info.Path
			if a.Errors != nil {
				_, _ = io.WriteString(a.Errors, "resumed worktree "+info.Name+"\n")
			}
		} else if a.Errors != nil {
			_, _ = io.WriteString(a.Errors, "worktree resume skipped: "+err.Error()+"\n")
		}
	}
	manager := a.ExternalManager
	if manager == nil {
		servers, warnings, err := external.LoadMergedMCPServers(cwd, home)
		if err != nil {
			return err
		}
		for _, warning := range warnings {
			if a.Errors != nil {
				_, _ = io.WriteString(a.Errors, warning+"\n")
			}
		}
		manager = external.NewManager(servers, nil)
	}
	if manager != nil {
		if err := registry.Register(external.NewToolSearch(manager, registry)); err != nil {
			return err
		}
		defer manager.Close()
	}
	checker := a.PermissionChecker
	if checker == nil {
		userRules, err := permissions.LoadRulesFile(permissions.UserRulesFile(), permissions.SourceUser)
		if err != nil {
			return err
		}
		projectRules, err := permissions.LoadRulesFile(permissions.ProjectRulesFile(cwd), permissions.SourceProject)
		if err != nil {
			return err
		}
		checker = &permissions.Checker{
			Root:        cwd,
			Mode:        permissions.Mode(a.Config.PermissionMode),
			DefaultMode: permissions.Mode(a.Config.PermissionMode),
			Session:     permissions.NewSessionStore(),
			Project:     projectRules,
			User:        userRules,
		}
	}
	compactManager := a.Compact
	if compactManager == nil {
		compactManager = &compact.Manager{Root: cwd, Provider: a.Provider}
	}
	contextMessages := append([]chat.Message{}, a.ContextMessages...)
	loadedInstructions := instructions.Load(cwd, home)
	contextMessages = append(contextMessages, instructions.Messages(loadedInstructions.Blocks)...)
	if a.Errors != nil {
		for _, warning := range loadedInstructions.Warnings {
			_, _ = io.WriteString(a.Errors, warning+"\n")
		}
	}
	loadedSkills, err := skill.Load(cwd, home, registry)
	if err != nil {
		return err
	}
	skillManager := skill.NewManager(cwd, home, loadedSkills)
	if err := skill.RegisterTools(registry, skillManager); err != nil {
		return err
	}
	if a.Errors != nil {
		for _, warning := range loadedSkills.Warnings {
			_, _ = io.WriteString(a.Errors, warning+"\n")
		}
	}
	loadedHooks, err := hooks.Load(cwd, home)
	if err != nil {
		return err
	}
	hookEngine := hooks.NewEngine(loadedHooks.Rules, nil)
	if a.Errors != nil {
		for _, warning := range loadedHooks.Warnings {
			_, _ = io.WriteString(a.Errors, warning+"\n")
		}
	}
	_ = hookEngine.Fire(ctx, hooks.Context{Event: hooks.EventSystemStart})
	workerOpts := worker.Options{
		EnableVerify:        a.Config.WorkerEnableVerify,
		BackgroundThreshold: workerThreshold(a.Config.WorkerBackgroundThreshold),
	}
	loadedWorkers := worker.Load(cwd, home, workerOpts)
	workerManager := worker.NewManager(loadedWorkers, workerOpts)
	workerManager.Runner = a.workerRunner(registry, checker, compactManager, hookEngine, contextMessages, worktreeManager)
	if err := worker.RegisterTools(registry, workerManager); err != nil {
		return err
	}
	if a.Errors != nil {
		for _, warning := range loadedWorkers.Warnings {
			_, _ = io.WriteString(a.Errors, warning+"\n")
		}
	}
	teamManager := a.TeamManager
	if teamManager == nil {
		teamManager = team.NewManager(cwd, team.Options{
			DefaultBackend:        teamBackend(a.Config.TeamDefaultBackend),
			SchedulerAllowed:      a.Config.TeamSchedulerEnabled,
			DefaultMemberApproval: a.Config.TeamDefaultMemberApproval,
		})
	}
	if err := team.RegisterTools(registry, teamManager); err != nil {
		return err
	}
	catalog := a.SessionCatalog
	if catalog == nil {
		catalog = &sessionstore.Store{ProjectRoot: cwd}
	}
	activeStore := a.SessionStore
	if activeStore == nil {
		activeStore, err = catalog.Create()
		if err != nil {
			return err
		}
	}
	session := chat.NewSession()
	bindSessionStore(session, activeStore, a.Errors)
	notes := a.Notes
	if notes == nil {
		notes = &memory.Notes{HomeDir: home, ProjectRoot: cwd}
	}
	noteUpdater := a.NoteUpdater
	if noteUpdater == nil {
		noteUpdater = &memory.Updater{Notes: *notes, Provider: a.Provider}
	}
	loop := tui.Loop{
		Input:             a.Input,
		Output:            a.Output,
		Errors:            a.Errors,
		Session:           session,
		Provider:          a.Provider,
		Registry:          registry,
		Tools:             registry.Definitions(),
		PermissionChecker: checker,
		Compact:           compactManager,
		ContextMessages:   contextMessages,
		SessionStore:      activeStore,
		SessionCatalog:    *catalog,
		Notes:             notes,
		NoTypeDelay:       a.NoTypeDelay,
		SkillManager:      skillManager,
		HookEngine:        hookEngine,
		WorkerManager:     workerManager,
		ExternalManager:   manager,
		TeamManager:       teamManager,
		WorktreeManager:   worktreeManager,
		WorktreeCleaned:   cleanup.Removed,
		HomeDir:           home,
		MaxIterations:     a.Config.MaxIterations,
	}
	if a.ForceFullScreen || interactive(a.Input, a.Output) {
		cmdRegistry, err := command.Builtins()
		if err != nil {
			return err
		}
		if err := command.RegisterSkillCommands(cmdRegistry, skillCommandsForCommand(skillManager)); err != nil {
			return err
		}
		out := a.Output
		if out == nil {
			out = os.Stdout
		}
		errs := a.Errors
		if errs == nil {
			errs = os.Stderr
		}
		input := a.Input
		if input == nil {
			input = os.Stdin
		}
		controller := tui.NewController(&loop, ctx, io.Discard, errs, loop.Session)
		submit := newFullscreenSubmit(loop, noteUpdater, errs)
		err = tuiapp.Run(ctx, input, out, cmdRegistry, controller, submit)
		if err == nil && len(session.Messages()) > 0 {
			if updateErr := noteUpdater.Update(ctx, session.Messages()); updateErr != nil {
				_, _ = fmt.Fprintf(errs, "notes update failed: %v\n", updateErr)
			}
		}
		return err
	}
	return loop.Run(ctx)
}

func newFullscreenSubmit(loop tui.Loop, updater *memory.Updater, errorsOut io.Writer) tuiapp.SubmitFunc {
	turns := 0
	return func(ctx context.Context, text string, permissionPrompt tuiapp.PermissionPromptFunc) <-chan tuiapp.StreamEvent {
		out := make(chan tuiapp.StreamEvent, agent.EventBufferSize)
		if loop.WorkerManager != nil {
			loop.WorkerManager.WaitForRunning(ctx)
			for _, notification := range loop.WorkerManager.PendingNotifications() {
				out <- tuiapp.StreamEvent{Kind: tuiapp.StreamTextDelta, Text: fmt.Sprintf("worker %s %s: %s\n", notification.TaskID, notification.Status, workerNotificationResult(notification))}
			}
		}
		source := runAgentForTUI(ctx, loop, text, permissionPrompt)
		go func() {
			defer close(out)
			for event := range source {
				select {
				case <-ctx.Done():
					return
				case out <- event:
				}
			}
			if loop.WorkerManager != nil {
				loop.WorkerManager.WaitForRunning(ctx)
				for _, notification := range loop.WorkerManager.PendingNotifications() {
					out <- tuiapp.StreamEvent{Kind: tuiapp.StreamTextDelta, Text: fmt.Sprintf("worker %s %s: %s\n", notification.TaskID, notification.Status, workerNotificationResult(notification))}
				}
			}
			turns++
			if updater != nil {
				if err := updater.MaybeUpdate(ctx, turns, loop.Session.Messages()); err != nil {
					if errorsOut != nil {
						_, _ = fmt.Fprintf(errorsOut, "notes update failed: %v\n", err)
					}
				}
			}
		}()
		return out
	}
}

func workerNotificationResult(notification worker.Notification) string {
	if notification.Error != "" {
		return notification.Error
	}
	return notification.Result
}

func bindSessionStore(session *chat.Session, store *sessionstore.SessionStore, errorsOut io.Writer) {
	if session == nil || store == nil {
		return
	}
	if errorsOut == nil {
		errorsOut = io.Discard
	}
	session.SetAppendHook(func(message chat.Message) {
		if err := store.Append(message); err != nil {
			_, _ = fmt.Fprintf(errorsOut, "session append failed: %v\n", err)
		}
	})
}

func interactive(input io.Reader, output io.Writer) bool {
	inFile, inOK := input.(*os.File)
	outFile, outOK := output.(*os.File)
	return inOK && outOK && isatty.IsTerminal(inFile.Fd()) && isatty.IsTerminal(outFile.Fd())
}

func runAgentForTUI(ctx context.Context, loop tui.Loop, text string, permissionPrompt tuiapp.PermissionPromptFunc) <-chan tuiapp.StreamEvent {
	out := make(chan tuiapp.StreamEvent, agent.EventBufferSize)
	go func() {
		defer close(out)
		runner := &agent.Agent{
			Provider:          loop.Provider,
			Registry:          loop.Registry,
			Session:           loop.Session,
			Tools:             loop.Tools,
			MaxIterations:     loop.MaxIterations,
			PlanOnly:          loop.PlanOnly,
			ToolTimeout:       loop.ToolTimeout,
			PermissionChecker: loop.PermissionChecker,
			PermissionPrompt: func(ctx context.Context, request permissions.Request, decision permissions.Decision) permissions.HITLChoice {
				if permissionPrompt == nil {
					return permissions.HITLDeny
				}
				return permissionPrompt(ctx, request, decision)
			},
			Compact:         loop.Compact,
			ContextMessages: loop.ContextMessages,
			SkillManager:    loop.SkillManager,
			HookEngine:      loop.HookEngine,
			WorkerManager:   loop.WorkerManager,
			TeamManager:     loop.TeamManager,
		}
		if loop.TeamManager != nil {
			runner.TeamActor = loop.TeamManager.ActiveActor()
		}
		for event := range runner.Run(ctx, text) {
			streamEvent, ok := tuiEventFromAgent(event)
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- streamEvent:
			}
		}
		select {
		case <-ctx.Done():
		case out <- tuiapp.StreamEvent{Kind: tuiapp.StreamDone}:
		}
	}()
	return out
}

func tuiEventFromAgent(event agent.Event) (tuiapp.StreamEvent, bool) {
	switch event.Kind {
	case agent.EventThinkingText:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamThinking, Text: event.Text}, true
	case agent.EventStreamText:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamTextDelta, Text: event.Text}, true
	case agent.EventToolCallStart:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamToolStart, CallID: event.ToolCallID, ToolName: event.ToolName, Target: toolEventTarget(event.ToolArguments)}, true
	case agent.EventToolResult:
		stream := tuiapp.StreamEvent{Kind: tuiapp.StreamToolResult, CallID: event.ToolCallID, ToolName: event.ToolName, ToolOK: event.Result != nil && event.Result.OK}
		if event.Result != nil && event.Result.Error != nil {
			stream.ToolCode = event.Result.Error.Code
			stream.Text = event.Result.Error.Message
		}
		return stream, true
	case agent.EventPermissionRequest:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamPermissionRequest, ToolName: event.ToolName}, true
	case agent.EventUsage:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamUsage, Usage: provider.Usage{
			InputTokens:      event.Usage.InputTokens,
			OutputTokens:     event.Usage.OutputTokens,
			CacheReadTokens:  event.Usage.CacheReadTokens,
			CacheWriteTokens: event.Usage.CacheWriteTokens,
		}}, true
	case agent.EventError:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamError, Err: event.Error}, true
	case agent.EventIteration:
		return tuiapp.StreamEvent{Kind: tuiapp.StreamIteration, Iteration: event.Iteration, MaxIterations: event.MaxIterations}, true
	default:
		return tuiapp.StreamEvent{}, false
	}
}

func toolEventTarget(arguments []byte) string {
	var args map[string]any
	if json.Unmarshal(arguments, &args) != nil {
		return ""
	}
	for _, key := range []string{"path", "file_path", "root", "pattern", "command"} {
		if value, ok := args[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func (a App) workerRunner(registry *tool.Registry, checker *permissions.Checker, compactManager *compact.Manager, hookEngine *hooks.Engine, baseContext []chat.Message, worktreeManager *worktree.Manager) worker.RunnerFunc {
	return func(ctx context.Context, req worker.RunRequest) worker.RunResult {
		originalCWD, _ := os.Getwd()
		createdWorktree := worktree.Info{}
		isolated := req.Isolation == worker.IsolationWorktree || req.Role.Isolation == worker.IsolationWorktree
		if isolated {
			if worktreeManager == nil {
				return worker.RunResult{Error: fmt.Errorf("worktree manager is not configured")}
			}
			name := "worker/" + req.TaskID
			info, err := worktreeManager.Create(ctx, name)
			if err != nil {
				return worker.RunResult{Error: err}
			}
			createdWorktree = info
			if err := os.Chdir(info.Path); err != nil {
				return worker.RunResult{Error: err}
			}
			defer func() { _ = os.Chdir(originalCWD) }()
		}
		modelProvider := a.Provider
		cfg := a.Config
		if cfg.Protocol != "" {
			if req.Role.Model != "" {
				cfg.Model = req.Role.Model
			}
			if req.Model != "" {
				cfg.Model = req.Model
			}
			factory := a.ProviderFactory
			next, err := factory.New(cfg)
			if err != nil {
				return worker.RunResult{Error: err}
			}
			modelProvider = next
		}
		session := chat.NewSession()
		contextMessages := append([]chat.Message{}, baseContext...)
		if req.Fork {
			session.ReplaceMessages(req.ParentMessages)
			req.Task = worker.ForkInstruction(req.Task)
		} else if strings.TrimSpace(req.Role.Body) != "" {
			contextMessages = append(contextMessages, prompt.InternalInstruction("<mewcode-worker-role name=\""+req.Role.Name+"\">\n"+req.Role.Body+"\n</mewcode-worker-role>"))
		}
		if isolated {
			contextMessages = append(contextMessages, prompt.InternalInstruction(fmt.Sprintf("<mewcode-worktree-isolation>\nmain_root: %s\nworktree_root: %s\ncurrent_cwd: %s\n路径翻译：主仓库路径需要映射到 worktree_root 后再读写；不要混用旧 cwd。\n</mewcode-worktree-isolation>", worktreeManager.MainRoot, createdWorktree.Path, createdWorktree.Path)))
		}
		defs := worker.FilterDefinitions(registry.Definitions(), req.Role, req.Background)
		childRegistry := registry.CloneFiltered(defs)
		maxIterations := req.MaxIterations
		if maxIterations == 0 {
			maxIterations = req.Role.MaxIterations
		}
		childChecker := cloneChecker(checker)
		runner := &agent.Agent{
			Provider:          modelProvider,
			Registry:          childRegistry,
			Session:           session,
			Tools:             defs,
			MaxIterations:     maxIterations,
			ToolTimeout:       30 * time.Second,
			PermissionChecker: childChecker,
			Compact:           compactManager,
			ContextMessages:   contextMessages,
			HookEngine:        hookEngine,
			TeamManager:       nil,
		}
		var final string
		var usage provider.Usage
		for event := range runner.Run(ctx, req.Task) {
			switch event.Kind {
			case agent.EventFinalResponse:
				final = event.Text
			case agent.EventUsage:
				usage.InputTokens += event.Usage.InputTokens
				usage.OutputTokens += event.Usage.OutputTokens
				usage.CacheReadTokens += event.Usage.CacheReadTokens
				usage.CacheWriteTokens += event.Usage.CacheWriteTokens
			case agent.EventError:
				if event.Error != nil {
					return worker.RunResult{Text: final, Usage: usage, Error: event.Error}
				}
			}
		}
		if strings.TrimSpace(final) == "" {
			final = "(empty)"
		}
		if isolated {
			dirty, err := worktreeManager.IsDirty(ctx, createdWorktree.Path)
			if err == nil && !dirty {
				_ = os.Chdir(originalCWD)
				_ = worktreeManager.Delete(ctx, createdWorktree.Name, true)
				final += "\nworktree cleaned: " + createdWorktree.Name
			} else {
				final += "\nworktree retained: " + createdWorktree.Name
			}
		}
		return worker.RunResult{Text: final, Usage: usage}
	}
}

func workerThreshold(raw string) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return worker.DefaultThreshold
	}
	duration, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return worker.DefaultThreshold
	}
	return duration
}

func teamBackend(raw string) team.Backend {
	switch strings.TrimSpace(raw) {
	case string(team.BackendTerminalPane):
		return team.BackendTerminalPane
	default:
		return team.BackendInProcess
	}
}

func worktreeTTL(raw string) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return worktree.DefaultTTL
	}
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err == nil {
			return days * 24
		}
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return worktree.DefaultTTL
	}
	return duration
}

func cloneChecker(checker *permissions.Checker) *permissions.Checker {
	if checker == nil {
		return nil
	}
	return &permissions.Checker{
		Root:        checker.Root,
		Mode:        checker.CurrentMode(),
		DefaultMode: checker.DefaultMode,
		Session:     permissions.NewSessionStore(),
		Project:     append([]permissions.Rule(nil), checker.Project...),
		User:        append([]permissions.Rule(nil), checker.User...),
	}
}

func skillCommandsForCommand(manager *skill.Manager) []command.SkillCommand {
	items := manager.List()
	commands := make([]command.SkillCommand, 0, len(items))
	for _, item := range items {
		commands = append(commands, command.SkillCommand{Name: item.Name, Description: item.Description, Mode: string(item.Mode)})
	}
	return commands
}

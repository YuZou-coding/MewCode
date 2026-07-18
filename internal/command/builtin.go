package command

import (
	"context"
	"strings"

	"mewcode/internal/version"
)

func Builtins() (*Registry, error) {
	r := NewRegistry()
	for _, cmd := range []Command{
		{Name: "help", Aliases: []string{"h", "?"}, Description: "显示命令帮助", Usage: "/help [command]", Type: TypeLocal, ArgHint: "command", Handler: helpHandler(r)},
		{Name: "compact", Aliases: []string{"ctx"}, Description: "压缩当前上下文", Usage: "/compact", Type: TypeLocal, Handler: compactHandler},
		{Name: "clear", Aliases: []string{"reset"}, Description: "清空当前会话", Usage: "/clear", Type: TypeUIState, Handler: clearHandler},
		{Name: "plan", Description: "切换到规划模式", Usage: "/plan", Type: TypeUIState, Handler: planHandler},
		{Name: "do", Aliases: []string{"execute"}, Description: "切换到执行模式", Usage: "/do", Type: TypeUIState, Handler: doHandler},
		{Name: "sessions", Aliases: []string{"ls"}, Description: "列出历史会话", Usage: "/sessions", Type: TypeLocal, Handler: sessionsHandler},
		{Name: "resume", Aliases: []string{"r"}, Description: "恢复指定会话", Usage: "/resume <id>", Type: TypeUIState, ArgHint: "id", Handler: resumeHandler},
		{Name: "notes", Aliases: []string{"memory", "mem"}, Description: "查看和管理笔记", Usage: "/notes [path|clear user|clear project|clear all]", Type: TypeLocal, ArgHint: "path|clear", Subcommands: []string{"path", "clear"}, Handler: notesHandler},
		{Name: "permissions", Aliases: []string{"perms"}, Description: "查看和切换权限模式", Usage: "/permissions [strict|default|yolo|reset|clear-session|mode strict|mode default|mode yolo|mode reset]", Type: TypeLocal, ArgHint: "strict|default|yolo|reset|clear-session|mode", Subcommands: []string{"strict", "default", "yolo", "reset", "clear-session", "mode"}, Handler: permissionsHandler},
		{Name: "skills", Description: "查看和管理 Skill", Usage: "/skills [show <name>|run <name> [args]|reload]", Type: TypeLocal, ArgHint: "show|run|reload", Subcommands: []string{"show", "run", "reload"}, Handler: skillsHandler},
		{Name: "workers", Description: "查看和管理后台 worker", Usage: "/workers [list|show <id>|cancel <id>]", Type: TypeLocal, ArgHint: "list|show|cancel", Subcommands: []string{"list", "show", "cancel"}, Handler: workersHandler},
		{Name: "worktrees", Description: "查看和管理隔离工作目录", Usage: "/worktrees [create|list|enter|exit|status|delete]", Type: TypeUIState, ArgHint: "create|list|enter|exit|status|delete", Subcommands: []string{"create", "list", "enter", "exit", "status", "delete"}, Handler: worktreesHandler},
		{Name: "teams", Description: "查看和管理 Team 小组", Usage: "/teams [create|list|show|start|stop|send|status|scheduler]", Type: TypeUIState, ArgHint: "create|list|show|start|stop|send|status|scheduler", Subcommands: []string{"create", "list", "show", "start", "stop", "send", "status", "scheduler"}, Handler: teamsHandler},
		{Name: "status", Aliases: []string{"st"}, Description: "显示综合状态", Usage: "/status", Type: TypeLocal, Handler: statusHandler},
		{Name: "version", Description: "显示 MewCode 版本", Usage: "/version", Type: TypeLocal, Handler: versionHandler},
		{Name: "exit", Aliases: []string{"quit", "q"}, Description: "退出 MewCode", Usage: "/exit", Type: TypeLocal, Handler: exitHandler},
	} {
		if err := r.Register(cmd); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func helpHandler(r *Registry) Handler {
	return func(ctx context.Context, controller Controller, inv Invocation) Result {
		return Message(r.Help(inv.Args))
	}
}

func compactHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Compact(ctx))
}

func clearHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	if err := controller.ClearConversation(); err != nil {
		return Result{Err: err}
	}
	return Message("cleared conversation")
}

func planHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	controller.SetPlanMode(true)
	return Message("mode=plan")
}

func doHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	controller.SetPlanMode(false)
	return Message("mode=execute")
}

func sessionsHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	if strings.TrimSpace(inv.Args) != "" {
		return Message("sessions does not take an id; use /resume " + strings.TrimSpace(inv.Args))
	}
	return Message(controller.ListSessions())
}

func resumeHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	if strings.TrimSpace(inv.Args) == "" {
		return Message("resume error: missing session id")
	}
	return Message(controller.ResumeSession(ctx, inv.Args))
}

func notesHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Notes(inv.Name, inv.Args))
}

func permissionsHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Permissions(inv.Args))
}

func skillsHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Skills(ctx, inv.Args))
}

func workersHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Workers(ctx, inv.Args))
}

func worktreesHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Worktrees(ctx, inv.Args))
}

func teamsHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(controller.Teams(ctx, inv.Args))
}

func statusHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	state := controller.Status()
	return Messagef("mode=%s session=%s messages=%d tokens(in=%d out=%d cache_read=%d cache_write=%d) hooks(rules=%d warnings=%d) workers(running=%d completed=%d) worktree(name=%s path=%s main=%s cleaned=%d) team(active=%s lead=%s running=%d pending=%d incomplete=%d scheduler=%v)", state.Mode, state.SessionID, state.MessageCount, state.LastUsage.InputTokens, state.LastUsage.OutputTokens, state.LastUsage.CacheReadTokens, state.LastUsage.CacheWriteTokens, state.HookRules, state.HookWarnings, state.WorkerRunning, state.WorkerCompleted, state.WorktreeName, state.WorktreePath, state.WorktreeMainRoot, state.WorktreeCleaned, state.TeamActive, state.TeamLead, state.TeamRunning, state.TeamPending, state.TeamIncomplete, state.TeamScheduler)
}

func versionHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(version.String())
}

func exitHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Result{Exit: true}
}

package team

import (
	"context"
	"strings"

	"mewcode/internal/tool"
)

type taskCreateTool struct{ Manager *Manager }
type taskGetTool struct{ Manager *Manager }
type taskListTool struct{ Manager *Manager }
type taskUpdateTool struct{ Manager *Manager }
type messageSendTool struct{ Manager *Manager }
type memberStartTool struct{ Manager *Manager }
type memberStopTool struct{ Manager *Manager }

type taskCreateArgs struct {
	Team        string   `json:"team"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Assignee    string   `json:"assignee"`
	DependsOn   []string `json:"depends_on"`
}

type taskGetArgs struct {
	Team string `json:"team"`
	ID   string `json:"id"`
}

type taskUpdateArgs struct {
	Team     string `json:"team"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	Result   string `json:"result"`
	Assignee string `json:"assignee"`
}

type messageSendArgs struct {
	Team    string         `json:"team"`
	From    string         `json:"from"`
	To      string         `json:"to"`
	Type    string         `json:"type"`
	Content string         `json:"content"`
	Summary string         `json:"summary"`
	Payload map[string]any `json:"payload"`
}

type memberRunArgs struct {
	Team   string `json:"team"`
	Member string `json:"member"`
	Task   string `json:"task"`
}

func RegisterTools(registry *tool.Registry, manager *Manager) error {
	if registry == nil || manager == nil {
		return nil
	}
	for _, item := range []tool.Executor{
		taskCreateTool{manager},
		taskGetTool{manager},
		taskListTool{manager},
		taskUpdateTool{manager},
		messageSendTool{manager},
		memberStartTool{manager},
		memberStopTool{manager},
	} {
		if err := registry.Register(item); err != nil {
			return err
		}
	}
	return nil
}

func (t taskCreateTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolTaskCreate, Description: "创建 Team 共享任务，可设置 assignee 和 depends_on。", Schema: tool.ObjectSchema([]string{"team", "title"}, map[string]any{
		"team":        tool.StringProperty("Team name."),
		"title":       tool.StringProperty("Task title."),
		"description": tool.StringProperty("Task details."),
		"assignee":    tool.StringProperty("Optional member name."),
		"depends_on":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	})}
}

func (t taskCreateTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[taskCreateArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	task, err := t.Manager.CreateTask(args.Team, args.Title, args.Description, args.Assignee, args.DependsOn)
	if err != nil {
		return tool.Fail("team_task_create_failed", err.Error())
	}
	return tool.OK(map[string]any{"task": task})
}

func (t taskGetTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolTaskGet, Description: "查看一个 Team 共享任务。", Schema: tool.ObjectSchema([]string{"team", "id"}, map[string]any{
		"team": tool.StringProperty("Team name."),
		"id":   tool.StringProperty("Task id."),
	})}
}

func (t taskGetTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[taskGetArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	task, err := t.Manager.GetTask(args.Team, args.ID)
	if err != nil {
		return tool.Fail("team_task_get_failed", err.Error())
	}
	return tool.OK(map[string]any{"task": task})
}

func (t taskListTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolTaskList, Description: "列出 Team 共享任务。", Schema: tool.ObjectSchema([]string{"team"}, map[string]any{
		"team": tool.StringProperty("Team name."),
	})}
}

func (t taskListTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[taskGetArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	tasks, err := t.Manager.ListTasks(args.Team)
	if err != nil {
		return tool.Fail("team_task_list_failed", err.Error())
	}
	return tool.OK(map[string]any{"tasks": tasks})
}

func (t taskUpdateTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolTaskUpdate, Description: "更新 Team 共享任务状态、负责人或结果。", Schema: tool.ObjectSchema([]string{"team", "id"}, map[string]any{
		"team":     tool.StringProperty("Team name."),
		"id":       tool.StringProperty("Task id."),
		"status":   tool.StringProperty("Task status."),
		"result":   tool.StringProperty("Task result."),
		"assignee": tool.StringProperty("Optional assignee."),
	})}
}

func (t taskUpdateTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[taskUpdateArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	task, err := t.Manager.UpdateTask(args.Team, args.ID, TaskStatus(args.Status), args.Result, args.Assignee)
	if err != nil {
		return tool.Fail("team_task_update_failed", err.Error())
	}
	return tool.OK(map[string]any{"task": task})
}

func (t messageSendTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolMessageSend, Description: "向 Team 成员发送点对点消息或广播。", Schema: tool.ObjectSchema([]string{"team", "to"}, map[string]any{
		"team":    tool.StringProperty("Team name."),
		"from":    tool.StringProperty("Sender name."),
		"to":      tool.StringProperty("Member name, all, broadcast, or *."),
		"type":    tool.StringProperty("text, summary, lifecycle, or approval_reply."),
		"content": tool.StringProperty("Message content."),
		"summary": tool.StringProperty("Optional summary."),
		"payload": map[string]any{"type": "object"},
	})}
}

func (t messageSendTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[messageSendArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	if strings.TrimSpace(args.From) == "" {
		args.From = "lead"
	}
	messages, err := t.Manager.SendMessage(args.Team, args.From, args.To, MessageType(args.Type), args.Content, args.Summary, args.Payload)
	if err != nil {
		return tool.Fail("team_message_send_failed", err.Error())
	}
	return tool.OK(map[string]any{"sent": len(messages), "messages": messages})
}

func (t memberStartTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolMemberStart, Description: "启动 Team 成员执行一个派发任务。", Schema: tool.ObjectSchema([]string{"team", "member", "task"}, map[string]any{
		"team":   tool.StringProperty("Team name."),
		"member": tool.StringProperty("Member name."),
		"task":   tool.StringProperty("Task text for the member."),
	})}
}

func (t memberStartTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[memberRunArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	result := t.Manager.StartMember(ctx, args.Team, args.Member, args.Task)
	if !result.OK {
		return tool.Result{OK: false, Data: map[string]any{"status": string(result.Status)}, Error: &tool.Error{Code: "team_member_start_failed", Message: result.Error}}
	}
	return tool.OK(map[string]any{"status": string(result.Status)})
}

func (t memberStopTool) Definition() tool.Definition {
	return tool.Definition{Name: ToolMemberStop, Description: "停止一个正在运行的 Team 成员。", Schema: tool.ObjectSchema([]string{"team", "member"}, map[string]any{
		"team":   tool.StringProperty("Team name."),
		"member": tool.StringProperty("Member name."),
	})}
}

func (t memberStopTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[memberRunArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	result := t.Manager.StopMember(ctx, args.Team, args.Member)
	if !result.OK {
		return tool.Result{OK: false, Data: map[string]any{"status": string(result.Status)}, Error: &tool.Error{Code: "team_member_stop_failed", Message: result.Error}}
	}
	return tool.OK(map[string]any{"status": string(result.Status)})
}

package worker

import (
	"context"
	"encoding/json"

	"mewcode/internal/tool"
)

type RunWorkerTool struct {
	Manager *Manager
}

type runWorkerArgs struct {
	Task          string `json:"task"`
	Role          string `json:"role"`
	Background    bool   `json:"background"`
	Model         string `json:"model"`
	MaxIterations int    `json:"max_iterations"`
	Isolation     string `json:"isolation"`
}

func (r RunWorkerTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        RunWorkerToolName,
		Description: "启动一个 MewCode 子工作者执行任务；不指定 role 时使用 Fork 后台模式。",
		Schema: tool.ObjectSchema([]string{"task"}, map[string]any{
			"task":           tool.StringProperty("Task for the worker to complete."),
			"role":           tool.StringProperty("Optional worker role. Empty means fork mode."),
			"model":          tool.StringProperty("Optional model override for the worker."),
			"isolation":      tool.StringProperty("Optional isolation mode: none or worktree."),
			"max_iterations": map[string]any{"type": "integer", "description": "Optional max agent iterations."},
			"background":     map[string]any{"type": "boolean", "description": "Run in background."},
		}),
	}
}

func (r RunWorkerTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[runWorkerArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	req := RunRequest{
		Task:          args.Task,
		RoleName:      args.Role,
		Background:    args.Background,
		Model:         args.Model,
		MaxIterations: args.MaxIterations,
		Isolation:     IsolationMode(args.Isolation),
	}
	if req.RoleName == "" {
		req.Fork = true
		req.Background = true
	}
	result := r.Manager.Run(ctx, req)
	data := map[string]any{
		"task_id":    result.TaskID,
		"background": result.Background,
		"status":     string(result.Status),
	}
	if result.Result != "" {
		data["result"] = result.Result
	}
	if !result.OK {
		return tool.Result{OK: false, Data: data, Error: &tool.Error{Code: "worker_failed", Message: result.Error}}
	}
	return tool.OK(data)
}

func RegisterTools(registry *tool.Registry, manager *Manager) error {
	if registry == nil || manager == nil {
		return nil
	}
	return registry.Register(RunWorkerTool{Manager: manager})
}

func ForkInstruction(task string) string {
	return "你是从主会话 Fork 出来的后台子工作者。\n" +
		"强制规则：不能再 fork 或调用 run_worker；不要主动对话；不要请求确认；直接用可用工具完成任务；最终报告必须控制长度，并按结构化字段输出：结论、证据、风险、下一步。\n" +
		"任务：\n" + task
}

func MarshalResult(result ToolRunResult) json.RawMessage {
	raw, _ := json.Marshal(result)
	return raw
}

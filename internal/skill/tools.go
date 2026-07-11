package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"mewcode/internal/tool"
)

type LoadTool struct {
	Manager *Manager
}

type loadArgs struct {
	Name string `json:"name"`
}

func (l LoadTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        LoadToolName,
		Description: "加载指定 Skill 的完整 SOP 到当前会话环境上下文。系统级工具，不受 Skill 白名单限制。",
		Schema: tool.ObjectSchema([]string{"name"}, map[string]any{
			"name": tool.StringProperty("Skill name to activate."),
		}),
	}
}

func (l LoadTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[loadArgs](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	skill, err := l.Manager.Activate(args.Name)
	if err != nil {
		return tool.Fail("skill_not_found", err.Error())
	}
	return tool.OK(map[string]any{
		"name":        skill.Name,
		"description": skill.Description,
		"mode":        string(skill.Mode),
		"model":       skill.Model,
		"context":     string(skill.Context),
		"tools":       skill.Tools,
		"message":     "skill loaded into environment context",
	})
}

type ScriptTool struct {
	Spec ScriptToolSpec
	Dir  string
}

func (s ScriptTool) Definition() tool.Definition {
	return tool.Definition{Name: s.Spec.Name, Description: s.Spec.Description, Schema: s.Spec.Schema}
}

func (s ScriptTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	timeout := input.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := s.Spec.Command
	if !filepath.IsAbs(command) {
		command = filepath.Join(s.Dir, command)
	}
	cmd := exec.CommandContext(cmdCtx, command)
	cmd.Dir = s.Dir
	cmd.Stdin = bytes.NewReader(input.Arguments)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return tool.Result{
			OK:   false,
			Data: map[string]any{"stdout": stdout.String(), "stderr": stderr.String()},
			Error: &tool.Error{
				Code:    "script_failed",
				Message: fmt.Sprintf("%v", err),
			},
		}
	}
	var data map[string]any
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
			return tool.Fail("invalid_script_output", err.Error())
		}
	}
	return tool.OK(data)
}

func RegisterTools(registry *tool.Registry, manager *Manager) error {
	if registry == nil || manager == nil {
		return nil
	}
	if err := registry.Register(LoadTool{Manager: manager}); err != nil {
		return err
	}
	for _, skill := range manager.List() {
		for _, spec := range skill.ScriptTools {
			if err := registry.Register(ScriptTool{Spec: spec, Dir: skill.Dir}); err != nil {
				return err
			}
		}
	}
	return nil
}

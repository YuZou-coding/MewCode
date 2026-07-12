package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mewcode/internal/hooks"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

type toolExecutionResult struct {
	Call   provider.ToolCall
	Result tool.Result
}

func IsReadOnlyTool(name string) bool {
	switch name {
	case "read_file", "find_files", "search_code":
		return true
	default:
		return false
	}
}

func (a *Agent) executeToolBatch(ctx context.Context, calls []provider.ToolCall, events chan<- Event) []toolExecutionResult {
	results := make([]toolExecutionResult, len(calls))
	readIndexes := make([]int, 0, len(calls))
	writeIndexes := make([]int, 0, len(calls))
	for index, call := range calls {
		if IsReadOnlyTool(call.Name) {
			readIndexes = append(readIndexes, index)
		} else {
			writeIndexes = append(writeIndexes, index)
		}
	}

	done := make(chan toolExecutionResult, len(readIndexes))
	for _, index := range readIndexes {
		call := calls[index]
		go func() {
			done <- toolExecutionResult{Call: call, Result: a.executeSingleTool(ctx, call, events)}
		}()
	}
	for _, index := range writeIndexes {
		call := calls[index]
		results[index] = toolExecutionResult{Call: call, Result: a.executeSingleTool(ctx, call, events)}
		sendEvent(ctx, events, Event{Kind: EventToolResult, ToolCallID: call.ID, ToolName: call.Name, Result: &results[index].Result})
	}
	for range readIndexes {
		result := <-done
		for index, call := range calls {
			if call.ID == result.Call.ID {
				results[index] = result
				break
			}
		}
		sendEvent(ctx, events, Event{Kind: EventToolResult, ToolCallID: result.Call.ID, ToolName: result.Call.Name, Result: &result.Result})
	}
	return results
}

func (a *Agent) executeSingleTool(ctx context.Context, call provider.ToolCall, events chan<- Event) tool.Result {
	if a.PlanOnly && !IsReadOnlyTool(call.Name) {
		return tool.Fail("plan_only_blocked", "当前处于 plan-only 模式，请关闭 plan-only 后再执行写入、编辑或命令工具")
	}
	hookCtx := hookContextForTool(call)
	if a.HookEngine != nil {
		block := a.HookEngine.BeforeTool(ctx, hookCtx)
		if block.Blocked {
			return tool.Fail("hook_blocked", block.Reason)
		}
	}
	if a.PreToolHook != nil {
		allowed, message := a.PreToolHook(ctx, call)
		if !allowed {
			if message == "" {
				message = "tool blocked by pre hook"
			}
			return tool.Fail("hook_blocked", message)
		}
	}
	if decision := a.checkPermission(ctx, call, events); decision.Effect == permissions.EffectDeny {
		code := decision.Code
		if code == "" {
			code = "permission_denied"
		}
		return tool.Fail(code, decision.Reason)
	}
	if a.Registry == nil {
		return tool.Fail("tool_registry_missing", "tool registry is not configured")
	}
	executor, err := a.Registry.Get(call.Name)
	if err != nil {
		return tool.Fail("tool_not_found", err.Error())
	}

	timeout := a.ToolTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan tool.Result, 1)
	go func() {
		resultCh <- executor.Execute(toolCtx, tool.Input{
			Arguments: call.Arguments,
			Timeout:   timeout,
			Confirm: func(ctx context.Context, command string) bool {
				if a.PermissionChecker != nil {
					return true
				}
				if a.ConfirmCommand == nil {
					return false
				}
				return a.ConfirmCommand(ctx, command)
			},
		})
	}()

	var result tool.Result
	select {
	case <-ctx.Done():
		result = tool.Fail("cancelled", ctx.Err().Error())
	case <-toolCtx.Done():
		if ctx.Err() != nil {
			result = tool.Fail("cancelled", ctx.Err().Error())
		} else {
			result = tool.Fail("tool_timeout", fmt.Sprintf("tool timed out after %s", timeout))
		}
	case result = <-resultCh:
	}
	if a.PostToolHook != nil {
		_ = a.PostToolHook(ctx, call, result)
	}
	if a.HookEngine != nil {
		hookCtx.Event = hooks.EventToolAfterExecute
		hookCtx.ToolResult = string(mustMarshal(result))
		_ = a.HookEngine.Fire(ctx, hookCtx)
	}
	return result
}

func hookContextForTool(call provider.ToolCall) hooks.Context {
	var args map[string]any
	_ = json.Unmarshal(call.Arguments, &args)
	hookCtx := hooks.Context{Event: hooks.EventToolBeforeExecute, ToolName: call.Name, ToolArgs: args}
	for _, key := range []string{"path", "file_path", "target", "root", "pattern"} {
		if value, ok := args[key].(string); ok && hookCtx.Path == "" {
			hookCtx.Path = value
		}
	}
	if value, ok := args["command"].(string); ok {
		hookCtx.Command = value
	}
	return hookCtx
}

func requiresToolConfirmation(name string) bool {
	return name == "write_file" || name == "edit_file"
}

func (a *Agent) checkPermission(ctx context.Context, call provider.ToolCall, events chan<- Event) permissions.Decision {
	if a.PermissionChecker == nil {
		if requiresToolConfirmation(call.Name) && a.ConfirmTool != nil && !a.ConfirmTool(ctx, call) {
			return permissions.Deny("permission_denied", "tool denied by user")
		}
		return permissions.Allow("no permission checker configured")
	}
	request := permissions.Request{Tool: call.Name, Arguments: call.Arguments, Root: a.PermissionChecker.Root}
	decision := a.PermissionChecker.Check(request)
	switch decision.Effect {
	case permissions.EffectAllow:
		return decision
	case permissions.EffectDeny:
		if decision.Code == "" {
			decision.Code = "permission_denied"
		}
		return decision
	case permissions.EffectAsk:
		if a.PermissionPrompt == nil {
			return permissions.Deny("permission_denied", "permission prompt is not configured")
		}
		sendEvent(ctx, events, permissionEvent(call, decision))
		choice := a.PermissionPrompt(ctx, request, decision)
		switch choice {
		case permissions.HITLAllowOnce:
			return permissions.Allow("allowed once by user")
		case permissions.HITLAllowSession:
			if decision.Mode == permissions.ModeStrict {
				return permissions.Deny("permission_denied", "strict mode only allows once")
			}
			a.PermissionChecker.AddSessionRule(permissions.RuleForRequest(permissions.EffectAllow, permissions.SourceSession, request))
			return permissions.Allow("allowed for this session by user")
		case permissions.HITLAllowAlways:
			if decision.Mode == permissions.ModeStrict {
				return permissions.Deny("permission_denied", "strict mode only allows once")
			}
			rule := permissions.RuleForRequest(permissions.EffectAllow, permissions.SourceUser, request)
			if err := permissions.AppendUserRule(rule); err != nil {
				return permissions.Deny("permission_denied", err.Error())
			}
			return permissions.Allow("allowed always by user")
		default:
			return permissions.Deny("permission_denied", "permission denied by user")
		}
	default:
		return permissions.Deny("permission_denied", "unknown permission decision")
	}
}

func permissionEvent(call provider.ToolCall, decision permissions.Decision) Event {
	event := Event{
		Kind:             EventPermissionRequest,
		ToolCallID:       call.ID,
		ToolName:         call.Name,
		PermissionReason: decision.Reason,
	}
	var args map[string]any
	_ = json.Unmarshal(call.Arguments, &args)
	if value, ok := args["path"].(string); ok {
		event.PermissionPath = value
	}
	if value, ok := args["root"].(string); ok && event.PermissionPath == "" {
		event.PermissionPath = value
	}
	if value, ok := args["command"].(string); ok {
		event.PermissionCommand = value
	}
	return event
}

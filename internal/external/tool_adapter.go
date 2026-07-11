package external

import (
	"context"
	"encoding/json"
	"fmt"

	"mewcode/internal/tool"
)

type RemoteExecutor struct {
	ServerName string
	Remote     RemoteTool
	LocalName  string
	Manager    *Manager
}

func (r RemoteExecutor) Definition() tool.Definition {
	return tool.Definition{
		Name:        r.LocalName,
		Description: fmt.Sprintf("[%s] %s", r.ServerName, r.Remote.Description),
		Schema:      tool.Schema(r.Remote.InputSchema),
	}
}

func (r RemoteExecutor) Execute(ctx context.Context, input tool.Input) tool.Result {
	client, err := r.Manager.Client(ctx, r.ServerName)
	if err != nil {
		return tool.Fail("external_tool_failed", err.Error())
	}
	result, err := client.CallTool(ctx, r.Remote.Name, input.Arguments)
	if err != nil {
		return tool.Fail("external_tool_failed", err.Error())
	}
	data := map[string]any{}
	if len(result.Content) > 0 {
		data["content"] = result.Content
		var text string
		for _, block := range result.Content {
			if block.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += block.Text
			}
		}
		if text != "" {
			data["text"] = text
		}
	}
	for key, value := range result.Data {
		data[key] = value
	}
	if result.IsError {
		return tool.Result{OK: false, Data: data, Error: &tool.Error{Code: "external_tool_failed", Message: "remote tool returned error"}}
	}
	return tool.OK(data)
}

func ToolCallArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

package external

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"mewcode/internal/tool"
)

const DiscoverToolName = "discover_mcp_tools"

type DiscoveryTool struct {
	Registry *tool.Registry
	Manager  *Manager
}

func (d DiscoveryTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        DiscoverToolName,
		Description: "Connect to configured MCP servers and add their remote tools. Pass a server name to discover one server; omit it to discover all configured servers.",
		Schema: tool.ObjectSchema(nil, map[string]any{
			"server": tool.StringProperty("Configured MCP server name; omit to discover all servers"),
		}),
	}
}

func (d DiscoveryTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	var args struct {
		Server string `json:"server"`
	}
	if len(input.Arguments) > 0 {
		if err := json.Unmarshal(input.Arguments, &args); err != nil {
			return tool.Fail("invalid_arguments", err.Error())
		}
	}
	names := d.Manager.ServerNames()
	if args.Server != "" {
		names = []string{args.Server}
	}
	discovered := map[string]any{}
	errorsByServer := map[string]string{}
	for _, name := range names {
		tools, err := registerServerTools(ctx, d.Registry, d.Manager, name)
		if err != nil {
			errorsByServer[name] = err.Error()
			continue
		}
		discovered[name] = len(tools)
	}
	data := map[string]any{"discovered": discovered}
	if len(errorsByServer) > 0 {
		data["errors"] = errorsByServer
	}
	return tool.OK(data)
}

func RegisterDiscoveryTool(registry *tool.Registry, manager *Manager) error {
	return registry.Register(DiscoveryTool{Registry: registry, Manager: manager})
}

func RegisterDiscoveredTools(ctx context.Context, registry *tool.Registry, manager *Manager) map[string]error {
	discovered, errs := manager.Discover(ctx)
	used := map[string]bool{}
	for _, def := range registry.Definitions() {
		used[def.Name] = true
	}
	for server, tools := range discovered {
		for _, remote := range tools {
			local := uniqueToolName(server, remote.Name, used)
			used[local] = true
			if err := registry.Register(RemoteExecutor{ServerName: server, Remote: remote, LocalName: local, Manager: manager}); err != nil {
				errs[server+"/"+remote.Name] = err
			}
		}
	}
	return errs
}

func registerServerTools(ctx context.Context, registry *tool.Registry, manager *Manager, server string) ([]RemoteTool, error) {
	client, err := manager.Client(ctx, server)
	if err != nil {
		return nil, err
	}
	remotes, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, def := range registry.Definitions() {
		used[def.Name] = true
	}
	for _, remote := range remotes {
		if remoteAlreadyRegistered(registry, server, remote.Name) {
			continue
		}
		local := uniqueToolName(server, remote.Name, used)
		used[local] = true
		if err := registry.Register(RemoteExecutor{ServerName: server, Remote: remote, LocalName: local, Manager: manager}); err != nil {
			return nil, err
		}
	}
	return remotes, nil
}

func remoteAlreadyRegistered(registry *tool.Registry, server string, remote string) bool {
	for _, def := range registry.Definitions() {
		executor, err := registry.Get(def.Name)
		if err != nil {
			continue
		}
		remoteExecutor, ok := executor.(RemoteExecutor)
		if ok && remoteExecutor.ServerName == server && remoteExecutor.Remote.Name == remote {
			return true
		}
	}
	return false
}

func uniqueToolName(server string, remote string, used map[string]bool) string {
	base := sanitizeToolName("external_" + server + "_" + remote)
	if base == "" {
		base = "external_tool"
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

var invalidToolNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func sanitizeToolName(name string) string {
	name = invalidToolNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	return name
}

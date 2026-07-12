package external

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"mewcode/internal/tool"
)

const ToolSearchName = "tool_search"

type ToolSearch struct {
	manager    *Manager
	registry   *tool.Registry
	mu         sync.Mutex
	registered map[string]bool
}

func NewToolSearch(manager *Manager, registry *tool.Registry) *ToolSearch {
	return &ToolSearch{manager: manager, registry: registry, registered: map[string]bool{}}
}

func (s *ToolSearch) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolSearchName,
		Description: "Search configured MCP tools and load matching tools for later use. Use select:<external_server_tool> to load one exact tool.",
		Schema: tool.ObjectSchema([]string{"query"}, map[string]any{
			"query": tool.StringProperty("MCP server name, tool name, capability keyword, or select:<external_server_tool>."),
		}),
	}
}

func (s *ToolSearch) Execute(ctx context.Context, input tool.Input) tool.Result {
	args, err := tool.DecodeArgs[struct {
		Query string `json:"query"`
	}](input.Arguments)
	if err != nil {
		return tool.Fail("invalid_arguments", err.Error())
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return tool.Fail("invalid_arguments", "query is required")
	}
	if s.manager == nil || s.registry == nil {
		return tool.Fail("tool_search_unavailable", "MCP tool search is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	selectName := strings.TrimSpace(strings.TrimPrefix(query, "select:"))
	exact := strings.HasPrefix(query, "select:")
	servers := s.candidateServers(query, exact, selectName)
	if exact && len(servers) == 0 {
		return tool.Fail("tool_not_found", "MCP tool selection does not match a configured server")
	}
	tools := make([]map[string]string, 0)
	errors := map[string]string{}
	used := map[string]bool{}
	for _, def := range s.registry.Definitions() {
		used[def.Name] = true
	}

	for _, server := range servers {
		client, err := s.manager.Client(ctx, server)
		if err != nil {
			errors[server] = err.Error()
			continue
		}
		remoteTools, err := client.ListTools(ctx)
		if err != nil {
			errors[server] = err.Error()
			continue
		}
		for _, remote := range remoteTools {
			local := "external_" + sanitizeToolName(server) + "_" + sanitizeToolName(remote.Name)
			if !matchesToolSearch(query, server, remote, local, exact, selectName) {
				continue
			}
			key := server + "\x00" + remote.Name
			if !s.registered[key] {
				local = uniqueToolName(server, remote.Name, used)
				if err := s.registry.Register(RemoteExecutor{ServerName: server, Remote: remote, LocalName: local, Manager: s.manager}); err != nil {
					errors[server+"/"+remote.Name] = err.Error()
					continue
				}
				used[local] = true
				s.registered[key] = true
			}
			tools = append(tools, map[string]string{"server": server, "name": local, "description": remote.Description})
		}
	}

	return tool.OK(map[string]any{"tools": tools, "errors": errors})
}

func (s *ToolSearch) candidateServers(query string, exact bool, selectName string) []string {
	names := s.manager.ServerNames()
	if exact {
		for _, name := range names {
			prefix := "external_" + sanitizeToolName(name) + "_"
			if strings.HasPrefix(selectName, prefix) {
				return []string{name}
			}
		}
		return nil
	}
	query = strings.ToLower(query)
	matched := make([]string, 0, len(names))
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), query) {
			matched = append(matched, name)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return names
}

func matchesToolSearch(query, server string, remote RemoteTool, local string, exact bool, selectName string) bool {
	if exact {
		return local == selectName
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	return strings.Contains(strings.ToLower(server), needle) || strings.Contains(strings.ToLower(remote.Name), needle) || strings.Contains(strings.ToLower(remote.Description), needle)
}

var _ tool.Executor = (*ToolSearch)(nil)

func (s *ToolSearch) String() string {
	return fmt.Sprintf("%s", ToolSearchName)
}

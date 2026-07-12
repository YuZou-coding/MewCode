package external

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"mewcode/internal/tool"
)

const ToolSearchName = "tool_search"

type ToolSearch struct {
	manager    *Manager
	registry   *tool.Registry
	mu         sync.Mutex
	registered map[string]bool
}

type toolSearchMatch struct {
	Server       string
	Name         string
	Description  string
	Capabilities []string
	Matched      []string
	Score        int
	Recommended  bool
}

func NewToolSearch(manager *Manager, registry *tool.Registry) *ToolSearch {
	return &ToolSearch{manager: manager, registry: registry, registered: map[string]bool{}}
}

func (s *ToolSearch) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolSearchName,
		Description: "Discover configured MCP tools for external capabilities such as current documentation, SaaS/API access, browser state, databases, issue trackers, cloud services, and knowledge bases. Returns ranked matches with capabilities, matched reasons, scores, and a recommended candidate. Use select:<external_server_tool> to load one exact tool.",
		Schema: tool.ObjectSchema([]string{"query"}, map[string]any{
			"query": tool.StringProperty("MCP server name, tool name, capability keyword, natural-language task, or select:<external_server_tool>."),
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
	matches := make([]toolSearchMatch, 0)
	errors := map[string]string{}
	used := map[string]bool{}
	for _, def := range s.registry.Definitions() {
		used[def.Name] = true
	}

	for _, server := range servers {
		cfg := s.serverConfig(server)
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
			match := scoreToolSearch(query, cfg, remote, local, exact, selectName)
			if match.Score == 0 {
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
			match.Server = server
			match.Name = local
			match.Description = remote.Description
			match.Capabilities = append([]string(nil), cfg.Capabilities...)
			matches = append(matches, match)
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Server != matches[j].Server {
			return matches[i].Server < matches[j].Server
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > 0 {
		matches[0].Recommended = true
	}
	tools := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		tools = append(tools, map[string]any{
			"server":       match.Server,
			"name":         match.Name,
			"description":  match.Description,
			"capabilities": match.Capabilities,
			"matched":      match.Matched,
			"score":        match.Score,
			"recommended":  match.Recommended,
		})
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
	metadataMatched := make([]string, 0, len(names))
	for _, cfg := range s.manager.configs {
		if scoreServerConfig(query, cfg).Score > 0 {
			metadataMatched = append(metadataMatched, cfg.Name)
		}
	}
	if len(metadataMatched) > 0 {
		return metadataMatched
	}
	return names
}

func (s *ToolSearch) serverConfig(name string) ServerConfig {
	for _, cfg := range s.manager.configs {
		if cfg.Name == name {
			return cfg
		}
	}
	return ServerConfig{Name: name}
}

func matchesToolSearch(query, server string, remote RemoteTool, local string, exact bool, selectName string) bool {
	return scoreToolSearch(query, ServerConfig{Name: server}, remote, local, exact, selectName).Score > 0
}

func scoreToolSearch(query string, cfg ServerConfig, remote RemoteTool, local string, exact bool, selectName string) toolSearchMatch {
	if exact {
		if local != selectName {
			return toolSearchMatch{}
		}
		return toolSearchMatch{Score: 1000, Matched: []string{"exact tool name"}}
	}
	match := scoreServerConfig(query, cfg)
	addFieldScore(&match, query, "tool name", remote.Name, 25)
	addFieldScore(&match, query, "tool description", remote.Description, 15)
	addFieldScore(&match, query, "local tool name", local, 20)
	return match
}

func scoreServerConfig(query string, cfg ServerConfig) toolSearchMatch {
	match := toolSearchMatch{Capabilities: append([]string(nil), cfg.Capabilities...)}
	addFieldScore(&match, query, "server name", cfg.Name, 30)
	addFieldScore(&match, query, "server description", cfg.Description, 15)
	addListScore(&match, query, "server capability", cfg.Capabilities, 25)
	addListScore(&match, query, "server keyword", cfg.Keywords, 25)
	addListScore(&match, query, "server example", cfg.Examples, 10)
	return match
}

func addListScore(match *toolSearchMatch, query, label string, values []string, weight int) {
	for _, value := range values {
		addFieldScore(match, query, label, value, weight)
	}
}

func addFieldScore(match *toolSearchMatch, query, label, value string, weight int) {
	query = strings.ToLower(strings.TrimSpace(query))
	value = strings.ToLower(strings.TrimSpace(value))
	if query == "" || value == "" {
		return
	}
	if strings.Contains(value, query) || strings.Contains(query, value) {
		match.Score += weight * 2
		match.Matched = appendUnique(match.Matched, label+": "+value)
		return
	}
	for _, token := range searchTokens(query) {
		if strings.Contains(value, token) {
			match.Score += weight
			match.Matched = appendUnique(match.Matched, label+": "+token)
		}
	}
}

func searchTokens(value string) []string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len([]rune(part)) < 3 {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

var _ tool.Executor = (*ToolSearch)(nil)

func (s *ToolSearch) String() string {
	return fmt.Sprintf("%s", ToolSearchName)
}

package external

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"mewcode/internal/tool"
)

func RegisterDiscoveredTools(ctx context.Context, registry *tool.Registry, manager *Manager) map[string]error {
	discovered, errs := manager.Discover(ctx)
	for server, tools := range discovered {
		for _, remote := range tools {
			if err := registerRemoteTool(registry, manager, server, remote); err != nil {
				errs[server+"/"+remote.Name] = err
			}
		}
	}
	return errs
}

func registerRemoteTool(registry *tool.Registry, manager *Manager, server string, remote RemoteTool) error {
	if remoteAlreadyRegistered(registry, server, remote.Name) {
		return nil
	}
	used := map[string]bool{}
	for _, def := range registry.Definitions() {
		used[def.Name] = true
	}
	local := uniqueToolName(server, remote.Name, used)
	return registry.Register(RemoteExecutor{ServerName: server, Remote: remote, LocalName: local, Manager: manager})
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

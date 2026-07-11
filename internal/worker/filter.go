package worker

import "mewcode/internal/tool"

func FilterDefinitions(defs []tool.Definition, role Role, background bool) []tool.Definition {
	allowed := listSet(role.ToolsAllow)
	denied := listSet(role.ToolsDeny)
	backgroundAllowed := listSet(role.BackgroundTools)
	filtered := make([]tool.Definition, 0, len(defs))
	for _, def := range defs {
		if def.Name == RunWorkerToolName {
			continue
		}
		if len(allowed) > 0 && !allowed[def.Name] {
			continue
		}
		if denied[def.Name] {
			continue
		}
		if background && len(backgroundAllowed) > 0 && !backgroundAllowed[def.Name] {
			continue
		}
		filtered = append(filtered, def)
	}
	return filtered
}

func listSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, item := range items {
		set[item] = true
	}
	return set
}

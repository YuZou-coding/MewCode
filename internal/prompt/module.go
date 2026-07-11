package prompt

import (
	"sort"
	"strings"
)

type Module struct {
	ID       string
	Priority int
	Content  string
}

func Build(modules []Module) string {
	ordered := make([]Module, len(modules))
	copy(ordered, modules)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority == ordered[j].Priority {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Priority < ordered[j].Priority
	})

	parts := make([]string, 0, len(ordered))
	for _, module := range ordered {
		content := strings.TrimSpace(module.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

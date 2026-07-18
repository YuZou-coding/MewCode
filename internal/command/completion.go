package command

import (
	"sort"
	"strings"
)

type Completion struct {
	Replacement string
	Candidates  []string
}

type PanelItem struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	ArgHint     string
	Type        Type
	Group       string
}

func (r *Registry) Complete(input string) Completion {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return Completion{}
	}
	body := strings.TrimPrefix(strings.TrimLeft(input, " \t"), "/")
	name, rest, hasSpace := strings.Cut(body, " ")
	name = normalizeName(name)
	if hasSpace {
		cmd, ok := r.Lookup(name)
		if !ok || cmd.Hidden {
			return Completion{}
		}
		prefix := strings.TrimSpace(rest)
		var candidates []string
		for _, sub := range cmd.Subcommands {
			if strings.HasPrefix(sub, prefix) {
				candidates = append(candidates, sub)
			}
		}
		return completeFromCandidates("/"+cmd.Name+" ", candidates)
	}
	var candidates []string
	for _, cmd := range r.Commands(false) {
		if strings.HasPrefix(cmd.Name, name) {
			candidates = append(candidates, cmd.Name)
			continue
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, name) {
				candidates = append(candidates, cmd.Name)
				break
			}
		}
	}
	return completeFromCandidates("/", uniqueSorted(candidates))
}

func (r *Registry) PanelItems(input string, limit int) []PanelItem {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	body := strings.TrimPrefix(strings.TrimLeft(input, " \t"), "/")
	commandName, subQuery, hasSpace := strings.Cut(body, " ")
	if hasSpace {
		cmd, ok := r.Lookup(commandName)
		if !ok || cmd.Hidden || len(cmd.Subcommands) == 0 {
			return nil
		}
		subQuery = strings.TrimSpace(subQuery)
		if !strings.HasSuffix(body, " ") {
			for _, subcommand := range cmd.Subcommands {
				if subcommand == subQuery {
					return nil
				}
			}
		}
		items := make([]PanelItem, 0, len(cmd.Subcommands))
		for _, subcommand := range cmd.Subcommands {
			if !strings.HasPrefix(subcommand, subQuery) {
				continue
			}
			items = append(items, PanelItem{
				Name:        cmd.Name + " " + subcommand,
				Description: cmd.Description,
				Usage:       cmd.Usage,
				Type:        cmd.Type,
				Group:       commandGroup(cmd.Name),
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		if limit > 0 && len(items) > limit {
			return items[:limit]
		}
		return items
	}
	query := normalizeName(commandName)
	common := map[string]int{
		"help": 1, "plan": 2, "compact": 3, "sessions": 4,
		"resume": 5, "status": 6, "clear": 7, "exit": 8,
	}
	var items []PanelItem
	for _, cmd := range r.Commands(false) {
		if query == "" {
			if _, ok := common[cmd.Name]; !ok {
				continue
			}
		} else if !commandMatchesPanelQuery(cmd, query) {
			continue
		}
		items = append(items, PanelItem{
			Name:        cmd.Name,
			Aliases:     append([]string(nil), cmd.Aliases...),
			Description: cmd.Description,
			Usage:       cmd.Usage,
			ArgHint:     cmd.ArgHint,
			Type:        cmd.Type,
			Group:       commandGroup(cmd.Name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if query == "" {
			return common[items[i].Name] < common[items[j].Name]
		}
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		return items[i].Name < items[j].Name
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func commandMatchesPanelQuery(cmd Command, query string) bool {
	if strings.HasPrefix(cmd.Name, query) {
		return true
	}
	for _, alias := range cmd.Aliases {
		if strings.HasPrefix(alias, query) {
			return true
		}
	}
	return false
}

func commandGroup(name string) string {
	switch name {
	case "sessions", "resume", "clear":
		return "Session"
	case "plan", "do":
		return "Mode"
	case "compact", "status", "help":
		return "Context"
	case "notes", "permissions":
		return "Memory & Permissions"
	case "skills", "workers", "worktrees", "teams":
		return "Agent Extensions"
	case "exit":
		return "Exit"
	default:
		return "Other"
	}
}

func completeFromCandidates(prefix string, candidates []string) Completion {
	candidates = uniqueSorted(candidates)
	if len(candidates) == 1 {
		return Completion{Replacement: prefix + candidates[0]}
	}
	return Completion{Candidates: candidates}
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

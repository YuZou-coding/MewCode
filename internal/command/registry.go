package command

import (
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	commands map[string]Command
	aliases  map[string]string
}

func NewRegistry() *Registry {
	return &Registry{commands: map[string]Command{}, aliases: map[string]string{}}
}

func (r *Registry) Register(cmd Command) error {
	if r.commands == nil {
		r.commands = map[string]Command{}
	}
	if r.aliases == nil {
		r.aliases = map[string]string{}
	}
	cmd.Name = normalizeName(cmd.Name)
	if cmd.Name == "" {
		return fmt.Errorf("command name is required")
	}
	if _, exists := r.commands[cmd.Name]; exists {
		return fmt.Errorf("command name conflict: %s", cmd.Name)
	}
	if owner, exists := r.aliases[cmd.Name]; exists {
		return fmt.Errorf("command name %s conflicts with alias of %s", cmd.Name, owner)
	}
	seen := map[string]bool{}
	for i, alias := range cmd.Aliases {
		alias = normalizeName(alias)
		if alias == "" {
			return fmt.Errorf("empty alias for %s", cmd.Name)
		}
		if alias == cmd.Name {
			return fmt.Errorf("alias %s duplicates command name", alias)
		}
		if seen[alias] {
			return fmt.Errorf("duplicate alias %s", alias)
		}
		if _, exists := r.commands[alias]; exists {
			return fmt.Errorf("alias %s conflicts with command name", alias)
		}
		if owner, exists := r.aliases[alias]; exists {
			return fmt.Errorf("alias %s conflicts with alias of %s", alias, owner)
		}
		seen[alias] = true
		cmd.Aliases[i] = alias
	}
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.aliases[alias] = cmd.Name
	}
	return nil
}

func (r *Registry) Lookup(name string) (Command, bool) {
	name = normalizeName(name)
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	cmd, ok := r.commands[name]
	return cmd, ok
}

func (r *Registry) Commands(includeHidden bool) []Command {
	items := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.Hidden && !includeHidden {
			continue
		}
		items = append(items, cmd)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (r *Registry) Help(name string) string {
	name = normalizeName(name)
	if name != "" {
		cmd, ok := r.Lookup(name)
		if !ok || cmd.Hidden {
			return "unknown command /" + name + "; type /help"
		}
		aliases := ""
		if len(cmd.Aliases) > 0 {
			aliases = " aliases: /" + strings.Join(cmd.Aliases, ", /")
		}
		return fmt.Sprintf("/%s - %s\nusage: %s%s", cmd.Name, cmd.Description, cmd.Usage, aliases)
	}
	var b strings.Builder
	b.WriteString("MewCode commands:\n")
	for _, cmd := range r.Commands(false) {
		b.WriteString(fmt.Sprintf("/%-12s %s\n", cmd.Name, cmd.Description))
	}
	b.WriteString("type /help <command> for usage")
	return strings.TrimRight(b.String(), "\n")
}

package skill

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/prompt"
	"mewcode/internal/tool"
)

const (
	ProjectDir    = ".mewcode/skills"
	UserDirName   = "skills"
	EntryFileName = "SKILL.md"
	LoadToolName  = "load_skill"
	RecentCount   = 6
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

type Mode string

const (
	ModeShared   Mode = "shared"
	ModeIsolated Mode = "isolated"
)

type ContextStrategy string

const (
	ContextFullSummary ContextStrategy = "full_summary"
	ContextRecent      ContextStrategy = "recent"
	ContextEmpty       ContextStrategy = "empty"
)

type Skill struct {
	Name        string
	Description string
	Tools       []string
	Mode        Mode
	Model       string
	Context     ContextStrategy
	Body        string
	Source      Source
	Path        string
	Dir         string
	ScriptTools []ScriptToolSpec
}

func (m *Manager) RefreshSkill(name string) (Skill, error) {
	skill, ok := m.Get(name)
	if !ok {
		return Skill{}, fmt.Errorf("skill not found: %s", normalizeName(name))
	}
	if skill.Source == SourceBuiltin || skill.Path == "" {
		return skill, nil
	}
	raw, err := os.ReadFile(skill.Path)
	if err != nil {
		return Skill{}, err
	}
	refreshed, err := ParseMarkdown(skill.Path, skill.Source, raw)
	if err != nil {
		return Skill{}, err
	}
	m.Skills[refreshed.Name] = refreshed
	if _, active := m.Active[refreshed.Name]; active {
		m.Active[refreshed.Name] = refreshed
	}
	return refreshed, nil
}

type ScriptToolSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Schema      tool.Schema `json:"schema"`
	Command     string      `json:"command"`
}

type LoadResult struct {
	Skills   map[string]Skill
	Warnings []string
}

type Manager struct {
	ProjectRoot string
	HomeDir     string
	Skills      map[string]Skill
	Warnings    []string
	Active      map[string]Skill
}

func NewManager(projectRoot, homeDir string, loaded LoadResult) *Manager {
	return &Manager{
		ProjectRoot: projectRoot,
		HomeDir:     homeDir,
		Skills:      loaded.Skills,
		Warnings:    loaded.Warnings,
		Active:      map[string]Skill{},
	}
}

func (m *Manager) List() []Skill {
	if m == nil {
		return nil
	}
	items := make([]Skill, 0, len(m.Skills))
	for _, skill := range m.Skills {
		items = append(items, skill)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (m *Manager) Get(name string) (Skill, bool) {
	if m == nil {
		return Skill{}, false
	}
	skill, ok := m.Skills[normalizeName(name)]
	return skill, ok
}

func (m *Manager) Activate(name string) (Skill, error) {
	if m == nil {
		return Skill{}, fmt.Errorf("skill manager is not configured")
	}
	skill, ok := m.Get(name)
	if !ok {
		return Skill{}, fmt.Errorf("skill not found: %s", normalizeName(name))
	}
	if m.Active == nil {
		m.Active = map[string]Skill{}
	}
	m.Active[skill.Name] = skill
	return skill, nil
}

func (m *Manager) ClearActive() {
	if m != nil {
		m.Active = map[string]Skill{}
	}
}

func (m *Manager) ActiveCount() int {
	if m == nil {
		return 0
	}
	return len(m.Active)
}

func (m *Manager) SummaryMessage() chat.Message {
	var b strings.Builder
	b.WriteString("<mewcode-skills>\n")
	b.WriteString("可用 Skill 摘要。需要完整 SOP 时调用 load_skill，不要猜测未加载的 SOP。\n")
	for _, skill := range m.List() {
		fmt.Fprintf(&b, "- %s: %s\n", skill.Name, skill.Description)
	}
	b.WriteString("</mewcode-skills>")
	return prompt.InternalInstruction(b.String())
}

func (m *Manager) ActiveMessages() []chat.Message {
	if m == nil || len(m.Active) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.Active))
	for name := range m.Active {
		names = append(names, name)
	}
	sort.Strings(names)
	messages := make([]chat.Message, 0, len(names))
	for _, name := range names {
		skill := m.Active[name]
		content := fmt.Sprintf("<mewcode-active-skill name=%q mode=%q model=%q context=%q>\n%s\n</mewcode-active-skill>",
			skill.Name, skill.Mode, skill.Model, skill.Context, strings.TrimSpace(skill.Body))
		messages = append(messages, prompt.InternalInstruction(content))
	}
	return messages
}

func (m *Manager) ContextMessages() []chat.Message {
	if m == nil {
		return nil
	}
	messages := []chat.Message{m.SummaryMessage()}
	messages = append(messages, m.ActiveMessages()...)
	return messages
}

func (m *Manager) FilterDefinitions(defs []tool.Definition) []tool.Definition {
	if m == nil || len(m.Active) == 0 {
		return defs
	}
	allowed := map[string]bool{}
	first := true
	for _, skill := range m.Active {
		current := map[string]bool{}
		for _, name := range skill.Tools {
			current[name] = true
		}
		if len(current) == 0 {
			continue
		}
		if first {
			for name := range current {
				allowed[name] = true
			}
			first = false
			continue
		}
		for name := range allowed {
			if !current[name] {
				delete(allowed, name)
			}
		}
	}
	if first {
		return defs
	}
	allowed[LoadToolName] = true
	filtered := make([]tool.Definition, 0, len(defs))
	for _, def := range defs {
		if allowed[def.Name] {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
}

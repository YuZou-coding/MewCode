package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ParseMarkdown(path string, source Source, content []byte) (Skill, error) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, fmt.Errorf("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(text, "---\n")
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return Skill{}, fmt.Errorf("unterminated YAML frontmatter")
	}
	metaText := rest[:index]
	body := strings.TrimPrefix(rest[index:], "\n---")
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	meta, err := parseMeta(metaText)
	if err != nil {
		return Skill{}, err
	}
	name := normalizeName(meta["name"])
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	mode := Mode(valueOr(meta["mode"], string(ModeShared)))
	if mode != ModeShared && mode != ModeIsolated {
		return Skill{}, fmt.Errorf("unsupported skill mode: %s", mode)
	}
	contextStrategy := ContextStrategy(valueOr(meta["context"], string(ContextRecent)))
	if contextStrategy != ContextFullSummary && contextStrategy != ContextRecent && contextStrategy != ContextEmpty {
		return Skill{}, fmt.Errorf("unsupported skill context: %s", contextStrategy)
	}
	dir := filepath.Dir(path)
	skill := Skill{
		Name:        name,
		Description: strings.TrimSpace(meta["description"]),
		Tools:       parseList(meta["tools"]),
		Mode:        mode,
		Model:       strings.TrimSpace(meta["model"]),
		Context:     contextStrategy,
		Body:        body,
		Source:      source,
		Path:        path,
		Dir:         dir,
	}
	scripts, err := loadScriptToolSpecs(dir)
	if err != nil {
		return Skill{}, err
	}
	skill.ScriptTools = scripts
	return skill, nil
}

func parseMeta(text string) (map[string]string, error) {
	values := map[string]string{}
	lines := strings.Split(text, "\n")
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid frontmatter line %d", index+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" && index+1 < len(lines) {
			var items []string
			for index+1 < len(lines) {
				next := strings.TrimSpace(lines[index+1])
				if !strings.HasPrefix(next, "- ") {
					break
				}
				items = append(items, strings.TrimSpace(strings.TrimPrefix(next, "- ")))
				index++
			}
			value = "[" + strings.Join(items, ",") + "]"
		}
		values[key] = strings.Trim(value, `"'`)
	}
	return values, nil
}

func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), `"'`)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func loadScriptToolSpecs(dir string) ([]ScriptToolSpec, error) {
	if filepath.Base(dir) != strings.TrimSuffix(EntryFileName, ".md") && !fileExists(filepath.Join(dir, EntryFileName)) {
		return nil, nil
	}
	path := filepath.Join(dir, "tools.json")
	if !fileExists(path) {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var specs []ScriptToolSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i := range specs {
		if specs[i].Name == "" || specs[i].Command == "" {
			return nil, fmt.Errorf("script tool requires name and command")
		}
	}
	return specs, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

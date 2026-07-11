package worker

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseMarkdown(path string, source Source, content []byte) (Role, error) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return Role{}, fmt.Errorf("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(text, "---\n")
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return Role{}, fmt.Errorf("unterminated YAML frontmatter")
	}
	metaText := rest[:index]
	body := strings.TrimPrefix(rest[index:], "\n---")
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	meta, err := parseMeta(metaText)
	if err != nil {
		return Role{}, err
	}
	name := normalizeName(meta["name"])
	if name == "" {
		return Role{}, fmt.Errorf("worker name is required")
	}
	mode := PermissionMode(valueOr(meta["permission_mode"], string(PermissionDefault)))
	if mode != PermissionDefault && mode != PermissionStrict && mode != PermissionAllow {
		return Role{}, fmt.Errorf("unsupported permission_mode: %s", mode)
	}
	maxIterations := 0
	if raw := strings.TrimSpace(meta["max_iterations"]); raw != "" {
		maxIterations, err = strconv.Atoi(raw)
		if err != nil {
			return Role{}, fmt.Errorf("invalid max_iterations: %w", err)
		}
	}
	isolation := IsolationMode(valueOr(meta["isolation"], string(IsolationNone)))
	if isolation != IsolationNone && isolation != IsolationWorktree {
		return Role{}, fmt.Errorf("unsupported isolation: %s", isolation)
	}
	return Role{
		Name:            name,
		Description:     strings.TrimSpace(meta["description"]),
		ToolsAllow:      parseList(meta["tools_allow"]),
		ToolsDeny:       parseList(meta["tools_deny"]),
		Model:           strings.TrimSpace(meta["model"]),
		MaxIterations:   maxIterations,
		PermissionMode:  mode,
		BackgroundTools: parseList(meta["background_tools"]),
		Isolation:       isolation,
		Body:            body,
		Source:          source,
		Path:            path,
	}, nil
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
			if len(items) > 0 {
				value = "[" + strings.Join(items, ",") + "]"
			}
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
		item := normalizeName(strings.Trim(strings.TrimSpace(part), `"'`))
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

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
}

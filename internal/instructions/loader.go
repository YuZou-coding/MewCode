package instructions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Load(projectRoot, homeDir string) Result {
	var result Result
	userPath := filepath.Join(homeDir, ".mewcode", FileName)
	if content, warnings := loadFile(userPath, filepath.Dir(userPath), filepath.Join(homeDir, ".mewcode"), 0, map[string]bool{}); strings.TrimSpace(content) != "" {
		result.Blocks = append(result.Blocks, Block{Source: "user", Priority: 20, Content: content})
		result.Warnings = append(result.Warnings, warnings...)
	} else {
		result.Warnings = append(result.Warnings, warnings...)
	}
	projectPath := filepath.Join(projectRoot, FileName)
	if content, warnings := loadFile(projectPath, filepath.Dir(projectPath), projectRoot, 0, map[string]bool{}); strings.TrimSpace(content) != "" {
		result.Blocks = append([]Block{{Source: "project", Priority: 10, Content: content}}, result.Blocks...)
		result.Warnings = append(result.Warnings, warnings...)
	} else {
		result.Warnings = append(result.Warnings, warnings...)
	}
	return result
}

func loadFile(path, baseDir, sandboxRoot string, depth int, seen map[string]bool) (string, []string) {
	if depth > MaxIncludeDepth {
		return "", []string{fmt.Sprintf("instruction include depth exceeded: %s", path)}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	realPath, err := filepath.Abs(path)
	if err != nil {
		return "", []string{fmt.Sprintf("instruction path failed: %v", err)}
	}
	if !inside(realPath, sandboxRoot) {
		return "", []string{fmt.Sprintf("instruction include blocked outside root: %s", path)}
	}
	if seen[realPath] {
		return "", []string{fmt.Sprintf("instruction include cycle skipped: %s", path)}
	}
	raw, err := os.ReadFile(realPath)
	if err != nil {
		return "", []string{fmt.Sprintf("instruction read failed: %v", err)}
	}
	seen[realPath] = true
	defer delete(seen, realPath)

	var out []string
	var warnings []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@include ") {
			target := strings.TrimSpace(strings.TrimPrefix(trimmed, "@include "))
			target = strings.Trim(target, `"'`)
			included, nestedWarnings := loadFile(filepath.Join(filepath.Dir(realPath), target), filepath.Dir(realPath), sandboxRoot, depth+1, seen)
			warnings = append(warnings, nestedWarnings...)
			if strings.TrimSpace(included) != "" {
				out = append(out, included)
			}
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")), warnings
}

func inside(path, root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rootEval, err := filepath.EvalSymlinks(absRoot)
	if err == nil {
		absRoot = rootEval
	}
	pathEval, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = pathEval
	}
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"mewcode/internal/tool"
)

func Load(projectRoot, homeDir string, registry *tool.Registry) (LoadResult, error) {
	result := LoadResult{Skills: map[string]Skill{}}
	for _, skill := range Builtins() {
		result.Skills[skill.Name] = skill
	}
	loadDir(filepath.Join(homeDir, ".mewcode", UserDirName), SourceUser, result.Skills, &result.Warnings)
	loadDir(filepath.Join(projectRoot, ProjectDir), SourceProject, result.Skills, &result.Warnings)
	if err := ValidateTools(result.Skills, registry); err != nil {
		return result, err
	}
	return result, nil
}

func loadDir(root string, source Source, skills map[string]Skill, warnings *[]string) {
	entries, err := discover(root)
	if err != nil {
		if !os.IsNotExist(err) {
			*warnings = append(*warnings, fmt.Sprintf("skill discover warning: %v", err))
		}
		return
	}
	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("skill read warning %s: %v", path, err))
			continue
		}
		skill, err := ParseMarkdown(path, source, raw)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("skill parse warning %s: %v", path, err))
			continue
		}
		skills[skill.Name] = skill
	}
}

func discover(root string) ([]string, error) {
	infos, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, info := range infos {
		path := filepath.Join(root, info.Name())
		if info.IsDir() {
			entry := filepath.Join(path, EntryFileName)
			if fileExists(entry) {
				paths = append(paths, entry)
			}
			continue
		}
		if filepath.Ext(info.Name()) == ".md" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func ValidateTools(skills map[string]Skill, registry *tool.Registry) error {
	known := map[string]bool{LoadToolName: true}
	if registry != nil {
		for _, def := range registry.Definitions() {
			known[def.Name] = true
		}
	}
	for _, skill := range skills {
		for _, spec := range skill.ScriptTools {
			known[spec.Name] = true
		}
	}
	for _, skill := range skills {
		for _, name := range skill.Tools {
			if !known[name] {
				return fmt.Errorf("skill %s references missing tool %s", skill.Name, name)
			}
		}
	}
	return nil
}

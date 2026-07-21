package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func Load(projectRoot, homeDir string, opts Options) LoadResult {
	result := LoadResult{Roles: map[string]Role{}}
	for _, role := range Builtins(opts.EnableVerify) {
		result.Roles[role.Name] = role
	}
	loadDir(filepath.Join(homeDir, ".mewcode", UserDirName), SourceUser, result.Roles, &result.Warnings)
	loadDir(filepath.Join(projectRoot, ProjectDir), SourceProject, result.Roles, &result.Warnings)
	return result
}

func Builtins(enableVerify bool) []Role {
	roles := []Role{
		{Name: "explore", Description: "探索代码结构并汇报关键发现", ToolsAllow: []string{"read_file", "find_files", "search_code"}, PermissionMode: PermissionDefault, Body: "你负责直接探索代码结构。必须实际调用可用的读类工具检查项目，禁止只返回计划、承诺或下一步。最终报告必须给出：项目入口、主要目录职责、关键数据流、测试位置、风险或未知项；每项都引用具体文件路径作为证据。若工具匹配为空，调整查询继续探索，不得据此提前结束。", Source: SourceBuiltin},
		{Name: "plan", Description: "制定实现计划和风险清单", ToolsAllow: []string{"read_file", "find_files", "search_code"}, PermissionMode: PermissionDefault, Body: "你负责制定可执行计划，先理解现状，再给出步骤、风险和验证方式。", Source: SourceBuiltin},
		{Name: "general", Description: "通用子工作者", PermissionMode: PermissionDefault, Body: "你是通用子工作者，直接完成交给你的任务并给出简洁结果。", Source: SourceBuiltin},
	}
	if enableVerify {
		roles = append(roles, Role{Name: "verify", Description: "执行验证检查并汇报结果", ToolsAllow: []string{"read_file", "find_files", "search_code", "run_command"}, BackgroundTools: []string{"read_file", "find_files", "search_code", "run_command"}, PermissionMode: PermissionDefault, Body: "你负责运行或设计验证步骤，汇报命令、结果和失败原因。", Source: SourceBuiltin})
	}
	return roles
}

func loadDir(root string, source Source, roles map[string]Role, warnings *[]string) {
	entries, err := discover(root)
	if err != nil {
		if !os.IsNotExist(err) {
			*warnings = append(*warnings, fmt.Sprintf("worker discover warning: %v", err))
		}
		return
	}
	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("worker read warning %s: %v", path, err))
			continue
		}
		role, err := ParseMarkdown(path, source, raw)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("worker parse warning %s: %v", path, err))
			continue
		}
		roles[role.Name] = role
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

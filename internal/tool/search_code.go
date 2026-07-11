package tool

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type SearchCode struct{}

type searchCodeArgs struct {
	Pattern string `json:"pattern"`
	Root    string `json:"root"`
	Regex   bool   `json:"regex"`
}

type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func (SearchCode) Definition() Definition {
	return Definition{
		Name:        "search_code",
		Description: "搜索文件内容并返回路径、行号和匹配行。优先使用这个专用工具搜索代码，不要用 run_command 调 grep。",
		Schema: ObjectSchema([]string{"pattern"}, map[string]any{
			"pattern": StringProperty("Text or regex pattern to search for."),
			"root":    StringProperty("Root directory to search. Defaults to current directory."),
			"regex": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a regular expression.",
			},
		}),
	}
}

func (SearchCode) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[searchCodeArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	root := args.Root
	if root == "" {
		root = "."
	}

	var re *regexp.Regexp
	if args.Regex {
		re, err = regexp.Compile(args.Pattern)
		if err != nil {
			return Fail("invalid_pattern", err.Error())
		}
	}

	var matches []SearchMatch
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if matchesPattern(text, args.Pattern, re) {
				matches = append(matches, SearchMatch{Path: path, Line: line, Text: text})
			}
		}
		return scanner.Err()
	})
	if walkErr != nil {
		return Fail("search_failed", walkErr.Error())
	}
	return OK(map[string]any{"matches": matches})
}

func matchesPattern(text string, pattern string, re *regexp.Regexp) bool {
	if re != nil {
		return re.MatchString(text)
	}
	return strings.Contains(text, pattern)
}

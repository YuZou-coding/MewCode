package tool

import (
	"context"
	"path/filepath"
	"sort"
)

type FindFiles struct{}

type findFilesArgs struct {
	Pattern string `json:"pattern"`
}

func (FindFiles) Definition() Definition {
	return Definition{
		Name:        "find_files",
		Description: "按 glob 模式查找文件。优先使用这个专用工具定位文件，不要用 run_command 调 find。",
		Schema: ObjectSchema([]string{"pattern"}, map[string]any{
			"pattern": StringProperty("Glob pattern, for example *.go or internal/**/*.go."),
		}),
	}
}

func (FindFiles) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[findFilesArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	select {
	case <-ctx.Done():
		return Fail("cancelled", ctx.Err().Error())
	default:
	}

	matches, err := filepath.Glob(args.Pattern)
	if err != nil {
		return Fail("invalid_pattern", err.Error())
	}
	sort.Strings(matches)
	return OK(map[string]any{"pattern": args.Pattern, "matches": matches})
}

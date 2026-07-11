package tool

import (
	"context"
	"os"
)

type WriteFile struct{}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (WriteFile) Definition() Definition {
	return Definition{
		Name:        "write_file",
		Description: "确认后创建或覆盖文本文件。写入前应确认目标内容完整，优先使用专用工具而不是 run_command。",
		Schema: ObjectSchema([]string{"path", "content"}, map[string]any{
			"path":    StringProperty("Path of the file to write."),
			"content": StringProperty("Complete file content to write."),
		}),
	}
}

func (WriteFile) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[writeFileArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	select {
	case <-ctx.Done():
		return Fail("cancelled", ctx.Err().Error())
	default:
	}

	if err := os.WriteFile(args.Path, []byte(args.Content), 0644); err != nil {
		return Fail("write_failed", err.Error())
	}
	return OK(map[string]any{"path": args.Path, "bytes": len(args.Content)})
}

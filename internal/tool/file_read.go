package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

type ReadFile struct{}

type readFileArgs struct {
	Path string `json:"path"`
}

func (ReadFile) Definition() Definition {
	return Definition{
		Name:        "read_file",
		Description: "读取文本文件内容。优先使用这个专用工具观察文件，不要用 run_command 调 cat。",
		Schema: ObjectSchema([]string{"path"}, map[string]any{
			"path": StringProperty("Path of the file to read."),
		}),
	}
}

func (ReadFile) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[readFileArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	select {
	case <-ctx.Done():
		return Fail("cancelled", ctx.Err().Error())
	default:
	}

	content, err := os.ReadFile(args.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Fail("file_not_found", err.Error())
		}
		return Fail("read_failed", err.Error())
	}
	return OK(map[string]any{"path": args.Path, "content": string(content)})
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

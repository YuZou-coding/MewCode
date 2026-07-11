package tool

import (
	"context"
	"os"
	"strings"
)

type EditFile struct{}

type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

func (EditFile) Definition() Definition {
	return Definition{
		Name:        "edit_file",
		Description: "先读取相关文件，再确认后用原文唯一匹配替换修改文件。优先使用这个专用编辑工具，不要用 run_command 改文件。",
		Schema: ObjectSchema([]string{"path", "old_text", "new_text"}, map[string]any{
			"path":     StringProperty("Path of the file to edit."),
			"old_text": StringProperty("Exact original text. It must match exactly once."),
			"new_text": StringProperty("Replacement text."),
		}),
	}
}

func (EditFile) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[editFileArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	if args.OldText == "" {
		return Fail("invalid_arguments", "old_text is required")
	}
	select {
	case <-ctx.Done():
		return Fail("cancelled", ctx.Err().Error())
	default:
	}

	raw, err := os.ReadFile(args.Path)
	if err != nil {
		return Fail("read_failed", err.Error())
	}
	content := string(raw)
	count := strings.Count(content, args.OldText)
	if count == 0 {
		return Fail("edit_failed", "old_text not found")
	}
	if count > 1 {
		return Fail("edit_failed", "old_text matched multiple times")
	}

	next := strings.Replace(content, args.OldText, args.NewText, 1)
	if err := os.WriteFile(args.Path, []byte(next), 0644); err != nil {
		return Fail("write_failed", err.Error())
	}
	return OK(map[string]any{"path": args.Path, "replacements": 1})
}

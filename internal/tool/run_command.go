package tool

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

type RunCommand struct{}

type runCommandArgs struct {
	Command string `json:"command"`
}

func (RunCommand) Definition() Definition {
	return Definition{
		Name:        "run_command",
		Description: "确认后执行 shell 命令。仅在专用工具无法完成时使用，不要优先使用 run_command 读取、搜索或编辑文件。",
		Schema: ObjectSchema([]string{"command"}, map[string]any{
			"command": StringProperty("Shell command to execute."),
		}),
	}
}

func (RunCommand) Execute(ctx context.Context, input Input) Result {
	args, err := DecodeArgs[runCommandArgs](input.Arguments)
	if err != nil {
		return Fail("invalid_arguments", err.Error())
	}
	if input.Confirm == nil || !input.Confirm(ctx, args.Command) {
		return Fail("command_denied", "command denied by user")
	}

	timeout := input.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, cmdArgs := shellCommand(args.Command)
	cmd := exec.CommandContext(cmdCtx, name, cmdArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	result := map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode(err),
	}
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return Result{OK: false, Data: result, Error: &Error{Code: "command_timeout", Message: "command timed out"}}
	}
	if err != nil {
		return Result{OK: false, Data: result, Error: &Error{Code: "command_failed", Message: err.Error()}}
	}
	return OK(result)
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

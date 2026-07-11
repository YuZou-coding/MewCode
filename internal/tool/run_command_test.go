package tool

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCommandDenied(t *testing.T) {
	if (RunCommand{}).Definition().Name != "run_command" {
		t.Fatalf("unexpected run_command definition")
	}
	result := RunCommand{}.Execute(context.Background(), Input{
		Arguments: mustJSON(map[string]any{"command": "echo hello"}),
		Confirm: func(ctx context.Context, command string) bool {
			return false
		},
	})
	if result.OK || result.Error == nil || result.Error.Code != "command_denied" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCommandSuccess(t *testing.T) {
	result := RunCommand{}.Execute(context.Background(), Input{
		Arguments: mustJSON(map[string]any{"command": "echo hello"}),
		Confirm: func(ctx context.Context, command string) bool {
			return true
		},
	})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Data["stdout"].(string), "hello") {
		t.Fatalf("stdout = %#v", result.Data["stdout"])
	}
	if result.Data["exit_code"] != 0 {
		t.Fatalf("exit_code = %#v", result.Data["exit_code"])
	}
}

func TestRunCommandFailure(t *testing.T) {
	result := RunCommand{}.Execute(context.Background(), Input{
		Arguments: mustJSON(map[string]any{"command": failingCommand()}),
		Confirm: func(ctx context.Context, command string) bool {
			return true
		},
	})
	if result.OK || result.Error == nil || result.Error.Code != "command_failed" {
		t.Fatalf("result = %#v", result)
	}
	if result.Data["exit_code"] == 0 {
		t.Fatalf("exit_code = %#v", result.Data["exit_code"])
	}
}

func TestRunCommandTimeout(t *testing.T) {
	result := RunCommand{}.Execute(context.Background(), Input{
		Arguments: mustJSON(map[string]any{"command": sleepCommand()}),
		Timeout:   10 * time.Millisecond,
		Confirm: func(ctx context.Context, command string) bool {
			return true
		},
	})
	if result.OK || result.Error == nil || result.Error.Code != "command_timeout" {
		t.Fatalf("result = %#v", result)
	}
}

func failingCommand() string {
	if runtime.GOOS == "windows" {
		return "exit 7"
	}
	return "exit 7"
}

func sleepCommand() string {
	if runtime.GOOS == "windows" {
		return "ping -n 2 127.0.0.1 > nul"
	}
	return "sleep 1"
}

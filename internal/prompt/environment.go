package prompt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"mewcode/internal/chat"
)

type Environment struct {
	CWD  string
	OS   string
	Time time.Time
	Git  string
}

func CurrentEnvironment(ctx context.Context, now time.Time) Environment {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unknown"
	}
	return Environment{
		CWD:  cwd,
		OS:   runtime.GOOS,
		Time: now,
		Git:  gitSummary(ctx),
	}
}

func EnvironmentMessage(env Environment) chat.Message {
	content := fmt.Sprintf("<mewcode-environment>\ncwd: %s\nos: %s\ntime: %s\ngit: %s\n</mewcode-environment>",
		env.CWD,
		env.OS,
		env.Time.Format(time.RFC3339),
		env.Git,
	)
	return chat.Message{Role: chat.RoleSystem, Content: content}
}

func gitSummary(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "status", "--short", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	summary := strings.TrimSpace(string(out))
	if summary == "" {
		return "clean"
	}
	return strings.ReplaceAll(summary, "\n", "; ")
}

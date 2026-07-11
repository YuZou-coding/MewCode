package team

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type MergeResult struct {
	OK       bool
	Rolled   bool
	Output   string
	Error    string
	Strategy string
}

func ConservativeMerge(ctx context.Context, repo, branch string) MergeResult {
	if strings.TrimSpace(repo) == "" || strings.TrimSpace(branch) == "" {
		return MergeResult{OK: false, Error: "repo and branch are required", Strategy: "conservative"}
	}
	if out, err := gitOutput(ctx, repo, "status", "--porcelain"); err != nil {
		return MergeResult{OK: false, Error: err.Error(), Output: out, Strategy: "conservative"}
	} else if strings.TrimSpace(out) != "" {
		return MergeResult{OK: false, Error: "working tree is dirty", Output: out, Strategy: "conservative"}
	}
	if out, err := gitOutput(ctx, repo, "merge", "--ff-only", branch); err == nil {
		return MergeResult{OK: true, Output: out, Strategy: "ff-only"}
	}
	out, err := gitOutput(ctx, repo, "merge", "--no-edit", "--no-ff", branch)
	if err == nil {
		return MergeResult{OK: true, Output: out, Strategy: "no-ff"}
	}
	abortOut, _ := gitOutput(ctx, repo, "merge", "--abort")
	return MergeResult{OK: false, Rolled: true, Output: out + abortOut, Error: err.Error(), Strategy: "conservative"}
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	if err != nil {
		return b.String(), fmt.Errorf("%s: %w", strings.TrimSpace(b.String()), err)
	}
	return b.String(), nil
}

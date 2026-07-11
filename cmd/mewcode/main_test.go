package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupGlobalHelpDocumentsCopyPolicy(t *testing.T) {
	var out bytes.Buffer
	code := runCLI(context.Background(), []string{"mewcode", "setup-global", "--help"}, strings.NewReader(""), &out, &out)
	if code != 0 {
		t.Fatalf("code = %d output=%s", code, out.String())
	}
	for _, want := range []string{
		"mewcode setup-global",
		"默认复制: mewcode.yaml, MEWCODE.md",
		"可选复制: permissions, hooks, notes, skills, workers, servers",
		"永不复制: sessions, artifacts, worktrees, teams",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, out.String())
		}
	}
}

func TestSetupGlobalDryRunAndCopyDefaultFiles(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(source, "mewcode.yaml"), []byte("protocol: openai\nmodel: gpt-test\nbase_url: http://provider.test\napi_key: key\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "MEWCODE.md"), []byte("USER RULE"), 0600); err != nil {
		t.Fatalf("write instruction: %v", err)
	}

	var dry bytes.Buffer
	code := runCLI(context.Background(), []string{"mewcode", "setup-global", "--from", source, "--dry-run"}, strings.NewReader(""), &dry, &dry)
	if code != 0 {
		t.Fatalf("dry code = %d output=%s", code, dry.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".mewcode", "mewcode.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config, stat err=%v", err)
	}

	var out bytes.Buffer
	code = runCLI(context.Background(), []string{"mewcode", "setup-global", "--from", source}, strings.NewReader(""), &out, &out)
	if code != 0 {
		t.Fatalf("code = %d output=%s", code, out.String())
	}
	raw, err := os.ReadFile(filepath.Join(home, ".mewcode", "mewcode.yaml"))
	if err != nil || !strings.Contains(string(raw), "gpt-test") {
		t.Fatalf("global config raw=%q err=%v", raw, err)
	}
	raw, err = os.ReadFile(filepath.Join(home, ".mewcode", "MEWCODE.md"))
	if err != nil || string(raw) != "USER RULE" {
		t.Fatalf("global instruction raw=%q err=%v", raw, err)
	}
}

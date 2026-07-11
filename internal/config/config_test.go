package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValidConfig(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
protocol: anthropic
model: claude-sonnet-4-5
base_url: https://api.anthropic.com
api_key: test-key
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Protocol != "anthropic" || cfg.Model == "" || cfg.BaseURL == "" || cfg.APIKey != "test-key" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseMissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "model",
			body: "protocol: openai\nbase_url: http://localhost\napi_key: key\n",
			want: "missing required config field: model",
		},
		{
			name: "base_url",
			body: "protocol: openai\nmodel: gpt-test\napi_key: key\n",
			want: "missing required config field: base_url",
		},
		{
			name: "api_key",
			body: "protocol: openai\nmodel: gpt-test\nbase_url: http://localhost\n",
			want: "missing required config field: api_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.body))
			if err == nil || err.Error() != tt.want {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseStripsCommentsAndQuotes(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
protocol: "openai" # provider
model: 'gpt-test'
base_url: http://localhost
api_key: secret
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Protocol != "openai" || cfg.Model != "gpt-test" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseWorkerConfigFieldsAndKeepsOldConfigCompatible(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
protocol: openai
model: gpt-test
base_url: http://localhost
api_key: secret
worker_enable_verify: true
worker_background_threshold: 10s
worktree_copy_files: settings.local.json,.env.local
worktree_link_dirs: node_modules,.venv
worktree_ttl: 12h
team_default_backend: terminal_pane
team_scheduler_enabled: true
team_default_member_approval: true
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.WorkerEnableVerify || cfg.WorkerBackgroundThreshold != "10s" {
		t.Fatalf("worker config = %#v", cfg)
	}
	if strings.Join(cfg.WorktreeCopyFiles, ",") != "settings.local.json,.env.local" || strings.Join(cfg.WorktreeLinkDirs, ",") != "node_modules,.venv" || cfg.WorktreeTTL != "12h" {
		t.Fatalf("worktree config = %#v", cfg)
	}
	if cfg.TeamDefaultBackend != "terminal_pane" || !cfg.TeamSchedulerEnabled || !cfg.TeamDefaultMemberApproval {
		t.Fatalf("team config = %#v", cfg)
	}

	old, err := Parse(strings.NewReader(`
protocol: openai
model: gpt-test
base_url: http://localhost
api_key: secret
`))
	if err != nil {
		t.Fatalf("Parse old config returned error: %v", err)
	}
	if old.WorkerEnableVerify || old.WorkerBackgroundThreshold != "" || len(old.WorktreeCopyFiles) != 0 || len(old.WorktreeLinkDirs) != 0 || old.WorktreeTTL != "" || old.TeamDefaultBackend != "" || old.TeamSchedulerEnabled || old.TeamDefaultMemberApproval {
		t.Fatalf("old config should keep zero worker fields: %#v", old)
	}
}

func TestLoadProjectFallsBackToUserConfig(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(project); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	userConfigDir := filepath.Join(home, ".mewcode")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userConfigDir, FileName), []byte(`
protocol: openai
model: user-model
base_url: http://user.example
api_key: user-key
`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := LoadProject()
	if err != nil {
		t.Fatalf("LoadProject returned error: %v", err)
	}
	if cfg.Model != "user-model" || cfg.BaseURL != "http://user.example" || cfg.APIKey != "user-key" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestLoadProjectPrefersProjectConfigOverUserConfig(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(project); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	userConfigDir := filepath.Join(home, ".mewcode")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userConfigDir, FileName), []byte(`
protocol: openai
model: user-model
base_url: http://user.example
api_key: user-key
`), 0o600); err != nil {
		t.Fatalf("WriteFile user returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, FileName), []byte(`
protocol: openai
model: project-model
base_url: http://project.example
api_key: project-key
`), 0o600); err != nil {
		t.Fatalf("WriteFile project returned error: %v", err)
	}

	cfg, err := LoadProject()
	if err != nil {
		t.Fatalf("LoadProject returned error: %v", err)
	}
	if cfg.Model != "project-model" || cfg.BaseURL != "http://project.example" || cfg.APIKey != "project-key" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestLoadProjectMissingConfigMentionsGlobalSetup(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(project); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}

	_, err = LoadProject()
	if err == nil {
		t.Fatalf("LoadProject returned nil error")
	}
	for _, want := range []string{"mewcode.yaml", filepath.Join(home, ".mewcode", "mewcode.yaml"), "mewcode setup-global"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestParseMaxIterationsDefaultsAndOverride(t *testing.T) {
	base := "protocol: openai\nmodel: test\nbase_url: http://example.test\napi_key: key\n"
	cfg, err := Parse(strings.NewReader(base))
	if err != nil {
		t.Fatalf("Parse default: %v", err)
	}
	if cfg.MaxIterations != 30 {
		t.Fatalf("default max iterations = %d", cfg.MaxIterations)
	}
	cfg, err = Parse(strings.NewReader(base + "max_iterations: 42\n"))
	if err != nil {
		t.Fatalf("Parse override: %v", err)
	}
	if cfg.MaxIterations != 42 {
		t.Fatalf("override max iterations = %d", cfg.MaxIterations)
	}
}

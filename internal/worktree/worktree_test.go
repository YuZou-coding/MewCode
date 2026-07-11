package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateNameAndPaths(t *testing.T) {
	m := NewManager(t.TempDir(), Config{})
	info, err := m.Info("feature/foo")
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if info.Name != "feature/foo" || info.Branch != "codex/feature-foo" || !strings.HasSuffix(info.Path, filepath.Join(".mewcode", "worktrees", "feature", "foo")) {
		t.Fatalf("info = %#v", info)
	}
	for _, name := range []string{"", ".", "..", "feature//foo", "feature/../x", "/abs", "bad space", strings.Repeat("a", MaxNameLength+1)} {
		if _, err := m.Info(name); err == nil {
			t.Fatalf("expected invalid name %q", name)
		}
	}
}

func TestCreateRequiresInitialCommit(t *testing.T) {
	repo := initRepo(t, false)
	m := NewManager(repo, Config{})
	_, err := m.Create(context.Background(), "feature/foo")
	if err == nil || !strings.Contains(err.Error(), "initial commit") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateInitializesAndFastRestoresExistingDirectory(t *testing.T) {
	repo := initRepo(t, true)
	writeFile(t, filepath.Join(repo, "settings.local.json"), `{"ok":true}`)
	if err := os.Mkdir(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(repo, Config{CopyFiles: []string{"settings.local.json", "missing.env"}, LinkDirs: []string{"node_modules", "missing_dir"}, TTL: 7 * 24 * time.Hour})
	created, err := m.Create(context.Background(), "feature/foo")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.FastRestored {
		t.Fatalf("first create should not fast restore")
	}
	if _, err := os.Stat(filepath.Join(created.Path, "settings.local.json")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	link, err := os.Readlink(filepath.Join(created.Path, "node_modules"))
	if err != nil || link != filepath.Join(repo, "node_modules") {
		t.Fatalf("link = %q err=%v", link, err)
	}
	if len(created.Warnings) != 2 {
		t.Fatalf("warnings = %#v", created.Warnings)
	}
	if got := gitOut(t, created.Path, "config", "--get", "core.hooksPath"); got == "" || !strings.Contains(got, ".git/hooks") {
		t.Fatalf("hooksPath = %q", got)
	}
	state, err := m.LoadState()
	if err != nil || state.MainRoot != repo || state.ActiveName != "" {
		t.Fatalf("state=%#v err=%v", state, err)
	}

	fast, err := m.Create(context.Background(), "feature/foo")
	if err != nil {
		t.Fatalf("fast Create returned error: %v", err)
	}
	if !fast.FastRestored {
		t.Fatalf("expected fast restore")
	}
}

func TestDeleteProtectsDirtyAndForceDeletes(t *testing.T) {
	repo := initRepo(t, true)
	m := NewManager(repo, Config{})
	created, err := m.Create(context.Background(), "feature/foo")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	writeFile(t, filepath.Join(created.Path, "dirty.txt"), "dirty")
	if err := m.Delete(context.Background(), "feature/foo", false); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("delete err = %v", err)
	}
	if err := m.Delete(context.Background(), "feature/foo", true); err != nil {
		t.Fatalf("force delete returned error: %v", err)
	}
	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Fatalf("path still exists err=%v", err)
	}
}

func TestCleanupSkipsActiveAndDirtyExpiredWorktrees(t *testing.T) {
	repo := initRepo(t, true)
	m := NewManager(repo, Config{TTL: time.Nanosecond})
	active, err := m.Create(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "dirty"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, ".mewcode", "worktrees", "dirty", "dirty.txt"), "dirty")
	if err := m.SaveState(State{MainRoot: repo, ActiveName: "active", ActivePath: active.Path, LastEntered: time.Now()}); err != nil {
		t.Fatal(err)
	}
	mustChtimes(t, active.Path, time.Now().Add(-time.Hour))
	mustChtimes(t, filepath.Join(repo, ".mewcode", "worktrees", "dirty"), time.Now().Add(-time.Hour))
	result := m.Cleanup(context.Background())
	if result.Removed != 0 || result.Skipped == 0 {
		t.Fatalf("cleanup = %#v", result)
	}
}

func initRepo(t *testing.T, commit bool) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if commit {
		writeFile(t, filepath.Join(dir, "README.md"), "hello")
		runGit(t, dir, "add", "README.md")
		runGit(t, dir, "commit", "-m", "initial")
	}
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustChtimes(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

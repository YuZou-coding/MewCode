package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	DirName       = ".mewcode/worktrees"
	StateFileName = "state.json"
	BranchPrefix  = "codex/"
	MaxNameLength = 80
	DefaultTTL    = 7 * 24 * time.Hour
)

var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Config struct {
	CopyFiles []string
	LinkDirs  []string
	TTL       time.Duration
}

type Info struct {
	Name         string
	Path         string
	Branch       string
	Active       bool
	FastRestored bool
	Warnings     []string
	Head         string
}

type State struct {
	MainRoot    string    `json:"main_root"`
	ActiveName  string    `json:"active_name"`
	ActivePath  string    `json:"active_path"`
	LastEntered time.Time `json:"last_entered"`
}

type CleanupResult struct {
	Removed  int
	Skipped  int
	Warnings []string
}

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

type Manager struct {
	MainRoot string
	BaseDir  string
	Config   Config
	Runner   Runner
}

func NewManager(mainRoot string, cfg Config) *Manager {
	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}
	return &Manager{
		MainRoot: mainRoot,
		BaseDir:  filepath.Join(mainRoot, DirName),
		Config:   cfg,
		Runner:   ExecRunner{},
	}
}

func (m *Manager) Info(name string) (Info, error) {
	if err := ValidateName(name); err != nil {
		return Info{}, err
	}
	return Info{
		Name:   name,
		Path:   filepath.Join(m.BaseDir, filepath.FromSlash(name)),
		Branch: BranchName(name),
	}, nil
}

func ValidateName(name string) error {
	if name == "" {
		return errors.New("worktree name is required")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("worktree name exceeds %d characters", MaxNameLength)
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid worktree name: %s", name)
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid worktree name segment: %s", name)
		}
		if !segmentPattern.MatchString(part) {
			return fmt.Errorf("invalid worktree name segment: %s", part)
		}
	}
	return nil
}

func BranchName(name string) string {
	return BranchPrefix + strings.ReplaceAll(name, "/", "-")
}

func (m *Manager) Create(ctx context.Context, name string) (Info, error) {
	info, err := m.Info(name)
	if err != nil {
		return Info{}, err
	}
	if fileExists(info.Path) {
		head, err := m.git(ctx, info.Path, "rev-parse", "HEAD")
		if err != nil {
			return Info{}, fmt.Errorf("fast restore failed: %w", err)
		}
		info.FastRestored = true
		info.Head = strings.TrimSpace(string(head))
		_ = m.ensureState()
		return info, nil
	}
	if _, err := m.git(ctx, m.MainRoot, "rev-parse", "--verify", "HEAD"); err != nil {
		return Info{}, fmt.Errorf("initial commit required before creating worktree")
	}
	if err := os.MkdirAll(filepath.Dir(info.Path), 0o755); err != nil {
		return Info{}, err
	}
	if _, err := m.git(ctx, m.MainRoot, "worktree", "add", "-b", info.Branch, info.Path, "HEAD"); err != nil {
		return Info{}, err
	}
	info.Warnings = append(info.Warnings, m.initialize(ctx, info.Path)...)
	head, _ := m.git(ctx, info.Path, "rev-parse", "HEAD")
	info.Head = strings.TrimSpace(string(head))
	if err := m.ensureState(); err != nil {
		info.Warnings = append(info.Warnings, err.Error())
	}
	return info, nil
}

func (m *Manager) Enter(ctx context.Context, name string) (Info, error) {
	info, err := m.Create(ctx, name)
	if err != nil {
		return Info{}, err
	}
	if err := os.Chdir(info.Path); err != nil {
		return Info{}, err
	}
	_ = m.SaveState(State{MainRoot: m.MainRoot, ActiveName: info.Name, ActivePath: info.Path, LastEntered: time.Now()})
	return info, nil
}

func (m *Manager) Exit() error {
	if err := os.Chdir(m.MainRoot); err != nil {
		return err
	}
	return m.SaveState(State{MainRoot: m.MainRoot})
}

func (m *Manager) ensureState() error {
	if _, err := m.LoadState(); err == nil {
		return nil
	}
	return m.SaveState(State{MainRoot: m.MainRoot})
}

func (m *Manager) Delete(ctx context.Context, name string, force bool) error {
	info, err := m.Info(name)
	if err != nil {
		return err
	}
	if !force {
		if dirty, err := m.IsDirty(ctx, info.Path); err != nil || dirty {
			if err != nil {
				return fmt.Errorf("delete blocked: dirty check failed: %w", err)
			}
			return fmt.Errorf("delete blocked: dirty worktree")
		}
		if ahead, err := m.HasUnpushed(ctx, info.Path); err != nil || ahead {
			if err != nil {
				return fmt.Errorf("delete blocked: unpushed check failed: %w", err)
			}
			return fmt.Errorf("delete blocked: unpushed commits")
		}
	}
	_, _ = m.git(ctx, m.MainRoot, "worktree", "remove", "--force", info.Path)
	return os.RemoveAll(info.Path)
}

func (m *Manager) List() ([]Info, error) {
	var result []Info
	state, _ := m.LoadState()
	err := filepath.WalkDir(m.BaseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == m.BaseDir {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(m.BaseDir, path)
		name := filepath.ToSlash(rel)
		info, infoErr := m.Info(name)
		if infoErr != nil {
			return nil
		}
		info.Active = state.ActiveName == name
		result = append(result, info)
		return filepath.SkipDir
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return result, err
}

func (m *Manager) IsDirty(ctx context.Context, path string) (bool, error) {
	out, err := m.git(ctx, path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (m *Manager) HasUnpushed(ctx context.Context, path string) (bool, error) {
	out, err := m.git(ctx, path, "status", "--short", "--branch")
	if err != nil {
		return false, err
	}
	status := string(out)
	return strings.Contains(status, "[ahead ") || strings.Contains(status, "ahead "), nil
}

func (m *Manager) Cleanup(ctx context.Context) CleanupResult {
	var result CleanupResult
	items, err := m.List()
	if err != nil {
		if !os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, err.Error())
		}
		return result
	}
	state, _ := m.LoadState()
	for _, item := range items {
		if state.ActiveName == item.Name {
			result.Skipped++
			continue
		}
		info, err := os.Stat(item.Path)
		if err != nil || time.Since(info.ModTime()) < m.Config.TTL {
			result.Skipped++
			continue
		}
		if dirty, err := m.IsDirty(ctx, item.Path); err != nil || dirty {
			result.Skipped++
			continue
		}
		if ahead, err := m.HasUnpushed(ctx, item.Path); err != nil || ahead {
			result.Skipped++
			continue
		}
		if err := m.Delete(ctx, item.Name, true); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			result.Skipped++
			continue
		}
		result.Removed++
	}
	return result
}

func (m *Manager) SaveState(state State) error {
	path := filepath.Join(m.BaseDir, StateFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (m *Manager) LoadState() (State, error) {
	raw, err := os.ReadFile(filepath.Join(m.BaseDir, StateFileName))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (m *Manager) Resume(ctx context.Context) (Info, error) {
	state, err := m.LoadState()
	if err != nil {
		return Info{}, err
	}
	if state.ActiveName == "" {
		return Info{}, errors.New("no active worktree")
	}
	info, err := m.Info(state.ActiveName)
	if err != nil {
		return Info{}, err
	}
	if _, err := m.git(ctx, info.Path, "rev-parse", "HEAD"); err != nil {
		return Info{}, err
	}
	if err := os.Chdir(info.Path); err != nil {
		return Info{}, err
	}
	info.Active = true
	return info, nil
}

func (m *Manager) initialize(ctx context.Context, path string) []string {
	var warnings []string
	for _, name := range m.Config.CopyFiles {
		if err := safeRelative(name); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		src := filepath.Join(m.MainRoot, name)
		dst := filepath.Join(path, name)
		if err := copyFile(src, dst); err != nil {
			warnings = append(warnings, fmt.Sprintf("copy %s: %v", name, err))
		}
	}
	for _, name := range m.Config.LinkDirs {
		if err := safeRelative(name); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		src := filepath.Join(m.MainRoot, name)
		dst := filepath.Join(path, name)
		if _, err := os.Stat(src); err != nil {
			warnings = append(warnings, fmt.Sprintf("link %s: %v", name, err))
			continue
		}
		if err := os.Symlink(src, dst); err != nil {
			warnings = append(warnings, fmt.Sprintf("link %s: %v", name, err))
		}
	}
	hooksPath := filepath.Join(m.MainRoot, ".git", "hooks")
	if _, err := m.git(ctx, path, "config", "core.hooksPath", hooksPath); err != nil {
		warnings = append(warnings, fmt.Sprintf("hooks path: %v", err))
	}
	return warnings
}

func (m *Manager) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := m.Runner.Run(ctx, dir, "git", args...)
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func safeRelative(path string) error {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "..") {
		return fmt.Errorf("unsafe relative path: %s", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

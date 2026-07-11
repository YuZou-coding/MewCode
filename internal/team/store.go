package team

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	ProjectRoot string
	Options     Options
	Runner      RunnerFunc

	mu               sync.Mutex
	activeTeam       string
	activeActor      Actor
	schedulerEnabled bool
	warnings         []string
	running          map[string]context.CancelFunc
}

func NewManager(projectRoot string, opts Options) *Manager {
	if opts.DefaultBackend == "" {
		opts.DefaultBackend = BackendInProcess
	}
	return &Manager{
		ProjectRoot: projectRoot,
		Options:     opts,
		running:     map[string]context.CancelFunc{},
	}
}

func (m *Manager) root() string {
	return filepath.Join(m.ProjectRoot, RootDir)
}

func (m *Manager) teamDir(name string) string {
	return filepath.Join(m.root(), safeName(name))
}

func (m *Manager) Create(name string) (Team, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return Team{}, err
	}
	dir := m.teamDir(name)
	if err := os.MkdirAll(filepath.Join(dir, "mailboxes"), 0o755); err != nil {
		return Team{}, err
	}
	now := time.Now()
	team := Team{Name: name, Lead: "lead", Backend: m.Options.DefaultBackend, Status: TeamStatusStopped, Root: dir}
	members := []Member{{
		Name:             "lead",
		Role:             "lead",
		InstanceID:       "lead",
		Workdir:          m.ProjectRoot,
		Backend:          m.Options.DefaultBackend,
		RequiresApproval: m.Options.DefaultMemberApproval,
		Status:           MemberStatusIdle,
		LastActiveAt:     now,
		ResumeRef:        "lead",
	}}
	if err := writeJSON(filepath.Join(dir, "team.json"), team); err != nil {
		return Team{}, err
	}
	if err := writeJSON(filepath.Join(dir, "members.json"), members); err != nil {
		return Team{}, err
	}
	if err := writeJSON(filepath.Join(dir, "tasks.json"), []Task{}); err != nil {
		return Team{}, err
	}
	if err := appendJSONL(filepath.Join(dir, "events.jsonl"), Event{Type: "team.created", Message: name, CreatedAt: now}); err != nil {
		return Team{}, err
	}
	if err := ensureFile(filepath.Join(dir, "mailboxes", "lead.jsonl")); err != nil {
		return Team{}, err
	}
	return team, nil
}

func (m *Manager) Load(name string) (Team, error) {
	dir := m.teamDir(name)
	var team Team
	if err := readJSON(filepath.Join(dir, "team.json"), &team); err != nil {
		return Team{}, err
	}
	team.Root = dir
	return team, nil
}

func (m *Manager) List() ([]Team, error) {
	entries, err := os.ReadDir(m.root())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var teams []Team
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		team, err := m.Load(entry.Name())
		if err != nil {
			m.addWarning("team load skipped: " + err.Error())
			continue
		}
		teams = append(teams, team)
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })
	return teams, nil
}

func (m *Manager) Start(name string) (Team, error) {
	if _, err := m.ReloadMembersFromConfig(name); err != nil {
		return Team{}, err
	}
	team, err := m.Load(name)
	if err != nil {
		return Team{}, err
	}
	team.Status = TeamStatusRunning
	if err := writeJSON(filepath.Join(team.Root, "team.json"), team); err != nil {
		return Team{}, err
	}
	m.mu.Lock()
	m.activeTeam = team.Name
	m.activeActor = Actor{Team: team.Name, Name: team.Lead, Kind: ActorLead}
	m.mu.Unlock()
	_ = appendJSONL(filepath.Join(team.Root, "events.jsonl"), Event{Type: "team.started", Message: team.Name, CreatedAt: time.Now()})
	return team, nil
}

func (m *Manager) Stop(name string) error {
	team, err := m.Load(name)
	if err != nil {
		return err
	}
	team.Status = TeamStatusStopped
	if err := writeJSON(filepath.Join(team.Root, "team.json"), team); err != nil {
		return err
	}
	m.mu.Lock()
	if m.activeTeam == team.Name {
		m.activeTeam = ""
		m.activeActor = Actor{}
	}
	m.mu.Unlock()
	_ = appendJSONL(filepath.Join(team.Root, "events.jsonl"), Event{Type: "team.stopped", Message: team.Name, CreatedAt: time.Now()})
	return nil
}

func (m *Manager) ActiveActor() Actor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeActor
}

func (m *Manager) SetActor(actor Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeActor = actor
	m.activeTeam = actor.Team
}

func (m *Manager) SetSchedulerEnabled(enabled bool) error {
	if enabled && !m.Options.SchedulerAllowed {
		return errors.New("team scheduler is disabled by config")
	}
	m.mu.Lock()
	m.schedulerEnabled = enabled
	m.mu.Unlock()
	return nil
}

func (m *Manager) SchedulerEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.schedulerEnabled && m.Options.SchedulerAllowed
}

func (m *Manager) Warnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.warnings...)
}

func (m *Manager) WarningCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.warnings)
}

func (m *Manager) addWarning(warning string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warnings = append(m.warnings, warning)
}

func validateName(name string) error {
	if name == "" {
		return errors.New("team name is required")
	}
	if len(name) > 80 {
		return errors.New("team name is too long")
	}
	ok, _ := regexp.MatchString(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`, name)
	if !ok {
		return fmt.Errorf("invalid team name: %s", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid team name segment: %s", name)
		}
	}
	return nil
}

func safeName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "/", "__")
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func ensureFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func appendJSONL(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = file.Write(append(raw, '\n'))
	return err
}

func readJSONL[T any](path string) ([]T, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer file.Close()
	var items []T
	var warnings []string
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(text), &item); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s:%d invalid jsonl skipped", path, line))
			continue
		}
		items = append(items, item)
	}
	return items, warnings, scanner.Err()
}

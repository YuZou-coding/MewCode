package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/prompt"
)

type Manager struct {
	Roles           map[string]Role
	Warnings        []string
	Options         Options
	Runner          RunnerFunc
	ParentMessages  []chat.Message
	mu              sync.Mutex
	tasks           map[string]*Task
	notifications   []Notification
	nextID          int
	recentCompleted int
}

func NewManager(loaded LoadResult, opts Options) *Manager {
	if opts.BackgroundThreshold == 0 {
		opts.BackgroundThreshold = DefaultThreshold
	}
	return &Manager{
		Roles:    loaded.Roles,
		Warnings: loaded.Warnings,
		Options:  opts,
		tasks:    map[string]*Task{},
	}
}

func (m *Manager) ListRoles() []Role {
	if m == nil {
		return nil
	}
	items := make([]Role, 0, len(m.Roles))
	for _, role := range m.Roles {
		items = append(items, role)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (m *Manager) Role(name string) (Role, bool) {
	if m == nil {
		return Role{}, false
	}
	role, ok := m.Roles[normalizeName(name)]
	return role, ok
}

func (m *Manager) Run(ctx context.Context, req RunRequest) ToolRunResult {
	if m == nil {
		return ToolRunResult{OK: false, Error: "worker manager is not configured"}
	}
	if strings.TrimSpace(req.Task) == "" {
		return ToolRunResult{OK: false, Error: "worker task is required"}
	}
	if req.Fork {
		req.Background = true
	}
	if req.RoleName != "" {
		role, ok := m.Role(req.RoleName)
		if !ok {
			return ToolRunResult{OK: false, Error: "worker role not found: " + req.RoleName}
		}
		req.Role = role
		req.RoleName = role.Name
	}
	if req.RoleName == "" && !req.Fork {
		req.Fork = true
		req.Background = true
	}
	if req.Fork {
		req.ParentMessages = m.ParentSnapshot()
		req.RoleName = "fork"
	}
	if m.Runner == nil {
		return ToolRunResult{OK: false, Error: "worker runner is not configured"}
	}
	workerParent := ctx
	if req.Background {
		workerParent = context.WithoutCancel(ctx)
	}
	taskCtx, cancel := context.WithCancel(workerParent)
	task := m.createTask(req, cancel)
	req.TaskID = task.ID
	completed := make(chan struct{})
	go func() {
		result := m.Runner(taskCtx, req)
		m.finish(task.ID, result)
		close(completed)
	}()
	if req.Background {
		return ToolRunResult{OK: true, TaskID: task.ID, Background: true, Status: StatusRunning}
	}
	select {
	case <-completed:
		return m.resultForTask(task.ID, false)
	case <-time.After(m.Options.BackgroundThreshold):
		return ToolRunResult{OK: true, TaskID: task.ID, Background: true, Status: StatusRunning}
	case <-ctx.Done():
		cancel()
		return ToolRunResult{OK: false, TaskID: task.ID, Error: ctx.Err().Error()}
	}
}

func (m *Manager) SetParentMessages(messages []chat.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ParentMessages = append([]chat.Message(nil), messages...)
}

func (m *Manager) ParentSnapshot() []chat.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]chat.Message(nil), m.ParentMessages...)
}

func (m *Manager) createTask(req RunRequest, cancel context.CancelFunc) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	task := &Task{
		ID:        fmt.Sprintf("worker_%d", m.nextID),
		RoleName:  req.RoleName,
		Fork:      req.Fork,
		Task:      req.Task,
		Status:    StatusRunning,
		StartedAt: time.Now(),
		cancel:    cancel,
	}
	m.tasks[task.ID] = task
	return task
}

func (m *Manager) finish(id string, result RunResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok || task.Status == StatusCanceled {
		return
	}
	task.EndedAt = time.Now()
	task.Usage = result.Usage
	if result.Error != nil {
		task.Status = StatusFailed
		task.Error = result.Error.Error()
	} else {
		task.Status = StatusCompleted
		task.Result = result.Text
	}
	task.Notification = notificationText(*task)
	m.notifications = append(m.notifications, Notification{TaskID: task.ID, Role: task.RoleName, Status: string(task.Status), Result: task.Result, Error: task.Error})
	m.recentCompleted++
}

func (m *Manager) resultForTask(id string, background bool) ToolRunResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[id]
	result := ToolRunResult{OK: task.Status != StatusFailed && task.Status != StatusCanceled, TaskID: id, Background: background, Status: task.Status, Result: task.Result, Error: task.Error}
	if background {
		result.OK = true
	}
	return result
}

func (m *Manager) Task(id string) (Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *task, true
}

func (m *Manager) Tasks() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		items = append(items, *task)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.Before(items[j].StartedAt) })
	return items
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok || task.Status != StatusRunning {
		return false
	}
	task.Status = StatusCanceled
	task.EndedAt = time.Now()
	task.Error = "canceled"
	if task.cancel != nil {
		task.cancel()
	}
	return true
}

func (m *Manager) RunningCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, task := range m.tasks {
		if task.Status == StatusRunning {
			count++
		}
	}
	return count
}

// WaitForRunning blocks until all currently running workers finish.
func (m *Manager) WaitForRunning(ctx context.Context) {
	if m == nil {
		return
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.RunningCount() == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) RecentCompletedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recentCompleted
}

func (m *Manager) EnqueueNotification(n Notification) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, n)
}

func (m *Manager) DrainNotifications() []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := append([]Notification(nil), m.notifications...)
	m.notifications = nil
	return items
}

func (m *Manager) PendingNotifications() []Notification {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Notification(nil), m.notifications...)
}

func (m *Manager) ContextMessages() []chat.Message {
	if m == nil {
		return nil
	}
	notifications := m.DrainNotifications()
	var b strings.Builder
	if len(m.ListRoles()) > 0 {
		b.WriteString("<mewcode-worker-roles>\n")
		for _, role := range m.ListRoles() {
			fmt.Fprintf(&b, "- %s (source=%s): %s\n", role.Name, role.Source, role.Description)
		}
		b.WriteString("</mewcode-worker-roles>")
	}
	if len(notifications) == 0 {
		if b.Len() == 0 {
			return nil
		}
		return []chat.Message{prompt.InternalInstruction(b.String())}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("<mewcode-worker-notifications>\n")
	for _, n := range notifications {
		fmt.Fprintf(&b, "- id=%s role=%s status=%s result=%s error=%s\n", n.TaskID, n.Role, n.Status, n.Result, n.Error)
	}
	b.WriteString("</mewcode-worker-notifications>")
	return []chat.Message{prompt.InternalInstruction(b.String())}
}

func notificationText(task Task) string {
	if task.Status == StatusCompleted {
		return fmt.Sprintf("worker %s completed: %s", task.ID, task.Result)
	}
	return fmt.Sprintf("worker %s %s: %s", task.ID, task.Status, task.Error)
}

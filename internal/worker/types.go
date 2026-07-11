package worker

import (
	"context"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
)

const (
	ProjectDir        = ".mewcode/workers"
	UserDirName       = "workers"
	EntryFileName     = "WORKER.md"
	RunWorkerToolName = "run_worker"
	DefaultThreshold  = 10 * time.Second
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
	SourceProject Source = "project"
	SourcePlugin  Source = "plugin"
)

type PermissionMode string

const (
	PermissionDefault PermissionMode = "default"
	PermissionStrict  PermissionMode = "strict"
	PermissionAllow   PermissionMode = "allow"
)

type IsolationMode string

const (
	IsolationNone     IsolationMode = "none"
	IsolationWorktree IsolationMode = "worktree"
)

type Role struct {
	Name            string
	Description     string
	ToolsAllow      []string
	ToolsDeny       []string
	Model           string
	MaxIterations   int
	PermissionMode  PermissionMode
	BackgroundTools []string
	Isolation       IsolationMode
	Body            string
	Source          Source
	Path            string
}

type Options struct {
	EnableVerify        bool
	BackgroundThreshold time.Duration
}

type LoadResult struct {
	Roles    map[string]Role
	Warnings []string
}

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Task struct {
	ID           string
	RoleName     string
	Fork         bool
	Task         string
	Status       Status
	Result       string
	Error        string
	Notification string
	Usage        provider.Usage
	StartedAt    time.Time
	EndedAt      time.Time
	cancel       context.CancelFunc
}

type Notification struct {
	TaskID string
	Role   string
	Status string
	Result string
	Error  string
}

type RunRequest struct {
	Task           string
	TaskID         string
	RoleName       string
	Role           Role
	Fork           bool
	Background     bool
	Model          string
	MaxIterations  int
	Isolation      IsolationMode
	ParentMessages []chat.Message
}

type RunResult struct {
	Text  string
	Usage provider.Usage
	Error error
}

type ToolRunResult struct {
	OK         bool
	TaskID     string
	Background bool
	Status     Status
	Result     string
	Error      string
}

type RunnerFunc func(context.Context, RunRequest) RunResult

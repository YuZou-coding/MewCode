package team

import (
	"context"
	"time"

	"mewcode/internal/tool"
)

const (
	RootDir = ".mewcode/teams"

	ToolTaskCreate  = "team_task_create"
	ToolTaskGet     = "team_task_get"
	ToolTaskList    = "team_task_list"
	ToolTaskUpdate  = "team_task_update"
	ToolMessageSend = "team_message_send"
	ToolMemberStart = "team_member_start"
	ToolMemberStop  = "team_member_stop"
)

type Backend string

const (
	BackendInProcess    Backend = "in_process"
	BackendTerminalPane Backend = "terminal_pane"
)

type TeamStatus string

const (
	TeamStatusStopped TeamStatus = "stopped"
	TeamStatusRunning TeamStatus = "running"
)

type MemberStatus string

const (
	MemberStatusIdle    MemberStatus = "idle"
	MemberStatusRunning MemberStatus = "running"
	MemberStatusStopped MemberStatus = "stopped"
	MemberStatusFailed  MemberStatus = "failed"
)

type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusBlocked    TaskStatus = "blocked"
)

type MessageType string

const (
	MessageText          MessageType = "text"
	MessageSummary       MessageType = "summary"
	MessageLifecycle     MessageType = "lifecycle"
	MessageApprovalReply MessageType = "approval_reply"
)

type Options struct {
	DefaultBackend        Backend
	SchedulerAllowed      bool
	DefaultMemberApproval bool
}

type Team struct {
	Name     string     `json:"name"`
	Lead     string     `json:"lead"`
	Backend  Backend    `json:"backend"`
	Status   TeamStatus `json:"status"`
	Warnings []string   `json:"warnings,omitempty"`
	Root     string     `json:"-"`
}

type Member struct {
	Name             string       `json:"name"`
	Role             string       `json:"role"`
	InstanceID       string       `json:"instance_id"`
	Workdir          string       `json:"workdir"`
	Backend          Backend      `json:"backend"`
	RequiresApproval bool         `json:"requires_approval"`
	Status           MemberStatus `json:"status"`
	LastActiveAt     time.Time    `json:"last_active_at"`
	ResumeRef        string       `json:"resume_ref"`
}

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Assignee    string     `json:"assignee,omitempty"`
	Status      TaskStatus `json:"status"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type MailMessage struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Type      MessageType    `json:"type"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Event struct {
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type ActorKind string

const (
	ActorNone   ActorKind = ""
	ActorLead   ActorKind = "lead"
	ActorMember ActorKind = "member"
)

type Actor struct {
	Team string
	Name string
	Kind ActorKind
}

type Stats struct {
	ActiveTeam       string
	Lead             string
	RunningMembers   int
	PendingMessages  int
	IncompleteTasks  int
	SchedulerEnabled bool
	Warnings         int
}

type RunRequest struct {
	Team   string
	Member string
	Task   string
}

type RunResult struct {
	OK     bool
	Error  string
	Status MemberStatus
}

type RunnerFunc func(context.Context, RunRequest) RunResult

type BackendRunner interface {
	Start(context.Context, *Manager, string, string, string) RunResult
	Stop(context.Context, *Manager, string, string) RunResult
}

type ToolsProvider interface {
	TeamToolDefinitions() []tool.Definition
}

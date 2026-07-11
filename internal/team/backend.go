package team

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

type InProcessBackend struct{}

func (InProcessBackend) Start(ctx context.Context, m *Manager, teamName, memberName, task string) RunResult {
	member, err := m.ResolveMember(teamName, memberName)
	if err != nil {
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.running[teamName+"/"+member.InstanceID] = cancel
	m.mu.Unlock()
	if err := m.updateMemberStatus(teamName, member.InstanceID, MemberStatusRunning); err != nil {
		cancel()
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.running, teamName+"/"+member.InstanceID)
			m.mu.Unlock()
		}()
		result := RunResult{OK: true, Status: MemberStatusIdle}
		if m.Runner != nil {
			result = m.Runner(runCtx, RunRequest{Team: teamName, Member: member.Name, Task: task})
		} else {
			select {
			case <-time.After(10 * time.Millisecond):
			case <-runCtx.Done():
				result = RunResult{OK: false, Error: runCtx.Err().Error(), Status: MemberStatusStopped}
			}
		}
		status := MemberStatusIdle
		if !result.OK {
			status = MemberStatusFailed
		}
		_ = m.updateMemberStatus(teamName, member.InstanceID, status)
		_, _ = m.SendMessage(teamName, member.Name, "lead", MessageLifecycle, fmt.Sprintf("member %s finished with status %s", member.Name, status), "", map[string]any{"status": string(status), "error": result.Error})
	}()
	return RunResult{OK: true, Status: MemberStatusRunning}
}

func (InProcessBackend) Stop(ctx context.Context, m *Manager, teamName, memberName string) RunResult {
	member, err := m.ResolveMember(teamName, memberName)
	if err != nil {
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	m.mu.Lock()
	cancel := m.running[teamName+"/"+member.InstanceID]
	delete(m.running, teamName+"/"+member.InstanceID)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := m.updateMemberStatus(teamName, member.InstanceID, MemberStatusStopped); err != nil {
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	return RunResult{OK: true, Status: MemberStatusStopped}
}

type TerminalPaneBackend struct{}

func (TerminalPaneBackend) Start(context.Context, *Manager, string, string, string) RunResult {
	return RunResult{OK: false, Error: "terminal_pane backend is not supported in v1", Status: MemberStatusFailed}
}

func (TerminalPaneBackend) Stop(context.Context, *Manager, string, string) RunResult {
	return RunResult{OK: false, Error: "terminal_pane backend is not supported in v1", Status: MemberStatusFailed}
}

func (m *Manager) StartMember(ctx context.Context, teamName, memberName, task string) RunResult {
	member, err := m.ResolveMember(teamName, memberName)
	if err != nil {
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	return backendFor(member.Backend).Start(ctx, m, teamName, memberName, task)
}

func (m *Manager) StopMember(ctx context.Context, teamName, memberName string) RunResult {
	member, err := m.ResolveMember(teamName, memberName)
	if err != nil {
		return RunResult{OK: false, Error: err.Error(), Status: MemberStatusFailed}
	}
	return backendFor(member.Backend).Stop(ctx, m, teamName, memberName)
}

func (m *Manager) RunningMemberCount(teamName string) int {
	members, err := m.Members(teamName)
	if err != nil {
		return 0
	}
	count := 0
	for _, member := range members {
		if member.Status == MemberStatusRunning {
			count++
		}
	}
	return count
}

func (m *Manager) updateMemberStatus(teamName, instanceID string, status MemberStatus) error {
	members, err := m.Members(teamName)
	if err != nil {
		return err
	}
	for i := range members {
		if members[i].InstanceID == instanceID {
			members[i].Status = status
			members[i].LastActiveAt = time.Now()
			if members[i].ResumeRef == "" {
				members[i].ResumeRef = filepath.Join(teamName, instanceID)
			}
			return m.SaveMembers(teamName, members)
		}
	}
	return fmt.Errorf("member not found: %s", instanceID)
}

func backendFor(backend Backend) BackendRunner {
	switch backend {
	case BackendTerminalPane:
		return TerminalPaneBackend{}
	default:
		return InProcessBackend{}
	}
}

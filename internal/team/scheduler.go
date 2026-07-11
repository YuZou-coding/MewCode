package team

import (
	"fmt"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/prompt"
	"mewcode/internal/tool"
)

func IsTeamTool(name string) bool {
	return strings.HasPrefix(name, "team_")
}

func (m *Manager) FilterDefinitions(defs []tool.Definition, actor Actor) []tool.Definition {
	filtered := make([]tool.Definition, 0, len(defs))
	teamActor := actor.Kind == ActorLead || actor.Kind == ActorMember
	for _, def := range defs {
		if IsTeamTool(def.Name) {
			if teamActor {
				filtered = append(filtered, def)
			}
			continue
		}
		if actor.Kind == ActorLead && m.SchedulerEnabled() {
			continue
		}
		filtered = append(filtered, def)
	}
	return filtered
}

func (m *Manager) ContextMessages(actor Actor) []chat.Message {
	if actor.Kind != ActorLead && actor.Kind != ActorMember {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<mewcode-team-context team=%q actor=%q kind=%q>\n", actor.Team, actor.Name, actor.Kind)
	if actor.Kind == ActorLead && m.SchedulerEnabled() {
		b.WriteString("纯调度模式已开启。你只能理解目标、拆任务、派发成员、收敛结果、合并/上报；不要直接读写代码或执行 shell。\n")
		b.WriteString("阶段：理解目标 -> 拆任务 -> 派发 -> 收敛结果 -> 合并/上报。\n")
	}
	b.WriteString("</mewcode-team-context>")
	return []chat.Message{prompt.InternalInstruction(b.String())}
}

func (m *Manager) Stats() Stats {
	m.mu.Lock()
	active := m.activeTeam
	scheduler := m.schedulerEnabled && m.Options.SchedulerAllowed
	m.mu.Unlock()
	stats := Stats{ActiveTeam: active, SchedulerEnabled: scheduler, Warnings: m.WarningCount()}
	if active == "" {
		return stats
	}
	if team, err := m.Load(active); err == nil {
		stats.Lead = team.Lead
	}
	stats.RunningMembers = m.RunningMemberCount(active)
	stats.PendingMessages = m.PendingMessageCount(active)
	stats.IncompleteTasks = m.IncompleteTaskCount(active)
	return stats
}

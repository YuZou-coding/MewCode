package team

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) Members(teamName string) ([]Member, error) {
	team, err := m.Load(teamName)
	if err != nil {
		return nil, err
	}
	var members []Member
	if err := readJSON(filepath.Join(team.Root, "members.json"), &members); err != nil {
		return nil, err
	}
	return members, nil
}

func (m *Manager) SaveMembers(teamName string, members []Member) error {
	team, err := m.Load(teamName)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(team.Root, "members.json"), members)
}

func (m *Manager) ResolveMember(teamName, name string) (Member, error) {
	members, err := m.Members(teamName)
	if err != nil {
		return Member{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, member := range members {
		if strings.ToLower(member.Name) == needle || strings.ToLower(member.InstanceID) == needle {
			return member, nil
		}
	}
	return Member{}, fmt.Errorf("unknown team member: %s", name)
}

func (m *Manager) SendMessage(teamName, from, to string, msgType MessageType, content, summary string, payload map[string]any) ([]MailMessage, error) {
	if msgType == "" {
		msgType = MessageText
	}
	if strings.TrimSpace(content) == "" && strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("message content is required")
	}
	if to == "*" || strings.EqualFold(to, "all") || strings.EqualFold(to, "broadcast") {
		return m.Broadcast(teamName, from, msgType, content, summary, payload)
	}
	member, err := m.ResolveMember(teamName, to)
	if err != nil {
		return nil, err
	}
	msg := newMailMessage(from, member.Name, msgType, content, summary, payload)
	if err := m.appendMailbox(teamName, member.InstanceID, msg); err != nil {
		return nil, err
	}
	return []MailMessage{msg}, nil
}

func (m *Manager) Broadcast(teamName, from string, msgType MessageType, content, summary string, payload map[string]any) ([]MailMessage, error) {
	team, err := m.Load(teamName)
	if err != nil {
		return nil, err
	}
	members, err := m.Members(teamName)
	if err != nil {
		return nil, err
	}
	var sent []MailMessage
	for _, member := range members {
		if member.Name == team.Lead {
			continue
		}
		msg := newMailMessage(from, member.Name, msgType, content, summary, payload)
		if err := m.appendMailbox(teamName, member.InstanceID, msg); err != nil {
			return sent, err
		}
		sent = append(sent, msg)
	}
	return sent, nil
}

func (m *Manager) Mailbox(teamName, memberID string) ([]MailMessage, error) {
	team, err := m.Load(teamName)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(team.Root, "mailboxes", memberID+".jsonl")
	items, warnings, err := readJSONL[MailMessage](path)
	for _, warning := range warnings {
		m.addWarning(warning)
	}
	return items, err
}

func (m *Manager) PendingMessageCount(teamName string) int {
	members, err := m.Members(teamName)
	if err != nil {
		return 0
	}
	count := 0
	for _, member := range members {
		items, err := m.Mailbox(teamName, member.InstanceID)
		if err == nil {
			count += len(items)
		}
	}
	return count
}

func (m *Manager) appendMailbox(teamName, memberID string, msg MailMessage) error {
	team, err := m.Load(teamName)
	if err != nil {
		return err
	}
	return appendJSONL(filepath.Join(team.Root, "mailboxes", memberID+".jsonl"), msg)
}

func newMailMessage(from, to string, msgType MessageType, content, summary string, payload map[string]any) MailMessage {
	now := time.Now()
	return MailMessage{
		ID:        fmt.Sprintf("mail_%d", now.UnixNano()),
		From:      from,
		To:        to,
		Type:      msgType,
		Content:   content,
		Summary:   summary,
		Payload:   payload,
		CreatedAt: now,
	}
}

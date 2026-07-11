package memory

import (
	"context"
	"fmt"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
)

type Updater struct {
	Notes    Notes
	Provider provider.Provider
}

func (u *Updater) MaybeUpdate(ctx context.Context, turns int, messages []chat.Message) error {
	if turns > 0 && turns%NoteUpdateInterval != 0 {
		return nil
	}
	return u.Update(ctx, messages)
}

func (u *Updater) Update(ctx context.Context, messages []chat.Message) error {
	if u == nil || u.Provider == nil {
		return nil
	}
	prompt := u.prompt(messages)
	stream, errs := u.Provider.StreamChat(ctx, []chat.Message{{Role: chat.RoleUser, Content: prompt}}, nil)
	var text string
	for event := range stream {
		if event.Kind == provider.EventText {
			text += event.Text
		}
	}
	if err := <-errs; err != nil {
		return err
	}
	user, project := splitNotes(text)
	if strings.TrimSpace(user) != "" {
		if err := u.Notes.WriteUser(strings.TrimSpace(user) + "\n"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(project) != "" {
		if err := u.Notes.WriteProject(strings.TrimSpace(project) + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (u *Updater) prompt(messages []chat.Message) string {
	var recent []string
	start := len(messages) - 12
	if start < 0 {
		start = 0
	}
	for _, message := range messages[start:] {
		if strings.TrimSpace(message.Content) != "" {
			recent = append(recent, fmt.Sprintf("%s: %s", message.Role, message.Content))
		}
	}
	return "请更新 MewCode 笔记。不要调用任何工具。\n\n当前用户级笔记：\n" + u.Notes.ReadUser() +
		"\n\n当前项目级笔记：\n" + u.Notes.ReadProject() +
		"\n\n最近对话：\n" + strings.Join(recent, "\n") +
		"\n\n请只输出以下两个区块：\n## 用户级笔记\n- 用户偏好：\n- 纠正反馈：\n\n## 项目级笔记\n- 项目知识：\n- 参考资料："
}

func splitNotes(text string) (string, string) {
	userMarker := "## 用户级笔记"
	projectMarker := "## 项目级笔记"
	userIndex := strings.Index(text, userMarker)
	projectIndex := strings.Index(text, projectMarker)
	if userIndex == -1 && projectIndex == -1 {
		return text, ""
	}
	if userIndex == -1 {
		return "", strings.TrimSpace(text[projectIndex+len(projectMarker):])
	}
	if projectIndex == -1 {
		return strings.TrimSpace(text[userIndex+len(userMarker):]), ""
	}
	if userIndex < projectIndex {
		return strings.TrimSpace(text[userIndex+len(userMarker) : projectIndex]), strings.TrimSpace(text[projectIndex+len(projectMarker):])
	}
	return strings.TrimSpace(text[userIndex+len(userMarker):]), strings.TrimSpace(text[projectIndex+len(projectMarker) : userIndex])
}

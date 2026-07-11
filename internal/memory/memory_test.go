package memory

import (
	"context"
	"strings"
	"testing"

	"mewcode/internal/chat"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

type memoryProvider struct {
	toolsSeen [][]tool.Definition
}

func (p *memoryProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	copied := make([]tool.Definition, len(tools))
	copy(copied, tools)
	p.toolsSeen = append(p.toolsSeen, copied)
	events := make(chan provider.StreamEvent, 1)
	errs := make(chan error, 1)
	events <- provider.StreamEvent{Kind: provider.EventText, Text: "## 用户级笔记\n- 用户偏好：喜欢中文\n- 纠正反馈：不要假装执行\n\n## 项目级笔记\n- 项目知识：Go 项目\n- 参考资料：README.md"}
	close(events)
	errs <- nil
	return events, errs
}

func TestUpdaterWritesSplitNotesWithoutTools(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	fp := &memoryProvider{}
	updater := &Updater{Notes: Notes{HomeDir: home, ProjectRoot: project}, Provider: fp}
	if err := updater.Update(context.Background(), []chat.Message{{Role: chat.RoleUser, Content: "记住偏好"}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(fp.toolsSeen) != 1 || len(fp.toolsSeen[0]) != 0 {
		t.Fatalf("tools = %#v", fp.toolsSeen)
	}
	notes := Notes{HomeDir: home, ProjectRoot: project}
	if !strings.Contains(notes.ReadUser(), "喜欢中文") || strings.Contains(notes.ReadUser(), "Go 项目") {
		t.Fatalf("user notes = %q", notes.ReadUser())
	}
	if !strings.Contains(notes.ReadProject(), "Go 项目") || strings.Contains(notes.ReadProject(), "喜欢中文") {
		t.Fatalf("project notes = %q", notes.ReadProject())
	}
}

func TestNotesClearTargets(t *testing.T) {
	notes := Notes{HomeDir: t.TempDir(), ProjectRoot: t.TempDir()}
	_ = notes.WriteUser("user")
	_ = notes.WriteProject("project")
	if err := notes.Clear("user"); err != nil {
		t.Fatalf("clear user: %v", err)
	}
	if notes.ReadUser() != "" || notes.ReadProject() == "" {
		t.Fatalf("after user clear user=%q project=%q", notes.ReadUser(), notes.ReadProject())
	}
	if err := notes.Clear("all"); err != nil {
		t.Fatalf("clear all: %v", err)
	}
	if notes.ReadUser() != "" || notes.ReadProject() != "" {
		t.Fatalf("after all clear user=%q project=%q", notes.ReadUser(), notes.ReadProject())
	}
}

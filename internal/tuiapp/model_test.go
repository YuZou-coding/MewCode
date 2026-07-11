package tuiapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mewcode/internal/command"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
)

func TestModelCompletionAndStatusBar(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute", SessionID: "s1", MessageCount: 3}}, nil)
	model.input = "/co"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.InputValue() != "/compact" {
		t.Fatalf("input = %q", model.InputValue())
	}
	model.input = "/s"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if got := strings.Join(model.Candidates(), ","); got != "sessions,skills,status" {
		t.Fatalf("candidates = %q", got)
	}
	view := model.View()
	for _, want := range []string{"MewCode", "execute", "session s1", "msgs 3", "/help", "/compact", "/status"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestInputLineShowsCursor(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	if !strings.Contains(model.inputLine(), "❯ ") || !strings.Contains(model.inputLine(), "▌") {
		t.Fatalf("input line = %q", model.inputLine())
	}
	model.input = "你好"
	model.cursor = len([]rune(model.input))
	if !strings.Contains(model.inputLine(), "你好") || !strings.Contains(model.inputLine(), "▌") {
		t.Fatalf("input line with chinese = %q", model.inputLine())
	}
	model.busy = true
	if strings.Contains(model.inputLine(), "▌") || !strings.Contains(model.inputLine(), "working") {
		t.Fatalf("busy input line = %q", model.inputLine())
	}
}

func TestStatusBarFallsBackOnNarrowWidth(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "plan", SessionID: "session-with-a-long-id", MessageCount: 12}}, nil)
	model.width = 32
	line := model.statusLine()
	if !strings.Contains(line, "plan") || !strings.Contains(line, "m:12") {
		t.Fatalf("narrow status = %q", line)
	}
}

func TestModelShowsWelcomeCeremonyOnce(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	if err := command.RegisterSkillCommands(registry, []command.SkillCommand{{Name: "review", Description: "review skill"}}); err != nil {
		t.Fatalf("RegisterSkillCommands: %v", err)
	}
	var events <-chan StreamEvent
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute", SessionID: "s1", MessageCount: 0}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		events = streamFrom(StreamEvent{Kind: StreamTextDelta, Text: "hello"}, StreamEvent{Kind: StreamDone})
		return events
	})

	view := model.View()
	for _, want := range []string{"/\\_/\\", "MewCode", "Terminal AI coding agent", "/help", "/plan", "/compact", "/sessions", "/exit", "mode=execute", "session=s1", "messages=0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}

	model.input = "hello"
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("submit did not produce command")
	}
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	if got := strings.Count(model.View(), "/\\_/\\"); got != 1 {
		t.Fatalf("welcome rendered %d times: %q", got, model.View())
	}
}

func TestModelAcceptsChineseIMECommittedRunes(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("你好")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("，MewCode")})
	model = updated.(Model)

	if model.InputValue() != "你好，MewCode" {
		t.Fatalf("input = %q", model.InputValue())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.InputValue() != "你好，MewCod" {
		t.Fatalf("backspace input = %q", model.InputValue())
	}
}

func TestModelMovesCursorAndEditsChineseSafely(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("你好世界")})
	model = updated.(Model)
	for i := 0; i < 2; i++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
		model = updated.(Model)
	}
	if !strings.Contains(model.inputLine(), "你好▌世界") {
		t.Fatalf("cursor line = %q", model.inputLine())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("，")})
	model = updated.(Model)
	if model.InputValue() != "你好，世界" || !strings.Contains(model.inputLine(), "你好，▌世界") {
		t.Fatalf("input=%q line=%q", model.InputValue(), model.inputLine())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.InputValue() != "你好世界" || !strings.Contains(model.inputLine(), "你好▌世界") {
		t.Fatalf("after backspace input=%q line=%q", model.InputValue(), model.inputLine())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDelete})
	model = updated.(Model)
	if model.InputValue() != "你好界" || !strings.Contains(model.inputLine(), "你好▌界") {
		t.Fatalf("after delete input=%q line=%q", model.InputValue(), model.inputLine())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(Model)
	if !strings.Contains(model.inputLine(), "你好界▌") {
		t.Fatalf("end line = %q", model.inputLine())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = updated.(Model)
	if !strings.Contains(model.inputLine(), "❯ ▌你好界") {
		t.Fatalf("home line = %q", model.inputLine())
	}
}

func TestModelHelpAndReviewCommand(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	if err := command.RegisterSkillCommands(registry, []command.SkillCommand{{Name: "review", Description: "review skill"}}); err != nil {
		t.Fatalf("RegisterSkillCommands: %v", err)
	}
	var submitted string
	var events <-chan StreamEvent
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "plan", SessionID: "s2"}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		submitted = text
		events = streamFrom(StreamEvent{Kind: StreamTextDelta, Text: "review done"}, StreamEvent{Kind: StreamDone})
		return events
	})
	model.input = "/help"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "/compact") {
		t.Fatalf("transcript = %q", model.Transcript())
	}
	model.input = "/review internal/tui"
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("review did not produce submit command")
	}
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	if !strings.Contains(submitted, "internal/tui") || !strings.Contains(submitted, "skill:review") {
		t.Fatalf("submitted = %q", submitted)
	}
	if !strings.Contains(model.Transcript(), "review done") || !strings.Contains(model.View(), "plan") || !strings.Contains(model.View(), "session s2") {
		t.Fatalf("transcript/view = %q / %q", model.Transcript(), model.View())
	}
}

func TestModelIgnoresOrdinaryInputWhileBusy(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	submits := 0
	var events <-chan StreamEvent
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		submits++
		events = streamFrom(StreamEvent{Kind: StreamTextDelta, Text: "done"}, StreamEvent{Kind: StreamDone})
		return events
	})
	model.input = "first"
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil || !model.busy {
		t.Fatalf("expected busy submit")
	}
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("第二条")},
		{Type: tea.KeySpace},
		{Type: tea.KeyBackspace},
		{Type: tea.KeyTab},
		{Type: tea.KeyEnter},
	} {
		updated, _ = model.Update(msg)
		model = updated.(Model)
	}
	if model.InputValue() != "" || submits != 1 {
		t.Fatalf("input=%q submits=%d", model.InputValue(), submits)
	}
	if !strings.Contains(model.View(), "working") {
		t.Fatalf("view missing busy hint: %q", model.View())
	}
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	if model.busy || submits != 1 {
		t.Fatalf("busy=%v submits=%d", model.busy, submits)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("now")})
	model = updated.(Model)
	if model.InputValue() != "now" {
		t.Fatalf("input after response = %q", model.InputValue())
	}
}

func TestStreamingTextDeltasUpdateOneAssistantLine(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	events := streamFrom(
		StreamEvent{Kind: StreamTextDelta, Text: "hel"},
		StreamEvent{Kind: StreamTextDelta, Text: "lo"},
		StreamEvent{Kind: StreamDone},
	)
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		return events
	})
	model.input = "say hello"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	transcript := model.Transcript()
	if !strings.Contains(transcript, "first token in") {
		t.Fatalf("transcript missing first token timing: %q", transcript)
	}
	if strings.Count(transcript, "● hello") != 1 {
		t.Fatalf("transcript = %q", transcript)
	}
	if strings.Count(transcript, "● hel") != 1 {
		t.Fatalf("streaming line was split: %q", transcript)
	}
}

func TestStreamingThinkingIsCollapsed(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	events := streamFrom(
		StreamEvent{Kind: StreamThinking, Text: "raw chain of thought"},
		StreamEvent{Kind: StreamTextDelta, Text: "visible"},
		StreamEvent{Kind: StreamDone},
	)
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		return events
	})
	model.input = "think"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	if strings.Contains(model.Transcript(), "raw chain of thought") {
		t.Fatalf("thinking leaked to transcript: %q", model.Transcript())
	}
	if !strings.Contains(model.inputLine(), "thinking") {
		t.Fatalf("activity missing thinking phase: %q", model.inputLine())
	}
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "visible") {
		t.Fatalf("transcript = %q", model.Transcript())
	}
}

func TestIterationStatusAndPermissionEventStayOutOfTranscript(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.busy = true
	events := streamFrom(StreamEvent{Kind: StreamDone})
	updated, _ := model.Update(streamEventMsg{event: StreamEvent{Kind: StreamIteration, Iteration: 8, MaxIterations: 30}, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: StreamEvent{Kind: StreamPermissionRequest, ToolName: "read_file"}, events: events})
	model = updated.(Model)
	if !strings.Contains(model.statusLine(), "iteration 8/30") {
		t.Fatalf("status = %q", model.statusLine())
	}
	if strings.Contains(model.Transcript(), "permission required for") {
		t.Fatalf("permission request leaked into transcript: %q", model.Transcript())
	}
}

func TestStreamingToolEventsAreVisible(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.busy = true
	model.phase = "thinking"
	events := streamFrom(StreamEvent{Kind: StreamDone})
	updated, _ := model.Update(streamEventMsg{event: StreamEvent{Kind: StreamToolStart, ToolName: "read_file"}, events: events})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "● read_file running") || !strings.Contains(model.inputLine(), "tool") {
		t.Fatalf("tool start transcript/input = %q / %q", model.Transcript(), model.inputLine())
	}
	updated, _ = model.Update(streamEventMsg{event: StreamEvent{Kind: StreamToolResult, ToolName: "read_file"}, events: events})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "✓ read_file done") || !strings.Contains(model.inputLine(), "thinking") {
		t.Fatalf("tool result transcript/input = %q / %q", model.Transcript(), model.inputLine())
	}
}

func TestFailedToolExpandsReasonButSuccessfulToolStaysCompact(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.appendToolStart("ok", "read_file", "README.md")
	model.completeTool("ok", "read_file", ToolDone, "large successful output should stay hidden")
	model.appendToolStart("bad", "edit_file", "main.go")
	model.completeTool("bad", "edit_file", ToolBlocked, "permission denied\nproject rule matched\ntry another path\nextra line")

	text := model.Transcript()
	if strings.Contains(text, "large successful output") {
		t.Fatalf("successful result expanded:\n%s", text)
	}
	for _, want := range []string{"README.md", "main.go", "permission denied", "project rule matched", "try another path", "…"} {
		if !strings.Contains(text, want) {
			t.Fatalf("transcript missing %q:\n%s", want, text)
		}
	}
}

func TestActivityTickKeepsBusyStateLive(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.busy = true
	model.phase = "thinking"
	model.phaseStartedAt = time.Now()
	updated, cmd := model.Update(activityTickMsg(time.Now()))
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("busy tick did not schedule another tick")
	}
	if !strings.Contains(model.inputLine(), "working") || !strings.Contains(model.inputLine(), "thinking") {
		t.Fatalf("busy input line = %q", model.inputLine())
	}
	model.busy = false
	_, cmd = model.Update(activityTickMsg(time.Now()))
	if cmd != nil {
		t.Fatalf("idle tick scheduled command")
	}
}

func TestModelPermissionPromptWaitsForUserChoice(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	choiceSeen := make(chan permissions.HITLChoice, 1)
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	go func() {
		choiceSeen <- model.permissionPrompt()(context.Background(), permissions.Request{Tool: "edit_file", Arguments: []byte(`{"path":"web/snake/index.html"}`)}, permissions.Ask("needs approval"))
	}()
	updated, _ := model.Update(model.Init()())
	model = updated.(Model)
	if !strings.Contains(model.View(), "Permission required") || !strings.Contains(model.View(), "path: web/snake/index.html") {
		t.Fatalf("view missing permission prompt: %q", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(Model)
	if got := <-choiceSeen; got != permissions.HITLAllowOnce {
		t.Fatalf("choice = %s", got)
	}
}

func TestStructuredTranscriptRendersBlocks(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	events := streamFrom(
		StreamEvent{Kind: StreamTextDelta, Text: "hello"},
		StreamEvent{Kind: StreamToolStart, ToolName: "read_file"},
		StreamEvent{Kind: StreamToolResult, ToolName: "read_file"},
		StreamEvent{Kind: StreamUsage, Usage: provider.Usage{InputTokens: 3, OutputTokens: 4}},
		StreamEvent{Kind: StreamDone},
	)
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, func(ctx context.Context, text string, prompt PermissionPromptFunc) <-chan StreamEvent {
		return events
	})
	model.input = "hi"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	text := model.Transcript()
	for _, want := range []string{"❯ hi", "● hello", "● read_file running"} {
		if !strings.Contains(text, want) {
			t.Fatalf("transcript missing %q:\n%s", want, text)
		}
	}
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	updated, _ = model.Update(streamEventMsg{event: <-events, events: events})
	model = updated.(Model)
	text = model.Transcript()
	for _, want := range []string{"✓ read_file done", "tokens in=3 out=4"} {
		if !strings.Contains(text, want) {
			t.Fatalf("transcript missing %q:\n%s", want, text)
		}
	}
}

func TestClaudeStyleTranscriptUsesCompactMarkers(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.appendUser("检查 README")
	model.appendAssistantDelta("已检查。")

	transcript := model.Transcript()
	if !strings.Contains(transcript, "❯ 检查 README") {
		t.Fatalf("transcript missing compact user marker:\n%s", transcript)
	}
	if !strings.Contains(transcript, "● 已检查。") {
		t.Fatalf("transcript missing compact assistant marker:\n%s", transcript)
	}
	if strings.Contains(transcript, "You >") || strings.Contains(transcript, "MewCode >") {
		t.Fatalf("legacy prefixes remain:\n%s", transcript)
	}
}

func TestCommandPanelKeyboardSelectionAndDismissal(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.input = "/wo"
	model.cursor = len([]rune(model.input))
	model.syncCommandPanel()
	first := model.commandSelection
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.commandSelection == first {
		t.Fatalf("down did not change command selection")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.input != "/wo" || !model.commandPanelDismissed {
		t.Fatalf("escape input=%q dismissed=%v", model.input, model.commandPanelDismissed)
	}
}

func TestCommandPanelEnterSelectsWithoutSubmitting(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/wo")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.InputValue() != "/workers " || strings.Contains(model.Transcript(), "workers") {
		t.Fatalf("selection input=%q transcript=%q", model.InputValue(), model.Transcript())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "workers") {
		t.Fatalf("second enter did not execute:\n%s", model.Transcript())
	}
}

func TestCommandPanelSelectionLeavesArgumentCursorReady(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.input = "/res"
	model.cursor = len([]rune(model.input))
	model.syncCommandPanel()
	for index, item := range model.commandItems {
		if item.Name == "resume" {
			model.commandSelection = index
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.InputValue() != "/resume " || model.cursor != len([]rune("/resume ")) {
		t.Fatalf("input=%q cursor=%d", model.InputValue(), model.cursor)
	}
}

func TestPermissionChoiceLeavesTranscriptRecord(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.pendingPermission = &permissionPromptMsg{
		request: permissions.Request{Tool: "edit_file", Arguments: []byte(`{"path":"README.md"}`)},
		respond: make(chan permissions.HITLChoice, 1),
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(Model)
	if !strings.Contains(model.Transcript(), "Allowed edit_file once") {
		t.Fatalf("permission record missing:\n%s", model.Transcript())
	}
}

func TestViewportPausesFollowingAndReportsNewOutput(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.viewport.Height = 4
	for i := 0; i < 20; i++ {
		model.appendSystem(fmt.Sprintf("line %d", i))
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	offset := model.viewport.YOffset
	model.appendSystem("new line")
	if model.viewport.YOffset != offset || !model.newOutput || !strings.Contains(model.View(), "New output") {
		t.Fatalf("offset=%d want=%d newOutput=%v view=%q", model.viewport.YOffset, offset, model.newOutput, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(Model)
	if !model.followOutput || model.newOutput || !model.viewport.AtBottom() {
		t.Fatalf("follow=%v new=%v atBottom=%v", model.followOutput, model.newOutput, model.viewport.AtBottom())
	}
}

func TestResponsiveLayoutDoesNotOverflow(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	for _, width := range []int{40, 80, 120} {
		controller := &fakeAppController{state: command.State{Mode: "execute", SessionID: "long-session-id", MessageCount: 12, GitBranch: "feature/tui", ContextPercent: 42}}
		model := New(context.Background(), registry, controller, nil)
		model.width = width
		model.height = 30
		model.viewport.Width = width
		model.refresh()
		model.pendingPermission = &permissionPromptMsg{
			request:  permissions.Request{Tool: "run_command", Arguments: []byte(`{"command":"a very long command with many arguments and a path /tmp/example/that/keeps/going"}`)},
			decision: permissions.Ask("a long reason that must remain inside the terminal width"),
		}
		for _, line := range strings.Split(model.View(), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width=%d line width=%d: %q", width, got, line)
			}
		}
	}
}

func TestCommandPanelShowsAndFiltersCommands(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{"Commands", "/help", "/plan", "/compact"} {
		if !strings.Contains(view, want) {
			t.Fatalf("command panel missing %q:\n%s", want, view)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("wo")})
	model = updated.(Model)
	panel := model.commandPanel()
	if !strings.Contains(panel, "/workers") || !strings.Contains(panel, "/worktrees") || strings.Contains(panel, "/sessions") {
		t.Fatalf("filtered command panel = %q", panel)
	}
}

func TestCommandPanelUsesBorderWithoutBackgroundFill(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.input = "/"
	model.syncCommandPanel()
	panel := model.commandPanel()
	if strings.Contains(panel, "\x1b[48;") {
		t.Fatalf("command panel contains background fill: %q", panel)
	}
	for _, line := range strings.Split(panel, "\n") {
		if !strings.Contains(line, "│") {
			t.Fatalf("command line missing border: %q", line)
		}
	}
}

func TestFullscreenTUIStylesDoNotDeclareBackgroundColors(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(raw), ".Background(") {
			t.Fatalf("%s declares a fixed background color", entry.Name())
		}
	}
}

func TestPermissionPanelIsStructuredAndBlocksOrdinaryInput(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	choiceSeen := make(chan permissions.HITLChoice, 1)
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	go func() {
		choiceSeen <- model.permissionPrompt()(context.Background(), permissions.Request{Tool: "run_command", Arguments: []byte(`{"command":"rm -rf tmp"}`)}, permissions.Ask("dangerous command"))
	}()
	updated, _ := model.Update(model.Init()())
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{"Permission required", "tool: run_command", "command: rm -rf tmp", "reason: dangerous command", "n deny", "y once", "s session", "a always"} {
		if !strings.Contains(view, want) {
			t.Fatalf("permission panel missing %q:\n%s", want, view)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("普通输入")})
	model = updated.(Model)
	if model.InputValue() != "" {
		t.Fatalf("ordinary input entered while permission pending: %q", model.InputValue())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model = updated.(Model)
	if got := <-choiceSeen; got != permissions.HITLAllowSession {
		t.Fatalf("choice = %s", got)
	}
}

func TestPermissionPanelUsesBorderWithoutBackgroundFill(t *testing.T) {
	registry, err := command.Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	model := New(context.Background(), registry, &fakeAppController{state: command.State{Mode: "execute"}}, nil)
	model.pendingPermission = &permissionPromptMsg{
		request:  permissions.Request{Tool: "edit_file", Arguments: []byte(`{"path":"README.md"}`)},
		decision: permissions.Ask("file write requires approval"),
	}
	panel := model.permissionLine()
	if strings.Contains(panel, "\x1b[48;") {
		t.Fatalf("permission panel contains background fill: %q", panel)
	}
	for _, line := range strings.Split(panel, "\n") {
		if !strings.Contains(line, "│") {
			t.Fatalf("permission line missing warning border: %q", line)
		}
	}
}

type fakeAppController struct {
	state command.State
}

func (f *fakeAppController) ShowSystemMessage(message string)                       {}
func (f *fakeAppController) SendUserMessage(ctx context.Context, text string) error { return nil }
func (f *fakeAppController) SetPlanMode(enabled bool) {
	if enabled {
		f.state.Mode = "plan"
	} else {
		f.state.Mode = "execute"
	}
}
func (f *fakeAppController) PlanMode() bool           { return f.state.Mode == "plan" }
func (f *fakeAppController) ClearConversation() error { f.state.MessageCount = 0; return nil }
func (f *fakeAppController) Status() command.State {
	if f.state.Mode == "" {
		f.state.Mode = "execute"
	}
	f.state.LastUsage = provider.Usage{InputTokens: 7, OutputTokens: 9}
	return f.state
}
func (f *fakeAppController) Compact(ctx context.Context) string { return "compacted" }
func (f *fakeAppController) ListSessions() string               { return "sessions" }
func (f *fakeAppController) ResumeSession(ctx context.Context, id string) string {
	return "resumed " + id
}
func (f *fakeAppController) Notes(command string, args string) string { return "notes" }
func (f *fakeAppController) Permissions(command string) string        { return "permissions" }
func (f *fakeAppController) Skills(ctx context.Context, args string) string {
	return "skills"
}
func (f *fakeAppController) Workers(ctx context.Context, args string) string {
	return "workers"
}
func (f *fakeAppController) Worktrees(ctx context.Context, args string) string {
	return "worktrees"
}
func (f *fakeAppController) Teams(ctx context.Context, args string) string {
	return "teams"
}
func (f *fakeAppController) RunSkill(ctx context.Context, name string, args string) (string, error) {
	return "skill:" + name + ":" + args, nil
}

func streamFrom(events ...StreamEvent) <-chan StreamEvent {
	ch := make(chan StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

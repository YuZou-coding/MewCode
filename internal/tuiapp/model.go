package tuiapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"mewcode/internal/command"
	"mewcode/internal/permissions"
	"mewcode/internal/provider"
)

type PermissionPromptFunc func(context.Context, permissions.Request, permissions.Decision) permissions.HITLChoice
type SubmitFunc func(context.Context, string, PermissionPromptFunc) <-chan StreamEvent

type StreamKind string

const (
	StreamThinking          StreamKind = "thinking"
	StreamTextDelta         StreamKind = "text_delta"
	StreamToolStart         StreamKind = "tool_start"
	StreamToolResult        StreamKind = "tool_result"
	StreamPermissionRequest StreamKind = "permission_request"
	StreamUsage             StreamKind = "usage"
	StreamError             StreamKind = "error"
	StreamDone              StreamKind = "done"
	StreamIteration         StreamKind = "iteration"
)

type StreamEvent struct {
	Kind          StreamKind
	Text          string
	CallID        string
	ToolName      string
	Target        string
	ToolOK        bool
	ToolCode      string
	Iteration     int
	MaxIterations int
	Usage         provider.Usage
	Err           error
}

type BlockKind string

const (
	BlockWelcome    BlockKind = "welcome"
	BlockUser       BlockKind = "user"
	BlockAssistant  BlockKind = "assistant"
	BlockThinking   BlockKind = "thinking"
	BlockTool       BlockKind = "tool"
	BlockUsage      BlockKind = "usage"
	BlockError      BlockKind = "error"
	BlockSystem     BlockKind = "system"
	BlockPermission BlockKind = "permission"
)

type ToolBlockState string

const (
	ToolRunning ToolBlockState = "running"
	ToolDone    ToolBlockState = "done"
	ToolFailed  ToolBlockState = "failed"
	ToolBlocked ToolBlockState = "blocked"
)

type TranscriptBlock struct {
	Kind      BlockKind
	Text      string
	ToolName  string
	CallID    string
	Target    string
	Detail    string
	ToolState ToolBlockState
	StartedAt time.Time
	Duration  time.Duration
	Usage     provider.Usage
}

type Model struct {
	ctx                   context.Context
	registry              *command.Registry
	controller            command.Controller
	submit                SubmitFunc
	input                 string
	cursor                int
	viewport              viewport.Model
	width                 int
	height                int
	blocks                []TranscriptBlock
	candidates            []string
	err                   error
	permissionRequests    chan permissionPromptMsg
	pendingPermission     *permissionPromptMsg
	busy                  bool
	streamingBlock        int
	turnStartedAt         time.Time
	phaseStartedAt        time.Time
	phase                 string
	firstTokenShown       bool
	commandItems          []command.PanelItem
	commandSelection      int
	commandPanelDismissed bool
	followOutput          bool
	newOutput             bool
	iteration             int
	maxIterations         int
}

type permissionPromptMsg struct {
	request  permissions.Request
	decision permissions.Decision
	respond  chan permissions.HITLChoice
}

type streamEventMsg struct {
	event  StreamEvent
	events <-chan StreamEvent
}

type streamDoneMsg struct{}

type activityTickMsg time.Time

func New(ctx context.Context, registry *command.Registry, controller command.Controller, submit SubmitFunc) Model {
	view := viewport.New(80, 20)
	model := Model{ctx: ctx, registry: registry, controller: controller, submit: submit, viewport: view, width: 80, height: 24, permissionRequests: make(chan permissionPromptMsg), streamingBlock: -1, followOutput: true}
	model.blocks = append(model.blocks, TranscriptBlock{Kind: BlockWelcome, Text: welcomeText(controller.Status())})
	model.refresh()
	return model
}

func (m Model) Init() tea.Cmd { return waitPermission(m.permissionRequests) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.viewport.Width = v.Width
		m.viewport.Height = max(1, v.Height-4)
		m.refresh()
		return m, nil
	case streamEventMsg:
		return m.handleStreamEvent(v.event, v.events)
	case streamDoneMsg:
		m.busy = false
		m.phase = ""
		m.streamingBlock = -1
		return m, nil
	case activityTickMsg:
		if m.busy {
			return m, activityTick()
		}
		return m, nil
	case permissionPromptMsg:
		m.pendingPermission = &v
		return m, nil
	case tea.MouseMsg:
		return m.updateViewport(v)
	case tea.KeyMsg:
		if m.pendingPermission != nil {
			return m.handlePermissionKey(v)
		}
		if m.commandPanelVisible() {
			switch v.Type {
			case tea.KeyEsc:
				m.commandPanelDismissed = true
				return m, nil
			case tea.KeyDown:
				m.moveCommandSelection(1)
				return m, nil
			case tea.KeyUp:
				m.moveCommandSelection(-1)
				return m, nil
			case tea.KeyEnter:
				m.applySelectedCommand()
				m.commandPanelDismissed = true
				return m, nil
			}
		}
		switch v.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
		if m.busy {
			switch v.Type {
			case tea.KeyPgUp:
				m.viewport.PageUp()
				m.followOutput = false
				return m, nil
			case tea.KeyPgDown:
				m.viewport.PageDown()
				if m.viewport.AtBottom() {
					m.followOutput = true
					m.newOutput = false
				}
				return m, nil
			case tea.KeyEnd:
				m.viewport.GotoBottom()
				m.followOutput = true
				m.newOutput = false
				return m, nil
			}
			return m, nil
		}
		switch v.Type {
		case tea.KeyRunes:
			if len(v.Runes) > 0 {
				m.insertRunes(v.Runes)
				m.candidates = nil
				m.commandPanelDismissed = false
				m.syncCommandPanel()
			}
			return m, nil
		case tea.KeyTab:
			m.complete()
			return m, nil
		case tea.KeyEnter:
			return m.submitInput()
		case tea.KeyBackspace:
			m.backspace()
			m.commandPanelDismissed = false
			m.syncCommandPanel()
			return m, nil
		case tea.KeyDelete:
			m.deleteForward()
			m.commandPanelDismissed = false
			m.syncCommandPanel()
			return m, nil
		case tea.KeyLeft:
			m.moveCursor(-1)
			return m, nil
		case tea.KeyRight:
			m.moveCursor(1)
			return m, nil
		case tea.KeyHome, tea.KeyCtrlA:
			m.cursor = 0
			return m, nil
		case tea.KeyEnd, tea.KeyCtrlE:
			if m.input == "" && !m.followOutput {
				m.viewport.GotoBottom()
				m.followOutput = true
				m.newOutput = false
				return m, nil
			}
			m.cursor = len([]rune(m.input))
			return m, nil
		case tea.KeyPgUp:
			m.viewport.PageUp()
			m.followOutput = false
			return m, nil
		case tea.KeyPgDown:
			m.viewport.PageDown()
			if m.viewport.AtBottom() {
				m.followOutput = true
				m.newOutput = false
			}
			return m, nil
		case tea.KeySpace:
			m.insertRunes([]rune(" "))
			m.commandPanelDismissed = false
			m.syncCommandPanel()
			return m, nil
		}
		if s := v.String(); len([]rune(s)) == 1 && s >= " " {
			m.insertRunes([]rune(s))
			m.candidates = nil
			m.commandPanelDismissed = false
			m.syncCommandPanel()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateViewport(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.viewport.YOffset
	viewportModel, cmd := m.viewport.Update(msg)
	m.viewport = viewportModel
	if m.viewport.YOffset != before {
		if m.viewport.AtBottom() {
			m.followOutput = true
			m.newOutput = false
		} else {
			m.followOutput = false
		}
	}
	return m, cmd
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input)
	m.input = ""
	m.cursor = 0
	m.candidates = nil
	m.commandItems = nil
	m.commandSelection = 0
	m.commandPanelDismissed = false
	if text == "" {
		return *m, nil
	}
	m.appendUser(text)
	result := command.Dispatch(m.ctx, m.registry, m.controller, text)
	for _, message := range result.Messages {
		m.appendSystem(message)
	}
	if result.Err != nil {
		m.appendError("command error: " + result.Err.Error())
	}
	if result.Exit {
		return *m, tea.Quit
	}
	if result.SendToAgent == "" && command.Parse(text).IsCommand {
		return *m, nil
	}
	toSend := text
	if result.SendToAgent != "" {
		toSend = result.SendToAgent
	}
	m.appendThinking()
	m.busy = true
	m.streamingBlock = -1
	m.turnStartedAt = time.Now()
	m.phaseStartedAt = m.turnStartedAt
	m.phase = "thinking"
	m.firstTokenShown = false
	events := (<-chan StreamEvent)(nil)
	if m.submit == nil {
		events = singleStreamEvent(StreamEvent{Kind: StreamError, Err: fmt.Errorf("submit handler is not configured")})
	} else {
		events = m.submit(m.ctx, toSend, m.permissionPrompt())
	}
	return *m, tea.Batch(waitStream(events), activityTick())
}

func singleStreamEvent(event StreamEvent) <-chan StreamEvent {
	ch := make(chan StreamEvent, 1)
	ch <- event
	close(ch)
	return ch
}

func waitStream(events <-chan StreamEvent) tea.Cmd {
	return func() tea.Msg {
		if events == nil {
			return streamDoneMsg{}
		}
		event, ok := <-events
		if !ok {
			return streamDoneMsg{}
		}
		return streamEventMsg{event: event, events: events}
	}
}

func activityTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return activityTickMsg(t)
	})
}

func (m Model) handleStreamEvent(event StreamEvent, events <-chan StreamEvent) (tea.Model, tea.Cmd) {
	switch event.Kind {
	case StreamThinking:
		if m.phase != "thinking" {
			m.phaseStartedAt = time.Now()
		}
		m.phase = "thinking"
	case StreamTextDelta:
		if !m.firstTokenShown {
			m.appendSystem(fmt.Sprintf("first token in %s", formatDuration(time.Since(m.turnStartedAt))))
			m.firstTokenShown = true
		}
		m.phase = "responding"
		m.appendAssistantDelta(event.Text)
	case StreamToolStart:
		m.phase = "tool"
		m.phaseStartedAt = time.Now()
		m.streamingBlock = -1
		m.appendToolStart(event.CallID, event.ToolName, event.Target)
	case StreamToolResult:
		m.phase = "thinking"
		m.phaseStartedAt = time.Now()
		m.streamingBlock = -1
		state := ToolDone
		if !event.ToolOK && (event.ToolCode != "" || event.Text != "") {
			state = ToolFailed
			if event.ToolCode == "permission_denied" || event.ToolCode == "hook_blocked" || event.ToolCode == "plan_only_blocked" {
				state = ToolBlocked
			}
		}
		m.completeTool(event.CallID, event.ToolName, state, event.Text)
	case StreamPermissionRequest:
		// The live permission panel owns this state; only the final decision is persisted in the transcript.
	case StreamIteration:
		m.iteration = event.Iteration
		m.maxIterations = event.MaxIterations
	case StreamUsage:
		m.appendUsage(event.Usage)
	case StreamError:
		m.busy = false
		m.phase = ""
		if event.Err != nil {
			m.appendError(event.Err.Error())
		}
		return m, nil
	case StreamDone:
		m.busy = false
		m.phase = ""
		m.streamingBlock = -1
		return m, nil
	}
	return m, waitStream(events)
}

func (m Model) permissionPrompt() PermissionPromptFunc {
	return func(ctx context.Context, request permissions.Request, decision permissions.Decision) permissions.HITLChoice {
		respond := make(chan permissions.HITLChoice, 1)
		msg := permissionPromptMsg{request: request, decision: decision, respond: respond}
		select {
		case <-ctx.Done():
			return permissions.HITLDeny
		case m.permissionRequests <- msg:
		}
		select {
		case <-ctx.Done():
			return permissions.HITLDeny
		case choice := <-respond:
			return choice
		}
	}
}

func (m Model) handlePermissionKey(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	choice := permissions.HITLDeny
	answered := true
	switch v.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		choice = permissions.HITLDeny
	case tea.KeyEnter:
		choice = permissions.HITLDeny
	default:
		switch strings.ToLower(v.String()) {
		case "y":
			choice = permissions.HITLAllowOnce
		case "s":
			if m.pendingPermission != nil && m.pendingPermission.decision.Mode == permissions.ModeStrict {
				answered = false
				break
			}
			choice = permissions.HITLAllowSession
		case "a":
			if m.pendingPermission != nil && m.pendingPermission.decision.Mode == permissions.ModeStrict {
				answered = false
				break
			}
			choice = permissions.HITLAllowAlways
		case "n":
			choice = permissions.HITLDeny
		default:
			answered = false
		}
	}
	if !answered {
		return m, nil
	}
	pending := m.pendingPermission
	m.pendingPermission = nil
	if pending != nil {
		m.appendPermissionDecision(pending.request.Tool, choice)
		pending.respond <- choice
	}
	return m, waitPermission(m.permissionRequests)
}

func (m *Model) complete() {
	completion := m.registry.Complete(m.input)
	if completion.Replacement != "" {
		m.input = completion.Replacement
		m.cursor = len([]rune(m.input))
		m.candidates = nil
		return
	}
	m.candidates = completion.Candidates
}

func (m *Model) insertRunes(inserted []rune) {
	runes := []rune(m.input)
	m.clampCursor(len(runes))
	next := make([]rune, 0, len(runes)+len(inserted))
	next = append(next, runes[:m.cursor]...)
	next = append(next, inserted...)
	next = append(next, runes[m.cursor:]...)
	m.input = string(next)
	m.cursor += len(inserted)
}

func (m *Model) backspace() {
	runes := []rune(m.input)
	m.clampCursor(len(runes))
	if len(runes) == 0 || m.cursor == 0 {
		return
	}
	next := append([]rune{}, runes[:m.cursor-1]...)
	next = append(next, runes[m.cursor:]...)
	m.input = string(next)
	m.cursor--
	m.candidates = nil
}

func (m *Model) deleteForward() {
	runes := []rune(m.input)
	m.clampCursor(len(runes))
	if len(runes) == 0 || m.cursor >= len(runes) {
		return
	}
	next := append([]rune{}, runes[:m.cursor]...)
	next = append(next, runes[m.cursor+1:]...)
	m.input = string(next)
	m.candidates = nil
}

func (m *Model) moveCursor(delta int) {
	runes := []rune(m.input)
	m.cursor += delta
	m.clampCursor(len(runes))
}

func (m *Model) clampCursor(length int) {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > length {
		m.cursor = length
	}
}

func waitPermission(ch <-chan permissionPromptMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m Model) InputValue() string   { return m.input }
func (m Model) Transcript() string   { return m.renderTranscript(false) }
func (m Model) Candidates() []string { return append([]string(nil), m.candidates...) }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

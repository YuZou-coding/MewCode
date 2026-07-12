package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/compact"
	"mewcode/internal/hooks"
	"mewcode/internal/permissions"
	"mewcode/internal/prompt"
	"mewcode/internal/provider"
	"mewcode/internal/skill"
	"mewcode/internal/team"
	"mewcode/internal/tool"
	"mewcode/internal/worker"
)

const (
	EventBufferSize      = 64
	DefaultMaxIterations = 30
)

type ConfirmToolFunc func(ctx context.Context, call provider.ToolCall) bool
type ConfirmCommandFunc func(ctx context.Context, command string) bool
type PermissionPromptFunc func(ctx context.Context, request permissions.Request, decision permissions.Decision) permissions.HITLChoice
type PreToolHook func(ctx context.Context, call provider.ToolCall) (bool, string)
type PostToolHook func(ctx context.Context, call provider.ToolCall, result tool.Result) error

type Agent struct {
	Provider          provider.Provider
	Registry          *tool.Registry
	Session           *chat.Session
	Tools             []tool.Definition
	MaxIterations     int
	PlanOnly          bool
	ToolTimeout       time.Duration
	ConfirmTool       ConfirmToolFunc
	ConfirmCommand    ConfirmCommandFunc
	PermissionChecker *permissions.Checker
	PermissionPrompt  PermissionPromptFunc
	PreToolHook       PreToolHook
	PostToolHook      PostToolHook
	Compact           *compact.Manager
	ContextMessages   []chat.Message
	SkillManager      *skill.Manager
	HookEngine        *hooks.Engine
	WorkerManager     *worker.Manager
	TeamManager       *team.Manager
	TeamActor         team.Actor
}

func (a *Agent) Run(ctx context.Context, userText string) <-chan Event {
	events := make(chan Event, EventBufferSize)
	go func() {
		defer close(events)
		a.run(ctx, userText, events)
	}()
	return events
}

func (a *Agent) run(ctx context.Context, userText string, events chan<- Event) {
	if a.Provider == nil {
		sendEvent(ctx, events, Event{Kind: EventError, Error: errors.New("provider is required")})
		return
	}
	session := a.Session
	if session == nil {
		session = chat.NewSession()
		a.Session = session
	}

	if !sendEvent(ctx, events, Event{Kind: EventUserMessage, Text: userText}) {
		return
	}
	a.fireHook(ctx, hooks.Context{Event: hooks.EventSessionStart})
	session.AddUser(userText)
	if a.WorkerManager != nil {
		a.WorkerManager.SetParentMessages(session.Messages())
	}

	maxIterations := a.MaxIterations
	if maxIterations == 0 {
		maxIterations = DefaultMaxIterations
	}
	var finalText string
	for iteration := 0; ; iteration++ {
		a.fireHook(ctx, hooks.Context{Event: hooks.EventTurnStart, MessageContent: userText})
		if ctx.Err() != nil {
			return
		}
		if iteration >= maxIterations {
			a.finishAtIterationLimit(ctx, session, maxIterations, events)
			return
		}
		sendEvent(ctx, events, Event{Kind: EventIteration, Iteration: iteration + 1, MaxIterations: maxIterations})

		if a.Compact != nil {
			a.fireHook(ctx, hooks.Context{Event: hooks.EventCompactBefore})
			result := a.Compact.CompactBeforeRequest(ctx, session.Messages())
			session.ReplaceMessages(result.Messages)
			for _, err := range result.Stats.Errors {
				a.fireHook(ctx, hooks.Context{Event: hooks.EventSystemError, Error: err.Error()})
				sendEvent(ctx, events, Event{Kind: EventError, Error: err})
			}
			a.fireHook(ctx, hooks.Context{Event: hooks.EventCompactAfter})
		}
		a.fireHook(ctx, hooks.Context{Event: hooks.EventMessageBeforeSend, MessageContent: userText})
		messages := a.messagesForTurn(ctx, session.Messages(), iteration+1)
		assistantText, calls, err := a.streamOneTurn(ctx, messages, a.toolsForTurn(), events)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.fireHook(ctx, hooks.Context{Event: hooks.EventSystemError, Error: err.Error()})
			sendEvent(ctx, events, Event{Kind: EventError, Error: err})
			return
		}
		a.fireHook(ctx, hooks.Context{Event: hooks.EventMessageAfterRecv, MessageContent: assistantText})
		finalText = assistantText
		if len(calls) == 0 {
			if ctx.Err() != nil {
				return
			}
			session.AddAssistant(assistantText)
			a.fireHook(ctx, hooks.Context{Event: hooks.EventTurnEnd, MessageContent: assistantText})
			a.fireHook(ctx, hooks.Context{Event: hooks.EventSessionEnd})
			sendEvent(ctx, events, Event{Kind: EventFinalResponse, Text: assistantText})
			return
		}

		chatCalls := make([]chat.ToolCall, 0, len(calls))
		for _, call := range calls {
			chatCalls = append(chatCalls, chat.ToolCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments)})
		}
		session.AddAssistantToolCalls(assistantText, chatCalls)
		if a.WorkerManager != nil {
			a.WorkerManager.SetParentMessages(session.Messages())
		}

		results := a.executeToolBatch(ctx, calls, events)
		if ctx.Err() != nil {
			return
		}
		for _, item := range results {
			session.AddToolResult(chat.ToolResult{
				CallID:  item.Call.ID,
				Name:    item.Call.Name,
				Content: mustMarshal(item.Result),
			})
		}
		a.fireHook(ctx, hooks.Context{Event: hooks.EventTurnEnd, MessageContent: assistantText})
		_ = finalText
	}
}

func (a *Agent) finishAtIterationLimit(ctx context.Context, session *chat.Session, maxIterations int, events chan<- Event) {
	messages := a.messagesForTurn(ctx, session.Messages(), maxIterations+1)
	messages = append(messages, chat.Message{Role: chat.RoleSystem, Content: "The agent reached its tool iteration limit. Do not call any tools. Briefly report: completed work, unfinished work, failures, and the exact next step for the user. This is a final status report."})
	text, err := a.streamFinalReport(ctx, messages, events)
	if err != nil {
		err = fmt.Errorf("max iterations reached; final report failed: %w", err)
		a.fireHook(ctx, hooks.Context{Event: hooks.EventSystemError, Error: err.Error()})
		sendEvent(ctx, events, Event{Kind: EventError, Error: err})
		return
	}
	if strings.TrimSpace(text) == "" {
		text = fmt.Sprintf("Reached the %d-iteration limit. Send 'continue' to resume from the current context.", maxIterations)
		sendEvent(ctx, events, Event{Kind: EventStreamText, Text: text})
	}
	session.AddAssistant(text)
	a.fireHook(ctx, hooks.Context{Event: hooks.EventSessionEnd})
	sendEvent(ctx, events, Event{Kind: EventFinalResponse, Text: text})
}

func (a *Agent) streamFinalReport(ctx context.Context, messages []chat.Message, events chan<- Event) (string, error) {
	stream, errs := a.Provider.StreamChat(ctx, messages, nil)
	var text string
	for event := range stream {
		switch event.Kind {
		case provider.EventText:
			text += event.Text
			sendEvent(ctx, events, Event{Kind: EventStreamText, Text: event.Text})
		case provider.EventThinking:
			sendEvent(ctx, events, Event{Kind: EventThinkingText, Text: event.Text})
		case provider.EventUsage:
			sendEvent(ctx, events, Event{Kind: EventUsage, Usage: Usage{InputTokens: event.Usage.InputTokens, OutputTokens: event.Usage.OutputTokens, CacheReadTokens: event.Usage.CacheReadTokens, CacheWriteTokens: event.Usage.CacheWriteTokens}})
		}
	}
	return text, <-errs
}

func (a *Agent) streamOneTurn(ctx context.Context, messages []chat.Message, tools []tool.Definition, events chan<- Event) (string, []provider.ToolCall, error) {
	stream, errs := a.Provider.StreamChat(ctx, messages, tools)
	var assistantText string
	var calls []provider.ToolCall
	for event := range stream {
		switch event.Kind {
		case provider.EventThinking:
			if !sendEvent(ctx, events, Event{Kind: EventThinkingText, Text: event.Text}) {
				return "", nil, ctx.Err()
			}
		case provider.EventText:
			assistantText += event.Text
			if !sendEvent(ctx, events, Event{Kind: EventStreamText, Text: event.Text}) {
				return "", nil, ctx.Err()
			}
		case provider.EventToolCall:
			if event.ToolCall != nil {
				call := *event.ToolCall
				calls = append(calls, call)
				if !sendEvent(ctx, events, Event{Kind: EventToolCallStart, ToolCallID: call.ID, ToolName: call.Name, ToolArguments: append([]byte(nil), call.Arguments...)}) {
					return "", nil, ctx.Err()
				}
			}
		case provider.EventStop:
		case provider.EventUsage:
			usage := Usage{
				InputTokens:      event.Usage.InputTokens,
				OutputTokens:     event.Usage.OutputTokens,
				CacheReadTokens:  event.Usage.CacheReadTokens,
				CacheWriteTokens: event.Usage.CacheWriteTokens,
			}
			if !sendEvent(ctx, events, Event{Kind: EventUsage, Usage: usage}) {
				return "", nil, ctx.Err()
			}
		}
	}
	if err := <-errs; err != nil {
		return "", nil, err
	}
	return assistantText, calls, nil
}

func (a *Agent) messagesForTurn(ctx context.Context, history []chat.Message, iteration int) []chat.Message {
	messages := make([]chat.Message, 0, len(history)+len(a.ContextMessages)+2)
	messages = append(messages, prompt.EnvironmentMessage(prompt.CurrentEnvironment(ctx, time.Now())))
	messages = append(messages, a.ContextMessages...)
	if a.HookEngine != nil {
		messages = append(messages, a.HookEngine.ContextMessages()...)
	}
	if a.SkillManager != nil {
		messages = append(messages, a.SkillManager.ContextMessages()...)
	}
	if a.WorkerManager != nil {
		messages = append(messages, a.WorkerManager.ContextMessages()...)
	}
	if a.TeamManager != nil {
		messages = append(messages, a.TeamManager.ContextMessages(a.TeamActor)...)
	}
	if prompt.ShouldInjectPlanOnly(a.PlanOnly) {
		messages = append(messages, prompt.PlanOnlyReminder(iteration))
	}
	messages = append(messages, history...)
	return messages
}

func (a *Agent) fireHook(ctx context.Context, hookCtx hooks.Context) {
	if a.HookEngine != nil {
		_ = a.HookEngine.Fire(ctx, hookCtx)
	}
}

func (a *Agent) toolsForTurn() []tool.Definition {
	defs := a.Tools
	if a.Registry != nil {
		defs = a.Registry.Definitions()
	}
	if a.SkillManager != nil {
		defs = a.SkillManager.FilterDefinitions(defs)
	}
	if a.TeamManager == nil {
		filtered := make([]tool.Definition, 0, len(defs))
		for _, def := range defs {
			if !team.IsTeamTool(def.Name) {
				filtered = append(filtered, def)
			}
		}
		return filtered
	}
	return a.TeamManager.FilterDefinitions(defs, a.TeamActor)
}

func sendEvent(ctx context.Context, events chan<- Event, event Event) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}

func mustMarshal(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

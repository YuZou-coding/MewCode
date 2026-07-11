package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/config"
	"mewcode/internal/sse"
	"mewcode/internal/tool"
)

type Anthropic struct {
	cfg        config.Config
	httpClient HTTPClient
}

func NewAnthropic(cfg config.Config, opts ...Option) *Anthropic {
	options := applyOptions(opts)
	return &Anthropic{cfg: cfg, httpClient: newHTTPClient(options)}
}

func (p *Anthropic) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		errs <- p.stream(ctx, messages, tools, events)
	}()

	return events, errs
}

func (p *Anthropic) stream(ctx context.Context, messages []chat.Message, tools []tool.Definition, events chan<- StreamEvent) error {
	systemPrompt, normalMessages := providerSystemAndMessages(messages, tools)
	body := map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": 4096,
		"stream":     true,
		"messages":   anthropicMessages(normalMessages),
	}
	if strings.TrimSpace(systemPrompt) != "" {
		body["system"] = systemPrompt
	}
	if len(tools) > 0 {
		body["tools"] = anthropicTools(tools)
	}
	if supportsThinking(p.cfg.Model) {
		body["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": 1024,
		}
	}

	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/messages", &payload)
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := validateStatus(resp); err != nil {
		return err
	}

	reader := sse.NewReader(resp.Body)
	toolAccumulator := newAnthropicToolAccumulator()
	for {
		event, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				for _, call := range toolAccumulator.Calls() {
					events <- StreamEvent{Kind: EventToolCall, ToolCall: &call}
				}
				events <- StreamEvent{Kind: EventStop, StopReason: "end_turn"}
				return nil
			}
			return err
		}
		if event.Name == "message_stop" {
			for _, call := range toolAccumulator.Calls() {
				events <- StreamEvent{Kind: EventToolCall, ToolCall: &call}
			}
			events <- StreamEvent{Kind: EventStop, StopReason: "end_turn"}
			return nil
		}
		if err := toolAccumulator.Add(event); err != nil {
			return err
		}
		streamEvent, ok, err := parseAnthropicEvent(event)
		if err != nil {
			return err
		}
		if ok {
			events <- streamEvent
		}
	}
}

func anthropicMessages(messages []chat.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message.ToolResult != nil {
			result = append(result, map[string]any{
				"role": string(chat.RoleUser),
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": message.ToolResult.CallID,
					"content":     string(message.ToolResult.Content),
				}},
			})
			continue
		}
		if len(message.ToolCalls) > 0 || message.ToolCall != nil {
			calls := message.ToolCalls
			if len(calls) == 0 && message.ToolCall != nil {
				calls = []chat.ToolCall{*message.ToolCall}
			}
			content := make([]map[string]any, 0, len(calls)+1)
			if strings.TrimSpace(message.Content) != "" {
				content = append(content, map[string]any{"type": "text", "text": message.Content})
			}
			for _, call := range calls {
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Name,
					"input": jsonRawObject(call.Arguments),
				})
			}
			result = append(result, map[string]any{
				"role":    string(chat.RoleAssistant),
				"content": content,
			})
			continue
		}
		result = append(result, map[string]any{"role": string(message.Role), "content": message.Content})
	}
	return result
}

func jsonRawObject(raw json.RawMessage) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	return v
}

func anthropicTools(defs []tool.Definition) []map[string]any {
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"name":         def.Name,
			"description":  def.Description,
			"input_schema": def.Schema,
		})
	}
	return result
}

func parseAnthropicEvent(event sse.Event) (StreamEvent, bool, error) {
	if event.Data == "" {
		return StreamEvent{}, false, nil
	}

	var payload struct {
		Type  string `json:"type"`
		Delta struct {
			Type       string `json:"type"`
			Text       string `json:"text"`
			Thinking   string `json:"thinking"`
			StopReason string `json:"stop_reason"`
		} `json:"delta"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens          int `json:"cache_read_tokens"`
			CacheWriteTokens         int `json:"cache_write_tokens"`
		} `json:"usage"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return StreamEvent{}, false, fmt.Errorf("malformed SSE event")
	}
	if payload.Error != nil {
		return StreamEvent{}, false, fmt.Errorf("provider stream error")
	}

	if usage := anthropicUsage(payload.Usage); usage != (Usage{}) {
		return StreamEvent{Kind: EventUsage, Usage: usage}, true, nil
	}
	if payload.Type == "message_delta" && payload.Delta.StopReason != "" {
		return StreamEvent{Kind: EventStop, StopReason: payload.Delta.StopReason}, true, nil
	}
	if payload.Type != "content_block_delta" {
		return StreamEvent{}, false, nil
	}
	switch payload.Delta.Type {
	case "text_delta":
		return StreamEvent{Kind: EventText, Text: payload.Delta.Text}, true, nil
	case "thinking_delta":
		return StreamEvent{Kind: EventThinking, Text: payload.Delta.Thinking}, true, nil
	default:
		return StreamEvent{}, false, nil
	}
}

func anthropicUsage(raw struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens          int `json:"cache_read_tokens"`
	CacheWriteTokens         int `json:"cache_write_tokens"`
}) Usage {
	usage := Usage{
		InputTokens:      raw.InputTokens,
		OutputTokens:     raw.OutputTokens,
		CacheReadTokens:  raw.CacheReadTokens,
		CacheWriteTokens: raw.CacheWriteTokens,
	}
	if usage.CacheReadTokens == 0 {
		usage.CacheReadTokens = raw.CacheReadInputTokens
	}
	if usage.CacheWriteTokens == 0 {
		usage.CacheWriteTokens = raw.CacheCreationInputTokens
	}
	return usage
}

type anthropicToolAccumulator struct {
	current *ToolCall
	calls   []ToolCall
}

func newAnthropicToolAccumulator() *anthropicToolAccumulator {
	return &anthropicToolAccumulator{}
}

func (a *anthropicToolAccumulator) Add(event sse.Event) error {
	if event.Data == "" {
		return nil
	}
	var payload struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return fmt.Errorf("malformed SSE event")
	}
	switch payload.Type {
	case "content_block_start":
		if payload.ContentBlock.Type == "tool_use" {
			a.current = &ToolCall{ID: payload.ContentBlock.ID, Name: payload.ContentBlock.Name}
		}
	case "content_block_delta":
		if a.current != nil && payload.Delta.Type == "input_json_delta" {
			a.current.Arguments = append(a.current.Arguments, []byte(payload.Delta.PartialJSON)...)
		}
	case "content_block_stop":
		if a.current != nil {
			a.calls = append(a.calls, *a.current)
			a.current = nil
		}
	}
	return nil
}

func (a *anthropicToolAccumulator) Calls() []ToolCall {
	calls := make([]ToolCall, len(a.calls))
	copy(calls, a.calls)
	return calls
}

func supportsThinking(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "claude") || strings.Contains(model, "sonnet") || strings.Contains(model, "opus")
}

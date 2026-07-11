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

type OpenAI struct {
	cfg        config.Config
	httpClient HTTPClient
}

func NewOpenAI(cfg config.Config, opts ...Option) *OpenAI {
	options := applyOptions(opts)
	return &OpenAI{cfg: cfg, httpClient: newHTTPClient(options)}
}

func (p *OpenAI) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		errs <- p.stream(ctx, messages, tools, events)
	}()

	return events, errs
}

func (p *OpenAI) stream(ctx context.Context, messages []chat.Message, tools []tool.Definition, events chan<- StreamEvent) error {
	systemPrompt, normalMessages := providerSystemAndMessages(messages, tools)
	body := map[string]any{
		"model":    p.cfg.Model,
		"messages": openAIMessages(systemPrompt, normalMessages),
		"stream":   true,
	}
	if len(tools) > 0 {
		body["tools"] = openAITools(tools)
	}

	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/chat/completions", &payload)
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := validateStatus(resp); err != nil {
		return err
	}

	reader := sse.NewReader(resp.Body)
	toolAccumulator := newOpenAIToolAccumulator()
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
		if event.Data == "[DONE]" || event.Name == "response.completed" {
			for _, call := range toolAccumulator.Calls() {
				events <- StreamEvent{Kind: EventToolCall, ToolCall: &call}
			}
			events <- StreamEvent{Kind: EventStop, StopReason: "end_turn"}
			return nil
		}
		if err := toolAccumulator.Add(event); err != nil {
			return err
		}
		streamEvent, ok, err := parseOpenAIEvent(event)
		if err != nil {
			return err
		}
		if ok {
			events <- streamEvent
		}
	}
}

func openAIMessages(systemPrompt string, messages []chat.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		result = append(result, map[string]any{"role": string(chat.RoleSystem), "content": systemPrompt})
	}
	for _, message := range messages {
		if len(message.ToolCalls) > 0 || message.ToolCall != nil {
			calls := message.ToolCalls
			if len(calls) == 0 && message.ToolCall != nil {
				calls = []chat.ToolCall{*message.ToolCall}
			}
			toolCalls := make([]map[string]any, 0, len(calls))
			for _, call := range calls {
				toolCalls = append(toolCalls, map[string]any{
					"id":   call.ID,
					"type": "function",
					"function": map[string]any{
						"name":      call.Name,
						"arguments": string(call.Arguments),
					},
				})
			}
			result = append(result, map[string]any{
				"role":       string(chat.RoleAssistant),
				"content":    message.Content,
				"tool_calls": toolCalls,
			})
			continue
		}
		if message.ToolResult != nil {
			result = append(result, map[string]any{
				"role":         string(chat.RoleTool),
				"tool_call_id": message.ToolResult.CallID,
				"name":         message.ToolResult.Name,
				"content":      string(message.ToolResult.Content),
			})
			continue
		}
		result = append(result, map[string]any{"role": string(message.Role), "content": message.Content})
	}
	return result
}

func openAITools(defs []tool.Definition) []map[string]any {
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        def.Name,
				"description": def.Description,
				"parameters":  def.Schema,
			},
		})
	}
	return result
}

func parseOpenAIEvent(event sse.Event) (StreamEvent, bool, error) {
	if event.Data == "" {
		return StreamEvent{}, false, nil
	}

	var payload struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Error any    `json:"error"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheReadTokens     int `json:"cache_read_tokens"`
			CacheWriteTokens    int `json:"cache_write_tokens"`
			PromptTokensDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheReadTokens  int `json:"cache_read_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Delta        struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return StreamEvent{}, false, fmt.Errorf("malformed SSE event")
	}
	if payload.Error != nil {
		return StreamEvent{}, false, fmt.Errorf("provider stream error")
	}

	if usage := openAIUsage(payload.Usage); usage != (Usage{}) {
		return StreamEvent{Kind: EventUsage, Usage: usage}, true, nil
	}
	if payload.Type == "response.output_text.delta" && payload.Delta != "" {
		return StreamEvent{Kind: EventText, Text: payload.Delta}, true, nil
	}
	if len(payload.Choices) > 0 && payload.Choices[0].FinishReason != "" {
		return StreamEvent{Kind: EventStop, StopReason: payload.Choices[0].FinishReason}, true, nil
	}
	if len(payload.Choices) > 0 && payload.Choices[0].Delta.Content != "" {
		return StreamEvent{Kind: EventText, Text: payload.Choices[0].Delta.Content}, true, nil
	}
	return StreamEvent{}, false, nil
}

func openAIUsage(raw struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_tokens"`
	CacheWriteTokens    int `json:"cache_write_tokens"`
	PromptTokensDetails struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheReadTokens  int `json:"cache_read_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
}) Usage {
	usage := Usage{
		InputTokens:      raw.PromptTokens,
		OutputTokens:     raw.CompletionTokens,
		CacheReadTokens:  raw.CacheReadTokens,
		CacheWriteTokens: raw.CacheWriteTokens,
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = raw.InputTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = raw.OutputTokens
	}
	if usage.CacheReadTokens == 0 {
		if raw.PromptTokensDetails.CacheReadTokens != 0 {
			usage.CacheReadTokens = raw.PromptTokensDetails.CacheReadTokens
		} else {
			usage.CacheReadTokens = raw.PromptTokensDetails.CachedTokens
		}
	}
	if usage.CacheWriteTokens == 0 {
		usage.CacheWriteTokens = raw.PromptTokensDetails.CacheWriteTokens
	}
	return usage
}

type openAIToolAccumulator struct {
	calls map[int]*ToolCall
}

func newOpenAIToolAccumulator() *openAIToolAccumulator {
	return &openAIToolAccumulator{calls: map[int]*ToolCall{}}
}

func (a *openAIToolAccumulator) Add(event sse.Event) error {
	if event.Data == "" || event.Data == "[DONE]" {
		return nil
	}
	var payload struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return fmt.Errorf("malformed SSE event")
	}
	if len(payload.Choices) == 0 {
		return nil
	}
	for _, delta := range payload.Choices[0].Delta.ToolCalls {
		call := a.calls[delta.Index]
		if call == nil {
			call = &ToolCall{}
			a.calls[delta.Index] = call
		}
		if delta.ID != "" {
			call.ID = delta.ID
		}
		if delta.Function.Name != "" {
			call.Name = delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			call.Arguments = append(call.Arguments, []byte(delta.Function.Arguments)...)
		}
	}
	return nil
}

func (a *openAIToolAccumulator) Calls() []ToolCall {
	calls := make([]ToolCall, 0, len(a.calls))
	for index := 0; index < len(a.calls); index++ {
		if call := a.calls[index]; call != nil {
			calls = append(calls, *call)
		}
	}
	return calls
}

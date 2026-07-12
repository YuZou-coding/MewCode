package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mewcode/internal/chat"
	"mewcode/internal/config"
	"mewcode/internal/sse"
	"mewcode/internal/tool"
)

func TestAnthropicStreamsTextAndHidesThinking(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatalf("missing api key header")
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		return sseResponse("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hidden\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"from Anthropic\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	provider := NewAnthropic(config.Config{
		Protocol: "anthropic",
		Model:    "claude-sonnet-4-5",
		BaseURL:  "http://provider.test",
		APIKey:   "secret",
	}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{{Role: chat.RoleUser, Content: "hello"}}, nil)

	var got strings.Builder
	var thinkingSeen bool
	for event := range events {
		if event.Kind == EventThinking {
			thinkingSeen = true
		}
		if event.Kind == EventText {
			got.WriteString(event.Text)
		}
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}
	if got.String() != "Hello from Anthropic" {
		t.Fatalf("text = %q", got.String())
	}
	if !thinkingSeen {
		t.Fatalf("expected thinking event")
	}
	if requestBody["thinking"] == nil {
		t.Fatalf("expected thinking settings in request")
	}
}

func TestAnthropicMalformedSSE(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return sseResponse("event: content_block_delta\ndata: not-json\n\n"), nil
	})

	provider := NewAnthropic(config.Config{Protocol: "anthropic", Model: "claude-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{{Role: chat.RoleUser, Content: "hello"}}, nil)
	for range events {
	}
	err := <-errs
	if err == nil || err.Error() != "malformed SSE event" {
		t.Fatalf("got %v, want malformed SSE event", err)
	}
}

func TestAnthropicRequestIncludesTools(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return sseResponse("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	provider := NewAnthropic(config.Config{Protocol: "anthropic", Model: "claude-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{{Role: chat.RoleUser, Content: "hello"}}, []tool.Definition{{
		Name:        "read_file",
		Description: "Read file",
		Schema:      tool.ObjectSchema([]string{"path"}, map[string]any{"path": tool.StringProperty("Path")}),
	}})
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}
	if requestBody["tools"] == nil {
		t.Fatalf("request missing tools: %#v", requestBody)
	}
	if !strings.Contains(fmt.Sprint(requestBody["system"]), "有专用工具时绝不要使用 run_command") {
		t.Fatalf("request missing tool system instruction: %#v", requestBody)
	}
}

func TestAnthropicToolSchemaContainsStrengthenedDescription(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	tools := anthropicTools(registry.Definitions())
	if !containsAnthropicToolDescription(tools, "edit_file", "先读取") {
		t.Fatalf("Anthropic tools missing strengthened edit_file description: %#v", tools)
	}
}

func TestAnthropicRequestSeparatesStableSystemAndDynamicMessages(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return sseResponse("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	provider := NewAnthropic(config.Config{Protocol: "anthropic", Model: "claude-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{
		{Role: chat.RoleSystem, Content: "<mewcode-environment>\ncwd: /tmp/project\n</mewcode-environment>"},
		{Role: chat.RoleUser, Content: "hello"},
	}, nil)
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}

	if !strings.Contains(fmt.Sprint(requestBody["system"]), "有专用工具时绝不要使用 run_command") {
		t.Fatalf("system missing stable instruction: %#v", requestBody["system"])
	}
	messages := requestBody["messages"].([]any)
	first := messages[0].(map[string]any)
	if first["role"] != "system" || !strings.Contains(fmt.Sprint(first["content"]), "cwd: /tmp/project") {
		t.Fatalf("first message is not dynamic environment: %#v", first)
	}
}

func TestAnthropicParsesUsageCacheFields(t *testing.T) {
	got, ok, err := parseAnthropicEvent(sse.Event{Data: `{"type":"message_delta","usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":128,"cache_creation_input_tokens":256}}`})
	if err != nil {
		t.Fatalf("parseAnthropicEvent returned error: %v", err)
	}
	if !ok || got.Kind != EventUsage {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	if got.Usage.InputTokens != 10 || got.Usage.OutputTokens != 20 || got.Usage.CacheReadTokens != 128 || got.Usage.CacheWriteTokens != 256 {
		t.Fatalf("usage = %#v", got.Usage)
	}
}

func TestAnthropicToolAccumulator(t *testing.T) {
	acc := newAnthropicToolAccumulator()
	events := []sse.Event{
		{Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`},
		{Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`},
		{Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"th\":\"README.md\"}"}}`},
		{Data: `{"type":"content_block_stop","index":0}`},
	}
	for _, event := range events {
		if err := acc.Add(event); err != nil {
			t.Fatalf("Add returned error: %v", err)
		}
	}
	calls := acc.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].ID != "toolu_1" || calls[0].Name != "read_file" || string(calls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("call = %#v", calls[0])
	}
}

func containsAnthropicToolDescription(tools []map[string]any, name string, text string) bool {
	for _, item := range tools {
		if item["name"] == name && strings.Contains(fmt.Sprint(item["description"]), text) {
			return true
		}
	}
	return false
}

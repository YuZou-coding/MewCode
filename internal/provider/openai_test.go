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

func TestOpenAIStreamsChatCompletionEvents(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"from OpenAI\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	provider := NewOpenAI(config.Config{Protocol: "openai", Model: "gpt-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{{Role: chat.RoleUser, Content: "hello"}}, nil)

	var got strings.Builder
	for event := range events {
		got.WriteString(event.Text)
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}
	if got.String() != "Hello from OpenAI" {
		t.Fatalf("text = %q", got.String())
	}
	if requestBody["stream"] != true {
		t.Fatalf("expected stream=true request")
	}
	if requestBody["messages"] == nil {
		t.Fatalf("expected messages request field")
	}
}

func TestOpenAIStreamsResponsesCompatibleEvents(t *testing.T) {
	got, ok, err := parseOpenAIEvent(sse.Event{Data: `{"type":"response.output_text.delta","delta":"hi"}`})
	if err != nil {
		t.Fatalf("parseOpenAIEvent returned error: %v", err)
	}
	if !ok || got.Text != "hi" {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
}

func TestOpenAIStreamsChatCompletionCompatibleEvents(t *testing.T) {
	got, ok, err := parseOpenAIEvent(sse.Event{Data: `{"choices":[{"delta":{"content":"hi"}}]}`})
	if err != nil {
		t.Fatalf("parseOpenAIEvent returned error: %v", err)
	}
	if !ok || got.Text != "hi" {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
}

func TestOpenAIRequestIncludesTools(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return sseResponse("data: [DONE]\n\n"), nil
	})

	provider := NewOpenAI(config.Config{Protocol: "openai", Model: "gpt-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
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
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("request missing messages: %#v", requestBody)
	}
	systemMessage, ok := messages[0].(map[string]any)
	if !ok || systemMessage["role"] != "system" || !strings.Contains(fmt.Sprint(systemMessage["content"]), "优先使用专用工具") {
		t.Fatalf("request missing tool system instruction: %#v", messages)
	}
}

func TestOpenAIToolSchemaContainsStrengthenedDescription(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	tools := openAITools(registry.Definitions())
	if !containsToolDescription(tools, "edit_file", "先读取") {
		t.Fatalf("OpenAI tools missing strengthened edit_file description: %#v", tools)
	}
}

func TestOpenAIRequestSeparatesStableSystemAndDynamicMessages(t *testing.T) {
	var requestBody map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return sseResponse("data: [DONE]\n\n"), nil
	})

	provider := NewOpenAI(config.Config{Protocol: "openai", Model: "gpt-test", BaseURL: "http://provider.test", APIKey: "secret"}, WithHTTPClient(client))
	events, errs := provider.StreamChat(context.Background(), []chat.Message{
		{Role: chat.RoleSystem, Content: "<mewcode-environment>\ncwd: /tmp/project\n</mewcode-environment>"},
		{Role: chat.RoleUser, Content: "hello"},
	}, nil)
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}

	messages := requestBody["messages"].([]any)
	first := messages[0].(map[string]any)
	second := messages[1].(map[string]any)
	third := messages[2].(map[string]any)
	if first["role"] != "system" || !strings.Contains(fmt.Sprint(first["content"]), "优先使用专用工具") {
		t.Fatalf("first message is not stable system: %#v", first)
	}
	if second["role"] != "system" || !strings.Contains(fmt.Sprint(second["content"]), "cwd: /tmp/project") {
		t.Fatalf("second message is not dynamic environment: %#v", second)
	}
	if third["role"] != "user" || third["content"] != "hello" {
		t.Fatalf("third message is not user input: %#v", third)
	}
}

func TestOpenAIParsesUsageCacheFields(t *testing.T) {
	got, ok, err := parseOpenAIEvent(sse.Event{Data: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":128,"cache_write_tokens":256}}}`})
	if err != nil {
		t.Fatalf("parseOpenAIEvent returned error: %v", err)
	}
	if !ok || got.Kind != EventUsage {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	if got.Usage.InputTokens != 10 || got.Usage.OutputTokens != 20 || got.Usage.CacheReadTokens != 128 || got.Usage.CacheWriteTokens != 256 {
		t.Fatalf("usage = %#v", got.Usage)
	}
}

func TestOpenAIToolCallAccumulator(t *testing.T) {
	acc := newOpenAIToolAccumulator()
	events := []sse.Event{
		{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`},
		{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"README.md\"}"}}]}}]}`},
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
	if calls[0].ID != "call_1" || calls[0].Name != "read_file" || string(calls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("call = %#v", calls[0])
	}
}

func containsToolDescription(tools []map[string]any, name string, text string) bool {
	for _, item := range tools {
		fn, ok := item["function"].(map[string]any)
		if !ok {
			continue
		}
		if fn["name"] == name && strings.Contains(fmt.Sprint(fn["description"]), text) {
			return true
		}
	}
	return false
}

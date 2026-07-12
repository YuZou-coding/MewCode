package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mewcode/internal/agent"
	"mewcode/internal/chat"
	"mewcode/internal/config"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
)

func TestOpenAIPromptRequestContainsStableEnvironmentAndUser(t *testing.T) {
	var body map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(decoded) {
			return notesResponse(), nil
		}
		body = decoded
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	_ = runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "hello\n/exit\n", provider.WithHTTPClient(client))
	messages := body["messages"].([]any)
	stable := messages[0].(map[string]any)
	env := messages[1].(map[string]any)
	user := findMessage(messages, "user", "hello")
	if stable["role"] != "system" || !strings.Contains(fmt.Sprint(stable["content"]), "有专用工具时绝不要使用 run_command") {
		t.Fatalf("missing stable system: %#v", stable)
	}
	if env["role"] != "system" || !strings.Contains(fmt.Sprint(env["content"]), "cwd") {
		t.Fatalf("missing environment supplement: %#v", env)
	}
	if user == nil {
		t.Fatalf("missing user message: %#v", user)
	}
	joined := fmt.Sprint(messages)
	for _, want := range []string{"commit: 生成提交前检查", "review: 进行代码审查", "test: 制定并执行测试策略"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing skill summary %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "你是 MewCode 的 review Skill") || strings.Contains(joined, "你是 MewCode 的 commit Skill") {
		t.Fatalf("first request leaked full skill SOP: %s", joined)
	}
	if !requestToolsContain(body, "edit_file", "先读取") {
		t.Fatalf("request tools missing strengthened edit_file description: %#v", body["tools"])
	}
}

func TestStableGlobalInstructionIsIdenticalAcrossTurns(t *testing.T) {
	var bodies []map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			return notesResponse(), nil
		}
		bodies = append(bodies, body)
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	_ = runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "first\nsecond\n/exit\n", provider.WithHTTPClient(client))
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	firstMessages := bodies[0]["messages"].([]any)
	secondMessages := bodies[1]["messages"].([]any)
	firstStable := firstMessages[0].(map[string]any)["content"]
	secondStable := secondMessages[0].(map[string]any)["content"]
	if firstStable != secondStable {
		t.Fatalf("stable system changed:\n%v\n---\n%v", firstStable, secondStable)
	}
}

func TestAnthropicPromptRequestContainsStableEnvironmentAndUser(t *testing.T) {
	var body map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(decoded) {
			return anthropicNotesResponse(), nil
		}
		body = decoded
		return sseResponse("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	_ = runInTempProject(t, configBody("anthropic", "claude-test", "http://provider.test"), "hello\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(fmt.Sprint(body["system"]), "有专用工具时绝不要使用 run_command") {
		t.Fatalf("missing stable system: %#v", body["system"])
	}
	messages := body["messages"].([]any)
	env := messages[0].(map[string]any)
	user := findMessage(messages, "user", "hello")
	if env["role"] != "system" || !strings.Contains(fmt.Sprint(env["content"]), "cwd") {
		t.Fatalf("missing environment supplement: %#v", env)
	}
	if user == nil {
		t.Fatalf("missing user message: %#v", user)
	}
}

func TestOpenAIPromptCacheUsageEndToEndDoesNotBreakOutput(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return sseResponse("data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":128,\"cache_write_tokens\":256}}}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "hello\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q", out)
	}
}

func TestOpenAICacheUsageReachesAgentEvent(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return sseResponse("data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":128,\"cache_write_tokens\":256}}}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})
	p := provider.NewOpenAI(config.Config{Protocol: "openai", Model: "gpt-test", BaseURL: "http://provider.test", APIKey: "test-key"}, provider.WithHTTPClient(client))
	events := collectAgentEvents((&agent.Agent{Provider: p, Session: chat.NewSession()}).Run(context.Background(), "hello"))
	assertUsageEvent(t, events)
}

func TestPlanOnlyBlocksWriteToolAndReturnsPlan(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	p := &scriptedProvider{series: [][]provider.StreamEvent{
		{{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{ID: "call_1", Name: "edit_file", Arguments: []byte(`{"path":"tmp_tool_test.txt","old_text":"a","new_text":"b"}`)}}},
		{{Kind: provider.EventText, Text: "计划：先读取文件，再提出修改方案。"}},
	}}
	events := collectAgentEvents((&agent.Agent{
		Provider: p,
		Registry: registry,
		Session:  chat.NewSession(),
		Tools:    registry.Definitions(),
		PlanOnly: true,
	}).Run(context.Background(), "修改文件"))

	var blocked bool
	var final string
	for _, event := range events {
		if event.Kind == agent.EventToolResult && event.Result != nil && event.Result.Error != nil && event.Result.Error.Code == "plan_only_blocked" {
			blocked = true
		}
		if event.Kind == agent.EventFinalResponse {
			final = event.Text
		}
	}
	if !blocked {
		t.Fatalf("missing plan-only blocked result: %#v", events)
	}
	if !strings.Contains(final, "计划") {
		t.Fatalf("final response = %q", final)
	}
}

func TestAnthropicCacheUsageReachesAgentEvent(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return sseResponse("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"cache_read_input_tokens\":128,\"cache_creation_input_tokens\":256}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})
	p := provider.NewAnthropic(config.Config{Protocol: "anthropic", Model: "claude-test", BaseURL: "http://provider.test", APIKey: "test-key"}, provider.WithHTTPClient(client))
	events := collectAgentEvents((&agent.Agent{Provider: p, Session: chat.NewSession()}).Run(context.Background(), "hello"))
	assertUsageEvent(t, events)
}

func collectAgentEvents(events <-chan agent.Event) []agent.Event {
	var result []agent.Event
	for event := range events {
		result = append(result, event)
	}
	return result
}

func assertUsageEvent(t *testing.T, events []agent.Event) {
	t.Helper()
	for _, event := range events {
		if event.Kind == agent.EventUsage {
			if event.Usage.CacheReadTokens == 128 && event.Usage.CacheWriteTokens == 256 {
				return
			}
			t.Fatalf("usage event has wrong tokens: %#v", event.Usage)
		}
	}
	t.Fatalf("missing usage event: %#v", events)
}

type scriptedProvider struct {
	series [][]provider.StreamEvent
}

func (p *scriptedProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	selected := []provider.StreamEvent{}
	if len(p.series) > 0 {
		selected = p.series[0]
		p.series = p.series[1:]
	}
	events := make(chan provider.StreamEvent, len(selected))
	errs := make(chan error, 1)
	for _, event := range selected {
		events <- event
	}
	close(events)
	errs <- nil
	return events, errs
}

func findMessage(messages []any, role string, content string) map[string]any {
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if message["role"] == role && message["content"] == content {
			return message
		}
	}
	return nil
}

func requestToolsContain(body map[string]any, name string, text string) bool {
	tools, ok := body["tools"].([]any)
	if !ok {
		return false
	}
	for _, item := range tools {
		toolMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fn, ok := toolMap["function"].(map[string]any)
		if !ok {
			continue
		}
		if fn["name"] == name && strings.Contains(fmt.Sprint(fn["description"]), text) {
			return true
		}
	}
	return false
}

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/app"
	"mewcode/internal/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAnthropicEndToEnd(t *testing.T) {
	var requests []map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			return anthropicNotesResponse(), nil
		}
		requests = append(requests, body)

		return sseResponse("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"from Anthropic\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	out := runInTempProject(t, configBody("anthropic", "claude-sonnet-4-5", "http://provider.test"), "hello\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "Hello from Anthropic") {
		t.Fatalf("output = %q", out)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
}

func TestOpenAIEndToEndAndContext(t *testing.T) {
	var requests []map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			return notesResponse(), nil
		}
		requests = append(requests, body)

		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"from OpenAI\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "my name is Mew\nwhat is my name?\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "Hello from OpenAI") {
		t.Fatalf("output = %q", out)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	input, ok := requests[1]["messages"].([]any)
	if !ok {
		t.Fatalf("second request messages has unexpected shape: %#v", requests[1]["messages"])
	}
	if len(input) < 3 {
		t.Fatalf("second request input length = %d, want prior context", len(input))
	}
	if !containsMessage(input, "my name is Mew") {
		t.Fatalf("second request missing prior user context: %#v", input)
	}
}

func runInTempProject(t *testing.T, config string, input string, opts ...provider.Option) string {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("HOME", filepath.Join(tempDir, "home"))
	if err := os.WriteFile(filepath.Join(tempDir, "mewcode.yaml"), []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	var out strings.Builder
	var errs strings.Builder
	err = app.RunWithProviderOptions(context.Background(), strings.NewReader(input), &out, &errs, opts...)
	if err != nil {
		t.Fatalf("app.Run returned error: %v; stderr=%q", err, errs.String())
	}
	if errs.Len() > 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return out.String()
}

func configBody(protocol, model, baseURL string) string {
	return "protocol: " + protocol + "\nmodel: " + model + "\nbase_url: " + baseURL + "\napi_key: test-key\n"
}

func containsMessage(input []any, content string) bool {
	for _, item := range input {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if message["content"] == content {
			return true
		}
	}
	return false
}

func sseResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"content-type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func notesResponse() *http.Response {
	return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"## 用户级笔记\\n- 用户偏好：\\n- 纠正反馈：\\n\\n## 项目级笔记\\n- 项目知识：\\n- 参考资料：\"}}]}\n\n" +
		"data: [DONE]\n\n")
}

func anthropicNotesResponse() *http.Response {
	return sseResponse("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"## 用户级笔记\\n- 用户偏好：\\n- 纠正反馈：\\n\\n## 项目级笔记\\n- 项目知识：\\n- 参考资料：\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
}

func isNotesRequest(body map[string]any) bool {
	for _, message := range bodyMessages(body) {
		if strings.Contains(fmt.Sprint(message["content"]), "请更新 MewCode 笔记") {
			return true
		}
	}
	return false
}

func bodyMessages(body map[string]any) []map[string]any {
	raw, ok := body["messages"].([]any)
	if !ok {
		return nil
	}
	var messages []map[string]any
	for _, item := range raw {
		if message, ok := item.(map[string]any); ok {
			messages = append(messages, message)
		}
	}
	return messages
}

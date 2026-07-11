package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/provider"
)

func TestOpenAIToolEndToEndReadFile(t *testing.T) {
	tempDir := t.TempDir()
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("README from tool"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			return notesResponse(), nil
		}
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("call_1", "read_file", map[string]any{"path": readme})), nil
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"README final answer\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "读 README\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "README final answer") {
		t.Fatalf("output = %q", out)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestAnthropicToolEndToEndSearchCode(t *testing.T) {
	tempDir := t.TempDir()
	file := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n// MewCode marker\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			return anthropicNotesResponse(), nil
		}
		requests++
		if requests == 1 {
			return sseResponse(anthropicToolCallSSE("toolu_1", "search_code", map[string]any{"root": tempDir, "pattern": "MewCode"})), nil
		}
		return sseResponse("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"found main.go\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), nil
	})

	out := runInTempProject(t, configBody("anthropic", "claude-test", "http://provider.test"), "搜索 MewCode\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "found main.go") {
		t.Fatalf("output = %q", out)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestRunCommandDeniedEndToEnd(t *testing.T) {
	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("call_1", "run_command", map[string]any{"command": "echo no"})), nil
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"command was denied\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "run command\nn\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "Allow run_command?") || !strings.Contains(out, "command was denied") {
		t.Fatalf("output = %q", out)
	}
}

func TestEditFileMultipleMatchesEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	file := filepath.Join(tempDir, "note.txt")
	if err := os.WriteFile(file, []byte("same same"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("call_1", "edit_file", map[string]any{"path": file, "old_text": "same", "new_text": "changed"})), nil
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"old_text matched multiple times\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "edit file\ny\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "old_text matched multiple times") {
		t.Fatalf("output = %q", out)
	}
}

func TestToolResultHistoryIsSentToProvider(t *testing.T) {
	var secondRequest map[string]any
	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("call_1", "read_file", map[string]any{"path": "README.md"})), nil
		}
		if err := json.NewDecoder(r.Body).Decode(&secondRequest); err != nil {
			t.Fatalf("decode second request: %v", err)
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	_ = runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "read\n/exit\n", provider.WithHTTPClient(client))
	messages, ok := secondRequest["messages"].([]any)
	if !ok || len(messages) < 3 {
		t.Fatalf("second request messages = %#v", secondRequest["messages"])
	}
	if !containsRole(messages, "tool") {
		t.Fatalf("second request missing tool result: %#v", messages)
	}
}

func openAIToolCallSSE(id string, name string, args map[string]any) string {
	raw, _ := json.Marshal(args)
	arg := string(raw)
	mid := len(arg) / 2
	return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":%q,\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\n", id, name, arg[:mid]) +
		fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":%q}}]}}]}\n\n", arg[mid:]) +
		"data: [DONE]\n\n"
}

func anthropicToolCallSSE(id string, name string, args map[string]any) string {
	raw, _ := json.Marshal(args)
	arg := string(raw)
	mid := len(arg) / 2
	return fmt.Sprintf("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":%q,\"name\":%q,\"input\":{}}}\n\n", id, name) +
		fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%q}}\n\n", arg[:mid]) +
		fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%q}}\n\n", arg[mid:]) +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

func containsRole(messages []any, role string) bool {
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if ok && message["role"] == role {
			return true
		}
	}
	return false
}

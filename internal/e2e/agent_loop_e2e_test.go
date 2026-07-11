package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/provider"
)

func TestOpenAIAgentLoopRunsMultipleToolRounds(t *testing.T) {
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
			return notesResponse(), nil
		}
		requests++
		switch requests {
		case 1:
			return sseResponse(openAIToolCallSSE("call_1", "search_code", map[string]any{"root": tempDir, "pattern": "MewCode"})), nil
		case 2:
			return sseResponse(openAIToolCallSSE("call_2", "read_file", map[string]any{"path": file})), nil
		default:
			return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"found main.go and read it\"}}]}\n\n" +
				"data: [DONE]\n\n"), nil
		}
	})

	out := runInTempProject(t, configBody("openai", "gpt-test", "http://provider.test"), "搜索 MewCode 然后读取匹配文件\n/exit\n", provider.WithHTTPClient(client))
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if !strings.Contains(out, "found main.go and read it") {
		t.Fatalf("output = %q", out)
	}
}

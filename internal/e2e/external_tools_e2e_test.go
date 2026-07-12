package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mewcode/internal/app"
	"mewcode/internal/config"
	"mewcode/internal/external"
	"mewcode/internal/provider"
)

func TestExternalStdioToolEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	countFile := filepath.Join(tempDir, "stdio-count.txt")
	writeExternalProjectConfig(t, tempDir, externalStdioConfig("stdio", countFile))
	client := externalToolProvider("external_stdio_echo", map[string]any{"text": "hello stdio"}, "echo result")

	out := runExternalProject(t, tempDir, "echo\ny\ny\n/exit\n", client)
	if !strings.Contains(out, "echo result") || !strings.Contains(out, "using tool external_stdio_echo") {
		t.Fatalf("output = %q", out)
	}
	if got := readCountFile(t, countFile); got != "initialize=1\ntools/list=1\ntools/call=1\n" {
		t.Fatalf("count file = %q output=%q", got, out)
	}
}

func TestExternalHTTPToolEndToEnd(t *testing.T) {
	clientHTTP := &scriptedExternalHTTP{handler: func(w http.ResponseWriter, r *http.Request, counts map[string]int) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode external request: %v", err)
		}
		method := req["method"].(string)
		w.Header().Set("Content-Type", "text/event-stream")
		switch method {
		case "initialize":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"ok\":true}}\n\n", req["id"])
		case "tools/list":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"tools\":[{\"name\":\"time\",\"description\":\"Time tool\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n", req["id"])
		case "tools/call":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"time result\"}]}}\n\n", req["id"])
		}
	}}

	tempDir := t.TempDir()
	writeModelConfig(t, tempDir)
	manager := external.NewManager([]external.ServerConfig{{Name: "clock", Transport: "http", URL: "http://external.test/mcp"}}, clientHTTP)
	client := externalToolProvider("external_clock_time", map[string]any{}, "time result")
	out := runExternalProjectWithManager(t, tempDir, manager, "time\ny\ny\n/exit\n", client)
	if !strings.Contains(out, "time result") || !strings.Contains(out, "using tool external_clock_time") {
		t.Fatalf("output = %q", out)
	}
	if clientHTTP.counts["initialize"] != 1 || clientHTTP.counts["tools/list"] != 1 || clientHTTP.counts["tools/call"] != 1 {
		t.Fatalf("counts = %#v", clientHTTP.counts)
	}
}

func TestExternalConnectionReuseAndServerIsolation(t *testing.T) {
	tempDir := t.TempDir()
	countFile := filepath.Join(tempDir, "stdio-count.txt")
	healthy := externalStdioConfig("stdio", countFile)
	broken := `- name: broken
  transport: http
  url: http://127.0.0.1:1/mcp
`
	writeExternalProjectConfig(t, tempDir, "servers:\n"+strings.TrimPrefix(healthy, "servers:\n")+broken)

	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			return sseResponse(openAIToolCallSSE("search_1", "tool_search", map[string]any{"query": "select:external_stdio_echo"})), nil
		case 2:
			return sseResponse(openAIToolCallSSE("call_1", "external_stdio_echo", map[string]any{"text": "first"})), nil
		case 3:
			return sseResponse(openAIToolCallSSE("call_2", "external_stdio_echo", map[string]any{"text": "second"})), nil
		default:
			return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
				"data: [DONE]\n\n"), nil
		}
	})
	out := runExternalProjectAllowingStderr(t, tempDir, "reuse\ny\ny\ny\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "done") {
		t.Fatalf("output = %q", out)
	}
	if got := readCountFile(t, countFile); got != "initialize=1\ntools/list=1\ntools/call=2\n" {
		t.Fatalf("count file = %q output=%q", got, out)
	}
}

func TestExternalToolErrorFeedsBackToModel(t *testing.T) {
	clientHTTP := &scriptedExternalHTTP{handler: func(w http.ResponseWriter, r *http.Request, counts map[string]int) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		switch req["method"] {
		case "initialize":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{}}\n\n", req["id"])
		case "tools/list":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"tools\":[{\"name\":\"fail\",\"description\":\"Fail\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n", req["id"])
		case "tools/call":
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"error\":{\"code\":-32000,\"message\":\"remote boom\"}}\n\n", req["id"])
		}
	}}

	var secondRequest map[string]any
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
			return sseResponse(openAIToolCallSSE("search_1", "tool_search", map[string]any{"query": "select:external_bad_fail"})), nil
		}
		if requests == 2 {
			return sseResponse(openAIToolCallSSE("call_1", "external_bad_fail", map[string]any{})), nil
		}
		secondRequest = body
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"error handled\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	tempDir := t.TempDir()
	writeModelConfig(t, tempDir)
	manager := external.NewManager([]external.ServerConfig{{Name: "bad", Transport: "http", URL: "http://external.test/mcp"}}, clientHTTP)
	out := runExternalProjectWithManager(t, tempDir, manager, "fail\ny\ny\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "error handled") {
		t.Fatalf("output = %q", out)
	}
	raw, _ := json.Marshal(secondRequest)
	if !strings.Contains(string(raw), "external_tool_failed") {
		t.Fatalf("second request missing external_tool_failed: %s", raw)
	}
}

func TestExternalToolsStdioHelper(t *testing.T) {
	if os.Getenv("MEWCODE_EXTERNAL_STDIO_HELPER") != "1" {
		return
	}
	countFile := os.Getenv("MEWCODE_EXTERNAL_COUNT_FILE")
	counts := map[string]int{}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req map[string]any
		_ = json.Unmarshal(scanner.Bytes(), &req)
		id := req["id"]
		method := req["method"].(string)
		counts[method]++
		_ = writeExternalCounts(countFile, counts)
		switch method {
		case "initialize":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"ok\":true}}\n", id)
		case "tools/list":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"tools\":[{\"name\":\"echo\",\"description\":\"Echo tool\",\"inputSchema\":{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\"}}}}]}}\n", id)
		case "tools/call":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"echo result\"}]}}\n", id)
		}
	}
	os.Exit(0)
}

func externalToolProvider(toolName string, args map[string]any, final string) provider.Option {
	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("search_1", "tool_search", map[string]any{"query": "select:" + toolName})), nil
		}
		if requests == 2 {
			return sseResponse(openAIToolCallSSE("call_1", toolName, args)), nil
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":" + strconvQuote(final) + "}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})
	return provider.WithHTTPClient(client)
}

func externalStdioConfig(name string, countFile string) string {
	return `servers:
- name: ` + name + `
  transport: stdio
  command: ` + os.Args[0] + `
  args: ["-test.run=TestExternalToolsStdioHelper"]
  env:
    MEWCODE_EXTERNAL_STDIO_HELPER: "1"
    MEWCODE_EXTERNAL_COUNT_FILE: "` + countFile + `"
`
}

func writeExternalProjectConfig(t *testing.T, dir string, servers string) {
	t.Helper()
	writeModelConfig(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".mewcode"), 0700); err != nil {
		t.Fatalf("mkdir .mewcode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mewcode", "mcp_servers.yaml"), []byte(servers), 0600); err != nil {
		t.Fatalf("write servers: %v", err)
	}
}

func writeModelConfig(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "mewcode.yaml"), []byte(configBody("openai", "gpt-test", "http://provider.test")), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func runExternalProject(t *testing.T, dir string, input string, opts ...provider.Option) string {
	t.Helper()
	out, errs := runExternalProjectRaw(t, dir, input, opts...)
	if errs != "" {
		t.Fatalf("stderr = %q", errs)
	}
	return out
}

func runExternalProjectAllowingStderr(t *testing.T, dir string, input string, opts ...provider.Option) string {
	t.Helper()
	out, _ := runExternalProjectRaw(t, dir, input, opts...)
	return out
}

func runExternalProjectWithManager(t *testing.T, dir string, manager *external.Manager, input string, opts ...provider.Option) string {
	t.Helper()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	modelProvider, err := provider.New(config.Config{Protocol: "openai", Model: "gpt-test", BaseURL: "http://provider.test", APIKey: "test-key"}, opts...)
	if err != nil {
		t.Fatalf("provider.New returned error: %v", err)
	}
	var out strings.Builder
	var errs strings.Builder
	if err := (app.App{
		Input:           strings.NewReader(input),
		Output:          &out,
		Errors:          &errs,
		Provider:        modelProvider,
		ExternalManager: manager,
		NoTypeDelay:     true,
	}).Run(context.Background()); err != nil {
		t.Fatalf("App.Run returned error: %v; stderr=%q", err, errs.String())
	}
	if errs.Len() > 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return out.String()
}

func runExternalProjectRaw(t *testing.T, dir string, input string, opts ...provider.Option) (string, string) {
	t.Helper()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	var out strings.Builder
	var errs strings.Builder
	if err := app.RunWithProviderOptions(context.Background(), strings.NewReader(input), &out, &errs, opts...); err != nil {
		t.Fatalf("RunWithProviderOptions returned error: %v; stderr=%q", err, errs.String())
	}
	return out.String(), errs.String()
}

func readCountFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read count file: %v", err)
	}
	return string(raw)
}

func writeExternalCounts(path string, counts map[string]int) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("initialize=%d\ntools/list=%d\ntools/call=%d\n", counts["initialize"], counts["tools/list"], counts["tools/call"])), 0600)
}

var _ io.Reader

type scriptedExternalHTTP struct {
	mu      sync.Mutex
	counts  map[string]int
	handler func(http.ResponseWriter, *http.Request, map[string]int)
}

func (s *scriptedExternalHTTP) Do(req *http.Request) (*http.Response, error) {
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	var body strings.Builder
	recorder := responseRecorder{header: http.Header{}, body: &body, status: http.StatusOK}
	var decoded map[string]any
	_ = json.NewDecoder(req.Body).Decode(&decoded)
	method, _ := decoded["method"].(string)
	req.Body = io.NopCloser(strings.NewReader(mustJSONText(decoded)))
	s.mu.Lock()
	s.counts[method]++
	s.mu.Unlock()
	s.handler(&recorder, req, s.counts)
	return &http.Response{StatusCode: recorder.status, Header: recorder.header, Body: io.NopCloser(strings.NewReader(body.String()))}, nil
}

type responseRecorder struct {
	header http.Header
	body   *strings.Builder
	status int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func mustJSONText(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

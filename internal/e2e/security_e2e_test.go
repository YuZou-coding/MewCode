package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/app"
	"mewcode/internal/provider"
)

func TestSecurityBlocksDangerousCommandEndToEnd(t *testing.T) {
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
			return sseResponse(openAIToolCallSSE("call_1", "run_command", map[string]any{"command": "rm -rf /"})), nil
		}
		secondRequest = body
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"blocked\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out, _ := runSecurityProject(t, "run dangerous\n/exit\n", nil, provider.WithHTTPClient(client))
	if !strings.Contains(out, "blocked") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "permission required") || strings.Contains(out, "[n] deny [y] allow once") {
		t.Fatalf("dangerous command should not trigger HITL: %q", out)
	}
	if !requestContains(secondRequest, "dangerous_command") {
		t.Fatalf("second request missing dangerous_command: %#v", secondRequest)
	}
}

func TestSecurityBlocksPathOutsideSandboxEndToEnd(t *testing.T) {
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
			return sseResponse(openAIToolCallSSE("call_1", "read_file", map[string]any{"path": "../outside.txt"})), nil
		}
		secondRequest = body
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"outside blocked\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out, _ := runSecurityProject(t, "read outside\n/exit\n", nil, provider.WithHTTPClient(client))
	if !strings.Contains(out, "outside blocked") {
		t.Fatalf("output = %q", out)
	}
	if !requestContains(secondRequest, "path_outside_sandbox") {
		t.Fatalf("second request missing path_outside_sandbox: %#v", secondRequest)
	}
}

func TestSecurityHITLDenySessionAndAlwaysEndToEnd(t *testing.T) {
	t.Run("deny", func(t *testing.T) {
		client := editThenFinalClient("denied")
		out, dir := runSecurityProject(t, "edit\nn\n/exit\n", writeToolFixture, client)
		if !strings.Contains(out, "[n] deny [y] allow once [s] allow session [a] allow always") {
			t.Fatalf("output missing HITL options: %q", out)
		}
		assertSecurityFile(t, filepath.Join(dir, "tmp_tool_test.txt"), "hello tool")
	})

	t.Run("once", func(t *testing.T) {
		requests := 0
		client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			switch requests {
			case 1:
				return sseResponse(openAIToolCallSSE("call_1", "edit_file", map[string]any{"path": "tmp_tool_test.txt", "old_text": "hello tool", "new_text": "hello zy"})), nil
			case 2:
				return sseResponse(openAIToolCallSSE("call_2", "edit_file", map[string]any{"path": "tmp_tool_test.txt", "old_text": "hello zy", "new_text": "hello tool"})), nil
			default:
				return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
					"data: [DONE]\n\n"), nil
			}
		})
		out, dir := runSecurityProject(t, "edit\ny\nn\n/exit\n", writeToolFixture, provider.WithHTTPClient(client))
		if strings.Count(out, "Allow edit_file?") != 2 {
			t.Fatalf("allow once should ask again, output = %q", out)
		}
		assertSecurityFile(t, filepath.Join(dir, "tmp_tool_test.txt"), "hello zy")
	})

	t.Run("session", func(t *testing.T) {
		requests := 0
		client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			switch requests {
			case 1:
				return sseResponse(openAIToolCallSSE("call_1", "edit_file", map[string]any{"path": "tmp_tool_test.txt", "old_text": "hello tool", "new_text": "hello zy"})), nil
			case 2:
				return sseResponse(openAIToolCallSSE("call_2", "edit_file", map[string]any{"path": "tmp_tool_test.txt", "old_text": "hello zy", "new_text": "hello tool"})), nil
			default:
				return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
					"data: [DONE]\n\n"), nil
			}
		})
		out, dir := runSecurityProject(t, "edit\ns\n/exit\n", writeToolFixture, provider.WithHTTPClient(client))
		if strings.Count(out, "allow session") != 1 {
			t.Fatalf("expected one HITL prompt, output = %q", out)
		}
		assertSecurityFile(t, filepath.Join(dir, "tmp_tool_test.txt"), "hello tool")
	})

	t.Run("always", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		client := editThenFinalClient("allowed")
		_, _ = runSecurityProject(t, "edit\na\n/exit\n", writeToolFixture, client)
		content, err := os.ReadFile(filepath.Join(home, ".mewcode", "permissions.yaml"))
		if err != nil {
			t.Fatalf("read user permissions: %v", err)
		}
		if !strings.Contains(string(content), "edit_file") || !strings.Contains(string(content), "allow") {
			t.Fatalf("permissions = %s", content)
		}

		out, dir := runSecurityProject(t, "edit\n/exit\n", writeToolFixture, editThenFinalClient("allowed from user rule"))
		if strings.Contains(out, "Allow edit_file?") {
			t.Fatalf("allow always should avoid prompt on next startup: %q", out)
		}
		assertSecurityFile(t, filepath.Join(dir, "tmp_tool_test.txt"), "hello zy")
	})
}

func TestProjectDenyOverridesUserAllowEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".mewcode"), 0700); err != nil {
		t.Fatalf("mkdir home permissions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mewcode", "permissions.yaml"), []byte("rules:\n- effect: allow\n  tool: edit_file\n  path_pattern: tmp_tool_test.txt\n"), 0600); err != nil {
		t.Fatalf("write user permissions: %v", err)
	}
	client := editThenFinalClient("project denied")
	out, dir := runSecurityProject(t, "edit\n/exit\n", func(t *testing.T, dir string) {
		writeToolFixture(t, dir)
		if err := os.MkdirAll(filepath.Join(dir, ".mewcode"), 0700); err != nil {
			t.Fatalf("mkdir project permissions: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".mewcode", "permissions.yaml"), []byte("rules:\n- effect: deny\n  tool: edit_file\n  path_pattern: tmp_tool_test.txt\n"), 0600); err != nil {
			t.Fatalf("write project permissions: %v", err)
		}
	}, client)
	if !strings.Contains(out, "project denied") {
		t.Fatalf("output = %q", out)
	}
	assertSecurityFile(t, filepath.Join(dir, "tmp_tool_test.txt"), "hello tool")
}

func editThenFinalClient(final string) provider.Option {
	requests := 0
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return sseResponse(openAIToolCallSSE("call_1", "edit_file", map[string]any{"path": "tmp_tool_test.txt", "old_text": "hello tool", "new_text": "hello zy"})), nil
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":" + strconvQuote(final) + "}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})
	return provider.WithHTTPClient(client)
}

func runSecurityProject(t *testing.T, input string, prepare func(*testing.T, string), opts ...provider.Option) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if os.Getenv("HOME") == "" || os.Getenv("HOME") == "/Users/theone" {
		t.Setenv("HOME", filepath.Join(dir, "home"))
	}
	if err := os.WriteFile(filepath.Join(dir, "mewcode.yaml"), []byte(configBody("openai", "gpt-test", "http://provider.test")), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if prepare != nil {
		prepare(t, dir)
	}
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
	if errs.Len() > 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return out.String(), dir
}

func writeToolFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "tmp_tool_test.txt"), []byte("hello tool"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func assertSecurityFile(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func requestContains(request map[string]any, text string) bool {
	raw, _ := json.Marshal(request)
	return strings.Contains(string(raw), text)
}

func strconvQuote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

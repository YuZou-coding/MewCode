package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mewcode/internal/app"
	"mewcode/internal/provider"
)

func TestInstructionsSessionArchiveResumeAndNotesEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "mewcode.yaml"), []byte(configBody("openai", "gpt-test", "http://provider.test")), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".mewcode"), 0700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mewcode", "MEWCODE.md"), []byte("USER-INSTRUCTION"), 0600); err != nil {
		t.Fatalf("write user instruction: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "rules.md"), []byte("INCLUDED-PROJECT-RULE"), 0600); err != nil {
		t.Fatalf("write include: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "MEWCODE.md"), []byte("PROJECT-INSTRUCTION\n@include ./docs/rules.md"), 0600); err != nil {
		t.Fatalf("write project instruction: %v", err)
	}

	var businessBodies []map[string]any
	var notesTools []any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(body) {
			if tools, ok := body["tools"].([]any); ok {
				notesTools = tools
			}
			return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"## 用户级笔记\\n- 用户偏好：中文回答\\n- 纠正反馈：不要假装执行\\n\\n## 项目级笔记\\n- 项目知识：MewCode 是 Go 项目\\n- 参考资料：README.md\"}}]}\n\n" +
				"data: [DONE]\n\n"), nil
		}
		businessBodies = append(businessBodies, body)
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runProjectWithHome(t, project, "first question\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q", out)
	}
	if len(businessBodies) != 1 {
		t.Fatalf("business bodies = %d", len(businessBodies))
	}
	text := requestText(businessBodies[0])
	if !strings.Contains(text, "PROJECT-INSTRUCTION") || !strings.Contains(text, "USER-INSTRUCTION") || !strings.Contains(text, "INCLUDED-PROJECT-RULE") {
		t.Fatalf("request missing instructions: %s", text)
	}
	if strings.Index(text, "PROJECT-INSTRUCTION") > strings.Index(text, "USER-INSTRUCTION") {
		t.Fatalf("project instruction should precede user instruction: %s", text)
	}
	if len(notesTools) != 0 {
		t.Fatalf("notes tools = %#v", notesTools)
	}

	matches, err := filepath.Glob(filepath.Join(project, ".mewcode", "sessions", "*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("session matches=%#v err=%v", matches, err)
	}
	sessionID := filepath.Base(matches[0])
	if raw, err := os.ReadFile(filepath.Join(matches[0], "messages.jsonl")); err != nil || !strings.Contains(string(raw), "first question") {
		t.Fatalf("messages jsonl raw=%q err=%v", raw, err)
	}
	metaRaw, err := os.ReadFile(filepath.Join(matches[0], "meta.json"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if !strings.Contains(string(metaRaw), `"message_count"`) || !strings.Contains(string(metaRaw), `"updated_at"`) {
		t.Fatalf("meta = %s", metaRaw)
	}
	var resumedBody map[string]any
	resumeClient := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode resume request: %v", err)
		}
		if isNotesRequest(body) {
			return notesResponse(), nil
		}
		resumedBody = body
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"resumed ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})
	out = runProjectWithHome(t, project, "/sessions\n/resume "+sessionID+"\nwhat did I ask?\n/notes path\n/notes clear all\n/exit\n", provider.WithHTTPClient(resumeClient))
	if !strings.Contains(out, sessionID) || !strings.Contains(out, "resumed") || !strings.Contains(out, "notes path") || !strings.Contains(out, "cleared notes: all") {
		t.Fatalf("resume output = %q", out)
	}
	if !strings.Contains(requestText(resumedBody), "first question") {
		t.Fatalf("resumed request missing prior history: %s", requestText(resumedBody))
	}
	if strings.TrimSpace(readFileForE2E(filepath.Join(home, ".mewcode", "notes.md"))) != "" || strings.TrimSpace(readFileForE2E(filepath.Join(project, ".mewcode", "notes.md"))) != "" {
		t.Fatalf("notes should be cleared")
	}
}

func TestRestoreBadLinesIncompleteToolUseAndTimeGapEndToEnd(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "mewcode.yaml"), []byte(configBody("openai", "gpt-test", "http://provider.test")), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	sessionID := "20260706-010203-abcdef"
	dir := filepath.Join(project, ".mewcode", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	meta := `{"id":"` + sessionID + `","title":"old","summary":"","message_count":3,"updated_at":"2000-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	jsonl := `{"role":"user","content":"valid before bad","created_at":"2026-07-06T01:00:00Z"}` + "\n" +
		`{bad json` + "\n" +
		`{"role":"assistant","tool_calls":[{"ID":"call_1","Name":"read_file","Arguments":{"path":"README.md"}}],"created_at":"2026-07-06T01:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "messages.jsonl"), []byte(jsonl), 0600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	var body map[string]any
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if isNotesRequest(decoded) {
			return notesResponse(), nil
		}
		if body == nil {
			body = decoded
		}
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runProjectWithHome(t, project, "/resume "+sessionID+"\ncontinue\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "restore warning") || !strings.Contains(out, "bad jsonl") || !strings.Contains(out, "tool_use") {
		t.Fatalf("output missing restore warnings: %q", out)
	}
	text := requestText(body)
	if !strings.Contains(text, "valid before bad") || strings.Contains(text, "call_1") {
		t.Fatalf("unexpected restored text: %s", text)
	}
	if !strings.Contains(text, "会话恢复提醒") {
		t.Fatalf("missing time gap reminder: %s", text)
	}
	_ = time.Now()
}

func runProjectWithHome(t *testing.T, dir string, input string, opts ...provider.Option) string {
	t.Helper()
	if os.Getenv("HOME") == "" || os.Getenv("HOME") == "/Users/theone" {
		t.Setenv("HOME", filepath.Join(dir, "home"))
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
		t.Fatalf("app run: %v; stderr=%q", err, errs.String())
	}
	if errs.Len() > 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return out.String()
}

func requestText(body map[string]any) string {
	raw, _ := json.Marshal(body)
	return string(raw)
}

func readFileForE2E(path string) string {
	raw, _ := os.ReadFile(path)
	return string(raw)
}

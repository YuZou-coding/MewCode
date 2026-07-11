package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mewcode/internal/agent"
	"mewcode/internal/app"
	"mewcode/internal/chat"
	"mewcode/internal/compact"
	"mewcode/internal/provider"
	"mewcode/internal/tool"
	"mewcode/internal/tui"
)

func TestCompactExternalizesLargeToolResultEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	large := strings.Repeat("L", compact.SingleToolResultThreshold+1)
	if err := os.WriteFile(filepath.Join(tempDir, "big.txt"), []byte(large), 0644); err != nil {
		t.Fatalf("write big file: %v", err)
	}
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
			return sseResponse(openAIToolCallSSE("call_1", "read_file", map[string]any{"path": "big.txt"})), nil
		}
		secondRequest = body
		return sseResponse("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
			"data: [DONE]\n\n"), nil
	})

	out := runExistingProject(t, tempDir, "read big\ny\n/exit\n", provider.WithHTTPClient(client))
	if !strings.Contains(out, "done") {
		t.Fatalf("output = %q", out)
	}
	matches, err := filepath.Glob(filepath.Join(tempDir, compact.ArtifactDir, "*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("artifact matches=%#v err=%v", matches, err)
	}
	raw, _ := json.Marshal(secondRequest)
	text := string(raw)
	if strings.Contains(text, large) {
		t.Fatalf("second request contains full large result")
	}
	if !strings.Contains(text, compact.ArtifactDir) {
		t.Fatalf("second request missing artifact path: %.1000s", text)
	}
	if !strings.Contains(text, strings.Repeat("L", compact.ToolResultPreviewLength)) {
		t.Fatalf("second request missing 2000 char preview: %.1000s", text)
	}
}

func TestCompactSummaryAndManualCommandEndToEnd(t *testing.T) {
	session := chat.NewSession()
	for i := 0; i < 7; i++ {
		session.AddUser("recent-user-" + string(rune('0'+i)))
		session.AddAssistant(strings.Repeat("A", 12000))
	}
	fp := &compactE2EProvider{texts: []string{compactE2ESummary(), "business done"}}
	manager := &compact.Manager{Root: t.TempDir(), Provider: fp}
	ag := &agent.Agent{Provider: fp, Session: session, Compact: manager}
	events := collectCompactEvents(ag.Run(context.Background(), "latest user"))
	if !hasCompactEvent(events, agent.EventFinalResponse) {
		t.Fatalf("events = %#v", events)
	}
	if len(fp.toolsSeen) < 2 || len(fp.toolsSeen[0]) != 0 {
		t.Fatalf("summary tools = %#v", fp.toolsSeen)
	}
	business := compactMessagesText(fp.messagesSeen[1])
	if !strings.Contains(business, "上下文压缩摘要") || !strings.Contains(business, "如需文件细节请重新读取") || !strings.Contains(business, "不要根据摘要脑补代码") {
		t.Fatalf("business missing summary or boundary: %.500s", business)
	}
	for i := 2; i < 7; i++ {
		if !strings.Contains(business, "recent-user-"+string(rune('0'+i))) {
			t.Fatalf("recent user %d missing from business request", i)
		}
	}
	if !strings.Contains(business, "latest user") {
		t.Fatalf("latest user missing from business request")
	}

	manualSession := chat.NewSession()
	for i := 0; i < 7; i++ {
		manualSession.AddUser("manual-user-" + string(rune('0'+i)))
		manualSession.AddAssistant(strings.Repeat("B", 12000))
	}
	fpManual := &compactE2EProvider{texts: []string{compactE2ESummary()}}
	var out strings.Builder
	err := tui.Loop{
		Input:       strings.NewReader("/compact\n/exit\n"),
		Output:      &out,
		Errors:      &strings.Builder{},
		Session:     manualSession,
		Provider:    fpManual,
		NoTypeDelay: true,
		Compact:     &compact.Manager{Root: t.TempDir(), Provider: fpManual},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "compacted messages") || !strings.Contains(out.String(), "artifacts") {
		t.Fatalf("manual compact output = %q", out.String())
	}
	for _, message := range manualSession.Messages() {
		if message.Role == chat.RoleUser && message.Content == compact.ManualCommand {
			t.Fatalf("/compact stored in session")
		}
	}
}

func TestCompactSummaryBreakerEndToEnd(t *testing.T) {
	session := chat.NewSession()
	for i := 0; i < 7; i++ {
		session.AddUser("user-" + string(rune('0'+i)))
		session.AddAssistant(strings.Repeat("C", 12000))
	}
	fp := &compactE2EProvider{err: assertErr("summary failed")}
	manager := &compact.Manager{Root: t.TempDir(), Provider: fp}
	for i := 0; i < compact.SummaryFailureLimit+1; i++ {
		ag := &agent.Agent{Provider: fp, Session: session, Compact: manager, MaxIterations: 1}
		collectCompactEvents(ag.Run(context.Background(), "turn"))
	}
	if fp.summaryCalls != compact.SummaryFailureLimit {
		t.Fatalf("summary calls = %d, want %d", fp.summaryCalls, compact.SummaryFailureLimit)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

type compactE2EProvider struct {
	texts        []string
	err          error
	messagesSeen [][]chat.Message
	toolsSeen    [][]tool.Definition
	summaryCalls int
}

func (p *compactE2EProvider) StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan provider.StreamEvent, <-chan error) {
	copied := make([]chat.Message, len(messages))
	copy(copied, messages)
	p.messagesSeen = append(p.messagesSeen, copied)
	copiedTools := make([]tool.Definition, len(tools))
	copy(copiedTools, tools)
	p.toolsSeen = append(p.toolsSeen, copiedTools)
	if len(tools) == 0 && len(messages) == 1 && strings.Contains(messages[0].Content, "禁止调用工具") {
		p.summaryCalls++
	}
	text := "ok"
	if len(p.texts) > 0 {
		text = p.texts[0]
		p.texts = p.texts[1:]
	}
	events := make(chan provider.StreamEvent, 1)
	errs := make(chan error, 1)
	if p.err == nil {
		events <- provider.StreamEvent{Kind: provider.EventText, Text: text}
	}
	close(events)
	errs <- p.err
	return events, errs
}

func runExistingProject(t *testing.T, dir string, input string, opts ...provider.Option) string {
	t.Helper()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	if err := os.WriteFile(filepath.Join(dir, "mewcode.yaml"), []byte(configBody("openai", "gpt-test", "http://provider.test")), 0600); err != nil {
		t.Fatalf("write config: %v", err)
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
	if err := appRun(context.Background(), strings.NewReader(input), &out, &errs, opts...); err != nil {
		t.Fatalf("app run returned error: %v; stderr=%q", err, errs.String())
	}
	if errs.Len() > 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return out.String()
}

func appRun(ctx context.Context, input *strings.Reader, out *strings.Builder, errs *strings.Builder, opts ...provider.Option) error {
	return app.RunWithProviderOptions(ctx, input, out, errs, opts...)
}

func collectCompactEvents(events <-chan agent.Event) []agent.Event {
	var result []agent.Event
	for event := range events {
		result = append(result, event)
	}
	return result
}

func hasCompactEvent(events []agent.Event, kind agent.EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func compactMessagesText(messages []chat.Message) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Content)
		if message.ToolResult != nil {
			b.WriteString(string(message.ToolResult.Content))
		}
	}
	return b.String()
}

func compactE2ESummary() string {
	return "【分析草稿】draft\n【正式摘要】\n主要请求: req\n关键概念: concept\n文件代码: code\n错误修复: fix\n解决过程: process\n用户原话: quote\n待办: todo\n当前工作: current\n下一步: next"
}

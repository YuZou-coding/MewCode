package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"mewcode/internal/chat"
	"mewcode/internal/permissions"
	"mewcode/internal/prompt"
)

func (e *Engine) Fire(ctx context.Context, hookCtx Context) error {
	if e == nil {
		return nil
	}
	hookCtx.Event = normalizeEvent(hookCtx.Event)
	for _, rule := range e.rules {
		if !rule.Matches(hookCtx) || e.alreadyRan(rule) {
			continue
		}
		if rule.Once {
			e.markRan(rule)
		}
		if rule.Async {
			ruleCopy := rule
			ctxCopy := hookCtx
			go func() {
				if err := e.execute(ctx, ruleCopy, ctxCopy); err != nil {
					e.addWarning("hook %s failed: %v", normalizeRuleName(ruleCopy.Name), err)
				}
			}()
			continue
		}
		if err := e.execute(ctx, rule, hookCtx); err != nil {
			e.addWarning("hook %s failed: %v", normalizeRuleName(rule.Name), err)
		}
	}
	return nil
}

func (e *Engine) BeforeTool(ctx context.Context, hookCtx Context) BlockResult {
	if e == nil {
		return BlockResult{}
	}
	hookCtx.Event = EventToolBeforeExecute
	for _, rule := range e.rules {
		if !rule.Matches(hookCtx) || e.alreadyRan(rule) {
			continue
		}
		if rule.Once {
			e.markRan(rule)
		}
		if rule.Block != "" {
			return BlockResult{Blocked: true, Reason: Render(rule.Block, hookCtx)}
		}
		if err := e.execute(ctx, rule, hookCtx); err != nil {
			e.addWarning("hook %s failed: %v", normalizeRuleName(rule.Name), err)
		}
	}
	return BlockResult{}
}

func (e *Engine) execute(ctx context.Context, rule Rule, hookCtx Context) error {
	timeout := time.Duration(rule.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	actionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch rule.Action.Type {
	case ActionInjectPrompt:
		e.mu.Lock()
		e.prompts = append(e.prompts, prompt.InternalInstruction(Render(rule.Action.Prompt, hookCtx)))
		e.mu.Unlock()
		return nil
	case ActionShell:
		command := Render(rule.Action.Command, hookCtx)
		raw, _ := json.Marshal(map[string]string{"command": command})
		if decision, blocked := permissions.CheckDangerousCommand(permissions.Request{Tool: "run_command", Arguments: raw}); blocked {
			return errors.New(decision.Reason)
		}
		runner := e.runner
		if runner == nil {
			runner = ShellRunner{}
		}
		return runner.Run(actionCtx, command, rule.TimeoutMS)
	case ActionHTTP:
		client := e.client
		if client == nil {
			client = DefaultHTTPClient{}
		}
		return client.Do(actionCtx, valueOr(rule.Action.Method, "POST"), Render(rule.Action.URL, hookCtx), renderHeaders(rule.Action.Headers, hookCtx), Render(rule.Action.Body, hookCtx), rule.TimeoutMS)
	case ActionSubAgent:
		e.addWarning("hook %s sub_agent action is a placeholder", normalizeRuleName(rule.Name))
		return nil
	default:
		return nil
	}
}

func (e *Engine) alreadyRan(rule Rule) bool {
	if !rule.Once {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.once[normalizeRuleName(rule.Name)]
}

func (e *Engine) markRan(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.once[normalizeRuleName(rule.Name)] = true
}

func normalizeEvent(event EventName) EventName {
	return event
}

func renderHeaders(headers map[string]string, ctx Context) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	rendered := map[string]string{}
	for key, value := range headers {
		rendered[key] = Render(value, ctx)
	}
	return rendered
}

func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

type ShellRunner struct{}

func (ShellRunner) Run(ctx context.Context, command string, timeoutMS int) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}

type DefaultHTTPClient struct{}

func (DefaultHTTPClient) Do(ctx context.Context, method string, url string, headers map[string]string, body string, timeoutMS int) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("http status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func MessagesText(messages []chat.Message) string {
	var b bytes.Buffer
	for _, message := range messages {
		b.WriteString(message.Content)
	}
	return b.String()
}

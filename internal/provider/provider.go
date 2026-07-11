package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/config"
	"mewcode/internal/tool"
)

type EventKind string

const (
	EventText     EventKind = "text"
	EventThinking EventKind = "thinking"
	EventToolCall EventKind = "tool_call"
	EventStop     EventKind = "stop"
	EventUsage    EventKind = "usage"
)

type StreamEvent struct {
	Kind       EventKind
	Text       string
	ToolCall   *ToolCall
	StopReason string
	Usage      Usage
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments []byte
}

type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

type Provider interface {
	StreamChat(ctx context.Context, messages []chat.Message, tools []tool.Definition) (<-chan StreamEvent, <-chan error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Option func(*clientOptions)

type clientOptions struct {
	HTTPClient HTTPClient
}

func WithHTTPClient(client HTTPClient) Option {
	return func(options *clientOptions) {
		options.HTTPClient = client
	}
}

func collectStream(events <-chan StreamEvent, errs <-chan error, writer io.Writer) (string, error) {
	var assistant string
	for event := range events {
		if event.Kind != EventText {
			continue
		}
		if _, err := io.WriteString(writer, event.Text); err != nil {
			return "", err
		}
		assistant += event.Text
	}

	if err := <-errs; err != nil {
		return "", err
	}
	return assistant, nil
}

func newHTTPClient(options clientOptions) HTTPClient {
	if options.HTTPClient != nil {
		return options.HTTPClient
	}
	return http.DefaultClient
}

func validateStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return errors.New("provider request failed: " + resp.Status)
	}
	return fmt.Errorf("provider request failed: %s: %s", resp.Status, detail)
}

func applyOptions(opts []Option) clientOptions {
	var options clientOptions
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

func New(cfg config.Config, opts ...Option) (Provider, error) {
	return NewFactory(opts...).New(cfg)
}

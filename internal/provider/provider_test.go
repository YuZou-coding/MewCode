package provider

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCollectStreamWritesTextOnly(t *testing.T) {
	events := make(chan StreamEvent, 3)
	errs := make(chan error, 1)
	events <- StreamEvent{Kind: EventThinking, Text: "hidden"}
	events <- StreamEvent{Kind: EventText, Text: "hel"}
	events <- StreamEvent{Kind: EventText, Text: "lo"}
	close(events)
	errs <- nil

	var out bytes.Buffer
	got, err := collectStream(events, errs, &out)
	if err != nil {
		t.Fatalf("collectStream returned error: %v", err)
	}
	if got != "hello" || out.String() != "hello" {
		t.Fatalf("got %q / %q, want hello", got, out.String())
	}
}

func TestCollectStreamIgnoresToolCalls(t *testing.T) {
	events := make(chan StreamEvent, 2)
	errs := make(chan error, 1)
	events <- StreamEvent{Kind: EventToolCall, ToolCall: &ToolCall{ID: "call_1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}}
	events <- StreamEvent{Kind: EventText, Text: "done"}
	close(events)
	errs <- nil

	var out bytes.Buffer
	got, err := collectStream(events, errs, &out)
	if err != nil {
		t.Fatalf("collectStream returned error: %v", err)
	}
	if got != "done" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateStatusIncludesResponseBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Status:     "503 Service Unavailable",
		Body:       io.NopCloser(strings.NewReader(`{"error":"upstream unavailable"}`)),
	}

	err := validateStatus(resp)
	if err == nil {
		t.Fatalf("validateStatus returned nil")
	}
	if err.Error() != `provider request failed: 503 Service Unavailable: {"error":"upstream unavailable"}` {
		t.Fatalf("error = %q", err.Error())
	}
}

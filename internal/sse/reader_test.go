package sse

import (
	"io"
	"strings"
	"testing"
)

func TestReaderParsesEvents(t *testing.T) {
	reader := NewReader(strings.NewReader("event: content_block_delta\ndata: {\"text\":\"Hel\"}\n\nevent: done\ndata: [DONE]\n\n"))

	first, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if first.Name != "content_block_delta" || first.Data != `{"text":"Hel"}` {
		t.Fatalf("unexpected first event: %#v", first)
	}

	second, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if second.Name != "done" || second.Data != "[DONE]" {
		t.Fatalf("unexpected second event: %#v", second)
	}

	_, err = reader.Next()
	if err != io.EOF {
		t.Fatalf("Next error = %v, want io.EOF", err)
	}
}

func TestReaderJoinsMultiLineData(t *testing.T) {
	reader := NewReader(strings.NewReader("data: one\ndata: two\n\n"))

	event, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if event.Data != "one\ntwo" {
		t.Fatalf("Data = %q, want joined lines", event.Data)
	}
}

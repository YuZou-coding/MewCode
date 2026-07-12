package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

type memoryTransport struct {
	in      chan []byte
	out     chan []byte
	closed  chan struct{}
	closeMu sync.Mutex
}

func newMemoryTransport() *memoryTransport {
	return &memoryTransport{in: make(chan []byte, 64), out: make(chan []byte, 64), closed: make(chan struct{})}
}

func (m *memoryTransport) Send(ctx context.Context, data []byte) error {
	select {
	case m.out <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *memoryTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-m.in:
		return data, nil
	case <-m.closed:
		return nil, fmt.Errorf("closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *memoryTransport) Close() error {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
	return nil
}

func TestSessionMatchesConcurrentResponses(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close()
	go func() {
		var requests [][]byte
		for len(requests) < 20 {
			requests = append(requests, <-transport.out)
		}
		for i := len(requests) - 1; i >= 0; i-- {
			var req Request
			_ = json.Unmarshal(requests[i], &req)
			resp := Response{JSONRPC: Version, ID: req.ID, Result: json.RawMessage(`{"id":"` + req.ID.String() + `"}`)}
			raw, _ := Encode(resp)
			transport.in <- raw
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := session.Call(context.Background(), "echo", nil)
			if err != nil {
				t.Errorf("Call returned error: %v", err)
				return
			}
			if len(raw) == 0 {
				t.Errorf("empty result")
			}
		}()
	}
	wg.Wait()
	if session.PendingCount() != 0 {
		t.Fatalf("pending = %d", session.PendingCount())
	}
}

func TestSessionNotifySendsNotificationWithoutWaiting(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close()

	if err := session.Notify(context.Background(), "notifications/initialized", map[string]any{}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	select {
	case raw := <-transport.out:
		var note Notification
		if err := json.Unmarshal(raw, &note); err != nil {
			t.Fatalf("decode notification: %v", err)
		}
		if note.Method != "notifications/initialized" {
			t.Fatalf("method = %q", note.Method)
		}
		if string(raw) == "" || json.Valid(raw) == false {
			t.Fatalf("invalid notification = %q", raw)
		}
	default:
		t.Fatal("notification was not sent")
	}
	if session.PendingCount() != 0 {
		t.Fatalf("pending = %d", session.PendingCount())
	}
}

func TestSessionUnknownNotificationTimeoutCancelAndError(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close()

	unknown, _ := Encode(Response{JSONRPC: Version, ID: StringID("missing"), Result: json.RawMessage(`{}`)})
	transport.in <- unknown
	note, _ := Encode(Notification{JSONRPC: Version, Method: "server/notice"})
	transport.in <- note

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := session.Call(ctx, "never", nil); err == nil {
		t.Fatalf("timeout returned nil error")
	}
	if session.PendingCount() != 0 {
		t.Fatalf("pending after timeout = %d", session.PendingCount())
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := session.Call(ctx, "cancelled", nil); err == nil {
		t.Fatalf("cancel returned nil error")
	}
	select {
	case <-transport.out:
	default:
	}

	go func() {
		var req Request
		_ = json.Unmarshal(<-transport.out, &req)
		raw, _ := Encode(Response{JSONRPC: Version, ID: req.ID, Error: &Error{Code: -32000, Message: "boom"}})
		transport.in <- raw
	}()
	if _, err := session.Call(context.Background(), "fails", nil); err == nil {
		t.Fatalf("jsonrpc error returned nil")
	}
}

package external

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPTransportSendsAndReadsSSE(t *testing.T) {
	var contentType string
	transport := NewHTTPTransport("http://server.test/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		contentType = req.Header.Get("Content-Type")
		raw, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(raw), `"method":"initialize"`) {
			t.Fatalf("request body = %s", raw)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{\"ok\":true}}\n\n")),
		}, nil
	}))
	raw, err := transport.SendAndReceive(context.Background(), []byte(`{"jsonrpc":"2.0","id":"1","method":"initialize"}`))
	if err != nil {
		t.Fatalf("SendAndReceive returned error: %v", err)
	}
	if contentType != "application/json" {
		t.Fatalf("content type = %s", contentType)
	}
	if !strings.Contains(string(raw), `"result"`) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestHTTPTransportErrors(t *testing.T) {
	transport := NewHTTPTransport("http://server.test/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom"))}, nil
	}))
	if _, err := transport.SendAndReceive(context.Background(), []byte(`{}`)); err == nil {
		t.Fatalf("500 returned nil error")
	}

	transport = NewHTTPTransport("http://server.test/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if _, err := transport.SendAndReceive(context.Background(), []byte(`{}`)); err == nil {
		t.Fatalf("broken stream returned nil error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	transport = NewHTTPTransport("http://server.test/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	if _, err := transport.SendAndReceive(ctx, []byte(`{}`)); err == nil {
		t.Fatalf("timeout returned nil error")
	}
}

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
	transport := NewHTTPTransport("http://server.test/mcp", nil, roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
	transport := NewHTTPTransport("http://server.test/mcp", nil, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom"))}, nil
	}))
	if _, err := transport.SendAndReceive(context.Background(), []byte(`{}`)); err == nil {
		t.Fatalf("500 returned nil error")
	}

	transport = NewHTTPTransport("http://server.test/mcp", nil, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if _, err := transport.SendAndReceive(context.Background(), []byte(`{}`)); err == nil {
		t.Fatalf("broken stream returned nil error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	transport = NewHTTPTransport("http://server.test/mcp", nil, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	if _, err := transport.SendAndReceive(ctx, []byte(`{}`)); err == nil {
		t.Fatalf("timeout returned nil error")
	}
}

func TestHTTPTransportAddsHeadersAndRedactsErrorBody(t *testing.T) {
	const secret = "mewcode-secret-credential"
	transport := NewHTTPTransport("http://server.test/mcp", map[string]string{
		"CONTEXT7_API_KEY": secret,
	}, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("CONTEXT7_API_KEY"); got != secret {
			t.Fatalf("auth header = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader("rejected token " + secret)),
		}, nil
	}))
	_, err := transport.SendAndReceive(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func TestHTTPTransportReusesMCPSessionID(t *testing.T) {
	requests := 0
	transport := NewHTTPTransport("http://server.test/mcp", nil, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if got := req.Header.Get("Mcp-Session-Id"); got != "" {
				t.Fatalf("initial request session id = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":                  []string{"text/event-stream"},
					"Mcp-Session-Id":                []string{"session-123"},
					"Access-Control-Expose-Headers": []string{"Mcp-Session-Id"},
				},
				Body: io.NopCloser(strings.NewReader("data: {\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{\"ok\":true}}\n\n")),
			}, nil
		case 2, 3:
			if got := req.Header.Get("Mcp-Session-Id"); got != "session-123" {
				t.Fatalf("request %d session id = %q", requests, got)
			}
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		return nil, nil
	}))

	if _, err := transport.SendAndReceive(context.Background(), []byte(`{"jsonrpc":"2.0","id":"1","method":"initialize"}`)); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); err != nil {
		t.Fatalf("notify returned error: %v", err)
	}
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/ping"}`)); err != nil {
		t.Fatalf("second send returned error: %v", err)
	}
}

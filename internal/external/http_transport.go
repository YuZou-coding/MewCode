package external

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPTransport struct {
	url       string
	headers   map[string]string
	client    HTTPDoer
	oauth     *MCPOAuthProvider
	mu        sync.Mutex
	sessionID string
}

func NewHTTPTransport(url string, headers map[string]string, client HTTPDoer) *HTTPTransport {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{url: url, headers: headers, client: client}
}

func (t *HTTPTransport) SendAndReceive(ctx context.Context, data []byte) ([]byte, error) {
	resp, err := t.send(ctx, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("content-type")
	if strings.Contains(contentType, "text/event-stream") {
		return readSSEMessage(resp.Body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(body), nil
}

func (t *HTTPTransport) Send(ctx context.Context, data []byte) error {
	resp, err := t.send(ctx, data)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (t *HTTPTransport) send(ctx context.Context, data []byte) (*http.Response, error) {
	bearer := ""
	if t.oauth != nil && !hasHeader(t.headers, "Authorization") {
		token, err := t.oauth.BearerToken(ctx)
		if err != nil {
			return nil, err
		}
		bearer = token
	}
	resp, err := t.sendOnce(ctx, data, bearer)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && t.oauth != nil && !hasHeader(t.headers, "Authorization") {
		header := resp.Header.Clone()
		_ = resp.Body.Close()
		token, err := t.oauth.HandleUnauthorized(ctx, header)
		if err != nil {
			return nil, err
		}
		resp, err = t.sendOnce(ctx, data, token)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	if nextSessionID := resp.Header.Get("Mcp-Session-Id"); nextSessionID != "" {
		t.mu.Lock()
		t.sessionID = nextSessionID
		t.mu.Unlock()
	}
	return resp, nil
}

func (t *HTTPTransport) sendOnce(ctx context.Context, data []byte, bearerToken string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(bytes.TrimSpace(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	for name, value := range t.headers {
		req.Header.Set(name, value)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	t.mu.Lock()
	sessionID := t.sessionID
	t.mu.Unlock()
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func hasHeader(headers map[string]string, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func (t *HTTPTransport) Close() error {
	return nil
}

func readSSEMessage(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

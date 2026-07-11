package external

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPTransport struct {
	url    string
	client HTTPDoer
}

func NewHTTPTransport(url string, client HTTPDoer) *HTTPTransport {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{url: url, client: client}
}

func (t *HTTPTransport) SendAndReceive(ctx context.Context, data []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(bytes.TrimSpace(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
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

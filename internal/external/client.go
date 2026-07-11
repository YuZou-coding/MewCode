package external

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mewcode/internal/rpc"
)

type RemoteTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type CallResult struct {
	Content []ContentBlock `json:"content,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type rpcCaller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

type Client struct {
	Name        string
	timeout     time.Duration
	caller      rpcCaller
	closeFunc   func() error
	initialized bool
	discovered  bool
	tools       []RemoteTool
}

func NewClient(name string, caller rpcCaller, timeout time.Duration, closeFunc func() error) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{Name: name, caller: caller, timeout: timeout, closeFunc: closeFunc}
}

func NewClientFromConfig(ctx context.Context, cfg ServerConfig, httpClient HTTPDoer) (*Client, error) {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	switch cfg.Transport {
	case "stdio":
		transport, err := NewStdioTransport(ctx, cfg)
		if err != nil {
			return nil, err
		}
		session := rpc.NewSession(transport)
		return NewClient(cfg.Name, session, timeout, session.Close), nil
	case "http":
		transport := NewHTTPTransport(cfg.URL, httpClient)
		return NewClient(cfg.Name, httpCaller{transport: transport}, timeout, transport.Close), nil
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Transport)
	}
}

func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if _, err := c.caller.Call(ctx, "initialize", map[string]any{"client": "MewCode"}); err != nil {
		return err
	}
	c.initialized = true
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if !c.initialized {
		return nil, fmt.Errorf("external client is not initialized")
	}
	if c.discovered {
		return c.tools, nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	raw, err := c.caller.Call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []RemoteTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	c.tools = result.Tools
	c.discovered = true
	return c.tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	if !c.initialized {
		return CallResult{}, fmt.Errorf("external client is not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	raw, err := c.caller.Call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(arguments),
	})
	if err != nil {
		return CallResult{}, err
	}
	var result CallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, err
	}
	return result, nil
}

func (c *Client) Close() error {
	if c.closeFunc == nil {
		return nil
	}
	return c.closeFunc()
}

type httpCaller struct {
	transport *HTTPTransport
}

func (h httpCaller) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	req, err := rpc.NewRequest(rpc.StringID("1"), method, params)
	if err != nil {
		return nil, err
	}
	raw, err := rpc.Encode(req)
	if err != nil {
		return nil, err
	}
	respRaw, err := h.transport.SendAndReceive(ctx, raw)
	if err != nil {
		return nil, err
	}
	_, resp, _, err := rpc.Decode(respRaw)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("http response did not contain jsonrpc response")
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("jsonrpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

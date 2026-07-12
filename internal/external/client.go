package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"mewcode/internal/rpc"
	"mewcode/internal/version"
)

const MCPProtocolVersion = "2025-06-18"

type PeerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      PeerInfo       `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      PeerInfo       `json:"serverInfo"`
}

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
	Notify(ctx context.Context, method string, params any) error
}

type Client struct {
	Name            string
	timeout         time.Duration
	caller          rpcCaller
	closeFunc       func() error
	initialized     bool
	discovered      bool
	tools           []RemoteTool
	sensitiveValues []string
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
		transport, err := NewStdioTransport(context.WithoutCancel(ctx), cfg)
		if err != nil {
			return nil, err
		}
		session := rpc.NewSession(transport)
		return NewClient(cfg.Name, session, timeout, session.Close), nil
	case "http":
		headers, err := ResolveHeaders(cfg, os.LookupEnv)
		if err != nil {
			return nil, err
		}
		transport := NewHTTPTransport(cfg.URL, headers, httpClient)
		client := NewClient(cfg.Name, httpCaller{transport: transport}, timeout, transport.Close)
		for _, value := range headers {
			if value != "" {
				client.sensitiveValues = append(client.sensitiveValues, value)
			}
		}
		return client, nil
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
	raw, err := c.caller.Call(ctx, "initialize", InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      PeerInfo{Name: "MewCode", Version: version.Value},
	})
	if err != nil {
		return c.redact(err)
	}
	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("decode initialize result: %w", err)
	}
	if result.ProtocolVersion != MCPProtocolVersion {
		return fmt.Errorf("unsupported MCP protocol version: %s", result.ProtocolVersion)
	}
	if err := c.caller.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return c.redact(err)
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
		return nil, c.redact(err)
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
		return CallResult{}, c.redact(err)
	}
	var result CallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, err
	}
	return result, nil
}

func (c *Client) redact(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, value := range c.sensitiveValues {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[REDACTED]")
		}
	}
	return errors.New(message)
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

func (h httpCaller) Notify(ctx context.Context, method string, params any) error {
	note, err := rpc.NewNotification(method, params)
	if err != nil {
		return err
	}
	raw, err := rpc.Encode(note)
	if err != nil {
		return err
	}
	return h.transport.Send(ctx, raw)
}

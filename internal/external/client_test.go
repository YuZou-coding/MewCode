package external

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeCaller struct {
	calls  []string
	params []any
	err    error
}

func (f *fakeCaller) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	f.calls = append(f.calls, method)
	f.params = append(f.params, params)
	if f.err != nil {
		return nil, f.err
	}
	switch method {
	case "initialize":
		return json.RawMessage(`{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"fake","version":"1.0.0"}}`), nil
	case "tools/list":
		return json.RawMessage(`{"tools":[{"name":"echo","description":"Echoes text","inputSchema":{"type":"object","properties":{"text":{"type":"string"}}}}]}`), nil
	case "tools/call":
		raw, _ := json.Marshal(CallResult{Content: []ContentBlock{{Type: "text", Text: "hello"}}, Data: map[string]any{"value": "structured"}})
		return raw, nil
	default:
		return nil, fmt.Errorf("unexpected method: %s", method)
	}
}

func (f *fakeCaller) Notify(ctx context.Context, method string, params any) error {
	f.calls = append(f.calls, method)
	f.params = append(f.params, params)
	return f.err
}

func TestClientLifecycle(t *testing.T) {
	caller := &fakeCaller{}
	closed := false
	client := NewClient("server", caller, time.Second, func() error {
		closed = true
		return nil
	})
	if _, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{}`)); err == nil {
		t.Fatalf("call before initialize returned nil error")
	}
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %#v", tools)
	}
	result, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" || result.Data["value"] != "structured" {
		t.Fatalf("result = %#v", result)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !closed {
		t.Fatalf("close func was not called")
	}
	if strings.Join(caller.calls, ",") != "initialize,notifications/initialized,tools/list,tools/call" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	params, ok := caller.params[0].(InitializeParams)
	if !ok {
		t.Fatalf("initialize params type = %T", caller.params[0])
	}
	if params.ProtocolVersion != MCPProtocolVersion || params.ClientInfo.Name != "MewCode" || params.ClientInfo.Version == "" || params.Capabilities == nil {
		t.Fatalf("initialize params = %#v", params)
	}
}

func TestClientRejectsUnsupportedNegotiatedProtocolVersion(t *testing.T) {
	caller := &fakeCaller{}
	client := NewClient("server", caller, time.Second, nil)
	callerResponse := json.RawMessage(`{"protocolVersion":"unsupported","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}`)
	caller.err = nil
	client.caller = rpcCallerFunc{
		call:   func(context.Context, string, any) (json.RawMessage, error) { return callerResponse, nil },
		notify: func(context.Context, string, any) error { return nil },
	}
	if err := client.Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Initialize error = %v", err)
	}
	if client.initialized {
		t.Fatal("client marked initialized")
	}
}

type rpcCallerFunc struct {
	call   func(context.Context, string, any) (json.RawMessage, error)
	notify func(context.Context, string, any) error
}

func (f rpcCallerFunc) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return f.call(ctx, method, params)
}

func (f rpcCallerFunc) Notify(ctx context.Context, method string, params any) error {
	return f.notify(ctx, method, params)
}

func TestClientDiscoveryFailure(t *testing.T) {
	client := NewClient("server", &fakeCaller{err: fmt.Errorf("boom")}, time.Second, nil)
	if err := client.Initialize(context.Background()); err == nil {
		t.Fatalf("Initialize returned nil error")
	}
}

func TestClientRedactsConfiguredHeaderValuesFromErrors(t *testing.T) {
	const secret = "mewcode-secret-credential"
	caller := &fakeCaller{err: fmt.Errorf("remote rejected %s", secret)}
	client := NewClient("server", caller, time.Second, nil)
	client.sensitiveValues = []string{secret}
	err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("Initialize returned nil error")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error was not redacted: %v", err)
	}
}

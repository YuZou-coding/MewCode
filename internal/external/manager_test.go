package external

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"mewcode/internal/tool"
)

func TestManagerCachesAndCloses(t *testing.T) {
	client := NewClient("cached", &fakeCaller{}, time.Second, nil)
	manager := &Manager{clients: map[string]*Client{"cached": client}}
	first, err := manager.Client(context.Background(), "cached")
	if err != nil {
		t.Fatalf("Client first returned error: %v", err)
	}
	second, err := manager.Client(context.Background(), "cached")
	if err != nil {
		t.Fatalf("Client second returned error: %v", err)
	}
	if first != second || manager.CachedCount() != 1 {
		t.Fatalf("cache failed")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if manager.CachedCount() != 0 {
		t.Fatalf("cached after close = %d", manager.CachedCount())
	}
}

func TestManagerPrewarmReturnsBeforeDiscoveryAndRegistersTools(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	client := NewClient("slow", blockingCaller{started: started, release: release}, time.Second, nil)
	client.initialized = true
	manager := &Manager{
		configs: []ServerConfig{{Name: "slow"}},
		clients: map[string]*Client{"slow": client},
	}

	done := manager.Prewarm(context.Background(), registry, nil)
	select {
	case <-started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("prewarm did not start discovery")
	}
	select {
	case <-done:
		t.Fatal("prewarm blocked until discovery finished")
	default:
	}
	if _, err := registry.Get("external_slow_echo"); err == nil {
		t.Fatal("remote tool was registered before discovery finished")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prewarm did not finish")
	}
	if _, err := registry.Get("external_slow_echo"); err != nil {
		t.Fatalf("remote tool missing after prewarm: %v", err)
	}
}

type blockingCaller struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (c blockingCaller) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "tools/list" {
		close(c.started)
		select {
		case <-c.release:
			return json.RawMessage(`{"tools":[{"name":"echo","description":"Echoes text","inputSchema":{"type":"object"}}]}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("unexpected method: %s", method)
}

func (c blockingCaller) Notify(ctx context.Context, method string, params any) error {
	if method == "notifications/initialized" {
		return nil
	}
	return fmt.Errorf("unexpected method: %s", method)
}

func TestRemoteExecutorAdaptsTool(t *testing.T) {
	client := NewClient("alpha", &fakeCaller{}, time.Second, nil)
	client.initialized = true
	manager := &Manager{clients: map[string]*Client{"alpha": client}}
	executor := RemoteExecutor{
		ServerName: "alpha",
		LocalName:  "external_alpha_echo",
		Remote: RemoteTool{
			Name:        "echo",
			Description: "Echoes text",
			InputSchema: map[string]any{"type": "object"},
		},
		Manager: manager,
	}
	def := executor.Definition()
	if def.Name != "external_alpha_echo" || def.Schema["type"] != "object" {
		t.Fatalf("definition = %#v", def)
	}
	result := executor.Execute(context.Background(), tool.Input{Arguments: json.RawMessage(`{"text":"hello"}`)})
	if !result.OK || result.Data["text"] != "hello" || result.Data["value"] != "structured" {
		t.Fatalf("result = %#v", result)
	}
}

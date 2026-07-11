package external

import (
	"context"
	"encoding/json"
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

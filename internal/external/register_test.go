package external

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mewcode/internal/tool"
)

func TestRegisterDiscoveredToolsNamesAndConflicts(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	alpha := NewClient("alpha", &fakeCaller{}, time.Second, nil)
	alpha.initialized = true
	alpha.discovered = true
	alpha.tools = []RemoteTool{{Name: "read_file", Description: "remote read", InputSchema: map[string]any{"type": "object"}}}
	beta := NewClient("beta", &fakeCaller{}, time.Second, nil)
	beta.initialized = true
	beta.discovered = true
	beta.tools = []RemoteTool{{Name: "query", Description: "query", InputSchema: map[string]any{"type": "object"}}}
	gamma := NewClient("gamma", &fakeCaller{}, time.Second, nil)
	gamma.initialized = true
	gamma.discovered = true
	gamma.tools = []RemoteTool{{Name: "query", Description: "query", InputSchema: map[string]any{"type": "object"}}}
	manager := &Manager{clients: map[string]*Client{"alpha": alpha, "beta": beta, "gamma": gamma}, configs: []ServerConfig{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}}

	errs := RegisterDiscoveredTools(context.Background(), registry, manager)
	if len(errs) != 0 {
		t.Fatalf("errs = %#v", errs)
	}
	if _, err := registry.Get("read_file"); err != nil {
		t.Fatalf("local read_file missing: %v", err)
	}
	for _, name := range []string{"external_alpha_read_file", "external_beta_query", "external_gamma_query"} {
		if _, err := registry.Get(name); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
}

func TestDiscoverToolConnectsOnlyRequestedServerAndRegistersTools(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	alpha := NewClient("alpha", &fakeCaller{}, time.Second, nil)
	beta := NewClient("beta", &fakeCaller{}, time.Second, nil)
	manager := &Manager{
		configs: []ServerConfig{{Name: "alpha"}, {Name: "beta"}},
		clients: map[string]*Client{},
		clientFactory: func(_ context.Context, cfg ServerConfig, _ HTTPDoer) (*Client, error) {
			if cfg.Name == "alpha" {
				return alpha, nil
			}
			return beta, nil
		},
	}
	if err := RegisterDiscoveryTool(registry, manager); err != nil {
		t.Fatalf("RegisterDiscoveryTool returned error: %v", err)
	}
	if manager.CachedCount() != 0 {
		t.Fatalf("startup connected clients = %d", manager.CachedCount())
	}
	executor, err := registry.Get(DiscoverToolName)
	if err != nil {
		t.Fatal(err)
	}
	result := executor.Execute(context.Background(), tool.Input{Arguments: json.RawMessage(`{"server":"alpha"}`)})
	if !result.OK {
		t.Fatalf("discover result = %#v", result)
	}
	if manager.CachedCount() != 1 {
		t.Fatalf("connected clients = %d", manager.CachedCount())
	}
	if _, err := registry.Get("external_alpha_echo"); err != nil {
		t.Fatalf("remote tool missing: %v", err)
	}
	if _, err := registry.Get("external_beta_echo"); err == nil {
		t.Fatal("unrequested beta tool was registered")
	}
}

func TestDiscoverToolRegistersSanitizedNameCollisions(t *testing.T) {
	registry := tool.NewRegistry()
	alpha := NewClient("alpha", &fakeCaller{}, time.Second, nil)
	alpha.initialized = true
	alpha.discovered = true
	alpha.tools = []RemoteTool{
		{Name: "query-all", Description: "query hyphen", InputSchema: map[string]any{"type": "object"}},
		{Name: "query_all", Description: "query underscore", InputSchema: map[string]any{"type": "object"}},
	}
	manager := &Manager{configs: []ServerConfig{{Name: "alpha"}}, clients: map[string]*Client{"alpha": alpha}}
	if err := RegisterDiscoveryTool(registry, manager); err != nil {
		t.Fatalf("RegisterDiscoveryTool returned error: %v", err)
	}

	executor, err := registry.Get(DiscoverToolName)
	if err != nil {
		t.Fatal(err)
	}
	result := executor.Execute(context.Background(), tool.Input{Arguments: json.RawMessage(`{"server":"alpha"}`)})
	if !result.OK {
		t.Fatalf("discover result = %#v", result)
	}
	for _, name := range []string{"external_alpha_query_all", "external_alpha_query_all_2"} {
		if _, err := registry.Get(name); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
}

func TestUniqueToolNameSanitizes(t *testing.T) {
	used := map[string]bool{"external_my_server_query": true}
	name := uniqueToolName("my-server", "query", used)
	if name != "external_my_server_query_2" {
		t.Fatalf("name = %s", name)
	}
}

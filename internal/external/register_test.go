package external

import (
	"context"
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

func TestUniqueToolNameSanitizes(t *testing.T) {
	used := map[string]bool{"external_my_server_query": true}
	name := uniqueToolName("my-server", "query", used)
	if name != "external_my_server_query_2" {
		t.Fatalf("name = %s", name)
	}
}

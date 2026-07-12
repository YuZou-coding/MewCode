package external

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"mewcode/internal/tool"
)

func TestToolSearchRegistersMatchingToolsOnlyOnce(t *testing.T) {
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	client := NewClient("alpha", &fakeCaller{}, time.Second, nil)
	client.initialized = true
	client.discovered = true
	client.tools = []RemoteTool{
		{Name: "query", Description: "Search knowledge", InputSchema: map[string]any{"type": "object"}},
		{Name: "write", Description: "Write knowledge", InputSchema: map[string]any{"type": "object"}},
	}
	manager := &Manager{clients: map[string]*Client{"alpha": client}, configs: []ServerConfig{{Name: "alpha"}}}
	search := NewToolSearch(manager, registry)

	result := search.Execute(context.Background(), tool.Input{Arguments: []byte(`{"query":"query"}`)})
	if !result.OK {
		t.Fatalf("search result = %#v", result)
	}
	if _, err := registry.Get("external_alpha_query"); err != nil {
		t.Fatalf("matching tool missing: %v", err)
	}
	if _, err := registry.Get("external_alpha_write"); err == nil {
		t.Fatal("nonmatching tool was registered")
	}

	result = search.Execute(context.Background(), tool.Input{Arguments: []byte(`{"query":"select:external_alpha_query"}`)})
	if !result.OK {
		t.Fatalf("repeat search result = %#v", result)
	}
	if _, err := registry.Get("external_alpha_query_2"); err == nil {
		t.Fatal("duplicate tool was registered")
	}
}

func TestToolSearchIsolatesServerFailures(t *testing.T) {
	registry := tool.NewRegistry()
	good := NewClient("good", &fakeCaller{}, time.Second, nil)
	good.initialized = true
	bad := NewClient("bad", &fakeCaller{err: fmt.Errorf("unavailable")}, time.Second, nil)
	bad.initialized = true
	manager := &Manager{
		clients: map[string]*Client{"good": good, "bad": bad},
		configs: []ServerConfig{{Name: "good"}, {Name: "bad"}},
	}
	search := NewToolSearch(manager, registry)

	result := search.Execute(context.Background(), tool.Input{Arguments: []byte(`{"query":"echo"}`)})
	if !result.OK {
		t.Fatalf("search result = %#v", result)
	}
	if _, err := registry.Get("external_good_echo"); err != nil {
		t.Fatalf("healthy server tool missing: %v", err)
	}
	errors, ok := result.Data["errors"].(map[string]string)
	if !ok || errors["bad"] != "unavailable" {
		t.Fatalf("errors = %#v", result.Data["errors"])
	}
}

func TestToolSearchDoesNotConnectBeforeExecution(t *testing.T) {
	manager := NewManager([]ServerConfig{{Name: "alpha"}}, nil)
	registry := tool.NewRegistry()
	if err := registry.Register(NewToolSearch(manager, registry)); err != nil {
		t.Fatalf("register tool search: %v", err)
	}
	if manager.CachedCount() != 0 {
		t.Fatalf("cached clients = %d, want 0", manager.CachedCount())
	}
	if _, err := registry.Get(ToolSearchName); err != nil {
		t.Fatalf("tool_search missing: %v", err)
	}
}

func TestToolSearchInvalidExactSelectionDoesNotScanServers(t *testing.T) {
	manager := &Manager{
		clients: map[string]*Client{},
		configs: []ServerConfig{{Name: "alpha"}, {Name: "beta"}},
	}
	search := NewToolSearch(manager, tool.NewRegistry())

	servers := search.candidateServers("select:external_missing_query", true, "external_missing_query")
	if len(servers) != 0 {
		t.Fatalf("candidate servers = %#v, want none", servers)
	}
	result := search.Execute(context.Background(), tool.Input{Arguments: []byte(`{"query":"select:external_missing_query"}`)})
	if result.OK || result.Error == nil || result.Error.Code != "tool_not_found" {
		t.Fatalf("search result = %#v", result)
	}
}

func TestToolSearchUsesServerMetadataAndExplainsRecommendation(t *testing.T) {
	registry := tool.NewRegistry()
	docs := NewClient("context7", &fakeCaller{}, time.Second, nil)
	docs.initialized = true
	docs.discovered = true
	docs.tools = []RemoteTool{
		{Name: "resolve_library_id", Description: "Resolve package names for current official documentation", InputSchema: map[string]any{"type": "object"}},
	}
	database := NewClient("database", &fakeCaller{}, time.Second, nil)
	database.initialized = true
	database.discovered = true
	database.tools = []RemoteTool{
		{Name: "query", Description: "Run SQL queries", InputSchema: map[string]any{"type": "object"}},
	}
	manager := &Manager{
		clients: map[string]*Client{"context7": docs, "database": database},
		configs: []ServerConfig{
			{Name: "context7", Description: "Current official library documentation", Capabilities: []string{"docs", "library-docs"}, Keywords: []string{"react", "useeffect", "api reference"}, Examples: []string{"Look up React useEffect latest docs"}},
			{Name: "database", Description: "Application database access", Capabilities: []string{"database"}, Keywords: []string{"sql"}},
		},
	}
	search := NewToolSearch(manager, registry)

	result := search.Execute(context.Background(), tool.Input{Arguments: []byte(`{"query":"React useEffect latest documentation"}`)})
	if !result.OK {
		t.Fatalf("search result = %#v", result)
	}
	tools, ok := result.Data["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", result.Data["tools"])
	}
	if tools[0]["server"] != "context7" || tools[0]["name"] != "external_context7_resolve_library_id" || tools[0]["recommended"] != true {
		t.Fatalf("recommended tool = %#v", tools[0])
	}
	if score, ok := tools[0]["score"].(int); !ok || score <= 0 {
		t.Fatalf("score = %#v", tools[0]["score"])
	}
	matched, ok := tools[0]["matched"].([]string)
	if !ok || len(matched) == 0 {
		t.Fatalf("matched = %#v", tools[0]["matched"])
	}
	if capabilities, ok := tools[0]["capabilities"].([]string); !ok || strings.Join(capabilities, ",") != "docs,library-docs" {
		t.Fatalf("capabilities = %#v", tools[0]["capabilities"])
	}
	if _, err := registry.Get("external_context7_resolve_library_id"); err != nil {
		t.Fatalf("docs tool missing: %v", err)
	}
	if _, err := registry.Get("external_database_query"); err == nil {
		t.Fatal("database server was connected despite metadata mismatch")
	}
}

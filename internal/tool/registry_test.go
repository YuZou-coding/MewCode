package tool

import "testing"

func TestRegistryRegistersAndLooksUpTools(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ReadFile{}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := registry.Get("read_file")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Definition().Name != "read_file" {
		t.Fatalf("got tool %q", got.Definition().Name)
	}
}

func TestRegistryRejectsDuplicateTools(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ReadFile{}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	err := registry.Register(ReadFile{})
	if err == nil || err.Error() != "tool already registered: read_file" {
		t.Fatalf("got %v", err)
	}
}

func TestRegistryMissingTool(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Get("missing_tool")
	if err == nil || err.Error() != "tool not found: missing_tool" {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultRegistryContainsCoreTools(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	want := []string{"read_file", "write_file", "edit_file", "run_command", "find_files", "search_code"}
	defs := registry.Definitions()
	if len(defs) != len(want) {
		t.Fatalf("len(Definitions()) = %d", len(defs))
	}
	for _, name := range want {
		if _, err := registry.Get(name); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

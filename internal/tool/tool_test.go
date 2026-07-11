package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefinitionSchemaHelpers(t *testing.T) {
	def := ReadFile{}.Definition()
	if def.Name != "read_file" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Schema["type"] != "object" {
		t.Fatalf("schema type = %#v", def.Schema["type"])
	}
}

func TestDecodeArgs(t *testing.T) {
	args, err := DecodeArgs[readFileArgs](json.RawMessage(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("DecodeArgs returned error: %v", err)
	}
	if args.Path != "README.md" {
		t.Fatalf("Path = %q", args.Path)
	}
}

func TestCoreToolDescriptionsContainStrengthenedRules(t *testing.T) {
	checks := map[string]string{
		"read_file":   "读取",
		"write_file":  "确认",
		"edit_file":   "先读取",
		"run_command": "确认",
	}
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry returned error: %v", err)
	}
	for _, def := range registry.Definitions() {
		want, ok := checks[def.Name]
		if !ok {
			continue
		}
		if !strings.Contains(def.Description, want) {
			t.Fatalf("%s description %q missing %q", def.Name, def.Description, want)
		}
	}
}

package skill

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mewcode/internal/tool"
)

type fakeTool struct{ name string }

func (f fakeTool) Definition() tool.Definition {
	return tool.Definition{Name: f.name, Description: f.name, Schema: tool.ObjectSchema(nil, map[string]any{})}
}

func (f fakeTool) Execute(ctx context.Context, input tool.Input) tool.Result {
	return tool.OK(nil)
}

func TestParseMarkdownKeepsMetadataAndBody(t *testing.T) {
	raw := []byte("---\nname: Demo\ndescription: Demo skill\ntools: [read_file, search_code]\nmode: isolated\nmodel: gpt-demo\ncontext: recent\n---\nKeep this SOP.\n")
	skill, err := ParseMarkdown("/tmp/demo.md", SourceProject, raw)
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if skill.Name != "demo" || skill.Description != "Demo skill" || skill.Mode != ModeIsolated || skill.Model != "gpt-demo" || skill.Context != ContextRecent {
		t.Fatalf("skill = %#v", skill)
	}
	if strings.TrimSpace(skill.Body) != "Keep this SOP." {
		t.Fatalf("body = %q", skill.Body)
	}
	if strings.Join(skill.Tools, ",") != "read_file,search_code" {
		t.Fatalf("tools = %#v", skill.Tools)
	}
}

func TestLoadDiscoversSourcesOverlaysAndWarns(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".mewcode", "skills", "demo.md"), "demo", "user skill", "USER")
	writeSkill(t, filepath.Join(project, ".mewcode", "skills", "demo", "SKILL.md"), "demo", "project skill", "PROJECT")
	if err := os.MkdirAll(filepath.Join(project, ".mewcode", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mewcode", "skills", "bad.md"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(project, home, registry)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Skills["demo"].Source != SourceProject || strings.TrimSpace(loaded.Skills["demo"].Body) != "PROJECT" {
		t.Fatalf("overlay failed: %#v", loaded.Skills["demo"])
	}
	if len(loaded.Warnings) == 0 || !strings.Contains(loaded.Warnings[0], "bad.md") {
		t.Fatalf("warnings = %#v", loaded.Warnings)
	}
	if _, ok := loaded.Skills["commit"]; !ok {
		t.Fatalf("missing builtin commit")
	}
}

func TestValidateToolsFailsForMissingTool(t *testing.T) {
	registry := tool.NewRegistry()
	err := ValidateTools(map[string]Skill{"bad": {Name: "bad", Tools: []string{"missing_tool"}}}, registry)
	if err == nil || !strings.Contains(err.Error(), "missing_tool") {
		t.Fatalf("err = %v", err)
	}
}

func TestManagerContextAndToolFiltering(t *testing.T) {
	manager := NewManager("", "", LoadResult{Skills: map[string]Skill{
		"a": {Name: "a", Description: "A", Tools: []string{"read_file", "search_code"}, Body: "SOP A"},
		"b": {Name: "b", Description: "B", Tools: []string{"read_file", "find_files"}, Body: "SOP B"},
	}})
	if _, err := manager.Activate("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Activate("b"); err != nil {
		t.Fatal(err)
	}
	defs := manager.FilterDefinitions([]tool.Definition{{Name: "read_file"}, {Name: "search_code"}, {Name: "find_files"}, {Name: LoadToolName}})
	if len(defs) != 2 || defs[0].Name != "read_file" || defs[1].Name != LoadToolName {
		t.Fatalf("defs = %#v", defs)
	}
	joined := manager.ContextMessages()[0].Content + manager.ContextMessages()[1].Content
	if !strings.Contains(joined, "a: A") || !strings.Contains(joined, "SOP A") {
		t.Fatalf("context = %s", joined)
	}
}

func TestLoadToolActivatesSkill(t *testing.T) {
	manager := NewManager("", "", LoadResult{Skills: map[string]Skill{
		"review": {Name: "review", Description: "Review", Mode: ModeShared, Context: ContextRecent, Body: "Review SOP"},
	}})
	result := LoadTool{Manager: manager}.Execute(context.Background(), tool.Input{Arguments: []byte(`{"name":"review"}`)})
	if !result.OK || manager.ActiveCount() != 1 || result.Data["name"] != "review" {
		t.Fatalf("result=%#v active=%d", result, manager.ActiveCount())
	}
}

func TestScriptToolUsesJSONStdinStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "echo_json.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\npython3 -c 'import sys,json; data=json.load(sys.stdin); print(json.dumps({\"seen\": data[\"name\"]}))'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	result := ScriptTool{Spec: ScriptToolSpec{Name: "demo_echo", Command: "echo_json.sh", Schema: tool.ObjectSchema(nil, nil)}, Dir: dir}.Execute(context.Background(), tool.Input{Arguments: []byte(`{"name":"mew"}`)})
	if !result.OK || result.Data["seen"] != "mew" {
		t.Fatalf("result = %#v error=%#v", result, result.Error)
	}
}

func TestRefreshSkillReloadsSourceFile(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mewcode", "skills", "hot.md")
	writeSkill(t, path, "hot", "hot skill", "OLD SOP")
	registry, err := tool.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(project, t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(project, t.TempDir(), loaded)
	writeSkill(t, path, "hot", "hot skill", "NEW SOP")
	refreshed, err := manager.RefreshSkill("hot")
	if err != nil {
		t.Fatalf("RefreshSkill: %v", err)
	}
	if !strings.Contains(refreshed.Body, "NEW SOP") {
		t.Fatalf("body = %q", refreshed.Body)
	}
}

func TestScriptToolFailureReturnsStructuredError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho nope >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	result := ScriptTool{Spec: ScriptToolSpec{Name: "demo_fail", Command: "fail.sh", Schema: tool.ObjectSchema(nil, nil)}, Dir: dir}.Execute(context.Background(), tool.Input{Arguments: []byte(`{}`)})
	if result.OK || result.Error == nil || result.Error.Code != "script_failed" || !strings.Contains(result.Error.Message, "exit status") {
		t.Fatalf("result = %#v", result)
	}
}

func writeSkill(t *testing.T, path, name, description, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\ntools: [read_file]\nmode: shared\ncontext: recent\n---\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

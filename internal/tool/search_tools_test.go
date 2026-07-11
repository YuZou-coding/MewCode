package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindFiles(t *testing.T) {
	if (FindFiles{}).Definition().Name != "find_files" {
		t.Fatalf("unexpected find_files definition")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	result := FindFiles{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"pattern": filepath.Join(dir, "*.go")})})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	matches := result.Data["matches"].([]string)
	if len(matches) != 1 || filepath.Base(matches[0]) != "main.go" {
		t.Fatalf("matches = %#v", matches)
	}

	result = FindFiles{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"pattern": filepath.Join(dir, "*.none")})})
	if !result.OK || len(result.Data["matches"].([]string)) != 0 {
		t.Fatalf("empty result = %#v", result)
	}
}

func TestSearchCode(t *testing.T) {
	if (SearchCode{}).Definition().Name != "search_code" {
		t.Fatalf("unexpected search_code definition")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	result := SearchCode{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"root": dir, "pattern": "func main"})})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	matches := result.Data["matches"].([]SearchMatch)
	if len(matches) != 1 || matches[0].Path != path || matches[0].Line != 2 {
		t.Fatalf("matches = %#v", matches)
	}

	result = SearchCode{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"root": dir, "pattern": "missing"})})
	if !result.OK || len(result.Data["matches"].([]SearchMatch)) != 0 {
		t.Fatalf("empty result = %#v", result)
	}
}

func TestSearchCodeInvalidRegex(t *testing.T) {
	result := SearchCode{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"pattern": "[", "regex": true})})
	if result.OK || result.Error == nil || result.Error.Code != "invalid_pattern" {
		t.Fatalf("result = %#v", result)
	}
}

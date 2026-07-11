package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEditFileUniqueMatch(t *testing.T) {
	if (EditFile{}).Definition().Name != "edit_file" {
		t.Fatalf("unexpected edit_file definition")
	}
	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := EditFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{
		"path":     path,
		"old_text": "world",
		"new_text": "MewCode",
	})})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if result.Data["replacements"] != 1 {
		t.Fatalf("replacements = %#v", result.Data["replacements"])
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "hello MewCode" {
		t.Fatalf("content = %q", content)
	}
}

func TestEditFileNoMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.txt")
	original := []byte("hello world")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := EditFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{
		"path":     path,
		"old_text": "missing",
		"new_text": "MewCode",
	})})
	if result.OK || result.Error == nil || result.Error.Message != "old_text not found" {
		t.Fatalf("result = %#v", result)
	}
	assertFileContent(t, path, string(original))
}

func TestEditFileMultipleMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.txt")
	original := "hello hello"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := EditFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{
		"path":     path,
		"old_text": "hello",
		"new_text": "MewCode",
	})})
	if result.OK || result.Error == nil || result.Error.Message != "old_text matched multiple times" {
		t.Fatalf("result = %#v", result)
	}
	assertFileContent(t, path, original)
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

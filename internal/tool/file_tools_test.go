package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	if (ReadFile{}).Definition().Name != "read_file" {
		t.Fatalf("unexpected read_file definition")
	}
	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := ReadFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"path": path})})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if result.Data["content"] != "hello" {
		t.Fatalf("content = %#v", result.Data["content"])
	}
}

func TestReadFileMissing(t *testing.T) {
	result := ReadFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"path": filepath.Join(t.TempDir(), "missing.txt")})})
	if result.OK || result.Error == nil || result.Error.Code != "file_not_found" {
		t.Fatalf("result = %#v", result)
	}
}

func TestWriteFileCreatesAndOverwrites(t *testing.T) {
	if (WriteFile{}).Definition().Name != "write_file" {
		t.Fatalf("unexpected write_file definition")
	}
	path := filepath.Join(t.TempDir(), "hello.txt")
	write := WriteFile{}

	result := write.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"path": path, "content": "first"})})
	if !result.OK {
		t.Fatalf("create result = %#v", result)
	}
	result = write.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"path": path, "content": "second"})})
	if !result.OK {
		t.Fatalf("overwrite result = %#v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "second" {
		t.Fatalf("content = %q", content)
	}
}

func TestWriteFileMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "hello.txt")
	result := WriteFile{}.Execute(context.Background(), Input{Arguments: mustJSON(map[string]any{"path": path, "content": "hello"})})
	if result.OK || result.Error == nil || result.Error.Code != "write_failed" {
		t.Fatalf("result = %#v", result)
	}
}

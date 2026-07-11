package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrdersProjectBeforeUserAndExpandsInclude(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".mewcode"), 0700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mewcode", FileName), []byte("user rule"), 0600); err != nil {
		t.Fatalf("write user: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "rules.md"), []byte("included project rule"), 0600); err != nil {
		t.Fatalf("write include: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, FileName), []byte("project rule\n@include ./docs/rules.md"), 0600); err != nil {
		t.Fatalf("write project: %v", err)
	}

	result := Load(project, home)
	if len(result.Blocks) != 2 {
		t.Fatalf("blocks = %#v warnings=%#v", result.Blocks, result.Warnings)
	}
	if result.Blocks[0].Source != "project" || !strings.Contains(result.Blocks[0].Content, "included project rule") {
		t.Fatalf("project block = %#v", result.Blocks[0])
	}
	if result.Blocks[1].Source != "user" || result.Blocks[1].Content != "user rule" {
		t.Fatalf("user block = %#v", result.Blocks[1])
	}
}

func TestIncludeBlocksOutsideRootAndDepth(t *testing.T) {
	project := t.TempDir()
	outside := filepath.Join(filepath.Dir(project), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, FileName), []byte("@include ../secret.md"), 0600); err != nil {
		t.Fatalf("write project: %v", err)
	}
	result := Load(project, t.TempDir())
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, "\n"), "outside root") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}

	deep := t.TempDir()
	for i := 0; i < 7; i++ {
		name := filepath.Join(deep, string(rune('a'+i))+".md")
		next := string(rune('a'+i+1)) + ".md"
		body := "leaf"
		if i < 6 {
			body = "@include " + next
		}
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatalf("write depth: %v", err)
		}
	}
	content, warnings := loadFile(filepath.Join(deep, "a.md"), deep, deep, 0, map[string]bool{})
	if strings.Contains(content, "leaf") {
		t.Fatalf("unexpected deep content = %q", content)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "depth exceeded") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

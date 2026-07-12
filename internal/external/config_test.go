package external

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServersFile(t *testing.T) {
	root := t.TempDir()
	path := ServersFile(root)
	if path != filepath.Join(root, ".mewcode", "mcp_servers.yaml") {
		t.Fatalf("ServersFile = %s", path)
	}
	servers, err := LoadServersFile(path)
	if err != nil || len(servers) != 0 {
		t.Fatalf("missing servers = %#v err=%v", servers, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `servers:
- name: local
  transport: stdio
  command: /bin/echo
  args: ["hello", "world"]
  env:
    MEWCODE_TEST: yes
  timeout_ms: 1500
- name: remote
  transport: http
  url: http://127.0.0.1:1234/mcp
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write servers: %v", err)
	}
	servers, err = LoadServersFile(path)
	if err != nil {
		t.Fatalf("LoadServersFile returned error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %#v", servers)
	}
	if servers[0].Transport != "stdio" || servers[0].Command != "/bin/echo" || len(servers[0].Args) != 2 || servers[0].Env["MEWCODE_TEST"] != "yes" || servers[0].TimeoutMS != 1500 {
		t.Fatalf("stdio config = %#v", servers[0])
	}
	if servers[1].Transport != "http" || servers[1].URL == "" {
		t.Fatalf("http config = %#v", servers[1])
	}
}

func TestValidateServersErrors(t *testing.T) {
	cases := []struct {
		name    string
		servers []ServerConfig
	}{
		{name: "missing name", servers: []ServerConfig{{Transport: "stdio", Command: "x"}}},
		{name: "duplicate", servers: []ServerConfig{{Name: "a", Transport: "stdio", Command: "x"}, {Name: "a", Transport: "http", URL: "http://x"}}},
		{name: "stdio command", servers: []ServerConfig{{Name: "a", Transport: "stdio"}}},
		{name: "http url", servers: []ServerConfig{{Name: "a", Transport: "http"}}},
		{name: "unknown", servers: []ServerConfig{{Name: "a", Transport: "tcp"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateServers(tc.servers); err == nil {
				t.Fatalf("ValidateServers returned nil")
			}
		})
	}
}

func TestLoadServersFileSyntaxError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.yaml")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, err := LoadServersFile(path); err == nil {
		t.Fatalf("bad yaml returned nil error")
	}
}

func TestLoadMergedServersProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	userPath := UserServersFile(home)
	projectPath := ServersFile(project)
	if err := os.MkdirAll(filepath.Dir(userPath), 0700); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(projectPath), 0700); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(userPath, []byte(`servers:
- name: shared
  transport: http
  url: http://user.example/mcp
- name: user-only
  transport: stdio
  command: /bin/echo
`), 0600); err != nil {
		t.Fatalf("write user: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte(`servers:
- name: shared
  transport: http
  url: http://project.example/mcp
- name: project-only
  transport: http
  url: http://project-only.example/mcp
`), 0600); err != nil {
		t.Fatalf("write project: %v", err)
	}

	servers, err := LoadMergedServers(project, home)
	if err != nil {
		t.Fatalf("LoadMergedServers returned error: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("servers = %#v", servers)
	}
	byName := map[string]ServerConfig{}
	for _, server := range servers {
		byName[server.Name] = server
	}
	if byName["shared"].URL != "http://project.example/mcp" {
		t.Fatalf("shared was not overridden by project: %#v", byName["shared"])
	}
	if byName["user-only"].Command != "/bin/echo" || byName["project-only"].URL == "" {
		t.Fatalf("merged servers = %#v", byName)
	}
}

func TestLoadMergedServersPrefersMCPConfigAndFallsBackToLegacy(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	for _, root := range []string{home, project} {
		if err := os.MkdirAll(filepath.Join(root, ".mewcode"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".mewcode", "servers.yaml"), []byte("servers:\n- name: legacy\n  transport: stdio\n  command: /bin/echo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mewcode", "mcp_servers.yaml"), []byte("servers:\n- name: user-preferred\n  transport: stdio\n  command: /bin/echo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mewcode", "mcp_servers.yaml"), []byte("servers:\n- name: preferred\n  transport: stdio\n  command: /bin/echo\n"), 0600); err != nil {
		t.Fatal(err)
	}

	servers, warnings, err := LoadMergedMCPServers(project, home)
	if err != nil {
		t.Fatalf("LoadMergedMCPServers returned error: %v", err)
	}
	if len(servers) != 2 || servers[0].Name != "user-preferred" || servers[1].Name != "preferred" {
		t.Fatalf("servers = %#v", servers)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if err := os.Remove(filepath.Join(home, ".mewcode", "mcp_servers.yaml")); err != nil {
		t.Fatal(err)
	}
	servers, warnings, err = LoadMergedMCPServers(project, home)
	if err != nil {
		t.Fatalf("legacy fallback returned error: %v", err)
	}
	if len(servers) != 2 || servers[0].Name != "legacy" || len(warnings) != 1 || warnings[0] == "" {
		t.Fatalf("legacy fallback servers=%#v warnings=%#v", servers, warnings)
	}
}

package external

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStdioTransportWithFakeServer(t *testing.T) {
	server := buildFakeStdioServer(t)
	transport, err := NewStdioTransport(context.Background(), ServerConfig{
		Name:      "stdio",
		Transport: "stdio",
		Command:   server,
		Env:       map[string]string{"MEWCODE_STDIO_ENV": "visible"},
	})
	if err != nil {
		t.Fatalf("NewStdioTransport returned error: %v", err)
	}
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":"1","method":"env"}`+"\n")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	raw, err := transport.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if !strings.Contains(string(raw), "visible") {
		t.Fatalf("raw = %s", raw)
	}
	_ = transport.Close()
	if transport.cmd.ProcessState == nil {
		t.Fatalf("process state is nil after close")
	}
	if !strings.Contains(transport.Stderr(), "fake server ready") {
		t.Fatalf("stderr = %q", transport.Stderr())
	}
}

func TestStdioTransportExitError(t *testing.T) {
	server := buildFakeStdioServer(t)
	transport, err := NewStdioTransport(context.Background(), ServerConfig{Name: "stdio", Transport: "stdio", Command: server, Args: []string{"exit"}})
	if err != nil {
		t.Fatalf("NewStdioTransport returned error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := transport.Receive(ctx); err == nil {
		t.Fatalf("Receive returned nil error")
	}
	_ = transport.Close()
}

func buildFakeStdioServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	binary := filepath.Join(dir, "fake-stdio")
	body := `package main
import (
	"bufio"
	"fmt"
	"os"
)
func main() {
	fmt.Fprintln(os.Stderr, "fake server ready")
	if len(os.Args) > 1 && os.Args[1] == "exit" {
		fmt.Fprintln(os.Stderr, "fake failure")
		os.Exit(2)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{\"env\":%q}}\n", os.Getenv("MEWCODE_STDIO_ENV"))
	}
}
`
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatalf("write fake server: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", binary, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake server: %v output=%s", err, output)
	}
	return binary
}

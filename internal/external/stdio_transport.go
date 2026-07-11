package external

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	stderr bytes.Buffer
	mu     sync.Mutex
}

func NewStdioTransport(ctx context.Context, cfg ServerConfig) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	transport := &StdioTransport{cmd: cmd, stdin: stdin, reader: bufio.NewReader(stdout)}
	cmd.Stderr = &transport.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return transport, nil
}

func (t *StdioTransport) Send(ctx context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := t.stdin.Write(data)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *StdioTransport) Receive(ctx context.Context) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := t.reader.ReadBytes('\n')
		done <- result{data: bytes.TrimSpace(line), err: err}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			if t.stderr.Len() > 0 {
				return nil, fmt.Errorf("%w: %s", result.err, t.stderr.String())
			}
			return nil, result.err
		}
		return result.data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *StdioTransport) Close() error {
	_ = t.stdin.Close()
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return t.cmd.Wait()
}

func (t *StdioTransport) Stderr() string {
	return t.stderr.String()
}

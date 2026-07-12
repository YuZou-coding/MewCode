package external

import (
	"context"
	"fmt"
	"sync"

	"mewcode/internal/tool"
)

type Manager struct {
	configs       []ServerConfig
	httpClient    HTTPDoer
	clientFactory func(context.Context, ServerConfig, HTTPDoer) (*Client, error)
	clients       map[string]*Client
	mu            sync.Mutex
}

func NewManager(configs []ServerConfig, httpClient HTTPDoer) *Manager {
	return &Manager{configs: configs, httpClient: httpClient, clients: map[string]*Client{}}
}

func (m *Manager) Client(ctx context.Context, name string) (*Client, error) {
	m.mu.Lock()
	if client := m.clients[name]; client != nil {
		m.mu.Unlock()
		return client, nil
	}
	var cfg *ServerConfig
	for index := range m.configs {
		if m.configs[index].Name == name {
			cfg = &m.configs[index]
			break
		}
	}
	m.mu.Unlock()
	if cfg == nil {
		return nil, fmt.Errorf("external server not configured: %s", name)
	}
	factory := m.clientFactory
	if factory == nil {
		factory = NewClientFromConfig
	}
	client, err := factory(context.WithoutCancel(ctx), *cfg, m.httpClient)
	if err != nil {
		return nil, err
	}
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	if _, err := client.ListTools(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	m.mu.Lock()
	if existing := m.clients[name]; existing != nil {
		m.mu.Unlock()
		_ = client.Close()
		return existing, nil
	}
	m.clients[name] = client
	m.mu.Unlock()
	return client, nil
}

func (m *Manager) Discover(ctx context.Context) (map[string][]RemoteTool, map[string]error) {
	discovered := map[string][]RemoteTool{}
	errs := map[string]error{}
	for _, cfg := range m.configs {
		client, err := m.Client(ctx, cfg.Name)
		if err != nil {
			errs[cfg.Name] = err
			continue
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			errs[cfg.Name] = err
			continue
		}
		discovered[cfg.Name] = tools
	}
	return discovered, errs
}

func (m *Manager) Prewarm(ctx context.Context, registry *tool.Registry, report func(string, error)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		var wait sync.WaitGroup
		for _, cfg := range m.configs {
			server := cfg.Name
			wait.Add(1)
			go func() {
				defer wait.Done()
				client, err := m.Client(ctx, server)
				if err != nil {
					if report != nil {
						report(server, err)
					}
					return
				}
				tools, err := client.ListTools(ctx)
				if err != nil {
					if report != nil {
						report(server, err)
					}
					return
				}
				for _, remote := range tools {
					if err := registerRemoteTool(registry, m, server, remote); err != nil && report != nil {
						report(server+"/"+remote.Name, err)
					}
				}
			}()
		}
		wait.Wait()
		close(done)
	}()
	return done
}

func (m *Manager) Close() error {
	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.clients = map[string]*Client{}
	m.mu.Unlock()
	var first error
	for _, client := range clients {
		if err := client.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) CachedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clients)
}

func (m *Manager) ServerNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.configs))
	for _, cfg := range m.configs {
		names = append(names, cfg.Name)
	}
	return names
}

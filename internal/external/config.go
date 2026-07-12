package external

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const ServersPath = ".mewcode/servers.yaml"

type ServerConfig struct {
	Name      string
	Transport string
	Command   string
	Args      []string
	URL       string
	Env       map[string]string
	Headers   map[string]string
	TimeoutMS int
}

func ServersFile(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ServersPath)
}

func UserServersFile(home string) string {
	if home == "" {
		if detected, err := os.UserHomeDir(); err == nil {
			home = detected
		}
	}
	return filepath.Join(home, ServersPath)
}

func LoadMergedServers(projectRoot string, homeDir string) ([]ServerConfig, error) {
	user, err := LoadServersFile(UserServersFile(homeDir))
	if err != nil {
		return nil, fmt.Errorf("user servers: %w", err)
	}
	project, err := LoadServersFile(ServersFile(projectRoot))
	if err != nil {
		return nil, fmt.Errorf("project servers: %w", err)
	}
	merged := make([]ServerConfig, 0, len(user)+len(project))
	indexByName := map[string]int{}
	for _, server := range user {
		indexByName[server.Name] = len(merged)
		merged = append(merged, server)
	}
	for _, server := range project {
		if index, ok := indexByName[server.Name]; ok {
			merged[index] = server
			continue
		}
		indexByName[server.Name] = len(merged)
		merged = append(merged, server)
	}
	return merged, nil
}

func LoadServersFile(path string) ([]ServerConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var servers []ServerConfig
	var current *ServerConfig
	var nestedMap string
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "servers:" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			if current != nil {
				servers = append(servers, *current)
			}
			current = &ServerConfig{Env: map[string]string{}, Headers: map[string]string{}}
			nestedMap = ""
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if trimmed == "" {
				continue
			}
		}
		if current == nil {
			return nil, fmt.Errorf("invalid server config line %d", lineNumber)
		}
		if trimmed == "env:" {
			nestedMap = "env"
			continue
		}
		if trimmed == "headers:" {
			nestedMap = "headers"
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("invalid server config line %d", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if nestedMap != "" && strings.HasPrefix(line, "    ") {
			if nestedMap == "env" {
				current.Env[key] = cleanYAMLValue(value)
			} else {
				current.Headers[key] = cleanYAMLValue(value)
			}
			continue
		}
		nestedMap = ""
		if err := assignConfigField(current, key, value); err != nil {
			return nil, fmt.Errorf("invalid server config line %d: %w", lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		servers = append(servers, *current)
	}
	if err := ValidateServers(servers); err != nil {
		return nil, err
	}
	return servers, nil
}

func ValidateServers(servers []ServerConfig) error {
	seen := map[string]bool{}
	for _, server := range servers {
		if server.Name == "" {
			return fmt.Errorf("server name is required")
		}
		if seen[server.Name] {
			return fmt.Errorf("duplicate server name: %s", server.Name)
		}
		seen[server.Name] = true
		switch server.Transport {
		case "stdio":
			if server.Command == "" {
				return fmt.Errorf("stdio server %s command is required", server.Name)
			}
		case "http":
			if server.URL == "" {
				return fmt.Errorf("http server %s url is required", server.Name)
			}
			for name, value := range server.Headers {
				if strings.Contains(value, "${") && !isEnvironmentReference(value) {
					return fmt.Errorf("http server %s header %s must use a complete ${ENV_NAME} reference", server.Name, name)
				}
			}
		default:
			return fmt.Errorf("unknown transport for server %s: %s", server.Name, server.Transport)
		}
	}
	return nil
}

func ResolveHeaders(server ServerConfig, lookup func(string) (string, bool)) (map[string]string, error) {
	resolved := make(map[string]string, len(server.Headers))
	for name, value := range server.Headers {
		if !isEnvironmentReference(value) {
			resolved[name] = value
			continue
		}
		envName := value[2 : len(value)-1]
		expanded, ok := lookup(envName)
		if !ok {
			return nil, fmt.Errorf("external server %s requires environment variable %s", server.Name, envName)
		}
		resolved[name] = expanded
	}
	return resolved, nil
}

func isEnvironmentReference(value string) bool {
	if len(value) < 4 || !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return false
	}
	name := value[2 : len(value)-1]
	if name == "" {
		return false
	}
	for index, char := range name {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func assignConfigField(server *ServerConfig, key string, value string) error {
	value = cleanYAMLValue(value)
	switch key {
	case "name":
		server.Name = value
	case "transport":
		server.Transport = value
	case "command":
		server.Command = value
	case "url":
		server.URL = value
	case "args":
		server.Args = parseInlineList(value)
	case "timeout_ms":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		server.TimeoutMS = n
	default:
		return fmt.Errorf("unknown key %s", key)
	}
	return nil
}

func parseInlineList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		items = append(items, cleanYAMLValue(part))
	}
	return items
}

func cleanYAMLValue(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

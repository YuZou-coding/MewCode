package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const FileName = "mewcode.yaml"

type Config struct {
	Protocol                  string
	Model                     string
	BaseURL                   string
	APIKey                    string
	WorkerEnableVerify        bool
	WorkerBackgroundThreshold string
	WorktreeCopyFiles         []string
	WorktreeLinkDirs          []string
	WorktreeTTL               string
	TeamDefaultBackend        string
	TeamSchedulerEnabled      bool
	TeamDefaultMemberApproval bool
	PermissionMode            string
	MaxIterations             int
}

func LoadProject() (Config, error) {
	cfg, err := loadFile(FileName)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}
	userPath, userErr := UserFile()
	if userErr != nil {
		return Config{}, userErr
	}
	cfg, userErr = loadFile(userPath)
	if userErr == nil {
		return cfg, nil
	}
	if os.IsNotExist(userErr) {
		return Config{}, fmt.Errorf("config file not found: %s or %s; create one manually or run mewcode setup-global --from /path/to/Mewcode", FileName, userPath)
	}
	return Config{}, userErr
}

func UserFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mewcode", FileName), nil
}

func loadFile(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	return Parse(file)
}

func Parse(r io.Reader) (Config, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(r)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("invalid config line %d", lineNumber)
		}

		key = strings.TrimSpace(key)
		value = cleanValue(value)
		if key == "" {
			return Config{}, fmt.Errorf("invalid config line %d", lineNumber)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Protocol:                  values["protocol"],
		Model:                     values["model"],
		BaseURL:                   values["base_url"],
		APIKey:                    values["api_key"],
		WorkerEnableVerify:        parseBool(values["worker_enable_verify"]),
		WorkerBackgroundThreshold: values["worker_background_threshold"],
		WorktreeCopyFiles:         parseCSV(values["worktree_copy_files"]),
		WorktreeLinkDirs:          parseCSV(values["worktree_link_dirs"]),
		WorktreeTTL:               values["worktree_ttl"],
		TeamDefaultBackend:        values["team_default_backend"],
		TeamSchedulerEnabled:      parseBool(values["team_scheduler_enabled"]),
		TeamDefaultMemberApproval: parseBool(values["team_default_member_approval"]),
		PermissionMode:            permissionMode(values["permission_mode"]),
		MaxIterations:             parsePositiveInt(values["max_iterations"], 30),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseCSV(value string) []string {
	var items []string
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (c Config) Validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"protocol", c.Protocol},
		{"model", c.Model},
		{"base_url", c.BaseURL},
		{"api_key", c.APIKey},
	}

	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("missing required config field: %s", field.name)
		}
	}
	if c.PermissionMode != "strict" && c.PermissionMode != "default" && c.PermissionMode != "yolo" {
		return fmt.Errorf("invalid permission_mode: %s", c.PermissionMode)
	}
	return nil
}

func permissionMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "default"
	}
	return value
}

func cleanValue(raw string) string {
	value := strings.TrimSpace(raw)
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	value = strings.Trim(value, `"'`)
	return value
}

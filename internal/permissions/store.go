package permissions

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	UserRulesPath    = ".mewcode/permissions.yaml"
	ProjectRulesPath = ".mewcode/permissions.yaml"
)

func UserRulesFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return UserRulesPath
	}
	return filepath.Join(home, UserRulesPath)
}

func ProjectRulesFile(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ProjectRulesPath)
}

func LoadRulesFile(path string, source Source) ([]Rule, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var rules []Rule
	var current *Rule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || line == "rules:" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if current != nil {
				rules = append(rules, *current)
			}
			current = &Rule{Source: source}
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line == "" {
				continue
			}
		}
		if current == nil {
			return nil, fmt.Errorf("invalid rule line: %s", line)
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid rule line: %s", line)
		}
		assignRuleField(current, strings.TrimSpace(key), trimYAMLValue(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		rules = append(rules, *current)
	}
	for _, rule := range rules {
		if rule.Effect == "" {
			return nil, fmt.Errorf("rule missing effect")
		}
	}
	return rules, nil
}

func AppendUserRule(rule Rule) error {
	path := UserRulesFile()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	exists := true
	if _, err := os.Stat(path); os.IsNotExist(err) {
		exists = false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if !exists {
		if _, err := fmt.Fprintln(file, "rules:"); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(file, formatRule(rule))
	return err
}

func assignRuleField(rule *Rule, key string, value string) {
	switch key {
	case "effect":
		rule.Effect = Effect(value)
	case "tool":
		rule.Tool = value
	case "path_pattern":
		rule.PathPattern = value
	case "command_pattern":
		rule.CommandPattern = value
	case "args_contains":
		rule.ArgsContains = value
	}
}

func trimYAMLValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func formatRule(rule Rule) string {
	var b strings.Builder
	b.WriteString("- effect: " + string(rule.Effect) + "\n")
	if rule.Tool != "" {
		b.WriteString("  tool: " + rule.Tool + "\n")
	}
	if rule.PathPattern != "" {
		b.WriteString("  path_pattern: " + quoteRuleValue(rule.PathPattern) + "\n")
	}
	if rule.CommandPattern != "" {
		b.WriteString("  command_pattern: " + quoteRuleValue(rule.CommandPattern) + "\n")
	}
	if rule.ArgsContains != "" {
		b.WriteString("  args_contains: " + quoteRuleValue(rule.ArgsContains) + "\n")
	}
	return b.String()
}

func quoteRuleValue(value string) string {
	if strings.ContainsAny(value, ":#*?[]{} |\"\\\r\n\t") {
		return strconv.Quote(value)
	}
	return value
}

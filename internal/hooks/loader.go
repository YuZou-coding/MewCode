package hooks

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func UserHooksFile(home string) string {
	if home == "" {
		return UserHooksPath
	}
	return filepath.Join(home, UserHooksPath)
}

func ProjectHooksFile(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ProjectHooksPath)
}

func Load(projectRoot, homeDir string) (Loaded, error) {
	var loaded Loaded
	user, err := LoadFile(UserHooksFile(homeDir), "user")
	if err != nil {
		return loaded, err
	}
	project, err := LoadFile(ProjectHooksFile(projectRoot), "project")
	if err != nil {
		return loaded, err
	}
	loaded.Rules = append(loaded.Rules, user...)
	loaded.Rules = append(loaded.Rules, project...)
	return loaded, nil
}

func LoadFile(path string, source string) ([]Rule, error) {
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
	var section string
	var lastClause *Clause
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || line == "rules:" {
			continue
		}
		if strings.HasPrefix(line, "- ") && indent(raw) == 0 {
			if current != nil {
				if err := validateRule(*current); err != nil {
					return nil, err
				}
				rules = append(rules, *current)
			}
			current = &Rule{Source: source}
			section = ""
			lastClause = nil
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line == "" {
				continue
			}
		}
		if current == nil {
			return nil, fmt.Errorf("invalid hook line: %s", line)
		}
		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, "- ") {
			section = strings.TrimSuffix(line, ":")
			lastClause = nil
			continue
		}
		if strings.HasPrefix(line, "- ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if section != "all" && section != "any" {
				return nil, fmt.Errorf("rule %s has list outside all/any", normalizeRuleName(current.Name))
			}
			clause := Clause{}
			assignClauseField(&clause, line)
			if section == "all" {
				current.Conditions.All = append(current.Conditions.All, clause)
				lastClause = &current.Conditions.All[len(current.Conditions.All)-1]
			} else {
				current.Conditions.Any = append(current.Conditions.Any, clause)
				lastClause = &current.Conditions.Any[len(current.Conditions.Any)-1]
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid hook line: %s", line)
		}
		key = strings.TrimSpace(key)
		value = trim(value)
		switch section {
		case "action":
			assignActionField(&current.Action, key, value)
		case "headers":
			if current.Action.Headers == nil {
				current.Action.Headers = map[string]string{}
			}
			current.Action.Headers[key] = value
		case "all", "any":
			if lastClause == nil {
				return nil, fmt.Errorf("rule %s has condition field without clause", normalizeRuleName(current.Name))
			}
			assignClauseField(lastClause, key+": "+value)
		default:
			assignRuleField(current, key, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		if err := validateRule(*current); err != nil {
			return nil, err
		}
		rules = append(rules, *current)
	}
	return rules, nil
}

func assignRuleField(rule *Rule, key string, value string) {
	switch key {
	case "name":
		rule.Name = value
	case "event":
		rule.Event = EventName(value)
	case "once":
		rule.Once = parseBool(value)
	case "async":
		rule.Async = parseBool(value)
	case "timeout_ms":
		rule.TimeoutMS, _ = strconv.Atoi(value)
	case "block":
		rule.Block = value
	}
}

func assignActionField(action *Action, key string, value string) {
	switch key {
	case "type":
		action.Type = ActionType(value)
	case "command":
		action.Command = value
	case "prompt":
		action.Prompt = value
	case "method":
		action.Method = value
	case "url":
		action.URL = value
	case "body":
		action.Body = value
	}
}

func assignClauseField(clause *Clause, line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	switch strings.TrimSpace(key) {
	case "field":
		clause.Field = trim(value)
	case "op":
		clause.Op = Op(trim(value))
	case "value":
		clause.Value = trim(value)
	}
}

func validateRule(rule Rule) error {
	name := normalizeRuleName(rule.Name)
	if rule.Event == "" {
		return fmt.Errorf("rule %s missing event", name)
	}
	if !validEvent(rule.Event) {
		return fmt.Errorf("rule %s has invalid event %s", name, rule.Event)
	}
	if rule.Action.Type == "" {
		return fmt.Errorf("rule %s missing action.type", name)
	}
	if !validAction(rule.Action.Type) {
		return fmt.Errorf("rule %s has invalid action type %s", name, rule.Action.Type)
	}
	if len(rule.Conditions.All) > 0 && len(rule.Conditions.Any) > 0 {
		return fmt.Errorf("rule %s cannot mix all and any", name)
	}
	if rule.Block != "" && rule.Event != EventToolBeforeExecute {
		return fmt.Errorf("rule %s block is only valid for %s", name, EventToolBeforeExecute)
	}
	if rule.Async && rule.Event == EventToolBeforeExecute {
		return fmt.Errorf("rule %s async is not allowed for %s", name, EventToolBeforeExecute)
	}
	switch rule.Action.Type {
	case ActionShell:
		if rule.Action.Command == "" {
			return fmt.Errorf("rule %s shell action missing command", name)
		}
	case ActionInjectPrompt, ActionSubAgent:
		if rule.Action.Prompt == "" {
			return fmt.Errorf("rule %s %s action missing prompt", name, rule.Action.Type)
		}
	case ActionHTTP:
		if rule.Action.URL == "" {
			return fmt.Errorf("rule %s http action missing url", name)
		}
	}
	for _, clause := range append(rule.Conditions.All, rule.Conditions.Any...) {
		if clause.Field == "" || clause.Op == "" {
			return fmt.Errorf("rule %s has invalid condition", name)
		}
		if !validOp(clause.Op) {
			return fmt.Errorf("rule %s has invalid op %s", name, clause.Op)
		}
	}
	return nil
}

func validEvent(event EventName) bool {
	switch event {
	case EventSystemStart, EventSystemExit, EventSystemError, EventCompactBefore, EventCompactAfter,
		EventSessionStart, EventSessionEnd, EventTurnStart, EventTurnEnd, EventMessageBeforeSend,
		EventMessageAfterRecv, EventToolBeforeExecute, EventToolAfterExecute:
		return true
	default:
		return false
	}
}

func validAction(action ActionType) bool {
	switch action {
	case ActionShell, ActionInjectPrompt, ActionHTTP, ActionSubAgent:
		return true
	default:
		return false
	}
}

func validOp(op Op) bool {
	switch op {
	case OpEq, OpNot, OpRegex, OpGlob:
		return true
	default:
		return false
	}
}

func trim(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func parseBool(value string) bool {
	return strings.EqualFold(value, "true") || value == "yes"
}

func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

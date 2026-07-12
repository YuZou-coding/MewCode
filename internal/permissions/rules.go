package permissions

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func MatchRule(rule Rule, request Request) bool {
	if rule.Tool != "" && rule.Tool != "*" && rule.Tool != request.Tool {
		return false
	}
	if rule.PathPattern != "" {
		path, ok := representativePath(request)
		if !ok || !globMatch(rule.PathPattern, path) {
			return false
		}
	}
	if rule.CommandPattern != "" {
		if !commandMatch(rule.CommandPattern, commandFromArgs(request.Arguments)) {
			return false
		}
	}
	if rule.ArgsContains != "" && !strings.Contains(string(request.Arguments), rule.ArgsContains) {
		return false
	}
	return true
}

func DecideByRules(request Request, session []Rule, project []Rule, user []Rule) Decision {
	groups := [][]Rule{session, project, user}
	for _, group := range groups {
		for index := range group {
			rule := group[index]
			if rule.Effect == EffectDeny && MatchRule(rule, request) {
				return Decision{Effect: rule.Effect, Reason: "matched rule", Rule: &rule}
			}
		}
	}
	for _, group := range groups {
		for index := range group {
			rule := group[index]
			if MatchRule(rule, request) {
				return Decision{Effect: rule.Effect, Reason: "matched rule", Rule: &rule}
			}
		}
	}
	return Ask("no matching rule")
}

func RuleForRequest(effect Effect, source Source, request Request) Rule {
	rule := Rule{Effect: effect, Source: source, Tool: request.Tool}
	if path, ok := representativePath(request); ok {
		rule.PathPattern = path
	}
	if request.Tool == "run_command" {
		rule.CommandPattern = commandFromArgs(request.Arguments)
	}
	if rule.PathPattern == "" && rule.CommandPattern == "" {
		rule.ArgsContains = compactArgs(request.Arguments)
	}
	return rule
}

func globMatch(pattern string, value string) bool {
	ok, err := filepath.Match(pattern, filepath.Base(value))
	if err == nil && ok {
		return true
	}
	ok, err = filepath.Match(pattern, value)
	return err == nil && ok
}

func commandMatch(pattern string, command string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		ok, err := filepath.Match(pattern, command)
		return err == nil && ok
	}
	return strings.Contains(command, pattern)
}

func compactArgs(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
}

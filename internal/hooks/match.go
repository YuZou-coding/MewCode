package hooks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func (r Rule) Matches(ctx Context) bool {
	if r.Event != ctx.Event {
		return false
	}
	if len(r.Conditions.All) == 0 && len(r.Conditions.Any) == 0 {
		return true
	}
	if len(r.Conditions.All) > 0 {
		for _, clause := range r.Conditions.All {
			if !matchClause(clause, ctx) {
				return false
			}
		}
		return true
	}
	for _, clause := range r.Conditions.Any {
		if matchClause(clause, ctx) {
			return true
		}
	}
	return false
}

func matchClause(clause Clause, ctx Context) bool {
	value := fieldValue(clause.Field, ctx)
	switch clause.Op {
	case OpEq:
		return value == clause.Value
	case OpNot:
		return value != clause.Value
	case OpRegex:
		ok, err := regexp.MatchString(clause.Value, value)
		return err == nil && ok
	case OpGlob:
		return globMatch(clause.Value, value)
	default:
		return false
	}
}

func fieldValue(field string, ctx Context) string {
	switch field {
	case "event":
		return string(ctx.Event)
	case "tool.name":
		return ctx.ToolName
	case "path":
		return ctx.Path
	case "command":
		return ctx.Command
	case "message.content":
		return ctx.MessageContent
	case "error":
		return ctx.Error
	case "session.id":
		return ctx.SessionID
	case "tool.result":
		return ctx.ToolResult
	}
	if strings.HasPrefix(field, "tool.args.") {
		key := strings.TrimPrefix(field, "tool.args.")
		if ctx.ToolArgs == nil {
			return ""
		}
		if value, ok := ctx.ToolArgs[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func globMatch(pattern string, value string) bool {
	if ok, err := filepath.Match(pattern, value); err == nil && ok {
		return true
	}
	if ok, err := filepath.Match(pattern, filepath.Base(value)); err == nil && ok {
		return true
	}
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[len(parts)-1])
	}
	return false
}

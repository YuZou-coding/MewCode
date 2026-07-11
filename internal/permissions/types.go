package permissions

import (
	"context"
	"encoding/json"
)

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
	EffectAsk   Effect = "ask"
)

type Source string

const (
	SourceSession Source = "session"
	SourceProject Source = "project"
	SourceUser    Source = "user"
)

type Rule struct {
	Effect         Effect
	Tool           string
	PathPattern    string
	CommandPattern string
	ArgsContains   string
	Source         Source
}

type Request struct {
	Tool      string
	Arguments json.RawMessage
	Root      string
}

type Decision struct {
	Effect Effect
	Code   string
	Reason string
	Rule   *Rule
}

type HITLChoice string

const (
	HITLDeny         HITLChoice = "deny"
	HITLAllowOnce    HITLChoice = "allow_once"
	HITLAllowSession HITLChoice = "allow_session"
	HITLAllowAlways  HITLChoice = "allow_always"
)

type HITLFunc func(ctx context.Context, request Request, decision Decision) HITLChoice

func Allow(reason string) Decision {
	return Decision{Effect: EffectAllow, Reason: reason}
}

func Deny(code, reason string) Decision {
	return Decision{Effect: EffectDeny, Code: code, Reason: reason}
}

func Ask(reason string) Decision {
	return Decision{Effect: EffectAsk, Reason: reason}
}

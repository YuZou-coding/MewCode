package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Schema map[string]any

type Definition struct {
	Name        string
	Description string
	Schema      Schema
}

type Call struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Executor interface {
	Definition() Definition
	Execute(ctx context.Context, input Input) Result
}

type Input struct {
	Arguments json.RawMessage
	Confirm   ConfirmFunc
	Timeout   time.Duration
}

type ConfirmFunc func(ctx context.Context, command string) bool

type Result struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data,omitempty"`
	Error *Error         `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(data map[string]any) Result {
	if data == nil {
		data = map[string]any{}
	}
	return Result{OK: true, Data: data}
}

func Fail(code, message string) Result {
	return Result{OK: false, Error: &Error{Code: code, Message: message}}
}

func DecodeArgs[T any](raw json.RawMessage) (T, error) {
	var args T
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, fmt.Errorf("invalid arguments: %w", err)
	}
	return args, nil
}

func ObjectSchema(required []string, properties map[string]any) Schema {
	return Schema{
		"type":       "object",
		"required":   required,
		"properties": properties,
	}
}

func StringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

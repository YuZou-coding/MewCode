package command

import (
	"context"
	"strings"
)

func Parse(input string) ParseResult {
	raw := strings.TrimSpace(input)
	if !strings.HasPrefix(raw, "/") {
		return ParseResult{Raw: raw}
	}
	body := strings.TrimPrefix(raw, "/")
	name, args, _ := strings.Cut(body, " ")
	return ParseResult{IsCommand: true, Name: normalizeName(name), Args: strings.TrimSpace(args), Raw: raw}
}

func Dispatch(ctx context.Context, registry *Registry, controller Controller, input string) Result {
	parsed := Parse(input)
	if !parsed.IsCommand {
		return Result{SendToAgent: parsed.Raw}
	}
	cmd, ok := registry.Lookup(parsed.Name)
	if !ok {
		return Messagef("unknown command /%s; type /help", parsed.Name).Emit(controller)
	}
	if cmd.Handler == nil {
		return Result{Err: ErrNoHandler(cmd.Name)}.Emit(controller)
	}
	invocation := Invocation{Raw: parsed.Raw, Name: parsed.Name, Args: parsed.Args, Command: cmd}
	result := cmd.Handler(ctx, controller, invocation)
	return result.Emit(controller)
}

type noHandlerError string

func (e noHandlerError) Error() string { return "command has no handler: " + string(e) }

func ErrNoHandler(name string) error { return noHandlerError(name) }

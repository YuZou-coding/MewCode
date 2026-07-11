# internal packages

MewCode keeps application code under `internal/` so the CLI entrypoint can stay small.

- `app`: startup wiring.
- `chat`: in-process conversation state.
- `config`: project-level YAML configuration loading.
- `provider`: model provider abstraction and implementations.
- `sse`: server-sent event parsing.
- `tui`: simple terminal interaction loop.

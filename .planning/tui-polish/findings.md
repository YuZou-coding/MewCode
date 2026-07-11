# Findings

- Repository is on `main` with no initial commit; Git worktree isolation is unavailable.
- Existing `internal/tuiapp/model.go` owns state, rendering, panels, input editing, and layout.
- Existing Agent events include tool call id and result, but the App-to-TUI bridge currently drops those fields.
- Existing command state does not expose cwd, branch, or context-window percentage.
- Context usage now follows the existing compaction character threshold, so it remains consistent with MewCode's current token approximation strategy.
- Git branch is read once when the TUI controller is created, avoiding subprocess work during activity ticks.

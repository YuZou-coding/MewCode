# MewCode Claude-Style TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the log-like fullscreen TUI with a lightweight, Claude Code-inspired transcript, command picker, permission panel, scrolling behavior, and responsive status presentation.

**Architecture:** Keep Bubble Tea Model as the event/state coordinator while moving transcript rendering, theme definitions, panels, and layout into focused files. Extend existing TUI events only with display metadata already available from Agent events; preserve the Agent loop, permission decisions, session history, and fallback TUI.

**Tech Stack:** Go, Bubble Tea, Bubbles viewport, Lip Gloss, existing command and permission packages.

---

### Task 1: Lock the transcript behavior with failing tests

**Files:**
- Modify: `internal/tuiapp/model_test.go`

- [ ] Add tests asserting `❯` user markers, compact assistant markers, a persistent single welcome block, compact successful tools, expanded failed tools, and usage separated from assistant text.
- [ ] Run `go test ./internal/tuiapp -count=1` and confirm failures reference the old `You >` / `MewCode >` rendering.

### Task 2: Extract theme and transcript rendering

**Files:**
- Create: `internal/tuiapp/theme.go`
- Create: `internal/tuiapp/transcript.go`
- Modify: `internal/tuiapp/model.go`

- [ ] Move semantic colors and symbols into a `theme` value with plain-text fallbacks.
- [ ] Move block types, append/update helpers, and rendering into `transcript.go`.
- [ ] Render user, assistant, thinking, tool, usage, error, and system blocks with the approved lightweight hierarchy.
- [ ] Preserve `Transcript()` as an unstyled test/debug view.
- [ ] Run `go test ./internal/tuiapp -count=1` and make Task 1 tests pass.

### Task 3: Add tool metadata and failure presentation

**Files:**
- Modify: `internal/agent/event.go`
- Modify: `internal/app/app.go`
- Modify: `internal/tuiapp/model.go`
- Modify: `internal/tuiapp/transcript.go`
- Modify: `internal/app/app_test.go`
- Modify: `internal/tuiapp/model_test.go`

- [ ] Add a failing bridge test proving call id, arguments/target, result state, and error summary reach the TUI.
- [ ] Extend `StreamEvent` with call id, target, result text, and tool state metadata.
- [ ] Pair concurrent tool starts/results by call id and update the original block in place.
- [ ] Keep successful output collapsed; show up to three lines for failed, denied, or blocked results.
- [ ] Run `go test ./internal/app ./internal/tuiapp -count=1`.

### Task 4: Build keyboard-driven command and permission panels

**Files:**
- Create: `internal/tuiapp/panels.go`
- Modify: `internal/tuiapp/model.go`
- Modify: `internal/tuiapp/model_test.go`

- [ ] Add failing tests for `/wo` filtering, up/down selection, Enter execution, Tab completion, and Esc dismissal.
- [ ] Track command-panel selection independently from text completion candidates.
- [ ] Render at most eight compact rows with one highlighted selection.
- [ ] Add failing tests for multiline permission content, width-safe truncation, and decision transcript records.
- [ ] Append a permission block after `n/y/s/a` without writing panel state to chat history.
- [ ] Run `go test ./internal/tuiapp -count=1`.

### Task 5: Implement viewport follow/pause behavior

**Files:**
- Modify: `internal/tuiapp/model.go`
- Create: `internal/tuiapp/layout.go`
- Modify: `internal/tuiapp/model_test.go`

- [ ] Add failing tests proving PageUp/Up while the command panel is closed pauses following and new output does not move the viewport.
- [ ] Track `followOutput` and `newOutput` state; only call `GotoBottom` while following.
- [ ] Render a `↓ New output` indicator and support End or a dedicated bottom action to resume following.
- [ ] Run `go test ./internal/tuiapp -count=1`.

### Task 6: Rebuild header, input, activity, and status layout

**Files:**
- Modify: `internal/command/types.go`
- Modify: `internal/tui/loop.go`
- Modify: `internal/tuiapp/layout.go`
- Modify: `internal/tuiapp/theme.go`
- Modify: `internal/tuiapp/model.go`
- Modify: `internal/tuiapp/model_test.go`

- [ ] Add failing tests for 120-, 80-, and 40-column layouts with no line wider than the viewport.
- [ ] Extend read-only command state with available cwd, branch, and context usage information; leave unavailable values empty.
- [ ] Render a minimal header, bordered prompt/activity line, and status fields ordered by width priority.
- [ ] Keep the complete welcome block visible at the transcript top.
- [ ] Run `go test ./internal/command ./internal/tui ./internal/tuiapp -count=1`.

### Task 7: Integrate, document, and verify

**Files:**
- Modify: `README.md`
- Modify: `checklist.md`
- Test: `internal/app/app_test.go`
- Test: `internal/e2e`

- [ ] Update README with command-picker, scrolling, permission, and activity key behavior.
- [ ] Run focused tests: `go test -count=1 ./internal/tuiapp ./internal/app ./internal/command ./internal/tui`.
- [ ] Run full verification: `GOCACHE=$PWD/.gocache go test -count=1 ./...`.
- [ ] Check every observable item in `checklist.md` and mark only verified items complete.


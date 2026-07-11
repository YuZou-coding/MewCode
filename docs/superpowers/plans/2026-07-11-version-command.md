# Version Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/version` command that reports the build-injected MewCode version and defaults to `dev`.

**Architecture:** A focused `internal/version` package owns the linker-overridable value and display formatting. The existing command registry exposes that value through a local `/version` handler, so both full-screen and fallback interfaces receive the feature through their current dispatch path.

**Tech Stack:** Go 1.25, standard library testing, Go linker `-X` flags

## Global Constraints

- The command output is exactly `MewCode <version>`.
- The unmodified development build uses `dev`.
- `/version` has no alias.
- Runtime version lookup performs no network, Git, environment, or filesystem access.
- Do not display Git commit, build time, Go version, operating system, or CPU architecture.

---

### Task 1: Version Module

**Files:**
- Create: `internal/version/version_test.go`
- Create: `internal/version/version.go`

**Interfaces:**
- Consumes: Go linker `-X mewcode/internal/version.Value=<version>` assignment
- Produces: exported variable `Value string` and function `String() string`

- [ ] **Step 1: Write the failing version test**

```go
package version

import "testing"

func TestStringUsesDevelopmentVersionByDefault(t *testing.T) {
	if got := String(); got != "MewCode dev" {
		t.Fatalf("String() = %q", got)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/version`

Expected: compilation fails because `String` is undefined.

- [ ] **Step 3: Implement the minimal version module**

```go
package version

const product = "MewCode"

var Value = "dev"

func String() string {
	return product + " " + Value
}
```

- [ ] **Step 4: Run the test and verify GREEN**

Run: `go test ./internal/version`

Expected: package passes.

### Task 2: Command Registration

**Files:**
- Modify: `internal/command/command_test.go`
- Modify: `internal/command/builtin.go`

**Interfaces:**
- Consumes: `version.String() string`
- Produces: registered local command `/version` with no aliases

- [ ] **Step 1: Write failing command tests**

Add a test that builds the registry, asserts `/ver` completes to `/version`, asserts lookup of `/v` fails, dispatches `/version`, and checks the sole message equals `MewCode dev`. Extend the existing help assertion to require `/version`.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/command -run 'TestBuiltins(Version|Dispatch)'`

Expected: failure because `/version` is not registered.

- [ ] **Step 3: Register and handle the command**

Import `mewcode/internal/version`, add this command to `Builtins`, and add the handler:

```go
{Name: "version", Description: "显示 MewCode 版本", Usage: "/version", Type: TypeLocal, Handler: versionHandler},

func versionHandler(ctx context.Context, controller Controller, inv Invocation) Result {
	return Message(version.String())
}
```

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/version ./internal/command`

Expected: both packages pass.

### Task 3: Documentation And Integration

**Files:**
- Modify: `README.md`
- Modify: `checklist.md`

**Interfaces:**
- Consumes: `/version` and linker variable `mewcode/internal/version.Value`
- Produces: user-facing build and command instructions

- [ ] **Step 1: Document development and release behavior**

Add `/version` to the command list. Add a build example using:

```bash
go build -ldflags "-X mewcode/internal/version.Value=v1.2.3" -o ./bin/mewcode ./cmd/mewcode
```

- [ ] **Step 2: Format and run focused tests**

Run: `gofmt -w internal/version/version.go internal/version/version_test.go internal/command/builtin.go internal/command/command_test.go`

Run: `go test ./internal/version ./internal/command`

Expected: both packages pass.

- [ ] **Step 3: Verify injected build behavior**

Build with the documented `-ldflags`, run the binary with `/version` through the fallback input path, and confirm output contains `MewCode v1.2.3` without invoking a model request.

- [ ] **Step 4: Run the complete test suite**

Run: `go test -count=1 ./...`

Expected: all packages pass with zero failures.

- [ ] **Step 5: Update acceptance evidence and commit**

Mark objectively verified entries in `checklist.md`, inspect `git diff --check`, then commit implementation and documentation with message `feat: add version command`.

### Task 4: Pull Request

**Files:**
- No source changes expected

**Interfaces:**
- Consumes: verified commits on `codex/add-version-command`
- Produces: remote branch and GitHub pull request targeting `main`

- [ ] **Step 1: Review the complete diff**

Run: `git diff origin/main...HEAD` and check every change against `spec.md` and `checklist.md`.

- [ ] **Step 2: Push the feature branch**

Run: `git push -u origin codex/add-version-command`

Expected: remote branch is created and tracking is configured.

- [ ] **Step 3: Create the pull request**

Run `gh pr create` with a concise summary and the exact verification commands, targeting `main`.

Expected: GitHub returns a pull request URL.

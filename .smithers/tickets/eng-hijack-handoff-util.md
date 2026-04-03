# Handoff Utility for Hijack

## Metadata
- ID: eng-hijack-handoff-util
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Implement a reusable wrapper around tea.ExecProcess for cleanly suspending and resuming the TUI.

## Acceptance Criteria

- Provides a function to execute an external CLI.
- Returns a tea.Cmd that handles the suspend and resume.

## Source Context

- internal/ui/util/handoff.go

## Implementation Notes

- Create `handoffToProgram(binary string, args []string, cwd string, onReturn func(error) tea.Msg) tea.Cmd`.

---

## Objective

Provide a single, reusable Go function (`HandoffToProgram`) in `internal/ui/util/handoff.go` that any Smithers TUI view can call to cleanly suspend the Bubble Tea program, hand full TTY control to an external process, and resume with a caller-defined message when the process exits. This utility is the foundation for every native-handoff site in the TUI: hijack, agent chat, ticket/prompt editor, and any future external-tool integration.

## Scope

### In scope

1. **`HandoffToProgram` function** — new file `internal/ui/util/handoff.go` exporting the wrapper.
2. **`HandoffReturnMsg` message type** — a generic Bubble Tea message that carries the exit error (or nil) plus caller-supplied metadata, enabling the Update loop of any view to react to a handoff return.
3. **Environment propagation** — the wrapper must forward the parent process's environment to the child, with the ability to merge additional env vars (needed for agent-specific variables like API keys).
4. **Pre-handoff validation** — binary existence check via `exec.LookPath` before calling `tea.ExecProcess`, producing a user-visible `util.InfoMsg` error rather than a blank-screen failure.
5. **Unit tests** for the public API surface.

### Out of scope

- Hijack-specific logic (session tokens, resume flags, run-state refresh) — those belong in `internal/ui/views/livechat.go` and downstream hijack tickets (`feat-hijack-run-command`, `feat-hijack-native-cli-resume`).
- Agent resume-flag matrix — handled by `feat-hijack-multi-engine-support`.
- Conversation-replay fallback — handled by `feat-hijack-conversation-replay-fallback`.
- Modifications to `internal/ui/model/ui.go` (the root model) — callers import the util and return the `tea.Cmd` from their own Update methods.

## Implementation Plan

### Slice 1 — Core `HandoffToProgram` function

**File**: `internal/ui/util/handoff.go`

Create the exported function with this signature:

```go
package util

import (
    "fmt"
    "os"
    "os/exec"

    tea "charm.land/bubbletea/v2"
)

// HandoffReturnMsg is sent to the Bubble Tea Update loop when the
// external process exits. Callers inspect Err and the opaque Tag
// to decide what to do next (refresh state, show summary, etc.).
type HandoffReturnMsg struct {
    // Err is nil on clean exit, non-nil on failure or signal.
    Err error
    // Tag is opaque caller-supplied data echoed back on return
    // (e.g., a run ID, agent name, or file path).
    Tag any
}

// HandoffToProgram suspends the Bubble Tea TUI and launches an external
// process with full TTY control. When the process exits the returned
// HandoffReturnMsg is dispatched to Update.
//
// Parameters:
//   - binary: absolute path or $PATH-resolvable name of the program.
//   - args:   arguments passed after the binary name.
//   - cwd:    working directory for the child; empty string inherits parent.
//   - env:    additional KEY=VALUE pairs merged onto os.Environ(); may be nil.
//   - tag:    opaque value echoed in HandoffReturnMsg.Tag for caller routing.
func HandoffToProgram(binary string, args []string, cwd string, env []string, tag any) tea.Cmd {
    // Validate that the binary exists before attempting exec.
    resolvedPath, err := exec.LookPath(binary)
    if err != nil {
        return ReportError(fmt.Errorf("handoff: binary %q not found in PATH: %w", binary, err))
    }

    cmd := exec.Command(resolvedPath, args...)

    if cwd != "" {
        cmd.Dir = cwd
    }

    // Merge parent env with caller-supplied overrides.
    if len(env) > 0 {
        cmd.Env = append(os.Environ(), env...)
    }

    return tea.ExecProcess(cmd, func(processErr error) tea.Msg {
        return HandoffReturnMsg{Err: processErr, Tag: tag}
    })
}
```

**Rationale for design choices**:

- **`Tag any` instead of `onReturn` callback**: The ticket's implementation note suggests an `onReturn func(error) tea.Msg` callback. This works but couples the util to each caller's message type, making testing harder. By returning a uniform `HandoffReturnMsg` with an opaque `Tag`, every call site can switch on `msg.Tag.(type)` in its own Update without the util needing to know about hijack sessions, agent names, or file paths. If the team prefers the callback style (matching `ExecShell`'s pattern), the alternative signature is documented in Slice 4.
- **`exec.LookPath` pre-check**: `tea.ExecProcess` with an invalid binary produces an opaque error after the terminal has already been released, leaving the user staring at a blank screen until the callback fires. Validating first yields a clean in-TUI error message.
- **Environment merge**: Hijack and agent-chat handoffs may need to inject `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or similar. Defaulting to `os.Environ()` plus overrides covers this without requiring every caller to build the list.

**Relationship to existing code**:

| Existing code | Location | Relationship |
|---|---|---|
| `ExecShell` | `internal/ui/util/util.go:87` | Parses a shell string and calls `tea.ExecProcess`. `HandoffToProgram` differs: it takes a resolved binary + args array (no shell parsing) and adds LookPath validation + env merge. Both live in `internal/ui/util/`. |
| `openEditor` | `internal/ui/model/ui.go:2630` | Creates a temp file, launches `$EDITOR` via `editor.Command`, and calls `tea.ExecProcess`. This is editor-specific. Future refactors may optionally redirect the temp-file flow through `HandoffToProgram`, but that is not required by this ticket. |
| `tea.Suspend` | `internal/ui/model/ui.go:1659` | The `Ctrl+Z` handler. This is a raw suspend (SIGTSTP to the process group); `HandoffToProgram` is a controlled handoff (spawn child, wait, resume). Different mechanisms, complementary. |

### Slice 2 — Callback-style alternative (`HandoffWithCallback`)

Some downstream tickets (e.g., agent-chat launch where the return handler needs to reload an agent list) may prefer a callback that returns their own message type directly. Provide a thin companion:

```go
// HandoffWithCallback is like HandoffToProgram but delegates the return
// message to a caller-supplied callback, matching the pattern used by
// ExecShell.
func HandoffWithCallback(binary string, args []string, cwd string, env []string, onReturn tea.ExecCallback) tea.Cmd {
    resolvedPath, err := exec.LookPath(binary)
    if err != nil {
        return ReportError(fmt.Errorf("handoff: binary %q not found in PATH: %w", binary, err))
    }

    cmd := exec.Command(resolvedPath, args...)
    if cwd != "" {
        cmd.Dir = cwd
    }
    if len(env) > 0 {
        cmd.Env = append(os.Environ(), env...)
    }

    return tea.ExecProcess(cmd, onReturn)
}
```

This keeps the `Tag`-based approach as the default while giving callers an escape hatch. Both functions share the LookPath + env logic; extract a private `buildCmd` helper to avoid duplication:

```go
func buildCmd(binary string, args []string, cwd string, env []string) (*exec.Cmd, error) {
    resolvedPath, err := exec.LookPath(binary)
    if err != nil {
        return nil, fmt.Errorf("handoff: binary %q not found in PATH: %w", binary, err)
    }
    cmd := exec.Command(resolvedPath, args...)
    if cwd != "" {
        cmd.Dir = cwd
    }
    if len(env) > 0 {
        cmd.Env = append(os.Environ(), env...)
    }
    return cmd, nil
}
```

### Slice 3 — Unit tests

**File**: `internal/ui/util/handoff_test.go`

| Test | What it verifies |
|---|---|
| `TestBuildCmd_ValidBinary` | `buildCmd("echo", ...)` resolves to a valid `exec.Cmd` with correct args, cwd, and merged env. |
| `TestBuildCmd_InvalidBinary` | `buildCmd("nonexistent-xyz", ...)` returns a descriptive error containing the binary name. |
| `TestBuildCmd_EmptyCwd` | When `cwd=""`, `cmd.Dir` is empty (inherits parent). |
| `TestBuildCmd_EnvMerge` | Parent env is preserved; caller vars are appended; duplicate keys use last-write-wins semantics (Go stdlib behavior). |
| `TestHandoffToProgram_ReturnsCmd` | Calling `HandoffToProgram("echo", []string{"hello"}, "", nil, "test-tag")` returns a non-nil `tea.Cmd` (not an error-report cmd). |
| `TestHandoffToProgram_InvalidBinary` | Returns an error-report cmd (verified by executing the cmd and checking the returned `InfoMsg` has `InfoTypeError`). |
| `TestHandoffReturnMsg_TagRoundTrip` | Execute a real `tea.ExecProcess` with `echo` to verify `HandoffReturnMsg` arrives with `Err == nil` and the correct `Tag`. This requires a Bubble Tea test program (use `tea.NewProgram` with a trivial model that captures the message). |

**Note**: `tea.ExecProcess` tests that exercise actual terminal hand-off require a real TTY. For CI, the `echo`-based tests are sufficient since `echo` exits immediately without TTY interaction. The terminal-suspend/resume path is validated in E2E and VHS tests (see Validation section).

### Slice 4 — GoDoc and file header

Add a package-level doc comment at the top of `handoff.go`:

```go
// handoff.go provides HandoffToProgram and HandoffWithCallback, reusable
// wrappers around tea.ExecProcess for cleanly suspending the Smithers TUI,
// handing full terminal control to an external process, and resuming on exit.
//
// Handoff sites in Smithers TUI:
//   - Hijack a run: launch agent CLI (claude-code --resume, codex, etc.)
//   - Agent browser: launch agent native TUI
//   - Ticket/prompt editor: launch $EDITOR
//
// See also: ExecShell in util.go for shell-string based execution.
```

## Validation

### Unit tests

```bash
go test ./internal/ui/util/ -run TestHandoff -v
go test ./internal/ui/util/ -run TestBuildCmd -v
```

All tests in Slice 3 must pass. Verify with:

```bash
go test ./internal/ui/util/... -count=1
```

### Build verification

```bash
go build ./...
go vet ./internal/ui/util/...
```

Ensure no new lint warnings from the project's configured linter:

```bash
golangci-lint run ./internal/ui/util/...
```

### Integration smoke test (manual)

Write a throwaway `main.go` (or use `go run`) that creates a minimal Bubble Tea program, calls `HandoffToProgram("bash", []string{"-c", "echo 'hello from child'; sleep 1"}, "", nil, "smoke")`, and verifies:

1. The TUI suspends (alternate screen clears).
2. "hello from child" prints to stdout.
3. After 1 second the TUI resumes.
4. The Update loop receives `HandoffReturnMsg{Err: nil, Tag: "smoke"}`.

### Terminal E2E test (modeled on upstream @microsoft/tui-test harness)

**File**: `tests/handoff.e2e.test.ts`

Following the pattern in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`:

```typescript
import { test, expect } from '@playwright/test';
import { TuiTestHarness } from './tui-helpers';

test.describe('HandoffToProgram', () => {
  let tui: TuiTestHarness;

  test.beforeEach(async () => {
    tui = new TuiTestHarness({ binary: './smithers-tui' });
    await tui.launch();
  });

  test.afterEach(async () => {
    await tui.close();
  });

  test('suspends TUI and resumes after external process exits', async () => {
    // Navigate to a view that triggers handoff (e.g., agent browser)
    // Or use a test-only command that invokes HandoffToProgram directly
    await tui.sendKeys('ctrl+p');
    await tui.type('/test-handoff echo hello');
    await tui.sendKeys('enter');

    // TUI should suspend — alternate screen clears
    await tui.waitForOutput('hello');

    // Process exits, TUI resumes
    await tui.waitForScreen((screen) =>
      screen.includes('SMITHERS') // Header re-appears
    );
  });

  test('shows error when binary not found', async () => {
    await tui.sendKeys('ctrl+p');
    await tui.type('/test-handoff nonexistent-binary-xyz');
    await tui.sendKeys('enter');

    // Should show error in status bar, NOT suspend
    await tui.waitForScreen((screen) =>
      screen.includes('not found in PATH')
    );
  });
});
```

**Note**: The `/test-handoff` command is a test-only command palette entry that calls `HandoffToProgram` directly. It should only be compiled into test builds (behind a build tag or registered conditionally). Alternatively, the E2E test can exercise the handoff through a real handoff site (e.g., `Ctrl+O` editor launch with `EDITOR=echo` set in the environment).

### VHS-style happy-path recording test

**File**: `tests/vhs/handoff-happy-path.tape`

```
# handoff-happy-path.tape
# Validates that HandoffToProgram cleanly suspends/resumes the TUI.

Output tests/vhs/handoff-happy-path.gif
Set Shell "bash"
Set FontSize 14
Set Width 120
Set Height 30
Set Env "EDITOR" "cat"

# Launch the TUI
Type "EDITOR='echo test-handoff-content' ./smithers-tui"
Enter
Sleep 2s

# Trigger editor handoff (Ctrl+O) — this exercises HandoffToProgram
# under the hood via the existing openEditor path
Ctrl+O
Sleep 1s

# TUI should suspend, editor runs, then TUI resumes
# After resume the TUI header should be visible again
Sleep 2s

# Verify the TUI is back and responsive
Type "hello after resume"
Sleep 1s

# Screenshot for visual verification
Screenshot tests/vhs/handoff-resume-screenshot.png
```

Run with:

```bash
vhs tests/vhs/handoff-happy-path.tape
```

The recording serves as both a regression test and visual documentation of the suspend/resume behavior.

### Downstream consumer verification

Once `HandoffToProgram` is merged, verify it is callable from the planned handoff sites by writing a trivial call in each (these will be fleshed out by downstream tickets):

```go
// In internal/ui/views/livechat.go (stub, fleshed out by feat-hijack-run-command)
cmd := util.HandoffToProgram("claude-code", []string{"--resume", sessionToken}, cwd, nil, hijackTag{RunID: runID})

// In internal/ui/views/agents.go (stub, fleshed out by feat-agents-native-tui-launch)
cmd := util.HandoffToProgram(agent.Binary, nil, projectDir, nil, agentChatTag{Agent: agent.Name})
```

These stubs should compile cleanly even if the views are not yet wired up.

## Risks

### 1. `exec.LookPath` and `$PATH` divergence

**Risk**: `exec.LookPath` checks the current process's `$PATH`. If the user's shell has a different `$PATH` (e.g., due to shell init scripts not sourced by the Go binary), an agent binary that exists in the user's interactive shell may fail the LookPath check.

**Mitigation**: Document that `smithers-tui` must be launched from the user's interactive shell (not from a daemon or cron). If this becomes a real issue, accept an absolute-path override in the agent configuration.

### 2. Terminal state corruption on child crash

**Risk**: If the child process crashes (SIGSEGV, SIGKILL) without restoring terminal state, Bubble Tea's recovery may leave the terminal in a broken state (no echo, raw mode artifacts).

**Mitigation**: Bubble Tea v2's `tea.ExecProcess` already wraps the child in a cleanup handler that restores terminal state in the callback path. However, if the *parent* process is killed during the handoff (e.g., `kill -9` on smithers-tui), the terminal will be left in the child's state. This is an inherent limitation of the approach. Document `reset` as a recovery command.

### 3. Crush upstream divergence in `tea.ExecProcess` API

**Risk**: Bubble Tea v2 (`charm.land/bubbletea/v2 v2.0.2`) is the current dep. If upstream changes the `tea.ExecProcess` signature or behavior, `HandoffToProgram` will break.

**Mitigation**: The wrapper isolates callers from the raw Bubble Tea API. If the upstream API changes, only `handoff.go` needs updating. Pin the Bubble Tea version in `go.mod`.

### 4. Mismatch: Crush's `openEditor` won't use `HandoffToProgram` initially

**Risk**: The existing `openEditor` in `internal/ui/model/ui.go:2630` uses `editor.Command` (from `charmbracelet/x/editor`) which returns a pre-configured `*exec.Cmd` with editor-specific logic (cursor positioning, fallback chain). It calls `tea.ExecProcess` directly. Refactoring it to use `HandoffToProgram` would require either (a) accepting a pre-built `*exec.Cmd` or (b) reimplementing the editor resolution logic.

**Decision**: Do NOT refactor `openEditor` in this ticket. The two paths (`openEditor` for `$EDITOR` and `HandoffToProgram` for arbitrary binaries) coexist cleanly. Future cleanup can unify them if desired, but forcing it now adds risk for no user-visible benefit.

### 5. No `smithers hijack` API endpoint yet

**Risk**: The `HandoffToProgram` utility itself has no dependency on Smithers server APIs, but downstream consumers (the actual hijack flow) depend on a `smithers hijack <run_id>` command or HTTP endpoint that returns session tokens and agent metadata. This endpoint may not exist yet in the Smithers CLI.

**Impact on this ticket**: None — this ticket delivers the generic handoff utility. The API dependency is owned by `feat-hijack-run-command` and `feat-hijack-native-cli-resume`.

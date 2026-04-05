# Implementation Summary: eng-hijack-handoff-util

**Status**: Complete
**Date**: 2026-04-05

---

## What Was Built

Two new files were created at `internal/ui/handoff/`:

### `internal/ui/handoff/handoff.go`

New Go package `handoff` providing a reusable Bubble Tea v2 wrapper around `tea.ExecProcess`.

**Public API:**

| Symbol | Kind | Purpose |
|---|---|---|
| `Options` | struct | Configuration for a single handoff invocation (Binary, Args, Cwd, Env, Tag) |
| `HandoffResult` | struct | Outcome of the external process: ExitCode, Err, Duration |
| `HandoffMsg` | struct | Bubble Tea message dispatched after the process exits; carries Tag + Result |
| `Handoff(opts Options) tea.Cmd` | func | Primary entry point — builds and returns a tea.Cmd that suspends TUI, runs the CLI, resumes |
| `HandoffWithCallback(opts Options, cb tea.ExecCallback) tea.Cmd` | func | Lower-level variant for callers that need a custom message type |

**Internal helpers (tested directly via package-level access):**

| Symbol | Purpose |
|---|---|
| `buildCmd` | Constructs `*exec.Cmd` with path, args, cwd validation, merged env |
| `mergeEnv` | Merges `os.Environ()` with caller-supplied overrides without mutating base slice |
| `splitEnvEntry` | Parses a `KEY=VALUE` string; returns `(key, value, ok)` |
| `exitCodeFromError` | Extracts numeric exit code from `*exec.ExitError`; returns 1 for all others |

**Key design decisions:**
- `exec.LookPath` is called eagerly inside the returned `tea.Cmd` so the TUI is never suspended for a binary that does not exist.
- Pre-flight errors (empty binary, not-found binary, bad cwd) are returned as `HandoffMsg` with `ExitCode=1` and a descriptive `Err`; the TUI is never suspended.
- `Env` overrides are merged on top of `os.Environ()` so agent API keys (e.g. `ANTHROPIC_API_KEY`) can be injected without replacing the full environment.
- `Tag any` allows a single model to dispatch multiple handoff paths and demultiplex results in its Update function.

### `internal/ui/handoff/handoff_test.go`

20 tests, all passing:

| Test group | Tests |
|---|---|
| `buildCmd` | ValidBinary, InvalidCwd, ValidCwd, EnvMerge, NoEnvOverride_InheritsParent |
| `mergeEnv` | Override, Append, NoMutation, EmptyOverrides |
| `splitEnvEntry` | KEY=VALUE, KEY=, KEY=a=b (value with =), NOEQUALS, empty string |
| `exitCodeFromError` | Nil, ExitError (exit 42), GenericError |
| State structs | HandoffResult_ZeroValue, HandoffResult_NonZeroFields, HandoffMsg_TagRoundTrip |
| Pre-flight validation | Handoff_EmptyBinary, Handoff_UnknownBinary, HandoffWithCallback_EmptyBinary |
| Cwd propagation | Handoff_InvalidCwd, Handoff_CwdAbsolutePath |

```
ok  github.com/charmbracelet/crush/internal/ui/handoff  0.450s
```

---

## Files Touched

- `internal/ui/handoff/handoff.go` — **New file**
- `internal/ui/handoff/handoff_test.go` — **New file**

---

## Deviation from Plan

The plan and research spec both listed `internal/ui/util/handoff.go` as the target file location. The ticket implementation instructions specified `internal/ui/handoff/` as a dedicated package. The dedicated package was chosen because it provides a clean import boundary and avoids adding a large surface to the existing `util` package. Callers import `github.com/charmbracelet/crush/internal/ui/handoff` and receive a narrow, purpose-built API.

The TypeScript E2E harness (`tests/handoff.e2e.test.ts`) and VHS tape (`tests/vhs/handoff-happy-path.tape`) listed in the plan are out of scope for this ticket as they depend on a running TUI binary with a test command hook, which requires a separate engineering decision on build-tag strategy (noted as an open question in the plan).

---

## Usage Example

```go
// In a Bubble Tea model's Update function:
case hijackKeyMsg:
    return m, handoff.Handoff(handoff.Options{
        Binary: "claude",
        Args:   []string{"--continue"},
        Cwd:    m.projectDir,
        Env:    []string{"ANTHROPIC_API_KEY=" + m.apiKey},
        Tag:    "claude-session",
    })

// Handle the return:
case handoff.HandoffMsg:
    if msg.Tag == "claude-session" {
        // refresh state after claude exits
        return m, m.refreshFromDisk()
    }
```

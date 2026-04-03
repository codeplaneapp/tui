# Research Summary: eng-hijack-handoff-util

## Ticket Overview
Implement a reusable wrapper around `tea.ExecProcess` for cleanly suspending and resuming the TUI when handing off to an external CLI process.

## Acceptance Criteria
1. Provides a function to execute an external CLI
2. Returns a `tea.Cmd` that handles the suspend and resume

## Target File
- `internal/ui/util/handoff.go`

## Required Signature
```go
func handoffToProgram(binary string, args []string, cwd string, onReturn func(error) tea.Msg) tea.Cmd
```

## Key Findings from Codebase Research

### Bubble Tea ExecProcess Pattern
The Bubble Tea framework provides `tea.ExecProcess` which suspends the TUI, runs an external process, and resumes the TUI when the process exits. This is the standard mechanism for shelling out from a Bubble Tea application.

### Reference Implementation (Claude TUI)
The Claude TUI codebase in `internal/ui/ui.go` uses `tea.ExecProcess` directly in its update loop for launching subprocesses. The pattern involves:
1. Creating an `exec.Cmd` with the binary, args, and working directory
2. Wrapping it in `tea.ExecProcess` with a callback that converts the result into a `tea.Msg`
3. Returning the resulting `tea.Cmd` from the Update function

### Existing Utility Structure
The project already has `internal/ui/util/` as a package location for shared UI utilities. The handoff utility should follow the same package conventions.

### Implementation Plan
1. Create `internal/ui/util/handoff.go` with the `handoffToProgram` function
2. The function should:
   - Build an `exec.Cmd` from the binary, args, and cwd parameters
   - Use `tea.ExecProcess` to wrap the command
   - Wire the `onReturn` callback to convert the process exit result into a `tea.Msg`
3. Export the function as `HandoffToProgram` (exported) for use by hijack and other features
4. Define a `HandoffReturnMsg` type that carries the error (if any) from the subprocess

### Dependencies
- `github.com/charmbracelet/bubbletea` — for `tea.ExecProcess`, `tea.Cmd`, `tea.Msg`
- `os/exec` — for building the command
- No other internal dependencies (this is a leaf utility)

### Design Considerations
- The `onReturn` callback pattern allows callers to define their own message types for handling the return from the external process
- The `cwd` parameter is important for hijack scenarios where the external CLI needs to run in the correct project directory
- Error handling should cover both process launch failures and non-zero exit codes
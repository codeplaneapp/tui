# Research: eng-hijack-handoff-util

## Existing Crush Surface
The existing Crush UI is implemented in Go using the `charm.land/bubbletea/v2` framework. We inspected two key files dealing with subprocess execution:
- `internal/ui/util/util.go`: Contains `ExecShell`, which parses a shell string using `shell.Fields` and wraps `exec.CommandContext` with `tea.ExecProcess(cmd, callback)`. This is suitable for shell commands but lacks binary resolution (`exec.LookPath`), environment overriding, and a unified return message type.
- `internal/ui/model/ui.go` (around line 2630-2720): Contains `openEditor`, which writes a temporary file, uses `charmbracelet/x/editor` to resolve `$EDITOR`, and directly calls `tea.ExecProcess`. It is tightly coupled to the editor lifecycle and temporary file cleanup.

Currently, there is no generic utility to safely suspend the TUI, launch an external binary (e.g., `claude-code` or `codex`) with a specific working directory and environment variables, and route the return result back to a view's Update loop using an opaque tag.

## Upstream Smithers Reference
The upstream Smithers TUI is a TypeScript-based terminal application. We inspected the reference files:
- `../smithers/docs/guides/smithers-tui-v2-agent-handoff.md`: Outlines the architecture of Smithers TUI v2, which separates a MockBroker from a client shell.
- `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: These define the E2E testing strategy for terminal apps. They spawn the TUI using `Bun.spawn` and wrap it in a `TUITestInstance` backend that reads `stdout`/`stderr` streams, strips ANSI codes, normalizes box-drawing characters, and provides `waitForText` / `sendKeys` primitives. 

This model of automated terminal interaction is the standard we must emulate for the Crush E2E test of the handoff utility.

## Gaps
1. **Data Model & Transport**: Crush needs a standardized Bubble Tea message (`HandoffReturnMsg` with a `Tag any`) to carry the external process's exit state back to the caller's Update loop, as `ExecShell` currently relies strictly on callbacks. 
2. **Environment & Path Safety**: Existing Crush subprocess execution (`ExecShell`) does not perform `exec.LookPath` validation prior to suspending the TUI. Launching a missing binary currently clears the screen before failing. Furthermore, external agent CLIs (e.g., Anthropic agents) require injected API keys, so the new utility must support merging custom `Env` variables with `os.Environ()`.
3. **Testing Infrastructure**: The Smithers reference uses a robust TypeScript test harness (`tui-helpers.ts`) for terminal interactions. Crush lacks an equivalent E2E test for subprocess suspension/resumption. The new implementation must bridge this gap by adding a similar E2E test harness (`tests/handoff.e2e.test.ts`) and a visual verification mechanism via `vhs` (`tests/vhs/handoff-happy-path.tape`).

## Recommended Direction
Following the engineering spec, we should:
1. Implement `HandoffToProgram` in `internal/ui/util/handoff.go`. This function must use `exec.LookPath` to validate the binary before modifying terminal state, build the `exec.Cmd` with merged environment variables and `cwd`, and execute it via `tea.ExecProcess`.
2. Introduce `HandoffReturnMsg` (containing `Err error` and `Tag any`) to allow views to react to the external process's exit.
3. Provide a fallback `HandoffWithCallback` for callers that prefer the `ExecShell` callback pattern.
4. Leave `openEditor` in `internal/ui/model/ui.go` as-is to avoid unnecessary scope creep.
5. Create comprehensive unit tests for the command building logic (`TestBuildCmd_ValidBinary`, `TestBuildCmd_EnvMerge`, etc.).
6. Implement a TypeScript E2E test modeled after the Smithers `tui-helpers.ts` harness to validate the TUI suspend/resume cycle.
7. Create a VHS tape recording (`tests/vhs/handoff-happy-path.tape`) to visually assert terminal state restoration.

## Files To Touch
- `internal/ui/util/handoff.go`
- `internal/ui/util/handoff_test.go`
- `tests/handoff.e2e.test.ts`
- `tests/vhs/handoff-happy-path.tape`
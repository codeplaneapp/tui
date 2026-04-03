# chat-helpbar-shortcuts Research Pass

## Existing Crush Surface

- **`internal/ui/model/keys.go`**: Defines the application's keyboard bindings. Currently, `ctrl+r` is assigned to `Editor.AttachmentDeleteMode` (`key.WithKeys("ctrl+r")` at lines 137-138) and `ctrl+r+r` deletes all attachments (line 146). There is no global assignment for `ctrl+a` or `ctrl+r`.
- **`internal/ui/model/ui.go`**: Manages the application view loop. `ShortHelp()` is defined at line 2203 and `FullHelp()` at line 2282, but neither returns bindings for the new runs dashboard or approval queue shortcuts. Global keystrokes are processed via the `handleGlobalKeys` closure (line 1648), which handles `Help`, `Commands`, etc., but will ignore `ctrl+r` and `ctrl+a`.
- **`internal/ui/dialog/commands.go`**: The `defaultCommands()` function (line 420) initializes command palette items (e.g., `new_session`, `switch_model`) but does not contain entries corresponding to the new runs dashboard or approval queues.

## Upstream Smithers Reference

- **`../smithers/tests/tui.e2e.test.ts` & `../smithers/tests/tui-helpers.ts`**: Upstream E2E testing uses a PTY-based testing harness. The code launches the TUI via `launchTUI()`, sends key events (`sendKeys("\x0f")` for `Ctrl+O`), and scrapes visual output via `waitForText("Inspector")`.
- **`docs/smithers-tui/02-DESIGN.md`**: Section 3.1 & 5 explicitly map the global UI behavior. `Ctrl+R` is assigned globally to open the Run Dashboard. `Ctrl+A` is assigned globally to open the Approval Queue. The layout demands both shortcuts appear in the bottom help bar.
- *(Note: Inspection of `../smithers/gui/src` and `../smithers/gui-ref` yielded no substantive React/TUI components relevant here, verifying that `02-DESIGN.md` and the E2E test harness are the definitive sources of truth.)*

## Gaps

1. **Shortcut conflict**: Crush overrides `Ctrl+R` with an editor-scoped action (`AttachmentDeleteMode`), which conflicts with Smithers' intent to use it as a global chord for the Run Dashboard.
2. **Missing UI visual cues**: Neither `ShortHelp` nor `FullHelp` structures return the required `ctrl+r runs` and `ctrl+a approvals` UI elements.
3. **No global router fallback**: `handleGlobalKeys` drops `ctrl+r` and `ctrl+a`. Since the views themselves (Run Dashboard, Approval Queue) are blocked on future tickets, Crush lacks an intermediate fallback to safely wire the keybindings end-to-end.
4. **Missing E2E terminal testing infrastructure**: Crush has no terminal E2E testing mechanism comparable to the Smithers `@microsoft/tui-test`-based framework.

## Recommended Direction

1. **Resolve conflict**: Reassign `Editor.AttachmentDeleteMode` in `internal/ui/model/keys.go` to `ctrl+shift+r`.
2. **Add global keys**: Introduce `RunDashboard` (`ctrl+r`) and `Approvals` (`ctrl+a`) keybindings to the base `KeyMap` struct in `internal/ui/model/keys.go`.
3. **Wire up UI hints**: Append these new bindings to the returned slices inside `ShortHelp()` and `FullHelp()` in `internal/ui/model/ui.go` so they render properly in the help bar.
4. **Implement view routing abstractions**: Define `NavigateToViewMsg{View: string}`. Dispatch this message inside `handleGlobalKeys` for both `ctrl+r` and `ctrl+a`. Then add a top-level message handler in `ui.go` that emits an informational placeholder string (`util.ReportInfo("runs view coming soon")`) to act as a routing stub until `PLATFORM_VIEW_STACK_ROUTER` lands.
5. **Palette alignment**: Add `"Run Dashboard"` (`ctrl+r`) and `"Approval Queue"` (`ctrl+a`) entries directly into `defaultCommands()` in `internal/ui/dialog/commands.go`.
6. **Port E2E testing patterns**: Replicate the Smithers PTY testing harness by implementing `tests/e2e/helpbar_shortcuts_test.go` and using Go's `pty` library to execute E2E flow testing. Complement this with a VHS recording test (`tests/vhs/helpbar-shortcuts.tape`) to catch rendering regressions.

## Files To Touch

- `internal/ui/model/keys.go`
- `internal/ui/model/ui.go`
- `internal/ui/dialog/commands.go`
- `internal/ui/model/keys_test.go`
- `internal/ui/model/ui_test.go`
- `tests/e2e/helpbar_shortcuts_test.go` (new)
- `tests/vhs/helpbar-shortcuts.tape` (new)
- `Taskfile.yaml` (new test task)
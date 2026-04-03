## Goal
Extend Crush's bottom help bar and global keymap with Smithers-specific keyboard shortcuts so that users can navigate to the Run Dashboard (`Ctrl+R`) and Approval Queue (`Ctrl+A`) with a single chord from any context. This requires safely remapping the existing `Ctrl+R` shortcut used for attachment deletion to prevent conflicts.

## Steps
1. **Resolve `Ctrl+R` Shortcut Conflict**:
   - Update `Editor.AttachmentDeleteMode` in the key bindings to use `ctrl+shift+r` instead of `ctrl+r`.
   - Update `Editor.DeleteAllAttachments` help text to reflect the new chord (`ctrl+shift+r+r`).
   - Ensure the `FullHelp` display is updated to reflect this change.

2. **Add New Global Key Bindings**:
   - Extend the top-level `KeyMap` struct with `RunDashboard` and `Approvals` key bindings.
   - Initialize these in `DefaultKeyMap()` with `ctrl+r` and `ctrl+a` respectively, including appropriate help text ("runs" and "approvals").

3. **Wire Key Press Handling in the View Loop**:
   - Define a new message type `NavigateToViewMsg{View: string}` to act as a placeholder for the future view router.
   - Update `handleGlobalKeys` within the main `UI.Update()` function to catch `RunDashboard` and `Approvals` bindings, emitting `NavigateToViewMsg` with the respective view name.
   - Add a top-level handler in `UI.Update()` to intercept `NavigateToViewMsg` and render an informational status message (e.g., "runs view coming soon") as a fallback until the actual router is implemented.

4. **Update Help Bar Displays**:
   - Modify the `ShortHelp()` and `FullHelp()` functions on the UI model so that `RunDashboard` and `Approvals` shortcuts are rendered in the bottom help bar and the expanded help view.

5. **Align Command Palette**:
   - Update the `defaultCommands()` function to include "Run Dashboard" (`ctrl+r`) and "Approval Queue" (`ctrl+a`) entries.
   - Ensure selecting these commands triggers the same `NavigateToViewMsg`.

6. **Implement Test Infrastructure and Coverage**:
   - Write unit tests to verify the new key mappings and help text generation.
   - Implement a terminal E2E testing harness modeled on the upstream `@microsoft/tui-test` pattern to validate end-to-end rendering and interactions.
   - Create a VHS happy-path recording script to visually test and document the new help bar and navigation feedback.

## File Plan
- `internal/ui/model/keys.go`: Add new `KeyMap` fields and reassign `Ctrl+R` attachment delete mode.
- `internal/ui/model/ui.go`: Handle key events, update `ShortHelp()` and `FullHelp()`, and define `NavigateToViewMsg`.
- `internal/ui/dialog/commands.go`: Add the new shortcuts to the command palette.
- `internal/ui/model/keys_test.go`: Add/update tests for key bindings.
- `internal/ui/model/ui_test.go`: Add/update tests for UI message handling.
- `tests/e2e/helpbar_shortcuts_test.go`: New file for PTY-based terminal E2E coverage.
- `tests/vhs/helpbar-shortcuts.tape`: New file for VHS happy-path recording.
- `Taskfile.yaml`: Add a command to execute the VHS test (`test:vhs`).

## Validation
**Automated Checks:**
- Build: `go build ./...` (must have zero compilation errors).
- Unit tests: `go test ./internal/ui/model/... -v`.
- Regression tests: `go test -race -failfast ./...`.
- Terminal E2E tests: `go test ./tests/e2e/... -run TestHelpbarShortcuts -v -timeout 30s`.
- VHS recording test: `vhs tests/vhs/helpbar-shortcuts.tape` (must exit 0 and produce a visual `.gif` artifact).
- Linting: `golangci-lint run ./...`.

**Terminal E2E Details (Modeled on `@microsoft/tui-test`):**
- **Help bar renders**: Launch TUI using a PTY wrapper -> `waitForText` for `ctrl+r` and `runs` to appear on screen.
- **Ctrl+R navigation**: Launch TUI -> `sendKeys` for `Ctrl+R` -> `waitForText` for `runs view coming soon`.
- **Ctrl+A navigation**: Launch TUI -> `sendKeys` for `Ctrl+A` -> `waitForText` for `approvals view coming soon`.
- **Full help**: Launch TUI -> `sendKeys` for `Ctrl+G` -> `waitForText` for both new bindings to appear.

**Manual Checks:**
- Launch the application (`go run .`) and verify the bottom bar visually contains `ctrl+r runs` and `ctrl+a approvals`.
- Press `Ctrl+R` and ensure the status bar logs "runs view coming soon".
- Press `Ctrl+A` and ensure the status bar logs "approvals view coming soon".
- Open the command palette (`Ctrl+P`), search for "Run Dashboard", and verify it displays the `ctrl+r` hint and operates correctly.
- Add an attachment, press `Ctrl+Shift+R`, and confirm the attachment delete mode still activates properly without conflicting.

## Open Questions
- Should the `NavigateToViewMsg` logic be placed in a shared `messages.go` file right away to prepare for the view router, or kept within `ui.go` for now?
- Is there a specific timeout threshold we should establish as the standard for the PTY `waitForText` helper function in the new E2E harness to prevent flaky tests in CI?
- Will adding `Ctrl+A` as a global shortcut cause friction for users in environments (like tmux or screen) that heavily use it as a prefix, requiring them to lean solely on the command palette implementation?
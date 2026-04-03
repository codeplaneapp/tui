# Helpbar Shortcuts

## Metadata
- ID: chat-helpbar-shortcuts
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_HELPBAR_SHORTCUTS
- Dependencies: none

## Summary

Update the bottom help bar and global keymap to include Smithers-specific shortcuts.

## Acceptance Criteria

- The help bar displays new shortcuts like `ctrl+r runs` and `ctrl+a approvals`.
- Pressing the configured shortcuts triggers the appropriate view switch in the application.

## Source Context

- internal/ui/model/keys.go
- internal/ui/model/ui.go

## Implementation Notes

- Add `RunDashboard` (`ctrl+r`) and `Approvals` (`ctrl+a`) key bindings to `DefaultKeyMap()` in `internal/ui/model/keys.go`.
- Implement handling for these keybindings in the main `UI.Update()` function to push the respective views.

---

## Objective

Extend Crush's bottom help bar and global keymap with Smithers-specific keyboard shortcuts so that users can navigate to the Run Dashboard (`Ctrl+R`) and Approval Queue (`Ctrl+A`) with a single chord from any context. This is the `CHAT_SMITHERS_HELPBAR_SHORTCUTS` feature from the canonical feature inventory (`docs/smithers-tui/features.ts:39`). The shortcuts must appear in both the short-form and full-form help views at the bottom of the chat screen, matching the layout shown in the Design Document (§3.1, line 116–119 of `02-DESIGN.md`):

```
/ or ctrl+p  commands    shift+enter  newline        ctrl+g  less
ctrl+r       runs        @            mention file   ctrl+c  quit
ctrl+s       sessions    ctrl+o       open editor
```

## Scope

### In scope

1. **Two new global key bindings**: `Ctrl+R` → Run Dashboard, `Ctrl+A` → Approval Queue.
2. **KeyMap struct extension**: New fields on the top-level `KeyMap` struct in `internal/ui/model/keys.go`.
3. **Help bar text**: Both `ShortHelp()` and `FullHelp()` on `*UI` updated to include the new bindings.
4. **Key press handling**: `handleKeyPressMsg` in `internal/ui/model/ui.go` routes the new key events. Initially, since the view router (`internal/ui/views/router.go`) and the actual Run Dashboard / Approval Queue views are delivered by separate tickets (`RUNS_DASHBOARD`, `APPROVALS_QUEUE`), the handler emits a typed `tea.Msg` (e.g., `NavigateToViewMsg{View: "runs"}`) that the router will consume once it exists. Before the router lands, the handler falls through to a no-op info status message ("Runs view not yet available") so the binding is safely wired end-to-end.
5. **Command palette alignment**: The existing `defaultCommands()` in `internal/ui/dialog/commands.go` is extended with entries for "Run Dashboard" (`ctrl+r`) and "Approval Queue" (`ctrl+a`) so that the shortcut hints shown in the palette match the new global keys.
6. **`Ctrl+R` conflict resolution**: Crush currently uses `Ctrl+R` for attachment-delete mode (`Editor.AttachmentDeleteMode`, `keys.go:136–138`). This must be reassigned; see Slice 1 below.
7. **Unit tests, terminal E2E test, VHS happy-path recording**.

### Out of scope

- Implementing the Run Dashboard view itself (ticket `RUNS_DASHBOARD`).
- Implementing the Approval Queue view itself (ticket `APPROVALS_QUEUE`).
- The view-stack router (ticket `PLATFORM_VIEW_STACK_ROUTER`).
- Any other Smithers-specific shortcuts beyond `Ctrl+R` and `Ctrl+A` (e.g., `Ctrl+W` for workflows is not part of this ticket).

## Implementation Plan

### Slice 1 — Resolve the `Ctrl+R` key conflict

**Problem**: `Ctrl+R` is currently bound to `Editor.AttachmentDeleteMode` (`internal/ui/model/keys.go:136–138`). The Smithers design doc (§5 of `02-DESIGN.md`) assigns `Ctrl+R` globally to the Run Dashboard. Attachment-delete mode is a sub-mode of the editor focus state; globally the new Smithers binding takes priority.

**Files changed**:
- `internal/ui/model/keys.go` — Change `Editor.AttachmentDeleteMode` from `ctrl+r` to `ctrl+shift+r`. Update the `WithHelp` string accordingly: `key.WithHelp("ctrl+shift+r+{i}", "delete attachment at index i")`.
- `internal/ui/model/keys.go` — Change `Editor.DeleteAllAttachments` help text from `"ctrl+r+r"` to `"ctrl+shift+r+r"`.
- `internal/ui/model/ui.go` — Verify that the attachment-delete handling in the editor focus branch (around line 1719) matches on the binding object, not on a hard-coded key string, so the reassignment propagates automatically.
- `internal/ui/model/ui.go` — Update `FullHelp()` (line 2278–2284) so the attachment-mode help hint reflects the new chord.

**Verification**: `go build ./...` passes. Existing unit tests in `internal/ui/model/ui_test.go` still pass. Manual test: launch TUI, add an attachment, confirm `Ctrl+Shift+R` enters delete mode.

### Slice 2 — Add `RunDashboard` and `Approvals` key bindings to `KeyMap`

**Files changed**:
- `internal/ui/model/keys.go`:
  - Add two new fields to the top-level `KeyMap` struct (alongside existing globals like `Quit`, `Help`, `Commands`, `Models`, `Sessions`):

    ```go
    RunDashboard key.Binding
    Approvals    key.Binding
    ```

  - In `DefaultKeyMap()`, initialize them:

    ```go
    km.RunDashboard = key.NewBinding(
        key.WithKeys("ctrl+r"),
        key.WithHelp("ctrl+r", "runs"),
    )
    km.Approvals = key.NewBinding(
        key.WithKeys("ctrl+a"),
        key.WithHelp("ctrl+a", "approvals"),
    )
    ```

**Verification**: `go build ./...` compiles. `grep -r 'ctrl+r' internal/ui/model/keys.go` shows exactly one occurrence (the new `RunDashboard` binding); the old attachment binding now references `ctrl+shift+r`.

### Slice 3 — Wire key press handling in `UI.Update()`

**Files changed**:
- `internal/ui/model/ui.go`:
  - Define a new message type at the top of the file (or in a shared messages file):

    ```go
    // NavigateToViewMsg requests a view switch. Consumed by the view
    // router once PLATFORM_VIEW_STACK_ROUTER lands; until then the
    // main Update() shows a placeholder status message.
    type NavigateToViewMsg struct {
        View string // "runs", "approvals", etc.
    }
    ```

  - In `handleKeyPressMsg`, inside the `handleGlobalKeys` closure (around line 1608–1663), add two new cases **before** the existing `Suspend` case:

    ```go
    case key.Matches(msg, m.keyMap.RunDashboard):
        cmds = append(cmds, func() tea.Msg {
            return NavigateToViewMsg{View: "runs"}
        })
        return true
    case key.Matches(msg, m.keyMap.Approvals):
        cmds = append(cmds, func() tea.Msg {
            return NavigateToViewMsg{View: "approvals"}
        })
        return true
    ```

  - In the top-level `Update()` method, add a handler for `NavigateToViewMsg`. If the view router exists (`m.router != nil`), delegate to it. Otherwise, emit a status info message:

    ```go
    case NavigateToViewMsg:
        if m.router != nil {
            cmd := m.router.Push(msg.View)
            return m, cmd
        }
        cmds = append(cmds, util.ReportInfo(
            fmt.Sprintf("%s view coming soon", msg.View),
        ))
    ```

**Why global keys and not editor-scoped**: The design doc (§5) marks `Ctrl+R` and `Ctrl+A` as "Any" context — they work regardless of focus state. Placing them in `handleGlobalKeys` ensures they fire even when the editor or chat pane has focus, matching the Smithers keybinding table.

**Verification**: Launch TUI, press `Ctrl+R` — status bar shows "runs view coming soon". Press `Ctrl+A` — status bar shows "approvals view coming soon". Neither chord interferes with typing in the editor (because they are ctrl-modified, and the editor doesn't intercept arbitrary ctrl chords).

### Slice 4 — Update `ShortHelp()` and `FullHelp()`

**Files changed**:
- `internal/ui/model/ui.go`:
  - `ShortHelp()` (line 2142–2213): In the `uiChat` branch, insert `k.RunDashboard` and `k.Approvals` into the returned bindings slice right after `k.Models`:

    ```go
    binds = append(binds,
        tab,
        commands,
        k.Models,
        k.RunDashboard,   // NEW
        k.Approvals,      // NEW
    )
    ```

  - `FullHelp()` (line 2215–2348): In the `uiChat` branch, add the two bindings into the `mainBinds` slice after `k.Sessions`:

    ```go
    mainBinds = append(mainBinds,
        tab,
        commands,
        k.Models,
        k.Sessions,
        k.RunDashboard,  // NEW
        k.Approvals,     // NEW
    )
    ```

  - Also update the `default` branch (line 2309–2337) with matching entries so the shortcuts appear even before a session is selected.

**Verification**: Launch TUI, confirm help bar at bottom shows `ctrl+r runs` and `ctrl+a approvals`. Press `Ctrl+G` to toggle full help; confirm both bindings appear in the expanded view.

### Slice 5 — Align command palette entries

**Files changed**:
- `internal/ui/dialog/commands.go`:
  - In `defaultCommands()` (around line 420), add two entries:

    ```go
    NewCommandItem(c.com.Styles, "run_dashboard", "Run Dashboard", "ctrl+r",
        ActionNavigate{View: "runs"}),
    NewCommandItem(c.com.Styles, "approval_queue", "Approval Queue", "ctrl+a",
        ActionNavigate{View: "approvals"}),
    ```

  - Define `ActionNavigate` if it does not already exist:

    ```go
    type ActionNavigate struct {
        View string
    }
    ```

  - In the `Commands.Update()` method's action-dispatch switch, handle `ActionNavigate` by returning a `NavigateToViewMsg`.

**Verification**: Open command palette (`Ctrl+P`), type "run" — "Run Dashboard" appears with `ctrl+r` hint. Select it — same behavior as pressing `Ctrl+R` directly.

### Slice 6 — Unit tests

**Files changed**:
- `internal/ui/model/keys_test.go` (new file or extend existing):
  - Test that `DefaultKeyMap().RunDashboard` key set is `["ctrl+r"]`.
  - Test that `DefaultKeyMap().Approvals` key set is `["ctrl+a"]`.
  - Test that `DefaultKeyMap().Editor.AttachmentDeleteMode` key set is `["ctrl+shift+r"]` (no longer `ctrl+r`).

- `internal/ui/model/ui_test.go` (extend):
  - Using Bubble Tea's `teatest` pattern, send a `tea.KeyPressMsg` for `ctrl+r` and assert the resulting command emits `NavigateToViewMsg{View: "runs"}`.
  - Same for `ctrl+a` → `NavigateToViewMsg{View: "approvals"}`.
  - Test that `ShortHelp()` returns a slice containing a binding with help key `"ctrl+r"` and help desc `"runs"`.

**Verification**: `go test ./internal/ui/model/... -run TestSmithersHelpbarShortcuts -v` passes.

### Slice 7 — Terminal E2E test (tui-test harness)

Model this on the upstream `@microsoft/tui-test` harness pattern used in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`. Those tests launch the TUI process, wait for it to render, send key sequences, and assert on terminal output via screen scraping.

**Files changed**:
- `tests/e2e/helpbar_shortcuts_test.go` (new):
  - Use the Go `os/exec` + `github.com/creack/pty` (or Bubble Tea's `teatest`) pattern to launch the built binary in a pseudo-terminal.
  - Helper functions modeled on `tui-helpers.ts` patterns:
    - `launchTUI(t *testing.T) *tuiProcess` — starts the binary, returns handle with pty fd.
    - `waitForText(proc, text, timeout)` — reads pty output until text appears or timeout.
    - `sendKeys(proc, keys)` — writes key sequences to pty stdin.
  - Test cases:
    1. **Help bar renders shortcuts**: Launch TUI → `waitForText("ctrl+r")` → assert `"runs"` appears on the same rendered frame.
    2. **Ctrl+R triggers navigation message**: Launch TUI → `sendKeys(ctrl+r)` → `waitForText("runs view coming soon")` (the placeholder from Slice 3).
    3. **Ctrl+A triggers navigation message**: Launch TUI → `sendKeys(ctrl+a)` → `waitForText("approvals view coming soon")`.
    4. **Full help shows both bindings**: Launch TUI → `sendKeys(ctrl+g)` → `waitForText("ctrl+r")` and `waitForText("ctrl+a")`.

**Verification**: `go test ./tests/e2e/... -run TestHelpbarShortcuts -v -timeout 30s` passes.

### Slice 8 — VHS happy-path recording test

**Files changed**:
- `tests/vhs/helpbar-shortcuts.tape` (new):

  ```tape
  # Helpbar Shortcuts — Happy Path
  # Verifies that Smithers-specific shortcuts appear in the help bar
  # and that pressing them triggers the expected navigation.

  Output tests/vhs/output/helpbar-shortcuts.gif
  Set Shell "bash"
  Set FontSize 14
  Set Width 120
  Set Height 40
  Set Theme "Catppuccin Mocha"

  # Launch the TUI
  Type "go run . 2>/dev/null"
  Enter
  Sleep 3s

  # Verify help bar is visible with new shortcuts
  Screenshot tests/vhs/output/helpbar-shortcuts-initial.png

  # Press Ctrl+R to navigate to runs
  Ctrl+R
  Sleep 1s
  Screenshot tests/vhs/output/helpbar-shortcuts-ctrl-r.png

  # Press Escape to return
  Escape
  Sleep 500ms

  # Press Ctrl+A to navigate to approvals
  Ctrl+A
  Sleep 1s
  Screenshot tests/vhs/output/helpbar-shortcuts-ctrl-a.png

  # Toggle full help
  Escape
  Sleep 500ms
  Ctrl+G
  Sleep 1s
  Screenshot tests/vhs/output/helpbar-shortcuts-full-help.png

  # Exit
  Ctrl+C
  Sleep 500ms
  ```

- `Taskfile.yaml` — Add a `test:vhs` task:

  ```yaml
  test:vhs:
    desc: Run VHS recording tests
    cmds:
      - vhs tests/vhs/helpbar-shortcuts.tape
  ```

**Verification**: `task test:vhs` produces `tests/vhs/output/helpbar-shortcuts.gif`. Visual inspection confirms `ctrl+r runs` and `ctrl+a approvals` are visible in the help bar, and the navigation feedback is shown after each keypress.

## Validation

### Automated checks

| Check | Command | Pass criteria |
|-------|---------|---------------|
| Build | `go build ./...` | Zero errors |
| Unit tests | `go test ./internal/ui/model/... -v` | All pass, including new `TestSmithersHelpbarShortcuts*` |
| Full test suite | `go test -race -failfast ./...` | No regressions |
| Terminal E2E | `go test ./tests/e2e/... -run TestHelpbarShortcuts -v -timeout 30s` | All 4 E2E cases pass |
| VHS recording | `vhs tests/vhs/helpbar-shortcuts.tape` | Exits 0, produces `.gif` output |
| Lint | `golangci-lint run ./...` | No new warnings |

### Manual verification

1. **Help bar visual check**: Launch `go run .`, confirm the bottom bar shows `ctrl+r  runs` and `ctrl+a  approvals` alongside existing shortcuts (`ctrl+p commands`, `ctrl+s sessions`, `ctrl+g more`, `ctrl+c quit`).
2. **Full help toggle**: Press `Ctrl+G`, confirm the expanded help view includes both new bindings in the "main" column.
3. **Ctrl+R fires**: Press `Ctrl+R` — status bar shows "runs view coming soon" (or navigates to runs view if the router is already landed).
4. **Ctrl+A fires**: Press `Ctrl+A` — status bar shows "approvals view coming soon" (or navigates to approvals view).
5. **No attachment-delete regression**: Add a file attachment to the editor, press `Ctrl+Shift+R` — attachment delete mode activates. Confirm old `Ctrl+R` no longer triggers delete mode.
6. **Command palette alignment**: Open command palette (`Ctrl+P`), search for "Run" — "Run Dashboard" item appears with `ctrl+r` shortcut hint. Select it — same navigation behavior.
7. **No editor interference**: With editor focused, type normal text including the letter "r" and "a" — no accidental view navigation (bindings require Ctrl modifier).

### Terminal E2E coverage (modeled on upstream `@microsoft/tui-test`)

The E2E tests in Slice 7 follow the same structural pattern as `../smithers/tests/tui.e2e.test.ts`:
- **Process launch**: Start the compiled binary in a pty (equivalent to `tui-helpers.ts`'s `launchTUI()`).
- **Screen scraping**: Read terminal output and assert on rendered text (equivalent to `waitForText()` / `expectScreen()` in `tui-helpers.ts`).
- **Key injection**: Send raw key sequences to pty stdin (equivalent to `sendKeys()` in `tui-helpers.ts`).
- **Assertions**: Confirm specific text appears on screen within a timeout.

Coverage matrix:

| Test case | Keys sent | Expected screen text |
|-----------|-----------|---------------------|
| Help bar renders | (none — initial render) | `ctrl+r` and `runs` on same frame |
| Ctrl+R navigation | `Ctrl+R` | `runs view coming soon` |
| Ctrl+A navigation | `Ctrl+A` | `approvals view coming soon` |
| Full help shows bindings | `Ctrl+G` | `ctrl+r` and `ctrl+a` in expanded help |

### VHS happy-path recording

The VHS tape in Slice 8 (`tests/vhs/helpbar-shortcuts.tape`) produces a visual recording that captures:
1. Initial help bar with new shortcuts visible.
2. `Ctrl+R` press and resulting feedback.
3. `Ctrl+A` press and resulting feedback.
4. Full help view (`Ctrl+G`) with both bindings.

The recording serves as both a regression artifact and a visual documentation asset.

## Risks

### 1. `Ctrl+R` conflict with attachment-delete mode (HIGH — mitigated by Slice 1)

**Impact**: `Ctrl+R` is currently `Editor.AttachmentDeleteMode`. Reassigning it to the Run Dashboard will break existing muscle memory for users who use attachment deletion.

**Mitigation**: Reassign attachment-delete to `Ctrl+Shift+R`. This is a less-used feature (only active when attachments are present and editor is focused) compared to the Run Dashboard shortcut which is global and high-frequency. The shift-modified chord is still discoverable via the full help view.

**Residual risk**: Low. Attachment deletion is a niche flow; the reassignment is documented in the help bar.

### 2. `Ctrl+A` may conflict with terminal "select all" expectations

**Impact**: Some terminal emulators intercept `Ctrl+A` (e.g., tmux prefix, GNU screen prefix, Emacs-style line-start). Users in those environments may not be able to reach the Smithers binding.

**Mitigation**: The binding is also reachable via the command palette (`Ctrl+P` → "Approval Queue"). The design doc acknowledges `Ctrl+A` in the keybinding table (§5 of `02-DESIGN.md`, line 867) so this is an accepted trade-off. Users can also remap via config if needed.

**Residual risk**: Medium. Users with tmux prefix set to `Ctrl+A` will need to double-tap or use the palette.

### 3. View router not yet landed

**Impact**: The `NavigateToViewMsg` message has no consumer until `PLATFORM_VIEW_STACK_ROUTER` is implemented. The shortcuts will fire but produce only a placeholder status message.

**Mitigation**: The placeholder message ("runs view coming soon") prevents user confusion. The `NavigateToViewMsg` type is designed so the router can consume it with zero changes to the keybinding code — it just needs to handle the message in its `Update()`.

**Residual risk**: Low. The shortcuts are fully wired; only the destination views are pending.

### 4. Crush upstream divergence on `Ctrl+R`

**Impact**: If upstream Crush changes the `Ctrl+R` attachment-delete binding or adds new bindings that conflict, cherry-picking those changes will require manual conflict resolution.

**Mitigation**: This is already accepted in the hard-fork strategy (`03-ENGINEERING.md` §1.1). The `keys.go` file is explicitly listed as a "Files to Modify" target. Keep the fork's `keys.go` changes minimal and well-commented to ease future merges.

**Residual risk**: Low.

### 5. No existing E2E test infrastructure

**Impact**: Crush currently has no terminal E2E or VHS test infrastructure. Slice 7 and 8 require creating this from scratch, including pty-based process management and the VHS toolchain.

**Mitigation**: The Go pty library (`github.com/creack/pty`) is mature and requires minimal setup. VHS is a single binary install (`go install github.com/charmbracelet/vhs@latest`). Both are additive — they don't modify existing test infrastructure.

**Residual risk**: Medium. The E2E helper functions (`launchTUI`, `waitForText`, `sendKeys`) will need iteration to handle timing flakiness across CI environments.

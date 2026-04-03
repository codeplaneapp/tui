# Implementation Summary: chat-helpbar-shortcuts

- Ticket: Helpbar Shortcuts
- Group: Chat And Console (chat-and-console)

## Summary

Implemented `chat-helpbar-shortcuts` on `impl/chat-helpbar-shortcuts` and committed as `24b546c394b1` (`feat: add smithers helpbar shortcuts`).

Key changes:
- Added global Smithers bindings in [keys.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/model/keys.go): `ctrl+r` (`runs`) and `ctrl+a` (`approvals`), and moved attachment delete mode to `ctrl+shift+r`.
- Wired navigation message flow in [ui.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/model/ui.go): new `NavigateToViewMsg`, global key handling, command action handling, and fallback status text (`"<view> view coming soon"`).
- Added command-palette navigation action in [actions.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/dialog/actions.go) and entries in [commands.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/dialog/commands.go) for Run Dashboard / Approval Queue.
- Updated help rendering (`ShortHelp` and `FullHelp`) to show `ctrl+r runs` and `ctrl+a approvals`.
- Added unit tests in [keys_test.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/model/keys_test.go) and [ui_shortcuts_test.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/ui/model/ui_shortcuts_test.go).
- Added terminal E2E test in [helpbar_shortcuts_test.go](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/internal/e2e/helpbar_shortcuts_test.go).
- Added VHS happy-path recording tape in [helpbar-shortcuts.tape](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/tests/vhs/helpbar-shortcuts.tape), updated [tests/vhs/README.md](/Users/williamcory/crush/.worktrees/chat-helpbar-shortcuts/tests/vhs/README.md), and generated output GIF/PNG artifacts.

Validation run:
- `go test ./internal/ui/model -run 'TestDefaultKeyMap|TestHandleKeyPressMsg_NavigateShortcuts|TestShortHelp_IncludesSmithersShortcutBindings|TestFullHelp_IncludesSmithersShortcutBindings|TestHandleNavigateToView_UsesComingSoonFallback|TestCurrentModelSupportsImages' -count=1` (pass)
- `go test ./internal/ui/dialog -count=1` (pass, no test files)
- `go test ./internal/e2e -run 'TestHelpbarShortcuts_TUI|TestSmithersDomainSystemPrompt' -count=1` (pass; skipped without `CRUSH_TUI_E2E=1`)
- `CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestHelpbarShortcuts_TUI -count=1 -v` (fails: existing startup crash path, not specific to this ticket)
- `CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestSmithersDomainSystemPrompt_TUI -count=1 -v` (fails with same existing startup crash)
- `vhs tests/vhs/helpbar-shortcuts.tape` (pass)
- `go test ./internal/ui/model -count=1` (pass)
- `go test ./internal/e2e -run TestHelpbarShortcuts_TUI -count=1` (pass; skipped without `CRUSH_TUI_E2E=1`)
- `go build ./...` (pass)

## Files Changed

- internal/e2e/helpbar_shortcuts_test.go
- internal/ui/dialog/actions.go
- internal/ui/dialog/commands.go
- internal/ui/model/keys.go
- internal/ui/model/keys_test.go
- internal/ui/model/ui.go
- internal/ui/model/ui_shortcuts_test.go
- tests/vhs/README.md
- tests/vhs/helpbar-shortcuts.tape
- tests/vhs/output/helpbar-shortcuts.gif
- tests/vhs/output/helpbar-shortcuts.png

## Validation

- go test ./internal/ui/model -run 'TestDefaultKeyMap|TestHandleKeyPressMsg_NavigateShortcuts|TestShortHelp_IncludesSmithersShortcutBindings|TestFullHelp_IncludesSmithersShortcutBindings|TestHandleNavigateToView_UsesComingSoonFallback|TestCurrentModelSupportsImages' -count=1 (pass)
- go test ./internal/ui/dialog -count=1 (pass)
- go test ./internal/e2e -run 'TestHelpbarShortcuts_TUI|TestSmithersDomainSystemPrompt' -count=1 (pass; skipped)
- CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestHelpbarShortcuts_TUI -count=1 -v (fails: startup crash)
- CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestSmithersDomainSystemPrompt_TUI -count=1 -v (fails: same startup crash)
- vhs tests/vhs/helpbar-shortcuts.tape (pass)
- go test ./internal/ui/model -count=1 (pass)
- go test ./internal/e2e -run TestHelpbarShortcuts_TUI -count=1 (pass; skipped)
- go build ./... (pass)

## Follow Up

- Investigate and stabilize the existing `CRUSH_TUI_E2E=1` startup crash path in `internal/e2e` so terminal E2E runs execute in CI rather than skipping/failing at boot.

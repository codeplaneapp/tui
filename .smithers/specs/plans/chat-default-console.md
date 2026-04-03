## Goal
Make Smithers chat the default/home console with stable back-stack semantics: launch into chat, keep chat as navigation root, and make `Esc` from any pushed Smithers view return to chat without breaking existing chat cancel behavior.

## Steps
1. Confirm dependency and scope guard first: verify `chat-ui-branding-status` is landed, and gate this behavior to Smithers mode (`Config.Smithers != nil`) so non-Smithers Crush flows are unchanged.
2. Add failing terminal E2E coverage first (modeled on upstream `@microsoft/tui-test`-style harness in `../smithers/tests/tui-helpers.ts` + `tui.e2e.test.ts`): spawn TUI, poll buffer, send keys, assert launch-at-chat and `Esc` back-to-chat from a secondary view.
3. Harden router root semantics in `internal/ui/views/router.go`: enforce a chat-root concept (`IsRoot` / `ResetToRoot` / no underflow) so stack pops cannot leave "no root" state.
4. Update startup routing in `internal/ui/model/ui.go` (and onboarding transitions) so Smithers configured startup defaults to `uiChat`, and initial-session restore is not gated only on `uiLanding`.
5. Implement `Esc` precedence in `UI.Update()` to minimize regressions: dialogs first, then Smithers pushed views back to chat root, while preserving current `uiChat` busy-agent cancel-on-`Esc` flow.
6. Fix rendering/help paths needed for this flow: ensure `uiSmithersView` layout is explicitly handled and help hints match back-to-chat semantics.
7. Add VHS happy-path recording for launch -> open secondary view -> `Esc` -> chat root, then run full validation.

## File Plan
- `internal/ui/model/ui.go`
- `internal/ui/model/onboarding.go`
- `internal/ui/views/router.go`
- `internal/ui/views/agents.go`
- `internal/ui/model/keys.go`
- `internal/ui/dialog/commands.go`
- `internal/e2e/tui_helpers_test.go`
- `internal/e2e/chat_default_console_test.go` (new)
- `tests/vhs/chat-default-console.tape` (new)
- `tests/vhs/README.md`
- Optional only if dependency is missing: `internal/ui/model/header.go`, `internal/ui/logo/logo.go`, `internal/ui/notification/native.go`

## Validation
1. Unit/integration pass for touched UI packages:
`go test ./internal/ui/views ./internal/ui/model -count=1`
2. Terminal E2E (explicitly modeled after upstream helper pattern: launch process, wait/poll, send keys, assert transitions):
`CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestChatDefaultConsole -count=1 -v`
3. VHS happy-path recording in this repo:
`vhs tests/vhs/chat-default-console.tape`
4. Full regression sweep:
`go test ./...`
5. Manual sanity check:
`CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=$(mktemp -d) go run .`
Then verify: launch lands in chat (not landing panel), open Agents from command dialog, press `Esc`, and return to chat root with Smithers branding visible.

## Open Questions
1. Should default-to-chat be Smithers-only, or should it replace current default behavior globally in Crush?
2. In chat root, should idle `Esc` remain "clear selection/cancel" only, with "back-to-chat" behavior limited to non-chat views?
3. Should `/console` command-palette routing be added in this ticket, or deferred to the command-palette extension ticket?
4. `../smithers/gui/src` and `../smithers/gui-ref` are unavailable in this checkout; confirm no additional GUI reference source is required for this ticket.
5. If `chat-ui-branding-status` is not merged yet, should this ticket block on it or carry a minimal branding subset?

```json
{
  "document": "## Goal\nMake Smithers chat the default/home console with stable back-stack semantics: launch into chat, keep chat as navigation root, and make Esc from any pushed Smithers view return to chat without breaking existing chat cancel behavior.\n\n## Steps\n1. Verify dependency `chat-ui-branding-status` and gate behavior to Smithers mode.\n2. Add failing terminal E2E first, modeled on upstream launch/poll/send-keys helpers.\n3. Add chat-root semantics to `internal/ui/views/router.go` (no underflow, reset-to-root).\n4. Change Smithers startup and session-restore flow in `internal/ui/model/ui.go` (+ onboarding transitions) to default to `uiChat`.\n5. Implement global Esc precedence in `UI.Update()` (dialogs first, non-chat views -> chat root, preserve busy-chat cancel).\n6. Ensure `uiSmithersView` layout/help paths are correct so secondary views render and back correctly.\n7. Add VHS happy path and run full validation.\n\n## File Plan\n- internal/ui/model/ui.go\n- internal/ui/model/onboarding.go\n- internal/ui/views/router.go\n- internal/ui/views/agents.go\n- internal/ui/model/keys.go\n- internal/ui/dialog/commands.go\n- internal/e2e/tui_helpers_test.go\n- internal/e2e/chat_default_console_test.go (new)\n- tests/vhs/chat-default-console.tape (new)\n- tests/vhs/README.md\n\n## Validation\n1. go test ./internal/ui/views ./internal/ui/model -count=1\n2. CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestChatDefaultConsole -count=1 -v\n3. vhs tests/vhs/chat-default-console.tape\n4. go test ./...\n5. Manual: CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=$(mktemp -d) go run . ; verify launch in chat, open Agents, Esc returns to chat root.\n\n## Open Questions\n1. Smithers-only default-to-chat, or global?\n2. Keep idle Esc in chat as clear-selection/cancel only?\n3. Add `/console` palette route now or defer?\n4. Missing ../smithers/gui/src and ../smithers/gui-ref: expected?\n5. If branding dependency is unmerged, block or include minimal branding patch?"
}
```
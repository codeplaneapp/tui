## Goal
Implement `chat-mcp-connection-status` by surfacing Smithers MCP connected/disconnected state in the chat-visible UI chrome, driven by existing MCP state events, with deterministic unit, terminal E2E, and VHS coverage.

## Steps
1. Lock the UI contract from ticket/docs before coding: display `smithers connected` or `smithers disconnected`; map MCP `connected` to connected and `starting/error/disabled/missing` to disconnected for first pass.
2. Add a small pure helper in UI model code to resolve the Smithers MCP state from `mcpStates` with stable name precedence: exact `smithers`, then `smithers-orchestrator`, then first configured MCP key containing `smithers`.
3. Add tests for that helper first (name resolution + state mapping) to minimize regressions before touching rendering.
4. Extend compact header rendering (`header.go`) to include a Smithers MCP indicator using existing resource styles/icons and keep current truncation behavior.
5. Add the same indicator to non-compact chat surface (`sidebar.go`) because compact header is not rendered in non-compact `uiChat`.
6. Reuse existing event flow only (`pubsub.Event[mcp.Event]` -> `handleStateChanged` -> `mcpStates`); avoid introducing new transport, polling, or cross-package state.
7. Add rendering tests for header/sidebar status text and icon state coverage (connected vs disconnected) including width-constrained cases.
8. Add terminal E2E coverage in `internal/e2e`, modeled on upstream harness behavior in `/Users/williamcory/smithers/tests/tui.e2e.test.ts` and `/Users/williamcory/smithers/tests/tui-helpers.ts`.
9. Add deterministic MCP test fixture (mock stdio server with optional startup delay) so E2E can assert a live transition from disconnected to connected.
10. Add one VHS happy-path tape that starts Crush with Smithers MCP fixture, captures visible connection status, and exits cleanly; update VHS README/fixtures.
11. Format and run full validation.

## File Plan
- [internal/ui/model/smithers_mcp_status.go](/Users/williamcory/crush/internal/ui/model/smithers_mcp_status.go) (new)
- [internal/ui/model/smithers_mcp_status_test.go](/Users/williamcory/crush/internal/ui/model/smithers_mcp_status_test.go) (new)
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go)
- [internal/ui/model/header_test.go](/Users/williamcory/crush/internal/ui/model/header_test.go) (new)
- [internal/ui/model/sidebar.go](/Users/williamcory/crush/internal/ui/model/sidebar.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) (signature/wiring updates only)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) (reuse/extend)
- [internal/e2e/chat_mcp_connection_status_test.go](/Users/williamcory/crush/internal/e2e/chat_mcp_connection_status_test.go) (new)
- [internal/e2e/testdata/mock_smithers_mcp/main.go](/Users/williamcory/crush/internal/e2e/testdata/mock_smithers_mcp/main.go) (new)
- [tests/vhs/smithers-mcp-connection-status.tape](/Users/williamcory/crush/tests/vhs/smithers-mcp-connection-status.tape) (new)
- [tests/vhs/fixtures/smithers-mcp-connection-status.json](/Users/williamcory/crush/tests/vhs/fixtures/smithers-mcp-connection-status.json) (new)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)

## Validation
1. `gofumpt -w internal/ui/model internal/e2e`
2. `go test ./internal/ui/model -run 'TestSmithersMCPStatus|TestRenderHeader' -count=1 -v`
3. `SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestSmithersMCPConnectionStatus_TUI -count=1 -v -timeout 120s`
4. `vhs tests/vhs/smithers-mcp-connection-status.tape`
5. `go test ./...`
6. Manual check: run Crush with config pointing `mcp.smithers` to delayed mock MCP server; verify UI shows disconnected first, then connected without restart.
7. Manual check: run Crush with invalid `mcp.smithers.command`; verify disconnected/error state is visible and TUI remains usable.
8. Harness parity check: confirm E2E helper behavior matches upstream pattern (spawn process, ANSI-stripped buffered polling every ~100ms, `WaitForText`, `WaitForNoText`, `SendKeys`, `Snapshot`, `Terminate`) from `/Users/williamcory/smithers/tests/tui-helpers.ts` and scenario shape from `/Users/williamcory/smithers/tests/tui.e2e.test.ts`.
9. VHS happy-path check: generated GIF/PNG shows visible Smithers MCP status indicator in chat UI.

## Open Questions
1. Should `starting` display as `disconnected` (first-pass plan) or as a third `connecting` state?
2. Is server-name fallback (`smithers`, `smithers-orchestrator`, then contains `smithers`) acceptable, or should we enforce exact `smithers` only?
3. Should this ticket require both compact-header and non-compact sidebar visibility, or is compact-header-only acceptable?
4. The engineering spec file is currently a placeholder (`.smithers/specs/engineering/chat-mcp-connection-status.md` points to itself). Should this ticket also backfill that spec?
5. Requested reference paths `../smithers/gui/src` and `../smithers/gui-ref` are missing in the local `/Users/williamcory/smithers` checkout. Is `/Users/williamcory/smithers/src` plus `tests/tui.e2e.test.ts` and `tests/tui-helpers.ts` the authoritative reference for this pass?
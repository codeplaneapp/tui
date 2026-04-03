## Goal
Implement a first-pass, low-regression global pending-approvals badge for Crush that is visible on the main UI when pending approvals exist, and updates dynamically from Smithers SSE-triggered refreshes.

## Steps
1. Lock contract and dependencies first. Confirm current approval/status semantics from `/Users/williamcory/smithers/src/server/index.ts` and `/Users/williamcory/smithers/src/db/adapter.ts` (`status: requested`, global approvals list response shape, run-scoped SSE endpoint).
2. Stabilize Smithers approval transport in `internal/smithers` before UI work. Normalize HTTP response decoding for approvals (plain JSON + envelope compatibility), and normalize upstream `requested` to UI-facing pending state.
3. Add minimal run/event primitives needed for badge updates. Introduce lightweight run/event types plus an SSE consumer (`event: smithers`) for run streams, with reconnect/backoff and cancellation-safe shutdown.
4. Wire Smithers config into UI client construction. Replace bare `smithers.NewClient()` in UI with config-driven options (`APIURL`, `APIToken`, `DBPath`) so badge state can use real API/DB transports.
5. Add badge state and update loop in UI model. Keep `pendingApprovalsCount` in `UI`, do initial count load, refresh on approval-related SSE events, and keep a periodic full refresh fallback to cover missed/disconnected streams.
6. Render the badge in global chrome with low layout risk. Add a compact header badge token and a status-bar fallback for non-compact chat layout so the indicator is visible across main UI modes when count > 0.
7. Keep approvals view behavior stable. Do not refactor routing; only add minimal integration needed so badge updates do not regress current approvals/tickets/agents view navigation.
8. Add tests in risk order: `internal/smithers` unit tests first (transport + SSE parsing), then UI rendering/state tests, then terminal E2E, then VHS recording.

## File Plan
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/smithers/events.go](/Users/williamcory/crush/internal/smithers/events.go) (new)
- [internal/smithers/events_test.go](/Users/williamcory/crush/internal/smithers/events_test.go) (new)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go)
- [internal/ui/model/status.go](/Users/williamcory/crush/internal/ui/model/status.go)
- [internal/ui/styles/styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go)
- [internal/ui/model/header_test.go](/Users/williamcory/crush/internal/ui/model/header_test.go) (new)
- [internal/ui/model/status_test.go](/Users/williamcory/crush/internal/ui/model/status_test.go) (new)
- [internal/e2e/approvals_pending_badges_test.go](/Users/williamcory/crush/internal/e2e/approvals_pending_badges_test.go) (new)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) (reuse; update only if required)
- [tests/vhs/approvals-pending-badges.tape](/Users/williamcory/crush/tests/vhs/approvals-pending-badges.tape) (new)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)

## Validation
1. `gofumpt -w internal/smithers internal/ui/model internal/ui/styles internal/e2e`
2. `go test ./internal/smithers -run 'TestListPendingApprovals|TestStreamRunEvents|TestParseSmithersSSE' -count=1 -v`
3. `go test ./internal/ui/model -run 'Test.*Approvals.*Badge|Test.*Header.*Badge|Test.*Status.*Badge' -count=1 -v`
4. `SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestApprovalsPendingBadges_TUI -count=1 -v -timeout 120s`
5. Terminal E2E modeling check: ensure the new test follows upstream harness semantics from `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` (launch, waitForText/waitForNoText polling, sendKeys, snapshot-on-failure, terminate lifecycle).
6. `vhs tests/vhs/approvals-pending-badges.tape`
7. Manual check: launch Crush with Smithers config, trigger a run into approval pending, verify badge appears without manual refresh, approve/deny, verify badge count decrements/disappears.

## Open Questions
1. As of April 4, 2026, `../smithers/gui/src`, `../smithers/gui-ref`, and `../smithers/docs/guides/smithers-tui-v2-agent-handoff.md` are not present in `/Users/williamcory/smithers`; should `/Users/williamcory/crush/smithers_tmp/gui-src`, `/Users/williamcory/crush/smithers_tmp/gui-ref`, and `/Users/williamcory/crush/smithers_tmp/docs/guides/smithers-tui-v2-agent-handoff.md` remain the accepted fallback references for this ticket?
2. Should this ticket render the badge in header only, status bar only, or both (recommended: both, because non-compact chat does not render the header)?
3. Upstream exposes run-scoped SSE (`/v1/runs/:id/events`) but no global approvals SSE stream; is the run-subscription + periodic full refresh fallback acceptable for this first pass?
4. Should approval status normalization (`requested` -> pending) be done only in the approvals path for this ticket, or standardized across all Smithers status handling now?
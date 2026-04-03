## Goal
Deliver the first approvals-queue implementation pass in Crush so `ctrl+a` opens a live approval queue backed by Smithers data (HTTP/SQLite/exec fallback), shows pending + recent decisions, and updates on approval-related SSE events with minimal regressions to existing Smithers view routing.

## Steps
1. Reconcile prerequisites before feature work. Confirm `eng-approvals-view-scaffolding` artifacts exist (`approvals` view route/action/keybinding shell); if missing, land the minimal scaffold in the same branch first so queue work has a stable entrypoint.
2. Add approval data contracts in `internal/smithers`. Introduce an `Approval`/queue row model that can normalize upstream core server (`/v1/approvals`/`/approvals`) and daemon (`/api/approvals`) payloads, including status normalization (`requested` -> `pending`) and wait-time derivation.
3. Implement `ListPendingApprovals(ctx)` in `internal/smithers/client.go` with conservative transport order to reduce breakage: HTTP first (support plain JSON and envelope JSON for this path), SQLite second (join `_smithers_approvals`, `_smithers_runs`, `_smithers_nodes`), exec fallback last (via `smithers sql --query ... --format json` if HTTP/DB are unavailable).
4. Add SSE consumer support in `internal/smithers/events.go` for `/v1/runs/{runId}/events` (`event: smithers`) with reconnect/backoff and typed event parsing for approval events (`ApprovalRequested`, `ApprovalGranted`, `ApprovalDenied`, `NodeWaitingApproval`).
5. Implement `internal/ui/views/approvals.go` as a real data view: load on `Init`, refresh on `r`, pop on `esc`, cursor nav (`up/down/j/k`), pending section + recent decisions section, loading/empty/error states, responsive width handling, and wait-age formatting thresholds from design.
6. Wire navigation and client construction in UI integration points. Add `ActionOpenApprovalsView` + command palette item (`approvals`), route both palette + `ctrl+a` through one helper in `ui.go`, and construct `smithers.NewClient(...)` with config-driven options (`APIURL`, `APIToken`, `DBPath`) instead of defaults.
7. Fix Smithers-view message routing risk before finalizing queue behavior. Remove the current double-dispatch path in `ui.go` so key presses/messages are processed once when `uiSmithersView` is active.
8. Add test coverage in increasing scope: smithers client unit tests, approvals view unit tests, terminal E2E for queue navigation/live update path modeled on upstream harness behavior, and one VHS happy-path recording.
9. Run formatting + full verification sweep, then do a manual live check against a mock/real Smithers server to confirm SSE-driven queue updates without manual refresh.

## File Plan
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/smithers/events.go](/Users/williamcory/crush/internal/smithers/events.go) (new)
- [internal/smithers/events_test.go](/Users/williamcory/crush/internal/smithers/events_test.go) (new)
- [internal/ui/views/approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go) (new or update, depending on scaffold state)
- [internal/ui/views/approvals_test.go](/Users/williamcory/crush/internal/ui/views/approvals_test.go) (new)
- [internal/ui/model/keys.go](/Users/williamcory/crush/internal/ui/model/keys.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/e2e/approvals_queue_test.go](/Users/williamcory/crush/internal/e2e/approvals_queue_test.go) (new)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) (reuse/minor update only if needed)
- [tests/vhs/approvals-queue.tape](/Users/williamcory/crush/tests/vhs/approvals-queue.tape) (new)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)

## Validation
1. `gofumpt -w internal/smithers internal/ui/views internal/ui/model internal/ui/dialog internal/e2e`
2. `go test ./internal/smithers -run 'TestListPendingApprovals|TestSmithersEventStream' -v`
3. `go test ./internal/ui/views -run TestApprovals -v`
4. `go test ./internal/ui/model -run 'Test.*Smithers.*|Test.*Approvals.*' -v`
5. `SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestApprovalsQueue_TUI -count=1 -v -timeout 90s`
6. Terminal E2E implementation check (modeled on upstream `../smithers/tests/tui.e2e.test.ts` + `../smithers/tests/tui-helpers.ts`): verify the test uses launch/poll `WaitForText`/`SendKeys`/`Snapshot` on failure/`Terminate` lifecycle.
7. `vhs tests/vhs/approvals-queue.tape`
8. Manual check: run `go run .`, press `ctrl+a`, confirm pending approvals render; inject/trigger an `ApprovalRequested` SSE event and confirm queue updates without pressing `r`; press `esc` and confirm return to prior screen.

## Open Questions
1. `../smithers/gui/src` and `../smithers/gui-ref` are not present in this checkout; confirm `../smithers/src`, `../smithers/apps/daemon`, and `smithers_tmp/gui-ref` are the accepted authoritative references for this ticket.
2. Should this ticket include the missing scaffold dependency (`eng-approvals-view-scaffolding`) in the same PR if it is still not merged (current codebase has no `internal/ui/views/approvals.go`)?
3. For `RECENT DECISIONS`, should we source resolved approvals from local SQLite only, or is there an approved HTTP endpoint to consume for this section?
4. Do we want to generalize Smithers client JSON decoding (plain JSON + envelope) across all existing methods now, or keep compatibility handling scoped to approvals to limit regression risk in this pass?
5. Is `smithers sql --query ... --format json` the approved exec fallback for approvals queue, given the current CLI has no `approve --list` command?
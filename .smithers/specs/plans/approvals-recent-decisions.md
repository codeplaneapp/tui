## Goal
Deliver the first implementation pass for `approvals-recent-decisions` by adding a stable, workspace-aware recent-decisions data path and rendering approved/denied history (with timestamps) inside the Approvals view, while sequencing work to avoid rework against `eng-approvals-view-scaffolding` and queue work.

## Steps
1. Gate on dependencies before feature code. Confirm `eng-approvals-view-scaffolding` (and the approvals view entrypoint) is present; if not, land the minimal scaffold first so recent-decisions work is layered, not duplicated.
2. Normalize approvals data contracts in `internal/smithers`. Add/extend approval types so one model can represent pending and decided rows from the daemon approvals feed (`approved`, `denied`, `pending`) and include `decided_at`/`decided_by` fields.
3. Add workspace-aware client methods in `internal/smithers/client.go`. Implement `ListApprovals(ctx, workspaceID string)` using HTTP primary path `GET /api/workspaces/{workspaceId}/approvals` (plain JSON), then derive `ListRecentApprovalDecisions(ctx, workspaceID string, limit int)` by filtering/sorting decided rows by decision timestamp.
4. Add conservative fallback behavior with explicit schema targeting. For non-HTTP fallback, query the daemon `approvals` table schema (not `_smithers_approvals`) and keep timestamp parsing aligned with ISO datetime strings; if fallback transport is unavailable, return a typed error and keep UI resilient.
5. Wire client construction and workspace propagation in UI integration points. In `internal/ui/model/ui.go`, pass Smithers client options from config (`APIURL`, `APIToken`, `DBPath`) and thread a workspace ID source into approvals view/client calls.
6. Implement recent-decisions UI behavior in `internal/ui/views/approvals.go`. Add a toggle/section for `RECENT DECISIONS`, cursor handling for that list, empty/loading/error states, refresh (`r`), and decision rows showing label/run/node + decision + timestamp (relative or absolute).
7. Keep navigation/help wiring regression-safe. If not already present from dependency tickets, add approvals action/command routing and view short-help updates without changing unrelated chat/editor key behavior.
8. Add tests in increasing scope: client unit tests for HTTP + fallback + parsing/sorting, approvals view unit tests for toggle/render/cursor states, terminal E2E flow, then VHS happy-path recording.
9. Run formatting and full verification sweep, then do a manual keyboard pass to confirm no state-routing regressions in `uiSmithersView`.

## File Plan
- `internal/smithers/types.go`
- `internal/smithers/client.go`
- `internal/smithers/client_test.go`
- `internal/ui/views/approvals.go` (new or update)
- `internal/ui/views/approvals_test.go` (new)
- `internal/ui/model/ui.go`
- `internal/ui/dialog/actions.go` (if approvals action is still missing)
- `internal/ui/dialog/commands.go` (if approvals command is still missing)
- `internal/config/config.go` (only if workspace ID is added to config)
- `internal/config/load.go` (only if workspace ID default/loading is added)
- `internal/e2e/approvals_recent_decisions_test.go` (new)
- `internal/e2e/tui_helpers_test.go` (reuse; update only if needed)
- `tests/vhs/approvals-recent-decisions.tape` (new)
- `tests/vhs/README.md`

## Validation
- `gofumpt -w internal/smithers internal/ui/views internal/ui/model internal/ui/dialog internal/e2e`
- `go test ./internal/smithers -run 'TestListApprovals|TestListRecentApprovalDecisions' -v`
- `go test ./internal/ui/views -run TestApprovalsViewRecentDecisions -v`
- `go test ./internal/ui/model -run 'Test.*Smithers.*|Test.*Approvals.*' -v`
- `SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestApprovalsRecentDecisions_TUI -count=1 -v -timeout 90s`
- `vhs tests/vhs/approvals-recent-decisions.tape`
- Manual check: launch `go run .`, open Approvals view, switch to recent decisions, verify approved/denied rows show timestamps, refresh with `r`, and `esc` returns to prior view.
- Terminal E2E modeling check: ensure test structure mirrors upstream harness semantics from `../smithers/tests/tui-helpers.ts` and flow style from `../smithers/tests/tui.e2e.test.ts` (launch, wait/poll, send keys, snapshot-on-failure, terminate), implemented in Crush’s Go harness (`internal/e2e/tui_helpers_test.go`).

## Open Questions
1. What is the authoritative workspace ID source for Crush right now (config value, derived workspace mapping, or selected workspace state)?
2. Should approvals HTTP support be daemon-only (`/api/workspaces/{workspaceId}/approvals`), or do we need temporary compatibility with core Smithers routes in the same method?
3. Which local DB path is canonical for fallback in this repo for approvals history (daemon `approvals` table) when current Smithers config defaults still point to `.smithers/smithers.db`?
4. Is `approvals-queue` expected to land before this ticket, or should this ticket include the minimal shared approvals view state needed to avoid branch churn?
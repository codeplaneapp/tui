## Goal
Deliver a regression-safe Systems and Analytics API client slice in Crush for SQL, scores, memory, and cron/trigger operations, with correct transport fallback behavior and explicit terminal E2E + VHS coverage for this ticket’s first pass.

## Steps
1. Freeze the contract before edits.
2. Compare ticket/spec expectations against current references in `../smithers/src`, `../smithers/src/server`, `../smithers/tests/tui.e2e.test.ts`, and `../smithers/tests/tui-helpers.ts`, and record where endpoints/CLI outputs differ.
3. Correct data-layer parity first in `internal/smithers` (table names, transport order, and parser shape handling) before touching UI wiring.
4. Update client queries to current schema names (`_smithers_scorers`, `_smithers_cron`, `_smithers_memory_facts`) and remove dead fallback assumptions that target non-existent tables.
5. Harden parse helpers to accept both wrapped and direct payload forms for HTTP and CLI responses so routing changes do not break decoding.
6. Wire Smithers client options from config (`apiUrl`, `apiToken`, `dbPath`) into TUI initialization so HTTP/SQLite fallbacks are reachable in-product.
7. Add minimal Systems view scaffolding needed for verification only (`/sql` and `/triggers`) and wire command actions/navigation to those views.
8. Expand `internal/smithers/client_test.go` with transport, parsing, and schema-regression tests before E2E.
9. Add terminal E2E coverage in `internal/e2e` using the same harness semantics as upstream (`launch`, `waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`) and validate keyboard flow through SQL + Triggers.
10. Add one VHS happy-path recording for the same flow and document how to run it.
11. Run full validation and only then iterate on non-essential UI polish.

## File Plan
1. [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
2. [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
3. [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
4. [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
5. [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
6. [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
7. [internal/ui/views/sqlbrowser.go](/Users/williamcory/crush/internal/ui/views/sqlbrowser.go) (new)
8. [internal/ui/views/triggers.go](/Users/williamcory/crush/internal/ui/views/triggers.go) (new)
9. [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) (extend only if parity gaps exist)
10. [internal/e2e/systems_api_client_test.go](/Users/williamcory/crush/internal/e2e/systems_api_client_test.go) (new)
11. [tests/vhs/systems-api-client-happy-path.tape](/Users/williamcory/crush/tests/vhs/systems-api-client-happy-path.tape) (new)
12. [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)
13. [tests/vhs/fixtures](/Users/williamcory/crush/tests/vhs/fixtures) (new or extended fixture inputs)

## Validation
1. `gofumpt -w internal/smithers internal/ui/views internal/ui/dialog internal/ui/model internal/e2e`
2. `go test ./internal/smithers -count=1`
3. `go test ./internal/smithers -run 'TestExecuteSQL|TestGetScores|TestGetAggregateScores|TestListMemoryFacts|TestRecallMemory|TestListCrons|TestCreateCron|TestToggleCron|TestDeleteCron' -count=1 -v`
4. `CRUSH_TUI_E2E=1 go test ./internal/e2e -run TestSystemsAPIClient_TUI -count=1 -v -timeout 120s`
5. Terminal E2E parity check against upstream harness behavior in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: 100ms polling, ANSI stripping, keyboard injection, snapshot-on-failure, terminate cleanup.
6. `vhs tests/vhs/systems-api-client-happy-path.tape`
7. `go test ./...`
8. Manual check: launch Crush with Smithers config and navigate to `/sql` and `/triggers`; verify data loads with server up, then repeat with server down to confirm SQLite/exec fallback behavior.

## Open Questions
1. As of April 3, 2026, `../smithers/gui/src` and `../smithers/gui-ref` are not present in the active `../smithers` tree; should this pass treat those as missing and use `../smithers/src` + tests as the sole primary reference?
2. Current `../smithers/src` contract differs from the older cron/sql HTTP assumptions in the ticket/spec; should unsupported HTTP calls remain opportunistic or be explicitly downgraded to SQLite/exec-only paths?
3. Should this ticket include the minimal `/sql` and `/triggers` view scaffolding needed for required TUI E2E/VHS checks, or should it stay strictly client-only with harness smoke?
4. Should we standardize immediately on current schema tables (`_smithers_scorers`, `_smithers_cron`) and treat old table names as hard failures rather than compatibility fallbacks?
5. Is `RecallMemory` exec-only acceptable for this ticket given semantic recall depends on runtime embedding infrastructure?
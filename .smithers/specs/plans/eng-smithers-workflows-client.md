## Goal
Deliver a first-pass, regression-safe Smithers workflows client layer in Crush that exposes `ListWorkflows`, `GetWorkflow`, and `RunWorkflow` using current upstream behavior (`workflow` discovery + `POST /v1/runs`), then validate it through unit tests, terminal E2E (modeled on upstream harness semantics), and one VHS happy-path recording.

## Steps
1. Re-baseline contracts before coding: treat `.smithers/tickets/eng-smithers-workflows-client.md` and `.smithers/specs/engineering/eng-smithers-workflows-client.md` assumptions about `/api/workflows*` as stale, and lock expected behavior to `../smithers/src/server/index.ts` plus `../smithers/src/cli/workflows.ts`.
2. Preserve existing client behavior: keep current envelope-based helpers for existing SQL/scores/memory/cron methods, and add workflow-specific parsing paths so current tests and callers do not regress.
3. Add workflow domain types in `internal/smithers/types.go`: discovered workflow record, workflow inspection payload, run request/response payloads, and typed error payload for plain JSON server errors.
4. Implement `ListWorkflows` in `internal/smithers/client.go` with deterministic fallback ordering to minimize rework: project discovery parser first (matching upstream metadata markers), CLI fallback (`smithers workflow list --format json`), and optional read-only DB fallback for historical workflow names when discovery is unavailable.
5. Implement `GetWorkflow` in `internal/smithers/client.go`: resolve by ID from discovery results and enrich with optional doctor/path metadata via CLI where available, returning a stable shape even when optional metadata is missing.
6. Implement `RunWorkflow` in `internal/smithers/client.go`: resolve workflow entry file, execute via HTTP `POST /v1/runs` when server is reachable, and fall back to CLI execution (`smithers workflow run` or `smithers up`) when HTTP is unavailable.
7. Add focused unit coverage in `internal/smithers/client_test.go`: success, malformed payload, 404/error envelope, transport fallback, and command argument assertions for workflow operations.
8. Add a minimal workflows navigation surface in the TUI to exercise the new client path without waiting for full workflows feature tickets: lightweight workflows view shell, command-palette action, and router wiring.
9. Add terminal E2E coverage in this repo modeled on upstream `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` semantics (`launch`, `waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`) and validate a workflows happy path.
10. Add one VHS tape for the same happy path and wire repeatable commands in `Taskfile.yaml`.

## File Plan
1. [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
2. [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
3. [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
4. [internal/ui/views/workflows.go](/Users/williamcory/crush/internal/ui/views/workflows.go)
5. [internal/ui/views/workflows_test.go](/Users/williamcory/crush/internal/ui/views/workflows_test.go)
6. [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
7. [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
8. [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
9. [tests/tui/helpers_test.go](/Users/williamcory/crush/tests/tui/helpers_test.go)
10. [tests/tui/workflows_client_e2e_test.go](/Users/williamcory/crush/tests/tui/workflows_client_e2e_test.go)
11. [tests/vhs/workflows-client-happy-path.tape](/Users/williamcory/crush/tests/vhs/workflows-client-happy-path.tape)
12. [Taskfile.yaml](/Users/williamcory/crush/Taskfile.yaml)

## Validation
1. `gofumpt -w internal/smithers internal/ui/views internal/ui/model internal/ui/dialog tests/tui`
2. `go test ./internal/smithers -run TestListWorkflows -v`
3. `go test ./internal/smithers -run TestGetWorkflow -v`
4. `go test ./internal/smithers -run TestRunWorkflow -v`
5. `go test ./internal/ui/views -run TestWorkflowsView -v`
6. `go test ./tests/tui -run TestWorkflowsClientE2E -count=1 -v -timeout 120s`
7. Terminal E2E parity check: confirm helper APIs and test flow explicitly mirror upstream harness behavior from `../smithers/tests/tui-helpers.ts` and `../smithers/tests/tui.e2e.test.ts` (process spawn, ANSI-stripped polling, keystroke injection, snapshot-on-failure, terminate cleanup).
8. `vhs tests/vhs/workflows-client-happy-path.tape`
9. `go test ./...`
10. Manual check: run `go run .`, open command palette, navigate to workflows view, confirm discovered workflows render, trigger workflow run, and verify returned run id/state feedback.

## Open Questions
1. Should `GetWorkflow` include schema/parameter details in this ticket, or should schema-rich inspection be deferred to `workflows-agent-and-schema-inspection` and `workflows-dynamic-input-forms`?
2. Should Smithers config namespace wiring (`apiUrl`, `apiToken`, `dbPath`, `workflowDir`) be included here, or deferred to `platform-config-namespace` with this ticket using client options and sensible defaults?
3. For discovery, which source should be authoritative when outputs disagree: direct project parser, `smithers workflow list --format json`, or read-only DB fallback?
4. `../smithers/gui/src` and `../smithers/gui-ref` are not present in this checkout; confirm that `../smithers/src`, `../smithers/src/server`, and `../smithers/tests` are the accepted primary references for this pass.
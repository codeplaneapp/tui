## Goal
Align Crush’s Smithers frontend foundation with the real upstream Smithers transport contract so the TUI remains a thin presentation layer: typed run/event domain models, HTTP+SSE primary transport, SQLite/CLI fallback where valid, and minimal but extensible UI wiring for Smithers views and rendering.

## Steps
1. Lock the authoritative contract before code changes.
- Use `../smithers/src/server/index.ts`, `../smithers/src/server/serve.ts`, `../smithers/src/db/internal-schema.ts`, and `../smithers/packages/shared/src/schemas/*` as canonical for this pass.
- Capture route/table/CLI compatibility in tests first (especially `/v1/runs*`, approvals, SSE `event: smithers`, `_smithers_cron`, `_smithers_scorers`).

2. Expand Smithers domain types without breaking existing call sites.
- Extend `internal/smithers/types.go` with run-centric models required by this ticket family: `Run`, `RunStatus`, `RunNode`, `Attempt`, `Approval`, `RunEvent`, and minimal workflow/input types.
- Keep existing types (`Agent`, SQL/memory/scores/cron) and preserve JSON tags for backward compatibility.

3. Refactor transport helpers to support real server responses and fallback order.
- Add explicit HTTP helpers for current upstream error shape (`{ error: { code, message, details } }`) alongside existing envelope handling where still used.
- Implement/normalize run APIs (`ListRuns`, `GetRun`, `ListApprovals`, `ApproveNode`, `DenyNode`, `CancelRun`) with HTTP-first behavior.
- Keep read-only fallback semantics explicit: SQLite for safe reads, CLI fallback only for routes not available over HTTP.

4. Add a dedicated SSE stream consumer.
- Introduce `internal/smithers/events.go` for parsing `text/event-stream` payloads, handling `event: smithers`, keep-alive comments, sequence tracking, and cancel-safe shutdown.
- Keep this package transport-only (no UI-specific business logic).

5. Correct schema/CLI drift in existing methods before expanding UI usage.
- Update incorrect table names (`_smithers_crons` -> `_smithers_cron`, `_smithers_scorer_results` -> `_smithers_scorers` if applicable to current schema).
- Reconcile CLI fallbacks with currently supported Smithers CLI commands/flags.

6. Wire Smithers config into client construction in UI/app.
- Replace `smithers.NewClient()` default construction in UI with options sourced from `config.Smithers` (`APIURL`, `APIToken`, `DBPath`).
- Ensure lifecycle cleanup (`Close`) is deterministic and non-invasive to existing Crush flows.

7. Extend thin UI scaffolding in a regression-safe order.
- Keep router/back-stack pattern, then add Smithers command entries beyond `Agents` as placeholders or minimal data-backed views that consume the typed client.
- Add first Smithers-specific tool rendering hooks (runs/approvals) while preserving generic MCP renderer fallback.

8. Add automated coverage before broad feature rollout.
- Strengthen `internal/smithers` unit tests for contract parsing, transport fallbacks, and SSE behavior.
- Add terminal E2E coverage in this repo modeled on upstream `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` (launch, waitForText, sendKeys, snapshot-on-failure, terminate).
- Add at least one VHS happy-path recording for a Smithers TUI flow in this repo.

## File Plan
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/events.go](/Users/williamcory/crush/internal/smithers/events.go) (new)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/smithers/events_test.go](/Users/williamcory/crush/internal/smithers/events_test.go) (new)
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go) (only if additional smithers transport knobs are required)
- [internal/config/load.go](/Users/williamcory/crush/internal/config/load.go)
- [internal/app/app.go](/Users/williamcory/crush/internal/app/app.go) (if client ownership/lifecycle is moved out of UI)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/views/router.go](/Users/williamcory/crush/internal/ui/views/router.go)
- [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go)
- [internal/ui/views/runs.go](/Users/williamcory/crush/internal/ui/views/runs.go) (new, minimal first pass)
- [internal/ui/views/approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go) (new, minimal first pass)
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/ui/chat/tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go)
- [internal/ui/chat/mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go)
- [internal/ui/chat/smithers_runs.go](/Users/williamcory/crush/internal/ui/chat/smithers_runs.go) (new)
- [internal/ui/chat/smithers_approvals.go](/Users/williamcory/crush/internal/ui/chat/smithers_approvals.go) (new)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go)
- [internal/e2e/smithers_thin_frontend_e2e_test.go](/Users/williamcory/crush/internal/e2e/smithers_thin_frontend_e2e_test.go) (new)
- [tests/vhs/smithers-thin-frontend-happy-path.tape](/Users/williamcory/crush/tests/vhs/smithers-thin-frontend-happy-path.tape) (new)
- [tests/vhs/fixtures](/Users/williamcory/crush/tests/vhs/fixtures)

## Validation
1. `gofumpt -w internal/smithers internal/ui internal/e2e`
2. `go test ./internal/smithers -count=1`
3. `go test ./internal/smithers -run 'TestListRuns|TestGetRun|TestListApprovals|TestApproveNode|TestDenyNode|TestCancelRun|TestStreamRunEvents' -count=1 -v`
4. `go test ./internal/ui/... -count=1`
5. `go test ./internal/e2e -run TestSmithersThinFrontendTerminalFlow -count=1 -v -timeout 120s` (terminal E2E modeled on upstream `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: launch, wait/poll, keyboard input, snapshot on failure, terminate)
6. `vhs tests/vhs/smithers-thin-frontend-happy-path.tape` (VHS happy-path recording in this repo)
7. `go test ./...`
8. Manual transport checks against upstream Smithers server:
9. `cd /Users/williamcory/smithers && bun run src/cli/index.ts up examples/fan-out-fan-in.tsx -d`
10. `cd /Users/williamcory/smithers && bun run src/cli/index.ts serve --root . --port 7331`
11. `curl -s http://127.0.0.1:7331/v1/runs | jq '.[0] // .'`
12. `curl -N 'http://127.0.0.1:7331/v1/runs/<run-id>/events?afterSeq=-1'`
13. Manual TUI smoke: launch Crush with Smithers config, open Smithers view via command palette, verify view renders data from configured API URL and exits cleanly with `Esc`.

## Open Questions
1. `../smithers/gui/src` and `../smithers/gui-ref` are not present in this checkout as of April 3, 2026; should `../smithers/src` + `packages/shared` be treated as sole source of truth for this pass?
2. Should this ticket remain strictly transport/foundation, with richer Smithers views deferred to follow-on tickets, or should minimal `runs`/`approvals` views land here as proof of thin-client integration?
3. For direct SQLite fallback, do we keep PRD’s “HTTP/exec only” intent or preserve current read-only DB fallback for resiliency until upstream API parity is complete?
4. Which CLI fallbacks are mandatory in this phase versus intentionally unsupported until corresponding Smithers CLI subcommands stabilize?
5. Should Smithers client initialization live in `internal/ui/model/ui.go` (current) or be promoted to `internal/app` for clearer lifecycle ownership and easier reuse?

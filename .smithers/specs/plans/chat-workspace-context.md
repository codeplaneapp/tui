## Goal
Implement `chat-workspace-context` so the Smithers system prompt receives live workspace context at render time: workflow directory plus a filtered list of active runs (`.ActiveRuns`), with graceful fallback when Smithers transport is unavailable.

## Steps
1. Align contract and scope before code changes: treat `chat-domain-system-prompt` as dependency already landed, keep this ticket focused on prompt context injection (not chat header/summary UI), and normalize against the current upstream runs API (`GET /v1/runs`) plus current Crush architecture.
2. Add run context primitives in `internal/smithers` first, with tests first: introduce run summary types and `ListRuns` client support that can decode upstream raw JSON responses (and existing error envelope shape), then keep fallback behavior non-blocking.
3. Extend prompt payload plumbing: add Smithers run context fields to `internal/agent/prompt/prompt.go` (for example `SmithersActiveRuns` / `ActiveRuns`) and update prompt option wiring so template execution receives both workflow directory and run context.
4. Update Smithers template rendering in `internal/agent/templates/smithers.md.tpl`: add an `Active runs` context block guarded by conditionals so empty or unavailable data does not add noise.
5. Wire coordinator-owned context refresh with minimal risk: in `internal/agent/coordinator.go`, fetch active runs for Smithers mode when building/rebuilding the system prompt, filter to active statuses (`running`, `waiting-approval`, `waiting-event`), and degrade to workflow-dir-only context on any fetch error.
6. Ensure refresh timing satisfies acceptance criteria: rebuild Smithers system prompt when a new session starts (or equivalently before each run via existing `UpdateModels` path) so context is not stale across sessions.
7. Expand regression tests after wiring: add/adjust unit tests for client run decoding, prompt data rendering, Smithers prompt snapshot/golden output, and coordinator refresh behavior.
8. Add terminal E2E coverage modeled on upstream harness semantics from `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`: keep the same launch/poll/send/snapshot/terminate pattern in `internal/e2e`, with a Smithers-configured smoke scenario for this ticket.
9. Add one VHS happy-path recording in this repo for workspace-context flow (boot with Smithers config, send one run-status-oriented prompt, capture frame), then run full validation.

## File Plan
- `internal/smithers/types.go`
- `internal/smithers/client.go`
- `internal/smithers/client_test.go`
- `internal/agent/prompt/prompt.go`
- `internal/agent/prompt/prompt_test.go`
- `internal/agent/templates/smithers.md.tpl`
- `internal/agent/prompts_test.go`
- `internal/agent/testdata/smithers_prompt.golden`
- `internal/agent/coordinator.go`
- `internal/agent/coordinator_test.go`
- `internal/app/app.go` (only if coordinator constructor signature changes)
- `internal/e2e/tui_helpers_test.go`
- `internal/e2e/chat_workspace_context_test.go` (new)
- `tests/vhs/smithers-workspace-context.tape` (new)
- `tests/vhs/README.md`
- `tests/vhs/fixtures/crush.json` (only if fixture needs API URL/token fields for deterministic run-context capture)

## Validation
1. `gofumpt -w internal/smithers internal/agent internal/e2e`
2. `go test ./internal/smithers -run 'TestListRuns|TestRuns' -count=1 -v`
3. `go test ./internal/agent/prompt ./internal/agent -run 'TestPromptData_WithSmithersMode|TestSmithersPrompt' -count=1 -v`
4. `go test ./internal/agent -run TestCoordinatorResolveAgent -count=1 -v`
5. Terminal E2E path (modeled on upstream helper semantics: launch process, ANSI-stripped polling, `WaitForText`, `WaitForNoText`, `SendKeys`, snapshot-on-failure, terminate):
`SMITHERS_TUI_E2E=1 go test ./internal/e2e -run TestSmithersWorkspaceContext_TUI -count=1 -v`
6. VHS happy-path recording test in this repo:
`vhs tests/vhs/smithers-workspace-context.tape`
7. Full regression sweep:
`go test ./...`
8. Manual end-to-end check with local Smithers server:
`cd /Users/williamcory/smithers && bun run src/cli/index.ts serve --root . --port 7331`
`cd /Users/williamcory/crush && SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=$(mktemp -d) go run .`
Then ask: `What active runs do you already know about in this workspace?` and verify response references pre-fetched run context without requiring a tool call first.

## Open Questions
1. Upstream `GET /v1/runs` currently returns raw JSON payloads while existing Crush HTTP helpers assume `{ok,data}` envelopes. Should this ticket add a dual-decoder helper or a runs-specific HTTP path to avoid regression risk for existing methods?
2. Should prompt context refresh happen before every run (`UpdateModels` path) or only on session boundary events to reduce unnecessary Smithers API traffic?
3. For `ActiveRuns`, should we include only active statuses (`running`, `waiting-approval`, `waiting-event`) or include recent terminal runs with explicit truncation?
4. `../smithers/gui/src` and `../smithers/gui-ref` are not present in the current checkout (as of April 3, 2026). Is `../smithers/src` + `../smithers/tests` the authoritative reference set for this pass?
5. Existing VHS docs/tape use `CRUSH_GLOBAL_CONFIG` env names, while current code/tests use `SMITHERS_TUI_GLOBAL_CONFIG` and `SMITHERS_TUI_GLOBAL_DATA`. Should this plan include normalizing VHS docs/fixtures to current env names in this ticket?

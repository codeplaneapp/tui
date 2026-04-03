First research pass for `chat-active-run-summary` completed. No code changes were made.

## Existing Crush Surface
- Product and ticket intent are explicit: active run summary belongs in chat header/status, with dynamic updates: [PRD 6.1 and 6.2](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md:100), [Design chat mock with `3 active · 1 pending approval`](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md:89), [Engineering status enhancement sketch](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:525), [feature enum `CHAT_SMITHERS_ACTIVE_RUN_SUMMARY`](/Users/williamcory/crush/docs/smithers-tui/features.ts:36), [ticket acceptance criteria](/Users/williamcory/crush/.smithers/tickets/chat-active-run-summary.md:16).
- Current header still renders Crush branding in compact mode (`Charm™ CRUSH`): [header.go](/Users/williamcory/crush/internal/ui/model/header.go:43).
- Current header details only include LSP error count, token percentage, and `ctrl+d` hint; there is no Smithers run/approval segment: [header.go](/Users/williamcory/crush/internal/ui/model/header.go:119).
- Header rendering path does not accept Smithers summary data and is only called with session/layout inputs: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:2028).
- `Status.Draw` renders help plus transient messages only, so there is no alternate active-run summary path there either: [status.go](/Users/williamcory/crush/internal/ui/model/status.go:70).
- UI constructs Smithers client with no options (`smithers.NewClient()`), so configured `apiUrl`, `apiToken`, and `dbPath` are not used: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:332), [config smithers fields](/Users/williamcory/crush/internal/config/config.go:373).
- Smithers client currently has no run-summary surface (`ListRuns`, `CachedRunSummary`, pending-approval summary). It only has agents/SQL/scores/memory/cron, and agent listing is still stubbed placeholders: [client.go](/Users/williamcory/crush/internal/smithers/client.go:106), [types.go](/Users/williamcory/crush/internal/smithers/types.go:3).
- Smithers view routing exists but only `Agents` is wired from commands; no runs dashboard or summary state loop is connected: [commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go:527), [ui.go agents action](/Users/williamcory/crush/internal/ui/model/ui.go:1436), [agents view](/Users/williamcory/crush/internal/ui/views/agents.go:25).
- Keybinding conflict exists with design intent: `ctrl+r` is currently attachment-delete mode, while design expects `ctrl+r` for runs navigation: [keys.go](/Users/williamcory/crush/internal/ui/model/keys.go:136), [design shortcut row](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md:117).
- Engineering spec file for this ticket is malformed in-repo (contains only a self path), so ticket plus PRD/design/engineering docs are the usable planning sources: [chat-active-run-summary spec](/Users/williamcory/crush/.smithers/specs/engineering/chat-active-run-summary.md:1).

## Upstream Smithers Reference
- Top-line behavior is concrete in TUI v2: active runs are computed from run summaries with statuses `running`, `waiting-approval`, `waiting-timer`, then rendered as `X runs Y approvals`: [TopBar.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/TopBar.tsx:12).
- Upstream state flow refreshes runs, nodes, approvals, and events together during broker sync, then updates store maps used by the top bar: [Broker.syncNow](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts:594).
- Upstream service layer exposes `listRuns` and `listPendingApprovals` via DB adapter: [SmithersService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts:71).
- API surfaces that Crush can consume today are present: `GET /v1/runs`, `GET /v1/runs/:id`, and approvals list endpoint: [server/index.ts](/Users/williamcory/smithers/src/server/index.ts:943), [server/index.ts](/Users/williamcory/smithers/src/server/index.ts:972), [server/index.ts](/Users/williamcory/smithers/src/server/index.ts:1075).
- DB adapter exposes the same primitives (`listRuns`, `listAllPendingApprovals`): [adapter.ts](/Users/williamcory/smithers/src/db/adapter.ts:350), [adapter.ts](/Users/williamcory/smithers/src/db/adapter.ts:1411).
- Run status taxonomy is explicit upstream and should drive active-count semantics: [RunStatus.ts](/Users/williamcory/smithers/src/RunStatus.ts:1).
- Upstream terminal E2E model is available and directly portable in shape (`waitForText`, `sendKeys`, `snapshot`, `terminate`): [tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts:18), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts:10).
- Requested `../smithers/gui/src` and `../smithers/gui-ref` paths are missing in this local checkout, so authoritative behavior had to come from `src/cli/tui-v2` and `src/server`: `missing /Users/williamcory/smithers/gui/src`, `missing /Users/williamcory/smithers/gui-ref`.

## Gaps
- Data-model gap: Crush Smithers types have no run, run-summary, or approval-summary structs, so active counts cannot be represented in-process: [types.go](/Users/williamcory/crush/internal/smithers/types.go:3).
- Transport gap: UI client instantiation drops Smithers config, so even if run APIs were added, current UI path does not target configured API/DB: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:332), [config.go](/Users/williamcory/crush/internal/config/config.go:373).
- State-update gap: no polling or event loop updates a cached run summary in UI state; smithers view updates are per-view only: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:889).
- Rendering gap: header/status rendering has no Smithers run-summary hook and still prioritizes Crush details only: [header.go](/Users/williamcory/crush/internal/ui/model/header.go:107), [status.go](/Users/williamcory/crush/internal/ui/model/status.go:70).
- UX/navigation gap: design and engineering expect discoverable runs affordance (`ctrl+r`), but keymap currently assigns `ctrl+r` to attachment deletion: [keys.go](/Users/williamcory/crush/internal/ui/model/keys.go:136), [03-ENGINEERING key sketch](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:445).
- Validation gap: current E2E coverage verifies Smithers prompt boot but not dynamic run-summary rendering in header/status: [chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go:18).

## Recommended Direction
1. Add Smithers run summary domain types in `internal/smithers/types.go` (`Run`, `RunStatus`, `RunSummary`, `RunStatusSummary`).
2. Extend `internal/smithers/client.go` with `ListRuns`, `ListPendingApprovals`, and a cached `RunStatusSummary` refresh method using existing transport order (HTTP first, DB fallback, exec fallback where practical).
3. Initialize `smithersClient` in UI with config-derived options (`WithAPIURL`, `WithAPIToken`, `WithDBPath`) so summary queries use real transport configuration.
4. Add a lightweight periodic refresh command in `internal/ui/model/ui.go` while in chat/smithers states and plumb results into header rendering.
5. Implement `renderSmithersStatus()` in `internal/ui/model/header.go` and append it in header details when Smithers mode is active, matching ticket wording (`X active`) and upstream semantics for active statuses.
6. Decide keybinding resolution for `ctrl+r` conflict (`runs` vs attachment delete mode) as part of adjacent chat-helpbar work; this affects discoverability even if summary is header-only.
7. Testing plan:
- Terminal E2E: add a Go test in `internal/e2e` modeled on upstream harness flow from [smithers/tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts:18) and Crush helper API in [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go:42). Assert header initially and after run-state change.
- VHS happy path: add a new tape under `tests/vhs` (or extend current Smithers tape) that records startup and visible active-run summary text in chat header: [existing VHS pattern](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape:1).

## Files To Touch
- `/Users/williamcory/crush/internal/smithers/types.go`.
- `/Users/williamcory/crush/internal/smithers/client.go`.
- `/Users/williamcory/crush/internal/ui/model/ui.go`.
- `/Users/williamcory/crush/internal/ui/model/header.go`.
- `/Users/williamcory/crush/internal/ui/model/status.go` if product wants status-bar placement fallback in addition to header.
- `/Users/williamcory/crush/internal/ui/model/keys.go` and `/Users/williamcory/crush/internal/ui/dialog/commands.go` for shortcut/entry-point alignment.
- `/Users/williamcory/crush/internal/smithers/client_test.go` for transport and summary derivation tests.
- `/Users/williamcory/crush/internal/e2e/*` new active-run-summary E2E.
- `/Users/williamcory/crush/tests/vhs/*.tape` new or updated Smithers happy-path recording.

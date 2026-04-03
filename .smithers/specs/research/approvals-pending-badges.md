## Existing Crush Surface
- Scope and acceptance explicitly call for a global pending-approval indicator and dynamic updates: [01-PRD.md](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md:163), [02-DESIGN.md](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md:89), [03-ENGINEERING.md](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:525), [features.ts](/Users/williamcory/crush/docs/smithers-tui/features.ts:83), [ticket](/Users/williamcory/crush/.smithers/tickets/approvals-pending-badges.md:16), [engineering spec](/Users/williamcory/crush/.smithers/specs/engineering/approvals-pending-badges.md:5).
- Header rendering currently has no approvals state. It renders cwd, LSP error count, context percentage, and `ctrl+d` hints only: [header.go](/Users/williamcory/crush/internal/ui/model/header.go:107).
- Status bar is help + transient info message rendering only, with no badge path: [status.go](/Users/williamcory/crush/internal/ui/model/status.go:70).
- Header styles have no badge-specific style slots: [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go:75), [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go:1091).
- UI model has no pending-approval count field. Smithers client is created without config options: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:149), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:332).
- Smithers config fields exist (`dbPath`, `apiUrl`, `apiToken`) and defaults are set, but they are not wired into UI client construction: [config.go](/Users/williamcory/crush/internal/config/config.go:373), [load.go](/Users/williamcory/crush/internal/config/load.go:401).
- Approvals view exists but is fetch-on-enter plus manual `r` refresh; no live updates and no inline approve/deny actions: [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go:47), [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go:90), [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go:329).
- Navigation into approvals is via command dialog action, not badge affordance: [commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go:528), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go:1450).
- Smithers agent prompt asks to mention pending approvals, but this is assistant behavior text, not a UI data pipeline: [smithers.md.tpl](/Users/williamcory/crush/internal/agent/templates/smithers.md.tpl:35).
- Existing Crush terminal E2E harness already mirrors the upstream spawn/wait/send-keys pattern and can be extended: [tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go:42). Existing VHS setup is also present: [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md:1).

## Upstream Smithers Reference
- Note: requested `../smithers/*` paths are not present in this workspace; the available mirror is `/Users/williamcory/crush/smithers_tmp/*`.
- Approval DB schema is `(runId,nodeId,iteration,status,requestedAtMs,decidedAtMs,note,decidedBy)` with no `id/workflowPath/gate/payload`: [internal-schema.ts](/Users/williamcory/crush/smithers_tmp/src/db/internal-schema.ts:86).
- Pending approvals are queried per-run with status `requested`: [adapter.ts](/Users/williamcory/crush/smithers_tmp/src/db/adapter.ts:674).
- Approval request persistence uses `status: requested` and emits `ApprovalRequested` plus `NodeWaitingApproval`: [engine/index.ts](/Users/williamcory/crush/smithers_tmp/src/engine/index.ts:1150).
- Event domain includes `NodeWaitingApproval`, `ApprovalRequested`, `ApprovalGranted`, `ApprovalDenied`: [SmithersEvent.ts](/Users/williamcory/crush/smithers_tmp/src/SmithersEvent.ts:97).
- Server transport is per-run for events and approve/deny, and responses are plain JSON via `sendJson` (no `ok/data` envelope): [server/index.ts](/Users/williamcory/crush/smithers_tmp/src/server/index.ts:158), [server/index.ts](/Users/williamcory/crush/smithers_tmp/src/server/index.ts:765), [server/index.ts](/Users/williamcory/crush/smithers_tmp/src/server/index.ts:900).
- Single-run serve app has `GET /events`, `POST /approve/:nodeId`, `POST /deny/:nodeId`: [serve.ts](/Users/williamcory/crush/smithers_tmp/src/server/serve.ts:100), [serve.ts](/Users/williamcory/crush/smithers_tmp/src/server/serve.ts:160).
- Upstream CLI supports `approve <runId>` and `deny <runId>`; there is no `approval list` command in `src/cli/index.ts`: [index.ts](/Users/williamcory/crush/smithers_tmp/src/cli/index.ts:2032).
- TUI v2 top bar computes global approval counts from store state: [TopBar.tsx](/Users/williamcory/crush/smithers_tmp/src/cli/tui-v2/client/components/TopBar.tsx:12).
- TUI v2 also exposes approval-focused action UX (`A approve`, `D deny`) in the main shell: [TuiAppV2.tsx](/Users/williamcory/crush/smithers_tmp/src/cli/tui-v2/client/app/TuiAppV2.tsx:19).
- Broker currently updates approvals/runs/events via periodic sync polling: [Broker.ts](/Users/williamcory/crush/smithers_tmp/src/cli/tui-v2/broker/Broker.ts:615).
- GUI reference has explicit aggregate pending-count behavior (`pendingCount`, `pendingTarget`): [tray-status-service.ts](/Users/williamcory/crush/smithers_tmp/gui-ref/apps/daemon/src/services/tray-status-service.ts:39), [tray-status-service.test.ts](/Users/williamcory/crush/smithers_tmp/gui-ref/apps/daemon/src/services/tray-status-service.test.ts:71).
- Current `gui-src` shell does not expose a global pending badge in navigation/header: [App.tsx](/Users/williamcory/crush/smithers_tmp/gui-src/ui/App.tsx:19).
- Upstream terminal E2E harness pattern is spawn + waitForText + sendKeys + snapshot: [tui.e2e.test.ts](/Users/williamcory/crush/smithers_tmp/tests/tui.e2e.test.ts:18), [tui-helpers.ts](/Users/williamcory/crush/smithers_tmp/tests/tui-helpers.ts:10).

## Gaps
- Data model gap: Crush `Approval` expects `id/workflowPath/gate/payload/resolvedAt/resolvedBy` and status `pending|approved|denied`: [types.go](/Users/williamcory/crush/internal/smithers/types.go:83). Upstream stores per-run approvals with `requestedAtMs/decidedAtMs/note/decidedBy` and `status=requested` for pending: [internal-schema.ts](/Users/williamcory/crush/smithers_tmp/src/db/internal-schema.ts:86), [adapter.ts](/Users/williamcory/crush/smithers_tmp/src/db/adapter.ts:674).
- SQLite query gap: Crush queries columns `requested_at/resolved_at` and extra fields not in upstream schema, so DB fallback is incompatible: [client.go](/Users/williamcory/crush/internal/smithers/client.go:281).
- Transport gap: Crush HTTP client expects envelope `{ok,data}` and calls `/approval/list`: [client.go](/Users/williamcory/crush/internal/smithers/client.go:121), [client.go](/Users/williamcory/crush/internal/smithers/client.go:272). Upstream server returns plain JSON and does not expose `/approval/list`: [server/index.ts](/Users/williamcory/crush/smithers_tmp/src/server/index.ts:158), [server/index.ts](/Users/williamcory/crush/smithers_tmp/src/server/index.ts:956).
- Exec fallback gap: Crush executes `smithers approval list --format json`: [client.go](/Users/williamcory/crush/internal/smithers/client.go:291), but upstream CLI command surface is `approve`/`deny` and run-scoped inspection: [index.ts](/Users/williamcory/crush/smithers_tmp/src/cli/index.ts:2032).
- Rendering gap: header/status have no pending badge state, styling, or rendering path: [header.go](/Users/williamcory/crush/internal/ui/model/header.go:107), [status.go](/Users/williamcory/crush/internal/ui/model/status.go:70), [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go:75).
- UX gap: no live badge, no badge-click navigation, no event-driven update loop; approvals list is static-on-load/manual-refresh: [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go:47), [approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go:90).
- Testing gap: no approvals-specific Smithers client tests and no approvals badge E2E/VHS coverage in Crush today: [client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go:55), [chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go:18), [smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape:1).

## Recommended Direction
- Normalize Crush approval model to upstream semantics first. Treat pending as upstream `requested`, and drop assumptions about `workflowPath/gate/payload` unless separately fetched from inspect/run-node data.
- Replace `ListPendingApprovals` transport assumptions with run-scoped primitives aligned to upstream: list active/waiting runs, list per-run pending approvals, and optionally inspect for richer context.
- Add a Smithers event stream path for run events (`ApprovalRequested/Granted/Denied`, `NodeWaitingApproval`) with a polling fallback. Keep the UI counter source-of-truth in `UI` state.
- Add a compact header badge render path (and style token) gated on `pending_count > 0`, matching the docs/ticket requirements.
- Route badge interaction to open approvals view (or equivalent keyboard action) so the badge is actionable, not just decorative.
- Testing plan for this ticket should include both required paths:
- Terminal E2E path: extend existing Crush harness in `internal/e2e` to drive a run into waiting-approval and assert badge appears/disappears, modeled on upstream harness interactions.
- VHS happy path: add a new tape under `tests/vhs` that records badge visibility during a canonical approval flow.

## Files To Touch
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go)
- [internal/ui/model/status.go](/Users/williamcory/crush/internal/ui/model/status.go)
- [internal/ui/styles/styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go)
- [internal/ui/views/approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go)
- [internal/e2e/chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)
- [tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape)

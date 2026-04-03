## Existing Crush Surface
- Product/design/engineering inputs require an approvals queue, pending indicators, and `Ctrl+A` navigation: [01-PRD.md](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md#L163), [02-DESIGN.md](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md#L326), [02-DESIGN.md](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md#L867), [03-ENGINEERING.md](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md#L505), [features.ts](/Users/williamcory/crush/docs/smithers-tui/features.ts#L83).
- Ticket expects selectable queue + dynamic SSE updates: [approvals-queue.md](/Users/williamcory/crush/.smithers/tickets/approvals-queue.md#L16). The engineering spec file is empty: [approvals-queue.md](/Users/williamcory/crush/.smithers/specs/engineering/approvals-queue.md).
- Crush Smithers types/client currently have no approval model or approval APIs; `ListAgents` is still stubbed and HTTP decode assumes `{ok,data,error}` envelope: [types.go](/Users/williamcory/crush/internal/smithers/types.go#L1), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L108), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L121).
- Smithers UI surface in Crush is only router + Agents view: [router.go](/Users/williamcory/crush/internal/ui/views/router.go#L5), [agents.go](/Users/williamcory/crush/internal/ui/views/agents.go#L25), directory has no approvals view: [views](/Users/williamcory/crush/internal/ui/views).
- UI wires `smithers.NewClient()` with no config options, has only `ActionOpenAgentsView`, and command palette only exposes `agents`: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L332), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1436), [actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go#L88), [commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go#L527).
- No global `Ctrl+A` mapping for approvals; current keymap still uses `ctrl+r` for attachment delete mode: [keys.go](/Users/williamcory/crush/internal/ui/model/keys.go#L137).
- Config has Smithers transport knobs (`dbPath`, `apiUrl`, `apiToken`, `workflowDir`) but UI client init does not consume them: [config.go](/Users/williamcory/crush/internal/config/config.go#L373), [load.go](/Users/williamcory/crush/internal/config/load.go#L401), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L332).
- Chat rendering is generic MCP-only; no Smithers-specific approval renderer files exist: [mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go#L30), [chat dir](/Users/williamcory/crush/internal/ui/chat).
- Branding is still `CRUSH`/`Charm™` rather than Smithers-specific header treatment from PRD: [header.go](/Users/williamcory/crush/internal/ui/model/header.go#L43), [logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go#L38).
- Input handling risk relevant to list navigation: smithers view gets `tea.KeyPressMsg` in both `handleKeyPressMsg` and the later generic forward path (likely double-processing): [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L835), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L889), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1735).
- Crush has a terminal E2E harness and VHS baseline to extend: [tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go#L42), [chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go#L18), [smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape#L1).

## Upstream Smithers Reference
- Core server approvals/read/write/event surfaces are in `src/server/index.ts`:
  - pending approvals list: `/approval/list`, `/v1/approval/list`, `/approvals`: [index.ts](/Users/williamcory/smithers/src/server/index.ts#L972).
  - approve/deny node endpoints: [index.ts](/Users/williamcory/smithers/src/server/index.ts#L1019).
  - run SSE stream (`event: smithers`): [index.ts](/Users/williamcory/smithers/src/server/index.ts#L865).
  - responses are plain JSON via `sendJson`, not envelope-wrapped: [index.ts](/Users/williamcory/smithers/src/server/index.ts#L159).
- Core DB schema and adapter include approvals table + joined pending list query: [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L90), [adapter.ts](/Users/williamcory/smithers/src/db/adapter.ts#L1411).
- Approval-related event types are explicit in core event model: [SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L157).
- Current GUI stack (daemon/web) adds richer global queue APIs and UX:
  - daemon routes: `/api/approvals`, `/api/approval/list`, approve/deny routes: [approval-routes.ts](/Users/williamcory/smithers/apps/daemon/src/server/routes/approval-routes.ts#L12).
  - daemon service builds enriched `PendingApproval` (workspace/workflow/waiting metadata) and sorts by wait age: [approval-service.ts](/Users/williamcory/smithers/apps/daemon/src/services/approval-service.ts#L119).
  - daemon run-route event ingestion syncs approval state from streamed events: [run-routes.ts](/Users/williamcory/smithers/apps/daemon/src/server/routes/run-routes.ts#L145).
  - shared schemas define both `Approval` and richer `PendingApproval`: [approval.ts](/Users/williamcory/smithers/packages/shared/src/schemas/approval.ts#L6).
  - client SDK exposes `listPendingApprovals()`: [burns-client.ts](/Users/williamcory/smithers/packages/client/src/burns-client.ts#L516).
  - web inbox shows queue + detail panel + approve/deny + 2s auto-refresh + sidebar badge: [inbox/page.tsx](/Users/williamcory/smithers/apps/web/src/app/routes/inbox/page.tsx#L47), [use-pending-approvals.ts](/Users/williamcory/smithers/apps/web/src/features/approvals/hooks/use-pending-approvals.ts#L7), [app-shell.tsx](/Users/williamcory/smithers/apps/web/src/app/layouts/app-shell.tsx#L248).
- Upstream terminal harness pattern to model: [tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts#L18), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L10).
- Historical GUI-ref note: requested paths `/Users/williamcory/smithers/gui/src` and `/Users/williamcory/smithers/gui-ref` are absent in this checkout; legacy reference exists at [approval.ts](/Users/williamcory/crush/smithers_tmp/gui-ref/packages/shared/src/schemas/approval.ts#L5) and [inbox/page.tsx](/Users/williamcory/crush/smithers_tmp/gui-ref/apps/web/src/app/routes/inbox/page.tsx#L12).

## Gaps
- Data-model gap: Crush lacks approval DTOs entirely (`Approval`/`PendingApproval`) and approval event typing, while upstream has both core and daemon-level shapes.
- Transport gap: Crush has no `ListPendingApprovals`, `Approve`, `Deny`, or event-stream client path; existing HTTP decode contract is envelope-based and does not match upstream plain JSON responses.
- Integration gap: Smithers config exists but is not applied when constructing the UI client.
- Rendering gap: no `ApprovalsView` implementation/file, no approvals badge surface, and no Smithers-specific chat renderer for approval tools.
- UX/navigation gap: no `Ctrl+A` open-queue flow, no approvals command item/action, and no queue/detail interaction pattern.
- Input handling risk: smithers view keypresses appear double-routed in current UI update flow, which would likely break cursor navigation in approval lists.
- Test gap: no approvals queue terminal E2E and no approvals-specific VHS tape despite explicit engineering/ticket expectations.

## Recommended Direction
1. Normalize the approval contract first in `internal/smithers` with a queue-oriented type that can map both core `/approvals` payloads and daemon `/api/approvals` payloads.
2. Add Smithers transport primitives required by this ticket: `ListPendingApprovals`, `ApproveNode`, `DenyNode`, and run-event stream consumption (`/v1/runs/:runId/events`) with graceful fallback when streaming is unavailable.
3. Wire `internal/config` Smithers values (`APIURL`, `APIToken`, `DBPath`) into UI client construction.
4. Implement `ApprovalsView` with cursor navigation, refresh, and pending/recent sections; add command and keymap entry for `Ctrl+A` plus router action.
5. Fix smithers view message routing so keypresses are handled exactly once before building list-heavy views.
6. Tests for this ticket should include:
- terminal E2E in Crush modeled on upstream harness behavior (spawn, wait/poll, send keys, snapshot-on-failure, terminate).
- at least one VHS happy-path recording that opens approvals queue and shows populated rows.

## Files To Touch
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/smithers/events.go](/Users/williamcory/crush/internal/smithers/events.go) (new)
- [internal/smithers/events_test.go](/Users/williamcory/crush/internal/smithers/events_test.go) (new)
- [internal/ui/views/approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go) (new)
- [internal/ui/views/approvals_test.go](/Users/williamcory/crush/internal/ui/views/approvals_test.go) (new)
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/ui/model/keys.go](/Users/williamcory/crush/internal/ui/model/keys.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) (reuse)
- [internal/e2e/approvals_queue_test.go](/Users/williamcory/crush/internal/e2e/approvals_queue_test.go) (new)
- [tests/vhs/approvals-queue.tape](/Users/williamcory/crush/tests/vhs/approvals-queue.tape) (new)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)

```json
{
  "document": "## Existing Crush Surface\n- Approval queue requirements are documented in PRD/DESIGN/ENGINEERING and features inventory, but Crush currently only has an Agents Smithers view and no approvals view, no Ctrl+A navigation, and no approval client APIs.\n- Smithers client expects envelope responses (`ok/data`) and lacks approval/event methods; UI creates client without Smithers config options.\n- Testing surface exists (Go terminal E2E helper + VHS), but no approvals-specific E2E/VHS coverage exists.\n\n## Upstream Smithers Reference\n- Core server exposes approvals list (`/approval/list`, `/v1/approval/list`, `/approvals`), approve/deny endpoints, and run SSE (`/v1/runs/:runId/events`, `event: smithers`) with plain JSON responses.\n- Core DB has `_smithers_approvals`; adapter exposes `listAllPendingApprovalsEffect`.\n- Daemon/web layer adds global `/api/approvals`, richer `PendingApproval` schema, decision flows, sidebar badge, and inbox UX with 2s refresh.\n- Upstream terminal harness pattern is in `tests/tui.e2e.test.ts` + `tests/tui-helpers.ts`.\n\n## Gaps\n- Data-model: no Approval/PendingApproval/event types in Crush.\n- Transport: no list/approve/deny/stream methods; response-shape mismatch with upstream JSON.\n- Rendering/UX: no ApprovalsView, no approvals command/action, no Ctrl+A keybinding, no pending badge surfaces.\n- Integration: Smithers config not wired into UI client creation.\n- Risk: smithers view keypress may be double-routed in UI update flow.\n- Testing: no approvals E2E path and no approvals VHS happy-path tape.\n\n## Recommended Direction\n1. Add approval models + transport methods in `internal/smithers` and support upstream approval payloads.\n2. Wire Smithers config (`APIURL`, `APIToken`, `DBPath`) into `smithers.NewClient(...)`.\n3. Implement `internal/ui/views/approvals.go` with selectable pending list + recent decisions + refresh/back handling.\n4. Add navigation plumbing (`ActionOpenApprovalsView`, command palette item, `Ctrl+A`) and fix smithers-view keypress double-handling before list interactions.\n5. Add tests: client/view unit tests, terminal E2E modeled on upstream harness pattern, and one VHS approvals happy-path recording.\n\n## Files To Touch\n- internal/smithers/types.go\n- internal/smithers/client.go\n- internal/smithers/client_test.go\n- internal/smithers/events.go (new)\n- internal/smithers/events_test.go (new)\n- internal/ui/views/approvals.go (new)\n- internal/ui/views/approvals_test.go (new)\n- internal/ui/dialog/actions.go\n- internal/ui/dialog/commands.go\n- internal/ui/model/keys.go\n- internal/ui/model/ui.go\n- internal/e2e/approvals_queue_test.go (new, modeled on upstream `tui.e2e`/`tui-helpers`)\n- tests/vhs/approvals-queue.tape (new)\n"
}
```
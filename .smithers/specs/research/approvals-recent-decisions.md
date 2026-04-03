## Existing Crush Surface
- Planned target is clear in local docs/ticket: `docs/smithers-tui/features.ts:83-89` includes `APPROVALS_RECENT_DECISIONS`, design shows a `RECENT DECISIONS` block in approvals view (`docs/smithers-tui/02-DESIGN.md:326-367`), and the ticket/spec call for a toggle/section with decision + timestamp (`.smithers/tickets/approvals-recent-decisions.md:12-25`, `.smithers/specs/engineering/approvals-recent-decisions.md:20-37`).
- Crush currently has Smithers view scaffolding but no approvals view: `internal/ui/views/router.go:5-54` and `internal/ui/views/agents.go:25-160` are the only Smithers views in `internal/ui/views`.
- Smithers routing is wired only for agents: `internal/ui/model/ui.go:1436-1445` handles `ActionOpenAgentsView` and `views.PopViewMsg`; no approvals action route exists.
- Command palette exposes only `agents` for Smithers navigation: `internal/ui/dialog/actions.go:88-89` and `internal/ui/dialog/commands.go:526-529`.
- Smithers client/data model in Crush has no approval entities or recent-decision API methods: `internal/smithers/types.go:1-87` contains `Agent`, `SQLResult`, `ScoreRow`, `MemoryFact`, `CronSchedule`; `internal/smithers/client.go:106-117` has stub `ListAgents`, and implemented methods focus on SQL/scores/memory/cron (`internal/smithers/client.go:268`, `:309`, `:356`, `:402`).
- UI currently constructs Smithers client without Smithers config injection (`internal/ui/model/ui.go:331-333`), even though config supports `dbPath/apiUrl/apiToken/workflowDir` (`internal/config/config.go:373-377`).
- Chat rendering has no Smithers approvals-specific renderer: MCP tools fall through generic MCP renderer (`internal/ui/chat/tools.go:257-264`, `internal/ui/chat/mcp.go:34-93`).
- Smithers branding is partial: compact header still shows `Charm™ CRUSH` (`internal/ui/model/header.go:43-45`) and logo package is still Crush wordmark (`internal/ui/logo/logo.go:1-37`).
- Crush test scaffolding exists but does not cover approvals flows yet: terminal harness (`internal/e2e/tui_helpers_test.go:49-177`), minimal Smithers smoke tests (`internal/e2e/chat_domain_system_prompt_test.go:18-57`), and one VHS happy path (`tests/vhs/smithers-domain-system-prompt.tape:1-19`).

## Upstream Smithers Reference
- Path note: `../smithers/gui/src` and `../smithers/gui-ref` are not present in this workspace. Current GUI surfaces are in `../smithers/apps/web` and `../smithers/apps/daemon`.
- Core Smithers approval state includes decision metadata in `_smithers_approvals`: `status`, `requestedAtMs`, `decidedAtMs`, `note`, `decidedBy` (`../smithers/src/db/internal-schema.ts:90-105`).
- Engine writes approval decisions and emits decision events: approve/deny set `status` + `decidedAtMs` + `decidedBy` (`../smithers/src/engine/approvals.ts:28-37`, `:102-111`), and events include `ApprovalGranted`/`ApprovalDenied` (`../smithers/src/engine/approvals.ts:17-22`, `:91-96`).
- Workflow engine requests approvals with `status: requested` and `NodeWaitingApproval` (`../smithers/src/engine/index.ts:1779-1811`).
- Server exposes pending approvals and approve/deny transport, but pending list is pending-only: pending list route (`../smithers/src/server/index.ts:972-989`) delegates to `listAllPendingApprovalsEffect` (`../smithers/src/server/index.ts:462-494`), and approve/deny endpoints are `POST /v1/runs/:runId/nodes/:nodeId/approve|deny` (`../smithers/src/server/index.ts:1019-1073`).
- DB adapter confirms pending-only behavior: `listAllPendingApprovalsEffect` filters `status = requested` (`../smithers/src/db/adapter.ts:1411-1443`), and run-scoped pending query does the same (`../smithers/src/db/adapter.ts:1393-1404`).
- Current web/daemon approval model supports both pending and decided rows:
- Shared schema includes `decidedBy` and `decidedAt` (`../smithers/packages/shared/src/schemas/approval.ts:4-22`).
- Repository list sorts `pending` first, then `updated_at DESC` (so decided rows are part of feed) (`../smithers/apps/daemon/src/db/repositories/approval-repository.ts:118-123`).
- Daemon decision service persists decision fields and appends run events (`../smithers/apps/daemon/src/services/approval-service.ts:121-153`).
- Web run detail uses full approvals list per run and renders approval decision cards inline (`../smithers/apps/web/src/app/routes/workspace/runs/detail/page.tsx:332-364`, `:545-572`; `../smithers/apps/web/src/features/approvals/components/approval-decision-card.tsx:66-82`).
- Upstream terminal E2E harness pattern is explicit and reusable: launch/wait/send/snapshot helpers (`../smithers/tests/tui-helpers.ts:10-115`) and keyboard-driven flow in E2E (`../smithers/tests/tui.e2e.test.ts:18-47`).
- Handoff guide explicitly calls for Playwright-style terminal E2E + TDD (`../smithers/docs/guides/smithers-tui-v2-agent-handoff.md:25-35`).

## Gaps
- Data-model gap: Crush has no approval domain types (pending or decided) in `internal/smithers/types.go`, so there is no strongly typed recent-decision model.
- Transport gap: Crush client has no approvals list/recent-decision methods; current upstream core `/approvals` route is pending-only (`status=requested`), so there is no direct server API for recent decisions to consume.
- Rendering gap: Crush has no `internal/ui/views/approvals.go` at all, and no approvals decision renderer in chat/tool output.
- UX/navigation gap: no approvals command/keybinding/view route; only agents view is reachable from command palette.
- Integration gap: Crush UI constructs `smithers.NewClient()` with defaults and does not wire Smithers config (`apiUrl`, `apiToken`, `dbPath`) into the client.
- Branding/polish gap: top-level header/logo still says CRUSH in multiple UI entry points.
- Testing gap: Crush has foundational terminal E2E and VHS, but nothing that exercises approvals queue + recent decisions flows.

## Recommended Direction
- Keep this ticket scoped as a read-path feature on top of approvals-view scaffolding (`eng-approvals-view-scaffolding` dependency remains valid).
- Add approval data types in Crush (`PendingApproval`, `ApprovalDecision`) mapped to real upstream fields used today (`runId`, `nodeId`, `iteration`, `label/requestTitle`, `note/requestSummary`, `requestedAt/decidedAt`, `decidedBy`, `runStatus`).
- Add two client methods in `internal/smithers/client.go`:
- `ListPendingApprovals(ctx)` using upstream pending approvals route.
- `ListRecentApprovalDecisions(ctx, limit)` with transport fallback strategy.
- For recent decisions transport, prefer API-first if upstream adds endpoint; otherwise use read-only SQLite fallback query against `_smithers_approvals` where `status IN (approved, denied)` joined with `_smithers_runs` and `_smithers_nodes` for labels/workflow context.
- Implement `internal/ui/views/approvals.go` with `Tab`/toggle between pending and recent decisions, cursor navigation, refresh, and empty states aligned to design doc.
- Wire approvals navigation in command palette + UI action handling + short help.
- Add minimal approvals-specific rendering/styling semantics for approved/denied status (at least color + icon consistency with existing style system).
- Testing plan for this ticket:
- Terminal E2E in Crush using existing `internal/e2e` harness, but modeled after upstream `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts` keyboard flow style.
- Add one VHS happy-path tape for approvals (open approvals view, switch to recent decisions, verify an approved/denied row renders with timestamp).

## Files To Touch
- `internal/smithers/types.go`.
- `internal/smithers/client.go`.
- `internal/smithers/client_test.go`.
- `internal/ui/views/approvals.go` (new).
- `internal/ui/dialog/actions.go`.
- `internal/ui/dialog/commands.go`.
- `internal/ui/model/ui.go`.
- `internal/ui/model/keys.go`.
- `internal/ui/styles/styles.go`.
- `internal/e2e/*` (new approvals E2E test using existing harness).
- `tests/vhs/` (new approvals happy-path tape).
- Optional upstream dependency (if API-first recent decisions is chosen): `../smithers/src/server/index.ts` and `../smithers/src/db/adapter.ts` for a decided-approvals listing route.
## Existing Crush Surface
- `/Users/williamcory/crush/internal/ui/views/router.go`: defines the `View` interface plus `Router` (`Push`, `Pop`, `Current`, `HasViews`) and `PopViewMsg`. Router starts empty (`NewRouter()`), not chat-rooted.
- `/Users/williamcory/crush/internal/ui/model/ui.go`: `UI` owns `viewRouter *views.Router` and `smithersClient *smithers.Client`; `New()` initializes `views.NewRouter()` and `smithers.NewClient()`. It only pushes Agents via `dialog.ActionOpenAgentsView`, sets `uiSmithersView`, forwards update/draw to `viewRouter.Current()`, and pops on `views.PopViewMsg`.
- `/Users/williamcory/crush/internal/ui/views/agents.go`: currently the only concrete Smithers view. It renders `SMITHERS › Agents`, handles `esc` by returning `PopViewMsg`, and loads data from `client.ListAgents`.
- `/Users/williamcory/crush/internal/ui/model/keys.go`: no global approvals binding. `ctrl+r` is already used by `Editor.AttachmentDeleteMode`; `ctrl+a` is currently unbound globally.
- `/Users/williamcory/crush/internal/ui/dialog/actions.go` and `/Users/williamcory/crush/internal/ui/dialog/commands.go`: only Smithers navigation action/command today is `ActionOpenAgentsView` / `agents`.
- `/Users/williamcory/crush/internal/smithers/client.go`: Smithers client surface currently covers `ListAgents` (stub), SQL, scores, memory, and cron. There are no runs, approvals, or event-stream methods.
- `/Users/williamcory/crush/internal/smithers/types.go`: has `Agent`, `SQLResult`, `ScoreRow`, `MemoryFact`, `CronSchedule`; lacks `Run`, `Node`, and `Approval` types.
- `/Users/williamcory/crush/internal/agent/prompts.go` and `/Users/williamcory/crush/internal/agent/templates/`: only `coder/task/initialize` prompts are embedded; no Smithers-specific system prompt template.
- `/Users/williamcory/crush/internal/ui/model/header.go`, `/Users/williamcory/crush/internal/ui/logo/logo.go`, `/Users/williamcory/crush/internal/config/config.go`: still Crush-native branding/config namespace (`CRUSH`, `.crush`, `CRUSH.md` context).

## Upstream Smithers Reference
- `/Users/williamcory/smithers/src/engine/approvals.ts`: `approveNode` / `denyNode` persist decisions, emit `ApprovalGranted` / `ApprovalDenied`, and transition node state.
- `/Users/williamcory/smithers/src/db/adapter.ts`: approvals persistence/query methods exist (`insertOrUpdateApproval`, `getApproval`, `listPendingApprovals` where status is `requested`).
- `/Users/williamcory/smithers/src/SmithersEvent.ts`: event contract includes `NodeWaitingApproval`, `ApprovalRequested`, `ApprovalGranted`, and `ApprovalDenied`.
- `/Users/williamcory/smithers/src/server/index.ts`: transport surface includes run list/detail, run SSE (`GET /v1/runs/:id/events`), and approval mutations (`POST /v1/runs/:id/nodes/:nodeId/approve|deny`), plus cancel.
- `/Users/williamcory/smithers/src/server/serve.ts`: alternate run-scoped server also exposes `/approve/:nodeId` and `/deny/:nodeId`.
- `/Users/williamcory/smithers/src/cli/tui/components/RunsList.tsx`: legacy TUI behavior includes quick approval keys (`y` approve, `d` deny) when run status is `waiting-approval`.
- `/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts`: first-class approval model (`ApprovalSummary`), `approvals` state map, `approval-dialog` overlay, and workspace attention mode `approval`.
- `/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts`: service methods for `listPendingApprovals`, `approve`, and `deny`.
- `/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts`: sync loop joins runs, nodes, approvals, and events; includes `approveActiveApproval` and `jumpToLatestApproval`.
- `/Users/williamcory/smithers/src/cli/tui-v2/client/app/TuiAppV2.tsx`, `/client/components/TopBar.tsx`, `/client/components/WorkspaceRail.tsx`: approval action bar, approval dialog overlay, top-bar approval counts, and workspace attention markers.
- `/Users/williamcory/smithers/tests/tui.e2e.test.ts` and `/Users/williamcory/smithers/tests/tui-helpers.ts`: upstream terminal harness pattern (`launch`, ANSI-stripped buffer polling, `waitForText`, `sendKeys`, `snapshot`, `terminate`).
- `/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md`: calls for Playwright/TDD and progressive replacement of mock broker paths.
- Path availability note: `/Users/williamcory/smithers/gui/src` and `/Users/williamcory/smithers/gui-ref` are not present in this checkout. `/Users/williamcory/smithers/gui` exists but contains artifacts, not active source.

## Gaps
- Data-model gap: Crush has no approvals/run domain model in `internal/smithers/types.go`, while upstream TUI v2 treats approvals as first-class state (`shared/types.ts`).
- Transport gap: Crush client has no `ListRuns`, `GetRun`, `StreamEvents`, `ListPendingApprovals`, `Approve`, `Deny`, `Cancel` methods (`internal/smithers/client.go`), while upstream flow depends on these capabilities across server + DB + engine files.
- Rendering gap: Crush has no `/internal/ui/views/approvals.go`; only `agents.go` exists under Smithers views.
- Navigation gap: no approvals key/action/command plumbing in `keys.go`, `dialog/actions.go`, or `dialog/commands.go`.
- Router contract gap: current Crush implementation uses an empty stack + `uiSmithersView` state, while planning docs/ticket text describe a chat-rooted stack invariant (`docs/smithers-tui/03-ENGINEERING.md` section 3.1.1 and ticket scope).
- UX gap: no approvals badge/count in header, no approvals queue view, no inline approve/deny affordances, and no recent decisions section.
- Test gap: no router tests in `internal/ui/views`, no terminal E2E harness in Crush, and no VHS `.tape` tests currently in repo.
- Planning artifact gap: `/Users/williamcory/crush/.smithers/specs/engineering/eng-approvals-view-scaffolding.md` is currently stale/incomplete (single narrative line rather than an engineering spec).

## Recommended Direction
- For this ticket scope, follow the existing Crush Smithers-view path (minimal change): scaffold `ApprovalsView` like `AgentsView`, with static header/body and `esc` returning `PopViewMsg`.
- Add approvals entry points together so behavior is coherent:
  - add global `Approvals` keybinding (`ctrl+a`) in `internal/ui/model/keys.go`;
  - add `ActionOpenApprovalsView` in `internal/ui/dialog/actions.go`;
  - add `approvals` command item in `internal/ui/dialog/commands.go`.
- In `internal/ui/model/ui.go`, wire `ctrl+a` and command action to `viewRouter.Push(views.NewApprovalsView(...))`, set `uiSmithersView`, and keep back behavior through `PopViewMsg`.
- Keep router refactor (chat-rooted `IsChat` stack) out of this scaffolding ticket unless required; that change affects broader navigation semantics.
- Testing for this ticket should include both required paths:
  - terminal E2E in Go modeled directly on `/Users/williamcory/smithers/tests/tui.e2e.test.ts` + `/Users/williamcory/smithers/tests/tui-helpers.ts`;
  - at least one VHS happy-path tape for `ctrl+a -> approvals view -> esc back`.
- Follow-on approvals tickets should add Smithers client run/approval/event APIs and corresponding types before implementing live approval queue/actions.

## Files To Touch
- `/Users/williamcory/crush/internal/ui/views/approvals.go` (new).
- `/Users/williamcory/crush/internal/ui/model/keys.go`.
- `/Users/williamcory/crush/internal/ui/dialog/actions.go`.
- `/Users/williamcory/crush/internal/ui/dialog/commands.go`.
- `/Users/williamcory/crush/internal/ui/model/ui.go`.
- `/Users/williamcory/crush/internal/ui/views/router_test.go` (new).
- `/Users/williamcory/crush/tests/tui_helpers_test.go` (new E2E helper, modeled on upstream harness).
- `/Users/williamcory/crush/tests/approvals_view_e2e_test.go` (new).
- `/Users/williamcory/crush/tests/vhs/approvals-scaffolding.tape` (new).
- `/Users/williamcory/crush/.smithers/specs/engineering/eng-approvals-view-scaffolding.md` (replace stale content so planning input is actionable).

First research pass complete.

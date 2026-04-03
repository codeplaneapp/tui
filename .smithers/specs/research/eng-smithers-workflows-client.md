## Existing Crush Surface
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go#L57) has a 3-tier transport client, but implemented domains are SQL/scores/memory/cron plus stubbed agents, not workflows ([ListAgents stub](/Users/williamcory/crush/internal/smithers/client.go#L106), [ExecuteSQL](/Users/williamcory/crush/internal/smithers/client.go#L267), [GetScores](/Users/williamcory/crush/internal/smithers/client.go#L309), [ListMemoryFacts](/Users/williamcory/crush/internal/smithers/client.go#L355), [ListCrons](/Users/williamcory/crush/internal/smithers/client.go#L401)).
- HTTP decoding expects an envelope (`ok/data/error`) in [apiEnvelope](/Users/williamcory/crush/internal/smithers/client.go#L121), [httpGetJSON](/Users/williamcory/crush/internal/smithers/client.go#L173), and [httpPostJSON](/Users/williamcory/crush/internal/smithers/client.go#L202).
- Smithers types currently cover Agent/SQL/Score/Memory/Cron only in [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go#L3); workflow/run/approval/event structs needed for workflow UX are missing.
- Smithers UI state exists in [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L108), but only one concrete Smithers view is wired: [AgentsView](/Users/williamcory/crush/internal/ui/views/agents.go#L25) via [ActionOpenAgentsView](/Users/williamcory/crush/internal/ui/model/ui.go#L1436). The view stack itself is generic in [router.go](/Users/williamcory/crush/internal/ui/views/router.go#L17).
- Agent selection is not wired to handoff yet (`Enter` is no-op) in [agents.go](/Users/williamcory/crush/internal/ui/views/agents.go#L92).
- Command palette exposes only Smithers `agents` entry in [commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go#L527) and [actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go#L88).
- Smithers client is instantiated without config-derived API/DB options in [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L332), and Crush config has no `smithers` namespace in [config.go](/Users/williamcory/crush/internal/config/config.go#L373).
- MCP tool rendering is generic (`mcp_*`) in [chat/tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go#L260) and [chat/mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go#L34); no Smithers-specific renderers exist under [internal/ui/chat](/Users/williamcory/crush/internal/ui/chat).
- Tests in [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go#L53) cover SQL/scores/memory/cron/fallbacks only; no workflow client tests exist.
- The current ticket/spec assumptions are stale versus code and upstream server in [ticket](/Users/williamcory/crush/.smithers/tickets/eng-smithers-workflows-client.md#L16) and [engineering spec](/Users/williamcory/crush/.smithers/specs/engineering/eng-smithers-workflows-client.md#L20).

## Upstream Smithers Reference
- Current server API is run-centric in [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts#L544): create/resume/cancel/get/list runs, per-run SSE, frames, approve/deny, approvals list, health ([/v1/runs](/Users/williamcory/smithers/src/server/index.ts#L1027), [/v1/runs/:id/events](/Users/williamcory/smithers/src/server/index.ts#L817), [/approve](/Users/williamcory/smithers/src/server/index.ts#L971), [/deny](/Users/williamcory/smithers/src/server/index.ts#L999)).
- Server responses are plain JSON in [sendJson](/Users/williamcory/smithers/src/server/index.ts#L158), not the Crush envelope shape.
- Workflow discovery is filesystem/metadata-based in [src/cli/workflows.ts](/Users/williamcory/smithers/src/cli/workflows.ts#L47) with `DiscoveredWorkflow { id, displayName, entryFile, sourceType }`.
- Workflow operations are exposed via CLI in [src/cli/index.ts workflow commands](/Users/williamcory/smithers/src/cli/index.ts#L1112): `workflow list/path/create/doctor/run`, and `up` for execution ([up](/Users/williamcory/smithers/src/cli/index.ts#L1703)).
- Smithers TUI v2 data model is explicit in [src/cli/tui-v2/shared/types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts#L28) (`Workspace`, `FeedEntry`, `RunSummary`, `WorkflowRecord`, approvals, overlays).
- Smithers broker/service implementation shows practical behavior to mirror: discovery + DB polling + approvals + workflow launch in [SmithersService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts#L52), [Broker.ts sync/overlays/composer mentions](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts#L587), and [TuiAppV2 keyboard UX](/Users/williamcory/smithers/src/cli/tui-v2/client/app/TuiAppV2.tsx#L179).
- Legacy TUI still provides useful parity targets for run list/detail/workflow launch in [src/cli/tui/app.tsx](/Users/williamcory/smithers/src/cli/tui/app.tsx#L29), [RunsList.tsx](/Users/williamcory/smithers/src/cli/tui/components/RunsList.tsx#L31), [WorkflowLauncher.tsx](/Users/williamcory/smithers/src/cli/tui/components/WorkflowLauncher.tsx#L6), and [RunDetailView.tsx](/Users/williamcory/smithers/src/cli/tui/components/RunDetailView.tsx#L6).
- Required test harness reference is in [tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts#L18) and [tests/tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L10).
- Handoff guidance reinforces chat-first + terminal E2E/TDD in [smithers-tui-v2-agent-handoff.md](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md#L15).
- Planning inputs explicitly require workflow list/run/forms/inspection and harness + VHS coverage in [01-PRD §6.7](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md#L179), [01-PRD nav](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md#L316), [features.ts workflow inventory](/Users/williamcory/crush/docs/smithers-tui/features.ts#L101), and [03-ENGINEERING tests](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md#L939).
- `../smithers/gui/src` and `../smithers/gui-ref` are not present in this checkout (verified at `/Users/williamcory/smithers/gui/src` and `/Users/williamcory/smithers/gui-ref`).

## Gaps
- Data-model gap: Crush Smithers types do not model discovered workflows, run summaries, run nodes, approvals, or event frames required by the upstream run/workflow UX.
- Transport gap: Crush workflow assumptions in ticket/spec target `/api/workflows*`, but upstream server exposes `/v1/runs*` and plain JSON. Inference from [server routes](/Users/williamcory/smithers/src/server/index.ts#L544): workflow list/get metadata should come from CLI discovery (`workflow list/path/doctor`) or filesystem, not a non-existent workflows REST surface.
- Transport contract gap: Crush HTTP helper expects envelope responses while upstream server emits raw payloads.
- Coverage gap: Crush client has no methods/tests for workflow discovery/run, run list/get, per-run SSE event tailing, frames, or approval actions.
- Rendering gap: Crush has only Agents Smithers view; no workflows list/executor, no run inspector, no approvals queue, no workflow DAG/schema surfaces.
- UX gap: command palette and navigation do not expose `/workflows`-class paths or keyboard flow parity described in PRD/design.
- Interaction gap: agent handoff is not implemented (`Enter` no-op), and there is no workflow picker/composer mention flow like upstream broker.
- Testing gap: Crush has no terminal E2E harness equivalent to upstream `tui-helpers`, and no VHS-style happy-path terminal recording test.

## Recommended Direction
1. Re-baseline the ticket/spec to current upstream contracts before coding: treat workflow discovery as CLI/filesystem-driven and run execution as `/v1/runs` (HTTP) + CLI fallback.
2. Extend `internal/smithers` with workflow/run domain types and methods: `DiscoverWorkflows`, `ResolveWorkflow`, `RunWorkflow`, `ListRuns`, `GetRun`, `StreamRunEvents` (or poll), `ListRunFrames`, `ApproveNode`, `DenyNode`, `CancelRun`.
3. Update transport parsing in `internal/smithers/client.go` to accept current plain JSON responses; keep fallback order pragmatic (HTTP when available, DB read-only where valid, shell-out for mutating ops).
4. Add a first workflow UI slice in Crush TUI: a `workflows` view (list + launch), command palette action, and state wiring through existing `viewRouter`; keep this scoped before full run inspector/DAG implementation.
5. Add tests in two layers:
- Terminal E2E path modeled on upstream harness style (`launch`, `waitForText`, `sendKeys`) using new Crush-side harness files.
- At least one VHS-style happy-path recording test (open workflows, launch one, confirm run appears/returns run id).

## Files To Touch
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- New: `/Users/williamcory/crush/internal/ui/views/workflows.go`
- New (if tool result specialization is included in this slice): `/Users/williamcory/crush/internal/ui/chat/smithers_workflow*.go`
- New E2E harness files (modeled on upstream): `/Users/williamcory/crush/tests/tui.e2e.test.ts`, `/Users/williamcory/crush/tests/tui-helpers.ts`
- New VHS happy-path artifact and runner wiring: `/Users/williamcory/crush/tests/vhs/workflows-happy-path.tape` and [Taskfile.yaml](/Users/williamcory/crush/Taskfile.yaml)
- Update stale planning artifact for alignment: [eng-smithers-workflows-client spec](/Users/williamcory/crush/.smithers/specs/engineering/eng-smithers-workflows-client.md)

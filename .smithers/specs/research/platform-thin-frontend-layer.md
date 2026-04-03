First-pass research based on current code (not speculation).

## Existing Crush Surface
- Ticket intent is still scaffolding-level: [platform-thin-frontend-layer ticket](/Users/williamcory/crush/.smithers/tickets/platform-thin-frontend-layer.md#L12) asks for `internal/smithers` package + API URL/token/DB config + core run/event types.
- Current `internal/smithers` exists, but `types.go` only defines `Agent`, `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, `CronSchedule` ([types.go](/Users/williamcory/crush/internal/smithers/types.go#L6)); no `Run`/`Node`/`Attempt`/`Event`/`Approval` structs.
- `Client` supports API URL/token/DB path options and transport helpers ([client.go](/Users/williamcory/crush/internal/smithers/client.go#L32), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L57)), but exposed methods are SQL/scores/memory/cron + stubbed agents ([client.go](/Users/williamcory/crush/internal/smithers/client.go#L108), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L266), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L310), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L356), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L402)).
- Smithers config namespace exists in Crush config ([config.go](/Users/williamcory/crush/internal/config/config.go#L373), [load.go](/Users/williamcory/crush/internal/config/load.go#L401)), and Smithers agent prompt wiring exists ([coordinator.go](/Users/williamcory/crush/internal/agent/coordinator.go#L124), [prompts.go](/Users/williamcory/crush/internal/agent/prompts.go#L20)).
- UI integration is minimal: one Smithers state + router/client in UI model ([ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L108), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L331)), and only one view implemented (`AgentsView`) ([router.go](/Users/williamcory/crush/internal/ui/views/router.go#L5), [agents.go](/Users/williamcory/crush/internal/ui/views/agents.go#L25), [internal/ui/views](/Users/williamcory/crush/internal/ui/views)).
- Command palette currently adds only `Agents` Smithers navigation ([commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go#L527), [actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go#L88), [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1436)).
- Chat rendering for MCP is generic `mcp_*` formatting, not Smithers-domain renderers ([tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go#L260), [mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go#L34)); styles are generic MCP styles ([styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go#L320), [styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go#L1177)).
- Crush already has a terminal E2E harness and one VHS tape, but both are smoke-level ([tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go#L42), [chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go#L18), [smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape#L1)).

## Upstream Smithers Reference
- Inspected required upstream paths. Present: `../smithers/src`, `../smithers/src/server`, tests, handoff doc. Missing in this checkout: `../smithers/gui/src` and `../smithers/gui-ref`.
- Current server contract is run-centric REST + SSE (`/v1/runs`, `/v1/runs/:id`, `/v1/runs/:id/events`, `/v1/runs/:id/frames`, approvals endpoints) ([index.ts](/Users/williamcory/smithers/src/server/index.ts#L546), [index.ts](/Users/williamcory/smithers/src/server/index.ts#L820), [index.ts](/Users/williamcory/smithers/src/server/index.ts#L945), [index.ts](/Users/williamcory/smithers/src/server/index.ts#L973), [index.ts](/Users/williamcory/smithers/src/server/index.ts#L1029)).
- SSE payload format is explicit `event: smithers` with event JSON ([index.ts](/Users/williamcory/smithers/src/server/index.ts#L857)); run-scoped Hono serve app exposes same shape (`/events`, `/frames`, `/approve/:nodeId`, `/deny/:nodeId`, `/cancel`) ([serve.ts](/Users/williamcory/smithers/src/server/serve.ts#L100), [serve.ts](/Users/williamcory/smithers/src/server/serve.ts#L140), [serve.ts](/Users/williamcory/smithers/src/server/serve.ts#L160), [serve.ts](/Users/williamcory/smithers/src/server/serve.ts#L175), [serve.ts](/Users/williamcory/smithers/src/server/serve.ts#L190)).
- Upstream DB model includes `_smithers_runs`, `_smithers_nodes`, `_smithers_attempts`, `_smithers_approvals`, `_smithers_events`, `_smithers_cron` ([internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L9), [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L33), [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L50), [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L90), [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L143), [internal-schema.ts](/Users/williamcory/smithers/src/db/internal-schema.ts#L191)).
- Scorer table name upstream is `_smithers_scorers` ([schema.ts](/Users/williamcory/smithers/src/scorers/schema.ts#L12)).
- Event model is rich and domain-wide (run/node/approval/tool/time-travel/memory/voice/openapi) ([SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L4), [SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L142), [SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L163), [SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L292), [SmithersEvent.ts](/Users/williamcory/smithers/src/SmithersEvent.ts#L360)).
- Shared DTO schemas used by current web/client are in `packages/shared` and `packages/client` ([run.ts](/Users/williamcory/smithers/packages/shared/src/schemas/run.ts#L3), [event.ts](/Users/williamcory/smithers/packages/shared/src/schemas/event.ts#L3), [approval.ts](/Users/williamcory/smithers/packages/shared/src/schemas/approval.ts#L6), [burns-client.ts](/Users/williamcory/smithers/packages/client/src/burns-client.ts#L443), [burns-client.ts](/Users/williamcory/smithers/packages/client/src/burns-client.ts#L488), [burns-client.ts](/Users/williamcory/smithers/packages/client/src/burns-client.ts#L511)).
- Upstream TUI v2 app model/broker has broader workspace-feed-run abstractions ([types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts#L28), [types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts#L135), [Broker.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts#L179), [SmithersService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts#L71)).
- Upstream terminal E2E harness pattern is keyboard-driven launch/wait/send/snapshot ([tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts#L18), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L10), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L57), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L75)); handoff doc explicitly calls for Playwright/TDD around v2 brokered flow ([smithers-tui-v2-agent-handoff.md](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md#L29)).

## Gaps
- Data model gap: Crush `internal/smithers/types.go` does not yet define core run graph/event/approval/workflow structures targeted by the ticket and engineering docs.
- Transport contract gap: Crush client assumes `/sql` and `/cron/*` HTTP routes ([client.go](/Users/williamcory/crush/internal/smithers/client.go#L274), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L406), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L437), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L458)), while current upstream server file only exposes run/approval routes in `src/server/index.ts`.
- DB fallback gap: client SQL queries use `_smithers_scorer_results` and `_smithers_crons` ([client.go](/Users/williamcory/crush/internal/smithers/client.go#L316), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L416)), but upstream schema uses `_smithers_scorers` and `_smithers_cron`.
- CLI fallback gap: Crush fallback expects commands/flags not aligned with current CLI surface: `smithers sql` (no such command), `cron toggle` (not present), memory commands requiring workflow context (upstream requires `--workflow`) ([client.go](/Users/williamcory/crush/internal/smithers/client.go#L290), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L466), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L370), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L384), [index.ts](/Users/williamcory/smithers/src/cli/index.ts#L1254), [index.ts](/Users/williamcory/smithers/src/cli/index.ts#L1451), [index.ts](/Users/williamcory/smithers/src/cli/index.ts#L3023)).
- Config-to-client gap: `smithers` config fields exist but UI constructs `smithers.NewClient()` without `WithAPIURL/WithAPIToken/WithDBPath` wiring ([ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L332)).
- Rendering gap: Smithers MCP outputs are still rendered by generic MCP tool renderer; no Smithers-specific run/approval/workflow cards from design targets.
- UX/navigation gap: current Smithers frontend surface is single `Agents` view; docs/features call for workspace+systems IA, runs/approvals/workflows/sql/triggers/timeline/live chat, and richer keybindings.
- Testing gap: existing Crush terminal E2E and VHS are smoke checks, not Smithers navigation/data-path E2E modeled on upstream runs-flow harness plus Smithers happy-path recording.

## Recommended Direction
- Lock the client contract to current upstream sources first: `src/server/index.ts` + `src/db/internal-schema.ts` + `packages/shared/src/schemas/*`. Treat these as canonical over older GUI-route assumptions.
- Expand `internal/smithers/types.go` to include run/node/attempt/event/approval/workflow DTOs, then refactor `client.go` around those types and explicit domain methods (`ListRuns`, `GetRun`, `StreamRunEvents`, `ListApprovals`, `ApproveNode`, `DenyNode`, etc.).
- Fix transport priority to match product docs but with real contracts: HTTP first (`/v1/runs*`), SQLite read fallback for list/read paths using actual table names, CLI fallback using existing command signatures/output shapes.
- Wire `config.Smithers` into UI client construction so transport settings are used in runtime, not only defined in config schema.
- Keep frontend thin by adding only presentation/routing in `internal/ui/views/*`; avoid embedding Smithers business rules in Go.
- Test plan for this ticket family:
- Terminal E2E: extend existing Crush `internal/e2e` harness (already similar to upstream `tui-helpers.ts`) with a Smithers flow that launches UI, navigates to a Smithers view, exercises keyboard actions, and snapshots on failure.
- VHS: add at least one new Smithers happy-path tape beyond prompt smoke (for example: open runs view -> inspect -> back), consistent with `tests/vhs` conventions.

## Files To Touch
- Core client/types:
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [internal/smithers/events.go](/Users/williamcory/crush/internal/smithers/events.go) (new, SSE consumer)
- UI wiring/routing/views:
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/views/router.go](/Users/williamcory/crush/internal/ui/views/router.go)
- [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go) (adjust after client contract changes)
- [internal/ui/views/runs.go](/Users/williamcory/crush/internal/ui/views/runs.go) (new)
- [internal/ui/views/approvals.go](/Users/williamcory/crush/internal/ui/views/approvals.go) (new)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- Chat rendering:
- [internal/ui/chat/tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go)
- [internal/ui/chat/mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go)
- [internal/ui/chat/smithers_runs.go](/Users/williamcory/crush/internal/ui/chat/smithers_runs.go) (new)
- [internal/ui/chat/smithers_approvals.go](/Users/williamcory/crush/internal/ui/chat/smithers_approvals.go) (new)
- Config/app integration:
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go) (if additional smithers transport knobs are needed)
- [internal/app/app.go](/Users/williamcory/crush/internal/app/app.go) (if promoting client initialization out of UI layer)
- Tests:
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go)
- [internal/e2e/chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go)
- [internal/e2e/smithers_views_e2e_test.go](/Users/williamcory/crush/internal/e2e/smithers_views_e2e_test.go) (new)
- [tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape)
- [tests/vhs/smithers-runs-happy-path.tape](/Users/williamcory/crush/tests/vhs/smithers-runs-happy-path.tape) (new)

```json
{
  "document": "First research pass completed."
}
```
## Existing Crush Surface
- The ticket asks for `ListTickets`, `CreateTicket`, and `UpdateTicket`, cites GUI transport, and says to add methods under `internal/app/smithers`: [ticket summary](/Users/williamcory/crush/.smithers/tickets/eng-tickets-api-client.md:12), [source context](/Users/williamcory/crush/.smithers/tickets/eng-tickets-api-client.md:22), [impl note](/Users/williamcory/crush/.smithers/tickets/eng-tickets-api-client.md:27).
- Crush has no `internal/app/smithers` package; the client is in [internal/smithers](/Users/williamcory/crush/internal/smithers).
- The current client expects an HTTP envelope `ok/data/error` in [client.go](/Users/williamcory/crush/internal/smithers/client.go#L121) and [client.go](/Users/williamcory/crush/internal/smithers/client.go#L189).
- Implemented methods are SQL, scores, memory, and cron only: [client.go](/Users/williamcory/crush/internal/smithers/client.go#L266), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L309), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L355), [client.go](/Users/williamcory/crush/internal/smithers/client.go#L401). No ticket methods are present.
- Domain types contain `Agent`, `SQLResult`, `ScoreRow`, `MemoryFact`, `CronSchedule` and no ticket type: [types.go](/Users/williamcory/crush/internal/smithers/types.go:1).
- UI Smithers routing currently exposes only Agents: [agents view](/Users/williamcory/crush/internal/ui/views/agents.go:37), [Agents command](/Users/williamcory/crush/internal/ui/dialog/commands.go:527), [Agents action](/Users/williamcory/crush/internal/ui/dialog/actions.go:88), [handler](/Users/williamcory/crush/internal/ui/model/ui.go#L1436).
- The Smithers client is instantiated with defaults and no transport wiring in UI state: [ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L332).
- Chat tool rendering is generic for `mcp_*`; there is no ticket-specific renderer path: [tool routing](/Users/williamcory/crush/internal/ui/chat/tools.go#L260), [generic MCP renderer](/Users/williamcory/crush/internal/ui/chat/mcp.go:34).

## Upstream Smithers Reference
- Requested GUI reference locations are missing in this checkout: [gui/src](/Users/williamcory/smithers/gui/src) and [gui-ref](/Users/williamcory/smithers/gui-ref).
- Current server transport is run-centric and uses raw JSON payloads via `sendJson`, not envelope-wrapped responses: [server sendJson](/Users/williamcory/smithers/src/server/index.ts:158), [POST /v1/runs](/Users/williamcory/smithers/src/server/index.ts:546), [GET /v1/runs](/Users/williamcory/smithers/src/server/index.ts:1029), [404 fallback](/Users/williamcory/smithers/src/server/index.ts:1044).
- Run-scoped Hono server also exposes run/approval/frames routes, not tickets: [serve app](/Users/williamcory/smithers/src/server/serve.ts:43).
- CLI registry has no `ticket` command family: [command registry](/Users/williamcory/smithers/src/cli/index.ts:3197).
- Cron CLI currently supports `start/add/list/rm` and no `toggle` subcommand: [cron CLI](/Users/williamcory/smithers/src/cli/index.ts:1435).
- TUI-v2 broker and shared types center on runs/workflows/approvals/agents and do not define ticket operations/models: [SmithersService](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts:71), [shared types](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts:76).
- Upstream terminal E2E harness pattern is explicit and reusable: [tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts:18), [tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts:10), [handoff testing guidance](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md:29).

## Gaps
1. Data-model gap: no `Ticket` type in Crush Smithers client types, and no ticket model in upstream TUI-v2 shared types.
2. Transport gap: no authoritative ticket HTTP/CLI surface is present in inspected upstream server/CLI files; meanwhile Crush client assumptions are tied to envelope + legacy-style endpoints.
3. Rendering gap: Crush has no Tickets view, no ticket command/action, and no ticket-focused renderer.
4. UX gap: PRD/Design require `/tickets` list/detail/create/edit split-pane behavior, but current Crush Smithers surface only exposes Agents: [PRD ticket manager](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md:203), [Design ticket manager](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md:487).
5. Testing gap: Crush docs require terminal E2E modeled on upstream harness plus VHS happy-path recording, but there is no equivalent Crush harness or tape yet: [engineering E2E requirement](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:941), [engineering VHS requirement](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md:946).

## Recommended Direction
- First-pass blocker callout: re-baseline the ticket transport contract before implementation, because the ticket points to missing GUI files and current upstream code does not expose ticket endpoints/commands.
- Define the canonical contract in current Smithers surfaces (`smithers/src` and `smithers/src/server`) for ticket list/create/update payloads and error shape.
- Implement `ListTickets`, `CreateTicket`, `UpdateTicket` in Crush `internal/smithers` (not `internal/app/smithers`) with clear fallback order and explicit unsupported-transport errors.
- Add ticket domain types and focused unit tests in the existing Smithers client test suite.
- Add Tickets navigation/view wiring in Crush UI to match PRD/Design split-pane behavior.
- Add a terminal E2E path in Crush modeled on upstream `waitForText` + `sendKeys` harness semantics.
- Add at least one VHS-style happy-path recording that covers tickets list -> open -> edit/save -> refresh.

## Files To Touch
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)
- [internal/smithers/client_test.go](/Users/williamcory/crush/internal/smithers/client_test.go)
- [/Users/williamcory/crush/internal/ui/views/tickets.go](/Users/williamcory/crush/internal/ui/views/tickets.go) new
- [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [/Users/williamcory/crush/internal/ui/chat/mcp_ticket.go](/Users/williamcory/crush/internal/ui/chat/mcp_ticket.go) new if ticket-specific renderer is required
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go) for Smithers transport settings wiring
- [/Users/williamcory/crush/tests/tui.e2e.test.ts](/Users/williamcory/crush/tests/tui.e2e.test.ts) new terminal E2E path
- [/Users/williamcory/crush/tests/tui-helpers.ts](/Users/williamcory/crush/tests/tui-helpers.ts) new harness helper modeled on upstream
- [/Users/williamcory/crush/tests/vhs/tickets-happy-path.tape](/Users/williamcory/crush/tests/vhs/tickets-happy-path.tape) new VHS recording test
- [feature inventory ticket capabilities](/Users/williamcory/crush/docs/smithers-tui/features.ts:117) and [HTTP client capability](/Users/williamcory/crush/docs/smithers-tui/features.ts:19) to keep acceptance traceable in implementation notes.
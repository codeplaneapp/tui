First research pass complete. No code changes were made.

## Existing Crush Surface
- [Ticket scope](/Users/williamcory/crush/.smithers/tickets/chat-specialized-agent.md) and [engineering spec](/Users/williamcory/crush/.smithers/specs/engineering/chat-specialized-agent.md) define this as a Smithers-specialized chat mode effort.
- [Smithers PRD](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md), [design](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md), [engineering](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md), and [feature inventory](/Users/williamcory/crush/docs/smithers-tui/features.ts) set the target behavior.
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go) `SetupAgents` already defines `AgentSmithers`, restricts tools, and limits MCP access to server `smithers`.
- [internal/config/load.go](/Users/williamcory/crush/internal/config/load.go) `setDefaults` sets Smithers paths and MCP map initialization, but does not auto-register a default Smithers MCP server entry (`smithers mcp-serve`).
- [internal/agent/coordinator.go](/Users/williamcory/crush/internal/agent/coordinator.go) prefers Smithers agent when configured and enforces tool/MCP filtering.
- [internal/agent/prompts.go](/Users/williamcory/crush/internal/agent/prompts.go) and [internal/agent/templates/smithers.md.tpl](/Users/williamcory/crush/internal/agent/templates/smithers.md.tpl) provide Smithers-specific prompt behavior.
- [internal/app/app.go](/Users/williamcory/crush/internal/app/app.go) initializes MCP from configured servers only.
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) has `uiSmithersView`, but current command routing is minimal via [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go).
- [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go) and [internal/ui/views/tickets.go](/Users/williamcory/crush/internal/ui/views/tickets.go) are basic list views, not run/feed/approval workflows.
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go) and [internal/ui/logo/logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go) still present Crush branding in key chat UI surfaces.
- [internal/ui/chat/tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go) and [internal/ui/chat/mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go) render Smithers MCP output generically.
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go) has useful Smithers endpoints, but `ListAgents` is placeholder/stub and UI construction in [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) does not inject full Smithers config.
- E2E and VHS exist but are thin/stale: [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go), [internal/e2e/chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go), and [tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape).

## Upstream Smithers Reference
- State model is explicit and run-centric in [shared/types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts) and [store.ts](/Users/williamcory/smithers/src/cli/tui-v2/client/state/store.ts).
- Broker/service layer drives transport + UX orchestration in [Broker.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts), [SmithersService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts), and [FeedService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/FeedService.ts).
- UI composition is Smithers-specific in [TuiAppV2.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/app/TuiAppV2.tsx), [TopBar.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/TopBar.tsx), [WorkspaceRail.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/WorkspaceRail.tsx), [Feed.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/Feed.tsx), [Inspector.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/Inspector.tsx), and [Composer.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/Composer.tsx).
- Server contract for runs/events/actions is in [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts).
- Terminal test harness pattern is in [tests/tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts) and [tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts).
- Handoff guidance is in [smithers-tui-v2-agent-handoff.md](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md).
- `../smithers/gui/src` and `../smithers/gui-ref` are not present in this checkout; current implementation evidence is under `../smithers/src/cli/tui-v2` (and legacy `../smithers/src/cli/tui`).

## Gaps
- Data-model gap: Crush UI remains session/chat-oriented while upstream Smithers uses first-class run/feed/approval/workspace state.
- Transport gap: Crush has prompt/tool specialization but lacks default Smithers MCP bootstrap and full Smithers client config injection path in UI startup.
- Rendering gap: Smithers MCP responses in Crush are generic JSON/markdown rendering instead of typed run/feed/inspector presentation.
- UX gap: Crush Smithers mode currently exposes only lightweight Agents/Tickets surfaces, without upstream-style focus cycling, pane orchestration, run control, or approval flows.
- Branding gap: Smithers mode still carries Crush-oriented title/logo surfaces.
- Testing gap: Crush lacks upstream-style interactive TUI E2E scenarios and has VHS env var drift versus current config loader behavior.

## Recommended Direction
1. Wire transport defaults first: when Smithers config is present, auto-seed MCP server `smithers` with `command: smithers` and `args: [mcp-serve]`, and pass Smithers API/DB/token config into UI client construction.
2. Add a Smithers-focused UI state slice in Crush (runs, feed entries, selected run/node, approvals, focus target) modeled after upstream types/store.
3. Introduce Smithers-specific render items for run/feed/approval events, reusing existing chat item plumbing but replacing generic MCP output for Smithers tools.
4. Expand Smithers UX: command-palette actions, focus shortcuts, run selection/control, approval actions, and Smithers-aligned header/help branding.
5. Testing: add terminal E2E flows modeled on upstream harness patterns ([tests/tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts), [tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts)) and add at least one VHS happy-path recording for Smithers TUI interaction in Crush.

## Files To Touch
- [internal/config/load.go](/Users/williamcory/crush/internal/config/load.go)
- [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go)
- [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/model/keys.go](/Users/williamcory/crush/internal/ui/model/keys.go)
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go)
- [internal/ui/logo/logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go)
- [internal/ui/chat/tools.go](/Users/williamcory/crush/internal/ui/chat/tools.go)
- [internal/ui/chat/mcp.go](/Users/williamcory/crush/internal/ui/chat/mcp.go)
- [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go)
- [internal/ui/views/tickets.go](/Users/williamcory/crush/internal/ui/views/tickets.go)
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go)
- [internal/e2e/chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go)
- [tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape)

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "document": {
      "type": "string"
    }
  },
  "required": [
    "document"
  ],
  "additionalProperties": {}
}
```
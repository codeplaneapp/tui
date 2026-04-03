## Existing Crush Surface
- Ticket intent is defined in [chat-mcp-connection-status.md](/Users/williamcory/crush/.smithers/tickets/chat-mcp-connection-status.md): show Smithers MCP connected/disconnected in chat header/welcome and update dynamically.
- Product/design/engineering inputs all call for chat-level status visibility: [01-PRD.md](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md), [02-DESIGN.md](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md), [03-ENGINEERING.md](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md), [features.ts](/Users/williamcory/crush/docs/smithers-tui/features.ts) (`CHAT_SMITHERS_MCP_CONNECTION_STATUS`).
- MCP runtime state already exists in Crush: [internal/agent/tools/mcp/init.go](/Users/williamcory/crush/internal/agent/tools/mcp/init.go) defines `State{disabled,starting,connected,error}`, stores per-server `ClientInfo`, and publishes `EventStateChanged`.
- Transport to UI is already wired: [internal/app/app.go](/Users/williamcory/crush/internal/app/app.go) subscribes MCP events; [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) receives `pubsub.Event[mcp.Event]`, calls `handleStateChanged`, and refreshes `mcpStates`.
- Current MCP rendering is section-based (landing/sidebar/details), not chat-header Smithers-specific: [internal/ui/model/mcp.go](/Users/williamcory/crush/internal/ui/model/mcp.go), [internal/ui/model/landing.go](/Users/williamcory/crush/internal/ui/model/landing.go), [internal/ui/model/sidebar.go](/Users/williamcory/crush/internal/ui/model/sidebar.go), [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go).
- Compact header currently renders cwd + LSP/context/help hint only; no MCP status line: [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go).
- Reusable online/offline dot styles already exist: [internal/ui/styles/styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go).
- Smithers agent prompt path hardcodes MCP server name `smithers` today: [internal/agent/coordinator.go](/Users/williamcory/crush/internal/agent/coordinator.go), [internal/agent/templates/smithers.md.tpl](/Users/williamcory/crush/internal/agent/templates/smithers.md.tpl).

## Upstream Smithers Reference
- Terminal E2E harness pattern is spawn + buffered output + polling `waitForText`: [tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts), [tests/tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts).
- TUI v2 top bar does not include MCP connection state: [src/cli/tui-v2/client/components/TopBar.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/components/TopBar.tsx).
- TUI v2 app state has no MCP connection field: [src/cli/tui-v2/shared/types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts).
- TUI v2 broker/service are DB/API/run focused; no MCP connection probe surfaced to UI: [src/cli/tui-v2/broker/Broker.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts), [src/cli/tui-v2/broker/SmithersService.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/SmithersService.ts).
- Server health endpoints exist, but no MCP connection contract for UI: [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts), [src/server/serve.ts](/Users/williamcory/smithers/src/server/serve.ts).
- Smithers CLI `ask` flow uses local MCP server name `smithers-orchestrator`, highlighting naming variability: [src/cli/ask.ts](/Users/williamcory/smithers/src/cli/ask.ts).
- Requested GUI reference paths are not present in this checkout: [gui](/Users/williamcory/smithers/gui) has no `src/`, and `/Users/williamcory/smithers/gui-ref` does not exist.

## Gaps
- Data model gap: Crush has generic `mcpStates`, but no Smithers-focused projection for chat header/welcome (connected/disconnected for the Smithers MCP specifically).
- Transport/naming gap: Ticket references `internal/mcp/client.go`, but runtime MCP code is at [internal/agent/tools/mcp/init.go](/Users/williamcory/crush/internal/agent/tools/mcp/init.go). Also, server-name assumptions vary (`smithers` vs `smithers-orchestrator`).
- Rendering gap: Header path in [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go) has no MCP indicator.
- UX gap: MCP state is visible in side/details sections, not in the chat-header/welcome location shown by design docs.
- Testing gap: Crush has VHS smoke only ([tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape)); it lacks a terminal E2E harness equivalent to Smithers `tui-helpers.ts` for polling/asserting live TUI text.
- Planning-input gap: Engineering spec file is effectively empty/self-referential: [chat-mcp-connection-status.md](/Users/williamcory/crush/.smithers/specs/engineering/chat-mcp-connection-status.md).

## Recommended Direction
1. Add a Smithers MCP status selector in UI model.
- Resolve target server key with `smithers` default, but include fallback for renamed MCP entries.
- Map MCP states to UX states: `connected` -> connected; `starting/error/disabled/missing` -> disconnected (or “connecting” if desired).
2. Render the indicator in header/chat surface.
- Extend header render inputs so [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go) can render `● smithers connected/disconnected` using existing resource icon styles.
3. Reuse existing event flow.
- Keep current MCP pubsub path; no new transport layer is required.
4. Add tests in two layers.
- Terminal E2E path modeled on upstream harness: spawn TUI, poll stripped ANSI buffer, assert status text changes.
- VHS happy-path recording: add a Smithers MCP status tape and artifact.

## Files To Touch
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/model/mcp.go](/Users/williamcory/crush/internal/ui/model/mcp.go) (optional helper reuse)
- [internal/ui/styles/styles.go](/Users/williamcory/crush/internal/ui/styles/styles.go) (only if new style tokens are needed)
- [internal/agent/coordinator.go](/Users/williamcory/crush/internal/agent/coordinator.go) (if making Smithers MCP server name resolution configurable)
- [internal/ui/model/header_test.go](/Users/williamcory/crush/internal/ui/model/header_test.go) (new)
- [tests/e2e/tui_helpers_test.go](/Users/williamcory/crush/tests/e2e/tui_helpers_test.go) (new, spawn/poll helper modeled on upstream)
- [tests/e2e/tui_mcp_connection_status_test.go](/Users/williamcory/crush/tests/e2e/tui_mcp_connection_status_test.go) (new)
- [tests/vhs/smithers-mcp-connection-status.tape](/Users/williamcory/crush/tests/vhs/smithers-mcp-connection-status.tape) (new)
- [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md)
- [tests/vhs/fixtures/crush.json](/Users/williamcory/crush/tests/vhs/fixtures/crush.json) (or add a dedicated fixture for this scenario)

```json
{
  "document": "First research pass completed for chat-mcp-connection-status. Sections included: Existing Crush Surface, Upstream Smithers Reference, Gaps, Recommended Direction, Files To Touch."
}
```
# Engineering Spec: feat-mcp-tool-discovery

## Existing Crush Surface
- `internal/config/config.go`: Contains the core configuration structures (`MCPConfig`, `Agent` definitions). `SetupAgents()` configures default agents (`Coder` and `Task`), currently disabling MCP for `Task` (`AllowedMCP: map[string][]string{}`).
- `internal/app/app.go`: Manages application lifecycle and explicitly handles MCP initialization (`mcp.Initialize(ctx, app.Permissions, store)`) and awaits it (`mcp.WaitForInit(ctx)`).
- `internal/config/defaults.go` was listed in the source context but is missing in the current codebase (likely refactored into `init.go` and `config.go`). Default initialization logic is handled in `internal/config/init.go` and `internal/config/config.go`.

## Upstream Smithers Reference
- `docs/smithers-tui/01-PRD.md`: Defines the goal of using a `smithers mcp-serve` subcommand via stdio transport. It lists various tool categories (Runs, Observe, Control, Time-travel, Workflow, Memory, etc.) that the TUI will consume.
- `docs/smithers-tui/features.ts`: Formally inventories the `MCP_TOOL_DISCOVERY_FROM_SMITHERS_SERVER` feature and associated MCP tool categories.
- `../smithers/tests/tui.e2e.test.ts` & `tui-helpers.ts`: Demonstrates an E2E testing harness using a programmatic `BunSpawnBackend` to spawn the TUI, send keystrokes, and assert text in the terminal buffer (`waitForText`).
- `../smithers/docs/guides/smithers-tui-v2-agent-handoff.md`: Provides context on Playwright-driven testing and the V2 UI architecture.

## Gaps
- **Configuration**: Crush currently defaults to its own tools and does not automatically configure an MCP server for Smithers. The default configuration does not prioritize Smithers tools over general coding tools.
- **Agent Settings**: The default agents in Crush (e.g., `Task`) explicitly disable MCP by default.
- **Testing**: Crush lacks E2E tests modeled after the Smithers TUI terminal testing harness and VHS-style recordings for this specific integration.

## Recommended Direction
- **Configuration Update**: Modify the default configuration instantiation (in `internal/config/config.go` or `init.go`) to automatically inject a `smithers` MCP server utilizing stdio (`command: smithers mcp-serve`).
- **Agent Permissions**: Update `SetupAgents()` to ensure the default agent allows and prioritizes Smithers MCP tools.
- **Testing Integration**: Implement a terminal E2E path based on `../smithers/tests/tui.e2e.test.ts` that asserts the Smithers tools are correctly loaded and accessible. Add at least one VHS tape test verifying the happy-path flow for connecting to the MCP server.

## Files To Touch
- `internal/config/config.go` (and/or `init.go`)
- `internal/app/app.go`
- `tests/tui.e2e.test.ts` (or equivalent E2E file in Crush based on the upstream harness)
- `.vhs` or test script for VHS recording of the happy-path tool discovery.

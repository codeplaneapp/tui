## Existing Crush Surface
- `internal/config/config.go`: Defines the core `MCPConfig` structure and the `MCPs` mapping. Crucially, the `SetupAgents()` function configures the default agents (like `AgentCoder`), but it currently explicitly disables MCP capabilities by setting `AllowedMCP: map[string][]string{}`.
- `internal/app/app.go`: Manages the application lifecycle and already contains the core plumbing for MCP. It triggers initialization (`mcp.Initialize`), awaits it (`mcp.WaitForInit(ctx)`), and forces an update of agent models to ensure MCP tools are loaded (`app.AgentCoordinator.UpdateModels(ctx)`).
- `internal/config/init.go`: Contains the default configuration instantiation where the base toolset (`DefaultTools`) and connections are defined.

## Upstream Smithers Reference
- `docs/smithers-tui/features.ts`: Serves as the canonical inventory for Smithers TUI features. It formally defines the `MCP_TOOL_DISCOVERY_FROM_SMITHERS_SERVER` feature flag within the `MCP_INTEGRATION` group, alongside specific tool categories like `MCP_RUNS_TOOLS`, `MCP_OBSERVABILITY_TOOLS`, `MCP_CONTROL_TOOLS`, and `MCP_TIME_TRAVEL_TOOLS`.
- `../smithers/tests/tui.e2e.test.ts` & `../smithers/tests/tui-helpers.ts`: Implements a robust terminal E2E testing harness utilizing a `BunSpawnBackend` class. This backend spawns the TUI as a child process with piped stdin/stdout, allowing tests to send programmatic keystrokes and snapshot terminal output to verify functionality.

## Gaps
- **Configuration**: Crush's default configuration does not include a `smithers` stdio MCP server pointing to `smithers mcp-serve`.
- **Agent Limitations**: The default agents in Crush (e.g., `AgentCoder`) are restricted from using MCP tools because their `AllowedMCP` map is initialized empty, meaning no tools from any MCP server will be exposed to the agent.
- **Tool Prioritization**: General coding tools are currently default/prioritized over the required Smithers MCP management tools.
- **Testing**: Crush lacks an E2E testing suite modeled after the upstream `BunSpawnBackend` harness, and there are no VHS-style happy-path recordings verifying the successful discovery and connection to the MCP server.

## Recommended Direction
- **Configuration Update**: Modify `internal/config/init.go` and `internal/config/config.go` to automatically define a `smithers` MCP server utilizing the `stdio` transport, specifically running the command `smithers mcp-serve`.
- **Agent Permissions**: Update `SetupAgents()` in `config.go` so that the default agents explicitly allow the `smithers` MCP server within the `AllowedMCP` map. Adjust `DefaultTools` to prioritize the Smithers tool categories.
- **Testing Integration**: Implement a terminal E2E testing path in Crush modeled directly on the `BunSpawnBackend` harness from `../smithers/tests/tui-helpers.ts`. Additionally, create at least one VHS-style recording test to visually validate the happy-path connection and tool discovery flow in the Crush TUI.

## Files To Touch
- `internal/config/config.go`
- `internal/config/init.go`
- `internal/app/app.go`
- E2E testing harness (e.g., creating a Go-equivalent to `tui.e2e.test.ts`)
- A new `.vhs` tape script to record the happy-path tool discovery.
# Platform: MCP Server Config Integration

## Metadata
- ID: platform-mcp-transport
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_MCP_TRANSPORT
- Dependencies: platform-config-namespace

## Summary

Configure the built-in MCP client to spawn and connect to the `smithers mcp-serve` stdio server by default, granting the agent access to all Smithers CLI tools.

## Acceptance Criteria

- The default config automatically spins up `smithers mcp-serve` as an MCP stdio server
- Smithers tools auto-register with the chat agent
- Status bar indicates the 'smithers' MCP connection is active

## Source Context

- internal/config/defaults.go
- internal/config/config.go

## Implementation Notes

- Add a 'smithers' server block to DefaultTools.MCPServers specifying the 'smithers' command with 'mcp-serve' args.
- Ensure unused Crush tools (e.g. sourcegraph) are moved to the Disabled tools array.

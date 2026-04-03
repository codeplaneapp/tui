# Configure Smithers MCP Server Discovery

## Metadata
- ID: feat-mcp-tool-discovery
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_TOOL_DISCOVERY_FROM_SMITHERS_SERVER
- Dependencies: none

## Summary

Configure Crush to automatically discover and connect to the Smithers MCP server (`smithers mcp-serve`) on startup, setting Smithers tools as the primary tools in the default config.

## Acceptance Criteria

- Default configuration automatically sets up `smithers` stdio MCP server pointing to `smithers mcp-serve`.
- Default tool list in config prioritizes Smithers tools over general coding tools.
- Agent successfully discovers and can invoke Smithers MCP tools.

## Source Context

- internal/config/defaults.go
- internal/config/config.go
- internal/app/app.go

## Implementation Notes

- Modify `DefaultTools` in `internal/config/defaults.go` to include expected Smithers tools.
- Add `mcpServers` config for `smithers` in default config.
- Ensure MCP initialization in `app.go` picks up the local Smithers server.

# Base Scaffolding for Smithers Tool Renderers

## Metadata
- ID: eng-mcp-renderer-scaffolding
- Group: Mcp Integration (mcp-integration)
- Type: engineering
- Feature: n/a
- Dependencies: feat-mcp-tool-discovery

## Summary

Create the base abstraction and registration patterns for Smithers-specific MCP tool renderers in the TUI chat interface.

## Acceptance Criteria

- A standard pattern for parsing Smithers tool result JSON is established.
- Common UI styles (tables, cards, success/error indicators) for Smithers tools are defined in `internal/ui/styles`.
- A registry entry or switch case maps `smithers_*` tool calls to their respective renderers.

## Source Context

- internal/ui/chat/tools.go
- internal/ui/styles/styles.go

## Implementation Notes

- Crush registers tool renderers in `internal/ui/chat/tools.go` or similar.
- Provide a helper function for unmarshaling `mcp.ToolResult.Content` to Smithers domain structs.

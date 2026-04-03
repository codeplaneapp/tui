# Custom Tool Renderers

## Metadata
- ID: chat-custom-tool-renderers
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_CUSTOM_TOOL_RENDERERS
- Dependencies: chat-specialized-agent

## Summary

Build custom UI renderers for Smithers MCP tool results so they display as styled components instead of raw JSON.

## Acceptance Criteria

- Tool calls to `smithers_ps` render as styled tabular data in the chat stream.
- Tool calls to `smithers_approve` render as a styled confirmation card.
- Other Smithers tools render nicely instead of just dumping JSON to the chat.

## Source Context

- internal/ui/chat/tools.go
- internal/ui/chat/smithers_ps.go
- internal/ui/chat/smithers_approve.go

## Implementation Notes

- Create new files in `internal/ui/chat/` for Smithers-specific renderers.
- Implement `renderSmithersPS(result mcp.ToolResult)` and `renderSmithersApprove(result mcp.ToolResult)`.
- Register these rendering functions in the primary tool renderer registry in `internal/ui/chat/tools.go`.

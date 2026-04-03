# MCP Connection Status

## Metadata
- ID: chat-mcp-connection-status
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_MCP_CONNECTION_STATUS
- Dependencies: chat-ui-branding-status

## Summary

Show the Smithers MCP server connection status visually in the UI header or chat welcome area.

## Acceptance Criteria

- The UI displays whether the Smithers CLI MCP server is connected or disconnected.
- Updates dynamically as the MCP client establishes its connection.

## Source Context

- internal/ui/model/header.go
- internal/mcp/client.go

## Implementation Notes

- Query the global MCP client registry for the 'smithers' server connection state.
- Render the state as an indicator (e.g., green dot for connected) next to the Smithers status.

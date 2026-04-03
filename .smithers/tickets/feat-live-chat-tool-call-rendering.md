# Tool Call Rendering

## Metadata
- ID: feat-live-chat-tool-call-rendering
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_TOOL_CALL_RENDERING
- Dependencies: feat-live-chat-streaming-output

## Summary

Map Smithers tool calls (from NDJSON) to Crush's tool renderers so they appear correctly in the stream.

## Acceptance Criteria

- Tool calls render as styled boxes rather than raw JSON.
- Reuses existing Crush tool renderers.

## Source Context

- internal/ui/views/livechat.go
- internal/ui/chat/

## Implementation Notes

- Convert Smithers tool calls to mcp.ToolCall and mcp.ToolResult.
- Feed them into Crush's chat renderer component.

# Streaming Chat Output

## Metadata
- ID: feat-live-chat-streaming-output
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_STREAMING_OUTPUT
- Dependencies: feat-live-chat-viewer

## Summary

Render real-time streaming of agent prompt, stdout, stderr, and response via Smithers SSE events.

## Acceptance Criteria

- Chat blocks are appended in real time.
- UI does not block while waiting for events.
- E2E test verifies that text from a background agent run appears in the TUI stream.

## Source Context

- internal/ui/views/livechat.go
- internal/smithers/client.go

## Implementation Notes

- Map Smithers ChatAttempt structures to Crush's chat.Message model.
- Stream via Client.StreamChat(ctx, runID).

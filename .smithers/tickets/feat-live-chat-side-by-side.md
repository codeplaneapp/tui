# Side by Side Context

## Metadata
- ID: feat-live-chat-side-by-side
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_SIDE_BY_SIDE_CONTEXT
- Dependencies: feat-live-chat-viewer

## Summary

Multi-pane layout supporting viewing chat alongside the run status list.

## Acceptance Criteria

- Users can toggle a split pane showing the dashboard and the live chat simultaneously.

## Source Context

- internal/ui/views/livechat.go
- internal/ui/components/splitpane.go

## Implementation Notes

- Integrate with `internal/ui/components/splitpane.go`.
- Render Dashboard on the left and LiveChatView on the right.

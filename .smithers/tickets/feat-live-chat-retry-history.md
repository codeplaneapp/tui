# Retry History

## Metadata
- ID: feat-live-chat-retry-history
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_RETRY_HISTORY
- Dependencies: feat-live-chat-attempt-tracking

## Summary

Allow navigation between different attempts if retries occurred for the node.

## Acceptance Criteria

- Users can page through previous attempts' chat logs.
- UI indicates if viewing a historical attempt.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Add keybindings to fetch previous attempts via the HTTP API.
- Cache previous attempts locally in the view state.

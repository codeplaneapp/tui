# Attempt Tracking

## Metadata
- ID: feat-live-chat-attempt-tracking
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_ATTEMPT_TRACKING
- Dependencies: feat-live-chat-viewer

## Summary

Display the current attempt number in the chat header for the running node.

## Acceptance Criteria

- Header updates dynamically to show 'Attempt: N'.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Extract attempt count from the running Node details via the Client.

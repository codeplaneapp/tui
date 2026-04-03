# Follow Mode

## Metadata
- ID: feat-live-chat-follow-mode
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_FOLLOW_MODE
- Dependencies: feat-live-chat-streaming-output

## Summary

Add an auto-scroll 'follow mode' toggled by pressing 'f'.

## Acceptance Criteria

- Pressing 'f' toggles follow mode.
- When active, the viewport automatically scrolls to the bottom on new messages.
- When inactive, scrolling is manual.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Add a `following bool` to LiveChatView.
- Call `viewport.GotoBottom()` during Update if following is true.

# Live Chat Viewer UI

## Metadata
- ID: feat-live-chat-viewer
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: feature
- Feature: LIVE_CHAT_VIEWER
- Dependencies: eng-live-chat-scaffolding

## Summary

Base UI frame for viewing a running agent's chat, showing the run ID, agent name, node, and elapsed time.

## Acceptance Criteria

- Header displays Run ID, Agent Name, Node, and Time.
- Pressing 'c' on Run Dashboard opens this view.
- Covered by a Playwright-style E2E test verifying header text.
- Covered by a VHS-style recording test displaying the chat.

## Source Context

- internal/ui/views/livechat.go

## Implementation Notes

- Follow 02-DESIGN.md section 3.3 for the layout.
- Use Lip Gloss for the header styling.

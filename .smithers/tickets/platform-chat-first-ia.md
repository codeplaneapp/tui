# Platform: Chat-First Info Architecture

## Metadata
- ID: platform-chat-first-ia
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_CHAT_FIRST_INFORMATION_ARCHITECTURE
- Dependencies: platform-view-router

## Summary

Initialize the View Router with the Chat view as the root element that can never be popped, ensuring chat is always the home screen.

## Acceptance Criteria

- Router initializes with Chat at index 0
- Router.Pop() is a no-op if the stack size is 1
- Startup opens directly to Chat

## Source Context

- internal/ui/views/router.go
- internal/ui/model/ui.go

## Implementation Notes

- In `NewRouter`, populate the stack `[]View{chat}` and store `chat` on the router explicitly so we can access it.

# Default Chat Console

## Metadata
- ID: chat-default-console
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_DEFAULT_CONSOLE
- Dependencies: chat-ui-branding-status

## Summary

Establish the chat interface as the default Smithers TUI view, ensuring it acts as the base of the navigation stack.

## Acceptance Criteria

- Launching the application opens the chat interface.
- Pressing `Esc` from any view returns the user to the chat console.
- The chat interface displays correctly under the new Smithers branding.

## Source Context

- internal/ui/model/ui.go

## Implementation Notes

- Integrate the existing chat model as the base view in the new `views.Router` (assuming the router structure from the platform group).
- Ensure `Esc` key handling in `UI.Update()` delegates to popping the view stack back to the chat.

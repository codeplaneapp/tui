# Platform: Back Stack Navigation (Esc)

## Metadata
- ID: platform-back-stack
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_BACK_STACK_NAVIGATION
- Dependencies: platform-view-router

## Summary

Wire the Escape key to pop the current view off the stack, providing a standard 'Back' action across the app.

## Acceptance Criteria

- Pressing Esc on any view pops the stack and returns to the previous view
- Pressing Esc on the Chat view does not crash or pop the Chat view
- Help bar shows '[Esc] Back' when stack > 1

## Source Context

- internal/ui/model/keys.go
- internal/ui/model/ui.go

## Implementation Notes

- Add a `Back` key binding in `keys.go`. In `ui.go`'s top-level Update, intercept `Back` and call `m.router.Pop()`, short-circuiting to avoid passing Esc to child models if they don't need it.

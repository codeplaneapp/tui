# Platform: Implement View Stack Router

## Metadata
- ID: platform-view-router
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_VIEW_STACK_ROUTER
- Dependencies: platform-view-model

## Summary

Build the stack-based router that manages pushing and popping Views and delegates Bubble Tea messages to the active View.

## Acceptance Criteria

- Router struct tracks a stack of Views ([]View)
- Push(v) and Pop() methods modify the stack
- Current() returns the top of the stack
- The main Bubble Tea Update and View functions delegate to Router.Current()

## Source Context

- internal/ui/views/router.go
- internal/ui/model/ui.go

## Implementation Notes

- Modify the main `UI` struct in internal/ui/model/ui.go to embed the Router.
- Pass `tea.Msg` through to `m.router.Current().Update(msg)`.

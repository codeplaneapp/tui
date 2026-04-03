# Integrate toast overlays into the main TUI loop

## Metadata
- ID: notifications-toast-overlays
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: NOTIFICATIONS_TOAST_OVERLAYS
- Dependencies: eng-in-terminal-toast-component

## Summary

Hook the new toast component into the global UI view so it renders on top of the active route.

## Acceptance Criteria

- The toast overlay renders over chat, runs, or any other routed view.
- Notifications can be triggered globally via the pubsub event bus.

## Source Context

- internal/ui/model/ui.go
- internal/pubsub

## Implementation Notes

- Modify `UI.View()` in `internal/ui/model/ui.go` to append the rendered notification string (positioned bottom-right) after rendering the current view from the Router.

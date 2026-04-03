# Research Summary: notifications-toast-overlays

## Ticket Overview
- **ID**: notifications-toast-overlays
- **Group**: Approvals And Notifications
- **Type**: feature
- **Feature Flag**: NOTIFICATIONS_TOAST_OVERLAYS
- **Dependencies**: eng-in-terminal-toast-component

## Summary
Hook the new toast component into the global UI view so it renders on top of the active route.

## Acceptance Criteria
1. The toast overlay renders over chat, runs, or any other routed view.
2. Notifications can be triggered globally via the pubsub event bus.

## Source Context
- `internal/ui/model/ui.go` — Main UI model with `View()` and `Draw()` methods
- `internal/pubsub` — Event bus for global notification dispatch

## Implementation Notes
- Modify `UI.View()` in `internal/ui/model/ui.go` to append the rendered notification string (positioned bottom-right) after the active route view.
- The toast component (from `eng-in-terminal-toast-component`) should be integrated into the overlay system.

## Key Architecture Findings

### Overlay System (`internal/ui/dialog/overlay.go`)
- The `Overlay` struct holds a stack of `dialogs` and renders them via `Draw(scr, area)`.
- `DrawOnboardingCursor` positions a string view at the bottom-left of the screen using `common.BottomLeftRect`.
- The overlay iterates over its dialog stack in `Draw()`, rendering each dialog on top of the screen area.
- `removeDialog(idx)` removes a dialog from the stack by index.

### UI Model (`internal/ui/model/ui.go`)
- The main `UI` model orchestrates the top-level view rendering.
- The `View()` / `Draw()` method composes the active route view and then renders overlays on top.
- This is the integration point where toast notifications need to be layered.

### Pubsub Event Bus (`internal/pubsub`)
- Provides global event dispatch for decoupled notification triggering.
- Toast notifications should subscribe to relevant pubsub events (e.g., approval requests, run completions, run failures).

## Recommended Implementation Approach
1. Add a toast notification model/list to the `UI` struct that tracks active toasts with auto-dismiss timers.
2. Subscribe to pubsub events in the UI initialization to create toast notifications on relevant events.
3. In `UI.Draw()`, after rendering the active route and existing overlay dialogs, render active toasts in the bottom-right corner of the screen area.
4. Handle tick messages to auto-dismiss toasts after their TTL expires.
5. The toast component from `eng-in-terminal-toast-component` provides the rendering primitive; this ticket wires it into the global UI loop.
# Platform: Keyboard-First Navigation

## Metadata
- ID: platform-keyboard-nav
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_KEYBOARD_FIRST_NAVIGATION
- Dependencies: platform-view-router

## Summary

Register global keyboard shortcuts to immediately jump to primary views (e.g. Ctrl+R for Runs, Ctrl+A for Approvals).

## Acceptance Criteria

- Ctrl+R bound to Runs Dashboard
- Ctrl+A bound to Approval Queue
- Other key views get corresponding shortcuts
- Shortcuts push the relevant View onto the router stack

## Source Context

- internal/ui/model/keys.go
- internal/ui/model/ui.go

## Implementation Notes

- Add `RunDashboard`, `Approvals` to `KeyMap` in `keys.go`. Add switch cases in `ui.go` to construct and push the new views.

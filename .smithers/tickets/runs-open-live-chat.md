# Open Live Chat Keybinding

## Metadata
- ID: runs-open-live-chat
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_OPEN_LIVE_CHAT
- Dependencies: runs-dashboard

## Summary

Allow users to press 'c' to navigate to the Live Chat Viewer for the selected run.

## Acceptance Criteria

- Pressing 'c' pushes the LiveChatView onto the router stack for the run ID

## Source Context

- internal/ui/views/runs.go
- internal/ui/model/ui.go

## Implementation Notes

- Requires integration with the router stack manager.

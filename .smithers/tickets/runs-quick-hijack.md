# Quick Hijack Keybinding

## Metadata
- ID: runs-quick-hijack
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_QUICK_HIJACK
- Dependencies: runs-dashboard

## Summary

Allow users to press 'h' to initiate native TUI handoff for the active run.

## Acceptance Criteria

- Pressing 'h' suspends the Smithers TUI and hands off to the agent CLI via tea.ExecProcess

## Source Context

- internal/ui/views/runs.go
- internal/ui/views/livechat.go

## Implementation Notes

- Follow the native TUI handoff pattern defined in the design doc.

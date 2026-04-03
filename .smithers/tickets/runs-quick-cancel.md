# Quick Cancel Keybinding

## Metadata
- ID: runs-quick-cancel
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_QUICK_CANCEL
- Dependencies: runs-dashboard

## Summary

Allow users to press 'x' to cancel the selected active run.

## Acceptance Criteria

- Pressing 'x' prompts for a quick confirmation
- Confirming sends a cancel request to the API

## Source Context

- internal/ui/views/runs.go
- internal/smithers/client.go

## Implementation Notes

- Might require a simple overlay dialog or prompt area.

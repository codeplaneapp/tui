# Quick Deny Keybinding

## Metadata
- ID: runs-quick-deny
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_QUICK_DENY
- Dependencies: runs-dashboard, runs-inline-run-details

## Summary

Allow users to press 'd' to quickly deny a pending gate for the highlighted run.

## Acceptance Criteria

- Pressing 'd' on a pending run submits a deny request
- Visual state updates appropriately

## Source Context

- internal/ui/views/runs.go
- internal/smithers/client.go

## Implementation Notes

- Share logic with quick-approve where possible.

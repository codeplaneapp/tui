# Quick Approve Keybinding

## Metadata
- ID: runs-quick-approve
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_QUICK_APPROVE
- Dependencies: runs-dashboard, runs-inline-run-details

## Summary

Allow users to press 'a' to quickly approve a pending gate for the currently highlighted run.

## Acceptance Criteria

- Pressing 'a' on a pending run submits an approval request via the API
- A visual confirmation (toast or list update) is shown upon success

## Source Context

- internal/ui/views/runs.go
- internal/smithers/client.go

## Implementation Notes

- Ensure the 'a' key is only active when the selected row has an approval gate.

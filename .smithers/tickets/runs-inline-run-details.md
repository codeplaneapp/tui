# Inline Run Details

## Metadata
- ID: runs-inline-run-details
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_INLINE_RUN_DETAILS
- Dependencies: runs-dashboard

## Summary

Display secondary details below the main run row, such as the active agent name or pending gate details.

## Acceptance Criteria

- Active runs show the agent executing them
- Pending runs show the approval gate question
- Failed runs show the error reason

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Requires dynamic row height or a multi-line row rendering approach.

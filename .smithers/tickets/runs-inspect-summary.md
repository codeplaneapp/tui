# Run Inspector Base View

## Metadata
- ID: runs-inspect-summary
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_INSPECT_SUMMARY
- Dependencies: runs-dashboard

## Summary

Create the Run Inspector view, showing detailed summary and node information for a selected run.

## Acceptance Criteria

- Pressing Enter on a run opens the Run Inspector
- Displays run metadata (time, status, overall progress)
- Includes an E2E test verifying inspector navigation using @microsoft/tui-test

## Source Context

- internal/ui/views/runinspect.go
- ../smithers/gui/src/routes/runs/NodeInspector.tsx
- ../smithers/tests/tui.e2e.test.ts

## Implementation Notes

- This is the parent container for the DAG and Node detail tabs.

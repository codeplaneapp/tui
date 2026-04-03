# Group Runs by Status Sections

## Metadata
- ID: runs-status-sectioning
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_STATUS_SECTIONING
- Dependencies: runs-dashboard

## Summary

Enhance the Run Dashboard to visually group runs into sections like 'Active', 'Completed Today', and 'Failed'.

## Acceptance Criteria

- Runs are partitioned by status sections
- Sections map to the GUI parity layout
- List navigation correctly traverses between sections

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Update the list component to support section headers that cannot be selected.

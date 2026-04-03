# Status Filter UI

## Metadata
- ID: runs-filter-by-status
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_FILTER_BY_STATUS
- Dependencies: runs-dashboard

## Summary

Add a UI filter control to toggle visibility of runs by status (All, Active, Completed, Failed).

## Acceptance Criteria

- Filter dropdown is navigable via keyboard
- List filters instantly upon selection

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Implement custom top bar navigation.

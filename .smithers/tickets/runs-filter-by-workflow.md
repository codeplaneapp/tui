# Workflow Filter UI

## Metadata
- ID: runs-filter-by-workflow
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_FILTER_BY_WORKFLOW
- Dependencies: runs-dashboard

## Summary

Add a UI filter control to toggle visibility of runs by workflow type.

## Acceptance Criteria

- Can select a specific workflow from active and completed runs
- List updates to only show runs of that workflow

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Dropdown choices should be populated based on the fetched data.

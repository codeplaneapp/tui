# Date Range Filter UI

## Metadata
- ID: runs-filter-by-date-range
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_FILTER_BY_DATE_RANGE
- Dependencies: runs-dashboard

## Summary

Add a UI filter control to restrict the runs displayed by date range.

## Acceptance Criteria

- Filter allows selection of standard ranges (Today, Last 7 Days, All Time)

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Combine with the Smithers API's time filters if available.

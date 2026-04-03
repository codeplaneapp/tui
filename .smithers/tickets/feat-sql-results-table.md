# SQL Results Table

## Metadata
- ID: feat-sql-results-table
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: SQL_RESULTS_TABLE
- Dependencies: feat-sql-query-editor

## Summary

Implement a dynamic table view to render the results of the executed SQL queries.

## Acceptance Criteria

- Results are displayed in a table format.
- Columns are dynamically detected from the SQL result set keys.
- Horizontal scrolling is supported for wide result sets.

## Source Context

- internal/ui/sqlbrowser.go

## Implementation Notes

- Use `bubbles/table` and update its columns and rows based on the JSON response from the SQL execution.

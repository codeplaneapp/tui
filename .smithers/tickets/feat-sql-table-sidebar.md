# SQL Table Sidebar

## Metadata
- ID: feat-sql-table-sidebar
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: SQL_TABLE_SIDEBAR
- Dependencies: feat-sql-browser

## Summary

Implement the clickable list of available database tables in the SQL Browser sidebar.

## Acceptance Criteria

- Sidebar displays `_smithers_runs`, `_smithers_nodes`, `_smithers_events`, `_smithers_chat_attempts`, `_smithers_memory`.
- Selecting a table populates the query editor with `SELECT * FROM table LIMIT 50;`.

## Source Context

- internal/ui/sqlbrowser.go

## Implementation Notes

- Use `bubbles/list` for the sidebar component.

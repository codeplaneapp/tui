# SQL Query Editor

## Metadata
- ID: feat-sql-query-editor
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: SQL_QUERY_EDITOR
- Dependencies: feat-sql-browser

## Summary

Implement a text input area for users to write raw SQL queries against the Smithers database.

## Acceptance Criteria

- Multiline text area accepts SQL input.
- Pressing Ctrl+Enter executes the query via the API client.
- Displays execution errors elegantly in the UI.

## Source Context

- internal/ui/sqlbrowser.go

## Implementation Notes

- Use `bubbles/textarea` for the input component.

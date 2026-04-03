# SQL Browser Layout and Activation

## Metadata
- ID: feat-sql-browser
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: SQL_BROWSER
- Dependencies: eng-sql-scaffolding

## Summary

Implement the core layout and pane management for the SQL Browser view, setting up the structural grid that will contain the sidebar and main query areas.

## Acceptance Criteria

- Layout splits the terminal window into a left sidebar and a main right area.
- Focus can be toggled between the sidebar and the main area.
- UI reflects the design in Design Document Section 3.10.

## Source Context

- internal/ui/sqlbrowser.go
- ../smithers/gui-ref/src/ui/tabs/SqlBrowser.tsx

## Implementation Notes

- Use lipgloss for layout boundaries and borders.

# Scaffold SQL Browser View

## Metadata
- ID: eng-sql-scaffolding
- Group: Systems And Analytics (systems-and-analytics)
- Type: engineering
- Feature: n/a
- Dependencies: eng-systems-api-client

## Summary

Create the base Bubble Tea model and routing for the `/sql` view. This establishes the UI shell that will hold the table sidebar, query editor, and results table.

## Acceptance Criteria

- internal/ui/sqlbrowser.go is created with a base Bubble Tea model.
- Typing `/sql` in the console or selecting it from the palette navigates to this view.
- Escape key pops the view off the back stack.

## Source Context

- internal/ui/sqlbrowser.go
- internal/ui/model/

## Implementation Notes

- Follow the established UI architecture in `internal/ui`. Use `tea.Model` patterns.

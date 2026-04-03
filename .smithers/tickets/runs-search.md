# Run Text Search

## Metadata
- ID: runs-search
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_SEARCH
- Dependencies: runs-dashboard

## Summary

Add a search input box to fuzzy filter the run list by ID, workflow name, or inline details.

## Acceptance Criteria

- Key shortcut to focus search
- Typing dynamically filters the list

## Source Context

- internal/ui/views/runs.go

## Implementation Notes

- Use the Bubbles textinput component.

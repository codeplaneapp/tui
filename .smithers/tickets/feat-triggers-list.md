# Triggers List

## Metadata
- ID: feat-triggers-list
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: TRIGGERS_LIST
- Dependencies: eng-triggers-scaffolding

## Summary

Implement a list view showing all scheduled cron triggers.

## Acceptance Criteria

- Displays a list of triggers including workflow path, cron pattern, and enabled status.

## Source Context

- internal/ui/triggers.go

## Implementation Notes

- Use `bubbles/table` to cleanly align the cron properties.

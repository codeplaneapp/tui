# Cross-Run Message History

## Metadata
- ID: feat-memory-cross-run-message-history
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: MEMORY_CROSS_RUN_MESSAGE_HISTORY
- Dependencies: feat-memory-browser

## Summary

Implement the detail pane showing the contextual conversation threads across runs associated with a memory fact.

## Acceptance Criteria

- When a fact is selected, its context/history thread is displayed.
- Content is wrapped and scrollable.

## Source Context

- internal/ui/memory.go

## Implementation Notes

- Use `bubbles/viewport` to display long message threads.

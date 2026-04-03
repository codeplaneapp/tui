# Memory Fact List

## Metadata
- ID: feat-memory-fact-list
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: MEMORY_FACT_LIST
- Dependencies: feat-memory-browser

## Summary

Implement a view listing cross-run memory facts retrieved from the API.

## Acceptance Criteria

- Displays a paginated or scrolling list of memory facts.
- Selecting a fact highlights it for detail viewing.

## Source Context

- internal/ui/memory.go

## Implementation Notes

- Use `bubbles/list`.

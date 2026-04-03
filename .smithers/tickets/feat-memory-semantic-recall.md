# Memory Semantic Recall

## Metadata
- ID: feat-memory-semantic-recall
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: MEMORY_SEMANTIC_RECALL
- Dependencies: feat-memory-browser

## Summary

Add a search input field to query the memory system via natural language.

## Acceptance Criteria

- Search input filters or queries the memory list.
- Executes a semantic recall request against the Smithers client.

## Source Context

- internal/ui/memory.go

## Implementation Notes

- Use `bubbles/textinput` embedded above the list.

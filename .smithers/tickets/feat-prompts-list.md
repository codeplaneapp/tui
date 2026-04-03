# Prompts List View

## Metadata
- ID: feat-prompts-list
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_LIST
- Dependencies: eng-prompts-api-client, eng-split-pane-component

## Summary

Implement the `/prompts` view showing a side-by-side list of available prompts.

## Acceptance Criteria

- Prompts are fetched from the API and listed on the left pane.
- Terminal E2E test navigating the prompts list.

## Source Context

- ../smithers/gui/src/ui/PromptsList.tsx
- internal/ui/prompts/

## Implementation Notes

- Create `internal/ui/prompts/prompts.go`.
- Use `bubbles/list` for the left pane and the shared split pane component for the layout.

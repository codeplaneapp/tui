# Save Prompt Source Changes

## Metadata
- ID: feat-prompts-save
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_SAVE
- Dependencies: feat-prompts-source-edit, eng-prompts-api-client

## Summary

Save modifications made to the prompt source back to disk via the backend API.

## Acceptance Criteria

- Pressing `Ctrl+S` saves the inline edits to the backend.
- Success or error message is shown.

## Source Context

- ../smithers/gui/src/ui/PromptsList.tsx

## Implementation Notes

- Hook `Ctrl+S` keybinding to the `UpdatePromptSource` API wrapper.

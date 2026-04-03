# Prompt Source Editor

## Metadata
- ID: feat-prompts-source-edit
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_SOURCE_EDIT
- Dependencies: feat-prompts-list

## Summary

Display and allow editing of the selected prompt's `.mdx` source in the right pane.

## Acceptance Criteria

- Selecting a prompt shows its source code in an editable textarea.
- User can type and modify the source directly.

## Source Context

- ../smithers/gui/src/ui/PromptsList.tsx

## Implementation Notes

- Use `bubbles/textarea` for rendering and editing the prompt source.

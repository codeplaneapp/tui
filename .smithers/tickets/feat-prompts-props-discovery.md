# Prompt Props Discovery and Input

## Metadata
- ID: feat-prompts-props-discovery
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_PROPS_DISCOVERY
- Dependencies: feat-prompts-source-edit

## Summary

Dynamically render input fields for discovered props required by the prompt.

## Acceptance Criteria

- A list of text inputs is rendered based on the prompt's `inputs` schema.
- Users can enter test values for the prompt variables.

## Source Context

- ../smithers/gui/src/ui/PromptsList.tsx

## Implementation Notes

- Iterate over the `inputs` property from the backend payload and generate `bubbles/textinput` instances.

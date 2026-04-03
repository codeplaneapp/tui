# Prompt Live Preview Rendering

## Metadata
- ID: feat-prompts-live-preview
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_LIVE_PREVIEW
- Dependencies: feat-prompts-props-discovery, eng-prompts-api-client

## Summary

Call the render API with inputted props to preview the prompt output.

## Acceptance Criteria

- A 'Render Preview' action triggers the backend preview endpoint.
- The output is displayed in a dedicated preview section.
- VHS-style happy-path recording test covers live preview.

## Source Context

- ../smithers/gui/src/ui/PromptsList.tsx

## Implementation Notes

- Bind a shortcut to invoke the `RenderPromptPreview` API with the state gathered from the prop inputs.

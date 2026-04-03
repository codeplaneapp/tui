# Implement Prompts API Client Methods

## Metadata
- ID: eng-prompts-api-client
- Group: Content And Prompts (content-and-prompts)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Add HTTP or MCP client methods to fetch prompts, update prompt sources, and render prompt previews.

## Acceptance Criteria

- Client exposes `ListPrompts`, `UpdatePromptSource`, and `RenderPromptPreview` operations.
- Terminal E2E test verifying API client capabilities for prompts.

## Source Context

- ../smithers/gui/src/api/transport.ts

## Implementation Notes

- Mirror `fetchPrompts`, `updatePromptSource`, and `renderPromptPreview` from the GUI transport.
- Ensure `RenderPromptPreview` correctly passes down a map of key-value props.

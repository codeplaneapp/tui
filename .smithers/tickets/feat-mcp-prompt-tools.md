# Prompt Tool Renderers

## Metadata
- ID: feat-mcp-prompt-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_PROMPT_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for prompt management tools (`smithers_prompt_list`, `smithers_prompt_update`, `smithers_prompt_render`).

## Acceptance Criteria

- `smithers_prompt_list` shows available prompts.
- `smithers_prompt_render` shows the evaluated output of the prompt template.
- `smithers_prompt_update` renders a success message.

## Source Context

- internal/ui/chat/smithers_prompts.go

## Implementation Notes

- For `smithers_prompt_render`, use markdown rendering (Glamour) if applicable.

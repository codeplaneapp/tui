# Run Management Tool Renderers

## Metadata
- ID: feat-mcp-runs-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_RUNS_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for Smithers run management tools (`smithers_ps`, `smithers_up`, `smithers_cancel`, `smithers_down`).

## Acceptance Criteria

- `smithers_ps` renders a formatted table of active and completed runs.
- `smithers_up` renders a success card with the new run ID.
- `smithers_cancel` and `smithers_down` render confirmation indicators.

## Source Context

- internal/ui/chat/smithers_runs.go

## Implementation Notes

- Use Lip Gloss tables for `smithers_ps`.
- Register renderers for all runs-related tools.

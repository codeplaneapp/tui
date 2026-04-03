# Scoring Tool Renderers

## Metadata
- ID: feat-mcp-scoring-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_SCORING_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for the `smithers_scores` tool.

## Acceptance Criteria

- `smithers_scores` displays evaluation metrics and token usage in a clear grid or table.

## Source Context

- internal/ui/chat/smithers_scoring.go

## Implementation Notes

- Metrics might need specific formatting (percentages, charts) if data allows.

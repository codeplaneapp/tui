# Observability Tool Renderers

## Metadata
- ID: feat-mcp-observability-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_OBSERVABILITY_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for Smithers observability tools (`smithers_logs`, `smithers_chat`, `smithers_inspect`).

## Acceptance Criteria

- `smithers_inspect` renders a detailed DAG or node list for a run.
- `smithers_logs` formats log events clearly with timestamps.
- `smithers_chat` renders conversational blocks from an agent run.

## Source Context

- internal/ui/chat/smithers_observability.go

## Implementation Notes

- Map JSON arrays from `smithers_chat` to readable message sequences.

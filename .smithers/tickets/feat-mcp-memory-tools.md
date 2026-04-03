# Memory Tool Renderers

## Metadata
- ID: feat-mcp-memory-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_MEMORY_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for memory tools (`smithers_memory_list`, `smithers_memory_recall`).

## Acceptance Criteria

- `smithers_memory_list` shows facts stored in memory.
- `smithers_memory_recall` displays facts matching the semantic query.

## Source Context

- internal/ui/chat/smithers_memory.go

## Implementation Notes

- Format memory facts as a simple list.

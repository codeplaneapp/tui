# SQL Tool Renderers

## Metadata
- ID: feat-mcp-sql-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_SQL_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for the `smithers_sql` tool.

## Acceptance Criteria

- `smithers_sql` renders raw database query results in a dynamic table.

## Source Context

- internal/ui/chat/smithers_sql.go

## Implementation Notes

- Dynamic table rendering since SQL results have arbitrary columns.

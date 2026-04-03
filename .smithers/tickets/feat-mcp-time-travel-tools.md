# Time-Travel Tool Renderers

## Metadata
- ID: feat-mcp-time-travel-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_TIME_TRAVEL_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for time-travel tools (`smithers_diff`, `smithers_fork`, `smithers_replay`, `smithers_timeline`).

## Acceptance Criteria

- `smithers_timeline` renders a horizontal or vertical snapshot history.
- `smithers_diff` highlights changed state between snapshots.
- `smithers_fork` and `smithers_replay` show success indicators with new run IDs.

## Source Context

- internal/ui/chat/smithers_time_travel.go

## Implementation Notes

- Use Lip Gloss or Charm `ansi` for diff highlighting.

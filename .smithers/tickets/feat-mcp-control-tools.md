# Control & Hijack Tool Renderers

## Metadata
- ID: feat-mcp-control-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_CONTROL_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for Smithers control tools (`smithers_approve`, `smithers_deny`, `smithers_hijack`).

## Acceptance Criteria

- `smithers_approve` and `smithers_deny` show clear success or error indicators.
- `smithers_hijack` shows instructions or confirmation of hijack transition.

## Source Context

- internal/ui/chat/smithers_control.go

## Implementation Notes

- Visual distinctness is important for approval/deny actions (e.g., green checkmark vs red X).

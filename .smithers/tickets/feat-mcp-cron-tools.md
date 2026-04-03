# Cron Tool Renderers

## Metadata
- ID: feat-mcp-cron-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_CRON_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for cron schedule tools (`smithers_cron_list`, `smithers_cron_add`, `smithers_cron_rm`, `smithers_cron_toggle`).

## Acceptance Criteria

- `smithers_cron_list` shows schedules with enable/disable indicators.
- Mutation tools render success notifications.

## Source Context

- internal/ui/chat/smithers_cron.go

## Implementation Notes

- Format crontab nicely with translation to human-readable strings if possible.

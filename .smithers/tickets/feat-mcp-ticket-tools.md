# Ticket Tool Renderers

## Metadata
- ID: feat-mcp-ticket-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_TICKET_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for ticket management tools (`smithers_ticket_list`, `smithers_ticket_create`, `smithers_ticket_update`).

## Acceptance Criteria

- `smithers_ticket_list` displays a table of tickets.
- `smithers_ticket_create` and `smithers_ticket_update` render success messages.

## Source Context

- internal/ui/chat/smithers_tickets.go

## Implementation Notes

- Basic list/detail formatting.

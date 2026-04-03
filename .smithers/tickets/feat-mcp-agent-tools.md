# Agent Tool Renderers

## Metadata
- ID: feat-mcp-agent-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_AGENT_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for agent tools (`smithers_agent_list`, `smithers_agent_chat`).

## Acceptance Criteria

- `smithers_agent_list` displays agent binaries and availability status.
- `smithers_agent_chat` prompts the user about native TUI handoff or shows chat outcome.

## Source Context

- internal/ui/chat/smithers_agents.go

## Implementation Notes

- Align `agent_list` table structure with the dedicated `/agents` view.

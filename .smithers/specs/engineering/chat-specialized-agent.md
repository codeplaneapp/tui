# Specialized Agent Configuration

## Metadata
- ID: chat-specialized-agent
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_SPECIALIZED_AGENT
- Dependencies: chat-workspace-context

## Summary

Configure the default agent to utilize the new Smithers system prompt, disable irrelevant tools, and prioritize Smithers MCP tools.

## Acceptance Criteria

- The default tool configuration disables irrelevant tools like `sourcegraph`.
- The agent is explicitly configured to interact with the Smithers MCP server tools.

## Source Context

- internal/config/defaults.go
- internal/agent/agent.go

## Implementation Notes

- The agent configuration system already exists in `internal/agent/agent.go` with tool filtering capabilities.
- The config system in `internal/config/` supports scoped configuration with defaults, project, and user layers.
- MCP tool integration patterns are established in the codebase.
- The TUI test harness at `internal/e2e/` provides `launchTUI()` for end-to-end testing of agent behavior.
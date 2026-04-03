# Workspace Context Discovery

## Metadata
- ID: chat-workspace-context
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_WORKSPACE_CONTEXT_DISCOVERY
- Dependencies: chat-domain-system-prompt

## Summary

Inject local workspace context (like workflow directory and active runs) into the agent's system prompt during template execution.

## Acceptance Criteria

- The agent system prompt receives dynamic context such as `.WorkflowDir` and `.ActiveRuns`.
- The agent is aware of the currently active workflow runs without needing to execute a tool first.

## Source Context

- internal/agent/agent.go
- internal/smithers/client.go

## Implementation Notes

- Modify the template execution data payload to include `WorkflowDir` from the configuration and `ActiveRuns` from the Smithers client state.
- Ensure the context is refreshed when a new session starts.

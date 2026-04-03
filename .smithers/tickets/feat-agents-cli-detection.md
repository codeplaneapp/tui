# Agent CLI Detection and Listing

## Metadata
- ID: feat-agents-cli-detection
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_CLI_DETECTION
- Dependencies: feat-agents-browser

## Summary

Populate the Agent Browser list with real data fetched from the Smithers API, showing all detected agent CLI tools on the system.

## Acceptance Criteria

- The agents list is populated dynamically via SmithersClient.ListAgents().
- Users can navigate the list using standard up/down arrow keys.
- The name of each agent (e.g., claude-code, codex) is rendered prominently.

## Source Context

- internal/ui/views/agents.go
- internal/smithers/client.go

## Implementation Notes

- Handle loading states and potential API errors gracefully within the view's Update and View loops.

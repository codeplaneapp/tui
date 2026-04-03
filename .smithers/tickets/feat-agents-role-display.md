# Agent Role Display

## Metadata
- ID: feat-agents-role-display
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_ROLE_DISPLAY
- Dependencies: feat-agents-cli-detection

## Summary

Render the list of supported roles or capabilities (e.g., coding, research, review) provided by the agent.

## Acceptance Criteria

- Displays 'Roles: <role1>, <role2>' on the same line as the auth status.
- Roles are comma-separated and properly capitalized.

## Source Context

- internal/ui/views/agents.go
- docs/smithers-tui/02-DESIGN.md

## Implementation Notes

- Extract roles from the agent metadata returned by the Smithers API.

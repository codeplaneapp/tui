# Agent Binary Path Display

## Metadata
- ID: feat-agents-binary-path-display
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_BINARY_PATH_DISPLAY
- Dependencies: feat-agents-cli-detection

## Summary

Enhance the agent list items to display the physical binary path discovered for each agent.

## Acceptance Criteria

- Each agent list item renders a 'Binary: <path>' line below the agent name.
- If no binary is found, it renders 'Binary: —'.

## Source Context

- internal/ui/views/agents.go
- docs/smithers-tui/02-DESIGN.md

## Implementation Notes

- Use Lip Gloss styles to format the path with a slightly dimmed or secondary text color.

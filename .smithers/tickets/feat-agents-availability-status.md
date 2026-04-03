# Agent Availability Status

## Metadata
- ID: feat-agents-availability-status
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_AVAILABILITY_STATUS
- Dependencies: feat-agents-cli-detection

## Summary

Render the overall availability status of each agent (e.g., likely-subscription, api-key, binary-only, unavailable).

## Acceptance Criteria

- Each agent displays a 'Status: ● <status>' line.
- Applies distinct colors for different states (e.g., green for likely-subscription/api-key, gray for unavailable).

## Source Context

- internal/ui/views/agents.go
- internal/ui/styles/styles.go

## Implementation Notes

- Define Lip Gloss styles in internal/ui/styles/styles.go mapping to these specific Smithers agent statuses.

# Agent Auth Status Classification

## Metadata
- ID: feat-agents-auth-status-classification
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_AUTH_STATUS_CLASSIFICATION
- Dependencies: feat-agents-cli-detection

## Summary

Render the detailed authentication and API key validation status markers for each agent.

## Acceptance Criteria

- Displays 'Auth: ✓' or 'Auth: ✗' indicating active authentication.
- Displays 'API Key: ✓' or 'API Key: ✗' indicating environment variable presence.
- Icons use standard success/error colors (green/red).

## Source Context

- internal/ui/views/agents.go
- docs/smithers-tui/02-DESIGN.md

## Implementation Notes

- Align the output on a single line matching the design doc: `Auth: ✓  API Key: ✓  Roles: ...`

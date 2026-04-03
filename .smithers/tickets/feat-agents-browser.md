# Agents Browser Base View

## Metadata
- ID: feat-agents-browser
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_BROWSER
- Dependencies: eng-agents-view-scaffolding

## Summary

Implement the main Bubble Tea view for the Agent Browser, rendering the layout frame and handling standard navigation.

## Acceptance Criteria

- Navigating to /agents or using the command palette opens the Agents view.
- The view displays a 'SMITHERS › Agents' header and a placeholder list.
- Pressing Esc returns the user to the previous view (chat/console).

## Source Context

- internal/ui/views/agents.go
- docs/smithers-tui/02-DESIGN.md

## Implementation Notes

- Reference the design doc section 3.7 for layout specifications.
- Leverage internal/ui/styles/styles.go for standard layout margins and colors.

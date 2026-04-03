# Ticket Detail View

## Metadata
- ID: feat-tickets-detail-view
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: TICKETS_DETAIL_VIEW
- Dependencies: feat-tickets-split-pane

## Summary

Render the full markdown content of the selected ticket in the right pane.

## Acceptance Criteria

- Selecting a ticket updates the right pane with its markdown content.
- Markdown is formatted properly.
- VHS-style happy-path recording test verifying ticket selection.

## Source Context

- ../smithers/gui/src/ui/TicketsList.tsx

## Implementation Notes

- Use `glamour` or `lipgloss` to render the ticket markdown.

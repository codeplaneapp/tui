# Tickets List View

## Metadata
- ID: feat-tickets-list
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: TICKETS_LIST
- Dependencies: eng-tickets-api-client

## Summary

Implement the main view that fetches and displays all available tickets from the backend.

## Acceptance Criteria

- Displays a list of tickets fetched from the backend.
- User can navigate the list using arrow keys.
- Terminal E2E test covers navigating the ticket list.

## Source Context

- ../smithers/gui/src/ui/TicketsList.tsx
- internal/ui/tickets/

## Implementation Notes

- Create `internal/ui/tickets/tickets.go`.
- Use the `bubbles/list` component to render the ticket IDs and snippets.

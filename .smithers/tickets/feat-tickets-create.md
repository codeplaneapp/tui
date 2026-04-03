# Create New Ticket Form

## Metadata
- ID: feat-tickets-create
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: TICKETS_CREATE
- Dependencies: feat-tickets-split-pane, eng-tickets-api-client

## Summary

Add functionality to create a new ticket from within the TUI.

## Acceptance Criteria

- User can open a form to enter a new Ticket ID and markdown content.
- Submitting the form creates the ticket via the API and refreshes the list.
- Terminal E2E test verifying ticket creation.

## Source Context

- ../smithers/gui/src/ui/TicketsList.tsx

## Implementation Notes

- Provide a keybinding (e.g., `n`) to switch the right pane to a creation form.
- Use `bubbles/textinput` for the ID and `bubbles/textarea` for the body.

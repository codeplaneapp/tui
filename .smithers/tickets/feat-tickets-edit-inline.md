# Inline Ticket Editing

## Metadata
- ID: feat-tickets-edit-inline
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: TICKETS_EDIT_INLINE
- Dependencies: feat-tickets-detail-view, eng-tickets-api-client

## Summary

Allow users to edit existing ticket content inline directly inside the right pane.

## Acceptance Criteria

- User can toggle edit mode for a selected ticket.
- Content becomes editable in a textarea.
- Saving persists the changes via the API.
- VHS-style happy-path recording test covers editing ticket content.

## Source Context

- ../smithers/gui/src/ui/TicketsList.tsx

## Implementation Notes

- Provide an `e` keybinding to switch the detail view to an edit form.
- Manage saving state and handle API errors gracefully.

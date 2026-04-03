# Tickets Split Pane Layout

## Metadata
- ID: feat-tickets-split-pane
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: TICKETS_SPLIT_PANE_LAYOUT
- Dependencies: feat-tickets-list, eng-split-pane-component

## Summary

Integrate the shared split pane component into the tickets view to show the list on the left and a detail pane on the right.

## Acceptance Criteria

- Tickets list renders on the left side.
- An empty or placeholder view renders on the right side if no ticket is selected.

## Source Context

- ../smithers/gui/src/ui/TicketsList.tsx

## Implementation Notes

- Wrap the tickets list in the left pane of `internal/ui/components/splitpane`.

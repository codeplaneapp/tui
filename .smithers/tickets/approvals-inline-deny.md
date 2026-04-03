# Implement inline deny action

## Metadata
- ID: approvals-inline-deny
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_INLINE_DENY
- Dependencies: approvals-context-display

## Summary

Allow the user to deny a gate directly from the TUI.

## Acceptance Criteria

- Pressing the configured key (e.g., 'd' or 'x') sends a deny API request.
- Upon success, the item is removed from the pending queue.

## Source Context

- internal/ui/views/approvals.go
- internal/smithers/client.go

## Implementation Notes

- Provide an optional input field for a denial reason if supported by the Smithers API.

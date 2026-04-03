# Implement inline approval action

## Metadata
- ID: approvals-inline-approve
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_INLINE_APPROVE
- Dependencies: approvals-context-display

## Summary

Allow the user to approve a gate directly from the TUI.

## Acceptance Criteria

- Pressing the configured key (e.g., 'a') sends an approve API request.
- Upon success, the item is removed from the pending queue.

## Source Context

- internal/ui/views/approvals.go
- internal/smithers/client.go

## Implementation Notes

- Show a loading indicator while the API request is inflight.

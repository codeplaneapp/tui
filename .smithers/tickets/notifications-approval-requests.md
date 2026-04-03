# Show notifications for approval requests

## Metadata
- ID: notifications-approval-requests
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: NOTIFICATIONS_APPROVAL_REQUESTS
- Dependencies: notifications-toast-overlays

## Summary

Listen for Smithers SSE events indicating a new approval gate and show a toast notification.

## Acceptance Criteria

- When an `ApprovalRequested` SSE event is received, a toast appears.
- Toast includes 'Approve' and 'View' action hints.

## Source Context

- internal/smithers/client.go
- internal/ui/model/ui.go

## Implementation Notes

- Subscribe to the appropriate event from `SmithersClient.StreamEvents()` and map it to the notification component.

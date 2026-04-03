# Show notifications for run failures

## Metadata
- ID: notifications-run-failures
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: NOTIFICATIONS_RUN_FAILURES
- Dependencies: notifications-toast-overlays

## Summary

Show a toast notification when a Smithers run fails.

## Acceptance Criteria

- When a `RunFailed` SSE event is received, a failure toast appears.

## Source Context

- internal/smithers/client.go

## Implementation Notes

- Format the toast with error styling (Lip Gloss red) and the run ID.

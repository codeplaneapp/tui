# Show notifications for run completions

## Metadata
- ID: notifications-run-completions
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: NOTIFICATIONS_RUN_COMPLETIONS
- Dependencies: notifications-toast-overlays

## Summary

Show a brief toast notification when a Smithers run completes successfully.

## Acceptance Criteria

- When a `RunCompleted` SSE event is received, a success toast appears.

## Source Context

- internal/smithers/client.go

## Implementation Notes

- Format the toast with success styling (Lip Gloss green) and a short TTL.

# Show pending approval badges in global UI

## Metadata
- ID: approvals-pending-badges
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_PENDING_BADGES
- Dependencies: approvals-queue

## Summary

Display a visual indicator in the main UI (e.g., header or status bar) when approvals are pending.

## Acceptance Criteria

- If `pending_count > 0`, a badge is visible on the main screen.
- Badge updates dynamically via SSE events.

## Source Context

- internal/ui/model/status.go
- internal/ui/model/header.go

## Implementation Notes

- Integrate with Crush's existing status bar (`internal/ui/model/status.go`) to add the badge.

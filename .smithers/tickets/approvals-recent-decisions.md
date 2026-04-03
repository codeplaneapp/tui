# Show recent approval decisions

## Metadata
- ID: approvals-recent-decisions
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_RECENT_DECISIONS
- Dependencies: eng-approvals-view-scaffolding

## Summary

Display a history of recently approved or denied gates in the approvals view.

## Acceptance Criteria

- A section or toggleable view shows historical decisions.
- Each entry shows the decision made and timestamps.

## Source Context

- internal/ui/views/approvals.go

## Implementation Notes

- This might be a separate tab within the approvals view or a section below the pending queue.

# Display context for the selected approval

## Metadata
- ID: approvals-context-display
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_CONTEXT_DISPLAY
- Dependencies: approvals-queue

## Summary

Show the task, inputs, and workflow context for the currently highlighted approval in the queue.

## Acceptance Criteria

- A details pane updates as the user moves the cursor through the queue.
- Details include the specific question/gate and any relevant payload.

## Source Context

- internal/ui/views/approvals.go

## Implementation Notes

- Consider a split-pane layout within the approvals view (list on left, details on right or bottom).

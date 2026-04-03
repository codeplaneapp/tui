# Pending Approval Summary

## Metadata
- ID: chat-pending-approval-summary
- Group: Chat And Console (chat-and-console)
- Type: feature
- Feature: CHAT_SMITHERS_PENDING_APPROVAL_SUMMARY
- Dependencies: chat-active-run-summary

## Summary

Display the aggregate number of pending Smithers approval gates in the UI header with a warning indicator.

## Acceptance Criteria

- The header displays '⚠ Y pending approval' when there are workflows waiting at a gate.
- If there are both active runs and pending approvals, they are separated by a center dot.

## Source Context

- internal/ui/model/header.go

## Implementation Notes

- Extend the `renderSmithersStatus()` function to check for pending approvals from the Smithers client and append the warning string to the status parts array.

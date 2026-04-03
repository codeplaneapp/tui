# Add E2E tests for approvals and notifications

## Metadata
- ID: eng-approvals-e2e-tests
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: engineering
- Feature: n/a
- Dependencies: approvals-inline-approve, approvals-inline-deny, notifications-approval-requests

## Summary

Implement automated testing for the approvals flow using both the terminal E2E harness and VHS recordings.

## Acceptance Criteria

- Playwright-style E2E test added covering the notification -> queue -> approve flow.
- VHS script created demonstrating a happy-path approval.

## Source Context

- ../smithers/tests/tui.e2e.test.ts
- ../smithers/tests/tui-helpers.ts

## Implementation Notes

- Mock the Smithers HTTP server in the E2E test to emit the `ApprovalRequested` SSE event and respond to the approval POST.

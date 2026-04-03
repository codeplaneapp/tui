# Live Chat & Hijack E2E Tests

## Metadata
- ID: eng-live-chat-e2e-testing
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: engineering
- Feature: n/a
- Dependencies: feat-live-chat-viewer, feat-hijack-seamless-transition

## Summary

Implement E2E testing for the Live Chat and Hijack flows utilizing the Playwright TUI test harness and a VHS recording script.

## Acceptance Criteria

- Playwright E2E tests exist for opening Live Chat and initiating Hijack.
- A .tape VHS file exists that successfully records a happy-path chat stream.

## Source Context

- tests/livechat.e2e.test.ts
- tests/vhs/live-chat.tape

## Implementation Notes

- Model the E2E tests on `tui.e2e.test.ts` from the Smithers UI v2 project.
- Ensure we await terminal buffer outputs for 'Hijacking run...'.

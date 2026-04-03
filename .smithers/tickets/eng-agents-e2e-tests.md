# Engineering: Agents View E2E and Visual Tests

## Metadata
- ID: eng-agents-e2e-tests
- Group: Agents (agents)
- Type: engineering
- Feature: n/a
- Dependencies: feat-agents-binary-path-display, feat-agents-availability-status, feat-agents-auth-status-classification, feat-agents-role-display, feat-agents-native-tui-launch

## Summary

Add comprehensive end-to-end testing and terminal recording for the Agents view functionality.

## Acceptance Criteria

- Add a test in ../smithers/tests/tui.e2e.test.ts that navigates to the /agents view and verifies the rendering of the agent list.
- Simulate an Enter keypress in the E2E test to verify the TUI correctly attempts a handoff.
- Create a VHS tape recording (.tape file) demonstrating navigation to the Agents view, scrolling, and launching an agent.

## Source Context

- ../smithers/tests/tui.e2e.test.ts
- ../smithers/tests/tui-helpers.ts

## Implementation Notes

- Model the E2E test on the existing fan-out-fan-in runs dashboard test. You may need to mock the smithers API or ensure the test environment exposes mock agents.

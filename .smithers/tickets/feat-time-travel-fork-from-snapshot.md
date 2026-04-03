# Fork Run From Snapshot

## Metadata
- ID: feat-time-travel-fork-from-snapshot
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_FORK_FROM_SNAPSHOT
- Dependencies: feat-time-travel-timeline-view

## Summary

Allow users to fork a run from the selected snapshot checkpoint.

## Acceptance Criteria

- Pressing 'f' hotkey triggers a fork for the currently selected snapshot.
- Application transitions to the newly created run after a successful fork.
- Terminal E2E test verifies the fork command invocation and navigation.

## Source Context

- docs/smithers-tui/01-PRD.md
- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Send a ForkRun request via the API client.
- On success, emit a Bubble Tea command to trigger a router navigation to the new run ID.

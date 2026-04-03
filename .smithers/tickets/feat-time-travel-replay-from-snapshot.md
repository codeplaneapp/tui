# Replay Run From Snapshot

## Metadata
- ID: feat-time-travel-replay-from-snapshot
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_REPLAY_FROM_SNAPSHOT
- Dependencies: feat-time-travel-timeline-view

## Summary

Allow users to replay a run starting from the selected snapshot checkpoint.

## Acceptance Criteria

- Pressing 'r' hotkey triggers a replay for the currently selected snapshot.
- Application transitions to live replay view of the run.
- Terminal E2E test verifies the replay behavior.

## Source Context

- docs/smithers-tui/01-PRD.md
- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Send a ReplayRun request via the API client.
- Transition the UI state to handle live run updates for the new or replayed run ID.

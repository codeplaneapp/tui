# Time Travel Snapshot Diff

## Metadata
- ID: feat-time-travel-snapshot-diff
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_SNAPSHOT_DIFF
- Dependencies: feat-time-travel-snapshot-inspector

## Summary

Implement snapshot comparison allowing users to view diffs between consecutive snapshots.

## Acceptance Criteria

- Pressing 'd' hotkey fetches the diff between the cursor and previous snapshot.
- Diff is rendered clearly, highlighting state changes.

## Source Context

- docs/smithers-tui/02-DESIGN.md
- internal/ui/diffview/diffview.go

## Implementation Notes

- Call the DiffSnapshots client API when the diff hotkey is pressed.
- Integrate internal/ui/diffview/diffview.go for rendering the output diff.
- Handle loading states gracefully while the diff is computed/fetched.

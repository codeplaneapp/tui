# Time Travel Snapshot Inspector

## Metadata
- ID: feat-time-travel-snapshot-inspector
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_SNAPSHOT_INSPECTOR
- Dependencies: feat-time-travel-timeline-view

## Summary

Render the detailed state of the currently selected snapshot below the timeline graph.

## Acceptance Criteria

- When a snapshot is selected, its details (ID, associated node, partial output/state) are displayed below the timeline.
- The view updates instantly as the cursor moves.

## Source Context

- docs/smithers-tui/02-DESIGN.md

## Implementation Notes

- Extract snapshot details from the currently selected `smithers.Snapshot`.
- Format the output using markdown components or standard lipgloss blocks.

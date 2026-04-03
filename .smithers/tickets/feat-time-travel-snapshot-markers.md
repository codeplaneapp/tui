# Time Travel Snapshot Markers

## Metadata
- ID: feat-time-travel-snapshot-markers
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_SNAPSHOT_MARKERS
- Dependencies: feat-time-travel-timeline-view

## Summary

Differentiate snapshot markers visually on the timeline based on run events or status.

## Acceptance Criteria

- Timeline graph uses different symbols or colors for different types of snapshots (e.g., error nodes, tool calls).
- Selected snapshot is clearly indicated with an arrow (▲).

## Source Context

- docs/smithers-tui/02-DESIGN.md
- internal/ui/styles/styles.go

## Implementation Notes

- Leverage internal/ui/styles for consistent marker rendering.
- Update the lipgloss rendering logic to conditionally style graph nodes based on the underlying `smithers.Snapshot` data.

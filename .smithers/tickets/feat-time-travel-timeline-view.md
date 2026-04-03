# Time Travel Timeline View

## Metadata
- ID: feat-time-travel-timeline-view
- Group: Time Travel (time-travel)
- Type: feature
- Feature: TIME_TRAVEL_TIMELINE_VIEW
- Dependencies: eng-time-travel-api-and-model

## Summary

Implement the horizontal visual timeline of run execution and snapshot navigation.

## Acceptance Criteria

- User can view a horizontal timeline graph of snapshots (e.g. ①──②──③...).
- User can navigate left and right using arrow keys to select a snapshot cursor.
- A VHS-style happy path test captures timeline navigation.

## Source Context

- docs/smithers-tui/02-DESIGN.md
- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Use Lip Gloss to render the timeline as described in 03-ENGINEERING.md section 3.3.1.
- Track `cursor` state integer for the selected snapshot.
- Ensure navigation keys update the cursor and trigger view re-renders.

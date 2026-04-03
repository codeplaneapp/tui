# Toggle Trigger Status

## Metadata
- ID: feat-triggers-toggle
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: TRIGGERS_TOGGLE
- Dependencies: feat-triggers-list

## Summary

Add functionality to enable or disable a trigger directly from the list.

## Acceptance Criteria

- Pressing 'Space' or 't' toggles the selected trigger's status.
- Status update is persisted via the Smithers API/CLI.

## Source Context

- internal/ui/triggers.go

## Implementation Notes

- Ensure optimistic UI updates are reverted if the backend call fails.

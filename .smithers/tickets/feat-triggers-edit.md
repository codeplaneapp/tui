# Edit Trigger

## Metadata
- ID: feat-triggers-edit
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: TRIGGERS_EDIT
- Dependencies: feat-triggers-list

## Summary

Implement functionality to modify an existing cron trigger.

## Acceptance Criteria

- User can edit the cron expression or workflow path of an existing trigger.
- Changes are saved to the backend.

## Source Context

- internal/ui/triggers.go

## Implementation Notes

- Reuse the form component created in TRIGGERS_CREATE, pre-populating it with existing data.

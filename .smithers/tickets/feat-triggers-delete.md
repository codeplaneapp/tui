# Delete Trigger

## Metadata
- ID: feat-triggers-delete
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: TRIGGERS_DELETE
- Dependencies: feat-triggers-list

## Summary

Implement functionality to delete a scheduled trigger.

## Acceptance Criteria

- User can select and delete a trigger.
- Prompts for confirmation before deletion.
- List updates appropriately.

## Source Context

- internal/ui/triggers.go

## Implementation Notes

- Simple modal or inline confirmation (y/n) is sufficient.

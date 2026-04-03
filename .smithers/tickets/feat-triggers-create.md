# Create Trigger

## Metadata
- ID: feat-triggers-create
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: TRIGGERS_CREATE
- Dependencies: feat-triggers-list

## Summary

Implement a form overlay or inline input to create a new cron trigger.

## Acceptance Criteria

- User can input a valid cron expression and target workflow path.
- Submission creates the trigger via the Smithers API/CLI.
- List is refreshed upon successful creation.

## Source Context

- internal/ui/triggers.go

## Implementation Notes

- Consider using `charmbracelet/huh` for a clean form input experience.

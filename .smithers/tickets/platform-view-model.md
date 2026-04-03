# Platform: View Stack Architecture

## Metadata
- ID: platform-view-model
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_WORKSPACE_AND_SYSTEMS_VIEW_MODEL
- Dependencies: none

## Summary

Introduce the View interface representing distinct TUI screens (Runs, SQL, Timeline, Chat) adhering to the Workspace/Systems separation.

## Acceptance Criteria

- View interface defined with Init, Update, View, and Name methods
- ShortHelp() method added to View to power contextual help bars

## Source Context

- internal/ui/views/router.go

## Implementation Notes

- Define the View interface. This abstracts the generic tea.Model so we can add Smithers-specific view metadata like Name and Help keys.

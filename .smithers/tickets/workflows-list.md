# Workflows List View

## Metadata
- ID: workflows-list
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_LIST
- Dependencies: workflows-discovery-from-project

## Summary

Create the main '/workflows' view displaying all available workflows in the project, allowing users to browse them.

## Acceptance Criteria

- Implement internal/ui/views/workflows.go utilizing a Bubble Tea list or table.
- Display workflow ID, display name, and source type for each discovered workflow.
- Allow keyboard navigation through the workflow list.
- Include a VHS-style happy-path recording test for navigating the workflow list.

## Source Context

- internal/ui/views/workflows.go
- internal/ui/model/ui.go
- ../smithers/gui/src/ui/WorkflowsList.tsx

## Implementation Notes

- Use Crush's imperative sub-component pattern as recommended in docs.
- Follow the brand color scheme (Bright cyan for headers, etc.).

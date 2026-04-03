# Node Input Tab

## Metadata
- ID: runs-task-tab-input
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_TASK_TAB_INPUT
- Dependencies: runs-node-inspector

## Summary

Implement the Input tab to show the payload passed to the selected node.

## Acceptance Criteria

- Displays JSON or structured text of the node's input payload

## Source Context

- internal/ui/views/tasktabs.go
- internal/ui/components/jsontree.go
- ../smithers/gui/src/routes/runs/TaskTabs.tsx

## Implementation Notes

- Utilize the internal/ui/components/jsontree.go component for rendering.

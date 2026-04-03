# DAG Inspection Visualizer

## Metadata
- ID: workflows-dag-inspection
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_DAG_INSPECTION
- Dependencies: workflows-list

## Summary

Render a visual representation of the workflow's Directed Acyclic Graph (DAG) structure.

## Acceptance Criteria

- Render the DAG using ASCII/UTF-8 box-drawing characters in a left-to-right layout.
- Color-code nodes based on their status (e.g., green=done, yellow=running, red=failed, gray=pending).
- Display the DAG overview when inspecting a workflow or viewing an active run.

## Source Context

- internal/ui/components/dagview.go
- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Implement `RenderDAG(nodes []smithers.Node) string` utilizing topological sorting to group nodes by depth.
- Ensure it fits within standard terminal widths without breaking layout.

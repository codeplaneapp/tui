# Node Inspector Selection

## Metadata
- ID: runs-node-inspector
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_NODE_INSPECTOR
- Dependencies: runs-dag-overview

## Summary

Allow selection of a specific node within the Run Inspector to view its specific details.

## Acceptance Criteria

- Can navigate through the nodes in the DAG view
- Selection drives the content of the detail tabs

## Source Context

- internal/ui/views/runinspect.go

## Implementation Notes

- Manage focused node state within the runinspect model.

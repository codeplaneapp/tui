# Node Config Tab

## Metadata
- ID: runs-task-tab-config
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_TASK_TAB_CONFIG
- Dependencies: runs-node-inspector

## Summary

Implement the Config tab to display configuration parameters for the selected node.

## Acceptance Criteria

- Shows node execution config (e.g., retries, timeout, tools allowed)

## Source Context

- internal/ui/views/tasktabs.go

## Implementation Notes

- Map from the node definition config.

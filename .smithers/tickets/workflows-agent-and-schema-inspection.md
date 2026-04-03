# Agent and Schema Inspection

## Metadata
- ID: workflows-agent-and-schema-inspection
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_AGENT_AND_SCHEMA_INSPECTION
- Dependencies: workflows-list

## Summary

Expose the underlying agents and I/O schemas associated with a selected workflow for detailed inspection.

## Acceptance Criteria

- Show assigned agents for specific workflow nodes.
- Display the JSON structures corresponding to input and output schemas of the nodes.
- Allow users to toggle schema visibility inside the node inspector.

## Source Context

- internal/ui/views/workflows.go
- ../smithers/src/SmithersWorkflow.ts

## Implementation Notes

- Surface the `schemaRegistry` and `zodToKeyName` data sent by the Smithers API in the node details pane.

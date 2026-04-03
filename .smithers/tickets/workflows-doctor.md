# Workflow Doctor Diagnostics

## Metadata
- ID: workflows-doctor
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_DOCTOR
- Dependencies: workflows-list

## Summary

Provide a diagnostic view for a workflow to catch misconfigurations, missing agents, or schema errors before execution.

## Acceptance Criteria

- Run the 'workflow doctor' MCP tool or equivalent validation logic for a selected workflow.
- Render diagnostic warnings and errors clearly in the UI.
- Provide actionable suggestions if a dependency (like an API key or CLI agent) is missing.

## Source Context

- internal/ui/views/workflows.go
- ../smithers/src/cli/workflow-pack.ts

## Implementation Notes

- If `workflow doctor` is exposed as an MCP tool, integrate with the existing MCP transport layer.
- Display the output in a split-pane or dedicated diagnostic overlay.

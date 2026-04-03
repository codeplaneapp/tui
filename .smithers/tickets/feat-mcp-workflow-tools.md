# Workflow Tool Renderers

## Metadata
- ID: feat-mcp-workflow-tools
- Group: Mcp Integration (mcp-integration)
- Type: feature
- Feature: MCP_WORKFLOW_TOOLS
- Dependencies: eng-mcp-renderer-scaffolding

## Summary

Implement UI renderers for workflow tools (`smithers_workflow_list`, `smithers_workflow_run`, `smithers_workflow_doctor`).

## Acceptance Criteria

- `smithers_workflow_list` displays available workflows in a list.
- `smithers_workflow_run` confirms execution.
- `smithers_workflow_doctor` clearly highlights warnings and errors.

## Source Context

- internal/ui/chat/smithers_workflows.go

## Implementation Notes

- Format `doctor` output similar to ESLint or Go diagnostics.

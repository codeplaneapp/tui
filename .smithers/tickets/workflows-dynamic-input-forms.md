# Dynamic Input Forms for Workflows

## Metadata
- ID: workflows-dynamic-input-forms
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_DYNAMIC_INPUT_FORMS
- Dependencies: workflows-list

## Summary

Generate interactive input forms for executing a workflow based on its declared input schema.

## Acceptance Criteria

- Selecting a workflow dynamically generates a form corresponding to its input parameters.
- Support string, number, boolean, object, and array types accurately.
- Pre-fill form fields with default values derived from the workflow's definition.

## Source Context

- internal/ui/components/form.go
- internal/ui/views/workflows.go
- ../smithers/gui/src/ui/WorkflowsList.tsx

## Implementation Notes

- Mirror the logic found in `../smithers/gui/src/ui/WorkflowsList.tsx` for parsing inputs and setting default states.
- Utilize a Bubble Tea form library (like `huh`) or custom input components to render the form.

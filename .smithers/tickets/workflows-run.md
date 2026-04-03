# Execute Workflow

## Metadata
- ID: workflows-run
- Group: Workflows (workflows)
- Type: feature
- Feature: WORKFLOWS_RUN
- Dependencies: workflows-dynamic-input-forms

## Summary

Connect the dynamic form submission to the execution API, allowing users to start a workflow directly from the TUI.

## Acceptance Criteria

- Pressing 'Enter' on a completed form submits the payload to the `RunWorkflow` API endpoint.
- Provide immediate visual feedback (e.g., executing spinner, success toast, or error message).
- On successful execution, automatically route the user to the newly created run's live chat or inspector view.
- Include a terminal E2E path modeled on the upstream @microsoft/tui-test harness in ../smithers/tests/tui.e2e.test.ts

## Source Context

- internal/ui/views/workflows.go
- internal/smithers/client.go
- ../smithers/tests/tui.e2e.test.ts
- ../smithers/tests/tui-helpers.ts

## Implementation Notes

- Ensure keyboard shortcuts (like [Tab] to next field, [Esc] to cancel) match the design spec in `02-DESIGN.md`.

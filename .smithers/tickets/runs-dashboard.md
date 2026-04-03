# Run Dashboard Base View

## Metadata
- ID: runs-dashboard
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_DASHBOARD
- Dependencies: eng-smithers-client-runs

## Summary

Create the foundational Run Dashboard view to display a list of runs.

## Acceptance Criteria

- Accessible via /runs or Ctrl+R from the chat
- Displays a tabular list of runs fetching data via the Smithers Client
- Includes basic navigation using Up/Down arrows
- Includes a VHS-style happy-path test recording the view opening and basic navigation
- Includes an E2E test using @microsoft/tui-test harness via ../smithers/tests/tui-helpers.ts

## Source Context

- internal/ui/views/runs.go
- internal/ui/components/runtable.go
- ../smithers/gui/src/routes/runs/RunsList.tsx
- ../smithers/tests/tui.e2e.test.ts
- ../smithers/tests/tui-helpers.ts

## Implementation Notes

- Model the view logic on Bubble Tea's viewport and table components.
- Ensure the E2E test asserts table columns like 'Workflow' and 'Status' are rendered correctly.

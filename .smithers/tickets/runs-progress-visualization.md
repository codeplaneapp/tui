# Node Progress Bars

## Metadata
- ID: runs-progress-visualization
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_PROGRESS_VISUALIZATION
- Dependencies: runs-dashboard

## Summary

Render inline progress bars (e.g., 3/5 nodes completed) for active runs.

## Acceptance Criteria

- Progress bars are drawn using block characters
- Completed fraction matches node completion state
- Color coding maps to status (e.g., green for complete, yellow for active)

## Source Context

- internal/ui/components/progressbar.go
- internal/ui/views/runs.go

## Implementation Notes

- Create a reusable progress bar component utilizing Lip Gloss for styling.

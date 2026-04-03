# Scores Daily & Weekly Summaries

## Metadata
- ID: feat-scores-daily-and-weekly-summaries
- Group: Systems And Analytics (systems-and-analytics)
- Type: feature
- Feature: SCORES_DAILY_AND_WEEKLY_SUMMARIES
- Dependencies: feat-scores-and-roi-dashboard

## Summary

Implement top-level aggregation metrics showing runs, success counts, and running states for today and the week.

## Acceptance Criteria

- Displays total runs, successful runs, running jobs, and failures aggregated by day/week.
- Metrics update when the view is loaded.

## Source Context

- internal/ui/scores.go

## Implementation Notes

- Use lipgloss to create a visually distinct header section.

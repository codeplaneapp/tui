# Scores Dashboard E2E Tests

## Metadata
- ID: eng-scores-e2e
- Group: Systems And Analytics (systems-and-analytics)
- Type: engineering
- Feature: n/a
- Dependencies: feat-scores-daily-and-weekly-summaries, feat-scores-run-evaluations, feat-scores-token-usage-metrics, feat-scores-tool-call-metrics, feat-scores-latency-metrics, feat-scores-cache-efficiency-metrics, feat-scores-cost-tracking

## Summary

Create automated tests for the Scores Dashboard to ensure data renders correctly.

## Acceptance Criteria

- Includes a terminal E2E Playwright-style test.
- Includes a VHS-style happy-path recording.

## Source Context

- ../smithers/tests/tui.e2e.test.ts

## Implementation Notes

- Mock the GetScores client method to provide consistent metrics.

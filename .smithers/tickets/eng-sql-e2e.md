# SQL Browser E2E Tests

## Metadata
- ID: eng-sql-e2e
- Group: Systems And Analytics (systems-and-analytics)
- Type: engineering
- Feature: n/a
- Dependencies: feat-sql-table-sidebar, feat-sql-results-table

## Summary

Create automated tests for the SQL Browser view to ensure regressions are not introduced.

## Acceptance Criteria

- Includes a terminal E2E test modeled on the upstream `@microsoft/tui-test` harness in `../smithers/tests/tui.e2e.test.ts`.
- Includes at least one VHS-style happy-path recording test verifying table selection and query execution.

## Source Context

- ../smithers/tests/tui.e2e.test.ts
- ../smithers/tests/tui-helpers.ts

## Implementation Notes

- Mock the Smithers API client to return deterministic database results for testing.

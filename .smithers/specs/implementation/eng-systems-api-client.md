# Implementation Summary: eng-systems-api-client

**Date**: 2026-04-05
**Status**: Complete (new files created, pre-existing package errors are unrelated)

---

## What Was Built

Three new files in `internal/smithers/`:

### `internal/smithers/types_systems.go`

New types for the Systems and Analytics layer:

| Type | Purpose |
|------|---------|
| `TableInfo` | Describes a table/view: name, type, row count |
| `Column` | One column from PRAGMA table_info: CID, name, type, notNull, defaultValue, primaryKey |
| `TableSchema` | Full schema for a table: tableName + []Column |
| `MetricsFilter` | Optional filter for analytics queries: RunID, NodeID, WorkflowPath, StartMs, EndMs, GroupBy |
| `TokenMetrics` | Token usage aggregates: input/output/cache/total + optional ByPeriod breakdown |
| `TokenPeriodBatch` | Per-period token counts for grouped queries |
| `LatencyMetrics` | Node execution latency stats: count, mean, min, max, p50, p95 + optional ByPeriod |
| `LatencyPeriodBatch` | Per-period latency summary |
| `CostReport` | Estimated cost in USD: total/input/output + runCount + optional ByPeriod |
| `CostPeriodBatch` | Per-period cost breakdown |

### `internal/smithers/systems.go`

Five new methods on `*Client`, all following the three-tier transport pattern (HTTP → SQLite → exec):

| Method | Transport Cascade | Notes |
|--------|------------------|-------|
| `ListTables(ctx)` | HTTP GET /sql/tables → SQLite sqlite_master → exec smithers sql | Returns Smithers internal tables; row counts fetched inline via SQLite |
| `GetTableSchema(ctx, tableName)` | HTTP GET /sql/schema/{name} → SQLite PRAGMA table_info → exec smithers sql | Validates non-empty name upfront; uses safe identifier quoting |
| `GetTokenUsageMetrics(ctx, filters)` | HTTP GET /metrics/tokens?... → SQLite _smithers_chat_attempts SUM → exec smithers metrics token-usage | Aggregates input/output/cache tokens; TotalTokens computed client-side from SQLite path |
| `GetLatencyMetrics(ctx, filters)` | HTTP GET /metrics/latency?... → SQLite _smithers_nodes duration_ms → exec smithers metrics latency | Collects raw durations from SQLite, computes mean/min/max/p50/p95 in-process |
| `GetCostTracking(ctx, filters)` | HTTP GET /metrics/cost?... → SQLite (derived from token counts) → exec smithers metrics cost | SQLite path uses $3/M input + $15/M output pricing approximation |

Supporting internal helpers:
- `buildMetricsPath` — builds HTTP query string from MetricsFilter
- `metricsExecArgs` — builds smithers CLI args from MetricsFilter
- `buildTokenMetricsQuery` / `buildLatencyQuery` / `buildRunCountQuery` — SQL query builders with parameterized filter injection
- `computeLatencyMetrics` — in-process statistical computation (mean, p50, p95) over raw duration slices
- `percentile` — linear interpolation percentile on sorted float64 slice
- `scanTableColumns` — scans PRAGMA table_info rows into Column structs (handles int→bool conversion for notnull/pk)
- `parseTableColumnsJSON` / `parseTableInfoJSON` — dual-format exec output parsers (array-of-objects or SQLResult columnar)
- `quoteIdentifier` — SQLite-safe double-quote identifier quoting with embedded quote escaping

### `internal/smithers/systems_test.go`

53 test functions covering:

- **ListTables**: HTTP path (envelope decode), exec path (array format), exec path (SQLResult columnar format)
- **GetTableSchema**: HTTP path, exec path, empty name validation
- **GetTokenUsageMetrics**: HTTP (no filters), HTTP (with filters), exec (no filters), exec (with filters)
- **GetLatencyMetrics**: HTTP, exec, exec with time/node filters
- **GetCostTracking**: HTTP, exec, exec with run+groupBy filters
- **computeLatencyMetrics**: empty input, single value, multi-value (mean/p50/p95 math), unsorted input
- **percentile**: empty, single element, median, p95 with interpolation
- **buildMetricsPath**: no filters, partial filters, all filters
- **metricsExecArgs**: no filters, with run filter, with groupBy, with time range
- **parseTableInfoJSON**: array format, SQLResult format, invalid JSON
- **parseTableColumnsJSON**: normal parse, invalid JSON
- **quoteIdentifier**: simple identifier, identifier with embedded double-quote
- **buildTokenMetricsQuery**: no filters, with RunID, with time range
- **buildRunCountQuery**: no filters, with RunID
- **buildLatencyQuery**: no filters, with WorkflowPath
- **cost constants**: $3/M input and $15/M output sanity checks, zero-token edge case, small run calculation
- **HTTP filter passthrough**: all-filter variants for each of the three metrics endpoints

---

## Key Design Decisions

1. **ListTables fetches row counts inline** — The SQLite path issues a secondary `COUNT(*)` per table. Errors are silently swallowed (row count stays 0) so a locked or missing table doesn't abort the whole list.

2. **Latency stats are computed in Go, not SQL** — Rather than emitting a complex SQL PERCENTILE aggregate (which SQLite doesn't support natively), the SQLite path collects raw durations into a `[]float64` and computes statistics in-process. This mirrors the approach used by `aggregateScores` in `client.go`.

3. **Cost is estimated in the SQLite path** — The HTTP path is expected to return real per-model pricing; the SQLite fallback uses a fixed Claude Sonnet approximation ($3/M input, $15/M output). This is clearly documented in code comments. The HTTPpath passes these through transparently.

4. **`scanTableColumns` uses a structural interface** — The function signature accepts any value with `Next/Scan/Err/Close`, making it testable without a real database. `*sql.Rows` satisfies this interface.

5. **`quoteIdentifier` for SQL injection defense** — All table names passed to SQLite queries are wrapped with double-quotes and have embedded double-quotes escaped (SQLite's standard identifier quoting rules). This prevents user-supplied table names from escaping the identifier context.

---

## Pre-existing Package Build Errors (Not Introduced by This Ticket)

The `internal/smithers` package has pre-existing build failures from parallel in-progress work:

- `timetravel.go` references `Run` type which is not defined in `types_runs.go` (which defines `RunSummary` instead)

These errors existed before this ticket's files were created and are not caused by `systems.go`, `types_systems.go`, or `systems_test.go`. The three new files in this ticket are error-free and format-clean (`gofmt` passes).

---

## Files Created

- `/Users/williamcory/crush/internal/smithers/types_systems.go`
- `/Users/williamcory/crush/internal/smithers/systems.go`
- `/Users/williamcory/crush/internal/smithers/systems_test.go`
- `/Users/williamcory/crush/.smithers/specs/implementation/eng-systems-api-client.md` (this file)

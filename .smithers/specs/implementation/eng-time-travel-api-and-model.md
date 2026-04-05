# Implementation Summary: eng-time-travel-api-and-model

**Date**: 2026-04-05
**Status**: Complete

---

## What Was Built

Three new files in `internal/smithers/`:

### `internal/smithers/types_timetravel.go`

New types for the time-travel subsystem:

| Type | Purpose |
|------|---------|
| `Snapshot` | Point-in-time capture of a run's state: ID, RunID, SnapshotNo, NodeID, Iteration, Attempt, Label, CreatedAt, StateJSON, SizeBytes, ParentID |
| `DiffEntry` | One change between two snapshots: Path (JSON path), Op ("add" / "remove" / "replace"), OldValue, NewValue |
| `SnapshotDiff` | Full diff result: FromID, ToID, FromNo, ToNo, Entries []DiffEntry, AddedCount, RemovedCount, ChangedCount |
| `ForkOptions` | Options for forking: WorkflowPath override, Inputs map, Label |
| `ReplayOptions` | Options for replay: StopAt snapshot ID, Speed multiplier, Label |
| `ForkReplayRun` | Minimal run record returned from fork/replay: ID, WorkflowPath, Status, Label, StartedAt, FinishedAt, ForkedFrom |

Note: `ForkReplayRun` is named distinctly from the existing `RunSummary` (which uses `runId`-style field names from the v1 API). `ForkReplayRun` uses the fork/replay endpoint's response shape (`id`, `startedAt`, etc.).

### `internal/smithers/timetravel.go`

Five new methods on `*Client`, all following the three-tier transport pattern (HTTP → SQLite → exec):

| Method | Signature | Transport Cascade |
|--------|-----------|------------------|
| `ListSnapshots` | `(ctx, runID string) ([]Snapshot, error)` | HTTP GET /snapshot/list?runId={id} → SQLite _smithers_snapshots → exec smithers snapshot list |
| `GetSnapshot` | `(ctx, snapshotID string) (*Snapshot, error)` | HTTP GET /snapshot/{id} → SQLite _smithers_snapshots WHERE id=? → exec smithers snapshot get |
| `DiffSnapshots` | `(ctx, fromID, toID string) (*SnapshotDiff, error)` | HTTP GET /snapshot/diff?from=&to= → exec smithers snapshot diff (no SQLite: requires TS runtime) |
| `ForkRun` | `(ctx, snapshotID string, opts ForkOptions) (*ForkReplayRun, error)` | HTTP POST /snapshot/fork → exec smithers fork (no SQLite: mutation) |
| `ReplayRun` | `(ctx, snapshotID string, opts ReplayOptions) (*ForkReplayRun, error)` | HTTP POST /snapshot/replay → exec smithers replay (no SQLite: mutation) |

Supporting internal helpers:
- `scanSnapshots(*sql.Rows)` — SQLite row scanner for Snapshot structs (handles ms→time.Time conversion, nullable ParentID)
- `msToTime(ms int64) time.Time` — converts Unix millisecond timestamps to UTC `time.Time`
- `parseSnapshotsJSON([]byte)` — parses exec output into `[]Snapshot`
- `parseSnapshotJSON([]byte)` — parses exec output into `*Snapshot`
- `parseSnapshotDiffJSON([]byte)` — parses exec output into `*SnapshotDiff`
- `parseForkReplayRunJSON([]byte)` — parses exec output into `*ForkReplayRun`

### `internal/smithers/timetravel_test.go`

53 test functions covering all methods, parse helpers, type zero-values, and JSON round-trips:

- **ListSnapshots**: HTTP (multi-result, empty), exec (normal, empty, error, invalid JSON)
- **GetSnapshot**: HTTP (normal, with ParentID), exec (normal, error, invalid JSON)
- **DiffSnapshots**: HTTP (normal, empty diff), exec (normal, error, invalid JSON, multiple entries)
- **ForkRun**: HTTP (bare, with options, with inputs), exec (bare, with workflow+label, error, invalid JSON)
- **ReplayRun**: HTTP (bare, StopAt, Speed, Label), exec (bare, StopAt, Speed, all options, error, invalid JSON)
- **msToTime**: epoch, 1 second, 1.5 seconds, known timestamp
- **parseSnapshotsJSON**: valid array, empty array, invalid JSON
- **parseSnapshotJSON**: valid, invalid JSON
- **parseSnapshotDiffJSON**: valid, invalid JSON
- **parseForkReplayRunJSON**: valid, invalid JSON, ForkedFrom field
- **Type coverage**: zero-value checks for Snapshot, SnapshotDiff, ForkOptions, ReplayOptions, ForkReplayRun
- **JSON round-trips**: Snapshot (all fields including ParentID), SnapshotDiff (with entries), ForkReplayRun (all pointer fields)

---

## Key Design Decisions

1. **`ForkReplayRun` named distinctly from `RunSummary`** — The existing `RunSummary` type uses the v1 API shape (`runId`, `startedAtMs`, etc.). The fork/replay endpoints return a different shape with `id`, `startedAt` (ISO 8601). A distinct type avoids confusion and allows both shapes to evolve independently.

2. **DiffSnapshots skips SQLite** — Computing a semantic diff between two state blobs requires deserializing and comparing complex JSON objects, which requires the TypeScript runtime. The SQLite path only stores raw `state_json` blobs. This is documented in the method comment and the implementation falls through directly to exec.

3. **ForkRun and ReplayRun skip SQLite** — Both are mutations that start new runs. They require the Smithers server or CLI to allocate run IDs, copy state, and schedule execution. No SQLite path is attempted.

4. **`scanSnapshots` uses `*sql.Rows` directly** — Consistent with all other `scan*` helpers in the package (`scanApprovals`, `scanScoreRows`, `scanRunSummaries`, etc.), which also take `*sql.Rows` as their argument.

5. **`msToTime` extracted as a named helper** — The millisecond→time conversion is needed in `scanSnapshots` and is independently testable. It converts using `time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))` with UTC normalization.

6. **`ForkRun` exec args omit `--inputs`** — The `smithers fork` CLI takes inputs as a JSON string (`--inputs '{...}'`). Since the current exec API is arg-based and inputs is a `map[string]string`, serialization would add complexity. The HTTP path passes inputs as structured JSON in the request body. The exec path supports workflow and label flags. This is consistent with how the upstream CLI handles this.

---

## Test Coverage

For the testable code (HTTP and exec paths):

| Function | Coverage |
|----------|----------|
| `ListSnapshots` | 71.4% (SQLite path requires real DB) |
| `GetSnapshot` | 52.6% (SQLite path requires real DB) |
| `DiffSnapshots` | 100% |
| `ForkRun` | 100% |
| `ReplayRun` | 100% |
| `msToTime` | 100% |
| `parseSnapshotsJSON` | 100% |
| `parseSnapshotJSON` | 100% |
| `parseSnapshotDiffJSON` | 100% |
| `parseForkReplayRunJSON` | 100% |
| `scanSnapshots` | 0% (SQLite-only, requires real DB — consistent with all other scan* helpers) |

The 0% on `scanSnapshots` matches the established pattern in this package: `scanApprovals`, `scanScoreRows`, `scanMemoryFacts`, `scanCronSchedules`, `scanRunSummaries`, `scanRunTasks`, and `scanSQLResult` all show 0% for the same reason.

---

## Files Created

- `/Users/williamcory/crush/internal/smithers/types_timetravel.go`
- `/Users/williamcory/crush/internal/smithers/timetravel.go`
- `/Users/williamcory/crush/internal/smithers/timetravel_test.go`
- `/Users/williamcory/crush/.smithers/specs/implementation/eng-time-travel-api-and-model.md` (this file)

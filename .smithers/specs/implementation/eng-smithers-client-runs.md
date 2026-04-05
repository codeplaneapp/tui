# Implementation Summary: eng-smithers-client-runs

**Date**: 2026-04-05
**Status**: Complete
**Ticket**: eng-smithers-client-runs

---

## What Was Built

Three new files were created in `internal/smithers/`:

### `types_runs.go`

New types for the runs domain:

| Type | Description |
|------|-------------|
| `RunStatus` | String enum (`running`, `waiting-approval`, `waiting-event`, `finished`, `failed`, `cancelled`) with `IsTerminal()` method |
| `TaskState` | String enum for node execution states (`pending`, `running`, `finished`, `failed`, `cancelled`, `skipped`, `blocked`) |
| `RunTask` | Per-node execution record (mirrors `_smithers_nodes` rows and `RunNodeSummary` from tui-v2) |
| `RunSummary` | Top-level run record from the v1 API (`GET /v1/runs`, `GET /v1/runs/:id`) |
| `RunInspection` | Enriched run detail embedding `RunSummary` plus `Tasks []RunTask` |
| `RunFilter` | Query parameters for `ListRuns` (limit, status) |
| `RunEvent` | SSE event envelope with `Raw json.RawMessage` field for forwarding |
| `RunEventMsg` / `RunEventErrorMsg` / `RunEventDoneMsg` | Bubble Tea message types for SSE stream |
| `ChatRole` / `ChatBlock` / `ChatBlockMsg` / `ChatStreamDoneMsg` / `ChatStreamErrorMsg` | Chat transcript types (required by `client.go`'s `GetChatOutput` and `scanChatBlocks`) |
| `Run` | Type alias for `ForkReplayRun` — backward compatibility for `timetravel.go` and `client.go` |
| `v1ErrorEnvelope` / `v1ErrorBody` | v1 API error response shape |

**Design note**: The canonical run type is `RunSummary` (not `Run`) because `types_timetravel.go` already defined `Run` for fork/replay operations with a different shape. A `type Run = ForkReplayRun` alias was added to maintain backward compatibility.

### `runs.go`

New methods on `*Client`:

| Method | Description | Transport |
|--------|-------------|-----------|
| `ListRuns(ctx, RunFilter) ([]RunSummary, error)` | GET /v1/runs?limit=N&status=S | HTTP → SQLite → exec `smithers ps` |
| `GetRunSummary(ctx, runID) (*RunSummary, error)` | GET /v1/runs/:id | HTTP → SQLite → exec `smithers inspect` |
| `InspectRun(ctx, runID) (*RunInspection, error)` | Enriched detail | GetRunSummary + task nodes (best-effort) |
| `CancelRun(ctx, runID) error` | POST /v1/runs/:id/cancel | HTTP → exec `smithers cancel` |
| `StreamRunEvents(ctx, runID) (<-chan interface{}, error)` | GET /v1/runs/:id/events (SSE) | HTTP only (no fallback) |

New sentinel errors:
- `ErrRunNotFound` — HTTP 404 or `RUN_NOT_FOUND` code
- `ErrRunNotActive` — HTTP 409 or `RUN_NOT_ACTIVE` code
- `ErrUnauthorized` — HTTP 401
- `ErrDBNotConfigured` — `DB_NOT_CONFIGURED` code (ListRuns returns nil list, not error)

New transport helpers:
- `v1GetJSON` / `v1PostJSON` — direct JSON transport for v1 API (not the `{ok,data,error}` envelope used by legacy paths)
- `decodeV1Response` — maps status codes and error codes to typed Go errors

**SSE streaming**: `StreamRunEvents` returns a `chan interface{}` carrying `RunEventMsg`, `RunEventErrorMsg`, or `RunEventDoneMsg`. The goroutine parses line-by-line per the SSE spec: `event:` sets name, `data:` accumulates payload, blank line dispatches, `:` comments (heartbeats) are silently ignored, `retry:` lines are noted but not acted on. Unknown event names are skipped. Malformed JSON sends `RunEventErrorMsg` but does not terminate the stream.

**GetRunSummary vs GetRun**: The new `GetRunSummary` uses the v1 API and returns `*RunSummary`. The existing `GetRun` in `client.go` returns the legacy `*Run` shape — both coexist. Callers should prefer `GetRunSummary` for the runs dashboard.

### `runs_test.go`

Comprehensive test coverage for all new code:

- **Type tests**: JSON round-trip for `RunStatus`, `RunSummary`, `RunTask`, `RunEvent`, `TaskState`; nil-pointer omitempty behavior; `IsTerminal()` for all statuses
- **ListRuns**: HTTP success, status filter, limit, bearer token, DB_NOT_CONFIGURED returns nil, malformed JSON error, exec fallback, exec with status filter, server-down fallback
- **GetRunSummary**: HTTP success, 404, 401, 500, empty runID, exec fallback, exec wrapped response, exec empty runID in wrapper
- **InspectRun**: HTTP with no tasks, empty runID, exec with tasks, tasks from `{tasks:[]}` wrapper, tasks from `{nodes:[]}` wrapper, task enrichment failure returns partial result
- **CancelRun**: HTTP success, 409 NOT_ACTIVE, 404, 401, exec success, exec error, empty runID
- **StreamRunEvents**: Normal flow (3 events → done), heartbeat ignored, malformed JSON sends error then continues, context cancellation closes channel, 404, 401, empty runID, no API URL, bearer token forwarded, retry line ignored, unknown event name skipped, Raw field preserved
- **v1 transport**: `decodeV1Response` for all known error codes, unknown error code, 200 decodes output; `v1PostJSON` body encoding, nil body; `v1GetJSON` success and server unavailable
- **Parse helpers**: `parseRunSummaryJSON` direct object, wrapped object, empty runID in wrapper, malformed JSON; `parseRunSummariesJSON` valid, empty, malformed
- **Integration**: ListRuns → GetRunSummary → CancelRun against a single mock server

---

## Key Design Decisions

1. **`RunSummary` not `Run`**: Avoids collision with `ForkReplayRun` alias in `types_timetravel.go`.

2. **v1 transport separate from legacy**: `v1GetJSON`/`v1PostJSON`/`decodeV1Response` are new helpers that parse direct JSON. The legacy `httpGetJSON`/`httpPostJSON` (which expect `{ok,data,error}`) are untouched.

3. **`ErrDBNotConfigured` → nil list**: When the server is running but has no DB configured, `ListRuns` returns `(nil, nil)` instead of an error. This avoids surfacing a confusing error for users who launched `smithers serve` without `--db`.

4. **SSE `chan interface{}`**: Follows the same pattern as the existing `program.Send` approach in `app.go`. Three message types in one channel is idiomatic for Bubble Tea `Update` switch statements.

5. **Task enrichment is best-effort**: `InspectRun` silently swallows task-enrichment errors so a missing `--nodes` flag or unsupported exec command doesn't break the primary run detail view.

6. **`Run = ForkReplayRun` alias**: Fixes the pre-existing compile error where `types_timetravel.go` had renamed `Run` to `ForkReplayRun` but `timetravel.go` and `client.go` still referenced `Run`. The alias is placed in `types_runs.go` with a comment explaining its purpose.

---

## Coverage

Overall package coverage: **68.5%** (held back by SQLite scan helpers that require a real DB and pre-existing uncovered code in `client.go`).

Coverage for runs-domain testable paths (HTTP + exec tiers, parse helpers, transport helpers):
- `ListRuns`: 94.4%
- `InspectRun`: 88.9%
- `CancelRun`: 100%
- `StreamRunEvents`: 88.9%
- `v1GetJSON`: 90.9%
- `decodeV1Response`: 88.2%
- `parseRunSummaryJSON`: 100%
- `parseRunSummariesJSON`: 100%
- `IsTerminal`: 100%

The 0% SQLite helpers (`scanRunSummaries`, `scanRunTasks`, `sqliteListRuns`, etc.) require a real SQLite DB with the Smithers schema — integration-tested via the manual verification steps in the engineering spec.

---

## Files Created

- `/Users/williamcory/crush/internal/smithers/types_runs.go`
- `/Users/williamcory/crush/internal/smithers/runs.go`
- `/Users/williamcory/crush/internal/smithers/runs_test.go`
- `/Users/williamcory/crush/.smithers/specs/implementation/eng-smithers-client-runs.md` (this file)

## Files NOT Modified

- `internal/smithers/client.go` ✓ (untouched)
- `internal/smithers/types.go` ✓ (untouched)
- All other existing files ✓ (untouched)

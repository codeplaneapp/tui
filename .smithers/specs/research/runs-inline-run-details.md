## Existing Crush Surface

- `RunsView` is implemented in [internal/ui/views/runs.go](/Users/williamcory/crush/internal/ui/views/runs.go). It holds `runs []smithers.RunSummary`, a `cursor int`, and calls `components.RunTable.View()` for rendering. The `Enter` keypress is explicitly a no-op (`// No-op for now; future: drill into run inspector`) at [runs.go:98](/Users/williamcory/crush/internal/ui/views/runs.go#L98).
- `RunTable` is implemented in [internal/ui/components/runtable.go](/Users/williamcory/crush/internal/ui/components/runtable.go). It renders section headers (ACTIVE / COMPLETED / FAILED), a column header row, and one line per run. It is stateless — `View()` takes `Runs`, `Cursor`, and `Width` and returns a string. There is no provision for variable-height rows or detail lines.
- `RunSummary` is defined in [internal/smithers/types_runs.go](/Users/williamcory/crush/internal/smithers/types_runs.go#L52). It carries `RunID`, `WorkflowName`, `WorkflowPath`, `Status`, `StartedAtMs`, `FinishedAtMs`, `Summary map[string]int`, and `ErrorJSON *string`. It has no agent/node name fields — those require `RunInspection`.
- `RunInspection` is defined in [internal/smithers/types_runs.go](/Users/williamcory/crush/internal/smithers/types_runs.go#L66). It embeds `RunSummary` and adds `Tasks []RunTask`. `RunTask` carries `NodeID`, `Label *string`, `Iteration int`, `State TaskState`, `LastAttempt *int`, and `UpdatedAtMs *int64`.
- `InspectRun` is implemented in [internal/smithers/runs.go](/Users/williamcory/crush/internal/smithers/runs.go#L200). It calls `GetRunSummary` then `getRunTasks` (SQLite preferred, exec fallback). It returns a `*RunInspection`.
- `RunStatus.IsTerminal()` is available on [types_runs.go:17](/Users/williamcory/crush/internal/smithers/types_runs.go#L17) — returns `true` for `finished`, `failed`, `cancelled`.
- `statusStyle()` in `runtable.go` already maps `waiting-approval` to yellow-bold — re-usable for the approval detail line variant.
- `fmtElapsed()` in [runtable.go:111](/Users/williamcory/crush/internal/ui/components/runtable.go#L111) computes human-readable elapsed time from `RunSummary` — directly usable in the `finished`/`cancelled` detail line.
- `partitionRuns()` in [runtable.go:36](/Users/williamcory/crush/internal/ui/components/runtable.go#L36) builds the `[]runVirtualRow` list. The `navigableIdx` counter in `View()` is the bridge between `RunTable.Cursor` (navigable-row index, run rows only) and the actual `RunSummary` in `RunTable.Runs`. This mapping must be preserved when inserting detail rows.
- Existing `runtable_test.go` at [internal/ui/components/runtable_test.go](/Users/williamcory/crush/internal/ui/components/runtable_test.go) covers section partitioning, cursor highlight, progress/elapsed columns, and empty-state rendering. These tests provide the baseline to extend.
- `RunStatusWaitingApproval`, `RunStatusRunning`, `RunStatusWaitingEvent`, `RunStatusFailed`, `RunStatusFinished`, `RunStatusCancelled` are all defined at [types_runs.go:8](/Users/williamcory/crush/internal/smithers/types_runs.go#L8).
- `types_runs_test.go` at [internal/smithers/types_runs_test.go](/Users/williamcory/crush/internal/smithers/types_runs_test.go) has JSON round-trip and status-enum tests — a natural home for `TestRunSummaryErrorReason`.

## Upstream Smithers Reference

- The design doc's Run Dashboard wireframe at [02-DESIGN.md:144](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md#L144) shows the exact inline detail format:
  - Active: `└─ claude-code agent on "review auth module"`
  - Approval: `└─ ⏸ APPROVAL PENDING: "Deploy to staging?"   [a]pprove`
  - Failed: `└─ Error: Schema validation failed at "update-lockfile"`
- The PRD's §6.2 Run Dashboard feature list at [01-PRD.md:133](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md#L133) explicitly lists "Inline details: Expand a run to see nodes, current step, elapsed time."
- Ticket acceptance criteria from [runs-and-inspection.json:155](/Users/williamcory/crush/.smithers/specs/ticket-groups/runs-and-inspection.json#L155):
  - Active runs show the agent executing them
  - Pending runs show the approval gate question
  - Failed runs show the error reason
- Ticket implementation note at [runs-and-inspection.json:168](/Users/williamcory/crush/.smithers/specs/ticket-groups/runs-and-inspection.json#L168): "Requires dynamic row height or a multi-line row rendering approach."
- `HijackSession` from [types_runs.go:197](/Users/williamcory/crush/internal/smithers/types_runs.go#L197) carries `AgentEngine` and `AgentBinary` — this is the richer agent-name source available from a hijack call. For inline details, the agent engine name from `RunTask` or a fallback to `HijackSession.AgentEngine` from a pre-fetched hijack is more appropriate than triggering a hijack to read the agent name.
- The `runs-quick-approve` ticket at [runs-and-inspection.json:271](/Users/williamcory/crush/.smithers/specs/ticket-groups/runs-and-inspection.json#L271) explicitly depends on `runs-inline-run-details`, confirming the approval detail line is the visual surface that makes `[a]pprove` discoverable.
- Agent name in `RunTask`: the `Label` field on `RunTask` holds the node label (e.g. `"review auth module"`), not the agent engine name. Agent engine name (e.g. `"claude-code"`) is not directly in `RunSummary` or `RunTask` in the current type set. It would require either a separate field on `RunSummary` (not yet present) or looking at `HijackSession.AgentEngine` (requires a server call). For v1, the detail line for `running` runs can show the node label from the active `RunTask` and omit the engine name, or display a placeholder like `Running: <nodeLabel>`.

## Gaps

- **No agent engine name in RunSummary or RunTask**: `RunSummary` and `RunTask` do not carry an `agentEngine` field. The design doc shows `claude-code agent on "review auth module"`, but the engine name is only accessible via `HijackSession` (requires a POST /v1/runs/:id/hijack call) or from the upstream `_smithers_nodes` table if it stores the agent type. For this ticket, the detail line for running tasks should degrade gracefully: `└─ Running: "<nodeLabel>"` or omit the engine prefix until the data is available.
- **No gate question in RunSummary**: For `waiting-approval` runs, the gate question is not a first-class field in `RunSummary`. It may be embedded in `ErrorJSON` (which holds structured JSON), or it may only be available by fetching the specific approval node via `InspectRun`. The `ErrorJSON` field on a `waiting-approval` run may be `nil`; the detail line should fall back to `APPROVAL PENDING` with no question text if unavailable.
- **RunTable is stateless**: Adding `Expanded`/`Inspections` fields changes `RunTable` from a data struct to a view-state-carrying struct. This is consistent with existing usage (it already holds `Cursor`) but should be documented.
- **Cursor-to-RunID mapping**: `RunsView` tracks `cursor` as a navigable-row index and passes it to `RunTable`. The view needs a `selectedRun()` method that replicates the `navigableIdx` logic from `RunTable.View()` to resolve the cursor to a `RunSummary`. Currently no such helper exists.
- **No runs_test.go for RunsView**: `internal/ui/views/runs_test.go` does not exist. It must be created for this ticket.

## Recommended Direction

- Add `expanded map[string]bool` and `inspections map[string]*smithers.RunInspection` to `RunsView`. Keep the map nil-safe (`make` on first use or in constructor).
- Add `selectedRun() smithers.RunSummary` to `RunsView` by replicating the `partitionRuns` + `navigableIdx` walk — or expose a `RunAtCursor(cursor int) (smithers.RunSummary, bool)` method on `RunTable` to avoid duplicating logic.
- Extend `RunTable` with `Expanded map[string]bool` and `Inspections map[string]*smithers.RunInspection`. In `View()`, after each `runRowKindRun` row, check `t.Expanded[run.RunID]` and write the detail line if true.
- Implement `fmtDetailLine(run smithers.RunSummary, insp *smithers.RunInspection) string` as a package-level function in `runtable.go` — keeps the rendering logic testable independently.
- Add `ErrorReason() string` to `RunSummary` in `types_runs.go` to encapsulate `ErrorJSON` parsing. This prevents JSON unmarshaling from leaking into the rendering layer.
- For the agent engine name gap: render `└─ Running: "<nodeLabel>"` using the first task with `State == running` from `insp.Tasks`. If inspection not yet loaded, show `└─ Running…` as a loading placeholder.
- For the gate question gap: parse `ErrorJSON` if present; if it contains `{"message":"..."}` use that; otherwise fall back to `APPROVAL PENDING` with no question text.
- Keep `Enter` as a pure toggle for this ticket. The full inspector navigation (`runs-inspect-summary`) is a separate ticket that will reuse the `expanded` state as a precursor.

## Files To Touch
- [internal/ui/views/runs.go](/Users/williamcory/crush/internal/ui/views/runs.go) — add `expanded`/`inspections`, `Enter` toggle, `fetchInspection` cmd, `selectedRun` helper, `runInspectionMsg` type
- [internal/ui/components/runtable.go](/Users/williamcory/crush/internal/ui/components/runtable.go) — add `Expanded`/`Inspections` fields, `fmtDetailLine` function, detail-line rendering in `View()`
- [internal/smithers/types_runs.go](/Users/williamcory/crush/internal/smithers/types_runs.go) — add `ErrorReason()` method on `RunSummary`
- `/Users/williamcory/crush/internal/ui/views/runs_test.go` (new) — expand/collapse message cycle tests
- [internal/ui/components/runtable_test.go](/Users/williamcory/crush/internal/ui/components/runtable_test.go) — extend with detail-line rendering assertions
- [internal/smithers/types_runs_test.go](/Users/williamcory/crush/internal/smithers/types_runs_test.go) — add `TestRunSummaryErrorReason`
- `/Users/williamcory/crush/tests/vhs/runs-inline-run-details.tape` (new) — VHS happy-path recording

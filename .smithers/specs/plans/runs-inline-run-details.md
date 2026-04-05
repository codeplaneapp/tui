# Implementation Plan: runs-inline-run-details

## Goal
Add expandable inline detail rows to the Run Dashboard. Pressing `Enter` on any run toggles a context-sensitive second line below the run row — showing the active node/agent for running runs, the gate question for approval-pending runs, or the error reason for failed runs — without navigating away from the list or performing any additional data fetches beyond what is already loaded.

## Steps

1. **Add `ErrorReason()` to `RunSummary`**
   - In `internal/smithers/types_runs.go`, add a method `ErrorReason() string` that parses the `ErrorJSON` pointer.
   - Try to unmarshal as `{"message":"..."}` first; fall back to the raw string trimmed to 80 chars; return `""` when `ErrorJSON` is nil.
   - Add `TestRunSummaryErrorReason` in `internal/smithers/types_runs_test.go` with table-driven cases: nil, raw string, JSON `{message}`, JSON without `message` key.

2. **Add `fmtDetailLine` to `runtable.go`**
   - In `internal/ui/components/runtable.go`, add package-level `fmtDetailLine(run smithers.RunSummary, insp *smithers.RunInspection, width int) string`.
   - Status dispatch table:
     - `running`: find first `RunTask` with `State == running` in `insp.Tasks`; render `└─ Running: "<*task.Label>"` (omit label if nil or insp nil, use `└─ Running…` as placeholder).
     - `waiting-approval`: render `└─ ⏸ APPROVAL PENDING: "<ErrorReason()>"   [a]pprove / [d]eny`; if `ErrorReason()` is empty, omit the colon+question.
     - `waiting-event`: render `└─ ⏳ Waiting for external event`.
     - `failed`: render `└─ ✗ Error: <ErrorReason()>`; fall back to `└─ ✗ Failed` if empty.
     - `finished` / `cancelled`: render `└─ Completed in <fmtElapsed(run)>`.
   - Indent: 4-space prefix (aligns under the workflow name column past cursor + ID).
   - Style: faint for all variants except `waiting-approval` which uses the existing yellow-bold `statusStyle(RunStatusWaitingApproval)` for the `⏸ APPROVAL PENDING` prefix.
   - Add table-driven tests in `runtable_test.go` for each status variant, both with and without `insp` being nil.

3. **Extend `RunTable` with `Expanded` and `Inspections` fields**
   - Add `Expanded map[string]bool` and `Inspections map[string]*smithers.RunInspection` to the `RunTable` struct in `runtable.go`.
   - In `View()`, immediately after writing each `runRowKindRun` line, check `t.Expanded[run.RunID]`. If true, write `"\n" + fmtDetailLine(run, t.Inspections[run.RunID], t.Width)`.
   - No change to `navigableIdx` counting — detail lines are not navigable rows.
   - No change to section headers, dividers, or column header rendering.

4. **Add expand/collapse state to `RunsView`**
   - Add to `RunsView` in `runs.go`:
     ```go
     expanded    map[string]bool
     inspections map[string]*smithers.RunInspection
     ```
   - Initialize both maps in `NewRunsView`.
   - Add `runInspectionMsg` type:
     ```go
     type runInspectionMsg struct {
         runID      string
         inspection *smithers.RunInspection
     }
     ```
   - Add `fetchInspection(runID string) tea.Cmd` method that calls `v.client.InspectRun` in a goroutine and returns `runInspectionMsg`.
   - Add `selectedRunID() string` helper: replicate the `partitionRuns` + `navigableIdx` walk to resolve `v.cursor` to a `RunID`. Return `""` if runs is empty.

5. **Wire `Enter` toggle in `RunsView.Update`**
   - Replace the existing no-op `enter` case with:
     ```go
     case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
         id := v.selectedRunID()
         if id == "" {
             break
         }
         if v.expanded[id] {
             delete(v.expanded, id)
         } else {
             v.expanded[id] = true
             if _, cached := v.inspections[id]; !cached {
                 run := v.selectedRunSummary()
                 if !run.Status.IsTerminal() {
                     return v, v.fetchInspection(id)
                 }
             }
         }
     ```
   - Handle `runInspectionMsg` in `Update`: store `msg.inspection` in `v.inspections[msg.runID]` (even if nil — prevents repeated fetch attempts).

6. **Pass expanded/inspections to RunTable in `RunsView.View`**
   - Update the `RunTable` construction:
     ```go
     table := components.RunTable{
         Runs:        v.runs,
         Cursor:      v.cursor,
         Width:       v.width,
         Expanded:    v.expanded,
         Inspections: v.inspections,
     }
     ```

7. **Update `ShortHelp`**
   - Change the `Enter` binding help text from `"inspect"` to `"toggle details"`.

8. **Unit tests for `RunsView`**
   - Create `internal/ui/views/runs_test.go`.
   - Test cases:
     - After `runsLoadedMsg`, `v.runs` is populated and `v.loading` is false.
     - `Enter` on cursor 0 sets `v.expanded[runs[0].RunID] = true` and returns a cmd (non-nil) when run is active and not yet inspected.
     - Second `Enter` on same cursor deletes the key from `v.expanded`.
     - `runInspectionMsg` stores the inspection in `v.inspections`.
     - `Enter` on a terminal run sets `v.expanded` but returns `nil` cmd (no fetch).

9. **VHS happy-path recording**
   - Create `tests/vhs/runs-inline-run-details.tape`.
   - Sequence: launch TUI → `/runs` → wait for list → `Down` to first run → `Enter` → sleep to show detail → `Enter` again to collapse → `Esc` → quit.

## File Plan
- `internal/smithers/types_runs.go` — add `ErrorReason()` method
- `internal/smithers/types_runs_test.go` — add `TestRunSummaryErrorReason`
- `internal/ui/components/runtable.go` — add `Expanded`/`Inspections` fields, `fmtDetailLine`, detail rendering in `View()`
- `internal/ui/components/runtable_test.go` — extend with detail-line tests
- `internal/ui/views/runs.go` — add `expanded`/`inspections`, `runInspectionMsg`, `fetchInspection`, `selectedRunID`, `selectedRunSummary`, `Enter` toggle, updated `ShortHelp`
- `internal/ui/views/runs_test.go` (new) — RunsView unit tests
- `tests/vhs/runs-inline-run-details.tape` (new) — VHS happy-path recording

## Validation
1. `gofumpt -w internal/smithers internal/ui/components internal/ui/views`
2. `go build ./...`
3. `go test ./internal/smithers/... -run TestRunSummaryErrorReason -v -count=1`
4. `go test ./internal/ui/components/... -run TestRunTable -v -count=1`
5. `go test ./internal/ui/views/... -run TestRunsView -v -count=1`
6. `go test ./... -count=1`
7. `vhs tests/vhs/runs-inline-run-details.tape`
8. Manual smoke:
   - `go run .` → Ctrl+R or `/runs` → select an active run → `Enter` → verify `└─ Running: "..."` appears indented below the row
   - `Enter` again on same run → detail line disappears
   - Navigate to a `waiting-approval` run → `Enter` → verify `⏸ APPROVAL PENDING` line in yellow
   - Navigate to a `failed` run → `Enter` → verify `✗ Error:` with reason text
   - `Esc` → returns to chat

## Open Questions
1. `selectedRunID()` must replicate the `partitionRuns` + `navigableIdx` walk from `RunTable.View()`. If this logic diverges it will cause cursor mismatches. Consider extracting a shared `RunAtCursor(runs []RunSummary, cursor int) (RunSummary, bool)` function in `runtable.go` used by both `View()` and `RunsView.selectedRunID()` — should this refactor be part of this ticket or a follow-on?
2. If `InspectRun` takes longer than 1–2 seconds (e.g. CLI fallback), the detail row shows a loading placeholder until the cmd returns. Should the expand state be deferred (show placeholder only, not `v.expanded[id] = true`) until data arrives, or should the placeholder be shown immediately? Showing immediately (optimistic) is simpler and matches the design doc aesthetic.
3. The `runs-inspect-summary` ticket says `Enter` pushes the inspector view. Once that ticket ships, `Enter` semantics will change. The cleanest migration path: first `Enter` expands inline details; second `Enter` (when already expanded) navigates to inspector. This ticket should leave a comment at the `Enter` handler indicating the planned evolution.

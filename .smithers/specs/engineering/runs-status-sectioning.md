# Engineering Spec: Run Dashboard Status Sectioning

## Metadata
- Ticket: `runs-status-sectioning`
- Feature: `RUNS_STATUS_SECTIONING`
- Group: Runs And Inspection
- Dependencies: `runs-dashboard` (RunsView, RunTable, RunSummary types must be present)
- Target files:
  - `internal/ui/views/runs.go` (modify — cursor navigation logic)
  - `internal/ui/components/runtable.go` (modify — section rendering)
  - `internal/ui/components/runtable_test.go` (modify — section tests)
  - `tests/vhs/runs-status-sectioning.tape` (new — VHS recording)

---

## Objective

Enhance the Run Dashboard to visually group runs into three status-based
sections — **Active**, **Completed**, and **Failed** — separated by styled
section headers that display run counts. Cursor navigation with `↑`/`↓`/`j`/`k`
traverses only run rows; section headers are non-selectable presentation
elements. This matches the design wireframe at `docs/smithers-tui/02-DESIGN.md:142-172`
and achieves GUI parity with the `RunsList.tsx` grouped layout.

---

## Scope

### In scope
- Partition `[]smithers.RunSummary` into three groups at render time: Active,
  Completed, Failed
- Section headers with run counts (`● ACTIVE (3)`, `● COMPLETED (12)`, `● FAILED (1)`)
- Horizontal dividers between non-empty sections
- Cursor navigation that skips section header rows (only run rows are selectable)
- Empty-section suppression (no header rendered if a section has zero runs)
- Unit tests for partition logic and section rendering
- VHS recording tape for the happy path

### Out of scope
- Collapsible sections (toggling section visibility — a separate enhancement)
- Status filtering by section (`RUNS_FILTER_BY_STATUS`)
- Real-time SSE updates (`RUNS_REALTIME_STATUS_UPDATES`)
- Inline run detail expansion
- Progress bar visualization
- Quick actions (approve, cancel, hijack)

---

## Implementation Plan

### Slice 1: Define virtual row types in runtable.go

**File**: `internal/ui/components/runtable.go`

Replace the current flat iteration with a virtual row model. A virtual row is
either a section header or a run data row. The cursor addresses only run rows.

Add these unexported types at the top of the file:

```go
// runRowKind distinguishes header rows from selectable run rows in the
// virtual row list used for section-aware rendering and cursor navigation.
type runRowKind int

const (
    runRowKindHeader runRowKind = iota
    runRowKindRun
)

// runVirtualRow is one entry in the virtual row list built by RunTable.View().
// Header rows have a non-empty sectionLabel and a zero runIdx.
// Run rows have runIdx set to the index into RunTable.Runs.
type runVirtualRow struct {
    kind         runRowKind
    sectionLabel string // only for runRowKindHeader
    runIdx       int    // only for runRowKindRun
}
```

These types are unexported; they are implementation details of `runtable.go`.

**Verification**: `go build ./internal/ui/components/...` passes.

---

### Slice 2: Add partition function

**File**: `internal/ui/components/runtable.go`

Add a package-level helper that partitions a `[]smithers.RunSummary` into three
ordered groups and builds the virtual row list:

```go
// partitionRuns returns a virtual row list with section headers followed by
// run rows. Sections with zero runs are omitted. Section order is:
// Active (running | waiting-approval | waiting-event),
// Completed (finished | cancelled),
// Failed (failed).
func partitionRuns(runs []smithers.RunSummary) []runVirtualRow {
    type section struct {
        label string
        idxs  []int
    }
    sections := []section{
        {label: "ACTIVE"},
        {label: "COMPLETED"},
        {label: "FAILED"},
    }

    for i, r := range runs {
        switch r.Status {
        case smithers.RunStatusRunning,
             smithers.RunStatusWaitingApproval,
             smithers.RunStatusWaitingEvent:
            sections[0].idxs = append(sections[0].idxs, i)
        case smithers.RunStatusFinished,
             smithers.RunStatusCancelled:
            sections[1].idxs = append(sections[1].idxs, i)
        case smithers.RunStatusFailed:
            sections[2].idxs = append(sections[2].idxs, i)
        }
    }

    var rows []runVirtualRow
    first := true
    for _, sec := range sections {
        if len(sec.idxs) == 0 {
            continue
        }
        label := fmt.Sprintf("● %s (%d)", sec.label, len(sec.idxs))
        if !first {
            // sentinel for divider before non-first sections
            rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: ""})
        }
        rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: label})
        for _, idx := range sec.idxs {
            rows = append(rows, runVirtualRow{kind: runRowKindRun, runIdx: idx})
        }
        first = false
    }
    return rows
}
```

The empty `sectionLabel` sentinel row renders as a faint horizontal divider
(`strings.Repeat("─", width)`).

**Verification**: Unit test in `runtable_test.go` (see Slice 5).

---

### Slice 3: Rewrite RunTable.View() to use virtual rows

**File**: `internal/ui/components/runtable.go`

Replace the current linear loop with a virtual row loop. The `Cursor` field on
`RunTable` is now a **navigable-row index** — it counts only `runRowKindRun`
rows, not header rows.

```go
// View renders the run table with status sections as a string.
func (t RunTable) View() string {
    var b strings.Builder

    showProgress := t.Width >= 80
    showTime := t.Width >= 80

    // Column widths (unchanged from baseline).
    const (
        cursorW   = 2
        idW       = 8
        statusW   = 18
        progressW = 7
        timeW     = 9
        gapW      = 2
    )
    fixed := cursorW + idW + gapW + statusW + gapW
    if showProgress { fixed += progressW + gapW }
    if showTime     { fixed += timeW + gapW }
    workflowW := t.Width - fixed
    if workflowW < 8 { workflowW = 8 }

    faint := lipgloss.NewStyle().Faint(true)
    sectionHeaderStyle := lipgloss.NewStyle().Bold(true)
    dividerStyle := lipgloss.NewStyle().Faint(true)

    // Column header row (unchanged).
    header := faint.Render(fmt.Sprintf("  %-*s  %-*s  %-*s", idW, "ID", workflowW, "Workflow", statusW, "Status"))
    if showProgress { header += faint.Render(fmt.Sprintf("  %-*s", progressW, "Nodes")) }
    if showTime     { header += faint.Render(fmt.Sprintf("  %-*s", timeW, "Time")) }
    b.WriteString(header + "\n")

    // Build virtual rows and iterate.
    rows := partitionRuns(t.Runs)
    navigableIdx := -1 // counts runRowKindRun rows seen so far

    for _, row := range rows {
        switch row.kind {
        case runRowKindHeader:
            if row.sectionLabel == "" {
                // divider sentinel
                b.WriteString(dividerStyle.Render(strings.Repeat("─", t.Width)) + "\n")
            } else {
                b.WriteString("\n" + sectionHeaderStyle.Render(row.sectionLabel) + "\n\n")
            }

        case runRowKindRun:
            navigableIdx++
            run := t.Runs[row.runIdx]

            cursor := "  "
            idStyle := lipgloss.NewStyle()
            if navigableIdx == t.Cursor {
                cursor = "▸ "
                idStyle = idStyle.Bold(true)
            }

            runID := run.RunID
            if len(runID) > idW { runID = runID[:idW] }

            workflow := run.WorkflowName
            if workflow == "" { workflow = run.WorkflowPath }
            if len(workflow) > workflowW {
                if workflowW > 3 { workflow = workflow[:workflowW-3] + "..." } else { workflow = workflow[:workflowW] }
            }

            statusStr := string(run.Status)
            styledStatus := statusStyle(run.Status).Render(fmt.Sprintf("%-*s", statusW, statusStr))

            line := fmt.Sprintf("%s%-*s  %-*s  %s",
                cursor, idW, idStyle.Render(runID), workflowW, workflow, styledStatus)
            if showProgress { line += fmt.Sprintf("  %-*s", progressW, fmtProgress(run)) }
            if showTime     { line += fmt.Sprintf("  %-*s", timeW, fmtElapsed(run)) }
            b.WriteString(line + "\n")
        }
    }

    return b.String()
}
```

**Verification**: Existing `runtable_test.go` tests still pass (cursor still
selects the correct run row). New section tests pass (see Slice 5).

---

### Slice 4: Update cursor bounds in RunsView.Update()

**File**: `internal/ui/views/runs.go`

The cursor now addresses navigable rows (run rows only), not the raw slice.
The upper bound must be `(number of run rows) - 1`, not `len(v.runs) - 1`.
Since all elements in `v.runs` are run rows (headers are virtual, not stored),
the upper bound is still `len(v.runs) - 1`. **No change is required** — the
cursor arithmetic in `runs.go` is already correct.

However, if a future enhancement stores headers in the runs slice, this
section would need updating. Add a comment to `runs.go` near the cursor
clamping logic:

```go
// cursor is a navigable-row index (counts only run rows, not section headers).
// Since v.runs contains only RunSummary values (no header entries), len(v.runs)-1
// is the correct upper bound.
if v.cursor < len(v.runs)-1 {
    v.cursor++
}
```

**Verification**: Navigate up/down across a mixed-status list in the TUI.
Cursor never stops on a header row. Cursor stops at first run (up from top)
and last run (down from bottom) correctly.

---

### Slice 5: Unit tests

**File**: `internal/ui/components/runtable_test.go`

Add tests for `partitionRuns` and the sectioned `View()`:

```go
// TestPartitionRuns_SectionOrder verifies that Active runs appear before
// Completed before Failed, and empty sections are omitted.
func TestPartitionRuns_SectionOrder(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "a", Status: smithers.RunStatusFailed},
        {RunID: "b", Status: smithers.RunStatusRunning},
        {RunID: "c", Status: smithers.RunStatusFinished},
    }
    rows := partitionRuns(runs)

    // Expect: header(ACTIVE), run(b), divider, header(COMPLETED), run(c), divider, header(FAILED), run(a)
    labels := []string{}
    runIDs := []string{}
    for _, r := range rows {
        if r.kind == runRowKindHeader { labels = append(labels, r.sectionLabel) }
        if r.kind == runRowKindRun   { runIDs = append(runIDs, runs[r.runIdx].RunID) }
    }
    // section label order
    if labels[0] != "" || // first header is not a divider — adjust for your impl
       !strings.Contains(labels[0], "ACTIVE") { t.Errorf("first section should be ACTIVE") }
    // run order within sections
    if runIDs[0] != "b" { t.Errorf("first run should be b (running)") }
    if runIDs[1] != "c" { t.Errorf("second run should be c (finished)") }
    if runIDs[2] != "a" { t.Errorf("third run should be a (failed)") }
}

// TestPartitionRuns_EmptySectionOmitted verifies no header appears for a
// section with zero runs.
func TestPartitionRuns_EmptySectionOmitted(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "x", Status: smithers.RunStatusRunning},
    }
    rows := partitionRuns(runs)
    for _, r := range rows {
        if r.kind == runRowKindHeader &&
           (strings.Contains(r.sectionLabel, "COMPLETED") ||
            strings.Contains(r.sectionLabel, "FAILED")) {
            t.Errorf("unexpected section header: %q", r.sectionLabel)
        }
    }
}

// TestRunTable_CursorCrossesSection verifies that navigable cursor index 1
// highlights the second run row even when a section header appears between them.
func TestRunTable_CursorCrossesSection(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "run1", WorkflowName: "wf-a", Status: smithers.RunStatusRunning},
        {RunID: "run2", WorkflowName: "wf-b", Status: smithers.RunStatusFailed},
    }
    table := RunTable{Runs: runs, Cursor: 1, Width: 120}
    out := table.View()
    // cursor indicator on run2, not on run1 or a header row
    if !strings.Contains(out, "▸ run2") {
        t.Errorf("expected cursor on run2; got:\n%s", out)
    }
    if strings.Contains(out, "▸ run1") {
        t.Errorf("unexpected cursor on run1; got:\n%s", out)
    }
}

// TestRunTable_SectionHeadersPresent verifies section header labels appear.
func TestRunTable_SectionHeadersPresent(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "r1", Status: smithers.RunStatusRunning},
        {RunID: "r2", Status: smithers.RunStatusFinished},
        {RunID: "r3", Status: smithers.RunStatusFailed},
    }
    out := RunTable{Runs: runs, Cursor: 0, Width: 120}.View()
    for _, label := range []string{"ACTIVE", "COMPLETED", "FAILED"} {
        if !strings.Contains(out, label) {
            t.Errorf("expected section label %q in output", label)
        }
    }
}
```

**Verification**: `go test ./internal/ui/components/ -run TestPartition -v` and
`go test ./internal/ui/components/ -run TestRunTable -v` all pass.

---

### Slice 6: VHS recording tape

**File**: `tests/vhs/runs-status-sectioning.tape`

```tape
# runs-status-sectioning.tape
# Records the runs dashboard with status sectioning: Active, Completed, Failed sections.
Output tests/vhs/output/runs-status-sectioning.gif
Set FontSize 14
Set Width 130
Set Height 40
Set Shell "bash"
Set Env CRUSH_GLOBAL_CONFIG tests/vhs/fixtures
Set Env SMITHERS_MOCK_RUNS "1"

Type "go run . --config tests/vhs/fixtures/crush.json"
Enter
Sleep 3s

# Navigate to runs dashboard
Ctrl+R
Sleep 2s

# Verify sections are visible (captured in recording)
# Scroll down through runs — cursor should skip section headers
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Up
Sleep 400ms

# Refresh
Type "r"
Sleep 2s

# Return to chat
Escape
Sleep 1s
```

**Fixture requirement**: The mock runs fixture (injected via `SMITHERS_MOCK_RUNS=1`
or a mock HTTP server in the E2E test) must include at least one run per section:
- `running` run — appears under ACTIVE
- `waiting-approval` run — appears under ACTIVE
- `finished` run — appears under COMPLETED
- `failed` run — appears under FAILED

**Verification**: `vhs validate tests/vhs/runs-status-sectioning.tape` exits 0.

---

## Validation

### Automated checks

| Check | Command | What it verifies |
|---|---|---|
| Build | `go build ./...` | All modifications compile without errors |
| Unit: partition | `go test ./internal/ui/components/ -run TestPartition -v` | Partition logic: order, empty-section suppression, count labels |
| Unit: table render | `go test ./internal/ui/components/ -run TestRunTable -v` | Section headers appear, cursor skips headers |
| Existing unit suite | `go test ./internal/ui/components/ -v` | No regressions in existing RunTable tests |
| Existing views suite | `go test ./internal/ui/views/ -v` | RunsView cursor navigation still correct |
| VHS validate | `vhs validate tests/vhs/runs-status-sectioning.tape` | Tape parses cleanly |

### Manual verification paths

1. **Mixed-status environment** (Smithers server with runs in multiple states):
   - Press `Ctrl+R` to open runs dashboard.
   - Verify ACTIVE section with count badge appears above running/waiting runs.
   - Verify COMPLETED section appears with count badge.
   - Verify FAILED section appears with count badge.
   - Press `↓` repeatedly — cursor never stops on a section header row.
   - Press `↑` from the first run in COMPLETED — cursor jumps directly to the
     last run in ACTIVE, skipping the divider and header.

2. **Single-section environment** (all runs finished, no active or failed):
   - Only the COMPLETED section header renders.
   - ACTIVE and FAILED headers do not appear.

3. **Narrow terminal** (< 80 columns):
   - Progress and Time columns are hidden (existing behavior preserved).
   - Section headers and dividers truncate or wrap gracefully.

### Acceptance criteria mapping

| Criterion | Verification |
|---|---|
| Runs are partitioned by status sections | Unit: `TestPartitionRuns_SectionOrder` passes |
| Sections map to the GUI parity layout | Manual: ACTIVE / COMPLETED / FAILED headers match `02-DESIGN.md:142-172` |
| List navigation correctly traverses between sections | Unit: `TestRunTable_CursorCrossesSection` + manual cross-section nav |

---

## Risks

### 1. Cursor index semantics change

**Impact**: `RunTable.Cursor` switches meaning from "index into `t.Runs`" to
"navigable-row index". Any caller that passes a cursor value derived from
`len(t.Runs)` without going through the RunsView navigation logic could
produce an off-by-one cursor position.

**Mitigation**: There is currently only one caller (`RunsView.View()` in
`runs.go`). The cursor is always managed by `RunsView.Update()` which clamps
to `len(v.runs)-1`. Since all elements in `v.runs` are run rows (no headers),
the clamping remains correct. Add a doc comment to `RunTable.Cursor` clarifying
the semantics.

**Severity**: Low — single caller, well-contained.

### 2. Divider width on narrow terminals

**Impact**: `strings.Repeat("─", t.Width)` renders a full-width divider. On
very narrow terminals (`t.Width == 0` before first `WindowSizeMsg`), this
produces an empty string rather than a visible separator.

**Mitigation**: Guard with `if t.Width > 0` or use a minimum width of 20.

**Severity**: Low — cosmetic only.

### 3. Section label colour decisions

**Impact**: The design doc uses `●` bullets but does not specify the colour of
the section header text. Using bold-faint (the ApprovalsView pattern) will look
different from the design wireframe which shows full-brightness headers.

**Mitigation**: Use `lipgloss.NewStyle().Bold(true)` (no faint) for section
labels to match the wireframe energy. Faint style is appropriate for column
header labels (ID, Workflow, Status), not section labels.

**Severity**: Low — purely cosmetic.

### 4. `cancelled` grouping in COMPLETED vs its own section

**Impact**: The design doc wireframe shows ACTIVE / COMPLETED TODAY / FAILED.
`cancelled` is not explicitly mentioned. Grouping it under COMPLETED is the
most intuitive choice (it is a terminal, non-error state) but some users may
expect a separate CANCELLED section.

**Mitigation**: Group `cancelled` under COMPLETED for v1 as described in this
spec. The partition function is trivially modified to add a CANCELLED section
in a follow-on ticket if user feedback demands it.

**Severity**: Low — UX preference, not a correctness issue.

---

## Files To Touch

- `/Users/williamcory/crush/internal/ui/components/runtable.go` — add virtual row types, `partitionRuns`, rewrite `View()`
- `/Users/williamcory/crush/internal/ui/components/runtable_test.go` — add section and partition unit tests
- `/Users/williamcory/crush/internal/ui/views/runs.go` — add clarifying comment on cursor semantics (no logic change)
- `/Users/williamcory/crush/tests/vhs/runs-status-sectioning.tape` — new VHS recording tape

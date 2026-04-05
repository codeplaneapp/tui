## Goal

Enhance the existing Run Dashboard (`internal/ui/views/runs.go` +
`internal/ui/components/runtable.go`) to visually group runs into three
status sections — **Active**, **Completed**, and **Failed** — with styled
section headers that show run counts and horizontal dividers between groups.
Cursor navigation with `↑`/`↓` skips section headers and traverses only
run rows. This achieves GUI parity with the grouped `RunsList.tsx` layout
shown in `docs/smithers-tui/02-DESIGN.md:142-172`.

This is a pure rendering/navigation enhancement. No new API calls, no new
types, no new view files. The full change lives in `runtable.go`,
`runtable_test.go`, and a one-line comment addition in `runs.go`.

Corresponds to ticket `runs-status-sectioning` (`RUNS_STATUS_SECTIONING`) in
`.smithers/specs/tickets.json` and the engineering spec at
`.smithers/specs/engineering/runs-status-sectioning.md`.

---

## Steps

### Step 1: Add virtual row types to runtable.go

**File**: `/Users/williamcory/crush/internal/ui/components/runtable.go`

Add two unexported types immediately after the package declaration and imports:

```go
type runRowKind int

const (
    runRowKindHeader runRowKind = iota
    runRowKindRun
)

type runVirtualRow struct {
    kind         runRowKind
    sectionLabel string // non-empty for header rows; empty string = divider sentinel
    runIdx       int    // index into RunTable.Runs; only meaningful for runRowKindRun
}
```

These types are purely internal to `runtable.go`. The public `RunTable` struct
signature does not change.

**Verification**: `go build ./internal/ui/components/...` passes with no new
errors.

---

### Step 2: Add partitionRuns helper function

**File**: `/Users/williamcory/crush/internal/ui/components/runtable.go`

Add after the `runVirtualRow` declaration:

```go
// partitionRuns builds the virtual row list for sectioned rendering.
// Section order: Active (running | waiting-approval | waiting-event),
// Completed (finished | cancelled), Failed (failed).
// Sections with zero runs are omitted. A divider sentinel row (empty
// sectionLabel) is inserted before each non-first section.
func partitionRuns(runs []smithers.RunSummary) []runVirtualRow {
    type sectionDef struct {
        label string
        idxs  []int
    }
    sections := []sectionDef{
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
        if !first {
            rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: ""})
        }
        label := fmt.Sprintf("● %s (%d)", sec.label, len(sec.idxs))
        rows = append(rows, runVirtualRow{kind: runRowKindHeader, sectionLabel: label})
        for _, idx := range sec.idxs {
            rows = append(rows, runVirtualRow{kind: runRowKindRun, runIdx: idx})
        }
        first = false
    }
    return rows
}
```

**Verification**: `go build ./internal/ui/components/...` still passes.

---

### Step 3: Rewrite RunTable.View() to iterate virtual rows

**File**: `/Users/williamcory/crush/internal/ui/components/runtable.go`

Replace the data-rows loop in the existing `View()` method (lines 132-182 of
the current file). Keep the column-width calculation, header row, and helper
calls (`fmtProgress`, `fmtElapsed`, `statusStyle`) unchanged. Only the
iteration changes.

In the existing `View()` method, replace the section starting at:
```go
// Data rows.
for i, run := range t.Runs {
```
...through the end of the loop (`}`) with:

```go
// Sectioned data rows.
sectionStyle := lipgloss.NewStyle().Bold(true)
dividerStyle := lipgloss.NewStyle().Faint(true)

rows := partitionRuns(t.Runs)
navigableIdx := -1

for _, row := range rows {
    switch row.kind {
    case runRowKindHeader:
        if row.sectionLabel == "" {
            // divider between sections
            divWidth := t.Width
            if divWidth < 1 { divWidth = 20 }
            b.WriteString(dividerStyle.Render(strings.Repeat("─", divWidth)) + "\n")
        } else {
            b.WriteString("\n" + sectionStyle.Render(row.sectionLabel) + "\n\n")
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
```

The column-width variables (`idW`, `workflowW`, `statusW`, `progressW`,
`timeW`, `showProgress`, `showTime`, `faint`) are already declared earlier in
`View()` and remain unchanged.

**Verification**: `go build ./...` passes. Manual run of the TUI: open runs
dashboard and verify sections appear with count badges and runs are grouped
correctly by status.

---

### Step 4: Add comment to runs.go about cursor semantics

**File**: `/Users/williamcory/crush/internal/ui/views/runs.go`

Add a single comment to the `down` key handler to document that `cursor` is a
navigable-row index (run rows only):

Locate the block:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
    if v.cursor < len(v.runs)-1 {
        v.cursor++
    }
```

Replace with:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
    // cursor is a navigable-row index (run rows only, not section headers).
    // len(v.runs)-1 is the correct upper bound because v.runs contains only
    // RunSummary values; section headers are virtual rows in RunTable.
    if v.cursor < len(v.runs)-1 {
        v.cursor++
    }
```

**Verification**: `go build ./...` passes. No logic changes.

---

### Step 5: Add unit tests

**File**: `/Users/williamcory/crush/internal/ui/components/runtable_test.go`

Add the following test functions. If the file does not yet exist, create it with
`package components` and the necessary imports (`testing`, `strings`,
`github.com/charmbracelet/crush/internal/smithers`).

```go
func TestPartitionRuns_SectionOrder(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "f1", Status: smithers.RunStatusFailed},
        {RunID: "r1", Status: smithers.RunStatusRunning},
        {RunID: "d1", Status: smithers.RunStatusFinished},
    }
    rows := partitionRuns(runs)
    var runOrder []string
    for _, row := range rows {
        if row.kind == runRowKindRun {
            runOrder = append(runOrder, runs[row.runIdx].RunID)
        }
    }
    want := []string{"r1", "d1", "f1"}
    for i, id := range want {
        if i >= len(runOrder) || runOrder[i] != id {
            t.Errorf("run order: got %v, want %v", runOrder, want)
            break
        }
    }
}

func TestPartitionRuns_EmptySectionOmitted(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "x", Status: smithers.RunStatusRunning},
    }
    rows := partitionRuns(runs)
    for _, r := range rows {
        if r.kind == runRowKindHeader && r.sectionLabel != "" {
            if strings.Contains(r.sectionLabel, "COMPLETED") ||
               strings.Contains(r.sectionLabel, "FAILED") {
                t.Errorf("unexpected section header %q for single-status input", r.sectionLabel)
            }
        }
    }
}

func TestRunTable_SectionHeadersPresent(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "r1", WorkflowName: "wf-run", Status: smithers.RunStatusRunning},
        {RunID: "d1", WorkflowName: "wf-done", Status: smithers.RunStatusFinished},
        {RunID: "f1", WorkflowName: "wf-fail", Status: smithers.RunStatusFailed},
    }
    out := RunTable{Runs: runs, Cursor: 0, Width: 120}.View()
    for _, label := range []string{"ACTIVE", "COMPLETED", "FAILED"} {
        if !strings.Contains(out, label) {
            t.Errorf("expected section label %q in View() output", label)
        }
    }
}

func TestRunTable_CursorCrossesSection(t *testing.T) {
    runs := []smithers.RunSummary{
        {RunID: "run1", WorkflowName: "first", Status: smithers.RunStatusRunning},
        {RunID: "run2", WorkflowName: "second", Status: smithers.RunStatusFailed},
    }
    // cursor=1 should land on run2 (the second navigable row)
    out := RunTable{Runs: runs, Cursor: 1, Width: 120}.View()
    if !strings.Contains(out, "▸") {
        t.Fatalf("no cursor indicator found in output")
    }
    // cursor line must contain "run2", not "run1"
    for _, line := range strings.Split(out, "\n") {
        if strings.Contains(line, "▸") {
            if !strings.Contains(line, "run2") {
                t.Errorf("cursor on wrong row; got: %q", line)
            }
        }
    }
}
```

**Verification**: `go test ./internal/ui/components/ -v` all tests pass including
both new and existing tests.

---

### Step 6: VHS recording tape

**File**: `tests/vhs/runs-status-sectioning.tape`

```tape
# runs-status-sectioning.tape — records the runs dashboard with grouped sections
Output tests/vhs/output/runs-status-sectioning.gif
Set FontSize 14
Set Width 130
Set Height 40
Set Shell "bash"
Set Env CRUSH_GLOBAL_CONFIG tests/vhs/fixtures

Type "go run . --config tests/vhs/fixtures/crush.json"
Enter
Sleep 3s

# Open runs dashboard
Ctrl+R
Sleep 2s

# Navigate down (cursor skips headers, only lands on run rows)
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

# Back to chat
Escape
Sleep 1s
```

**Verification**: `vhs validate tests/vhs/runs-status-sectioning.tape` exits 0.
Running `vhs tests/vhs/runs-status-sectioning.tape` produces a GIF at
`tests/vhs/output/runs-status-sectioning.gif` showing the three sections and
cursor navigation.

---

## Checklist

- [ ] `runVirtualRow` types added to `runtable.go`
- [ ] `partitionRuns` function implemented and compiles
- [ ] `RunTable.View()` data-row loop replaced with virtual-row loop
- [ ] Section headers render with `●`, label, and count badge
- [ ] Divider rows render between non-empty sections
- [ ] Cursor `▸` never appears on a header or divider row
- [ ] Comment added to `runs.go` cursor clamping block
- [ ] `TestPartitionRuns_SectionOrder` passes
- [ ] `TestPartitionRuns_EmptySectionOmitted` passes
- [ ] `TestRunTable_SectionHeadersPresent` passes
- [ ] `TestRunTable_CursorCrossesSection` passes
- [ ] All existing `runtable_test.go` tests still pass
- [ ] All existing `views/` tests still pass
- [ ] `go build ./...` clean
- [ ] VHS tape validates

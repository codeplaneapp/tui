package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
)

// makeRunSummary creates a RunSummary for testing.
func makeRunSummary(id, workflowName string, status smithers.RunStatus, startedMsAgo int64) smithers.RunSummary {
	startedAtMs := time.Now().UnixMilli() - startedMsAgo
	return smithers.RunSummary{
		RunID:        id,
		WorkflowName: workflowName,
		Status:       status,
		StartedAtMs:  &startedAtMs,
		Summary: map[string]int{
			"finished": 2,
			"total":    5,
		},
	}
}

func TestRunTable_View_ContainsColumnHeaders(t *testing.T) {
	table := RunTable{
		Runs:  []smithers.RunSummary{},
		Width: 120,
	}
	out := table.View()
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "Workflow")
	assert.Contains(t, out, "Status")
	assert.Contains(t, out, "Progress")
	assert.Contains(t, out, "Time")
}

func TestRunTable_View_NarrowTerminal_HidesProgressAndTime(t *testing.T) {
	table := RunTable{
		Runs:  []smithers.RunSummary{},
		Width: 60,
	}
	out := table.View()
	// On narrow terminals (<80), progress and time columns are hidden.
	assert.NotContains(t, out, "Progress")
	assert.NotContains(t, out, "Time")
}

func TestRunTable_View_RenderRunData(t *testing.T) {
	runs := []smithers.RunSummary{
		makeRunSummary("abc12345", "code-review", smithers.RunStatusRunning, 134000),
		makeRunSummary("def67890", "deploy-staging", smithers.RunStatusWaitingApproval, 482000),
		makeRunSummary("ghi11111", "test-suite", smithers.RunStatusFailed, 60000),
	}
	table := RunTable{
		Runs:   runs,
		Cursor: 0,
		Width:  120,
	}
	out := table.View()

	// Run IDs (truncated to 8 chars).
	assert.Contains(t, out, "abc12345")
	assert.Contains(t, out, "def67890")
	assert.Contains(t, out, "ghi11111")

	// Workflow names.
	assert.Contains(t, out, "code-review")
	assert.Contains(t, out, "deploy-staging")
	assert.Contains(t, out, "test-suite")

	// Status values.
	assert.Contains(t, out, "RUNNING")
	assert.Contains(t, out, "WAITING-APPROVAL")
	assert.Contains(t, out, "FAILED")
}

func TestRunTable_View_CursorOnFirstRow(t *testing.T) {
	runs := []smithers.RunSummary{
		makeRunSummary("run-aaa", "workflow-a", smithers.RunStatusRunning, 1000),
		makeRunSummary("run-bbb", "workflow-b", smithers.RunStatusFinished, 5000),
	}
	table := RunTable{
		Runs:   runs,
		Cursor: 0,
		Width:  120,
	}
	out := table.View()

	// The cursor indicator "│ " should appear before the first run.
	lines := strings.Split(out, "\n")
	// First line is header, second is first run row.
	found := false
	for _, line := range lines {
		if strings.Contains(line, "run-aaa") {
			assert.Contains(t, line, "│", "cursor row should have │ indicator")
			found = true
			break
		}
	}
	assert.True(t, found, "should find a line with run-aaa")
}

func TestRunTable_View_CursorOnSecondRow(t *testing.T) {
	runs := []smithers.RunSummary{
		makeRunSummary("run-aaa", "workflow-a", smithers.RunStatusRunning, 1000),
		makeRunSummary("run-bbb", "workflow-b", smithers.RunStatusFinished, 5000),
	}
	table := RunTable{
		Runs:   runs,
		Cursor: 1,
		Width:  120,
	}
	out := table.View()

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "run-bbb") {
			assert.Contains(t, line, "│", "cursor should be on second row")
			return
		}
	}
	t.Fatal("line with run-bbb not found")
}

func TestRunTable_View_NoCursorOnNonSelectedRows(t *testing.T) {
	runs := []smithers.RunSummary{
		makeRunSummary("run-aaa", "workflow-a", smithers.RunStatusRunning, 1000),
		makeRunSummary("run-bbb", "workflow-b", smithers.RunStatusFinished, 5000),
	}
	table := RunTable{
		Runs:   runs,
		Cursor: 0,
		Width:  120,
	}
	out := table.View()

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "run-bbb") {
			assert.NotContains(t, line, "│", "non-selected row should not have │")
			return
		}
	}
	t.Fatal("line with run-bbb not found")
}

func TestRunTable_View_EmptyRuns(t *testing.T) {
	table := RunTable{
		Runs:  []smithers.RunSummary{},
		Width: 120,
	}
	out := table.View()
	// Should at least render a header.
	assert.Contains(t, out, "ID")
}

func TestRunTable_View_LongRunIDTruncated(t *testing.T) {
	startedAtMs := time.Now().UnixMilli()
	runs := []smithers.RunSummary{
		{
			RunID:        "abcdefghijk123456789", // longer than 8 chars
			WorkflowName: "some-workflow",
			Status:       smithers.RunStatusRunning,
			StartedAtMs:  &startedAtMs,
		},
	}
	table := RunTable{
		Runs:  runs,
		Width: 120,
	}
	out := table.View()
	// Should show truncated ID.
	assert.Contains(t, out, "abcdefgh")
	assert.NotContains(t, out, "abcdefghijk123456789")
}

func TestRunTable_View_ProgressFromSummary(t *testing.T) {
	startedAtMs := time.Now().UnixMilli()
	runs := []smithers.RunSummary{
		{
			RunID:        "run-prog",
			WorkflowName: "test-wf",
			Status:       smithers.RunStatusRunning,
			StartedAtMs:  &startedAtMs,
			Summary: map[string]int{
				"finished": 3,
				"failed":   1,
				"total":    6,
			},
		},
	}
	table := RunTable{
		Runs:  runs,
		Width: 120,
	}
	out := table.View()
	// Progress = (finished + failed) / total = 4/6 ≈ 66%.
	// The visual bar should include block characters and a percentage.
	assert.Contains(t, out, "█")
	assert.Contains(t, out, "%")
}

func TestFmtElapsed_ActiveRun(t *testing.T) {
	startedAtMs := time.Now().Add(-2*time.Minute - 14*time.Second).UnixMilli()
	run := smithers.RunSummary{StartedAtMs: &startedAtMs}
	result := fmtElapsed(run)
	// Should be approximately "2m 14s" — just check it's not empty.
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "m")
}

func TestFmtElapsed_FinishedRun(t *testing.T) {
	startedAtMs := time.Now().Add(-10 * time.Minute).UnixMilli()
	finishedAtMs := time.Now().Add(-8 * time.Minute).UnixMilli()
	run := smithers.RunSummary{
		StartedAtMs:  &startedAtMs,
		FinishedAtMs: &finishedAtMs,
	}
	result := fmtElapsed(run)
	// Duration should be ~2 minutes.
	assert.Equal(t, "2m 0s", result)
}

func TestFmtElapsed_NoStartedAt(t *testing.T) {
	run := smithers.RunSummary{}
	result := fmtElapsed(run)
	assert.Equal(t, "", result)
}

func TestFmtProgress_NoSummary(t *testing.T) {
	run := smithers.RunSummary{}
	assert.Equal(t, "", fmtProgress(run))
}

func TestFmtProgress_ZeroTotal(t *testing.T) {
	run := smithers.RunSummary{
		Summary: map[string]int{"total": 0, "finished": 0},
	}
	assert.Equal(t, "", fmtProgress(run))
}

func TestFmtProgress_WithData(t *testing.T) {
	run := smithers.RunSummary{
		Summary: map[string]int{
			"finished": 2,
			"failed":   1,
			"total":    5,
		},
	}
	assert.Equal(t, "3/5", fmtProgress(run))
}

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
	if !strings.Contains(out, "│") {
		t.Fatalf("no cursor indicator found in output")
	}
	// cursor line must contain "run2", not "run1"
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "│") {
			if !strings.Contains(line, "run2") {
				t.Errorf("cursor on wrong row; got: %q", line)
			}
		}
	}
}

// --- RunAtCursor ---

func TestRunAtCursor_ValidIndex(t *testing.T) {
	runs := []smithers.RunSummary{
		{RunID: "run-a", Status: smithers.RunStatusRunning},
		{RunID: "run-b", Status: smithers.RunStatusFailed},
	}
	got, ok := RunAtCursor(runs, 0)
	assert.True(t, ok)
	assert.Equal(t, "run-a", got.RunID)

	got2, ok2 := RunAtCursor(runs, 1)
	assert.True(t, ok2)
	assert.Equal(t, "run-b", got2.RunID)
}

func TestRunAtCursor_OutOfRange(t *testing.T) {
	runs := []smithers.RunSummary{
		{RunID: "run-a", Status: smithers.RunStatusRunning},
	}
	_, ok := RunAtCursor(runs, 5)
	assert.False(t, ok)
}

func TestRunAtCursor_EmptyRuns(t *testing.T) {
	_, ok := RunAtCursor(nil, 0)
	assert.False(t, ok)
}

// --- fmtDetailLine ---

func TestFmtDetailLine_Running_WithInspection(t *testing.T) {
	label := "auth-review"
	insp := &smithers.RunInspection{
		Tasks: []smithers.RunTask{
			{NodeID: "n1", Label: &label, State: smithers.TaskStateRunning},
		},
	}
	run := smithers.RunSummary{RunID: "r1", Status: smithers.RunStatusRunning}
	out := fmtDetailLine(run, insp, 120)
	assert.Contains(t, out, "Running")
	assert.Contains(t, out, "auth-review")
	assert.Contains(t, out, "└─")
}

func TestFmtDetailLine_Running_NoInspection(t *testing.T) {
	run := smithers.RunSummary{RunID: "r1", Status: smithers.RunStatusRunning}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Running")
	assert.Contains(t, out, "└─")
}

func TestFmtDetailLine_Running_InspectionNoRunningTask(t *testing.T) {
	label := "pending-task"
	insp := &smithers.RunInspection{
		Tasks: []smithers.RunTask{
			{NodeID: "n1", Label: &label, State: smithers.TaskStatePending},
		},
	}
	run := smithers.RunSummary{RunID: "r1", Status: smithers.RunStatusRunning}
	out := fmtDetailLine(run, insp, 120)
	// Falls back to placeholder when no running task found.
	assert.Contains(t, out, "Running")
}

func TestFmtDetailLine_WaitingApproval_WithReason(t *testing.T) {
	reason := `{"message":"Should we deploy to prod?"}`
	run := smithers.RunSummary{
		RunID:     "r2",
		Status:    smithers.RunStatusWaitingApproval,
		ErrorJSON: &reason,
	}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "APPROVAL PENDING")
	assert.Contains(t, out, "Should we deploy to prod?")
	assert.Contains(t, out, "[a]pprove")
	assert.Contains(t, out, "[d]eny")
}

func TestFmtDetailLine_WaitingApproval_NoReason(t *testing.T) {
	run := smithers.RunSummary{RunID: "r2", Status: smithers.RunStatusWaitingApproval}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "APPROVAL PENDING")
	assert.Contains(t, out, "[a]pprove")
	assert.Contains(t, out, "[d]eny")
}

func TestFmtDetailLine_WaitingEvent(t *testing.T) {
	run := smithers.RunSummary{RunID: "r3", Status: smithers.RunStatusWaitingEvent}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Waiting for external event")
}

func TestFmtDetailLine_Failed_WithReason(t *testing.T) {
	reason := `{"message":"out of memory"}`
	run := smithers.RunSummary{
		RunID:     "r4",
		Status:    smithers.RunStatusFailed,
		ErrorJSON: &reason,
	}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "out of memory")
}

func TestFmtDetailLine_Failed_NoReason(t *testing.T) {
	run := smithers.RunSummary{RunID: "r4", Status: smithers.RunStatusFailed}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Failed")
}

func TestFmtDetailLine_Finished(t *testing.T) {
	startedAtMs := time.Now().Add(-2 * time.Minute).UnixMilli()
	finishedAtMs := time.Now().UnixMilli()
	run := smithers.RunSummary{
		RunID:        "r5",
		Status:       smithers.RunStatusFinished,
		StartedAtMs:  &startedAtMs,
		FinishedAtMs: &finishedAtMs,
	}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Completed")
}

func TestFmtDetailLine_Cancelled(t *testing.T) {
	startedAtMs := time.Now().Add(-30 * time.Second).UnixMilli()
	finishedAtMs := time.Now().UnixMilli()
	run := smithers.RunSummary{
		RunID:        "r6",
		Status:       smithers.RunStatusCancelled,
		StartedAtMs:  &startedAtMs,
		FinishedAtMs: &finishedAtMs,
	}
	out := fmtDetailLine(run, nil, 120)
	assert.Contains(t, out, "Completed")
}

func TestFmtDetailLine_Indent(t *testing.T) {
	run := smithers.RunSummary{RunID: "r7", Status: smithers.RunStatusWaitingEvent}
	out := fmtDetailLine(run, nil, 120)
	// Should start with 4-space indent.
	assert.True(t, strings.HasPrefix(out, "    "), "expected 4-space indent, got: %q", out)
}

// --- RunTable expanded detail rendering ---

func TestRunTable_ExpandedRow_ShowsDetailLine(t *testing.T) {
	run := smithers.RunSummary{
		RunID:        "exp-run",
		WorkflowName: "expand-wf",
		Status:       smithers.RunStatusWaitingEvent,
	}
	table := RunTable{
		Runs:     []smithers.RunSummary{run},
		Cursor:   0,
		Width:    120,
		Expanded: map[string]bool{"exp-run": true},
	}
	out := table.View()
	assert.Contains(t, out, "Waiting for external event")
}

func TestRunTable_CollapsedRow_NoDetailLine(t *testing.T) {
	run := smithers.RunSummary{
		RunID:        "col-run",
		WorkflowName: "collapse-wf",
		Status:       smithers.RunStatusWaitingEvent,
	}
	table := RunTable{
		Runs:   []smithers.RunSummary{run},
		Cursor: 0,
		Width:  120,
		// Expanded is nil — no detail lines.
	}
	out := table.View()
	assert.NotContains(t, out, "Waiting for external event")
}

func TestRunTable_ExpandedRow_WithInspection(t *testing.T) {
	label := "deploy-node"
	insp := &smithers.RunInspection{
		Tasks: []smithers.RunTask{
			{NodeID: "node1", Label: &label, State: smithers.TaskStateRunning},
		},
	}
	run := smithers.RunSummary{
		RunID:        "insp-run",
		WorkflowName: "inspect-wf",
		Status:       smithers.RunStatusRunning,
	}
	table := RunTable{
		Runs:     []smithers.RunSummary{run},
		Cursor:   0,
		Width:    120,
		Expanded: map[string]bool{"insp-run": true},
		Inspections: map[string]*smithers.RunInspection{
			"insp-run": insp,
		},
	}
	out := table.View()
	assert.Contains(t, out, "deploy-node")
}

// --- progressBar ---

func TestProgressBar_NoSummary(t *testing.T) {
	run := smithers.RunSummary{Status: smithers.RunStatusRunning}
	assert.Equal(t, "", progressBar(run, 8))
}

func TestProgressBar_ZeroTotal(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusRunning,
		Summary: map[string]int{"total": 0, "finished": 0},
	}
	assert.Equal(t, "", progressBar(run, 8))
}

func TestProgressBar_ZeroProgress(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusRunning,
		Summary: map[string]int{"total": 5, "finished": 0},
	}
	out := progressBar(run, 8)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "░") // all empty
	assert.Contains(t, out, "0%")
}

func TestProgressBar_FullProgress(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusFinished,
		Summary: map[string]int{"total": 4, "finished": 4},
	}
	out := progressBar(run, 8)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "█")
	assert.NotContains(t, out, "░") // fully filled — no empty blocks
	assert.Contains(t, out, "100%")
}

func TestProgressBar_HalfProgress(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusRunning,
		Summary: map[string]int{"total": 8, "finished": 4},
	}
	out := progressBar(run, 8)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "█")
	assert.Contains(t, out, "░")
	assert.Contains(t, out, "50%")
}

func TestProgressBar_CountsFailedAndCancelled(t *testing.T) {
	// completed = finished(2) + failed(1) + cancelled(1) = 4 out of 8 = 50%
	run := smithers.RunSummary{
		Status: smithers.RunStatusRunning,
		Summary: map[string]int{
			"total":     8,
			"finished":  2,
			"failed":    1,
			"cancelled": 1,
		},
	}
	out := progressBar(run, 8)
	assert.Contains(t, out, "50%")
}

func TestProgressBar_CompletedClampedToTotal(t *testing.T) {
	// completed > total should not panic or exceed 100%
	run := smithers.RunSummary{
		Status:  smithers.RunStatusRunning,
		Summary: map[string]int{"total": 3, "finished": 5},
	}
	out := progressBar(run, 8)
	assert.Contains(t, out, "100%")
}

func TestProgressBar_VisibleWidth(t *testing.T) {
	// "[████████] 100%": 1 + 8 + 2 + 4 = 15 visible characters.
	run := smithers.RunSummary{
		Status:  smithers.RunStatusFinished,
		Summary: map[string]int{"total": 1, "finished": 1},
	}
	out := progressBar(run, 8)
	// Strip ANSI escape sequences (simple regex-free approach: collect all
	// runes that are not inside ESC[...m sequences).
	visible := stripANSI(out)
	assert.Equal(t, 15, len([]rune(visible)),
		"visible width of bar with barWidth=8 must be 15 chars; got %q", visible)
}

// stripANSI removes ANSI CSI escape sequences (ESC [ ... m) from s.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// still inside escape sequence, skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestProgressBar_RunningColorGreen(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusRunning,
		Summary: map[string]int{"total": 4, "finished": 1},
	}
	out := progressBar(run, 8)
	// lipgloss renders color "2" as \x1b[32m (256-color green).
	assert.Contains(t, out, "32m", "running bar should use green (color 2 → 32m)")
}

func TestProgressBar_WaitingApprovalColorYellow(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusWaitingApproval,
		Summary: map[string]int{"total": 4, "finished": 1},
	}
	out := progressBar(run, 8)
	// lipgloss renders color "3" as \x1b[33m (256-color yellow).
	assert.Contains(t, out, "33m", "waiting-approval bar should use yellow (color 3 → 33m)")
}

func TestProgressBar_WaitingEventColorYellow(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusWaitingEvent,
		Summary: map[string]int{"total": 4, "finished": 1},
	}
	out := progressBar(run, 8)
	assert.Contains(t, out, "33m", "waiting-event bar should use yellow (color 3 → 33m)")
}

func TestProgressBar_FinishedFaint(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusFinished,
		Summary: map[string]int{"total": 4, "finished": 4},
	}
	out := progressBar(run, 8)
	// Faint style should not include any foreground color codes.
	assert.NotContains(t, out, "32m", "finished bar should not use green")
	assert.NotContains(t, out, "33m", "finished bar should not use yellow")
}

func TestProgressBar_FailedFaint(t *testing.T) {
	run := smithers.RunSummary{
		Status:  smithers.RunStatusFailed,
		Summary: map[string]int{"total": 4, "failed": 4},
	}
	out := progressBar(run, 8)
	assert.NotContains(t, out, "32m", "failed bar should not use green")
	assert.Contains(t, out, "100%")
}

func TestRunTable_View_ProgressBarRendered(t *testing.T) {
	// Verifies that the visual bar characters appear in the table output.
	startedAtMs := time.Now().UnixMilli()
	run := smithers.RunSummary{
		RunID:        "bar-run",
		WorkflowName: "bar-wf",
		Status:       smithers.RunStatusRunning,
		StartedAtMs:  &startedAtMs,
		Summary:      map[string]int{"total": 10, "finished": 5},
	}
	table := RunTable{Runs: []smithers.RunSummary{run}, Width: 120}
	out := table.View()
	assert.Contains(t, out, "█")
	assert.Contains(t, out, "░")
	assert.Contains(t, out, "%")
	assert.Contains(t, out, "Progress")
}

func TestRunTable_View_NoProgressBarWhenNoSummary(t *testing.T) {
	// When a run has no summary data the progress column should be blank.
	run := smithers.RunSummary{
		RunID:        "nosummary",
		WorkflowName: "nosummary-wf",
		Status:       smithers.RunStatusRunning,
	}
	table := RunTable{Runs: []smithers.RunSummary{run}, Width: 120}
	out := table.View()
	assert.NotContains(t, out, "█")
	assert.NotContains(t, out, "%")
}

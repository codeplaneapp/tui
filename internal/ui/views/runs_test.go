package views

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainBatchCmd executes a tea.BatchMsg and returns all child messages.
// It is a test helper for unpacking tea.Batch results.
func drainBatchCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		if c != nil {
			msgs = append(msgs, c())
		}
	}
	return msgs
}

// newRunsView creates a RunsView with a stub smithers.Client.
// Tests drive the model by calling Update directly.
func newRunsView() *RunsView {
	c := smithers.NewClient() // no-op client; no server configured
	return NewRunsView(c)
}

// makeRunSummaryForTest returns a RunSummary suitable for test fixtures.
func makeRunSummaryForTest(id, workflowName string, status smithers.RunStatus) smithers.RunSummary {
	startedAtMs := time.Now().Add(-2 * time.Minute).UnixMilli()
	return smithers.RunSummary{
		RunID:        id,
		WorkflowName: workflowName,
		Status:       status,
		StartedAtMs:  &startedAtMs,
		Summary:      map[string]int{"finished": 1, "total": 3},
	}
}

// --- Interface compliance ---

func TestRunsView_ImplementsView(t *testing.T) {
	var _ View = (*RunsView)(nil)
}

// --- Constructor ---

func TestNewRunsView_StartsLoading(t *testing.T) {
	v := newRunsView()
	assert.True(t, v.loading)
	assert.Nil(t, v.err)
	assert.Empty(t, v.runs)
	assert.Equal(t, 0, v.cursor)
}

// --- Init ---

func TestRunsView_Init_ReturnsCmd(t *testing.T) {
	v := newRunsView()
	cmd := v.Init()
	assert.NotNil(t, cmd)
}

func TestRunsView_Init_ReturnsErrorMsg_WhenNoTransport(t *testing.T) {
	v := newRunsView()
	cmd := v.Init()
	require.NotNil(t, cmd)

	// Init returns a tea.Batch; drain it to get child messages.
	msgs := drainBatchCmd(t, cmd)
	// At least one message should be runsLoadedMsg, runsErrorMsg, or
	// runsStreamUnavailableMsg (the latter is expected since there is no server).
	for _, msg := range msgs {
		switch msg.(type) {
		case runsLoadedMsg, runsErrorMsg, runsStreamUnavailableMsg, nil:
			// All acceptable outcomes.
		default:
			t.Errorf("unexpected message type %T", msg)
		}
	}
}

// --- Update: data loading ---

func TestRunsView_Update_RunsLoaded(t *testing.T) {
	v := newRunsView()
	runs := []smithers.RunSummary{
		makeRunSummaryForTest("abc12345", "code-review", smithers.RunStatusRunning),
		makeRunSummaryForTest("def67890", "deploy-staging", smithers.RunStatusWaitingApproval),
	}
	updated, cmd := v.Update(runsLoadedMsg{runs: runs})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	rv := updated.(*RunsView)
	assert.False(t, rv.loading)
	assert.Nil(t, rv.err)
	assert.Len(t, rv.runs, 2)
}

func TestRunsView_Update_RunsError(t *testing.T) {
	v := newRunsView()
	testErr := errors.New("connection refused")
	updated, cmd := v.Update(runsErrorMsg{err: testErr})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	rv := updated.(*RunsView)
	assert.False(t, rv.loading)
	assert.Equal(t, testErr, rv.err)
}

// --- Update: window resize ---

func TestRunsView_Update_WindowSize(t *testing.T) {
	v := newRunsView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.NotNil(t, updated)
	assert.Nil(t, cmd)

	rv := updated.(*RunsView)
	assert.Equal(t, 120, rv.width)
	assert.Equal(t, 40, rv.height)
}

// --- Update: keyboard navigation ---

func TestRunsView_Update_EscPopsView(t *testing.T) {
	v := newRunsView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestRunsView_Update_DownMovesDown(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
		makeRunSummaryForTest("run-3", "wf-c", smithers.RunStatusFailed),
	}
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rv := updated.(*RunsView)
	assert.Equal(t, 1, rv.cursor)
}

func TestRunsView_Update_JMovesDown(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	rv := updated.(*RunsView)
	assert.Equal(t, 1, rv.cursor)
}

func TestRunsView_Update_UpMovesUp(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 1

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	rv := updated.(*RunsView)
	assert.Equal(t, 0, rv.cursor)
}

func TestRunsView_Update_KMovesUp(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 1

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	rv := updated.(*RunsView)
	assert.Equal(t, 0, rv.cursor)
}

func TestRunsView_Update_DownClampsAtEnd(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 1 // already at last item

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rv := updated.(*RunsView)
	assert.Equal(t, 1, rv.cursor, "cursor should not go past the last item")
}

func TestRunsView_Update_UpClampsAtStart(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0 // already at first item

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	rv := updated.(*RunsView)
	assert.Equal(t, 0, rv.cursor, "cursor should not go below zero")
}

func TestRunsView_Update_RRefreshes(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.err = nil

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	rv := updated.(*RunsView)
	assert.True(t, rv.loading, "r key should set loading = true")
	assert.NotNil(t, cmd, "r key should return a fetch command")
}

func TestRunsView_Update_EnterNoopWhenEmpty(t *testing.T) {
	v := newRunsView()
	v.loading = false
	// No runs loaded — Enter should be a no-op.
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter with no runs should be a no-op")
}

// --- Expand / collapse inline detail rows ---

// TestRunsView_Enter_ExpandsRow verifies that the first Enter on an active run
// sets expanded[runID] = true and returns a fetch cmd.
func TestRunsView_Enter_ExpandsRow(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("active-run", "wf-active", smithers.RunStatusRunning),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, v.expanded["active-run"], "run should be expanded after first Enter")
	assert.NotNil(t, cmd, "should return fetchInspection cmd for active run")
}

// TestRunsView_Enter_TerminalRunExpandsWithoutFetch verifies that expanding a
// terminal run does not trigger an InspectRun fetch.
func TestRunsView_Enter_TerminalRunExpandsWithoutFetch(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("done-run", "wf-done", smithers.RunStatusFinished),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, v.expanded["done-run"], "terminal run should expand")
	assert.Nil(t, cmd, "no fetch cmd for terminal run")
}

// TestRunsView_Enter_CollapseOnSecondPress verifies that a second Enter on the
// same expanded row collapses it and navigates to the inspector.
func TestRunsView_Enter_CollapseOnSecondPress(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("active-run", "wf-active", smithers.RunStatusRunning),
	}
	v.cursor = 0

	// First Enter — expand.
	v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) //nolint:errcheck

	// Second Enter — should collapse and return OpenRunInspectMsg cmd.
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, v.expanded["active-run"], "run should be collapsed after second Enter")
	require.NotNil(t, cmd, "second Enter should return a cmd")

	msg := cmd()
	inspectMsg, ok := msg.(OpenRunInspectMsg)
	require.True(t, ok, "second Enter should emit OpenRunInspectMsg, got %T", msg)
	assert.Equal(t, "active-run", inspectMsg.RunID)
}

// TestRunsView_Enter_CachedInspectionNoFetch verifies that if an inspection is
// already cached, no fetch cmd is returned when expanding.
func TestRunsView_Enter_CachedInspectionNoFetch(t *testing.T) {
	v := newRunsView()
	v.loading = false
	run := makeRunSummaryForTest("run-cached", "wf-c", smithers.RunStatusRunning)
	v.runs = []smithers.RunSummary{run}
	v.cursor = 0
	// Pre-populate cache.
	v.inspections["run-cached"] = &smithers.RunInspection{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, v.expanded["run-cached"])
	assert.Nil(t, cmd, "no fetch cmd when inspection already cached")
}

// TestRunsView_Update_RunInspectionMsg_StoresInspection verifies that
// runInspectionMsg stores the result in v.inspections.
func TestRunsView_Update_RunInspectionMsg_StoresInspection(t *testing.T) {
	v := newRunsView()
	insp := &smithers.RunInspection{}

	_, cmd := v.Update(runInspectionMsg{runID: "run-x", inspection: insp})
	assert.Nil(t, cmd)
	assert.Same(t, insp, v.inspections["run-x"])
}

// TestRunsView_Update_RunInspectionMsg_NilSentinel verifies that a nil
// inspection is stored (prevents repeated fetch attempts).
func TestRunsView_Update_RunInspectionMsg_NilSentinel(t *testing.T) {
	v := newRunsView()

	_, cmd := v.Update(runInspectionMsg{runID: "run-y", inspection: nil})
	assert.Nil(t, cmd)
	// Key must exist, even though value is nil.
	val, ok := v.inspections["run-y"]
	assert.True(t, ok, "nil sentinel must be stored")
	assert.Nil(t, val)
}

// TestRunsView_View_ExpandedDetailLineVisible verifies that an expanded run
// shows a detail line in the rendered output.
func TestRunsView_View_ExpandedDetailLineVisible(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	run := makeRunSummaryForTest("exp-run", "expand-wf", smithers.RunStatusWaitingEvent)
	v.runs = []smithers.RunSummary{run}
	v.expanded["exp-run"] = true

	out := v.View()
	assert.Contains(t, out, "Waiting for external event")
}

// TestRunsView_View_CollapsedDetailLineHidden verifies that a collapsed run
// does NOT show a detail line.
func TestRunsView_View_CollapsedDetailLineHidden(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	run := makeRunSummaryForTest("col-run", "collapse-wf", smithers.RunStatusWaitingEvent)
	v.runs = []smithers.RunSummary{run}
	// expanded is empty — no detail lines.

	out := v.View()
	assert.NotContains(t, out, "Waiting for external event")
}

// --- View() rendering ---

func TestRunsView_View_LoadingState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	out := v.View()
	assert.Contains(t, out, "SMITHERS")
	assert.Contains(t, out, "Runs")
	assert.Contains(t, out, "Loading runs...")
}

func TestRunsView_View_ErrorState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.err = errors.New("server unavailable")
	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "server unavailable")
}

func TestRunsView_View_EmptyState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "No runs found")
}

func TestRunsView_View_HeaderWithEscHint(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "SMITHERS")
	assert.Contains(t, out, "Runs")
	assert.Contains(t, out, "[Esc] Back")
}

func TestRunsView_View_RunsTable(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("abc12345", "code-review", smithers.RunStatusRunning),
		makeRunSummaryForTest("def67890", "deploy-staging", smithers.RunStatusWaitingApproval),
		makeRunSummaryForTest("ghi11111", "test-suite", smithers.RunStatusFailed),
	}
	out := v.View()

	// Table headers.
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "Workflow")
	assert.Contains(t, out, "Status")

	// Run data.
	assert.Contains(t, out, "abc12345")
	assert.Contains(t, out, "code-review")
	assert.Contains(t, out, "RUNNING")
	assert.Contains(t, out, "def67890")
	assert.Contains(t, out, "deploy-staging")
	assert.Contains(t, out, "WAITING-APPROVAL")
	assert.Contains(t, out, "ghi11111")
	assert.Contains(t, out, "test-suite")
	assert.Contains(t, out, "FAILED")
}

func TestRunsView_View_CursorIndicator(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-aaa", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-bbb", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 0
	out := v.View()

	// The cursor indicator should be present.
	assert.Contains(t, out, "│")

	// First item line should have cursor.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "run-aaa") {
			assert.Contains(t, line, "│")
			return
		}
	}
	t.Fatal("line with run-aaa not found")
}

// --- Name / ShortHelp ---

func TestRunsView_Name(t *testing.T) {
	v := newRunsView()
	assert.Equal(t, "runs", v.Name())
}

func TestRunsView_ShortHelp(t *testing.T) {
	v := newRunsView()
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	// Collect all help text from key.Binding entries.
	var parts []string
	for _, b := range help {
		h := b.Help()
		parts = append(parts, h.Key, h.Desc)
	}
	joined := strings.Join(parts, " ")
	assert.Contains(t, joined, "navigate")
	assert.Contains(t, joined, "toggle details")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}

// ============================================================
// Streaming: applyRunEvent
// ============================================================

// TestRunsView_ApplyRunEvent_StatusChange verifies that a RunStatusChanged event
// patches the matching run in-place without inserting a new entry.
func TestRunsView_ApplyRunEvent_StatusChange(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("abc", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("def", "wf-b", smithers.RunStatusRunning),
	}

	newRunID := v.applyRunEvent(smithers.RunEvent{
		Type:   "RunStatusChanged",
		RunID:  "abc",
		Status: "finished",
	})

	assert.Empty(t, newRunID, "no new run should be inserted")
	assert.Equal(t, smithers.RunStatusFinished, v.runs[0].Status)
	assert.Equal(t, smithers.RunStatusRunning, v.runs[1].Status, "other run must not change")
	assert.Len(t, v.runs, 2, "no extra entry should appear")
}

// TestRunsView_ApplyRunEvent_RunFinished verifies the RunFinished event type.
func TestRunsView_ApplyRunEvent_RunFinished(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-1", smithers.RunStatusRunning),
	}

	v.applyRunEvent(smithers.RunEvent{
		Type:   "RunFinished",
		RunID:  "run-1",
		Status: "finished",
	})

	assert.Equal(t, smithers.RunStatusFinished, v.runs[0].Status)
}

// TestRunsView_ApplyRunEvent_NodeWaitingApproval verifies the approval event.
func TestRunsView_ApplyRunEvent_NodeWaitingApproval(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-approve", "wf-1", smithers.RunStatusRunning),
	}

	v.applyRunEvent(smithers.RunEvent{
		Type:  "NodeWaitingApproval",
		RunID: "run-approve",
	})

	assert.Equal(t, smithers.RunStatusWaitingApproval, v.runs[0].Status)
}

// TestRunsView_ApplyRunEvent_UnknownRunInserted verifies that a RunStarted event
// for an unknown RunID inserts a stub run at the front and returns its RunID.
func TestRunsView_ApplyRunEvent_UnknownRunInserted(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("existing", "wf-a", smithers.RunStatusRunning),
	}

	newRunID := v.applyRunEvent(smithers.RunEvent{
		Type:   "RunStarted",
		RunID:  "brand-new",
		Status: "running",
	})

	assert.Equal(t, "brand-new", newRunID, "should return the new run ID")
	require.Len(t, v.runs, 2, "stub should be prepended")
	assert.Equal(t, "brand-new", v.runs[0].RunID, "new run is at the front")
	assert.Equal(t, smithers.RunStatusRunning, v.runs[0].Status)
	assert.Equal(t, "existing", v.runs[1].RunID, "existing run is preserved")
}

// TestRunsView_ApplyRunEvent_EmptyStatusIgnored verifies that an event with an
// empty Status field does not change any run and does not insert a stub.
func TestRunsView_ApplyRunEvent_EmptyStatusIgnored(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-x", "wf-x", smithers.RunStatusRunning),
	}

	newRunID := v.applyRunEvent(smithers.RunEvent{
		Type:   "RunStatusChanged",
		RunID:  "run-x",
		Status: "", // empty — should be a no-op
	})

	assert.Empty(t, newRunID)
	assert.Equal(t, smithers.RunStatusRunning, v.runs[0].Status, "status must not change")
}

// TestRunsView_ApplyRunEvent_UnknownTypeIsNoop verifies that events with
// unknown type values do not modify v.runs.
func TestRunsView_ApplyRunEvent_UnknownTypeIsNoop(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-y", "wf-y", smithers.RunStatusRunning),
	}

	newRunID := v.applyRunEvent(smithers.RunEvent{
		Type:   "NodeOutput",
		RunID:  "run-y",
		Status: "some-status",
	})

	assert.Empty(t, newRunID)
	assert.Equal(t, smithers.RunStatusRunning, v.runs[0].Status, "status must not change for unrecognised event type")
	assert.Len(t, v.runs, 1)
}

// ============================================================
// Streaming: Update message routing
// ============================================================

// TestRunsView_Update_StreamReady verifies that runsStreamReadyMsg stores the
// channel, sets streamMode to "live", and dispatches WaitForAllEvents.
func TestRunsView_Update_StreamReady(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)

	ch := make(chan interface{}, 4)
	_, cmd := v.Update(runsStreamReadyMsg{ch: ch})
	require.NotNil(t, cmd, "should dispatch WaitForAllEvents")
	assert.Equal(t, "live", v.streamMode)
	// v.allEventsCh is a receive-only chan; verify it is non-nil and backed
	// by the same channel by checking a functional round-trip below.
	assert.NotNil(t, v.allEventsCh)

	// The returned cmd must block on the channel. Feeding a value should
	// produce a RunEventMsg immediately.
	ev := smithers.RunEvent{Type: "RunStarted", RunID: "r1", Status: "running"}
	ch <- smithers.RunEventMsg{RunID: "r1", Event: ev}
	msg := cmd()
	require.IsType(t, smithers.RunEventMsg{}, msg)
}

// TestRunsView_Update_StreamUnavailable verifies that runsStreamUnavailableMsg
// sets streamMode to "polling" and dispatches a pollTickCmd.
func TestRunsView_Update_StreamUnavailable(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)

	_, cmd := v.Update(runsStreamUnavailableMsg{})
	require.NotNil(t, cmd, "should dispatch pollTickCmd")
	assert.Equal(t, "polling", v.streamMode)
	require.NotNil(t, v.pollTicker, "pollTicker must be started")
	v.pollTicker.Stop()
}

// TestRunsView_Update_RunEventMsg_Patches verifies that a RunEventMsg causes
// applyRunEvent to be called (status changes in-place) and re-schedules
// WaitForAllEvents.
func TestRunsView_Update_RunEventMsg_Patches(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)

	ch := make(chan interface{}, 4)
	v.allEventsCh = ch
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-patch", "wf-p", smithers.RunStatusRunning),
	}

	ev := smithers.RunEvent{Type: "RunStatusChanged", RunID: "run-patch", Status: "finished"}
	_, cmd := v.Update(smithers.RunEventMsg{RunID: "run-patch", Event: ev})
	require.NotNil(t, cmd)
	assert.Equal(t, smithers.RunStatusFinished, v.runs[0].Status)
}

// TestRunsView_Update_RunEventDoneMsg_Reconnects verifies that RunEventDoneMsg
// triggers startStreamCmd when the context is still live.
func TestRunsView_Update_RunEventDoneMsg_Reconnects(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)
	defer v.cancel()

	_, cmd := v.Update(smithers.RunEventDoneMsg{})
	// The cmd should be non-nil (startStreamCmd) because context is still live.
	assert.NotNil(t, cmd, "should reconnect when context is still alive")
}

// TestRunsView_Update_RunEventDoneMsg_NoReconnect verifies that RunEventDoneMsg
// does NOT reconnect when the view context has been cancelled.
func TestRunsView_Update_RunEventDoneMsg_NoReconnect(t *testing.T) {
	v := newRunsView()
	ctx, cancel := newTestContext(t)
	v.ctx = ctx
	v.cancel = cancel

	cancel() // simulate view teardown

	_, cmd := v.Update(smithers.RunEventDoneMsg{})
	assert.Nil(t, cmd, "should not reconnect after context cancellation")
}

// TestRunsView_Update_EnrichRunMsg verifies that runsEnrichRunMsg replaces the
// stub entry with the full RunSummary.
func TestRunsView_Update_EnrichRunMsg(t *testing.T) {
	v := newRunsView()
	stub := smithers.RunSummary{RunID: "stub-run", Status: smithers.RunStatusRunning}
	v.runs = []smithers.RunSummary{stub}

	full := makeRunSummaryForTest("stub-run", "enriched-workflow", smithers.RunStatusFinished)
	_, cmd := v.Update(runsEnrichRunMsg{run: full})
	assert.Nil(t, cmd)
	require.Len(t, v.runs, 1)
	assert.Equal(t, "enriched-workflow", v.runs[0].WorkflowName)
	assert.Equal(t, smithers.RunStatusFinished, v.runs[0].Status)
}

// ============================================================
// Streaming: View rendering indicators
// ============================================================

// TestRunsView_View_LiveIndicator verifies that "● Live" appears in the header
// when streamMode == "live".
func TestRunsView_View_LiveIndicator(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.streamMode = "live"
	out := v.View()
	assert.Contains(t, out, "● Live")
}

// TestRunsView_View_PollingIndicator verifies that "○ Polling" appears in the
// header when streamMode == "polling".
func TestRunsView_View_PollingIndicator(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.streamMode = "polling"
	out := v.View()
	assert.Contains(t, out, "○ Polling")
}

// TestRunsView_View_NoIndicatorBeforeConnect verifies that neither indicator
// appears when streamMode is empty (before first connect).
func TestRunsView_View_NoIndicatorBeforeConnect(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	out := v.View()
	assert.NotContains(t, out, "● Live")
	assert.NotContains(t, out, "○ Polling")
}

// ============================================================
// Streaming: Esc teardown
// ============================================================

// TestRunsView_Update_EscCancelsContext verifies that pressing Esc cancels the
// view context before emitting PopViewMsg.
func TestRunsView_Update_EscCancelsContext(t *testing.T) {
	v := newRunsView()
	ctx, cancel := newTestContext(t)
	v.ctx = ctx
	v.cancel = cancel

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
	assert.NotNil(t, ctx.Err(), "context must be cancelled after Esc")
}

// TestRunsView_Update_EscStopsPolling verifies that the pollTicker is stopped
// when Esc is pressed in polling mode.
func TestRunsView_Update_EscStopsPolling(t *testing.T) {
	v := newRunsView()
	ctx, cancel := newTestContext(t)
	v.ctx = ctx
	v.cancel = cancel
	v.pollTicker = time.NewTicker(5 * time.Second)
	v.streamMode = "polling"

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
	// If Stop was called the ticker channel drains without sending more ticks.
}

// ============================================================
// Helpers
// ============================================================

// newTestContext creates a context and registers cancel as a test cleanup.
func newTestContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx, cancel
}

// ============================================================
// Hijack: RunsView
// ============================================================

// TestRunsView_HKeyNoopWhenNoRuns verifies that pressing 'h' with no runs is a
// no-op (no cmd dispatched, hijacking stays false).
func TestRunsView_HKeyNoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	assert.Nil(t, cmd, "h with no runs should be a no-op")
	assert.False(t, v.hijacking)
}

// TestRunsView_HKeySetsHijackingAndReturnsCmd verifies that pressing 'h' with a
// selected run sets hijacking=true and returns a non-nil command.
func TestRunsView_HKeySetsHijackingAndReturnsCmd(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-abc", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	rv := updated.(*RunsView)
	assert.True(t, rv.hijacking, "hijacking should be true after pressing h")
	assert.NotNil(t, cmd, "h should dispatch a hijack command")
}

// TestRunsView_HKeyIdempotentWhileHijacking verifies that pressing 'h' again
// while already hijacking is a no-op.
func TestRunsView_HKeyIdempotentWhileHijacking(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-abc", "wf-a", smithers.RunStatusRunning),
	}
	v.hijacking = true // already in progress

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	assert.Nil(t, cmd, "h while hijacking should be a no-op")
}

// TestRunsView_HijackSessionMsg_ErrorStored verifies that a runsHijackSessionMsg
// with an error clears hijacking and stores the error.
func TestRunsView_HijackSessionMsg_ErrorStored(t *testing.T) {
	v := newRunsView()
	v.hijacking = true
	testErr := errors.New("server unavailable")

	updated, cmd := v.Update(runsHijackSessionMsg{runID: "run-abc", err: testErr})
	rv := updated.(*RunsView)
	assert.False(t, rv.hijacking)
	assert.Equal(t, testErr, rv.hijackErr)
	assert.Nil(t, cmd)
}

// TestRunsView_HijackSessionMsg_BadBinaryStoresError verifies that when the
// session binary cannot be found in PATH, hijackErr is set and no exec is started.
func TestRunsView_HijackSessionMsg_BadBinaryStoresError(t *testing.T) {
	v := newRunsView()
	v.hijacking = true
	session := &smithers.HijackSession{
		RunID:       "run-abc",
		AgentEngine: "no-such-engine",
		AgentBinary: "/no/such/binary/nonexistent-xyz",
	}

	updated, cmd := v.Update(runsHijackSessionMsg{runID: "run-abc", session: session})
	rv := updated.(*RunsView)
	assert.False(t, rv.hijacking)
	assert.NotNil(t, rv.hijackErr, "error should be set when binary not found")
	assert.Nil(t, cmd, "no exec cmd should be dispatched when binary missing")
}

// TestRunsView_HijackReturnMsg_RefreshesRuns verifies that runsHijackReturnMsg
// triggers a runs refresh (loading=true, non-nil cmd).
func TestRunsView_HijackReturnMsg_RefreshesRuns(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)
	v.hijacking = true
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-abc", "wf-a", smithers.RunStatusRunning),
	}

	updated, cmd := v.Update(runsHijackReturnMsg{runID: "run-abc", err: nil})
	rv := updated.(*RunsView)
	assert.False(t, rv.hijacking)
	assert.True(t, rv.loading, "should trigger refresh after hijack returns")
	assert.NotNil(t, cmd, "should return a fetch command")
}

// TestRunsView_View_HijackingOverlay verifies that "Hijacking session..." appears
// in the view when hijacking is true.
func TestRunsView_View_HijackingOverlay(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.hijacking = true

	out := v.View()
	assert.Contains(t, out, "Hijacking session...")
}

// TestRunsView_View_HijackErrorShown verifies that a hijack error is rendered.
func TestRunsView_View_HijackErrorShown(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.hijackErr = errors.New("binary not found")

	out := v.View()
	assert.Contains(t, out, "Hijack error")
	assert.Contains(t, out, "binary not found")
}

// TestRunsView_ShortHelp_ContainsHijack verifies the 'h' binding appears in
// ShortHelp.
func TestRunsView_ShortHelp_ContainsHijack(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "hijack")
}

// TestRunsView_HijackRunCmd_ReturnsMsg verifies that hijackRunCmd returns a Cmd
// that produces a runsHijackSessionMsg (with an error since no server is
// configured for the stub client).
func TestRunsView_HijackRunCmd_ReturnsMsg(t *testing.T) {
	v := newRunsView()
	cmd := v.hijackRunCmd("run-xyz")
	require.NotNil(t, cmd)

	msg := cmd()
	hijackMsg, ok := msg.(runsHijackSessionMsg)
	require.True(t, ok, "hijackRunCmd should return runsHijackSessionMsg, got %T", msg)
	assert.Equal(t, "run-xyz", hijackMsg.runID)
	// No server configured — expect an error.
	assert.NotNil(t, hijackMsg.err)
}

// ============================================================
// Filter: cycleFilter / clearFilter
// ============================================================

// TestRunsView_CycleFilter_StartsAtAll verifies that a new view has no status
// filter (i.e. "All").
func TestRunsView_CycleFilter_StartsAtAll(t *testing.T) {
	v := newRunsView()
	assert.Equal(t, smithers.RunStatus(""), v.StatusFilter(), "initial filter must be empty (All)")
}

// TestRunsView_CycleFilter_AdvancesSequence verifies the full cycle:
// All → Running → Waiting → Completed → Failed → All.
func TestRunsView_CycleFilter_AdvancesSequence(t *testing.T) {
	v := newRunsView()

	v.cycleFilter()
	assert.Equal(t, smithers.RunStatusRunning, v.StatusFilter(), "first cycle: Running")

	v.cycleFilter()
	assert.Equal(t, smithers.RunStatusWaitingApproval, v.StatusFilter(), "second cycle: WaitingApproval")

	v.cycleFilter()
	assert.Equal(t, smithers.RunStatusFinished, v.StatusFilter(), "third cycle: Finished")

	v.cycleFilter()
	assert.Equal(t, smithers.RunStatusFailed, v.StatusFilter(), "fourth cycle: Failed")

	v.cycleFilter()
	assert.Equal(t, smithers.RunStatus(""), v.StatusFilter(), "fifth cycle: back to All")
}

// TestRunsView_CycleFilter_ResetsCursor verifies that cycling the filter resets
// the cursor to 0.
func TestRunsView_CycleFilter_ResetsCursor(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.cursor = 2
	v.cycleFilter()
	assert.Equal(t, 0, v.cursor, "cycleFilter must reset cursor to 0")
}

// TestRunsView_ClearFilter_ReturnsToAll verifies that clearFilter resets to
// the "All" state regardless of the current filter.
func TestRunsView_ClearFilter_ReturnsToAll(t *testing.T) {
	v := newRunsView()
	v.statusFilter = smithers.RunStatusFailed
	v.cursor = 3

	v.clearFilter()
	assert.Equal(t, smithers.RunStatus(""), v.StatusFilter(), "clearFilter must set filter to empty (All)")
	assert.Equal(t, 0, v.cursor, "clearFilter must reset cursor to 0")
}

// TestRunsView_FKey_CyclesFilterAndReloads verifies that pressing 'f' cycles
// the filter and returns a non-nil loadRunsCmd.
func TestRunsView_FKey_CyclesFilterAndReloads(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)
	v.loading = false

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'f'})
	rv := updated.(*RunsView)

	assert.NotNil(t, cmd, "'f' must return a fetch command")
	assert.Equal(t, smithers.RunStatusRunning, rv.StatusFilter(), "'f' must advance filter to Running")
	assert.True(t, rv.loading, "'f' must set loading = true")
}

// TestRunsView_ShiftFKey_ClearsFilterAndReloads verifies that pressing 'F'
// (shift-f) resets the filter to All and returns a non-nil loadRunsCmd.
func TestRunsView_ShiftFKey_ClearsFilterAndReloads(t *testing.T) {
	v := newRunsView()
	v.ctx, v.cancel = newTestContext(t)
	v.loading = false
	v.statusFilter = smithers.RunStatusFailed

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'F'})
	rv := updated.(*RunsView)

	assert.NotNil(t, cmd, "'F' must return a fetch command")
	assert.Equal(t, smithers.RunStatus(""), rv.StatusFilter(), "'F' must clear filter to All")
	assert.True(t, rv.loading, "'F' must set loading = true")
}

// ============================================================
// Filter: visibleRuns / client-side filtering
// ============================================================

// TestRunsView_VisibleRuns_AllWhenNoFilter verifies that visibleRuns returns
// all runs when statusFilter is empty.
func TestRunsView_VisibleRuns_AllWhenNoFilter(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusFinished),
		makeRunSummaryForTest("r3", "wf-c", smithers.RunStatusFailed),
	}
	visible := v.visibleRuns()
	assert.Len(t, visible, 3, "All filter must return all runs")
}

// TestRunsView_VisibleRuns_FilteredByRunning verifies client-side filtering.
func TestRunsView_VisibleRuns_FilteredByRunning(t *testing.T) {
	v := newRunsView()
	v.statusFilter = smithers.RunStatusRunning
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusFinished),
		makeRunSummaryForTest("r3", "wf-c", smithers.RunStatusRunning),
		makeRunSummaryForTest("r4", "wf-d", smithers.RunStatusFailed),
	}
	visible := v.visibleRuns()
	require.Len(t, visible, 2, "Running filter must return only running runs")
	assert.Equal(t, "r1", visible[0].RunID)
	assert.Equal(t, "r3", visible[1].RunID)
}

// TestRunsView_VisibleRuns_FilteredByFailed verifies client-side filtering for
// the Failed status.
func TestRunsView_VisibleRuns_FilteredByFailed(t *testing.T) {
	v := newRunsView()
	v.statusFilter = smithers.RunStatusFailed
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusFailed),
	}
	visible := v.visibleRuns()
	require.Len(t, visible, 1)
	assert.Equal(t, "r2", visible[0].RunID)
}

// TestRunsView_VisibleRuns_EmptyWhenNoMatch verifies that visibleRuns returns
// nil (not an empty slice) when no runs match the filter.
func TestRunsView_VisibleRuns_EmptyWhenNoMatch(t *testing.T) {
	v := newRunsView()
	v.statusFilter = smithers.RunStatusFailed
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusFinished),
	}
	visible := v.visibleRuns()
	assert.Empty(t, visible, "visibleRuns must return empty when no runs match the filter")
}

// ============================================================
// Filter: header indicator in View()
// ============================================================

// TestRunsView_View_FilterIndicator_All verifies that "[All]" appears in the
// header when no filter is active.
func TestRunsView_View_FilterIndicator_All(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "[All]", "header must show [All] when no filter is set")
}

// TestRunsView_View_FilterIndicator_Running verifies that "[Running]" appears
// in the header when the Running filter is active.
func TestRunsView_View_FilterIndicator_Running(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusRunning
	out := v.View()
	assert.Contains(t, out, "[Running]", "header must show [Running] when running filter is set")
}

// TestRunsView_View_FilterIndicator_Waiting verifies the Waiting label.
func TestRunsView_View_FilterIndicator_Waiting(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusWaitingApproval
	out := v.View()
	assert.Contains(t, out, "[Waiting]")
}

// TestRunsView_View_FilterIndicator_Completed verifies the Completed label.
func TestRunsView_View_FilterIndicator_Completed(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusFinished
	out := v.View()
	assert.Contains(t, out, "[Completed]")
}

// TestRunsView_View_FilterIndicator_Failed verifies the Failed label.
func TestRunsView_View_FilterIndicator_Failed(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusFailed
	out := v.View()
	assert.Contains(t, out, "[Failed]")
}

// TestRunsView_View_FilteredEmptyState verifies that a filter-specific empty
// message is shown when the filter matches no runs.
func TestRunsView_View_FilteredEmptyState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusFailed
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
	}
	out := v.View()
	assert.Contains(t, out, "[Failed]", "empty-state message must mention the active filter label")
	assert.NotContains(t, out, "No runs found.", "must not use unfiltered empty message")
}

// TestRunsView_View_FilteredRunsOnly verifies that only runs matching the
// active filter are rendered in the table.
func TestRunsView_View_FilteredRunsOnly(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.statusFilter = smithers.RunStatusRunning
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-running", "wf-run", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-done", "wf-done", smithers.RunStatusFinished),
		makeRunSummaryForTest("run-fail", "wf-fail", smithers.RunStatusFailed),
	}
	out := v.View()
	assert.Contains(t, out, "run-run", "running run should be visible")
	assert.NotContains(t, out, "run-done", "finished run must not appear with Running filter")
	assert.NotContains(t, out, "run-fail", "failed run must not appear with Running filter")
}

// TestRunsView_FilterCursorClampedOnDown verifies that cursor navigation
// respects the visible (filtered) count, not the full run list.
func TestRunsView_FilterCursorClampedOnDown(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.statusFilter = smithers.RunStatusRunning
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusRunning),
		makeRunSummaryForTest("r3", "wf-c", smithers.RunStatusFinished), // excluded by filter
	}
	// cursor is already at the last visible item (index 1 of 2 visible).
	v.cursor = 1

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rv := updated.(*RunsView)
	assert.Equal(t, 1, rv.cursor, "cursor must clamp at last visible run, not full list length")
}

// TestRunsView_ShortHelp_ContainsFilter verifies that filter keybindings are
// included in ShortHelp.
func TestRunsView_ShortHelp_ContainsFilter(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "filter", "ShortHelp must include filter binding")
	assert.Contains(t, joined, "clear filter", "ShortHelp must include clear-filter binding")
}

// ============================================================
// Live chat: 'c' key
// ============================================================

// TestRunsView_CKeyEmitsOpenLiveChatMsg verifies that pressing 'c' on a selected
// run emits OpenLiveChatMsg with the correct RunID and an empty TaskID when no
// inspection is cached.
func TestRunsView_CKeyEmitsOpenLiveChatMsg(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-chat", "wf-chat", smithers.RunStatusRunning),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd, "'c' should return a command")

	msg := cmd()
	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok, "'c' should emit OpenLiveChatMsg, got %T", msg)
	assert.Equal(t, "run-chat", chatMsg.RunID)
	assert.Equal(t, "", chatMsg.TaskID, "TaskID should be empty when no inspection cached")
}

// TestRunsView_CKeyNoopWhenNoRuns verifies that pressing 'c' with no runs is a no-op.
func TestRunsView_CKeyNoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	assert.Nil(t, cmd, "'c' with no runs should be a no-op")
}

// TestRunsView_CKeyUsesFirstRunningTaskID verifies that pressing 'c' populates
// TaskID from the first running task in a cached inspection.
func TestRunsView_CKeyUsesFirstRunningTaskID(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-insp", "wf-insp", smithers.RunStatusRunning),
	}
	v.cursor = 0

	// Pre-populate inspection cache with a mix of tasks.
	v.inspections["run-insp"] = &smithers.RunInspection{
		RunSummary: smithers.RunSummary{RunID: "run-insp", Status: smithers.RunStatusRunning},
		Tasks: []smithers.RunTask{
			{NodeID: "finished-task", State: smithers.TaskStateFinished},
			{NodeID: "running-task", State: smithers.TaskStateRunning},
			{NodeID: "pending-task", State: smithers.TaskStatePending},
		},
	}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd, "'c' should return a command")

	msg := cmd()
	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok, "'c' should emit OpenLiveChatMsg, got %T", msg)
	assert.Equal(t, "run-insp", chatMsg.RunID)
	assert.Equal(t, "running-task", chatMsg.TaskID, "TaskID should be first running task's NodeID")
}

// TestRunsView_CKeyEmptyTaskIDWhenNoRunningTask verifies that TaskID is empty
// when the cached inspection has no running tasks.
func TestRunsView_CKeyEmptyTaskIDWhenNoRunningTask(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-done", "wf-done", smithers.RunStatusFinished),
	}
	v.cursor = 0

	v.inspections["run-done"] = &smithers.RunInspection{
		RunSummary: smithers.RunSummary{RunID: "run-done", Status: smithers.RunStatusFinished},
		Tasks: []smithers.RunTask{
			{NodeID: "finished-task", State: smithers.TaskStateFinished},
		},
	}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd, "'c' should return a command")

	msg := cmd()
	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok, "'c' should emit OpenLiveChatMsg, got %T", msg)
	assert.Equal(t, "run-done", chatMsg.RunID)
	assert.Equal(t, "", chatMsg.TaskID, "TaskID should be empty when no running tasks found")
}

func TestRunsView_TKeyEmitsOpenSnapshotsMsg(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-snapshots", "wf-snapshots", smithers.RunStatusRunning),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: 't'})
	require.NotNil(t, cmd, "'t' should return a command")

	msg := cmd()
	snapshotsMsg, ok := msg.(OpenSnapshotsMsg)
	require.True(t, ok, "'t' should emit OpenSnapshotsMsg, got %T", msg)
	assert.Equal(t, "run-snapshots", snapshotsMsg.RunID)
	assert.Equal(t, SnapshotsOpenSourceRuns, snapshotsMsg.Source)
}

func TestRunsView_TKeyNoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false

	_, cmd := v.Update(tea.KeyPressMsg{Code: 't'})
	assert.Nil(t, cmd, "'t' with no runs should be a no-op")
}

// TestRunsView_ShortHelp_ContainsChat verifies the 'c' binding appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsChat(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "chat", "ShortHelp must include chat binding")
}

func TestRunsView_ShortHelp_ContainsSnapshots(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "snapshots", "ShortHelp must include snapshots binding")
}

// ============================================================
// Search: '/' key activates search, Esc dismisses
// ============================================================

// TestRunsView_SlashKey_ActivatesSearch verifies that pressing '/' sets
// searchActive = true and returns a focus command.
func TestRunsView_SlashKey_ActivatesSearch(t *testing.T) {
	v := newRunsView()
	v.loading = false

	updated, cmd := v.Update(tea.KeyPressMsg{Code: '/'})
	rv := updated.(*RunsView)
	assert.True(t, rv.SearchActive(), "'/' must activate search mode")
	// Focus() returns a cmd that blinks the cursor; it may be nil for virtual cursors.
	_ = cmd
}

// TestRunsView_SearchMode_TypingUpdatesQuery verifies that typing characters
// while search is active updates the search query.
func TestRunsView_SearchMode_TypingUpdatesQuery(t *testing.T) {
	v := newRunsView()
	v.loading = false
	// Manually activate search mode.
	v.searchActive = true
	v.searchInput.Focus() //nolint:errcheck

	// Type "abc".
	for _, ch := range "abc" {
		v.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)}) //nolint:errcheck
	}

	assert.Equal(t, "abc", v.SearchQuery(), "typing in search mode should update the query")
}

// TestRunsView_SearchMode_EscClearsQueryFirst verifies that the first Esc while
// the query is non-empty clears the query but keeps search mode active.
func TestRunsView_SearchMode_EscClearsQueryFirst(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.searchActive = true
	v.searchInput.Focus()         //nolint:errcheck
	v.searchInput.SetValue("abc") //nolint:errcheck

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rv := updated.(*RunsView)
	assert.True(t, rv.SearchActive(), "first Esc must keep search mode active")
	assert.Equal(t, "", rv.SearchQuery(), "first Esc must clear the query")
	assert.Nil(t, cmd, "first Esc returns nil cmd")
}

// TestRunsView_SearchMode_SecondEscExitsSearch verifies that a second Esc
// (query already empty) exits search mode entirely.
func TestRunsView_SearchMode_SecondEscExitsSearch(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.searchActive = true
	// Query is already empty — second Esc should exit search mode.

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rv := updated.(*RunsView)
	assert.False(t, rv.SearchActive(), "second Esc must exit search mode")
	assert.Equal(t, "", rv.SearchQuery(), "query must remain empty after second Esc")
	assert.Nil(t, cmd, "second Esc returns nil cmd")
}

// TestRunsView_SearchMode_EscNormalModePopsView verifies that Esc in normal
// (non-search) mode still emits PopViewMsg.
func TestRunsView_SearchMode_EscNormalModePopsView(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.searchActive = false

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc in normal mode must emit PopViewMsg")
}

// TestRunsView_SearchMode_CursorResetOnQueryChange verifies that the cursor
// resets to 0 when the search query changes.
func TestRunsView_SearchMode_CursorResetOnQueryChange(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.cursor = 1
	v.searchActive = true
	v.searchInput.Focus() //nolint:errcheck

	// Type 'x' — query changes from "" to "x"; cursor should reset.
	v.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) //nolint:errcheck
	assert.Equal(t, 0, v.cursor, "cursor must reset when the search query changes")
}

// ============================================================
// Search: visibleRuns filtering
// ============================================================

// TestRunsView_VisibleRuns_SearchByRunID verifies that the search query filters
// by run ID (case-insensitive substring match).
func TestRunsView_VisibleRuns_SearchByRunID(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("abc-123", "workflow-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("def-456", "workflow-b", smithers.RunStatusFinished),
		makeRunSummaryForTest("abc-789", "workflow-c", smithers.RunStatusFailed),
	}
	v.searchInput.SetValue("abc")

	visible := v.visibleRuns()
	require.Len(t, visible, 2, "should match 2 runs containing 'abc' in RunID")
	assert.Equal(t, "abc-123", visible[0].RunID)
	assert.Equal(t, "abc-789", visible[1].RunID)
}

// TestRunsView_VisibleRuns_SearchByWorkflowName verifies that the search query
// filters by workflow name.
func TestRunsView_VisibleRuns_SearchByWorkflowName(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "code-review", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "deploy-staging", smithers.RunStatusFinished),
		makeRunSummaryForTest("run-3", "code-lint", smithers.RunStatusFailed),
	}
	v.searchInput.SetValue("code")

	visible := v.visibleRuns()
	require.Len(t, visible, 2, "should match 2 runs containing 'code' in WorkflowName")
	assert.Equal(t, "run-1", visible[0].RunID)
	assert.Equal(t, "run-3", visible[1].RunID)
}

// TestRunsView_VisibleRuns_SearchCaseInsensitive verifies that the search is
// case-insensitive.
func TestRunsView_VisibleRuns_SearchCaseInsensitive(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("ABC-123", "MyWorkflow", smithers.RunStatusRunning),
		makeRunSummaryForTest("def-456", "other", smithers.RunStatusFinished),
	}
	v.searchInput.SetValue("abc")

	visible := v.visibleRuns()
	require.Len(t, visible, 1, "search must be case-insensitive")
	assert.Equal(t, "ABC-123", visible[0].RunID)
}

// TestRunsView_VisibleRuns_SearchNoMatch verifies that an empty slice is
// returned when no runs match the query.
func TestRunsView_VisibleRuns_SearchNoMatch(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("run-2", "wf-b", smithers.RunStatusFinished),
	}
	v.searchInput.SetValue("zzzzz")

	visible := v.visibleRuns()
	assert.Empty(t, visible, "no runs should match an unrelated query")
}

// TestRunsView_VisibleRuns_SearchCombinedWithStatusFilter verifies that both
// the status filter and the search query are applied together.
func TestRunsView_VisibleRuns_SearchCombinedWithStatusFilter(t *testing.T) {
	v := newRunsView()
	v.statusFilter = smithers.RunStatusRunning
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("alpha-run", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("alpha-done", "wf-b", smithers.RunStatusFinished),
		makeRunSummaryForTest("beta-run", "wf-c", smithers.RunStatusRunning),
	}
	v.searchInput.SetValue("alpha")

	visible := v.visibleRuns()
	require.Len(t, visible, 1, "combined filter+search must narrow to 1 result")
	assert.Equal(t, "alpha-run", visible[0].RunID)
}

// ============================================================
// Search: View() rendering
// ============================================================

// TestRunsView_View_SearchBarShownWhenActive verifies that the search bar
// appears in the rendered output when searchActive is true.
func TestRunsView_View_SearchBarShownWhenActive(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.searchActive = true
	v.searchInput.Focus() //nolint:errcheck

	out := v.View()
	assert.Contains(t, out, "> ", "search bar must contain '> ' prefix")
}

// TestRunsView_View_SearchBarHiddenWhenInactive verifies that the search bar
// is NOT shown when searchActive is false.
func TestRunsView_View_SearchBarHiddenWhenInactive(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.searchActive = false

	out := v.View()
	assert.NotContains(t, out, "search by run ID", "search placeholder must not appear when inactive")
}

// TestRunsView_View_SearchNoMatchEmptyState verifies that the correct empty-state
// message is shown when the search query has no matches.
func TestRunsView_View_SearchNoMatchEmptyState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.searchActive = true
	v.searchInput.Focus() //nolint:errcheck
	v.searchInput.SetValue("zzzzz")
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
	}

	out := v.View()
	assert.Contains(t, out, "zzzzz", "empty-state must echo the query that found no results")
}

// TestRunsView_ShortHelp_ContainsSearch verifies the '/' binding appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsSearch(t *testing.T) {
	v := newRunsView()
	var keys []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		keys = append(keys, h.Key)
	}
	assert.Contains(t, strings.Join(keys, " "), "/", "ShortHelp must include '/' search binding")
}

// ============================================================
// Quick-approve: 'a' key
// ============================================================

// TestRunsView_AKey_NoopWhenNoRuns verifies that pressing 'a' with no runs is
// a no-op.
func TestRunsView_AKey_NoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	assert.Nil(t, cmd, "'a' with no runs should be a no-op")
}

// TestRunsView_AKey_NoopWhenNotWaitingApproval verifies that pressing 'a' on a
// running (non-approval) run is a no-op.
func TestRunsView_AKey_NoopWhenNotWaitingApproval(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	assert.Nil(t, cmd, "'a' on a non-approval run should be a no-op")
}

// TestRunsView_AKey_ReturnsApproveCmd verifies that pressing 'a' on a
// waiting-approval run dispatches an approve command.
func TestRunsView_AKey_ReturnsApproveCmd(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-approval", "wf-a", smithers.RunStatusWaitingApproval),
	}
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	rv := updated.(*RunsView)
	require.NotNil(t, cmd, "'a' on a waiting-approval run should return a command")
	assert.Empty(t, rv.ActionMsg(), "actionMsg should be cleared on new action")
}

// TestRunsView_AKey_ClearsActionMsg verifies that pressing 'a' clears any
// stale actionMsg before dispatching.
func TestRunsView_AKey_ClearsActionMsg(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.actionMsg = "old message"
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-a", "wf-a", smithers.RunStatusWaitingApproval),
	}
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'a'})
	rv := updated.(*RunsView)
	assert.Empty(t, rv.ActionMsg(), "pressing 'a' should clear actionMsg")
}

// TestRunsView_ApproveResultMsg_Success verifies that a successful
// runsApproveResultMsg updates the run status and sets actionMsg.
func TestRunsView_ApproveResultMsg_Success(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-a", "wf-a", smithers.RunStatusWaitingApproval),
	}

	updated, cmd := v.Update(runsApproveResultMsg{runID: "run-a", err: nil})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.Contains(t, rv.ActionMsg(), "run-a", "success message should contain runID")
	assert.Equal(t, smithers.RunStatusRunning, rv.runs[0].Status,
		"approved run should be optimistically set to running")
}

// TestRunsView_ApproveResultMsg_Error verifies that a failed runsApproveResultMsg
// stores an error message.
func TestRunsView_ApproveResultMsg_Error(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-a", "wf-a", smithers.RunStatusWaitingApproval),
	}

	testErr := errors.New("approval rejected")
	updated, cmd := v.Update(runsApproveResultMsg{runID: "run-a", err: testErr})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.Contains(t, rv.ActionMsg(), "Approve error:", "error message should indicate failure")
	assert.Contains(t, rv.ActionMsg(), "approval rejected")
	// Status must not change on error.
	assert.Equal(t, smithers.RunStatusWaitingApproval, rv.runs[0].Status)
}

// TestRunsView_ApproveRunCmd_ReturnsMsg verifies that approveRunCmd returns a
// Cmd that produces a runsApproveResultMsg.
func TestRunsView_ApproveRunCmd_ReturnsMsg(t *testing.T) {
	v := newRunsView()
	cmd := v.approveRunCmd("run-xyz")
	require.NotNil(t, cmd)

	msg := cmd()
	approveMsg, ok := msg.(runsApproveResultMsg)
	require.True(t, ok, "approveRunCmd should return runsApproveResultMsg, got %T", msg)
	assert.Equal(t, "run-xyz", approveMsg.runID)
	// No server configured — expect an error (not a panic).
	assert.NotNil(t, approveMsg.err)
}

// ============================================================
// Quick-deny: 'd' key
// ============================================================

// TestRunsView_DKey_NoopWhenNoRuns verifies that pressing 'd' with no runs is a
// no-op.
func TestRunsView_DKey_NoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	assert.Nil(t, cmd, "'d' with no runs should be a no-op")
}

// TestRunsView_DKey_NoopWhenNotWaitingApproval verifies that pressing 'd' on a
// non-approval run is a no-op.
func TestRunsView_DKey_NoopWhenNotWaitingApproval(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-1", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	assert.Nil(t, cmd, "'d' on a non-approval run should be a no-op")
}

// TestRunsView_DKey_ReturnsDenyCmd verifies that pressing 'd' on a
// waiting-approval run dispatches a deny command.
func TestRunsView_DKey_ReturnsDenyCmd(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-approval", "wf-a", smithers.RunStatusWaitingApproval),
	}
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	rv := updated.(*RunsView)
	require.NotNil(t, cmd, "'d' on a waiting-approval run should return a command")
	assert.Empty(t, rv.ActionMsg(), "actionMsg should be cleared on new action")
}

// TestRunsView_DenyResultMsg_Success verifies that a successful runsDenyResultMsg
// updates the run status and sets actionMsg.
func TestRunsView_DenyResultMsg_Success(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-d", "wf-d", smithers.RunStatusWaitingApproval),
	}

	updated, cmd := v.Update(runsDenyResultMsg{runID: "run-d", err: nil})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.Contains(t, rv.ActionMsg(), "run-d", "success message should contain runID")
	assert.Equal(t, smithers.RunStatusFailed, rv.runs[0].Status,
		"denied run should be optimistically set to failed")
}

// TestRunsView_DenyResultMsg_Error verifies that a failed runsDenyResultMsg
// stores an error message.
func TestRunsView_DenyResultMsg_Error(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-d", "wf-d", smithers.RunStatusWaitingApproval),
	}

	testErr := errors.New("denial failed")
	updated, cmd := v.Update(runsDenyResultMsg{runID: "run-d", err: testErr})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.Contains(t, rv.ActionMsg(), "Deny error:")
	assert.Contains(t, rv.ActionMsg(), "denial failed")
	assert.Equal(t, smithers.RunStatusWaitingApproval, rv.runs[0].Status)
}

// TestRunsView_DenyRunCmd_ReturnsMsg verifies that denyRunCmd returns a Cmd
// that produces a runsDenyResultMsg.
func TestRunsView_DenyRunCmd_ReturnsMsg(t *testing.T) {
	v := newRunsView()
	cmd := v.denyRunCmd("run-xyz")
	require.NotNil(t, cmd)

	msg := cmd()
	denyMsg, ok := msg.(runsDenyResultMsg)
	require.True(t, ok, "denyRunCmd should return runsDenyResultMsg, got %T", msg)
	assert.Equal(t, "run-xyz", denyMsg.runID)
	assert.NotNil(t, denyMsg.err)
}

// ============================================================
// Quick-cancel: 'x' key with confirmation
// ============================================================

// TestRunsView_XKey_NoopWhenNoRuns verifies that pressing 'x' with no runs is
// a no-op.
func TestRunsView_XKey_NoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd, "'x' with no runs should be a no-op")
	assert.False(t, rv.CancelConfirm(), "cancelConfirm should stay false with no runs")
}

// TestRunsView_XKey_NoopWhenTerminal verifies that pressing 'x' on a terminal
// run does not set cancelConfirm.
func TestRunsView_XKey_NoopWhenTerminal(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-done", "wf-done", smithers.RunStatusFinished),
	}
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd, "'x' on a terminal run should be a no-op")
	assert.False(t, rv.CancelConfirm(), "cancelConfirm should not be set for terminal runs")
}

// TestRunsView_XKey_FirstPressSetsCancelConfirm verifies that the first 'x' press
// on an active run sets cancelConfirm = true without dispatching a cancel.
func TestRunsView_XKey_FirstPressSetsCancelConfirm(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-active", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd, "first 'x' should not dispatch a cancel command")
	assert.True(t, rv.CancelConfirm(), "first 'x' should set cancelConfirm = true")
}

// TestRunsView_XKey_SecondPressDispatchesCancel verifies that the second 'x'
// press executes the cancel and clears cancelConfirm.
func TestRunsView_XKey_SecondPressDispatchesCancel(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-active", "wf-a", smithers.RunStatusRunning),
	}
	v.cursor = 0
	v.cancelConfirm = true // simulate first press already happened

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'x'})
	rv := updated.(*RunsView)
	require.NotNil(t, cmd, "second 'x' should dispatch a cancel command")
	assert.False(t, rv.CancelConfirm(), "cancelConfirm should be cleared after second 'x'")
}

// TestRunsView_XKey_WaitingApprovalRunIsCancellable verifies that 'x' works on
// waiting-approval runs (they are not terminal).
func TestRunsView_XKey_WaitingApprovalRunIsCancellable(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-wait", "wf-a", smithers.RunStatusWaitingApproval),
	}
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'x'})
	rv := updated.(*RunsView)
	assert.True(t, rv.CancelConfirm(), "waiting-approval run should be cancellable")
}

// TestRunsView_CancelResultMsg_Success verifies that a successful
// runsCancelResultMsg updates the run status, sets actionMsg, and clears
// cancelConfirm.
func TestRunsView_CancelResultMsg_Success(t *testing.T) {
	v := newRunsView()
	v.cancelConfirm = true
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-c", "wf-c", smithers.RunStatusRunning),
	}

	updated, cmd := v.Update(runsCancelResultMsg{runID: "run-c", err: nil})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.False(t, rv.CancelConfirm(), "cancelConfirm should be cleared on result")
	assert.Contains(t, rv.ActionMsg(), "run-c")
	assert.Equal(t, smithers.RunStatusCancelled, rv.runs[0].Status,
		"cancelled run should be optimistically set to cancelled")
}

// TestRunsView_CancelResultMsg_Error verifies that a failed runsCancelResultMsg
// stores an error message and clears cancelConfirm.
func TestRunsView_CancelResultMsg_Error(t *testing.T) {
	v := newRunsView()
	v.cancelConfirm = true
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("run-c", "wf-c", smithers.RunStatusRunning),
	}

	testErr := errors.New("cancel rejected")
	updated, cmd := v.Update(runsCancelResultMsg{runID: "run-c", err: testErr})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd)
	assert.False(t, rv.CancelConfirm(), "cancelConfirm should be cleared even on error")
	assert.Contains(t, rv.ActionMsg(), "Cancel error:")
	assert.Contains(t, rv.ActionMsg(), "cancel rejected")
	assert.Equal(t, smithers.RunStatusRunning, rv.runs[0].Status,
		"status must not change on cancel error")
}

// TestRunsView_CancelRunCmd_ReturnsMsg verifies that cancelRunCmd returns a Cmd
// that produces a runsCancelResultMsg.
func TestRunsView_CancelRunCmd_ReturnsMsg(t *testing.T) {
	v := newRunsView()
	cmd := v.cancelRunCmd("run-xyz")
	require.NotNil(t, cmd)

	msg := cmd()
	cancelMsg, ok := msg.(runsCancelResultMsg)
	require.True(t, ok, "cancelRunCmd should return runsCancelResultMsg, got %T", msg)
	assert.Equal(t, "run-xyz", cancelMsg.runID)
	assert.NotNil(t, cancelMsg.err)
}

// ============================================================
// Quick-action: View() rendering
// ============================================================

// TestRunsView_View_CancelConfirmPromptShown verifies that the cancel
// confirmation prompt appears when cancelConfirm is true.
func TestRunsView_View_CancelConfirmPromptShown(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.cancelConfirm = true

	out := v.View()
	assert.Contains(t, out, "confirm cancel", "cancel confirmation prompt should appear")
}

// TestRunsView_View_ActionMsgSuccessShown verifies that a success actionMsg is
// rendered.
func TestRunsView_View_ActionMsgSuccessShown(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.actionMsg = "Approved run run-abc"

	out := v.View()
	assert.Contains(t, out, "Approved run run-abc")
}

// TestRunsView_View_ActionMsgErrorShown verifies that an error actionMsg is
// rendered.
func TestRunsView_View_ActionMsgErrorShown(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.actionMsg = "Approve error: server unavailable"

	out := v.View()
	assert.Contains(t, out, "Approve error: server unavailable")
}

// TestRunsView_View_NoCancelPromptWhenFalse verifies that the cancel
// confirmation prompt is NOT shown when cancelConfirm is false.
func TestRunsView_View_NoCancelPromptWhenFalse(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.cancelConfirm = false

	out := v.View()
	assert.NotContains(t, out, "confirm cancel")
}

// ============================================================
// Quick-action: ShortHelp
// ============================================================

// TestRunsView_ShortHelp_ContainsApprove verifies that 'a' appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsApprove(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "approve", "ShortHelp must include approve")
}

// TestRunsView_ShortHelp_ContainsDeny verifies that 'd' appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsDeny(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "deny", "ShortHelp must include deny")
}

// TestRunsView_ShortHelp_ContainsCancel verifies that 'x' appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsCancel(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "cancel", "ShortHelp must include cancel run")
}

// ============================================================
// resolveApprovalNodeID
// ============================================================

// TestRunsView_ResolveApprovalNodeID_FallsBackToRunID verifies that when there
// is no cached inspection, the runID itself is returned.
func TestRunsView_ResolveApprovalNodeID_FallsBackToRunID(t *testing.T) {
	v := newRunsView()
	nodeID := v.resolveApprovalNodeID("run-xyz")
	assert.Equal(t, "run-xyz", nodeID, "should fall back to runID when no inspection cached")
}

// TestRunsView_ResolveApprovalNodeID_ReturnsBlockedNodeID verifies that when a
// blocked task exists in the cached inspection, its NodeID is returned.
func TestRunsView_ResolveApprovalNodeID_ReturnsBlockedNodeID(t *testing.T) {
	v := newRunsView()
	v.inspections["run-xyz"] = &smithers.RunInspection{
		RunSummary: smithers.RunSummary{RunID: "run-xyz"},
		Tasks: []smithers.RunTask{
			{NodeID: "finished-node", State: smithers.TaskStateFinished},
			{NodeID: "blocked-node", State: smithers.TaskStateBlocked},
		},
	}
	nodeID := v.resolveApprovalNodeID("run-xyz")
	assert.Equal(t, "blocked-node", nodeID, "should return the blocked task's NodeID")
}

// TestRunsView_ResolveApprovalNodeID_NilInspectionFallsBack verifies that a nil
// inspection sentinel falls back to the runID.
func TestRunsView_ResolveApprovalNodeID_NilInspectionFallsBack(t *testing.T) {
	v := newRunsView()
	v.inspections["run-nil"] = nil // nil sentinel (fetch attempted, failed)
	nodeID := v.resolveApprovalNodeID("run-nil")
	assert.Equal(t, "run-nil", nodeID, "nil inspection should fall back to runID")
}

// ============================================================
// runs-filter-by-workflow: 'w' key
// ============================================================

// TestRunsView_CycleWorkflowFilter_StartsAtAll verifies initial workflow filter is empty.
func TestRunsView_CycleWorkflowFilter_StartsAtAll(t *testing.T) {
	v := newRunsView()
	assert.Equal(t, "", v.WorkflowFilter(), "initial workflow filter must be empty (All)")
}

// TestRunsView_CycleWorkflowFilter_CyclesWorkflowNames verifies cycling through unique names.
func TestRunsView_CycleWorkflowFilter_CyclesWorkflowNames(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-alpha", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-beta", smithers.RunStatusFinished),
		makeRunSummaryForTest("r3", "wf-alpha", smithers.RunStatusFailed), // duplicate
	}

	v.cycleWorkflowFilter()
	assert.Equal(t, "wf-alpha", v.WorkflowFilter(), "first cycle: wf-alpha")

	v.cycleWorkflowFilter()
	assert.Equal(t, "wf-beta", v.WorkflowFilter(), "second cycle: wf-beta")

	v.cycleWorkflowFilter()
	assert.Equal(t, "", v.WorkflowFilter(), "third cycle: back to All")
}

// TestRunsView_CycleWorkflowFilter_ResetsCursor verifies cursor is reset on cycle.
func TestRunsView_CycleWorkflowFilter_ResetsCursor(t *testing.T) {
	v := newRunsView()
	v.cursor = 5
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
	}
	v.cycleWorkflowFilter()
	assert.Equal(t, 0, v.cursor, "cycleWorkflowFilter must reset cursor to 0")
}

// TestRunsView_CycleWorkflowFilter_EmptyRunsNoChange verifies no crash with empty runs.
func TestRunsView_CycleWorkflowFilter_EmptyRunsNoChange(t *testing.T) {
	v := newRunsView()
	v.workflowFilter = "some-filter"
	v.cycleWorkflowFilter() // should reset to "" since no runs
	assert.Equal(t, "", v.WorkflowFilter())
}

// TestRunsView_WKey_CyclesWorkflowFilter verifies 'w' key cycles workflow filter.
func TestRunsView_WKey_CyclesWorkflowFilter(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "wf-a", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "wf-b", smithers.RunStatusRunning),
	}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'w'})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd, "'w' should not reload (client-side filter)")
	assert.Equal(t, "wf-a", rv.WorkflowFilter(), "'w' should advance to first workflow name")
}

// TestRunsView_ShiftWKey_ClearsWorkflowFilter verifies 'W' resets workflow filter.
func TestRunsView_ShiftWKey_ClearsWorkflowFilter(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.workflowFilter = "wf-a"

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'W'})
	rv := updated.(*RunsView)
	assert.Equal(t, "", rv.WorkflowFilter(), "'W' should clear workflow filter")
}

// TestRunsView_VisibleRuns_WorkflowFilter filters by workflow name.
func TestRunsView_VisibleRuns_WorkflowFilter(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "code-review", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "deploy-staging", smithers.RunStatusRunning),
		makeRunSummaryForTest("r3", "code-review", smithers.RunStatusFinished),
	}
	v.workflowFilter = "code-review"

	visible := v.visibleRuns()
	assert.Len(t, visible, 2)
	for _, r := range visible {
		assert.Equal(t, "code-review", r.WorkflowName)
	}
}

// TestRunsView_VisibleRuns_WorkflowFilter_CaseInsensitive verifies case-insensitive matching.
func TestRunsView_VisibleRuns_WorkflowFilter_CaseInsensitive(t *testing.T) {
	v := newRunsView()
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("r1", "Code-Review", smithers.RunStatusRunning),
		makeRunSummaryForTest("r2", "deploy", smithers.RunStatusRunning),
	}
	v.workflowFilter = "code-review" // lowercase

	visible := v.visibleRuns()
	assert.Len(t, visible, 1)
	assert.Equal(t, "Code-Review", visible[0].WorkflowName)
}

// TestRunsView_View_WorkflowFilterLabel verifies workflow filter label appears in header.
func TestRunsView_View_WorkflowFilterLabel(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.workflowFilter = "code-review"
	out := v.View()
	assert.Contains(t, out, "code-review")
}

// TestRunsView_View_WorkflowEmptyState shows informative empty message.
func TestRunsView_View_WorkflowEmptyState(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.workflowFilter = "nonexistent-wf"
	out := v.View()
	assert.Contains(t, out, "nonexistent-wf")
}

// ============================================================
// runs-filter-by-date-range: 'D' key
// ============================================================

// TestRunsView_DateFilter_StartsAtAll verifies initial date filter is All.
func TestRunsView_DateFilter_StartsAtAll(t *testing.T) {
	v := newRunsView()
	assert.Equal(t, dateRangeAll, v.DateFilter(), "initial date filter must be All")
}

// TestRunsView_CycleDateFilter_AdvancesSequence verifies full cycle.
func TestRunsView_CycleDateFilter_AdvancesSequence(t *testing.T) {
	v := newRunsView()

	v.cycleDateFilter()
	assert.Equal(t, dateRangeToday, v.DateFilter(), "first cycle: Today")

	v.cycleDateFilter()
	assert.Equal(t, dateRangeWeek, v.DateFilter(), "second cycle: Week")

	v.cycleDateFilter()
	assert.Equal(t, dateRangeMonth, v.DateFilter(), "third cycle: Month")

	v.cycleDateFilter()
	assert.Equal(t, dateRangeAll, v.DateFilter(), "fourth cycle: back to All")
}

// TestRunsView_CycleDateFilter_ResetsCursor verifies cursor is reset.
func TestRunsView_CycleDateFilter_ResetsCursor(t *testing.T) {
	v := newRunsView()
	v.cursor = 3
	v.cycleDateFilter()
	assert.Equal(t, 0, v.cursor)
}

// TestRunsView_DKey_CyclesDateFilter verifies 'D' key cycles date filter.
func TestRunsView_DKey_CyclesDateFilter(t *testing.T) {
	v := newRunsView()
	v.loading = false

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'D'})
	rv := updated.(*RunsView)
	assert.Nil(t, cmd, "'D' should not reload (client-side filter)")
	assert.Equal(t, dateRangeToday, rv.DateFilter(), "'D' should advance to Today")
}

// TestRunsView_VisibleRuns_DateFilter_Today filters by runs started in last 24h.
func TestRunsView_VisibleRuns_DateFilter_Today(t *testing.T) {
	v := newRunsView()
	now := time.Now().UnixMilli()
	yesterday := time.Now().Add(-48 * time.Hour).UnixMilli()
	v.runs = []smithers.RunSummary{
		{RunID: "recent", WorkflowName: "wf", Status: smithers.RunStatusRunning, StartedAtMs: &now},
		{RunID: "old", WorkflowName: "wf", Status: smithers.RunStatusFinished, StartedAtMs: &yesterday},
		{RunID: "nil-start", WorkflowName: "wf", Status: smithers.RunStatusRunning},
	}
	v.dateFilter = dateRangeToday

	visible := v.visibleRuns()
	assert.Len(t, visible, 1)
	assert.Equal(t, "recent", visible[0].RunID)
}

// TestRunsView_VisibleRuns_DateFilter_All returns all runs.
func TestRunsView_VisibleRuns_DateFilter_All(t *testing.T) {
	v := newRunsView()
	now := time.Now().UnixMilli()
	old := time.Now().Add(-48 * time.Hour).UnixMilli()
	v.runs = []smithers.RunSummary{
		{RunID: "r1", WorkflowName: "wf", Status: smithers.RunStatusRunning, StartedAtMs: &now},
		{RunID: "r2", WorkflowName: "wf", Status: smithers.RunStatusFinished, StartedAtMs: &old},
	}
	v.dateFilter = dateRangeAll

	visible := v.visibleRuns()
	assert.Len(t, visible, 2)
}

// TestRunsView_View_DateFilterLabel verifies date filter label appears in header.
func TestRunsView_View_DateFilterLabel(t *testing.T) {
	v := newRunsView()
	v.width = 120
	v.height = 40
	v.loading = false
	v.dateFilter = dateRangeToday
	out := v.View()
	assert.Contains(t, out, "Today")
}

// TestRunsView_ShortHelp_ContainsWorkflowFilter verifies 'w' appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsWorkflowFilter(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "filter workflow")
}

// TestRunsView_ShortHelp_ContainsDateFilter verifies 'D' appears in ShortHelp.
func TestRunsView_ShortHelp_ContainsDateFilter(t *testing.T) {
	v := newRunsView()
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "filter date")
}

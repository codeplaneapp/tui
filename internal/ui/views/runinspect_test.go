package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fixtures ---

// fixtureInspection returns a RunInspection suitable for unit tests.
func fixtureInspection() *smithers.RunInspection {
	label1 := "review-auth"
	label2 := "fetch-deps"
	label3 := "deploy"
	attempt1 := 1
	now := time.Now().UnixMilli()
	return &smithers.RunInspection{
		RunSummary: smithers.RunSummary{
			RunID:        "abc12345",
			WorkflowName: "code-review",
			Status:       smithers.RunStatusRunning,
			StartedAtMs:  &now,
		},
		Tasks: []smithers.RunTask{
			{NodeID: "fetch-deps", Label: &label2, State: smithers.TaskStateFinished},
			{NodeID: "review-auth", Label: &label1, State: smithers.TaskStateRunning, LastAttempt: &attempt1},
			{NodeID: "deploy", Label: &label3, State: smithers.TaskStatePending},
		},
	}
}

// newRunInspectView creates a RunInspectView with a stub smithers.Client.
func newRunInspectView(runID string) *RunInspectView {
	c := smithers.NewClient()
	return NewRunInspectView(c, runID)
}

// --- Interface compliance ---

func TestRunInspectView_ImplementsView(t *testing.T) {
	var _ View = (*RunInspectView)(nil)
}

// --- Constructor ---

func TestRunInspectView_NewStartsLoading(t *testing.T) {
	v := newRunInspectView("abc12345")
	assert.True(t, v.loading)
	assert.Nil(t, v.err)
	assert.Nil(t, v.inspection)
	assert.Equal(t, 0, v.cursor)
}

// --- Init ---

func TestRunInspectView_Init_ReturnsCmd(t *testing.T) {
	v := newRunInspectView("abc12345")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil command")
}

// --- Update: data loading ---

func TestRunInspectView_Loading(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	// Loading state is the default on construction.
	out := v.View()
	assert.Contains(t, out, "Loading run...")
}

func TestRunInspectView_Error(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, cmd := v.Update(runInspectErrorMsg{err: errors.New("connection refused")})
	assert.Nil(t, cmd)
	rv := updated.(*RunInspectView)
	assert.False(t, rv.loading)
	assert.NotNil(t, rv.err)
	out := rv.View()
	assert.Contains(t, out, "Error: connection refused")
}

func TestRunInspectView_EmptyTasks(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	emptyInspection := &smithers.RunInspection{
		RunSummary: smithers.RunSummary{
			RunID:  "abc12345",
			Status: smithers.RunStatusRunning,
		},
		Tasks: nil,
	}
	updated, _ := v.Update(runInspectLoadedMsg{inspection: emptyInspection})
	rv := updated.(*RunInspectView)
	out := rv.View()
	assert.Contains(t, out, "No nodes found.")
}

func TestRunInspectView_NodeList(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	out := rv.View()

	// All three node IDs should be visible.
	assert.Contains(t, out, "fetch-deps")
	assert.Contains(t, out, "review-auth")
	assert.Contains(t, out, "deploy")

	// State glyphs.
	assert.Contains(t, out, "●") // running and finished both use ●
	assert.Contains(t, out, "○") // pending uses ○
}

func TestRunInspectView_Cursor(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 1 // cursor on second row (review-auth)
	out := rv.View()

	lines := strings.Split(out, "\n")
	cursorCount := 0
	var cursorLine string
	for _, line := range lines {
		if strings.Contains(line, "▸") {
			cursorCount++
			cursorLine = line
		}
	}
	assert.Equal(t, 1, cursorCount, "exactly one cursor indicator should be present")
	assert.Contains(t, cursorLine, "review-auth", "cursor should be on review-auth (index 1)")
}

func TestRunInspectView_Header(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	out := rv.View()

	assert.Contains(t, out, "abc12345")
	assert.Contains(t, out, "code-review")
	assert.Contains(t, out, "[Esc] Back")
}

// --- Update: keyboard ---

func TestRunInspectView_EscEmitsPopMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestRunInspectView_QEmitsPopMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'q'})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "q should emit PopViewMsg")
}

func TestRunInspectView_ChatEmitsMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 1 // review-auth

	_, cmd := rv.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd, "c key should return a command")
	msg := cmd()

	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok, "c key should emit OpenLiveChatMsg, got %T", msg)
	assert.Equal(t, "abc12345", chatMsg.RunID)
	assert.Equal(t, "review-auth", chatMsg.TaskID)
}

func TestRunInspectView_ChatNoopWhenNoTasks(t *testing.T) {
	v := newRunInspectView("abc12345")
	// No inspection loaded yet.
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	assert.Nil(t, cmd, "c should be no-op when inspection is nil")
}

func TestRunInspectView_TKeyEmitsSnapshotsMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)

	_, cmd := rv.Update(tea.KeyPressMsg{Code: 't'})
	require.NotNil(t, cmd, "t key should return a command")
	msg := cmd()

	snapshotsMsg, ok := msg.(OpenSnapshotsMsg)
	require.True(t, ok, "t key should emit OpenSnapshotsMsg, got %T", msg)
	assert.Equal(t, "abc12345", snapshotsMsg.RunID)
	assert.Equal(t, SnapshotsOpenSourceRunInspect, snapshotsMsg.Source)
}

func TestRunInspectView_DownMovesDown(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 0

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, next.(*RunInspectView).cursor)
}

func TestRunInspectView_UpMovesUp(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 2

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, next.(*RunInspectView).cursor)
}

func TestRunInspectView_JMovesDown(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 0

	next, _ := rv.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, next.(*RunInspectView).cursor)
}

func TestRunInspectView_KMovesUp(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 1

	next, _ := rv.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, next.(*RunInspectView).cursor)
}

func TestRunInspectView_DownClampsAtEnd(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 2 // last item

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, next.(*RunInspectView).cursor, "cursor should not go past last item")
}

func TestRunInspectView_UpClampsAtStart(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 0

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, next.(*RunInspectView).cursor, "cursor should not go below zero")
}

func TestRunInspectView_RRefreshes(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.loading = false
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	rv := updated.(*RunInspectView)
	assert.True(t, rv.loading, "r should set loading = true")
	assert.NotNil(t, cmd, "r should return a fetch command")
}

func TestRunInspectView_WindowSizeStored(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rv := updated.(*RunInspectView)
	assert.Equal(t, 100, rv.width)
	assert.Equal(t, 30, rv.height)
}

func TestRunInspectView_LoadedMsg_ClampsOOBCursor(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.cursor = 99 // out of bounds before data arrives
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	assert.LessOrEqual(t, rv.cursor, len(rv.inspection.Tasks)-1, "cursor should be clamped after load")
}

// --- RunsView Enter key wiring ---

// TestRunsView_EnterEmitsOpenRunInspectMsg verifies that a second Enter
// (when the run is already expanded) collapses the row and emits
// OpenRunInspectMsg to navigate to the full inspector.
func TestRunsView_EnterEmitsOpenRunInspectMsg(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{
		makeRunSummaryForTest("abc12345", "code-review", smithers.RunStatusRunning),
		makeRunSummaryForTest("def67890", "deploy-staging", smithers.RunStatusFinished),
	}
	v.cursor = 0

	// First Enter: expand the row (returns fetchInspection cmd for active run).
	_, cmd1 := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd1, "first Enter with an active run should return a fetch cmd")
	assert.True(t, v.expanded["abc12345"], "run should be expanded after first Enter")

	// Second Enter: collapse and navigate to inspector.
	_, cmd2 := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd2, "second Enter should emit OpenRunInspectMsg cmd")
	assert.False(t, v.expanded["abc12345"], "run should be collapsed after second Enter")

	msg := cmd2()
	inspectMsg, ok := msg.(OpenRunInspectMsg)
	require.True(t, ok, "second Enter should emit OpenRunInspectMsg, got %T", msg)
	assert.Equal(t, "abc12345", inspectMsg.RunID)
}

func TestRunsView_EnterNoopWhenNoRuns(t *testing.T) {
	v := newRunsView()
	v.loading = false
	v.runs = []smithers.RunSummary{}

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter with no runs should be no-op")
}

// --- View rendering ---

func TestRunInspectView_SubHeaderContainsStatus(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	out := rv.View()
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "LIVE")
}

func TestRunInspectView_SubHeaderTerminalRun(t *testing.T) {
	v := newRunInspectView("done123")
	v.width = 120
	v.height = 40
	now := time.Now().UnixMilli()
	finished := &smithers.RunInspection{
		RunSummary: smithers.RunSummary{
			RunID:       "done123",
			Status:      smithers.RunStatusFinished,
			StartedAtMs: &now,
		},
	}
	updated, _ := v.Update(runInspectLoadedMsg{inspection: finished})
	rv := updated.(*RunInspectView)
	out := rv.View()
	assert.Contains(t, out, "DONE")
}

func TestRunInspectView_HelpBarContainsBindings(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	out := rv.View()
	assert.Contains(t, out, "navigate")
	assert.Contains(t, out, "chat")
	assert.Contains(t, out, "snapshots")
	assert.Contains(t, out, "refresh")
	assert.Contains(t, out, "back")
}

func TestRunInspectView_Name(t *testing.T) {
	v := newRunInspectView("abc12345")
	assert.Equal(t, "runinspect", v.Name())
}

func TestRunInspectView_SetSize(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.SetSize(80, 24)
	assert.Equal(t, 80, v.width)
	assert.Equal(t, 24, v.height)
}

// --- taskGlyphAndStyle ---

func TestTaskGlyphAndStyle_AllStates(t *testing.T) {
	states := []struct {
		state     smithers.TaskState
		wantGlyph string
	}{
		{smithers.TaskStateRunning, "●"},
		{smithers.TaskStateFinished, "●"},
		{smithers.TaskStateFailed, "●"},
		{smithers.TaskStatePending, "○"},
		{smithers.TaskStateCancelled, "–"},
		{smithers.TaskStateSkipped, "↷"},
		{smithers.TaskStateBlocked, "⏸"},
	}
	for _, tc := range states {
		glyph, _ := taskGlyphAndStyle(tc.state)
		assert.Equal(t, tc.wantGlyph, glyph, "state %s", tc.state)
	}
}

// ============================================================
// Hijack: RunInspectView
// ============================================================

// TestRunInspectView_HKeySetsHijackingAndReturnsCmd verifies that pressing 'h'
// sets hijacking=true and returns a non-nil command.
func TestRunInspectView_HKeySetsHijackingAndReturnsCmd(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)

	next, cmd := rv.Update(tea.KeyPressMsg{Code: 'h'})
	nv := next.(*RunInspectView)
	assert.True(t, nv.hijacking, "hijacking should be true after pressing h")
	assert.NotNil(t, cmd, "h should dispatch a hijack command")
}

// TestRunInspectView_HKeyIdempotentWhileHijacking verifies that pressing 'h'
// again while already hijacking is a no-op.
func TestRunInspectView_HKeyIdempotentWhileHijacking(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.hijacking = true

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'h'})
	assert.Nil(t, cmd, "h while hijacking should be a no-op")
}

// TestRunInspectView_HijackSessionMsg_ErrorStored verifies that a
// runInspectHijackSessionMsg with an error clears hijacking and stores the error.
func TestRunInspectView_HijackSessionMsg_ErrorStored(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.hijacking = true
	testErr := errors.New("server unavailable")

	updated, cmd := v.Update(runInspectHijackSessionMsg{runID: "abc12345", err: testErr})
	rv := updated.(*RunInspectView)
	assert.False(t, rv.hijacking)
	assert.Equal(t, testErr, rv.hijackErr)
	assert.Nil(t, cmd)
}

// TestRunInspectView_HijackSessionMsg_BadBinaryStoresError verifies that when
// the session binary cannot be found, hijackErr is set and no exec is started.
func TestRunInspectView_HijackSessionMsg_BadBinaryStoresError(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.hijacking = true
	session := &smithers.HijackSession{
		RunID:       "abc12345",
		AgentEngine: "no-such-engine",
		AgentBinary: "/no/such/binary/nonexistent-xyz",
	}

	updated, cmd := v.Update(runInspectHijackSessionMsg{runID: "abc12345", session: session})
	rv := updated.(*RunInspectView)
	assert.False(t, rv.hijacking)
	assert.NotNil(t, rv.hijackErr, "error should be set when binary not found")
	assert.Nil(t, cmd, "no exec cmd should be dispatched when binary missing")
}

// TestRunInspectView_HijackReturnMsg_RefreshesInspection verifies that
// runInspectHijackReturnMsg triggers a refresh (loading=true, non-nil cmd).
func TestRunInspectView_HijackReturnMsg_RefreshesInspection(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.hijacking = true

	updated, cmd := v.Update(runInspectHijackReturnMsg{runID: "abc12345", err: nil})
	rv := updated.(*RunInspectView)
	assert.False(t, rv.hijacking)
	assert.True(t, rv.loading, "should trigger refresh after hijack returns")
	assert.NotNil(t, cmd, "should return a fetch command")
}

// TestRunInspectView_View_HijackingOverlay verifies that "Hijacking session..."
// appears in the view when hijacking is true.
func TestRunInspectView_View_HijackingOverlay(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	v.hijacking = true

	out := v.View()
	assert.Contains(t, out, "Hijacking session...")
}

// TestRunInspectView_View_HijackErrorShown verifies that a hijack error is rendered.
func TestRunInspectView_View_HijackErrorShown(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	v.loading = false
	v.hijackErr = errors.New("binary not found")
	// Need to load inspection so body renders (past hijackErr section).
	v.inspection = fixtureInspection()

	out := v.View()
	assert.Contains(t, out, "Hijack error")
	assert.Contains(t, out, "binary not found")
}

// TestRunInspectView_ShortHelp_ContainsHijack verifies the 'h' binding appears
// in ShortHelp.
func TestRunInspectView_ShortHelp_ContainsHijack(t *testing.T) {
	v := newRunInspectView("abc12345")
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "hijack")
}

func TestRunInspectView_ShortHelp_ContainsSnapshots(t *testing.T) {
	v := newRunInspectView("abc12345")
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "snapshots")
}

// TestRunInspectView_HijackRunCmd_ReturnsMsg verifies that hijackRunCmd returns
// a Cmd that produces a runInspectHijackSessionMsg (with an error since no
// server is configured for the stub client).
func TestRunInspectView_HijackRunCmd_ReturnsMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	cmd := v.hijackRunCmd("abc12345")
	require.NotNil(t, cmd)

	msg := cmd()
	hijackMsg, ok := msg.(runInspectHijackSessionMsg)
	require.True(t, ok, "hijackRunCmd should return runInspectHijackSessionMsg, got %T", msg)
	assert.Equal(t, "abc12345", hijackMsg.runID)
	// No server configured — expect an error.
	assert.NotNil(t, hijackMsg.err)
}

// ============================================================
// DAG view: RunInspectView
// ============================================================

// TestRunInspectView_DKeyEntersDAGMode verifies that pressing 'd' switches to
// DAG view mode and syncs dagCursor from the list cursor.
func TestRunInspectView_DKeyEntersDAGMode(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 1

	next, cmd := rv.Update(tea.KeyPressMsg{Code: 'd'})
	assert.Nil(t, cmd)
	nv := next.(*RunInspectView)
	assert.Equal(t, dagViewModeDAG, nv.viewMode, "pressing d should activate DAG mode")
	assert.Equal(t, 1, nv.dagCursor, "dagCursor should sync from cursor")
}

// TestRunInspectView_LKeyEntersListMode verifies that pressing 'l' switches
// back to list mode and syncs list cursor from dagCursor.
func TestRunInspectView_LKeyEntersListMode(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 2

	next, cmd := rv.Update(tea.KeyPressMsg{Code: 'l'})
	assert.Nil(t, cmd)
	nv := next.(*RunInspectView)
	assert.Equal(t, dagViewModeList, nv.viewMode, "pressing l should return to list mode")
	assert.Equal(t, 2, nv.cursor, "list cursor should sync from dagCursor")
}

// TestRunInspectView_DAGViewRendersTree verifies that the DAG view renders the
// workflow name as root and task labels as children.
func TestRunInspectView_DAGViewRendersTree(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG

	out := rv.View()

	// Root should be the workflow name.
	assert.Contains(t, out, "code-review")
	// All task labels should appear.
	assert.Contains(t, out, "fetch-deps")
	assert.Contains(t, out, "review-auth")
	assert.Contains(t, out, "deploy")
}

// TestRunInspectView_DAGViewShowsStatusGlyphs verifies that status glyphs are
// present in the DAG view output.
func TestRunInspectView_DAGViewShowsStatusGlyphs(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG

	out := rv.View()

	// ● is used for running/finished, ○ for pending.
	assert.Contains(t, out, "●")
	assert.Contains(t, out, "○")
}

// TestRunInspectView_DAGViewDetailPanel verifies that the selected-node detail
// panel renders below the tree.
func TestRunInspectView_DAGViewDetailPanel(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 1 // review-auth (running, attempt #1)

	out := rv.View()

	// Detail panel should show state and node label.
	assert.Contains(t, out, "State:")
	assert.Contains(t, out, "review-auth")
	assert.Contains(t, out, "Attempt: #1")
}

// TestRunInspectView_DAGNavDown verifies ↓ moves dagCursor in DAG mode.
func TestRunInspectView_DAGNavDown(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 0

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, next.(*RunInspectView).dagCursor)
}

// TestRunInspectView_DAGNavUp verifies ↑ moves dagCursor in DAG mode.
func TestRunInspectView_DAGNavUp(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 2

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, next.(*RunInspectView).dagCursor)
}

// TestRunInspectView_DAGNavDownClamps verifies dagCursor does not exceed task count.
func TestRunInspectView_DAGNavDownClamps(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 2 // last task index

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, next.(*RunInspectView).dagCursor, "dagCursor should not exceed last index")
}

// TestRunInspectView_DAGNavUpClamps verifies dagCursor does not go below zero.
func TestRunInspectView_DAGNavUpClamps(t *testing.T) {
	v := newRunInspectView("abc12345")
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 0

	next, _ := rv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, next.(*RunInspectView).dagCursor, "dagCursor should not go below zero")
}

// TestRunInspectView_DAGChatUsesDAGCursor verifies that pressing 'c' in DAG
// mode opens chat for the DAG-cursor node, not the list cursor node.
func TestRunInspectView_DAGChatUsesDAGCursor(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 2 // deploy
	rv.cursor = 0    // fetch-deps (should be ignored)

	_, cmd := rv.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)
	msg := cmd()
	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok)
	assert.Equal(t, "deploy", chatMsg.TaskID, "chat should use DAG cursor node in DAG mode")
}

// TestRunInspectView_ShortHelp_ContainsDagView verifies the 'd' and 'l'
// bindings appear in ShortHelp.
func TestRunInspectView_ShortHelp_ContainsDagView(t *testing.T) {
	v := newRunInspectView("abc12345")
	var descs []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		descs = append(descs, h.Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "dag view")
	assert.Contains(t, joined, "list view")
}

// TestRunInspectView_DAGViewDefaultIsListMode verifies that a freshly
// constructed view starts in list mode (not DAG mode).
func TestRunInspectView_DAGViewDefaultIsListMode(t *testing.T) {
	v := newRunInspectView("abc12345")
	assert.Equal(t, dagViewModeList, v.viewMode, "default view mode should be list")
}

// TestRunInspectView_renderDAGTaskDetail_NoAttempt verifies the detail panel
// omits the attempt line when LastAttempt is nil.
func TestRunInspectView_renderDAGTaskDetail_NoAttempt(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	label := "deploy"
	task := smithers.RunTask{
		NodeID: "deploy",
		Label:  &label,
		State:  smithers.TaskStatePending,
	}
	detail := v.renderDAGTaskDetail(task)
	assert.NotContains(t, detail, "Attempt")
}

// TestRunInspectView_renderDAGView_WorkflowNameAsRoot verifies that when a
// workflow name is present the tree root shows it.
func TestRunInspectView_renderDAGView_WorkflowNameAsRoot(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	v.inspection = fixtureInspection() // WorkflowName = "code-review"

	treeOut := v.renderDAGView()
	assert.Contains(t, treeOut, "code-review")
}

// TestRunInspectView_renderDAGView_RunIDFallback verifies that when
// WorkflowName is empty the run ID prefix is used as root.
func TestRunInspectView_renderDAGView_RunIDFallback(t *testing.T) {
	v := newRunInspectView("xyzfallback")
	v.width = 120
	v.height = 40
	label := "task-a"
	v.inspection = &smithers.RunInspection{
		RunSummary: smithers.RunSummary{
			RunID:  "xyzfallback",
			Status: smithers.RunStatusRunning,
		},
		Tasks: []smithers.RunTask{
			{NodeID: "task-a", Label: &label, State: smithers.TaskStatePending},
		},
	}

	treeOut := v.renderDAGView()
	// Root is truncated run ID (first 8 chars).
	assert.Contains(t, treeOut, "xyzfallb")
}

// ============================================================
// runs-node-inspector: Enter key on task row
// ============================================================

// TestRunInspectView_EnterEmitsOpenNodeInspectMsg verifies that pressing Enter
// on a task row emits an OpenNodeInspectMsg.
func TestRunInspectView_EnterEmitsOpenNodeInspectMsg(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.cursor = 1 // review-auth

	_, cmd := rv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "Enter should emit a command")

	msg := cmd()
	nodeMsg, ok := msg.(OpenNodeInspectMsg)
	require.True(t, ok, "Enter should emit OpenNodeInspectMsg, got %T", msg)
	assert.Equal(t, "abc12345", nodeMsg.RunID)
	assert.Equal(t, "review-auth", nodeMsg.NodeID)
}

// TestRunInspectView_EnterNoopWhenNoTasks verifies Enter is a no-op without tasks.
func TestRunInspectView_EnterNoopWhenNoTasks(t *testing.T) {
	v := newRunInspectView("abc12345")
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter with no inspection loaded should be a no-op")
}

// TestRunInspectView_EnterDAGModeUsesDAGCursor verifies Enter in DAG mode uses dagCursor.
func TestRunInspectView_EnterDAGModeUsesDAGCursor(t *testing.T) {
	v := newRunInspectView("abc12345")
	v.width = 120
	v.height = 40
	updated, _ := v.Update(runInspectLoadedMsg{inspection: fixtureInspection()})
	rv := updated.(*RunInspectView)
	rv.viewMode = dagViewModeDAG
	rv.dagCursor = 2 // deploy
	rv.cursor = 0    // fetch-deps (should be ignored)

	_, cmd := rv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	nodeMsg, ok := msg.(OpenNodeInspectMsg)
	require.True(t, ok)
	assert.Equal(t, "deploy", nodeMsg.NodeID, "Enter in DAG mode should use dagCursor")
}

// TestRunInspectView_ShortHelp_ContainsEnterBinding verifies "node detail" is in ShortHelp.
func TestRunInspectView_ShortHelp_ContainsEnterBinding(t *testing.T) {
	v := newRunInspectView("abc12345")
	var descs []string
	for _, b := range v.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}
	assert.Contains(t, strings.Join(descs, " "), "node detail")
}

// ============================================================
// NodeInspectView
// ============================================================

// newNodeInspectView creates a NodeInspectView for tests.
func newNodeInspectView(runID string, task smithers.RunTask) *NodeInspectView {
	c := smithers.NewClient()
	return NewNodeInspectView(c, runID, task)
}

// TestNodeInspectView_ImplementsView verifies the interface.
func TestNodeInspectView_ImplementsView(t *testing.T) {
	var _ View = (*NodeInspectView)(nil)
}

// TestNodeInspectView_InitReturnsNil verifies Init is a no-op.
func TestNodeInspectView_InitReturnsNil(t *testing.T) {
	label := "my-task"
	task := smithers.RunTask{NodeID: "node-a", Label: &label, State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)
	cmd := v.Init()
	assert.Nil(t, cmd, "Init should return nil")
}

// TestNodeInspectView_EscEmitsPopMsg verifies Esc pops the view.
func TestNodeInspectView_EscEmitsPopMsg(t *testing.T) {
	label := "my-task"
	task := smithers.RunTask{NodeID: "node-a", Label: &label, State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

// TestNodeInspectView_QEmitsPopMsg verifies 'q' pops the view.
func TestNodeInspectView_QEmitsPopMsg(t *testing.T) {
	label := "my-task"
	task := smithers.RunTask{NodeID: "node-a", Label: &label, State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'q'})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "q should emit PopViewMsg")
}

// TestNodeInspectView_CEmitsLiveChatMsg verifies 'c' opens live chat.
func TestNodeInspectView_CEmitsLiveChatMsg(t *testing.T) {
	label := "my-task"
	task := smithers.RunTask{NodeID: "node-a", Label: &label, State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)
	msg := cmd()
	chatMsg, ok := msg.(OpenLiveChatMsg)
	require.True(t, ok, "c should emit OpenLiveChatMsg, got %T", msg)
	assert.Equal(t, "abc12345", chatMsg.RunID)
	assert.Equal(t, "node-a", chatMsg.TaskID)
}

// TestNodeInspectView_View_ContainsNodeID verifies node ID appears in view.
func TestNodeInspectView_View_ContainsNodeID(t *testing.T) {
	label := "my-task"
	task := smithers.RunTask{NodeID: "node-a", Label: &label, State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(120, 40)

	out := v.View()
	assert.Contains(t, out, "node-a")
	assert.Contains(t, out, "my-task")
}

// TestNodeInspectView_View_ContainsRunID verifies run ID appears in view.
func TestNodeInspectView_View_ContainsRunID(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-b", State: smithers.TaskStatePending}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(120, 40)

	out := v.View()
	assert.Contains(t, out, "abc12345")
}

// TestNodeInspectView_View_ContainsState verifies task state appears in view.
func TestNodeInspectView_View_ContainsState(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-c", State: smithers.TaskStateFailed}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(120, 40)

	out := v.View()
	assert.Contains(t, out, "failed")
}

// TestNodeInspectView_View_AttemptShown verifies attempt number is shown.
func TestNodeInspectView_View_AttemptShown(t *testing.T) {
	attempt := 3
	task := smithers.RunTask{NodeID: "node-d", State: smithers.TaskStateFailed, LastAttempt: &attempt}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(120, 40)

	out := v.View()
	assert.Contains(t, out, "#3")
}

// TestNodeInspectView_View_BackHint verifies help bar shows back hint.
func TestNodeInspectView_View_BackHint(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-e", State: smithers.TaskStatePending}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(120, 40)

	out := v.View()
	assert.Contains(t, out, "back")
}

// TestNodeInspectView_Name verifies Name returns "nodeinspect".
func TestNodeInspectView_Name(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-x", State: smithers.TaskStatePending}
	v := newNodeInspectView("abc12345", task)
	assert.Equal(t, "nodeinspect", v.Name())
}

// TestNodeInspectView_SetSize verifies SetSize stores dimensions.
func TestNodeInspectView_SetSize(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-x", State: smithers.TaskStatePending}
	v := newNodeInspectView("abc12345", task)
	v.SetSize(80, 24)
	assert.Equal(t, 80, v.width)
	assert.Equal(t, 24, v.height)
}

// TestNodeInspectView_WindowSizeMsgStored verifies tea.WindowSizeMsg updates dimensions.
func TestNodeInspectView_WindowSizeMsgStored(t *testing.T) {
	task := smithers.RunTask{NodeID: "node-y", State: smithers.TaskStateRunning}
	v := newNodeInspectView("abc12345", task)
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nv := updated.(*NodeInspectView)
	assert.Equal(t, 100, nv.width)
	assert.Equal(t, 30, nv.height)
}

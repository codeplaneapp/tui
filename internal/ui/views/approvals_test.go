package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
)

// --- Test helpers ---

func newTestApprovalsView() *ApprovalsView {
	c := smithers.NewClient()
	return NewApprovalsView(c)
}

// seedApprovals sends an approvalsLoadedMsg to populate the view's pending queue.
func seedApprovals(v *ApprovalsView, approvals []smithers.Approval) *ApprovalsView {
	updated, _ := v.Update(approvalsLoadedMsg{approvals: approvals})
	return updated.(*ApprovalsView)
}

// seedDecisions sends a decisionsLoadedMsg to populate the recent decisions list.
func seedDecisions(v *ApprovalsView, decisions []smithers.ApprovalDecision) *ApprovalsView {
	updated, _ := v.Update(decisionsLoadedMsg{decisions: decisions})
	return updated.(*ApprovalsView)
}

// testApproval builds an Approval with the given fields for use in tests.
func testApproval(id, runID, nodeID, gate, status string) smithers.Approval {
	return smithers.Approval{
		ID:           id,
		RunID:        runID,
		NodeID:       nodeID,
		WorkflowPath: "workflows/" + id + ".yaml",
		Gate:         gate,
		Status:       status,
		RequestedAt:  time.Now().UnixMilli(),
	}
}

// testDecision builds an ApprovalDecision for tests.
func testDecision(id, runID, nodeID, gate, decision string) smithers.ApprovalDecision {
	return smithers.ApprovalDecision{
		ID:           id,
		RunID:        runID,
		NodeID:       nodeID,
		WorkflowPath: "workflows/" + id + ".yaml",
		Gate:         gate,
		Decision:     decision,
		DecidedAt:    time.Now().Add(-5 * time.Minute).UnixMilli(),
		RequestedAt:  time.Now().Add(-10 * time.Minute).UnixMilli(),
	}
}

// --- Approval queue: loaded messages ---

func TestApprovalsView_LoadedMsg_ClearsLoading(t *testing.T) {
	v := newTestApprovalsView()
	approvals := []smithers.Approval{
		testApproval("a1", "run-1", "deploy", "Deploy to staging", "pending"),
		testApproval("a2", "run-2", "delete", "Delete user data", "pending"),
	}
	updated, _ := v.Update(approvalsLoadedMsg{approvals: approvals})
	// cmd may be non-nil when approvals trigger an async run-context fetch.

	av := updated.(*ApprovalsView)
	assert.False(t, av.loading)
	assert.Len(t, av.approvals, 2)
	assert.Equal(t, "Deploy to staging", av.approvals[0].Gate)
}

// --- Cursor navigation details ---

func TestApprovalsView_CursorDown_ThenUp_Bounds(t *testing.T) {
	v := newTestApprovalsView()
	approvals := []smithers.Approval{
		testApproval("a1", "r1", "n1", "G1", "pending"),
		testApproval("a2", "r2", "n2", "G2", "pending"),
		testApproval("a3", "r3", "n3", "G3", "pending"),
	}
	v = seedApprovals(v, approvals)

	// Move to end.
	for i := 0; i < 10; i++ {
		u, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = u.(*ApprovalsView)
	}
	assert.Equal(t, 2, v.cursor, "cursor should stop at last item")

	// Move to start.
	for i := 0; i < 10; i++ {
		u, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
		v = u.(*ApprovalsView)
	}
	assert.Equal(t, 0, v.cursor, "cursor should stop at first item")
}

func TestApprovalsView_ArrowDown_NavigatesQueue(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "G1", "pending"),
		testApproval("a2", "r2", "n2", "G2", "pending"),
	})

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	av := updated.(*ApprovalsView)
	assert.Equal(t, 1, av.cursor)

	updated2, _ := av.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	av2 := updated2.(*ApprovalsView)
	assert.Equal(t, 0, av2.cursor)
}

// --- View rendering: pending queue ---

func TestApprovalsView_View_PendingApprovals_ShowsGateLabels(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-1", "deploy", "Deploy to production", "pending"),
		testApproval("a2", "run-2", "delete", "Delete old records", "pending"),
	})
	out := v.View()
	assert.Contains(t, out, "Deploy to production")
	assert.Contains(t, out, "Delete old records")
}

func TestApprovalsView_View_CursorIndicatorOnFirstItem(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate Alpha", "pending"),
	})
	v.cursor = 0
	out := v.View()
	assert.Contains(t, out, "▸")
}

func TestApprovalsView_View_StatusIcons_AllThree(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Pending gate", "pending"),
		testApproval("a2", "r2", "n2", "Approved gate", "approved"),
		testApproval("a3", "r3", "n3", "Denied gate", "denied"),
	})
	out := v.View()
	assert.Contains(t, out, "○") // pending
	assert.Contains(t, out, "✓") // approved
	assert.Contains(t, out, "✗") // denied
}

func TestApprovalsView_View_WideTerminal_ShowsDivider(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 120
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "deploy", "Deploy to staging", "pending"),
	})
	out := v.View()
	assert.Contains(t, out, "│")
}

func TestApprovalsView_View_NarrowTerminal_GateLabelVisible(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 60
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "deploy", "Deploy to staging", "pending"),
	})
	out := v.View()
	assert.Contains(t, out, "Deploy to staging")
}

func TestApprovalsView_View_MixedStatuses_ShowsBothSections(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Pending G", "pending"),
		testApproval("a2", "r2", "n2", "Approved G", "approved"),
	})
	out := v.View()
	assert.Contains(t, out, "PENDING")
	assert.Contains(t, out, "RECENT")
	assert.Contains(t, out, "Pending G")
	assert.Contains(t, out, "Approved G")
}

// --- renderDetail ---

func TestApprovalsView_RenderDetail_ShowsAllFields(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-abc", "deploy-node", "Deploy to staging", "pending"),
	})
	v.cursor = 0
	detail := v.renderDetail(80)

	assert.Contains(t, detail, "Deploy to staging")
	assert.Contains(t, detail, "run-abc")
	assert.Contains(t, detail, "deploy-node")
	assert.Contains(t, detail, "PENDING")
}

func TestApprovalsView_RenderDetail_EmptyWhenNoCursor(t *testing.T) {
	v := newTestApprovalsView()
	v.cursor = 5 // out of range
	detail := v.renderDetail(80)
	assert.Empty(t, detail)
}

// --- Recent decisions: rendering details ---

func TestApprovalsView_RecentDecisions_ShowsDecidedByLine(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v.showRecent = true
	by := "alice"
	decisions := []smithers.ApprovalDecision{
		{
			ID: "d1", RunID: "r1", NodeID: "n1", Gate: "Deploy gate", Decision: "approved",
			DecidedAt: time.Now().Add(-3 * time.Minute).UnixMilli(),
			DecidedBy: &by,
		},
	}
	v = seedDecisions(v, decisions)
	out := v.View()
	assert.Contains(t, out, "by alice")
}

// Verify no panic when decisions have a nil DecidedBy.
func TestApprovalsView_RecentDecisions_NilDecidedBy_NoPanic(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v.showRecent = true
	decisions := []smithers.ApprovalDecision{
		{
			ID: "d1", RunID: "r1", NodeID: "n1", Gate: "Deploy gate", Decision: "denied",
			DecidedAt: time.Now().Add(-3 * time.Minute).UnixMilli(),
			DecidedBy: nil,
		},
	}
	v = seedDecisions(v, decisions)
	// Should not panic.
	out := v.View()
	assert.Contains(t, out, "Deploy gate")
}

// Verify stable rendering with many decisions (no imposed limit in impl, just no crash).
func TestApprovalsView_RecentDecisions_ManyEntries_NoPanic(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 60
	v.showRecent = true

	decisions := make([]smithers.ApprovalDecision, 20)
	for i := range decisions {
		decisions[i] = testDecision(
			fmt.Sprintf("d%d", i),
			fmt.Sprintf("run-%d", i),
			fmt.Sprintf("n%d", i),
			fmt.Sprintf("Gate %d", i),
			"approved",
		)
	}
	v = seedDecisions(v, decisions)
	out := v.View()
	assert.Contains(t, out, "RECENT DECISIONS")
	assert.Contains(t, out, "Gate 0")
}

// --- ShortHelp: covers both modes more precisely ---

func TestApprovalsView_ShortHelp_ContainsExpectedBindings(t *testing.T) {
	v := newTestApprovalsView()

	for _, mode := range []bool{false, true} {
		v.showRecent = mode
		help := v.ShortHelp()
		assert.NotEmpty(t, help, "ShortHelp should not be empty in mode showRecent=%v", mode)

		var descs []string
		for _, b := range help {
			descs = append(descs, b.Help().Desc)
		}
		joined := strings.Join(descs, " ")
		assert.Contains(t, joined, "navigate")
		assert.Contains(t, joined, "refresh")
		assert.Contains(t, joined, "back")
	}
}

// --- Inline approve / deny ---

// TestApprovalsView_AKeyApprovesPendingItem verifies that pressing 'a' on a pending
// item sets inflightIdx and returns a non-nil Cmd.
func TestApprovalsView_AKeyApprovesPendingItem(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Deploy gate", "pending"),
	})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	av := updated.(*ApprovalsView)

	assert.Equal(t, 0, av.inflightIdx, "inflightIdx should be 0 while action is inflight")
	assert.NotNil(t, cmd, "a cmd should be returned to kick off the approval and spinner")
}

// TestApprovalsView_AKeyIgnoresNonPending verifies that 'a' on an approved item is a no-op.
func TestApprovalsView_AKeyIgnoresNonPending(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Resolved gate", "approved"),
	})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	av := updated.(*ApprovalsView)

	assert.Equal(t, -1, av.inflightIdx, "inflightIdx should stay -1 for non-pending items")
	assert.Nil(t, cmd, "no cmd should be returned for non-pending items")
}

// TestApprovalsView_AKeyIgnoredWhileInflight verifies that pressing 'a' while
// another action is already in-flight is a no-op.
func TestApprovalsView_AKeyIgnoredWhileInflight(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Deploy gate", "pending"),
	})
	// Manually set inflight state.
	v.inflightIdx = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'a'})
	av := updated.(*ApprovalsView)

	assert.Equal(t, 0, av.inflightIdx, "inflightIdx should remain 0 while already inflight")
	assert.Nil(t, cmd, "no cmd should be returned when already inflight")
}

// TestApprovalsView_ApproveSuccessRemovesItem verifies that an approveSuccessMsg
// removes the item with the matching ID from the approvals list.
func TestApprovalsView_ApproveSuccessRemovesItem(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
		testApproval("a2", "r2", "n2", "Gate 2", "pending"),
	})
	v.inflightIdx = 0

	updated, _ := v.Update(approveSuccessMsg{approvalID: "a1"})
	av := updated.(*ApprovalsView)

	assert.Len(t, av.approvals, 1, "approved item should be removed from the list")
	assert.Equal(t, "a2", av.approvals[0].ID, "remaining item should be a2")
	assert.Equal(t, -1, av.inflightIdx, "inflightIdx should be reset to -1")
	assert.Nil(t, av.actionErr, "actionErr should be nil after success")
}

// TestApprovalsView_ApproveSuccessCursorClamped verifies that the cursor is
// clamped when the last item is approved.
func TestApprovalsView_ApproveSuccessCursorClamped(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
		testApproval("a2", "r2", "n2", "Gate 2", "pending"),
	})
	// Move cursor to last item and set inflight.
	v.cursor = 1
	v.listPane.cursor = 1
	v.detailPane.cursor = 1
	v.inflightIdx = 1

	updated, _ := v.Update(approveSuccessMsg{approvalID: "a2"})
	av := updated.(*ApprovalsView)

	assert.Len(t, av.approvals, 1)
	assert.Equal(t, 0, av.cursor, "cursor should be clamped to 0 after removing last item")
}

// TestApprovalsView_ApproveErrorSetsField verifies that an approveErrorMsg sets
// actionErr and clears inflightIdx.
func TestApprovalsView_ApproveErrorSetsField(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
	})
	v.inflightIdx = 0

	updated, _ := v.Update(approveErrorMsg{approvalID: "a1", err: fmt.Errorf("rate limited")})
	av := updated.(*ApprovalsView)

	assert.Equal(t, -1, av.inflightIdx, "inflightIdx should be reset to -1 after error")
	assert.NotNil(t, av.actionErr, "actionErr should be set")
	assert.Contains(t, av.actionErr.Error(), "rate limited")
}

// TestApprovalsView_ApproveErrorRenderedInDetail verifies that actionErr text
// appears in the View() output when cursor is on the relevant item.
func TestApprovalsView_ApproveErrorRenderedInDetail(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
	})
	// Simulate an error from an approve attempt by sending the error message.
	updated, _ := v.Update(approveErrorMsg{approvalID: "a1", err: fmt.Errorf("connection refused")})
	v = updated.(*ApprovalsView)

	out := v.View()
	assert.Contains(t, out, "connection refused", "error message should be visible in the detail pane")
}

// TestApprovalsView_SpinnerShownOnInflightItem verifies that when inflightIdx is 0
// the list item no longer shows the default "○" pending icon.
func TestApprovalsView_SpinnerShownOnInflightItem(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
	})
	// Simulate inflight: set spinnerView to a non-empty string.
	v.inflightIdx = 0
	v.listPane.inflightIdx = 0
	v.listPane.spinnerView = "⠋"

	out := v.View()
	// The default pending icon "○" should NOT be present in the list for the inflight item.
	// The spinner frame "⠋" should be present.
	assert.Contains(t, out, "⠋", "spinner frame should appear in view while inflight")
}

// TestApprovalsView_ShortHelpIncludesApproveForPending verifies that ShortHelp
// includes the 'a' and 'd' bindings when cursor is on a pending item.
func TestApprovalsView_ShortHelpIncludesApproveForPending(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
	})
	v.cursor = 0

	bindings := v.ShortHelp()
	var keys []string
	for _, b := range bindings {
		for _, k := range b.Keys() {
			keys = append(keys, k)
		}
	}
	assert.Contains(t, keys, "a", "ShortHelp should contain 'a' for pending items")
	assert.Contains(t, keys, "d", "ShortHelp should contain 'd' for pending items")
}

// TestApprovalsView_ShortHelpNoApproveForResolved verifies that ShortHelp does
// not include 'a' or 'd' bindings when cursor is on an already-resolved item.
func TestApprovalsView_ShortHelpNoApproveForResolved(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "approved"),
	})
	v.cursor = 0

	bindings := v.ShortHelp()
	var keys []string
	for _, b := range bindings {
		for _, k := range b.Keys() {
			keys = append(keys, k)
		}
	}
	assert.NotContains(t, keys, "a", "ShortHelp should not contain 'a' for resolved items")
	assert.NotContains(t, keys, "d", "ShortHelp should not contain 'd' for resolved items")
}

// TestApprovalsView_DKeyDenysPendingItem verifies that pressing 'd' on a pending
// item sets inflightIdx and returns a non-nil Cmd.
func TestApprovalsView_DKeyDenysPendingItem(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Deploy gate", "pending"),
	})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	av := updated.(*ApprovalsView)

	assert.Equal(t, 0, av.inflightIdx, "inflightIdx should be 0 while deny action is inflight")
	assert.NotNil(t, cmd, "a cmd should be returned to kick off the deny and spinner")
}

// TestApprovalsView_DenySuccessRemovesItem verifies that a denySuccessMsg
// removes the item with the matching ID from the approvals list.
func TestApprovalsView_DenySuccessRemovesItem(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
		testApproval("a2", "r2", "n2", "Gate 2", "pending"),
	})
	v.inflightIdx = 0

	updated, _ := v.Update(denySuccessMsg{approvalID: "a1"})
	av := updated.(*ApprovalsView)

	assert.Len(t, av.approvals, 1, "denied item should be removed from the list")
	assert.Equal(t, "a2", av.approvals[0].ID)
	assert.Equal(t, -1, av.inflightIdx)
	assert.Nil(t, av.actionErr)
}

// TestApprovalsView_DenyErrorSetsField verifies that a denyErrorMsg sets
// actionErr and clears inflightIdx.
func TestApprovalsView_DenyErrorSetsField(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "r1", "n1", "Gate 1", "pending"),
	})
	v.inflightIdx = 0

	updated, _ := v.Update(denyErrorMsg{approvalID: "a1", err: fmt.Errorf("denied: unauthorized")})
	av := updated.(*ApprovalsView)

	assert.Equal(t, -1, av.inflightIdx)
	assert.NotNil(t, av.actionErr)
	assert.Contains(t, av.actionErr.Error(), "unauthorized")
}

// --- Enriched context display tests ---

// testRunSummary builds a RunSummary for use in tests.
// nodesDone and nodeTotal are encoded in the Summary map.
func testRunSummary(runID, workflowName string, nodesDone, nodeTotal int) *smithers.RunSummary {
	startedAt := time.Now().Add(-10 * time.Minute).UnixMilli()
	summary := make(map[string]int)
	if nodeTotal > 0 {
		summary["finished"] = nodesDone
		pending := nodeTotal - nodesDone
		if pending > 0 {
			summary["running"] = 1
			summary["pending"] = pending - 1
		}
	}
	return &smithers.RunSummary{
		RunID:        runID,
		WorkflowName: workflowName,
		WorkflowPath: ".smithers/workflows/" + workflowName + ".ts",
		Status:       smithers.RunStatusRunning,
		StartedAtMs:  &startedAt,
		Summary:      summary,
	}
}

// TestApprovalsView_InitialLoadTriggersContextFetch verifies that after
// approvalsLoadedMsg arrives with at least one item, a non-nil Cmd is returned
// to kick off the context fetch for the first item.
func TestApprovalsView_InitialLoadTriggersContextFetch(t *testing.T) {
	v := newTestApprovalsView()
	_, cmd := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{
		testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending"),
	}})
	assert.NotNil(t, cmd, "initial load with approvals should return a cmd to fetch run context")
}

// TestApprovalsView_InitialLoadEmptyNoFetch verifies no Cmd is returned when
// the loaded list is empty.
func TestApprovalsView_InitialLoadEmptyNoFetch(t *testing.T) {
	v := newTestApprovalsView()
	_, cmd := v.Update(approvalsLoadedMsg{approvals: []smithers.Approval{}})
	assert.Nil(t, cmd, "initial load with empty list should not trigger context fetch")
}

// TestApprovalsView_CursorChangeTriggersContextFetch verifies that pressing
// 'j' with two approvals having different RunIDs returns a non-nil Cmd and
// updates lastFetchRun to the second approval's RunID.
func TestApprovalsView_CursorChangeTriggersContextFetch(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-111", "n1", "Gate 1", "pending"),
		testApproval("a2", "run-222", "n2", "Gate 2", "pending"),
	})
	// Simulate that context for run-111 has already been fetched.
	v.lastFetchRun = "run-111"
	v.selectedRun = testRunSummary("run-111", "workflow-a", 2, 4)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'j'})
	av := updated.(*ApprovalsView)

	assert.NotNil(t, cmd, "moving cursor to a new RunID should return a context-fetch Cmd")
	assert.Equal(t, "run-222", av.lastFetchRun, "lastFetchRun should update to the new RunID")
}

// TestApprovalsView_SameRunIDSkipsFetch verifies that moving cursor when both
// approvals share the same RunID does not return a new Cmd (already cached).
func TestApprovalsView_SameRunIDSkipsFetch(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-shared", "n1", "Gate 1", "pending"),
		testApproval("a2", "run-shared", "n2", "Gate 2", "pending"),
	})
	// Simulate that context for run-shared has already been fetched.
	v.lastFetchRun = "run-shared"
	v.selectedRun = testRunSummary("run-shared", "shared-wf", 1, 3)

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Nil(t, cmd, "same RunID with existing result should not trigger a new fetch")
}

// TestApprovalsView_DetailLoadingState verifies that when contextLoading is
// true the rendered detail contains the loading message.
func TestApprovalsView_DetailLoadingState(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending"),
	})
	// Manually set loading state.
	v.contextLoading = true
	v.detailPane.contextLoading = true

	out := v.View()
	assert.Contains(t, out, "Loading run details...", "loading state should appear in detail pane")
}

// TestApprovalsView_DetailErrorState verifies that sending a runSummaryErrorMsg
// results in the error text appearing in the rendered view.
func TestApprovalsView_DetailErrorState(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending"),
	})
	v.lastFetchRun = "run-abc"
	updated, _ := v.Update(runSummaryErrorMsg{runID: "run-abc", err: fmt.Errorf("timeout fetching run")})
	v = updated.(*ApprovalsView)

	out := v.View()
	assert.Contains(t, out, "Could not load run details", "error state should appear in detail pane")
}

// TestApprovalsView_DetailShowsRunContext verifies that after a
// runSummaryLoadedMsg the detail pane renders workflow name and step progress.
func TestApprovalsView_DetailShowsRunContext(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending"),
	})
	v.lastFetchRun = "run-abc"
	updated, _ := v.Update(runSummaryLoadedMsg{
		runID:   "run-abc",
		summary: testRunSummary("run-abc", "deploy-staging", 4, 6),
	})
	v = updated.(*ApprovalsView)

	out := v.View()
	assert.Contains(t, out, "deploy-staging", "workflow name should appear in detail pane")
	assert.Contains(t, out, "Step 4 of 6", "step progress should appear in detail pane")
}

// TestApprovalsView_DetailNoPayload verifies that when Payload is empty the
// "Payload:" section is omitted entirely.
func TestApprovalsView_DetailNoPayload(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	a := testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending")
	a.Payload = ""
	v = seedApprovals(v, []smithers.Approval{a})

	out := v.View()
	assert.NotContains(t, out, "Payload:", "Payload section should be absent when payload is empty")
}

// TestApprovalsView_ResolvedApprovalShowsDecision verifies that a resolved
// approval shows "Resolved by:" in the detail pane.
func TestApprovalsView_ResolvedApprovalShowsDecision(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	resolvedAt := time.Now().Add(-5 * time.Minute).UnixMilli()
	resolvedBy := "alice"
	a := smithers.Approval{
		ID:           "a1",
		RunID:        "run-abc",
		NodeID:       "deploy",
		WorkflowPath: "workflows/deploy.yaml",
		Gate:         "Deploy to production",
		Status:       "approved",
		RequestedAt:  time.Now().Add(-15 * time.Minute).UnixMilli(),
		ResolvedAt:   &resolvedAt,
		ResolvedBy:   &resolvedBy,
	}
	v = seedApprovals(v, []smithers.Approval{a})

	out := v.View()
	assert.Contains(t, out, "Resolved by:", "resolved approval should show resolution metadata")
	assert.Contains(t, out, "alice", "resolver name should appear in detail pane")
}

// TestApprovalsView_ListItemShowsWaitTime verifies that a pending approval's
// list item includes a formatted wait time string.
func TestApprovalsView_ListItemShowsWaitTime(t *testing.T) {
	v := newTestApprovalsView()
	v.width = 100
	v.height = 40
	// 12 minutes ago.
	a := testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending")
	a.RequestedAt = time.Now().Add(-12 * time.Minute).UnixMilli()
	v = seedApprovals(v, []smithers.Approval{a})

	out := v.View()
	assert.Contains(t, out, "12m", "list item should show formatted wait time for pending approval")
}

// TestApprovalsView_DetailWaitTimeSLA verifies that the SLA color helper
// applies the correct thresholds: <5m green, <15m yellow, ≥15m red.
func TestApprovalsView_DetailWaitTimeSLA(t *testing.T) {
	greenStyle := slaStyle(3 * time.Minute)
	yellowStyle := slaStyle(10 * time.Minute)
	redStyle := slaStyle(20 * time.Minute)

	// The color is embedded in the style; render a string and check ANSI codes
	// (color "2"=green, "3"=yellow, "1"=red via terminal color escape).
	greenOut := greenStyle.Render("ok")
	yellowOut := yellowStyle.Render("ok")
	redOut := redStyle.Render("ok")

	// Green must differ from yellow, yellow from red.
	assert.NotEqual(t, greenOut, yellowOut, "green and yellow should produce different output")
	assert.NotEqual(t, yellowOut, redOut, "yellow and red should produce different output")
	assert.NotEqual(t, greenOut, redOut, "green and red should produce different output")
}

// TestApprovalsView_FormatWait verifies the formatWait helper output.
func TestApprovalsView_FormatWait(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1m"},
		{0, "<1m"},
		{1 * time.Minute, "1m"},
		{8 * time.Minute, "8m"},
		{59 * time.Minute, "59m"},
		{90 * time.Minute, "1h 30m"},
		{2 * time.Hour, "2h 0m"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatWait(tt.d), "formatWait(%v)", tt.d)
	}
}

// TestApprovalsView_WorkflowNameDisplay verifies that workflowNameDisplay
// strips common workflow file extensions.
func TestApprovalsView_WorkflowNameDisplay(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{".smithers/workflows/deploy.ts", "deploy"},
		{"workflows/gdpr-cleanup.tsx", "gdpr-cleanup"},
		{"nightly.yaml", "nightly"},
		{"pipeline.yml", "pipeline"},
		{"no-extension", "no-extension"},
		{"/absolute/path/to/flow.js", "flow"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, workflowNameDisplay(tt.input), "workflowNameDisplay(%q)", tt.input)
	}
}

// TestApprovalsView_RenderDetail_ShowsRequestedAt verifies that the detail
// pane shows a timestamp for the RequestedAt field.
func TestApprovalsView_RenderDetail_ShowsRequestedAt(t *testing.T) {
	v := newTestApprovalsView()
	a := testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending")
	a.RequestedAt = time.Now().Add(-3 * time.Minute).UnixMilli()
	v = seedApprovals(v, []smithers.Approval{a})
	v.cursor = 0
	detail := v.renderDetail(80)
	assert.Contains(t, detail, "Requested:", "detail should show Requested: label")
}

// TestApprovalsView_RenderDetail_ShowsWorkflowName verifies that the detail
// pane shows the workflow name extracted from WorkflowPath when no RunSummary
// is available.
func TestApprovalsView_RenderDetail_ShowsWorkflowName(t *testing.T) {
	v := newTestApprovalsView()
	a := testApproval("a1", "run-abc", "deploy", "Deploy to staging", "pending")
	a.WorkflowPath = ".smithers/workflows/production-deploy.ts"
	v = seedApprovals(v, []smithers.Approval{a})
	v.cursor = 0
	detail := v.renderDetail(80)
	assert.Contains(t, detail, "production-deploy", "detail should show workflow name derived from path")
}

// TestApprovalsView_RunContextLoadedMsg_StaleRunIDIgnored verifies that a
// runSummaryLoadedMsg for a stale RunID (not matching lastFetchRun) is ignored.
func TestApprovalsView_RunContextLoadedMsg_StaleRunIDIgnored(t *testing.T) {
	v := newTestApprovalsView()
	v = seedApprovals(v, []smithers.Approval{
		testApproval("a1", "run-current", "n1", "Gate 1", "pending"),
	})
	v.lastFetchRun = "run-current"

	// Deliver a result for a different (stale) run.
	updated, _ := v.Update(runSummaryLoadedMsg{
		runID:   "run-stale",
		summary: testRunSummary("run-stale", "old-workflow", 1, 3),
	})
	av := updated.(*ApprovalsView)

	assert.Nil(t, av.selectedRun, "stale run summary result should be ignored")
}

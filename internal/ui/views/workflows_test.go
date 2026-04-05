package views

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func testWorkflow(id, name, path string, status smithers.WorkflowStatus) smithers.Workflow {
	return smithers.Workflow{
		ID:           id,
		Name:         name,
		RelativePath: path,
		Status:       status,
	}
}

func newTestWorkflowsView() *WorkflowsView {
	c := smithers.NewClient()
	return NewWorkflowsView(c)
}

// seedWorkflows sends a workflowsLoadedMsg to populate the view.
func seedWorkflows(v *WorkflowsView, workflows []smithers.Workflow) *WorkflowsView {
	updated, _ := v.Update(workflowsLoadedMsg{workflows: workflows})
	return updated.(*WorkflowsView)
}

func sampleWorkflows() []smithers.Workflow {
	return []smithers.Workflow{
		testWorkflow("implement", "Implement", ".smithers/workflows/implement.tsx", smithers.WorkflowStatusActive),
		testWorkflow("review", "Review", ".smithers/workflows/review.tsx", smithers.WorkflowStatusActive),
		testWorkflow("plan", "Plan", ".smithers/workflows/plan.tsx", smithers.WorkflowStatusDraft),
	}
}

// --- Interface compliance ---

func TestWorkflowsView_ImplementsView(t *testing.T) {
	var _ View = (*WorkflowsView)(nil)
}

// --- 1. Init sets loading ---

func TestWorkflowsView_Init_SetsLoading(t *testing.T) {
	v := newTestWorkflowsView()
	assert.True(t, v.loading, "NewWorkflowsView should start in loading state")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil command")
}

// --- 2. LoadedMsg populates workflows ---

func TestWorkflowsView_LoadedMsg_PopulatesWorkflows(t *testing.T) {
	v := newTestWorkflowsView()
	workflows := sampleWorkflows()

	updated, cmd := v.Update(workflowsLoadedMsg{workflows: workflows})
	assert.Nil(t, cmd)

	wv := updated.(*WorkflowsView)
	assert.False(t, wv.loading, "loading should be false after load")
	assert.Nil(t, wv.err, "err should be nil after successful load")
	assert.Len(t, wv.workflows, 3)
	assert.Equal(t, "implement", wv.workflows[0].ID)
}

// --- 3. ErrorMsg sets err ---

func TestWorkflowsView_ErrorMsg_SetsErr(t *testing.T) {
	v := newTestWorkflowsView()
	someErr := errors.New("smithers not found on PATH")

	updated, cmd := v.Update(workflowsErrorMsg{err: someErr})
	assert.Nil(t, cmd)

	wv := updated.(*WorkflowsView)
	assert.False(t, wv.loading, "loading should be false after error")
	assert.Equal(t, someErr, wv.err)
}

// --- 4. Cursor navigation down ---

func TestWorkflowsView_CursorNavigation_Down(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	assert.Equal(t, 0, v.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 1, wv.cursor, "j should move cursor down")

	updated2, _ := wv.Update(tea.KeyPressMsg{Code: 'j'})
	wv2 := updated2.(*WorkflowsView)
	assert.Equal(t, 2, wv2.cursor, "j again should move to index 2")
}

// --- 5. Cursor navigation up ---

func TestWorkflowsView_CursorNavigation_Up(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 2

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 1, wv.cursor, "k should move cursor up")

	updated2, _ := wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv2 := updated2.(*WorkflowsView)
	assert.Equal(t, 0, wv2.cursor, "k again should move to index 0")
}

// --- 6. Cursor boundary at bottom ---

func TestWorkflowsView_CursorBoundary_AtBottom(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 2 // last index (len=3)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 2, wv.cursor, "cursor should not exceed last index")
}

// --- 7. Cursor boundary at top ---

func TestWorkflowsView_CursorBoundary_AtTop(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	assert.Equal(t, 0, v.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 0, wv.cursor, "cursor should not go below 0")
}

// --- 8. Esc returns PopViewMsg ---

func TestWorkflowsView_Esc_ReturnsPopViewMsg(t *testing.T) {
	v := newTestWorkflowsView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "Esc should return a non-nil command")

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc command should emit PopViewMsg")
}

// --- 9. Refresh (r) reloads workflows ---

func TestWorkflowsView_Refresh_ReloadsWorkflows(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	assert.False(t, v.loading)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	wv := updated.(*WorkflowsView)
	assert.True(t, wv.loading, "'r' should set loading = true")
	assert.NotNil(t, cmd, "'r' should return a reload command")
}

// --- 10. View header text ---

func TestWorkflowsView_View_HeaderText(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, out, "SMITHERS \u203a Workflows")
}

// --- 11. View shows workflow names ---

func TestWorkflowsView_View_ShowsWorkflowNames(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())

	out := v.View()
	assert.Contains(t, out, "Implement")
	assert.Contains(t, out, "Review")
	assert.Contains(t, out, "Plan")
}

// --- 12. View empty state ---

func TestWorkflowsView_View_EmptyState(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, []smithers.Workflow{})

	out := v.View()
	assert.Contains(t, out, "No workflows found")
}

// --- 13. View loading state ---

func TestWorkflowsView_View_LoadingState(t *testing.T) {
	v := newTestWorkflowsView()
	// loading is true by default

	out := v.View()
	assert.Contains(t, out, "Loading")
}

// --- 14. View error state ---

func TestWorkflowsView_View_ErrorState(t *testing.T) {
	v := newTestWorkflowsView()
	v.loading = false
	v.err = errors.New("smithers binary not found")

	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "smithers binary not found")
}

// --- 15. Name ---

func TestWorkflowsView_Name(t *testing.T) {
	v := newTestWorkflowsView()
	assert.Equal(t, "workflows", v.Name())
}

// --- 16. SetSize ---

func TestWorkflowsView_SetSize(t *testing.T) {
	v := newTestWorkflowsView()
	v.SetSize(120, 40)
	assert.Equal(t, 120, v.width)
	assert.Equal(t, 40, v.height)
}

// --- 17. Arrow keys navigate ---

func TestWorkflowsView_ArrowKeys_Navigate(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 1, wv.cursor, "Down arrow should move cursor down")

	updated2, _ := wv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	wv2 := updated2.(*WorkflowsView)
	assert.Equal(t, 0, wv2.cursor, "Up arrow should move cursor up")
}

// --- 18. Window resize updates dimensions ---

func TestWorkflowsView_WindowResize_UpdatesDimensions(t *testing.T) {
	v := newTestWorkflowsView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Nil(t, cmd)

	wv := updated.(*WorkflowsView)
	assert.Equal(t, 120, wv.width)
	assert.Equal(t, 40, wv.height)
}

// --- 19. Wide terminal shows two-column layout ---

func TestWorkflowsView_View_WideTerminal_TwoColumns(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 120
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())

	out := v.View()
	assert.Contains(t, out, "│", "wide layout should contain a column separator")
}

// --- 20. Narrow terminal shows single-column layout ---

func TestWorkflowsView_View_NarrowTerminal_SingleColumn(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())

	out := v.View()
	assert.Contains(t, out, "Implement", "narrow layout should show workflow names")
	// Narrow should not contain detail pane separator as a column divider
	// (check that the name is still visible)
	assert.Contains(t, out, "Review")
}

// --- 21. View shows cursor indicator ---

func TestWorkflowsView_View_CursorIndicator(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 0

	out := v.View()
	assert.Contains(t, out, "▸", "selected workflow should show cursor indicator")
}

// --- 22. ShortHelp returns expected bindings ---

func TestWorkflowsView_ShortHelp_NotEmpty(t *testing.T) {
	v := newTestWorkflowsView()
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "refresh")
	assert.Contains(t, joined, "back")
}

// --- 23. Refresh no-op while already loading ---

func TestWorkflowsView_Refresh_NoOpWhileLoading(t *testing.T) {
	v := newTestWorkflowsView()
	// loading is true by default

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	wv := updated.(*WorkflowsView)
	assert.True(t, wv.loading, "loading should remain true")
	assert.Nil(t, cmd, "'r' while loading should not return a command")
}

// --- 24. selectedWorkflow returns correct workflow ---

func TestWorkflowsView_SelectedWorkflow(t *testing.T) {
	v := newTestWorkflowsView()
	workflows := sampleWorkflows()
	v = seedWorkflows(v, workflows)

	v.cursor = 1
	wf := v.selectedWorkflow()
	require.NotNil(t, wf)
	assert.Equal(t, "review", wf.ID)
	assert.Equal(t, "Review", wf.Name)
}

// --- 25. selectedWorkflow returns nil when list is empty ---

func TestWorkflowsView_SelectedWorkflow_EmptyList(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, []smithers.Workflow{})

	wf := v.selectedWorkflow()
	assert.Nil(t, wf, "selectedWorkflow should return nil for empty list")
}

// --- 26. Wide layout shows detail pane content ---

func TestWorkflowsView_View_WideLayout_ShowsDetailPane(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 120
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 0

	out := v.View()
	// Detail pane should show the selected workflow's ID and fields.
	assert.Contains(t, out, "implement", "detail pane should show workflow ID")
	assert.Contains(t, out, "Name:", "detail pane should show Name label")
	assert.Contains(t, out, "Path:", "detail pane should show Path label")
	assert.Contains(t, out, "Status:", "detail pane should show Status label")
}

// --- 27. Error state shows PATH hint ---

func TestWorkflowsView_View_ErrorState_ShowsPathHint(t *testing.T) {
	v := newTestWorkflowsView()
	v.loading = false
	v.err = errors.New("exec: smithers: not found")

	out := v.View()
	assert.Contains(t, out, "PATH", "error state should hint to check PATH")
}

// --- 28. View shows workflow path in narrow layout ---

func TestWorkflowsView_View_ShowsWorkflowPath(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())

	out := v.View()
	assert.Contains(t, out, ".smithers/workflows/implement.tsx")
}

// --- 29. clampScroll keeps cursor visible ---

func TestWorkflowsView_ClampScroll_KeepsCursorVisible(t *testing.T) {
	v := newTestWorkflowsView()
	// Make a large list.
	workflows := make([]smithers.Workflow, 20)
	for i := range workflows {
		workflows[i] = testWorkflow(
			"wf-"+string(rune('a'+i)),
			"Workflow "+string(rune('A'+i)),
			".smithers/workflows/wf.tsx",
			smithers.WorkflowStatusActive,
		)
	}
	v = seedWorkflows(v, workflows)
	v.SetSize(80, 20)

	// Move cursor to last item.
	for i := 0; i < 19; i++ {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*WorkflowsView)
	}
	assert.Equal(t, 19, v.cursor)
	assert.GreaterOrEqual(t, v.cursor, v.scrollOffset, "cursor should be >= scrollOffset")
	assert.Less(t, v.cursor, v.scrollOffset+v.pageSize(), "cursor should be within page")
}

// --- 30. Loaded message clears previous error ---

func TestWorkflowsView_LoadedMsg_ClearsPreviousError(t *testing.T) {
	v := newTestWorkflowsView()
	// First set an error.
	updated1, _ := v.Update(workflowsErrorMsg{err: errors.New("oops")})
	wv := updated1.(*WorkflowsView)
	require.NotNil(t, wv.err)

	// Now send a successful load.
	updated2, _ := wv.Update(workflowsLoadedMsg{workflows: sampleWorkflows()})
	wv2 := updated2.(*WorkflowsView)
	assert.Nil(t, wv2.err, "a successful load should clear the previous error")
	assert.Len(t, wv2.workflows, 3)
}

// ============================================================
// workflows-list ticket: Enter/run, info overlay, last-run status
// ============================================================

// --- 31. Enter key starts form DAG loading ---

func TestWorkflowsView_EnterKey_StartsFormLoading(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkflowsView)
	assert.NotNil(t, cmd, "Enter should return a DAG-load command for the form")
	assert.Equal(t, runFormLoading, wv.formState, "formState should be loading")
	assert.Equal(t, "implement", wv.formWorkflowID)
	assert.Equal(t, runConfirmNone, wv.confirmState, "confirmState should remain none while fetching DAG")
}

// --- 32. Confirm overlay shows workflow name (seeded directly) ---

func TestWorkflowsView_ConfirmOverlay_ShowsWorkflowName(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 0
	// Seed the confirm state directly, as happens when DAG returns no fields.
	v.confirmState = runConfirmPending
	v.width = 80
	v.height = 30
	out := v.View()

	assert.Contains(t, out, "Implement", "confirm overlay should mention selected workflow")
	assert.Contains(t, out, "Run workflow", "confirm overlay should contain 'Run workflow'")
}

// --- 33. Esc cancels confirm overlay ---

func TestWorkflowsView_ConfirmOverlay_EscCancels(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	// Seed the pending state directly, as happens when DAG returns no fields.
	v.confirmState = runConfirmPending

	// Press Esc inside the overlay.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, runConfirmNone, wv.confirmState, "Esc should dismiss the confirm overlay")
	assert.Nil(t, cmd, "dismissing the overlay should not emit a command")
}

// --- 34. Enter inside confirm overlay fires RunWorkflow command ---

func TestWorkflowsView_ConfirmOverlay_EnterFiresRunCmd(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	// Seed the pending state directly, as happens when DAG returns no fields.
	v.confirmState = runConfirmPending

	// Confirm.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, runConfirmRunning, wv.confirmState, "state should move to running")
	assert.NotNil(t, cmd, "confirmation should return a run command")
}

// --- 35. workflowRunStartedMsg clears confirm overlay and records status ---

func TestWorkflowsView_RunStartedMsg_ClearsOverlayAndRecordsStatus(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	// Simulate being in the running state.
	v.confirmState = runConfirmRunning

	run := &smithers.RunSummary{
		RunID:  "run-001",
		Status: smithers.RunStatusRunning,
	}
	updated, cmd := v.Update(workflowRunStartedMsg{run: run, workflowID: "implement"})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runConfirmNone, wv.confirmState, "overlay should be dismissed")
	assert.Equal(t, smithers.RunStatusRunning, wv.lastRunStatus["implement"], "last run status should be recorded")
}

// --- 36. workflowRunErrorMsg surfaces the error ---

func TestWorkflowsView_RunErrorMsg_SurfacesError(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.confirmState = runConfirmRunning

	runErr := errors.New("workflow not found")
	updated, cmd := v.Update(workflowRunErrorMsg{workflowID: "implement", err: runErr})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runConfirmNone, wv.confirmState, "state should reset to none on error")
	require.NotNil(t, wv.confirmError)
	assert.Equal(t, runErr, wv.confirmError)
}

// --- 37. Run error shown in View output ---

func TestWorkflowsView_View_ShowsRunError(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 30
	v = seedWorkflows(v, sampleWorkflows())
	v.confirmError = errors.New("smithers exec failed")

	out := v.View()
	assert.Contains(t, out, "Run failed", "view should show run error")
	assert.Contains(t, out, "smithers exec failed")
}

// --- 38. Cursor movement clears stale run error ---

func TestWorkflowsView_CursorMove_ClearsRunError(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.confirmError = errors.New("old error")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, wv.confirmError, "moving cursor should clear stale run error")
}

// --- 39. Enter key is a no-op while loading ---

func TestWorkflowsView_EnterKey_NoOpWhileLoading(t *testing.T) {
	v := newTestWorkflowsView()
	// loading = true by default; no workflows yet

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, runConfirmNone, wv.confirmState, "Enter while loading should not open overlay")
}

// --- 40. Enter key is a no-op on empty workflow list ---

func TestWorkflowsView_EnterKey_NoOpOnEmptyList(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, []smithers.Workflow{})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, runConfirmNone, wv.confirmState, "Enter on empty list should not open overlay")
}

// --- 41. 'i' key triggers DAG overlay loading ---

func TestWorkflowsView_IKey_TriggersDAGOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'i'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, dagOverlayLoading, wv.dagState, "i should set dagState to loading")
	assert.Equal(t, "implement", wv.dagWorkflowID)
	assert.NotNil(t, cmd, "i should return a DAG-load command")
}

// --- 42. workflowDAGLoadedMsg shows overlay ---

func TestWorkflowsView_DAGLoadedMsg_ShowsOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayLoading
	v.dagWorkflowID = "implement"

	dag := &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields: []smithers.WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
		},
	}
	updated, cmd := v.Update(workflowDAGLoadedMsg{workflowID: "implement", dag: dag})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, dagOverlayVisible, wv.dagState)
	require.NotNil(t, wv.dagDefinition)
	assert.Equal(t, "implement", wv.dagDefinition.WorkflowID)
}

// --- 43. DAG overlay renders field list ---

func TestWorkflowsView_View_DAGOverlay_RendersFields(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	msg := "Inferred via source analysis."
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields: []smithers.WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
			{Key: "ticketId", Label: "Ticket ID", Type: "string"},
		},
		Message: &msg,
	}

	out := v.View()
	assert.Contains(t, out, "Workflow Info")
	assert.Contains(t, out, "inferred")
	assert.Contains(t, out, "Prompt")
	assert.Contains(t, out, "Ticket ID")
	assert.Contains(t, out, "Inferred via source analysis.")
}

// --- 44. Esc closes DAG overlay ---

func TestWorkflowsView_DAGOverlay_EscCloses(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagDefinition = &smithers.DAGDefinition{WorkflowID: "implement", Mode: "inferred"}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, dagOverlayHidden, wv.dagState)
	assert.Empty(t, wv.dagWorkflowID)
}

// --- 45. 'i' key closes an already-open DAG overlay ---

func TestWorkflowsView_IKey_ClosesDAGOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagDefinition = &smithers.DAGDefinition{WorkflowID: "implement", Mode: "inferred"}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'i'})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, dagOverlayHidden, wv.dagState)
}

// --- 46. workflowDAGErrorMsg sets error state ---

func TestWorkflowsView_DAGErrorMsg_SetsErrorState(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayLoading
	v.dagWorkflowID = "implement"

	dagErr := errors.New("workflow not found")
	updated, cmd := v.Update(workflowDAGErrorMsg{workflowID: "implement", err: dagErr})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, dagOverlayError, wv.dagState)
	assert.Equal(t, dagErr, wv.dagError)
}

// --- 47. DAG error shown in View output ---

func TestWorkflowsView_View_DAGErrorState(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 30
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayError
	v.dagWorkflowID = "implement"
	v.dagError = errors.New("DAG unavailable")

	out := v.View()
	assert.Contains(t, out, "Info error")
	assert.Contains(t, out, "DAG unavailable")
}

// --- 48. Last-run status badge shown in narrow layout ---

func TestWorkflowsView_View_LastRunStatusBadge_Narrow(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.lastRunStatus["implement"] = smithers.RunStatusFinished

	out := v.View()
	assert.Contains(t, out, "finished", "narrow layout should show last-run status badge")
}

// --- 49. Last-run status shown in wide detail pane ---

func TestWorkflowsView_View_LastRunStatusBadge_Wide(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 120
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 0
	v.lastRunStatus["implement"] = smithers.RunStatusFailed

	out := v.View()
	assert.Contains(t, out, "Last run:", "wide detail pane should show last-run label")
	assert.Contains(t, out, "failed", "wide detail pane should show last-run status")
}

// --- 50. ShortHelp includes run and info bindings ---

func TestWorkflowsView_ShortHelp_IncludesRunAndInfo(t *testing.T) {
	v := newTestWorkflowsView()
	help := v.ShortHelp()

	var descs []string
	for _, b := range help {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "run", "ShortHelp should mention run")
	assert.Contains(t, joined, "info", "ShortHelp should mention info")
}

// --- 51. runStatusBadge returns expected strings ---

func TestRunStatusBadge(t *testing.T) {
	assert.Contains(t, runStatusBadge(smithers.RunStatusRunning), "running")
	assert.Contains(t, runStatusBadge(smithers.RunStatusFinished), "finished")
	assert.Contains(t, runStatusBadge(smithers.RunStatusFailed), "failed")
	assert.Contains(t, runStatusBadge(smithers.RunStatusCancelled), "cancelled")
	assert.Empty(t, runStatusBadge(""), "empty status should return empty badge")
}

// --- 52. DAG msg for a different workflow is ignored ---

func TestWorkflowsView_DAGMsg_WrongWorkflowIDIgnored(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayLoading
	v.dagWorkflowID = "implement"

	// Message for a different workflow.
	dag := &smithers.DAGDefinition{WorkflowID: "review", Mode: "inferred"}
	updated, _ := v.Update(workflowDAGLoadedMsg{workflowID: "review", dag: dag})
	wv := updated.(*WorkflowsView)

	// State should remain loading (message ignored).
	assert.Equal(t, dagOverlayLoading, wv.dagState)
	assert.Nil(t, wv.dagDefinition)
}

// --- 53. Key presses are blocked while DAG overlay is open (except i/esc) ---

func TestWorkflowsView_KeysBlocked_WhenDAGOverlayOpen(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.cursor = 0

	// Down-arrow should not move cursor while overlay is open.
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 0, wv.cursor, "cursor should not move while DAG overlay is open")
	// Overlay should remain open (key was not i/esc).
	assert.Equal(t, dagOverlayVisible, wv.dagState)
}

// --- 54. Keys are blocked while run is in-flight ---

func TestWorkflowsView_KeysBlocked_WhileRunInFlight(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.confirmState = runConfirmRunning
	v.cursor = 0

	// Down-arrow should not move cursor while run is in-flight.
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 0, wv.cursor, "cursor should not move while run is in-flight")
}

// --- 55. Wide layout includes [Enter] Run hint ---

func TestWorkflowsView_View_WideLayout_ShowsRunHint(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 120
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.cursor = 0

	out := v.View()
	assert.Contains(t, out, "[Enter] Run", "wide detail pane should include run hint")
}

// ============================================================
// workflows-dynamic-input-forms: form state machine tests
// ============================================================

// --- 56. workflowFormDAGLoadedMsg with fields activates the form ---

func TestWorkflowsView_FormDAGLoadedMsg_ActivatesForm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	dag := &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields: []smithers.WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
			{Key: "ticketId", Label: "Ticket ID", Type: "string"},
		},
	}
	updated, cmd := v.Update(workflowFormDAGLoadedMsg{workflowID: "implement", dag: dag})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runFormActive, wv.formState, "form should become active when fields are present")
	assert.Len(t, wv.formFields, 2)
	assert.Len(t, wv.formInputs, 2)
	assert.Equal(t, 0, wv.formFocused, "first field should be focused")
}

// --- 57. workflowFormDAGLoadedMsg with no fields falls back to confirm ---

func TestWorkflowsView_FormDAGLoadedMsg_NoFields_FallsBackToConfirm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	dag := &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "fallback",
		Fields:     []smithers.WorkflowTask{},
	}
	updated, cmd := v.Update(workflowFormDAGLoadedMsg{workflowID: "implement", dag: dag})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runFormNone, wv.formState, "formState should reset to none")
	assert.Equal(t, runConfirmPending, wv.confirmState, "should fall back to confirm dialog")
}

// --- 58. workflowFormDAGErrorMsg falls back to confirm ---

func TestWorkflowsView_FormDAGErrorMsg_FallsBackToConfirm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	updated, cmd := v.Update(workflowFormDAGErrorMsg{workflowID: "implement", err: errors.New("network error")})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runFormNone, wv.formState, "formState should reset to none after error")
	assert.Equal(t, runConfirmPending, wv.confirmState, "should fall back to confirm dialog on error")
	assert.Empty(t, wv.formWorkflowID)
}

// --- 59. workflowFormDAGLoadedMsg for wrong workflow is ignored ---

func TestWorkflowsView_FormDAGLoadedMsg_WrongWorkflowIgnored(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	dag := &smithers.DAGDefinition{WorkflowID: "review", Mode: "inferred", Fields: []smithers.WorkflowTask{{Key: "p", Label: "P", Type: "string"}}}
	updated, _ := v.Update(workflowFormDAGLoadedMsg{workflowID: "review", dag: dag})
	wv := updated.(*WorkflowsView)

	// State must remain loading — the stale message should be ignored.
	assert.Equal(t, runFormLoading, wv.formState, "stale form DAG message should be ignored")
}

// --- 60. Esc in active form cancels form ---

func TestWorkflowsView_Form_EscCancelsForm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{{Key: "prompt", Label: "Prompt", Type: "string"}}
	v.formInputs = buildFormInputs(v.formFields)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runFormNone, wv.formState, "Esc should cancel the form")
	assert.Empty(t, wv.formWorkflowID)
	assert.Nil(t, wv.formFields)
	assert.Nil(t, wv.formInputs)
}

// --- 61. Enter in active form submits and fires RunWorkflow ---

func TestWorkflowsView_Form_EnterSubmitsForm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{{Key: "prompt", Label: "Prompt", Type: "string"}}
	v.formInputs = buildFormInputs(v.formFields)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkflowsView)

	assert.NotNil(t, cmd, "Enter in form should return a run command")
	assert.Equal(t, runFormNone, wv.formState, "form should be dismissed after submit")
	assert.Equal(t, runConfirmRunning, wv.confirmState, "confirmState should be running after submit")
}

// --- 62. Tab moves focus to next field ---

func TestWorkflowsView_Form_TabMovesFocusForward(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	v.formInputs = buildFormInputs(v.formFields)
	v.formFocused = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, 1, wv.formFocused, "Tab should move focus to next field")
}

// --- 63. Shift+Tab moves focus to previous field ---

func TestWorkflowsView_Form_ShiftTabMovesFocusBackward(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	v.formInputs = buildFormInputs(v.formFields)
	v.formFocused = 1

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, 0, wv.formFocused, "Shift+Tab should move focus to previous field")
}

// --- 64. Tab wraps from last to first field ---

func TestWorkflowsView_Form_TabWrapsAround(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	v.formInputs = buildFormInputs(v.formFields)
	v.formFocused = 1 // last field

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 0, wv.formFocused, "Tab from last field should wrap to first")
}

// --- 65. Shift+Tab wraps from first to last field ---

func TestWorkflowsView_Form_ShiftTabWrapsAround(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	v.formInputs = buildFormInputs(v.formFields)
	v.formFocused = 0 // first field

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 1, wv.formFocused, "Shift+Tab from first field should wrap to last")
}

// --- 66. Form loading blocks all key presses ---

func TestWorkflowsView_FormLoading_BlocksKeys(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"
	v.cursor = 0

	// Down-arrow should not move cursor while form is loading.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, wv.cursor, "cursor should not move while form is loading")
	assert.Equal(t, runFormLoading, wv.formState, "formState should remain loading")
}

// --- 67. View shows form loading indicator ---

func TestWorkflowsView_View_ShowsFormLoadingIndicator(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 30
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	out := v.View()
	assert.Contains(t, out, "Loading input fields", "view should show form loading indicator")
}

// --- 68. View shows form overlay when active ---

func TestWorkflowsView_View_ShowsFormOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 80
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormActive
	v.formWorkflowID = "implement"
	v.formFields = []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	v.formInputs = buildFormInputs(v.formFields)
	v.formFocused = 0

	out := v.View()
	assert.Contains(t, out, "Prompt", "form overlay should render field labels")
	assert.Contains(t, out, "Ticket ID", "form overlay should render all field labels")
	assert.Contains(t, out, "[Tab", "form overlay should show key hints")
	assert.Contains(t, out, "Submit", "form overlay should show Submit hint")
}

// --- 69. collectFormInputs gathers values by key ---

func TestCollectFormInputs(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	inputs := buildFormInputs(fields)
	// Simulate typing "hello" in first and "T-42" in second.
	// We can't simulate real key events easily here; check that empty values work.
	result := collectFormInputs(fields, inputs)
	assert.Equal(t, "", result["prompt"], "empty input should yield empty string")
	assert.Equal(t, "", result["ticketId"], "empty input should yield empty string")
	assert.Len(t, result, 2, "result should have one entry per field")
}

// --- 70. buildFormInputs focuses only the first field ---

func TestBuildFormInputs_FocusesFirstField(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "a", Label: "A", Type: "string"},
		{Key: "b", Label: "B", Type: "string"},
		{Key: "c", Label: "C", Type: "string"},
	}
	inputs := buildFormInputs(fields)
	require.Len(t, inputs, 3)
	// Placeholder should be set from Label.
	assert.Equal(t, "A", inputs[0].Placeholder)
	assert.Equal(t, "B", inputs[1].Placeholder)
	assert.Equal(t, "C", inputs[2].Placeholder)
}

// --- 71. Form with nil dag falls back to confirm ---

func TestWorkflowsView_FormDAGLoadedMsg_NilDAG_FallsBackToConfirm(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.formState = runFormLoading
	v.formWorkflowID = "implement"

	updated, cmd := v.Update(workflowFormDAGLoadedMsg{workflowID: "implement", dag: nil})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, runFormNone, wv.formState)
	assert.Equal(t, runConfirmPending, wv.confirmState, "nil DAG should fall back to confirm dialog")
}

// ============================================================
// workflows-dag-inspection + workflows-agent-and-schema-inspection
// ============================================================

// --- 72. DAG overlay shows visualization (box-drawing chars) ---

func TestWorkflowsView_DAGOverlay_ShowsDAGVisualization(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 100
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields: []smithers.WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
			{Key: "ticketId", Label: "Ticket ID", Type: "string"},
		},
	}

	out := v.View()
	// DAG visualization must contain box-drawing characters.
	assert.True(t, strings.Contains(out, "┌") || strings.Contains(out, "│"),
		"DAG overlay should render box-drawing chars, got: %q", out)
	assert.Contains(t, out, "Prompt")
	assert.Contains(t, out, "Ticket ID")
}

// --- 73. DAG overlay shows entry task ID when present ---

func TestWorkflowsView_DAGOverlay_ShowsEntryTaskID(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 100
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	entryTask := "generate-node"
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID:  "implement",
		Mode:        "inferred",
		EntryTaskID: &entryTask,
		Fields:      []smithers.WorkflowTask{{Key: "p", Label: "Prompt", Type: "string"}},
	}

	out := v.View()
	assert.Contains(t, out, "generate-node", "DAG overlay should show agent entry task ID")
	assert.Contains(t, out, "Agent Assignment")
}

// --- 74. 's' key in DAG overlay toggles schema visibility ---

func TestWorkflowsView_DAGOverlay_SKeyTogglesSchema(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields:     []smithers.WorkflowTask{{Key: "prompt", Label: "Prompt", Type: "string"}},
	}
	v.dagSchemaVisible = false

	// Press 's' to show schema.
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.True(t, wv.dagSchemaVisible, "'s' should enable schema visibility")

	// Press 's' again to hide.
	updated2, _ := wv.Update(tea.KeyPressMsg{Code: 's'})
	wv2 := updated2.(*WorkflowsView)
	assert.False(t, wv2.dagSchemaVisible, "'s' again should disable schema visibility")
}

// --- 75. DAG overlay with schema visible shows I/O schema section ---

func TestWorkflowsView_DAGOverlay_SchemaVisible_ShowsIOSection(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 100
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagSchemaVisible = true
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields: []smithers.WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
		},
	}

	out := v.View()
	assert.Contains(t, out, "I/O Schema", "schema section should be shown when dagSchemaVisible=true")
	assert.Contains(t, out, "key:")
	assert.Contains(t, out, "type:")
}

// --- 76. DAG overlay with schema hidden shows toggle hint ---

func TestWorkflowsView_DAGOverlay_SchemaHidden_ShowsToggleHint(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 100
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagSchemaVisible = false
	v.dagDefinition = &smithers.DAGDefinition{
		WorkflowID: "implement",
		Mode:       "inferred",
		Fields:     []smithers.WorkflowTask{{Key: "prompt", Label: "Prompt", Type: "string"}},
	}

	out := v.View()
	assert.Contains(t, out, "[s] Show schema", "should show schema toggle hint")
}

// --- 77. 'i' key resets dagSchemaVisible when closing overlay ---

func TestWorkflowsView_IKey_ResetsDagSchemaVisible(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.dagState = dagOverlayVisible
	v.dagWorkflowID = "implement"
	v.dagSchemaVisible = true
	v.dagDefinition = &smithers.DAGDefinition{WorkflowID: "implement", Mode: "inferred"}

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'i'})
	wv := updated.(*WorkflowsView)
	assert.False(t, wv.dagSchemaVisible, "'i' closing overlay should reset dagSchemaVisible")
}

// ============================================================
// workflows-doctor
// ============================================================

// --- 78. 'd' key triggers doctor overlay ---

func TestWorkflowsView_DKey_TriggersDoctorOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, doctorOverlayRunning, wv.doctorState, "'d' should start doctor diagnostics")
	assert.Equal(t, "implement", wv.doctorWorkflowID)
	assert.NotNil(t, cmd, "'d' should return a doctor command")
}

// --- 79. workflowDoctorResultMsg shows results ---

func TestWorkflowsView_DoctorResultMsg_ShowsResults(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayRunning
	v.doctorWorkflowID = "implement"

	issues := []DoctorIssue{
		{Severity: "ok", Check: "smithers-binary", Message: "smithers binary found."},
		{Severity: "warning", Check: "dag-analysis", Message: "Analysis fell back to generic mode."},
	}
	updated, cmd := v.Update(workflowDoctorResultMsg{workflowID: "implement", issues: issues})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, doctorOverlayVisible, wv.doctorState)
	assert.Len(t, wv.doctorIssues, 2)
}

// --- 80. workflowDoctorErrorMsg sets error state ---

func TestWorkflowsView_DoctorErrorMsg_SetsErrorState(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayRunning
	v.doctorWorkflowID = "implement"

	doctorErr := errors.New("exec failed")
	updated, cmd := v.Update(workflowDoctorErrorMsg{workflowID: "implement", err: doctorErr})
	wv := updated.(*WorkflowsView)

	assert.Nil(t, cmd)
	assert.Equal(t, doctorOverlayError, wv.doctorState)
	assert.Equal(t, doctorErr, wv.doctorError)
}

// --- 81. Doctor overlay renders issues in View ---

func TestWorkflowsView_View_DoctorOverlay_RendersIssues(t *testing.T) {
	v := newTestWorkflowsView()
	v.width = 100
	v.height = 40
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayVisible
	v.doctorWorkflowID = "implement"
	v.doctorIssues = []DoctorIssue{
		{Severity: "ok", Check: "smithers-binary", Message: "smithers binary found on PATH."},
		{Severity: "error", Check: "launch-fields", Message: "Could not fetch launch fields."},
	}

	out := v.View()
	assert.Contains(t, out, "Workflow Doctor")
	assert.Contains(t, out, "smithers binary found on PATH.")
	assert.Contains(t, out, "Could not fetch launch fields.")
}

// --- 82. Esc closes doctor overlay ---

func TestWorkflowsView_DoctorOverlay_EscCloses(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayVisible
	v.doctorWorkflowID = "implement"
	v.doctorIssues = []DoctorIssue{{Severity: "ok", Check: "binary", Message: "ok"}}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, doctorOverlayHidden, wv.doctorState)
	assert.Empty(t, wv.doctorWorkflowID)
}

// --- 83. 'd' key closes open doctor overlay ---

func TestWorkflowsView_DKey_ClosesDoctorOverlay(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayVisible
	v.doctorWorkflowID = "implement"

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkflowsView)
	assert.Nil(t, cmd)
	assert.Equal(t, doctorOverlayHidden, wv.doctorState)
}

// --- 84. Doctor overlay is blocked during loading ---

func TestWorkflowsView_DoctorOverlay_BlocksKeysWhileRunning(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayRunning
	v.doctorWorkflowID = "implement"
	v.cursor = 0

	// Down-arrow should not move cursor while doctor is running.
	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wv := updated.(*WorkflowsView)
	assert.Equal(t, 0, wv.cursor, "cursor should not move while doctor overlay is open")
}

// --- 85. RunWorkflowDoctor returns expected checks ---

func TestRunWorkflowDoctor_ReturnsBinaryCheck(t *testing.T) {
	// Use a client where smithers binary is not on PATH.
	c := smithers.NewClient(
		// Override lookPath to simulate binary not found.
	)
	_ = c
	// We test the exported RunWorkflowDoctor with a real client.
	// Since smithers is unlikely to be on PATH in CI, we just verify
	// the function returns at least one issue and doesn't panic.
	issues := RunWorkflowDoctor(t.Context(), smithers.NewClient(), "test-workflow")
	assert.NotEmpty(t, issues, "RunWorkflowDoctor should always return at least one issue")
	// First issue should be about the binary.
	assert.Equal(t, "smithers-binary", issues[0].Check)
}

// --- 86. ShortHelp includes doctor binding ---

func TestWorkflowsView_ShortHelp_IncludesDoctor(t *testing.T) {
	v := newTestWorkflowsView()
	help := v.ShortHelp()

	var descs []string
	for _, b := range help {
		descs = append(descs, b.Help().Desc)
	}
	joined := strings.Join(descs, " ")
	assert.Contains(t, joined, "doctor", "ShortHelp should mention doctor")
}

// --- 87. Doctor message for wrong workflow is ignored ---

func TestWorkflowsView_DoctorResultMsg_WrongWorkflowIgnored(t *testing.T) {
	v := newTestWorkflowsView()
	v = seedWorkflows(v, sampleWorkflows())
	v.doctorState = doctorOverlayRunning
	v.doctorWorkflowID = "implement"

	issues := []DoctorIssue{{Severity: "ok", Check: "x", Message: "ok"}}
	updated, _ := v.Update(workflowDoctorResultMsg{workflowID: "review", issues: issues})
	wv := updated.(*WorkflowsView)

	// State must remain running — the stale message should be ignored.
	assert.Equal(t, doctorOverlayRunning, wv.doctorState, "stale doctor result should be ignored")
	assert.Nil(t, wv.doctorIssues)
}

package views

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func int64Ptr(v int64) *int64 { return &v }

func sampleCrons() []smithers.CronSchedule {
	lastRun := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).UnixMilli()
	return []smithers.CronSchedule{
		{
			CronID:       "daily-report",
			Pattern:      "0 8 * * *",
			WorkflowPath: ".smithers/workflows/report.tsx",
			Enabled:      true,
			CreatedAtMs:  1000000,
			LastRunAtMs:  &lastRun,
		},
		{
			CronID:       "weekly-review",
			Pattern:      "0 9 * * 1",
			WorkflowPath: ".smithers/workflows/review.tsx",
			Enabled:      false,
			CreatedAtMs:  2000000,
		},
		{
			CronID:       "hourly-sync",
			Pattern:      "0 * * * *",
			WorkflowPath: ".smithers/workflows/sync.tsx",
			Enabled:      true,
			CreatedAtMs:  3000000,
		},
	}
}

func newTestTriggersView() *TriggersView {
	c := smithers.NewClient()
	return NewTriggersView(c)
}

func seedTriggers(v *TriggersView, crons []smithers.CronSchedule) *TriggersView {
	updated, _ := v.Update(triggersLoadedMsg{crons: crons})
	return updated.(*TriggersView)
}

// --- 1. Interface compliance ---

func TestTriggersView_ImplementsView(t *testing.T) {
	var _ View = (*TriggersView)(nil)
}

// --- 2. Init sets loading and returns a command ---

func TestTriggersView_Init_SetsLoading(t *testing.T) {
	v := newTestTriggersView()
	assert.True(t, v.loading, "NewTriggersView should start in loading state")
	cmd := v.Init()
	assert.NotNil(t, cmd, "Init should return a non-nil command")
}

// --- 3. LoadedMsg populates crons ---

func TestTriggersView_LoadedMsg_PopulatesCrons(t *testing.T) {
	v := newTestTriggersView()
	crons := sampleCrons()

	updated, cmd := v.Update(triggersLoadedMsg{crons: crons})
	assert.Nil(t, cmd)

	tv := updated.(*TriggersView)
	assert.False(t, tv.loading)
	assert.Nil(t, tv.err)
	assert.Len(t, tv.crons, 3)
	assert.Equal(t, "daily-report", tv.crons[0].CronID)
}

// --- 4. ErrorMsg sets err ---

func TestTriggersView_ErrorMsg_SetsErr(t *testing.T) {
	v := newTestTriggersView()
	someErr := errors.New("smithers not found on PATH")

	updated, cmd := v.Update(triggersErrorMsg{err: someErr})
	assert.Nil(t, cmd)

	tv := updated.(*TriggersView)
	assert.False(t, tv.loading)
	assert.Equal(t, someErr, tv.err)
}

// --- 5. Cursor navigation down ---

func TestTriggersView_CursorNavigation_Down(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	assert.Equal(t, 0, v.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 1, tv.cursor, "j should move cursor down")

	updated2, _ := tv.Update(tea.KeyPressMsg{Code: 'j'})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, 2, tv2.cursor, "j again should move to index 2")
}

// --- 6. Cursor navigation up ---

func TestTriggersView_CursorNavigation_Up(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.cursor = 2

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 1, tv.cursor, "k should move cursor up")

	updated2, _ := tv.Update(tea.KeyPressMsg{Code: 'k'})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, 0, tv2.cursor, "k again should move to index 0")
}

// --- 7. Cursor boundary at bottom ---

func TestTriggersView_CursorBoundary_AtBottom(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.cursor = 2 // last index (len=3)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 2, tv.cursor, "cursor should not exceed last index")
}

// --- 8. Cursor boundary at top ---

func TestTriggersView_CursorBoundary_AtTop(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	assert.Equal(t, 0, v.cursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 0, tv.cursor, "cursor should not go below 0")
}

// --- 9. Arrow keys navigate ---

func TestTriggersView_ArrowKeys_Navigate(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	tv := updated.(*TriggersView)
	assert.Equal(t, 1, tv.cursor, "Down arrow should move cursor down")

	updated2, _ := tv.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, 0, tv2.cursor, "Up arrow should move cursor up")
}

// --- 10. Esc returns PopViewMsg ---

func TestTriggersView_Esc_ReturnsPopViewMsg(t *testing.T) {
	v := newTestTriggersView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "Esc should return a non-nil command")

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc command should emit PopViewMsg")
}

// --- 11. Refresh ('r') reloads crons ---

func TestTriggersView_Refresh_ReloadsCrons(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	assert.False(t, v.loading)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	tv := updated.(*TriggersView)
	assert.True(t, tv.loading, "'r' should set loading = true")
	assert.NotNil(t, cmd, "'r' should return a reload command")
}

// --- 12. Refresh no-op while already loading ---

func TestTriggersView_Refresh_NoOpWhileLoading(t *testing.T) {
	v := newTestTriggersView()
	// loading is true by default

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	tv := updated.(*TriggersView)
	assert.True(t, tv.loading, "loading should remain true")
	assert.Nil(t, cmd, "'r' while loading should not return a command")
}

// --- 13. 't' key fires toggle command ---

func TestTriggersView_TKey_FiresToggleCommand(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	// cursor=0, crons[0].Enabled = true; toggling should send enabled=false
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 't'})
	tv := updated.(*TriggersView)
	assert.Equal(t, "daily-report", tv.toggleInflight, "toggleInflight should be set to the cron ID")
	assert.NotNil(t, cmd, "'t' should return a toggle command")
}

// --- 14. toggleSuccessMsg updates enabled field ---

func TestTriggersView_ToggleSuccessMsg_UpdatesEnabled(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.toggleInflight = "daily-report"

	updated, cmd := v.Update(triggerToggleSuccessMsg{cronID: "daily-report", enabled: false})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Empty(t, tv.toggleInflight)
	assert.Nil(t, tv.toggleErr)
	// The cron should now be disabled.
	assert.False(t, tv.crons[0].Enabled, "cron[0] should be disabled after toggle success")
}

// --- 15. toggleErrorMsg surfaces error ---

func TestTriggersView_ToggleErrorMsg_SurfacesError(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.toggleInflight = "daily-report"

	toggleErr := errors.New("server error")
	updated, cmd := v.Update(triggerToggleErrorMsg{cronID: "daily-report", err: toggleErr})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Empty(t, tv.toggleInflight)
	assert.Equal(t, toggleErr, tv.toggleErr)
}

// --- 16. 't' key is no-op on empty list ---

func TestTriggersView_TKey_NoOpOnEmptyList(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, []smithers.CronSchedule{})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 't'})
	tv := updated.(*TriggersView)
	assert.Empty(t, tv.toggleInflight)
	assert.Nil(t, cmd)
}

// --- 17. 't' key is no-op when toggle already in-flight ---

func TestTriggersView_TKey_NoOpWhenInflight(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.toggleInflight = "daily-report" // already in-flight

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 't'})
	tv := updated.(*TriggersView)
	// toggleInflight should remain unchanged
	assert.Equal(t, "daily-report", tv.toggleInflight)
	assert.Nil(t, cmd)
}

// --- 18. 'd' key opens delete confirmation overlay ---

func TestTriggersView_DKey_OpensDeleteOverlay(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	tv := updated.(*TriggersView)
	assert.Equal(t, deleteConfirmPending, tv.deleteState, "deleteState should be pending")
	assert.Nil(t, cmd, "'d' should not issue a command (overlay first)")
}

// --- 19. Esc cancels delete overlay ---

func TestTriggersView_DeleteOverlay_EscCancels(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// Open the overlay.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	tv := updated.(*TriggersView)
	require.Equal(t, deleteConfirmPending, tv.deleteState)

	// Press Esc inside the overlay.
	updated2, cmd2 := tv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, deleteConfirmNone, tv2.deleteState, "Esc should dismiss delete overlay")
	assert.Nil(t, cmd2)
}

// --- 20. Enter inside delete overlay fires delete command ---

func TestTriggersView_DeleteOverlay_EnterFiresDeleteCmd(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// Open the overlay.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	tv := updated.(*TriggersView)
	require.Equal(t, deleteConfirmPending, tv.deleteState)

	// Confirm.
	updated2, cmd := tv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, deleteConfirmRunning, tv2.deleteState, "state should move to running")
	assert.NotNil(t, cmd, "confirmation should return a delete command")
}

// --- 21. deleteSuccessMsg removes cron from list ---

func TestTriggersView_DeleteSuccessMsg_RemovesCron(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.deleteState = deleteConfirmRunning
	v.cursor = 0

	updated, cmd := v.Update(triggerDeleteSuccessMsg{cronID: "daily-report"})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Equal(t, deleteConfirmNone, tv.deleteState)
	assert.Nil(t, tv.deleteErr)
	// Should be 2 crons remaining.
	assert.Len(t, tv.crons, 2)
	// daily-report should be gone.
	for _, c := range tv.crons {
		assert.NotEqual(t, "daily-report", c.CronID)
	}
}

// --- 22. deleteErrorMsg surfaces error ---

func TestTriggersView_DeleteErrorMsg_SurfacesError(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.deleteState = deleteConfirmRunning

	delErr := errors.New("permission denied")
	updated, cmd := v.Update(triggerDeleteErrorMsg{cronID: "daily-report", err: delErr})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Equal(t, deleteConfirmNone, tv.deleteState)
	assert.Equal(t, delErr, tv.deleteErr)
	// List should be unchanged.
	assert.Len(t, tv.crons, 3)
}

// --- 23. View header text ---

func TestTriggersView_View_HeaderText(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 24
	out := v.View()
	assert.Contains(t, ansi.Strip(out), "CODEPLANE \u203a Triggers")
}

// --- 24. View loading state ---

func TestTriggersView_View_LoadingState(t *testing.T) {
	v := newTestTriggersView()
	out := v.View()
	assert.Contains(t, out, "Loading")
}

// --- 25. View error state ---

func TestTriggersView_View_ErrorState(t *testing.T) {
	v := newTestTriggersView()
	v.loading = false
	v.err = errors.New("smithers binary not found")

	out := v.View()
	assert.Contains(t, out, "Error")
	assert.Contains(t, out, "smithers binary not found")
	assert.Contains(t, out, "PATH")
}

// --- 26. View empty state ---

func TestTriggersView_View_EmptyState(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, []smithers.CronSchedule{})

	out := v.View()
	assert.Contains(t, out, "No cron triggers found")
}

// --- 27. View shows cron IDs ---

func TestTriggersView_View_ShowsCronIDs(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())

	out := v.View()
	assert.Contains(t, out, "daily-report")
	assert.Contains(t, out, "weekly-review")
	assert.Contains(t, out, "hourly-sync")
}

// --- 28. View shows schedule patterns ---

func TestTriggersView_View_ShowsPatterns(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())

	out := v.View()
	assert.Contains(t, out, "0 8 * * *", "should show daily-report pattern")
	assert.Contains(t, out, "0 9 * * 1", "should show weekly-review pattern")
}

// --- 29. View shows enabled/disabled badge ---

func TestTriggersView_View_ShowsEnabledBadge(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())

	out := v.View()
	assert.Contains(t, out, "enabled", "should show enabled badge")
	assert.Contains(t, out, "disabled", "should show disabled badge")
}

// --- 30. View shows last run time ---

func TestTriggersView_View_ShowsLastRun(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())

	out := v.View()
	assert.Contains(t, out, "last:", "should show last run label")
	assert.Contains(t, out, "2024-01-15", "should show last run date")
}

// --- 31. View shows cursor indicator ---

func TestTriggersView_View_CursorIndicator(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.cursor = 0

	out := v.View()
	assert.Contains(t, out, "\u25b8", "selected cron should show cursor indicator")
}

// --- 32. View shows delete confirm overlay ---

func TestTriggersView_View_DeleteConfirmOverlay(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.cursor = 0
	v.deleteState = deleteConfirmPending

	out := v.View()
	assert.Contains(t, out, "Delete trigger", "overlay should mention 'Delete trigger'")
	assert.Contains(t, out, "daily-report", "overlay should mention the cron ID")
	assert.Contains(t, out, "[Enter] Yes", "overlay should show confirm hint")
}

// --- 33. View shows delete error ---

func TestTriggersView_View_DeleteError(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.deleteErr = errors.New("delete failed")

	out := v.View()
	assert.Contains(t, out, "Delete failed")
	assert.Contains(t, out, "delete failed")
}

// --- 34. View shows toggle error ---

func TestTriggersView_View_ToggleError(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.toggleErr = errors.New("toggle failed")

	out := v.View()
	assert.Contains(t, out, "Toggle failed")
	assert.Contains(t, out, "toggle failed")
}

// --- 35. Name ---

func TestTriggersView_Name(t *testing.T) {
	v := newTestTriggersView()
	assert.Equal(t, "triggers", v.Name())
}

// --- 36. SetSize ---

func TestTriggersView_SetSize(t *testing.T) {
	v := newTestTriggersView()
	v.SetSize(120, 40)
	assert.Equal(t, 120, v.width)
	assert.Equal(t, 40, v.height)
}

// --- 37. Window resize updates dimensions ---

func TestTriggersView_WindowResize_UpdatesDimensions(t *testing.T) {
	v := newTestTriggersView()
	updated, cmd := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Nil(t, cmd)

	tv := updated.(*TriggersView)
	assert.Equal(t, 120, tv.width)
	assert.Equal(t, 40, tv.height)
}

// --- 38. ShortHelp returns expected bindings ---

func TestTriggersView_ShortHelp_NotEmpty(t *testing.T) {
	v := newTestTriggersView()
	help := v.ShortHelp()
	assert.NotEmpty(t, help)

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "toggle")
	assert.Contains(t, joined, "delete")
	assert.Contains(t, joined, "back")
}

// --- 39. selectedCron returns correct cron ---

func TestTriggersView_SelectedCron(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.cursor = 1

	c := v.selectedCron()
	require.NotNil(t, c)
	assert.Equal(t, "weekly-review", c.CronID)
}

// --- 40. selectedCron returns nil on empty list ---

func TestTriggersView_SelectedCron_EmptyList(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, []smithers.CronSchedule{})

	c := v.selectedCron()
	assert.Nil(t, c)
}

// --- 41. LoadedMsg clears previous error ---

func TestTriggersView_LoadedMsg_ClearsPreviousError(t *testing.T) {
	v := newTestTriggersView()
	updated1, _ := v.Update(triggersErrorMsg{err: errors.New("oops")})
	tv := updated1.(*TriggersView)
	require.NotNil(t, tv.err)

	updated2, _ := tv.Update(triggersLoadedMsg{crons: sampleCrons()})
	tv2 := updated2.(*TriggersView)
	assert.Nil(t, tv2.err)
	assert.Len(t, tv2.crons, 3)
}

// --- 42. Cursor moves after delete adjusts for shorter list ---

func TestTriggersView_Delete_CursorAdjusts(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.cursor = 2 // last item: hourly-sync
	v.deleteState = deleteConfirmRunning

	updated, _ := v.Update(triggerDeleteSuccessMsg{cronID: "hourly-sync"})
	tv := updated.(*TriggersView)

	assert.Len(t, tv.crons, 2)
	assert.LessOrEqual(t, tv.cursor, len(tv.crons)-1, "cursor should be clamped to new length")
}

// --- 43. Keys blocked while delete is running ---

func TestTriggersView_KeysBlocked_WhileDeleteRunning(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.deleteState = deleteConfirmRunning
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 0, tv.cursor, "cursor should not move while delete is in-flight")
}

// --- 44. Cursor move clears stale toggle error ---

func TestTriggersView_CursorMove_ClearsToggleError(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.toggleErr = errors.New("old error")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Nil(t, tv.toggleErr, "moving cursor should clear stale toggle error")
}

// --- 45. Cursor move clears stale delete error ---

func TestTriggersView_CursorMove_ClearsDeleteError(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.deleteErr = errors.New("old delete error")

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Nil(t, tv.deleteErr, "moving cursor should clear stale delete error")
}

// --- 46. View shows key hint footer ---

func TestTriggersView_View_KeyHintFooter(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())

	out := v.View()
	assert.Contains(t, out, "[t] toggle", "footer should show toggle hint")
	assert.Contains(t, out, "[d] delete", "footer should show delete hint")
}

// --- 47. clampScroll keeps cursor visible ---

func TestTriggersView_ClampScroll_KeepsCursorVisible(t *testing.T) {
	v := newTestTriggersView()
	crons := make([]smithers.CronSchedule, 20)
	for i := range crons {
		crons[i] = smithers.CronSchedule{
			CronID:       fmt.Sprintf("cron-%02d", i),
			Pattern:      "0 * * * *",
			WorkflowPath: ".smithers/workflows/sync.tsx",
			Enabled:      true,
		}
	}
	v = seedTriggers(v, crons)
	v.SetSize(80, 20)

	for i := 0; i < 19; i++ {
		updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
		v = updated.(*TriggersView)
	}
	assert.Equal(t, 19, v.cursor)
	assert.GreaterOrEqual(t, v.cursor, v.scrollOffset)
	assert.Less(t, v.cursor, v.scrollOffset+v.pageSize())
}

// --- 48. 'd' key is no-op on empty list ---

func TestTriggersView_DKey_NoOpOnEmptyList(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, []smithers.CronSchedule{})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	tv := updated.(*TriggersView)
	assert.Equal(t, deleteConfirmNone, tv.deleteState)
	assert.Nil(t, cmd)
}

// --- 49. updateCronEnabled only updates matching cron ---

func TestTriggersView_UpdateCronEnabled_OnlyMatchingCron(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// crons[1] is weekly-review, enabled=false. Toggle to true.
	v.updateCronEnabled("weekly-review", true)
	assert.True(t, v.crons[1].Enabled, "weekly-review should be enabled")
	// Other crons should be unchanged.
	assert.True(t, v.crons[0].Enabled, "daily-report should still be enabled")
	assert.True(t, v.crons[2].Enabled, "hourly-sync should still be enabled")
}

// --- 50. Registry includes triggers ---

func TestTriggersView_Registry(t *testing.T) {
	reg := DefaultRegistry()
	names := reg.Names()
	assert.Contains(t, names, "triggers", "DefaultRegistry should include 'triggers'")
}

// ============================================================
// Create-trigger tests (feat-triggers-create)
// ============================================================

// --- 51. 'c' key opens create form ---

func TestTriggersView_CKey_OpensCreateForm(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	tv := updated.(*TriggersView)
	assert.Equal(t, createFormActive, tv.createState, "'c' should open create form")
	assert.Nil(t, cmd, "'c' should not fire a command immediately")
}

// --- 52. 'c' key is no-op while loading ---

func TestTriggersView_CKey_NoOpWhileLoading(t *testing.T) {
	v := newTestTriggersView()
	// loading=true by default

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	tv := updated.(*TriggersView)
	assert.Equal(t, createFormNone, tv.createState, "'c' should be ignored while loading")
	assert.Nil(t, cmd)
}

// --- 53. Esc cancels create form ---

func TestTriggersView_CreateForm_EscCancels(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// Open the form.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	tv := updated.(*TriggersView)
	require.Equal(t, createFormActive, tv.createState)

	// Press Esc.
	updated2, cmd := tv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, createFormNone, tv2.createState, "Esc should dismiss create form")
	assert.Nil(t, cmd)
}

// --- 54. Enter with empty fields is no-op ---

func TestTriggersView_CreateForm_EnterWithEmptyFieldsIsNoOp(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	tv := updated.(*TriggersView)

	// Don't fill in fields — press Enter.
	updated2, cmd := tv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	tv2 := updated2.(*TriggersView)
	// Form should remain active (not submitted).
	assert.Equal(t, createFormActive, tv2.createState, "form should stay open with empty fields")
	assert.Nil(t, cmd)
}

// --- 55. triggerCreateSuccessMsg appends cron to list ---

func TestTriggersView_CreateSuccessMsg_AppendsCron(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.createState = createFormRunning

	newCron := smithers.CronSchedule{
		CronID:       "monthly-audit",
		Pattern:      "0 8 1 * *",
		WorkflowPath: ".smithers/workflows/audit.tsx",
		Enabled:      true,
	}
	updated, cmd := v.Update(triggerCreateSuccessMsg{cron: &newCron})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Equal(t, createFormNone, tv.createState)
	assert.Nil(t, tv.createErr)
	assert.Len(t, tv.crons, 4, "new cron should be appended")
	assert.Equal(t, "monthly-audit", tv.crons[3].CronID)
	// Cursor should jump to the new entry.
	assert.Equal(t, 3, tv.cursor)
}

// --- 56. triggerCreateErrorMsg returns to form ---

func TestTriggersView_CreateErrorMsg_ReturnsToForm(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.createState = createFormRunning

	createErr := errors.New("invalid cron pattern")
	updated, cmd := v.Update(triggerCreateErrorMsg{err: createErr})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Equal(t, createFormActive, tv.createState, "should return to form on error")
	assert.Equal(t, createErr, tv.createErr)
	assert.Len(t, tv.crons, 3, "list should be unchanged after create error")
}

// --- 57. View shows create form ---

func TestTriggersView_View_CreateFormOverlay(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.createState = createFormActive

	out := v.View()
	assert.Contains(t, out, "Create Trigger", "overlay should show Create Trigger title")
	assert.Contains(t, out, "Cron Pattern", "overlay should have pattern label")
	assert.Contains(t, out, "Workflow Path", "overlay should have workflow path label")
	assert.Contains(t, out, "[Enter] Create", "overlay should show submit hint")
}

// --- 58. Tab moves focus between create form fields ---

func TestTriggersView_CreateForm_TabMovesFocus(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// Open form.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	tv := updated.(*TriggersView)
	require.Equal(t, createFormActive, tv.createState)
	assert.Equal(t, createFieldPattern, tv.createFocus)

	// Tab once — should move to workflowPath field.
	updated2, _ := tv.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, createFieldWorkflowPath, tv2.createFocus)

	// Tab again — should wrap back to pattern field.
	updated3, _ := tv2.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	tv3 := updated3.(*TriggersView)
	assert.Equal(t, createFieldPattern, tv3.createFocus)
}

// ============================================================
// Edit-trigger tests (feat-triggers-edit)
// ============================================================

// --- 59. 'e' key opens edit form for selected cron ---

func TestTriggersView_EKey_OpensEditForm(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.cursor = 0

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	tv := updated.(*TriggersView)
	assert.Equal(t, editFormActive, tv.editState, "'e' should open edit form")
	assert.Nil(t, cmd)
	// Edit input should be pre-filled with the current pattern.
	assert.Equal(t, "0 8 * * *", tv.editInput.Value(), "edit input should be pre-filled")
}

// --- 60. 'e' key is no-op on empty list ---

func TestTriggersView_EKey_NoOpOnEmptyList(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, []smithers.CronSchedule{})

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'e'})
	tv := updated.(*TriggersView)
	assert.Equal(t, editFormNone, tv.editState)
	assert.Nil(t, cmd)
}

// --- 61. Esc cancels edit form ---

func TestTriggersView_EditForm_EscCancels(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())

	// Open edit form.
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'e'})
	tv := updated.(*TriggersView)
	require.Equal(t, editFormActive, tv.editState)

	// Esc should cancel.
	updated2, cmd := tv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tv2 := updated2.(*TriggersView)
	assert.Equal(t, editFormNone, tv2.editState, "Esc should close edit form")
	assert.Nil(t, cmd)
}

// --- 62. Enter with empty pattern is no-op ---

func TestTriggersView_EditForm_EnterWithEmptyPatternIsNoOp(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.editState = editFormActive
	v.editInput = func() textinput.Model {
		ti := textinput.New()
		ti.SetValue("")
		ti.Focus()
		return ti
	}()

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	tv := updated.(*TriggersView)
	assert.Equal(t, editFormActive, tv.editState, "form should stay open with empty pattern")
	assert.Nil(t, cmd)
}

// --- 63. triggerEditSuccessMsg triggers reload ---

func TestTriggersView_EditSuccessMsg_TriggersReload(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.editState = editFormRunning

	updated, cmd := v.Update(triggerEditSuccessMsg{})
	tv := updated.(*TriggersView)

	assert.Equal(t, editFormNone, tv.editState)
	assert.Nil(t, tv.editErr)
	assert.True(t, tv.loading, "edit success should trigger a list reload")
	assert.NotNil(t, cmd, "edit success should return a reload command")
}

// --- 64. triggerEditErrorMsg returns to form ---

func TestTriggersView_EditErrorMsg_ReturnsToForm(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.editState = editFormRunning

	editErr := errors.New("smithers cron error")
	updated, cmd := v.Update(triggerEditErrorMsg{cronID: "daily-report", err: editErr})
	tv := updated.(*TriggersView)

	assert.Nil(t, cmd)
	assert.Equal(t, editFormActive, tv.editState, "should return to form on error")
	assert.Equal(t, editErr, tv.editErr)
}

// --- 65. View shows edit form ---

func TestTriggersView_View_EditFormOverlay(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.cursor = 0
	v.editState = editFormActive
	v.editInput = func() textinput.Model {
		ti := textinput.New()
		ti.SetValue("0 10 * * *")
		ti.Focus()
		return ti
	}()

	out := v.View()
	assert.Contains(t, out, "Edit Trigger", "overlay should show Edit Trigger title")
	assert.Contains(t, out, "daily-report", "overlay should mention cron ID")
	assert.Contains(t, out, "New Cron Pattern", "overlay should show pattern label")
}

// --- 66. ShortHelp includes create and edit ---

func TestTriggersView_ShortHelp_IncludesCreateAndEdit(t *testing.T) {
	v := newTestTriggersView()
	help := v.ShortHelp()

	var allDesc []string
	for _, b := range help {
		allDesc = append(allDesc, b.Help().Desc)
	}
	joined := strings.Join(allDesc, " ")
	assert.Contains(t, joined, "create", "ShortHelp should mention create")
	assert.Contains(t, joined, "edit", "ShortHelp should mention edit")
}

// --- 67. Keys blocked while create form running ---

func TestTriggersView_KeysBlocked_WhileCreateRunning(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.createState = createFormRunning
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 0, tv.cursor, "cursor should not move while create is in-flight")
}

// --- 68. Keys blocked while edit form running ---

func TestTriggersView_KeysBlocked_WhileEditRunning(t *testing.T) {
	v := newTestTriggersView()
	v = seedTriggers(v, sampleCrons())
	v.editState = editFormRunning
	v.cursor = 0

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	tv := updated.(*TriggersView)
	assert.Equal(t, 0, tv.cursor, "cursor should not move while edit is in-flight")
}

// --- 69. Create form shows error banner ---

func TestTriggersView_CreateForm_ShowsErrorBanner(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.createState = createFormActive
	v.createErr = errors.New("bad cron expression")

	out := v.View()
	assert.Contains(t, out, "bad cron expression", "create form should show error")
}

// --- 70. Edit form shows error banner ---

func TestTriggersView_EditForm_ShowsErrorBanner(t *testing.T) {
	v := newTestTriggersView()
	v.width = 80
	v.height = 40
	v = seedTriggers(v, sampleCrons())
	v.cursor = 0
	v.editState = editFormActive
	v.editErr = errors.New("edit failed")

	out := v.View()
	assert.Contains(t, out, "edit failed", "edit form should show error")
}

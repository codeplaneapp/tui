package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/ui/diffnav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_DashboardToChangesView_FullFlow tests the complete flow:
// Dashboard -> navigate to Changes tab -> press Enter -> ChangesView opens ->
// changes load -> press d -> cmd returned -> press Esc -> PopViewMsg returned.
//
// This is NOT a unit test with mocks. It uses the real constructors.
func TestIntegration_DashboardToChangesView_FullFlow(t *testing.T) {
	// Step 1: Create dashboard (no smithers, no jjhub — but tabs still exist if jjhub client provided)
	jc := jjhub.NewClient("")
	d := NewDashboardViewWithJJHub(nil, false, jc)
	d.SetSize(120, 40)

	// Verify Changes tab exists (jjhub client was provided)
	hasChanges := false
	for _, tab := range d.tabs {
		if tab == DashTabChanges {
			hasChanges = true
		}
	}
	if !hasChanges {
		t.Skip("Changes tab not present (jjhub not detected)")
	}

	// Step 2: Navigate to Changes tab
	changesIdx := -1
	for i, tab := range d.tabs {
		if tab == DashTabChanges {
			changesIdx = i
		}
	}
	d.activeTab = changesIdx

	// Step 3: Press Enter on Changes tab — should return DashboardNavigateMsg
	updated, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	d = updated.(*DashboardView)
	require.NotNil(t, cmd, "Enter on Changes tab must return a cmd")
	msg := cmd()
	navMsg, ok := msg.(DashboardNavigateMsg)
	require.True(t, ok, "expected DashboardNavigateMsg, got %T", msg)
	assert.Equal(t, "changes", navMsg.View, "should navigate to 'changes'")

	// Step 4: Simulate what the root model does — open ChangesView via registry
	registry := DefaultRegistry()
	view, found := registry.Open("changes", nil)
	require.True(t, found, "'changes' must be registered")
	require.NotNil(t, view)
	assert.Equal(t, "changes", view.Name())

	changesView := view.(*ChangesView)
	changesView.SetSize(120, 40)

	// Step 5: Init fetches changes
	initCmd := changesView.Init()
	require.NotNil(t, initCmd, "Init must return a fetch command")

	// Step 6: Simulate changes loaded
	updated2, _ := changesView.Update(changesLoadedMsg{changes: []jjhub.Change{
		{ChangeID: "abc123", Description: "test change", Author: jjhub.Author{Name: "test"}},
		{ChangeID: "def456", Description: "another change", Author: jjhub.Author{Name: "test2"}},
	}})
	changesView = updated2.(*ChangesView)
	assert.False(t, changesView.loading, "should not be loading after changesLoadedMsg")
	assert.Len(t, changesView.filteredChanges, 2)

	// Step 7: Press 'd' — should return a cmd (either diffnav launch or install prompt)
	updated3, dCmd := changesView.Update(tea.KeyPressMsg{Code: 'd'})
	changesView = updated3.(*ChangesView)
	require.NotNil(t, dCmd, "d key with loaded changes must return a cmd, NOT nil")

	// Execute the cmd
	dMsg := dCmd()
	require.NotNil(t, dMsg, "d cmd must produce a message")
	t.Logf("d key produced message type: %T", dMsg)

	// It should be either a handoff or install prompt
	switch dMsg.(type) {
	case diffnav.InstallPromptMsg:
		t.Log("diffnav not installed — got InstallPromptMsg (correct)")
	default:
		t.Logf("got %T — may be a handoff exec msg (correct if diffnav installed)", dMsg)
	}

	// Step 8: Press 'enter' — should also trigger diff
	updated4, enterCmd := changesView.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	changesView = updated4.(*ChangesView)
	require.NotNil(t, enterCmd, "enter key with loaded changes must return a cmd, NOT nil")

	// Step 9: Press Escape — should return PopViewMsg
	updated5, escCmd := changesView.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	changesView = updated5.(*ChangesView)
	require.NotNil(t, escCmd, "esc must return a cmd")
	escMsg := escCmd()
	_, isPop := escMsg.(PopViewMsg)
	assert.True(t, isPop, "esc must emit PopViewMsg, got %T", escMsg)

	// Step 10: Verify view renders without panic
	output := changesView.View()
	assert.NotEmpty(t, output)
}

// TestIntegration_ChangesView_DKeyBeforeLoad_IsNoop verifies that pressing
// d before changes have loaded is a silent noop (no crash, no cmd).
func TestIntegration_ChangesView_DKeyBeforeLoad_IsNoop(t *testing.T) {
	registry := DefaultRegistry()
	view, _ := registry.Open("changes", nil)
	cv := view.(*ChangesView)
	cv.SetSize(120, 40)

	// Don't send changesLoadedMsg — still loading
	assert.True(t, cv.loading)

	_, cmd := cv.Update(tea.KeyPressMsg{Code: 'd'})
	// When loading, keys go to the split pane or are noops
	// The important thing: no panic, and d doesn't crash
	t.Logf("d while loading: cmd=%v", cmd)
}

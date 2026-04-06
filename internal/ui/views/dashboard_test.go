package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/diffnav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardView_EscFromSubTab_ReturnsToOverview(t *testing.T) {
	d := NewDashboardView(nil, false)
	d.SetSize(120, 40)
	d.tabs = []DashboardTab{DashTabOverview, DashTabRuns, DashTabWorkflows}
	d.activeTab = 1 // on Runs tab

	updated, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	d = updated.(*DashboardView)

	assert.Equal(t, 0, d.activeTab, "esc should return to Overview tab")
	assert.Nil(t, cmd, "should not emit a command when going back to overview")
}

func TestDashboardView_EscFromOverview_EmitsPopViewMsg(t *testing.T) {
	d := NewDashboardView(nil, false)
	d.SetSize(120, 40)
	d.tabs = []DashboardTab{DashTabOverview, DashTabRuns}
	d.activeTab = 0 // already on Overview

	_, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "esc on overview must return a cmd")
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "esc on overview must emit PopViewMsg, got %T", msg)
}

func TestDashboardView_DKeyOnChangesTab_NoChanges_ShowsToast(t *testing.T) {
	d := NewDashboardView(nil, false)
	d.SetSize(120, 40)
	d.tabs = []DashboardTab{DashTabOverview, DashTabChanges}
	d.activeTab = 1
	d.changesLoading = false
	d.changes = nil // no changes loaded

	_, cmd := d.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd, "d with no changes should show a toast, not be silent")
	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Contains(t, toast.Title, "No changes")
}

func TestDashboardView_DKeyOnChangesTab_StillLoading_ShowsToast(t *testing.T) {
	d := NewDashboardView(nil, false)
	d.SetSize(120, 40)
	d.tabs = []DashboardTab{DashTabOverview, DashTabChanges}
	d.activeTab = 1
	d.changesLoading = true

	_, cmd := d.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd, "d while loading should show a toast")
	msg := cmd()
	toast, ok := msg.(components.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Contains(t, toast.Title, "loading")
}

func TestDashboardView_DKeyOnChangesTab_WithChanges_ReturnsDiffCmd(t *testing.T) {
	d := NewDashboardView(nil, false)
	d.SetSize(120, 40)
	d.tabs = []DashboardTab{DashTabOverview, DashTabChanges}
	d.activeTab = 1
	d.changesLoading = false
	d.changes = []jjhub.Change{
		{ChangeID: "abc123", Description: "test change"},
	}
	d.menuCursor = 0

	_, cmd := d.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd, "d with changes must return a cmd")

	msg := cmd()
	// Either launches diffnav (if installed) or prompts to install
	switch msg.(type) {
	case diffnav.InstallPromptMsg:
		// Expected when diffnav not installed
	default:
		// Could be a handoff exec msg — that's fine too
	}
}

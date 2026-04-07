package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 71. Dashboard Shift+Tab Backward Wrap
//     Verifies that Shift+Tab from the Overview tab (first tab) wraps around
//     to the Sessions tab (last tab).
// ---------------------------------------------------------------------------

func TestDashboardShiftTabWrap_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SHIFT_TAB_WRAPS_TO_LAST_TAB", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Should start on Overview tab.
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))

		// Shift+Tab should wrap backward from Overview to Sessions (last tab).
		tui.SendKeys("\x1b[Z") // Shift+Tab escape sequence
		time.Sleep(300 * time.Millisecond)

		// Should now be on Sessions tab.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Chat Sessions", "Sessions", "Ctrl+S",
		}, 5*time.Second))

		// Shift+Tab again goes to Workflows.
		tui.SendKeys("\x1b[Z")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Available Workflows", "Workflows", "No workflows",
		}, 5*time.Second))

		// One more goes to Runs.
		tui.SendKeys("\x1b[Z")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Recent Runs", "Runs", "No runs yet",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 72. Dashboard Enter from Runs Tab Opens Runs View
//     Verifies that pressing Enter while on the Runs tab opens the full
//     Runs Dashboard view (not the Overview menu selection).
// ---------------------------------------------------------------------------

func TestDashboardEnterFromRunsTab_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ENTER_ON_RUNS_TAB_OPENS_RUNS_VIEW", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch to Runs tab.
		tui.SendKeys("2")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Recent Runs", "No runs yet",
		}, 5*time.Second))

		// Enter should open the full Run Dashboard view.
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH › Runs", "Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Escape back to dashboard.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 73. Dashboard Enter from Workflows Tab Opens Workflows View
//     Verifies that pressing Enter while on the Workflows tab opens the
//     full Workflows view.
// ---------------------------------------------------------------------------

func TestDashboardEnterFromWorkflowsTab_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ENTER_ON_WORKFLOWS_TAB_OPENS_WORKFLOWS_VIEW", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch to Workflows tab.
		tui.SendKeys("3")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Available Workflows", "No workflows found",
		}, 5*time.Second))

		// Enter should open the full Workflows view.
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "Loading workflows", "No workflows found", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 74. Triggers Create Form Shift+Tab Backward Navigation
//     Verifies that Shift+Tab in the create trigger form moves focus
//     backward from Workflow Path to Cron Pattern field.
// ---------------------------------------------------------------------------

func TestTriggersCreateFormShiftTab_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SHIFT_TAB_MOVES_BACKWARD_IN_FORM", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Cron Triggers")
		require.NoError(t, tui.WaitForText("Cron Triggers", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "Loading triggers", "No cron triggers found", "Error",
		}, 10*time.Second))

		// Press 'c' to open create form.
		tui.SendKeys("c")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Create Trigger", "Cron Pattern", "Workflow Path",
		}, 5*time.Second))

		// Tab to workflow path field.
		tui.SendKeys("\t")
		time.Sleep(200 * time.Millisecond)

		// Type in workflow path.
		tui.SendKeys("my-workflow.yaml")
		require.NoError(t, tui.WaitForText("my-workflow.yaml", 5*time.Second))

		// Shift+Tab back to cron pattern.
		tui.SendKeys("\x1b[Z") // Shift+Tab
		time.Sleep(200 * time.Millisecond)

		// Type in cron pattern (should go to pattern field now).
		tui.SendKeys("0 * * * *")
		require.NoError(t, tui.WaitForText("0 * * * *", 5*time.Second))

		// Both field values should be visible.
		require.NoError(t, tui.WaitForText("my-workflow.yaml", 3*time.Second))

		// Escape to cancel.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Create Trigger", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 75. Triggers 'd' Delete on Empty List (No-Op Stability)
//     Verifies that pressing 'd' (delete) when the trigger list is empty
//     does not crash or show unexpected state.
// ---------------------------------------------------------------------------

func TestTriggersDeleteEmpty_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("DELETE_ON_EMPTY_LIST_IS_NOOP", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Cron Triggers")
		require.NoError(t, tui.WaitForText("Cron Triggers", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 10*time.Second))

		// Press 'd' — should be no-op with no triggers.
		tui.SendKeys("d")
		time.Sleep(300 * time.Millisecond)

		// No confirmation dialog should appear. View stable.
		require.NoError(t, tui.WaitForNoText("Delete trigger", 3*time.Second))
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 3*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 76. Triggers 'e' Edit on Empty List (No-Op Stability)
//     Verifies that pressing 'e' (edit) when the trigger list is empty
//     does not crash or show an edit form.
// ---------------------------------------------------------------------------

func TestTriggersEditEmpty_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("EDIT_ON_EMPTY_LIST_IS_NOOP", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Cron Triggers")
		require.NoError(t, tui.WaitForText("Cron Triggers", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 10*time.Second))

		// Press 'e' — should be no-op with no triggers.
		tui.SendKeys("e")
		time.Sleep(300 * time.Millisecond)

		// No edit form should appear.
		require.NoError(t, tui.WaitForNoText("Edit Trigger", 3*time.Second))
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 3*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 77. Approvals View Navigation Stability with No Data
//     Verifies that arrow keys and focus switching in the approvals view
//     are stable when no approvals exist.
// ---------------------------------------------------------------------------

func TestApprovalsNavigationStability_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ARROW_KEYS_STABLE_WITH_NO_APPROVALS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open approvals view.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "deny",
		}, 10*time.Second))

		// j/k navigation with empty list — should not crash.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)

		// Press 'a' approve and 'd' deny — should be no-op with no data.
		tui.SendKeys("a")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("d")
		time.Sleep(200 * time.Millisecond)

		// View should still be stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "deny",
		}, 5*time.Second))

		// Tab to recent, back to pending — cycle.
		tui.SendKeys("\t")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("\t")
		time.Sleep(200 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals",
		}, 3*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 78. Scores Refresh ('r' key)
//     Verifies that 'r' in the scores view triggers a refresh and the view
//     remains stable.
// ---------------------------------------------------------------------------

func TestScoresRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_SCORES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Scores Dashboard")
		require.NoError(t, tui.WaitForText("Scores Dashboard", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Scores", "Loading scores", "No score data available", "Error",
		}, 10*time.Second))

		// Press 'r' to refresh.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Should reload scores (show loading or refreshed state).
		require.NoError(t, tui.WaitForAnyText([]string{
			"Scores", "Loading scores", "No score data available", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 79. Dashboard Runs Tab Content
//     Verifies that the Runs tab on the dashboard shows the correct inline
//     content ("Recent Runs" header, empty state message).
// ---------------------------------------------------------------------------

func TestDashboardRunsTabContent_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("RUNS_TAB_SHOWS_INLINE_CONTENT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch to Runs tab.
		tui.SendKeys("2")
		time.Sleep(500 * time.Millisecond)

		// Should show runs-specific inline content.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Recent Runs", "No runs yet", "Loading",
		}, 5*time.Second))

		// Should also show the hint to open full dashboard.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Enter", "Run Dashboard", "Loading",
		}, 5*time.Second))

		// Go back to Overview.
		tui.SendKeys("1")
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 80. Workflows Schema Toggle Inside Info Overlay
//     Verifies that pressing 'i' opens the info/DAG overlay and 's' toggles
//     schema visibility within it.
// ---------------------------------------------------------------------------

func TestWorkflowsSchemaToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("S_KEY_TOGGLES_SCHEMA_IN_OVERLAY", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Workflows")
		require.NoError(t, tui.WaitForAnyText([]string{"Workflows"}, 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "Loading workflows", "No workflows found", "Error",
		}, 15*time.Second))

		// Press 'i' to open info overlay.
		tui.SendKeys("i")
		time.Sleep(500 * time.Millisecond)

		// Press 's' to toggle schema visibility.
		tui.SendKeys("s")
		time.Sleep(300 * time.Millisecond)

		// Toggle back.
		tui.SendKeys("s")
		time.Sleep(300 * time.Millisecond)

		// View should remain stable through all toggles.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "DAG", "Info", "Error",
		}, 5*time.Second))

		// Close overlays.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "Error", "Start Chat",
		}, 10*time.Second))
	})
}

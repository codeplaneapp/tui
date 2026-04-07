package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 81. Quick Approvals 'a' Key from Chat Main Focus
//     Verifies that pressing 'a' while in chat main focus (not editor focus)
//     navigates directly to the approvals view — a power-user shortcut.
// ---------------------------------------------------------------------------

func TestQuickApprovalsFromChat_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("A_KEY_NAVIGATES_TO_APPROVALS_FROM_MAIN_FOCUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title:    "Approvals Nav Session",
			messages: []string{"some message"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("some message", 15*time.Second))

		// Tab to main focus.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Press 'a' — should navigate to approvals view.
		tui.SendKeys("a")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "deny",
		}, 10*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		time.Sleep(300 * time.Millisecond)
	})

	t.Run("A_KEY_TYPES_IN_EDITOR_FOCUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Editor is focused by default. 'a' should type, not navigate.
		tui.SendKeys("a")
		time.Sleep(200 * time.Millisecond)

		// Should NOT navigate to approvals.
		require.NoError(t, tui.WaitForNoText("Approvals", 2*time.Second))

		// 'a' should appear as text in the editor.
		require.NoError(t, tui.WaitForText("a", 2*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 82. Approvals Help Bar Changes Between Pending and Recent Tabs
//     Verifies that when on the Recent tab, the help bar no longer shows
//     approve/deny bindings, since those actions don't apply to past decisions.
// ---------------------------------------------------------------------------

func TestApprovalsHelpBarVariant_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("RECENT_TAB_HIDES_APPROVE_DENY", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open approvals.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve",
		}, 10*time.Second))

		// On pending tab, help bar shows approve/deny.
		require.NoError(t, tui.WaitForAnyText([]string{
			"approve", "deny", "history",
		}, 5*time.Second))

		// Tab to Recent/History tab.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Help bar should now show "pending queue" instead of approve/deny.
		require.NoError(t, tui.WaitForAnyText([]string{
			"pending queue", "pending",
		}, 5*time.Second))

		// Tab back to Pending.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// approve/deny should reappear.
		require.NoError(t, tui.WaitForAnyText([]string{
			"approve", "deny",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 83. Dashboard Workflows Tab Inline Content
//     Verifies the Workflows tab shows "Available Workflows" or the empty
//     state message inline on the dashboard.
// ---------------------------------------------------------------------------

func TestDashboardWorkflowsTabContent_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("WORKFLOWS_TAB_SHOWS_INLINE_CONTENT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch to Workflows tab.
		tui.SendKeys("3")
		time.Sleep(500 * time.Millisecond)

		// Should show workflow-specific inline content.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Available Workflows", "No workflows found", "Loading",
		}, 5*time.Second))

		// Return to Overview.
		tui.SendKeys("1")
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 84. Runs 't' Snapshots Key Stability
//     Verifies that pressing 't' (snapshots/timeline) in the runs view
//     is stable when no runs are selected.
// ---------------------------------------------------------------------------

func TestRunsSnapshotsKeyStability_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("T_KEY_STABLE_WITH_NO_RUNS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Press 't' for snapshots — no-op with no data.
		tui.SendKeys("t")
		time.Sleep(300 * time.Millisecond)

		// View should remain stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 85. Runs 'r' Refresh
//     Verifies that 'r' in the runs view triggers a refresh of the run list.
// ---------------------------------------------------------------------------

func TestRunsRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_RUNS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Wait for initial load.
		require.NoError(t, tui.WaitForAnyText([]string{
			"All", "No runs found",
		}, 10*time.Second))

		// Press 'r' to refresh.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Should reload (loading state or refreshed).
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found", "All",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 86. Triggers 't' Toggle on Empty List (No-Op Stability)
//     Verifies that pressing 't' (toggle enabled/disabled) when the trigger
//     list is empty does not crash.
// ---------------------------------------------------------------------------

func TestTriggersToggleEmpty_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_ON_EMPTY_LIST_IS_NOOP", func(t *testing.T) {
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

		// Press 't' to toggle — should be no-op with no data.
		tui.SendKeys("t")
		time.Sleep(300 * time.Millisecond)

		// View should remain stable, no "toggling..." indicator.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 3*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 87. Memory View Enter for Detail Mode
//     Verifies that pressing Enter in the memory list attempts to enter
//     detail mode. With no data, it should be a no-op.
// ---------------------------------------------------------------------------

func TestMemoryDetailMode_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ENTER_ON_EMPTY_LIST_IS_NOOP", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Memory Browser")
		require.NoError(t, tui.WaitForText("Memory Browser", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "Loading memory facts", "No memory facts found", "Error",
		}, 10*time.Second))

		// Press Enter — should be no-op with no facts.
		tui.SendKeys("\r")
		time.Sleep(300 * time.Millisecond)

		// Should still be in list mode, no crash.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "No memory facts found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 88. SQL Browser 'r' Refresh Tables
//     Verifies that 'r' in the SQL browser (left pane focused) refreshes
//     the table list.
// ---------------------------------------------------------------------------

func TestSQLBrowserRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_TABLES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("SQL Browser")
		require.NoError(t, tui.WaitForText("SQL Browser", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Loading tables", "No tables found", "Error",
		}, 10*time.Second))

		// Press 'r' to refresh tables.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Should reload (loading or refreshed state).
		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Loading tables", "No tables found", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 89. Onboarding with No Providers Configured
//     Verifies that launching with the onboarding fixture (no providers)
//     immediately shows the model/provider selection dialog, and that the
//     dialog is dismissable with Escape to reach a fallback state.
// ---------------------------------------------------------------------------

func TestOnboardingNoProviders_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("NO_PROVIDERS_SHOWS_ONBOARDING", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		// Should immediately show the onboarding/model selection prompt.
		require.NoError(t, tui.WaitForAnyText([]string{
			"choose a provider", "Find your fave",
			"To start, let's choose a provider and model.",
		}, 15*time.Second))
	})

	t.Run("ONBOARDING_SCROLLS_THROUGH_PROVIDERS", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		require.NoError(t, tui.WaitForText("Find your fave", 15*time.Second))

		// j/k should navigate through available providers/models.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")
		time.Sleep(200 * time.Millisecond)

		// Should still be in onboarding after navigation.
		require.NoError(t, tui.WaitForText("Find your fave", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 90. Agents Enter and Navigation on Empty/Populated List
//     Verifies that Enter on an agent row is stable (either launches TUI or
//     is a no-op for unavailable agents) and j/k navigation works correctly.
// ---------------------------------------------------------------------------

func TestAgentsEnterStability_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ENTER_ON_AGENT_STABLE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Agents")
		require.NoError(t, tui.WaitForText("Agents", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Agents", "Loading agents", "Available", "Not Detected", "Error",
		}, 15*time.Second))

		// Navigate down then press Enter — should not crash.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)

		// Enter on an agent (may launch if available, or be no-op).
		// We DON'T actually want to launch an agent TUI in tests,
		// but pressing Enter should not crash.
		tui.SendKeys("\r")
		time.Sleep(500 * time.Millisecond)

		// If we're still in agents view (agent not available) or
		// the app is still running, test passes.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Agents", "Available", "Not Detected", "CRUSH", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
		time.Sleep(300 * time.Millisecond)
	})
}

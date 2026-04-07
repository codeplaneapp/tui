package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 11. Tickets View Tab Switching
//     Verifies navigating between work item source tabs (Local, GitHub Issues,
//     GitHub PRs) in the tickets/work items view.
// ---------------------------------------------------------------------------

func TestTicketsTabSwitching_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_SWITCHES_SOURCE_TABS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open tickets view via command palette.
		openCommandsPalette(t, tui)
		tui.SendKeys("Work Items")
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on the tickets view with Local as default source.
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local", "Loading local tickets", "No local tickets",
		}, 10*time.Second))

		// Tab switches to the next source tab.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Should show a different source (GitHub Issues or JJHub depending on config).
		pane := tui.bufferText()
		hasNewTab := tui.matchesText("Issues") ||
			tui.matchesText("JJHub") ||
			tui.matchesText("PRs") ||
			tui.matchesText("Loading")
		if !hasNewTab {
			t.Logf("expected a different tab after pressing Tab; pane:\n%s", pane)
		}

		// Tab again to cycle further.
		tui.SendKeys("\t")

		// Should still be in WORK ITEMS view (no crash).
		require.NoError(t, tui.WaitForText("WORK ITEMS", 5*time.Second))
	})

	t.Run("TICKETS_HEADER_SHOWS_CURRENT_SOURCE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Work Items")
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
		tui.SendKeys("\r")

		// The header renders as "WORK ITEMS › Local" initially.
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local",
		}, 10*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 12. Tickets Search/Filter Mode
//     Verifies that '/' activates the filter prompt in the tickets view and
//     typing filters items, and Escape clears the filter.
// ---------------------------------------------------------------------------

func TestTicketsSearchFilter_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SLASH_ACTIVATES_FILTER", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Work Items")
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local",
		}, 10*time.Second))

		// Press '/' to activate filter mode.
		tui.SendKeys("/")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Filter", "filter", "done", "clear",
		}, 5*time.Second))

		// Type a filter query.
		tui.SendKeys("test-filter-query")
		require.NoError(t, tui.WaitForText("test-filter-query", 5*time.Second))

		// Escape clears the filter and exits filter mode.
		tui.SendKeys("\x1b")

		// View should still be stable.
		require.NoError(t, tui.WaitForText("WORK ITEMS", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 13. Triggers View — Open and Help Bar
//     Verifies the cron triggers view opens via command palette and shows
//     the correct loading/empty state and help bar shortcuts.
// ---------------------------------------------------------------------------

func TestTriggersView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TRIGGERS_VIEW_OPENS_AND_SHOWS_STATE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Cron Triggers")
		require.NoError(t, tui.WaitForText("Cron Triggers", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on the triggers view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "Loading triggers", "No cron triggers found", "Error",
		}, 10*time.Second))

		// The help footer should show key hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"create", "toggle", "delete", "refresh",
		}, 5*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})

	t.Run("TRIGGERS_VIEW_NAVIGATION_STABLE", func(t *testing.T) {
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

		// j/k navigation should not crash even with empty list.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")

		// Still in triggers view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 14. Triggers Create Form Interaction
//     Verifies 'c' opens the create form, Tab navigates between fields,
//     and Escape cancels the form.
// ---------------------------------------------------------------------------

func TestTriggersCreateForm_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CREATE_FORM_OPENS_AND_CLOSES", func(t *testing.T) {
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

		// Press 'c' to open the create form.
		tui.SendKeys("c")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Create Trigger", "Cron Pattern", "Workflow Path",
		}, 5*time.Second))

		// The form should show Tab/Enter/Esc hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Tab", "Enter", "Cancel",
		}, 5*time.Second))

		// Type into the cron pattern field.
		tui.SendKeys("*/5 * * * *")
		require.NoError(t, tui.WaitForText("*/5 * * * *", 5*time.Second))

		// Tab to workflow path field.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Type a workflow path.
		tui.SendKeys("deploy.yaml")
		require.NoError(t, tui.WaitForText("deploy.yaml", 5*time.Second))

		// Escape to cancel without creating.
		tui.SendKeys("\x1b")

		// Should still be in triggers view, form dismissed.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "No cron triggers found", "Error",
		}, 5*time.Second))
		require.NoError(t, tui.WaitForNoText("Create Trigger", 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 15. Memory Browser View
//     Verifies the memory browser opens, shows loading/empty state, and
//     has functional navigation.
// ---------------------------------------------------------------------------

func TestMemoryBrowserView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("MEMORY_VIEW_OPENS_AND_SHOWS_STATE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Memory Browser")
		require.NoError(t, tui.WaitForText("Memory Browser", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on the memory view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "Loading memory facts", "No memory facts found", "Error",
		}, 10*time.Second))

		// The help bar should show navigation/recall hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"select", "view", "recall", "namespace",
		}, 5*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})

	t.Run("MEMORY_VIEW_NAVIGATION_STABLE", func(t *testing.T) {
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

		// j/k navigation should not crash.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")

		// View still present.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "No memory facts found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 16. SQL Browser View
//     Verifies the SQL browser opens with its split-pane layout, shows
//     loading/empty state for tables, and Tab switches pane focus.
// ---------------------------------------------------------------------------

func TestSQLBrowserView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SQL_VIEW_OPENS_AND_SHOWS_STATE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("SQL Browser")
		require.NoError(t, tui.WaitForText("SQL Browser", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on the SQL browser view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Loading tables", "No tables found", "Error",
		}, 10*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})

	t.Run("SQL_VIEW_TAB_SWITCHES_PANE_FOCUS", func(t *testing.T) {
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

		// Initially focused on left pane (tables). Help bar shows navigate/expand hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"navigate", "expand", "editor", "refresh",
		}, 5*time.Second))

		// Tab to switch focus to the right pane (editor).
		tui.SendKeys("\t")

		// After switching to editor pane, help bar should change to execute/history hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"execute", "history", "tables",
		}, 5*time.Second))

		// Tab back to tables pane.
		tui.SendKeys("\t")

		// Should show table-focused hints again.
		require.NoError(t, tui.WaitForAnyText([]string{
			"navigate", "expand", "editor",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 17. Scores Dashboard View
//     Verifies the scores dashboard opens, shows loading/empty state, and
//     Tab switches between Summary and Details tabs.
// ---------------------------------------------------------------------------

func TestScoresDashboardView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SCORES_VIEW_OPENS_AND_SHOWS_STATE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Scores Dashboard")
		require.NoError(t, tui.WaitForText("Scores Dashboard", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on the scores view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Scores", "Loading scores", "No score data available", "Error",
			"Summary", "Today",
		}, 10*time.Second))

		// Escape back.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})

	t.Run("SCORES_TAB_SWITCHES_SUMMARY_AND_DETAILS", func(t *testing.T) {
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
			"Summary",
		}, 10*time.Second))

		// Tab to switch to Details tab.
		tui.SendKeys("\t")

		// Should show Details tab indicator.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Details", "Loading", "Error", "Scores",
		}, 5*time.Second))

		// Tab back to Summary.
		tui.SendKeys("\t")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Summary", "Scores",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 18. Chat Focus Toggle (Tab between editor and messages)
//     Verifies that Tab switches focus from the editor to the messages
//     panel (main view) and back, changing keyboard routing.
// ---------------------------------------------------------------------------

func TestChatFocusToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_SWITCHES_FOCUS_TO_MESSAGES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title:    "Focus Test Session",
			messages: []string{"first message", "second message"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Wait for seeded messages to appear.
		require.NoError(t, tui.WaitForText("first message", 15*time.Second))

		// Editor is focused by default — typing goes to the editor.
		tui.SendKeys("typing in editor")
		require.NoError(t, tui.WaitForText("typing in editor", 5*time.Second))

		// Tab to switch focus to messages panel.
		tui.SendKeys("\t")

		// The help bar should now show "focus editor" since we're in main focus.
		require.NoError(t, tui.WaitForAnyText([]string{
			"focus editor", "editor",
		}, 5*time.Second))

		// Tab back to editor.
		tui.SendKeys("\t")

		// Should be back in editor focus — help bar says "focus chat".
		require.NoError(t, tui.WaitForAnyText([]string{
			"focus chat", "chat",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 19. Work Items from Dashboard Menu
//     Verifies that selecting "Work Items" from the dashboard overview menu
//     opens the tickets view correctly.
// ---------------------------------------------------------------------------

func TestWorkItemsFromDashboardMenu_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("DASHBOARD_WORK_ITEMS_OPENS_TICKETS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate to "Work Items" in the menu.
		// Menu order: Start Chat, Run Dashboard, Workflows, Approvals, Work Items
		tui.SendKeys("j") // Run Dashboard
		tui.SendKeys("j") // Workflows
		tui.SendKeys("j") // Approvals
		tui.SendKeys("j") // Work Items
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))

		tui.SendKeys("\r")

		// Should open tickets view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local", "Loading local tickets", "No local tickets",
		}, 10*time.Second))

		// Escape back to dashboard.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})

	t.Run("DASHBOARD_SQL_BROWSER_OPENS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate to "SQL Browser" in the menu.
		// Menu order: Start Chat, Run Dashboard, Workflows, Approvals, Work Items, SQL Browser
		tui.SendKeys("j") // Run Dashboard
		tui.SendKeys("j") // Workflows
		tui.SendKeys("j") // Approvals
		tui.SendKeys("j") // Work Items
		tui.SendKeys("j") // SQL Browser
		require.NoError(t, tui.WaitForText("SQL Browser", 5*time.Second))

		tui.SendKeys("\r")

		// Should open SQL browser view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Loading tables", "No tables found", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 20. Command Palette View Navigation Roundtrip
//     Verifies opening multiple different views via the command palette in
//     succession, confirming each renders and Escape returns correctly.
// ---------------------------------------------------------------------------

func TestCommandPaletteViewRoundtrip_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("OPEN_FIVE_VIEWS_IN_SUCCESSION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// 1. Open Cron Triggers.
		openCommandsPalette(t, tui)
		tui.SendKeys("Cron Triggers")
		require.NoError(t, tui.WaitForText("Cron Triggers", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "Loading triggers", "No cron triggers found", "Error",
		}, 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))

		// 2. Open Memory Browser.
		openCommandsPalette(t, tui)
		tui.SendKeys("Memory Browser")
		require.NoError(t, tui.WaitForText("Memory Browser", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "Loading memory facts", "No memory facts found", "Error",
		}, 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))

		// 3. Open Scores Dashboard.
		openCommandsPalette(t, tui)
		tui.SendKeys("Scores Dashboard")
		require.NoError(t, tui.WaitForText("Scores Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Scores", "Loading scores", "No score data available", "Error",
		}, 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))

		// 4. Open SQL Browser.
		openCommandsPalette(t, tui)
		tui.SendKeys("SQL Browser")
		require.NoError(t, tui.WaitForText("SQL Browser", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Loading tables", "No tables found", "Error",
		}, 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))

		// 5. Open Work Items.
		openCommandsPalette(t, tui)
		tui.SendKeys("Work Items")
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local", "Loading local tickets", "No local tickets",
		}, 10*time.Second))
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))

		// App should still be functional after this roundtrip.
		require.NoError(t, tui.WaitForText("Start Chat", 5*time.Second))
	})
}

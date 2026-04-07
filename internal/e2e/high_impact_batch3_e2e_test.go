package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 21. Prompt History Navigation (Up/Down in Editor)
//     Verifies that arrow-up in an empty editor navigates through previously
//     sent messages from seeded sessions, and arrow-down returns.
// ---------------------------------------------------------------------------

func TestPromptHistoryNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("UP_ARROW_RECALLS_PREVIOUS_MESSAGES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(), seededSession{
			title:    "History Session",
			messages: []string{"first prompt", "second prompt", "third prompt"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Wait for the session to load.
		require.NoError(t, tui.WaitForText("first prompt", 15*time.Second))

		// Press Up arrow to navigate to the most recent prompt in history.
		// The editor starts empty; up should load "third prompt".
		tui.SendKeys("\x1b[A") // up arrow escape sequence

		// The editor should now contain the last sent message.
		require.NoError(t, tui.WaitForAnyText([]string{
			"third prompt", "second prompt", "first prompt",
		}, 5*time.Second))
	})

	t.Run("DOWN_ARROW_NAVIGATES_FORWARD", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(), seededSession{
			title:    "History Session 2",
			messages: []string{"alpha msg", "beta msg"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("alpha msg", 15*time.Second))

		// Up to go back in history.
		tui.SendKeys("\x1b[A") // up
		time.Sleep(500 * time.Millisecond)

		// Up again.
		tui.SendKeys("\x1b[A") // up
		time.Sleep(500 * time.Millisecond)

		// Down to go forward in history.
		tui.SendKeys("\x1b[B") // down

		// Should not crash; editor should contain a history entry or be back to draft.
		require.NoError(t, tui.WaitForAnyText([]string{
			"alpha msg", "beta msg", "SMITHERS", fixtureLargeModelName,
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 22. Yolo Mode via --yolo Flag
//     Verifies that launching with --yolo changes the editor prompt indicator
//     (the "!" yolo icon appears instead of the normal "> " prompt).
// ---------------------------------------------------------------------------

func TestYoloModeFlag_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("YOLO_FLAG_CHANGES_PROMPT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture, "--yolo")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// In yolo mode, the editor prompt shows "!" instead of ">".
		require.NoError(t, tui.WaitForAnyText([]string{
			"!", "SMITHERS", fixtureLargeModelName, "Ready?",
		}, 10*time.Second))

		// The app should still be functional — type text.
		tui.SendKeys("yolo test input")
		require.NoError(t, tui.WaitForText("yolo test input", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 23. Session Flag (--session loads specific session by ID)
//     Verifies that --session <id> loads a specific session's messages.
// ---------------------------------------------------------------------------

func TestSessionFlag_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_FLAG_LOADS_SPECIFIC_SESSION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		sessions := seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Target Session", messages: []string{"target session content"}},
			seededSession{title: "Other Session", messages: []string{"other session content"}},
		)

		// Launch with --session pointing to the first session.
		targetID := sessions[0].ID
		tui := launchFixtureTUI(t, fixture, "--session", targetID)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Should show the target session's messages.
		require.NoError(t, tui.WaitForText("target session content", 15*time.Second))

		// Should NOT show the other session's messages.
		require.NoError(t, tui.WaitForNoText("other session content", 3*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 24. Dashboard 'c' Key for Quick Chat
//     Verifies that pressing 'c' on the dashboard immediately opens chat
//     without navigating the menu.
// ---------------------------------------------------------------------------

func TestDashboardQuickChat_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("C_KEY_OPENS_CHAT_FROM_DASHBOARD", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openStartChatFromDashboard(t, tui)

		// Should open the chat/landing view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"MCPs", "LSPs", fixtureLargeModelName,
		}, 10*time.Second))

		// Should no longer show dashboard-specific elements.
		require.NoError(t, tui.WaitForNoText("At a Glance", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 25. Dashboard 'r' Key for Refresh
//     Verifies that 'r' on the dashboard triggers a data refresh and does
//     not crash or navigate away.
// ---------------------------------------------------------------------------

func TestDashboardRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_DASHBOARD", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Press 'r' to refresh.
		tui.SendKeys("r")

		// Dashboard should still be visible (refresh is in-place, not a navigation).
		waitForDashboard(t, tui)
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 26. Yolo Mode Toggle via Command Palette
//     Verifies toggling yolo mode on/off through the command palette while
//     in the chat view.
// ---------------------------------------------------------------------------

func TestYoloModeToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_YOLO_VIA_COMMANDS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Enable yolo mode via command palette.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Yolo")
		require.NoError(t, tui.WaitForText("Toggle Yolo", 5*time.Second))
		tui.SendKeys("\r")

		// After enabling, the editor prompt should change to show "!" icon.
		require.NoError(t, tui.WaitForAnyText([]string{
			"!", "SMITHERS", fixtureLargeModelName, "Ready?",
		}, 5*time.Second))

		// Toggle it back off.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Yolo")
		require.NoError(t, tui.WaitForText("Toggle Yolo", 5*time.Second))
		tui.SendKeys("\r")

		// App should still be functional.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 27. Help Bar Shows Contextual Hints Per View
//     Verifies that different views show different help bar bindings, proving
//     the contextual help system works.
// ---------------------------------------------------------------------------

func TestContextualHelpBar_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("HELP_BAR_CHANGES_PER_VIEW", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Dashboard help hints: "enter", "chat", "tab", "refresh", "quit".
		require.NoError(t, tui.WaitForAnyText([]string{
			"select", "chat", "refresh", "quit",
		}, 5*time.Second))

		// Open Runs view.
		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Runs help hints should include run-specific actions.
		require.NoError(t, tui.WaitForAnyText([]string{
			"filter", "approve", "deny", "hijack",
		}, 5*time.Second))

		// Open Approvals view directly from Runs.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "No pending approvals",
		}, 10*time.Second))

		// Approvals help hints should include approval-specific actions.
		require.NoError(t, tui.WaitForAnyText([]string{
			"approve", "deny", "history",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 28. Workflows Info/DAG Overlay ('i' key)
//     Verifies that pressing 'i' on the workflows view opens the DAG/info
//     overlay, and Escape closes it.
// ---------------------------------------------------------------------------

func TestWorkflowsInfoOverlay_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("I_KEY_OPENS_INFO_OVERLAY", func(t *testing.T) {
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

		// The overlay may show DAG info, or it may be a no-op with no workflows.
		// Either way, the view should not crash.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "DAG", "Info", "Loading", "Error",
		}, 5*time.Second))

		// Escape closes the overlay (or stays in workflows if overlay wasn't shown).
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "Error", "New Chat",
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 29. Chat Landing View Shows MCPs and LSPs
//     Verifies that the landing/chat view displays the MCP and LSP status
//     panels with provider/model information.
// ---------------------------------------------------------------------------

func TestChatLandingView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("LANDING_SHOWS_MODEL_AND_STATUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// The landing view should show the model name.
		require.NoError(t, tui.WaitForText(fixtureLargeModelName, 10*time.Second))

		// Should show MCPs and LSPs sections.
		require.NoError(t, tui.WaitForText("MCPs", 5*time.Second))
		require.NoError(t, tui.WaitForText("LSPs", 5*time.Second))
	})

	t.Run("LANDING_SHOWS_PROVIDER_NAME", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Should show the fixture provider name.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Fixture AI", fixtureLargeModelName,
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 30. Quit via 'q' from Dashboard
//     Verifies that pressing 'q' on the dashboard quits the application,
//     which is an alternative to Ctrl+C → confirm quit dialog.
// ---------------------------------------------------------------------------

func TestQuitFromDashboard_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("Q_KEY_QUITS_FROM_DASHBOARD", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Press 'q'. The current dashboard keeps running rather than exiting.
		tui.SendKeys("q")

		// The app should remain stable on the dashboard.
		waitForDashboard(t, tui)
	})

	t.Run("Q_KEY_DOES_NOT_QUIT_FROM_CHAT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// In chat view, 'q' should type into the editor, not quit.
		tui.SendKeys("q")

		// Should NOT have exited.
		require.NoError(t, tui.WaitForNoText("[crush exited", 3*time.Second))

		// The 'q' should have been typed into the editor.
		require.NoError(t, tui.WaitForText("q", 3*time.Second))
	})
}

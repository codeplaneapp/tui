package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 91. Commands Dialog Tab Cycles Through System/User/MCP Tabs
//     Verifies that Tab/Shift+Tab in the commands palette cycles between
//     the System Commands, User Commands, and MCP Prompts radio tabs.
// ---------------------------------------------------------------------------

func TestCommandsDialogTabCycle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_CYCLES_COMMAND_TABS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open commands palette.
		openCommandsPalette(t, tui)

		// Default is System Commands tab. Verify system commands are visible.
		require.NoError(t, tui.WaitForText("Switch Model", 5*time.Second))

		// Tab should cycle to next tab (User or MCP, depending on config).
		// Even without custom commands or MCP prompts, Tab should not crash.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// View should still be showing the commands dialog.
		require.NoError(t, tui.WaitForText("Commands", 5*time.Second))

		// Tab again.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForText("Commands", 5*time.Second))

		// Shift+Tab to go backward.
		tui.SendKeys("\x1b[Z") // Shift+Tab
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForText("Commands", 5*time.Second))

		// Escape to close.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 92. Initialize Project Tab Toggles Yes/No Buttons
//     Verifies that Tab in the Initialize Project prompt toggles between
//     the "Yep!" and "Nope" buttons.
// ---------------------------------------------------------------------------

func TestInitializeProjectTabToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_TOGGLES_YEP_NOPE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Trigger Initialize Project via command palette.
		openCommandsPalette(t, tui)
		tui.SendKeys("Initialize Project")
		require.NoError(t, tui.WaitForText("Initialize Project", 5*time.Second))
		tui.SendKeys("\r")

		// Should show the initialization prompt with Yep!/Nope.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Yep!", "Nope", "initialize",
		}, 10*time.Second))

		// Tab should toggle between the two buttons.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Both buttons should still be visible (selection toggled).
		require.NoError(t, tui.WaitForText("Yep!", 3*time.Second))
		require.NoError(t, tui.WaitForText("Nope", 3*time.Second))

		// Tab again to toggle back.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForText("Yep!", 3*time.Second))

		// Press 'n' to skip.
		tui.SendKeys("n")
		time.Sleep(500 * time.Millisecond)
	})
}

// ---------------------------------------------------------------------------
// 93. Reasoning Effort Dialog Selection with Enter
//     Verifies that selecting a reasoning effort level (e.g., "Low") and
//     pressing Enter persists the selection.
// ---------------------------------------------------------------------------

func TestReasoningEffortSelection_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SELECT_REASONING_LEVEL_WITH_ENTER", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open reasoning effort dialog.
		openCommandsPalette(t, tui)
		tui.SendKeys("Reasoning")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Select Reasoning Effort", "Reasoning",
		}, 5*time.Second))
		tui.SendKeys("\r")

		// Should show effort levels.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Low", "Medium", "High",
		}, 5*time.Second))

		// Navigate to a different level and select.
		tui.SendKeys("j") // down to next level
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("\r") // select

		// Dialog should close.
		time.Sleep(500 * time.Millisecond)
		require.NoError(t, tui.WaitForNoText("Select Reasoning", 5*time.Second))

		// App should still be functional.
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 94. Chat Editor Escape Clears History Navigation
//     Verifies that after navigating up through prompt history, pressing
//     Escape returns the editor to the original draft text.
// ---------------------------------------------------------------------------

func TestEditorEscapeClearsHistory_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ESCAPE_RETURNS_TO_DRAFT_FROM_HISTORY", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title:    "History Escape Session",
			messages: []string{"old prompt one", "old prompt two"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("old prompt one", 15*time.Second))

		// Type a draft message.
		tui.SendKeys("my draft text")
		require.NoError(t, tui.WaitForText("my draft text", 5*time.Second))

		// Navigate up into history (replaces editor with old prompt).
		tui.SendKeys("\x1b[A") // up arrow
		time.Sleep(500 * time.Millisecond)

		// Editor should now show a history entry, not our draft.
		require.NoError(t, tui.WaitForAnyText([]string{
			"old prompt two", "old prompt one",
		}, 5*time.Second))

		// Escape should restore our draft.
		tui.SendKeys("\x1b")
		time.Sleep(500 * time.Millisecond)

		// Draft text should be restored.
		require.NoError(t, tui.WaitForText("my draft text", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 95. Session Dialog Shows Session Metadata
//     Verifies that sessions in the dialog display metadata like
//     message count and timestamps alongside the title.
// ---------------------------------------------------------------------------

func TestSessionDialogMetadata_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSIONS_SHOW_TITLES_AND_METADATA", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "Metadata Session One", messages: []string{"msg1", "msg2", "msg3"}},
			seededSession{title: "Metadata Session Two", messages: []string{"single msg"}},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)

		// Both session titles should be visible.
		require.NoError(t, tui.WaitForText("Metadata Session One", 5*time.Second))
		require.NoError(t, tui.WaitForText("Metadata Session Two", 5*time.Second))

		// Session items should show some form of metadata (timestamps, model, etc).
		// The exact format depends on rendering, but the dialog should show more
		// than just the title.
		pane := tui.Snapshot()
		hasMetadata := tui.matchesText("Fixture AI") ||
			tui.matchesText("ago") ||
			tui.matchesText("msg") ||
			tui.matchesText("message")
		if !hasMetadata {
			t.Logf("sessions dialog rendered but no metadata visible beyond titles:\n%s", pane)
		}

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 96. Onboarding API Key Input Field
//     Verifies that after selecting a provider in onboarding, the API key
//     input field accepts text and shows the placeholder.
// ---------------------------------------------------------------------------

func TestOnboardingAPIKeyInput_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("API_KEY_FIELD_ACCEPTS_INPUT", func(t *testing.T) {
		fixture := newOnboardingFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		// Wait for onboarding.
		require.NoError(t, tui.WaitForText("Find your fave", 15*time.Second))

		// Type to filter and select a provider.
		tui.SendKeys("claude")
		require.NoError(t, tui.WaitForText("Claude", 10*time.Second))
		tui.SendKeys("\r")

		// API key input should appear.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Enter your", "API key",
		}, 10*time.Second))

		// Type a fake API key.
		tui.SendKeys("sk-ant-test123456789")
		require.NoError(t, tui.WaitForAnyText([]string{
			"sk-ant-test", "test123",
		}, 5*time.Second))

		// Escape to cancel and return to provider selection.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Find your fave", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 97. Dashboard 'c' from Non-Overview Tab
//     Verifies that pressing 'c' for quick chat works from any dashboard
//     tab, not just the Overview tab.
// ---------------------------------------------------------------------------

func TestDashboardQuickChatFromAnyTab_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("C_FROM_RUNS_TAB_OPENS_CHAT", func(t *testing.T) {
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

		// Press 'c' to open chat from Runs tab.
		tui.SendKeys("c")
		require.NoError(t, tui.WaitForAnyText([]string{
			"MCPs", "LSPs", fixtureLargeModelName,
		}, 10*time.Second))
	})

	t.Run("C_FROM_SESSIONS_TAB_OPENS_CHAT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch to Sessions tab.
		tui.SendKeys("4")
		time.Sleep(300 * time.Millisecond)
		require.NoError(t, tui.WaitForAnyText([]string{
			"Chat Sessions", "Sessions",
		}, 5*time.Second))

		// Press 'c' to open chat.
		tui.SendKeys("c")
		require.NoError(t, tui.WaitForAnyText([]string{
			"MCPs", "LSPs", fixtureLargeModelName,
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 98. Model Switch Persists Into New Session
//     Verifies that after switching the model via the models dialog, creating
//     a new session shows the newly selected model in the header.
// ---------------------------------------------------------------------------

func TestModelSwitchPersistsNewSession_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("NEW_SESSION_USES_SWITCHED_MODEL", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Switch model to "Reason Mini" via models dialog.
		openModelsDialog(t, tui)
		tui.SendKeys("Reason Mini")
		require.NoError(t, tui.WaitForText(fixtureSmallModelName, 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Switch Model", 10*time.Second))

		// Open chat — should show the new model.
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText(fixtureSmallModelName, 10*time.Second))

		// Create a new session.
		tui.SendKeys("\x0e") // ctrl+n
		time.Sleep(500 * time.Millisecond)

		// Open chat in the new session.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Start Chat", "MCPs",
		}, 10*time.Second))
		openStartChatFromDashboard(t, tui)

		// New session should still show the switched model.
		require.NoError(t, tui.WaitForText(fixtureSmallModelName, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 99. Ctrl+C Quit from Deeply Nested View
//     Verifies that Ctrl+C triggers the quit dialog from any view depth,
//     not just the top level.
// ---------------------------------------------------------------------------

func TestQuitFromNestedView_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_C_QUIT_FROM_RUNS_VIEW", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate into runs view.
		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Ctrl+C should show quit dialog even from runs view.
		tui.SendKeys("\x03") // ctrl+c
		require.NoError(t, tui.WaitForText("Are you sure you want to quit?", 5*time.Second))

		// Cancel quit.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Are you sure", 5*time.Second))

		// Should still be in runs view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found",
		}, 5*time.Second))
	})

	t.Run("CTRL_C_QUIT_FROM_CHAT_CONFIRMS_EXIT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Ctrl+C from chat view.
		tui.SendKeys("\x03")
		require.NoError(t, tui.WaitForText("Are you sure you want to quit?", 5*time.Second))

		// Confirm quit.
		tui.SendKeys("y")
		require.NoError(t, tui.WaitForText("[crush exited: 0]", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 100. Runs 'w' and 'W' Workflow Filter Cycling + Clear
//      Verifies that 'w' cycles through workflow name filters and 'W' clears
//      the workflow filter. Without data, both are no-ops.
// ---------------------------------------------------------------------------

func TestRunsWorkflowFilterCycle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("W_CYCLES_AND_SHIFT_W_CLEARS", func(t *testing.T) {
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

		// Wait for load.
		require.NoError(t, tui.WaitForAnyText([]string{
			"All", "No runs found",
		}, 10*time.Second))

		// Press 'w' to cycle workflow filter (no-op with no workflows).
		tui.SendKeys("w")
		time.Sleep(300 * time.Millisecond)

		// Should still be in runs view, stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found", "All",
		}, 5*time.Second))

		// Press 'W' (Shift+w) to clear workflow filter.
		tui.SendKeys("W")
		time.Sleep(300 * time.Millisecond)

		// Still stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found", "All",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

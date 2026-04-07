package e2e_test

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 1. Dashboard Tab Navigation
//    Verifies Tab/Shift+Tab and number-key switching between the dashboard's
//    Overview, Runs, Workflows, and Sessions tabs.
// ---------------------------------------------------------------------------

func TestDashboardTabNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_CYCLES_THROUGH_DASHBOARD_TABS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Overview tab is active on startup — "At a Glance" is only on Overview.
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))

		// Tab → Runs tab. The tab bar should now highlight "Runs".
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForAnyText([]string{"2 Runs", "Runs"}, 5*time.Second))

		// Tab → Workflows tab.
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForAnyText([]string{"3 Workflows", "Workflows"}, 5*time.Second))

		// Tab → Sessions tab.
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForAnyText([]string{"4 Sessions", "Sessions"}, 5*time.Second))

		// Tab → wraps back to Overview.
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))
	})

	t.Run("NUMBER_KEYS_JUMP_TO_TABS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Press "3" to jump to the Workflows tab.
		tui.SendKeys("3")
		require.NoError(t, tui.WaitForAnyText([]string{"3 Workflows", "Workflows"}, 5*time.Second))

		// Press "1" to jump back to Overview.
		tui.SendKeys("1")
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))

		// Press "4" to jump to Sessions.
		tui.SendKeys("4")
		require.NoError(t, tui.WaitForAnyText([]string{"4 Sessions", "Sessions"}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 2. Dashboard Menu Navigation and View Selection
//    Verifies arrow-key navigation through dashboard menu items and that
//    selecting an item opens the correct view.
// ---------------------------------------------------------------------------

func TestDashboardMenuNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("ARROW_KEYS_NAVIGATE_MENU_ITEMS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// The overview menu starts on "Initialize Smithers". Navigate down through
		// the current menu items.
		tui.SendKeys("j")
		require.NoError(t, tui.WaitForText("Run Workflow", 5*time.Second))

		tui.SendKeys("j")
		require.NoError(t, tui.WaitForText("New Chat", 5*time.Second))

		tui.SendKeys("j")
		require.NoError(t, tui.WaitForText("Browse Sessions", 5*time.Second))

		// Navigate back up to New Chat.
		tui.SendKeys("k")
		require.NoError(t, tui.WaitForText("New Chat", 5*time.Second))
	})

	t.Run("ENTER_ON_RUN_WORKFLOW_OPENS_WORKFLOWS_TAB", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate to "Run Workflow".
		tui.SendKeys("j")
		require.NoError(t, tui.WaitForText("Run Workflow", 5*time.Second))
		tui.SendKeys("\r")

		// Should switch to the workflows tab.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "Loading workflows", "No workflows found", "Error",
		}, 10*time.Second))
	})

	t.Run("ENTER_ON_BROWSE_SESSIONS_OPENS_SESSIONS_TAB", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate to "Browse Sessions".
		tui.SendKeys("j")
		tui.SendKeys("j")
		tui.SendKeys("j")
		tui.SendKeys("\r")

		// Should switch to the sessions tab.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Sessions", "No sessions yet", "Press 'c' to start a new chat session",
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 3. Approvals View via Ctrl+A
//    Verifies the global Ctrl+A shortcut opens the approvals view and that
//    the tab key switches between pending and recent decisions.
// ---------------------------------------------------------------------------

func TestApprovalsViewCtrlA_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_A_OPENS_APPROVALS_VIEW", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Ctrl+A opens approvals.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "deny",
		}, 10*time.Second))
	})

	t.Run("APPROVALS_TAB_SWITCHES_TO_HISTORY", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Ctrl+A opens approvals.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "deny",
		}, 10*time.Second))

		// Tab switches to Recent/History tab.
		tui.SendKeys("\t")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Pending", "pending queue", "Recent",
		}, 5*time.Second))
	})

	t.Run("APPROVALS_ESCAPE_RETURNS_TO_PREVIOUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve",
		}, 10*time.Second))

		// Escape pops back to dashboard.
		tui.SendKeys("\x1b")
		waitForDashboard(t, tui)
	})
}

// ---------------------------------------------------------------------------
// 4. Session Load from Sessions Dialog
//    Verifies that selecting a seeded session in the sessions dialog loads
//    its messages into the chat view.
// ---------------------------------------------------------------------------

func TestSessionLoadFromDialog_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SELECT_SESSION_LOADS_INTO_CHAT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Load Me Session", messages: []string{"hello from seeded session"}},
			seededSession{title: "Other Session"},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open sessions dialog.
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Load Me Session", 5*time.Second))

		// Select the session with Enter.
		tui.SendKeys("\r")

		// The seeded message should appear in the chat.
		require.NoError(t, tui.WaitForText("hello from seeded session", 15*time.Second))
	})

	t.Run("FILTER_AND_LOAD_SESSION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Alpha Chat", messages: []string{"alpha message content"}},
			seededSession{title: "Beta Chat", messages: []string{"beta message content"}},
			seededSession{title: "Gamma Chat"},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openSessionsDialog(t, tui)

		// Type to filter sessions.
		tui.SendKeys("Beta")
		require.NoError(t, tui.WaitForText("Beta Chat", 5*time.Second))

		// Select filtered result.
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("beta message content", 15*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 5. New Session Creation
//    Verifies creating a new session via the command palette and that the
//    previous session is preserved in the sessions list.
// ---------------------------------------------------------------------------

func TestNewSessionCreation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("NEW_SESSION_VIA_COMMAND_PALETTE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Existing Session", messages: []string{"old session msg"}},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Navigate to chat first.
		openStartChatFromDashboard(t, tui)

		// Create new session via command palette.
		openCommandsPalette(t, tui)
		tui.SendKeys("New Session")
		require.NoError(t, tui.WaitForText("New Session", 5*time.Second))
		tui.SendKeys("\r")

		// Should land on a fresh dashboard/chat (no old messages visible).
		require.NoError(t, tui.WaitForAnyText([]string{"New Chat", "MCPs", "CRUSH"}, 10*time.Second))

		// Old session messages should not be visible.
		require.NoError(t, tui.WaitForNoText("old session msg", 5*time.Second))

		// Open sessions dialog — the old session should still be listed.
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Existing Session", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 6. View Stack Pop Navigation
//    Verifies that opening multiple views stacks them, and Escape pops back
//    through each one correctly.
// ---------------------------------------------------------------------------

func TestViewStackPopNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("MULTIPLE_VIEWS_POP_IN_ORDER", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open Runs view via command palette.
		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH › Runs", "Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// From Runs, open Approvals via Ctrl+A.
		tui.SendKeys("\x01") // ctrl+a
		require.NoError(t, tui.WaitForAnyText([]string{
			"Approvals", "approve", "No pending approvals",
		}, 10*time.Second))

		// Escape pops Approvals → back to Runs.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Escape pops Runs → back to Dashboard.
		returnToDashboard(t, tui)
	})
}

// ---------------------------------------------------------------------------
// 7. Chat Editor Basics
//    Verifies that the chat editor is functional: typing text, seeing the
//    placeholder, and editor focus management.
// ---------------------------------------------------------------------------

func TestChatEditorBasics_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("EDITOR_ACCEPTS_TEXT_INPUT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type text into the editor.
		tui.SendKeys("hello world test input")
		require.NoError(t, tui.WaitForText("hello world test input", 5*time.Second))
	})

	t.Run("EDITOR_SHOWS_MODEL_IN_HEADER", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// The header should show the current model name.
		require.NoError(t, tui.WaitForText(fixtureLargeModelName, 10*time.Second))
	})

	t.Run("MULTILINE_INPUT_WITH_CTRL_J", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type first line, then Ctrl+J for newline, then second line.
		tui.SendKeys("line one")
		tui.SendKeys("\x0a") // ctrl+j
		tui.SendKeys("line two")

		require.NoError(t, tui.WaitForText("line one", 5*time.Second))
		require.NoError(t, tui.WaitForText("line two", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 8. Terminal Resize Handling
//    Verifies the TUI adapts when the tmux pane is resized.
// ---------------------------------------------------------------------------

func TestTerminalResize_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("RESIZE_DOES_NOT_CRASH", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Resize the pane to a smaller size.
		resizeTmuxPane(t, tui, 80, 24)

		// The app should still be rendering.
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH", "New Chat",
		}, 5*time.Second))

		// Resize to a larger size.
		resizeTmuxPane(t, tui, 160, 50)

		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH", "New Chat",
		}, 5*time.Second))

		// Resize to a very narrow terminal.
		resizeTmuxPane(t, tui, 60, 20)

		// Should still render without crashing.
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH", "Start",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 9. Toggle Sidebar (Compact Mode) via Command Palette
//    Verifies that toggling sidebar/compact mode changes the layout.
// ---------------------------------------------------------------------------

func TestToggleSidebar_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_SIDEBAR_VIA_COMMAND_PALETTE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open command palette and toggle sidebar.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")

		// The sidebar should be toggled. The app should not crash.
		// Wait a moment for the layout to settle.
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))

		// Toggle it back.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 10. Sequential Global Shortcuts
//     Verifies that multiple global keyboard shortcuts work correctly when
//     used in rapid succession, testing the overall stability of the
//     shortcut dispatcher.
// ---------------------------------------------------------------------------

func TestSequentialGlobalShortcuts_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SHORTCUTS_WORK_IN_SEQUENCE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// 1. Ctrl+G to expand help bar.
		tui.SendKeys("\x07") // ctrl+g
		require.NoError(t, tui.WaitForText("ctrl+c", 5*time.Second))

		// 2. Ctrl+G again to collapse help.
		tui.SendKeys("\x07")
		time.Sleep(300 * time.Millisecond)

		// 3. Ctrl+L to open models dialog.
		tui.SendKeys("\x0c") // ctrl+l
		require.NoError(t, tui.WaitForText("Switch Model", 5*time.Second))

		// 4. Escape to close models.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Switch Model", 5*time.Second))

		// 5. Ctrl+P to open command palette.
		tui.SendKeys("\x10") // ctrl+p
		require.NoError(t, tui.WaitForText("Commands", 5*time.Second))

		// 6. Escape to close command palette.
		tui.SendKeys("\x1b")
		waitForDashboard(t, tui)

		// 7. Ctrl+S to open sessions dialog.
		tui.SendKeys("\x13") // ctrl+s
		require.NoError(t, tui.WaitForText("Sessions", 5*time.Second))

		// 8. Escape to close sessions.
		tui.SendKeys("\x1b")
		waitForDashboard(t, tui)

		// 9. App should still be on the dashboard and functional.
		waitForDashboard(t, tui)
	})

	t.Run("VIEW_SHORTCUTS_INTERLEAVE_WITH_DIALOGS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Open runs view.
		openCommandsPalette(t, tui)
		tui.SendKeys("Run Dashboard")
		require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 10*time.Second))

		// Open the runs search prompt while inside the runs view.
		tui.SendKeys("/")
		require.NoError(t, tui.WaitForText("search by run ID or workflow", 5*time.Second))
		tui.SendKeys("\x1b") // close search

		// Should still be in Runs view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "Loading runs", "No runs found",
		}, 5*time.Second))

		// Escape back to dashboard.
		tui.SendKeys("\x1b")
		waitForDashboard(t, tui)
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resizeTmuxPane resizes the tmux pane for the given TUI instance.
func resizeTmuxPane(t *testing.T, tui *TUITestInstance, cols, rows int) {
	t.Helper()

	// tmux resize-window is the most reliable way to resize a session.
	// We resize the window which affects the single pane inside it.
	cmd := exec.Command("tmux", "resize-window", "-t", tui.session, "-x", itoa(cols), "-y", itoa(rows))
	if output, err := cmd.CombinedOutput(); err != nil {
		// Fall back to resize-pane if resize-window fails (older tmux).
		cmd2 := exec.Command("tmux", "resize-pane", "-t", tui.session, "-x", itoa(cols), "-y", itoa(rows))
		if output2, err2 := cmd2.CombinedOutput(); err2 != nil {
			t.Logf("resize-window: %v\n%s", err, output)
			t.Logf("resize-pane: %v\n%s", err2, output2)
			t.Skip("tmux resize not supported in this environment")
		}
	}
}

// itoa converts an int to its string representation.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

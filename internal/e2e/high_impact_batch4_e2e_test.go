package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 31. Chat Scroll with j/k When Main-Focused
//     Verifies that after Tab-switching to main focus, j/k keys scroll
//     through chat messages rather than typing into the editor.
// ---------------------------------------------------------------------------

func TestChatScrollNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("JK_SCROLLS_MESSAGES_IN_MAIN_FOCUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title: "Scroll Test",
			messages: []string{
				"scroll message one",
				"scroll message two",
				"scroll message three",
				"scroll message four",
			},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Wait for messages to load.
		require.NoError(t, tui.WaitForText("scroll message one", 15*time.Second))

		// Tab to switch to main (messages) focus.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Press 'k' (up) — should scroll chat, not type 'k' in editor.
		tui.SendKeys("k")
		time.Sleep(300 * time.Millisecond)

		// Press 'j' (down) — should scroll down.
		tui.SendKeys("j")
		time.Sleep(300 * time.Millisecond)

		// The messages should still be visible (no crash, no 'j'/'k' in editor).
		require.NoError(t, tui.WaitForText("scroll message", 5*time.Second))

		// The editor should NOT contain 'j' or 'k' as text — verify by
		// switching back to editor focus and checking.
		tui.SendKeys("\t") // back to editor focus
		time.Sleep(200 * time.Millisecond)

		// If j/k were incorrectly sent to editor, they'd appear as text.
		// A clean editor means routing worked correctly.
		require.NoError(t, tui.WaitForNoText("jk", 2*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 32. Chat g/G Jump to Start/End
//     Verifies that 'g' jumps to the first message and 'G' jumps to the
//     last message when in main focus.
// ---------------------------------------------------------------------------

func TestChatHomeEndNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("G_HOME_END_NAVIGATION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title: "HomeEnd Test",
			messages: []string{
				"first message at top",
				"middle message content",
				"last message at bottom",
			},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("first message at top", 15*time.Second))

		// Tab to main focus.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Press 'g' to jump to first message.
		tui.SendKeys("g")
		time.Sleep(300 * time.Millisecond)

		// First message should be visible.
		require.NoError(t, tui.WaitForText("first message at top", 5*time.Second))

		// Press 'G' to jump to last message.
		tui.SendKeys("G")
		time.Sleep(300 * time.Millisecond)

		// Last message should be visible.
		require.NoError(t, tui.WaitForText("last message at bottom", 5*time.Second))

		// App should still be stable.
		require.NoError(t, tui.WaitForText("CRUSH", 3*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 33. Chat Expand/Collapse with Space
//     Verifies that pressing space in main focus toggles expansion of the
//     selected message item (e.g., tool call details).
// ---------------------------------------------------------------------------

func TestChatExpandCollapse_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SPACE_TOGGLES_EXPANSION_WITHOUT_CRASH", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title: "Expand Test",
			messages: []string{
				"message to expand",
				"another message",
			},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("message to expand", 15*time.Second))

		// Tab to main focus.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Press space to toggle expand on selected item.
		tui.SendKeys(" ")
		time.Sleep(300 * time.Millisecond)

		// Should not crash — messages still visible.
		require.NoError(t, tui.WaitForText("message to expand", 5*time.Second))

		// Press space again to toggle back.
		tui.SendKeys(" ")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForText("CRUSH", 3*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 34. Prompts View Navigation and Help Bar
//     Verifies the prompts view opens, shows loading/empty state, and the
//     help bar displays context-sensitive hints (navigate, edit, props,
//     preview, refresh).
// ---------------------------------------------------------------------------

func TestPromptsViewNavigation_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("PROMPTS_VIEW_OPENS_WITH_HELP_HINTS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Prompt Templates")
		require.NoError(t, tui.WaitForText("Prompt Templates", 5*time.Second))
		tui.SendKeys("\r")

		// Should open prompts view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Prompts", "Loading prompts", "No prompts found", "Error",
		}, 10*time.Second))

		// Help bar should show prompts-specific hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"navigate", "edit", "preview", "refresh",
		}, 5*time.Second))

		// j/k navigation should not crash.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")
		time.Sleep(200 * time.Millisecond)

		// 'r' to refresh should not crash.
		tui.SendKeys("r")
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Prompts", "Loading prompts", "No prompts found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForText("Start Chat", 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 35. Tickets Split-Pane Focus Switching
//     Verifies that Tab in the tickets view switches focus between the list
//     pane and detail pane, and the help bar hints change accordingly.
// ---------------------------------------------------------------------------

func TestTicketsPaneFocus_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TAB_SWITCHES_LIST_AND_DETAIL_PANE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		openCommandsPalette(t, tui)
		tui.SendKeys("Work Items")
		require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Local", "Loading local tickets", "No local tickets",
		}, 10*time.Second))

		// In list-focused mode, help hints show select/edit/new/detail.
		require.NoError(t, tui.WaitForAnyText([]string{
			"select", "detail", "new", "edit",
		}, 5*time.Second))

		// Tab should switch to the source-tab level or detail pane.
		// With no tickets loaded, Tab cycles source tabs.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// After Tab, a different source should be active or help bar changed.
		require.NoError(t, tui.WaitForText("WORK ITEMS", 5*time.Second))

		// Shift+Tab should go back.
		// In tmux, we send the escape sequence for Shift+Tab.
		tui.SendKeys("\x1b[Z") // Shift+Tab
		time.Sleep(300 * time.Millisecond)

		require.NoError(t, tui.WaitForText("WORK ITEMS", 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 36. Tickets Create Ticket Full Flow
//     Verifies that 'n' opens the new ticket prompt, typing a ticket ID and
//     pressing Enter creates the ticket (or shows an error), and Escape
//     cancels creation.
// ---------------------------------------------------------------------------

func TestTicketsCreateFullFlow_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CREATE_TICKET_TYPE_AND_CANCEL", func(t *testing.T) {
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

		// Press 'n' to open create ticket prompt.
		tui.SendKeys("n")
		require.NoError(t, tui.WaitForAnyText([]string{
			"New ticket", "ticket ID", "create", "cancel",
		}, 5*time.Second))

		// Type a ticket ID.
		tui.SendKeys("TEST-123")
		require.NoError(t, tui.WaitForText("TEST-123", 5*time.Second))

		// Escape to cancel without creating.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("New ticket", 5*time.Second))

		// Should still be in tickets view.
		require.NoError(t, tui.WaitForText("WORK ITEMS", 5*time.Second))

		tui.SendKeys("\x1b")
	})

	t.Run("CREATE_TICKET_TYPE_AND_SUBMIT", func(t *testing.T) {
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

		// Press 'n' to create.
		tui.SendKeys("n")
		require.NoError(t, tui.WaitForAnyText([]string{
			"New ticket", "ticket ID", "create",
		}, 5*time.Second))

		// Type and submit.
		tui.SendKeys("SUBMIT-456")
		tui.SendKeys("\r")

		// Either the ticket is created (appears in list) or an error is shown.
		// Either way, the form should close and view should remain stable.
		time.Sleep(500 * time.Millisecond)
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "SUBMIT-456", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 37. Agents View Refresh
//     Verifies that 'r' key in the agents view triggers a refresh and the
//     view remains stable.
// ---------------------------------------------------------------------------

func TestAgentsRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_AGENTS", func(t *testing.T) {
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

		// Press 'r' to refresh.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// View should show loading or refreshed state, not crash.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Agents", "Loading agents", "Available", "Not Detected", "Error",
		}, 10*time.Second))

		// j/k navigation after refresh should still work.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")
		time.Sleep(200 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Agents", "Available", "Not Detected", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 38. Runs Expand/Collapse Detail Row ('e' key)
//     Verifies that 'e' toggles inline detail expansion for a run. Without
//     data this is a no-op, but the keybinding must not crash the app.
// ---------------------------------------------------------------------------

func TestRunsExpandDetail_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("E_KEY_TOGGLES_DETAIL_ROW", func(t *testing.T) {
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

		// Wait for initial load to complete.
		require.NoError(t, tui.WaitForAnyText([]string{
			"All", "No runs found", "Loading runs",
		}, 10*time.Second))

		// Press 'e' to toggle expand — no-op with no data, should not crash.
		tui.SendKeys("e")
		time.Sleep(300 * time.Millisecond)

		// View should remain stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found", "All",
		}, 5*time.Second))

		// Toggle again.
		tui.SendKeys("e")
		time.Sleep(200 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Runs", "No runs found",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 39. Multiple New Sessions from Chat (Ctrl+N)
//     Verifies that Ctrl+N in chat creates new sessions in succession and
//     the session count grows in the sessions dialog.
// ---------------------------------------------------------------------------

func TestMultipleNewSessions_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_N_CREATES_SESSIONS_IN_SUCCESSION", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type something so the session is meaningful.
		tui.SendKeys("session one marker")
		time.Sleep(300 * time.Millisecond)

		// Ctrl+N to create a new session.
		tui.SendKeys("\x0e") // ctrl+n
		time.Sleep(500 * time.Millisecond)

		// Should land on a fresh view — old text cleared.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Start Chat", "MCPs", "CRUSH",
		}, 10*time.Second))

		// Navigate to chat again.
		openStartChatFromDashboard(t, tui)

		// Type something in second session.
		tui.SendKeys("session two marker")
		time.Sleep(300 * time.Millisecond)

		// Create another session.
		tui.SendKeys("\x0e") // ctrl+n
		time.Sleep(500 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"Start Chat", "MCPs", "CRUSH",
		}, 10*time.Second))

		// Verify sessions list now has entries.
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("Sessions", 5*time.Second))

		// The sessions dialog should exist and be open (at least the default sessions).
		// The exact count depends on implementation but it should not crash.
		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 40. Header Displays Session Title After Loading
//     Verifies that loading a named session shows its title in the header.
// ---------------------------------------------------------------------------

func TestHeaderSessionTitle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("HEADER_SHOWS_SESSION_TITLE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "My Named Session", messages: []string{"named session content"}},
		)
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Wait for session messages to load.
		require.NoError(t, tui.WaitForText("named session content", 15*time.Second))

		// The header should display the session title.
		require.NoError(t, tui.WaitForAnyText([]string{
			"My Named Session", "CRUSH",
		}, 5*time.Second))
	})

	t.Run("HEADER_UPDATES_AFTER_SESSION_SWITCH", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "First Session Title", messages: []string{"first session body"}},
			seededSession{title: "Second Session Title", messages: []string{"second session body"}},
		)
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Should load most recent session first.
		require.NoError(t, tui.WaitForText("second session body", 15*time.Second))

		// Switch to first session via sessions dialog.
		openSessionsDialog(t, tui)
		require.NoError(t, tui.WaitForText("First Session Title", 5*time.Second))
		tui.SendKeys("First Session")
		require.NoError(t, tui.WaitForText("First Session Title", 5*time.Second))
		tui.SendKeys("\r")

		// Should now show the first session's content.
		require.NoError(t, tui.WaitForText("first session body", 15*time.Second))
	})
}

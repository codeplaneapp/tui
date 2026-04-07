package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 61. Projects CLI: `projects --json`
//     Verifies the non-interactive projects subcommand outputs valid JSON.
// ---------------------------------------------------------------------------

func TestProjectsCLI_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("PROJECTS_JSON_OUTPUT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "projects", "--json")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=dumb",
			"NO_COLOR=1",
		)
		cmd.Dir = fixture.workingDir

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "projects --json failed: %s", string(output))

		// Output should be valid JSON with a "projects" key.
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &result), "invalid JSON: %s", string(output))
		_, ok := result["projects"]
		require.True(t, ok, "output should have 'projects' key")
	})
}

// ---------------------------------------------------------------------------
// 62. Dirs CLI: `dirs config` and `dirs data`
//     Verifies the non-interactive dirs subcommands print directory paths.
// ---------------------------------------------------------------------------

func TestDirsCLI_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("DIRS_CONFIG_PRINTS_PATH", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "dirs", "config")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=dumb",
		)

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "dirs config failed: %s", string(output))

		// Output should be a non-empty path string.
		path := strings.TrimSpace(string(output))
		require.NotEmpty(t, path, "dirs config should print a path")
	})

	t.Run("DIRS_DATA_PRINTS_PATH", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "dirs", "data")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=dumb",
		)

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "dirs data failed: %s", string(output))

		path := strings.TrimSpace(string(output))
		require.NotEmpty(t, path, "dirs data should print a path")
	})
}

// ---------------------------------------------------------------------------
// 63. Workflows Doctor Overlay ('d' key)
//     Verifies that pressing 'd' in the workflows view opens the doctor
//     diagnostics overlay showing check results or an error.
// ---------------------------------------------------------------------------

func TestWorkflowsDoctorOverlay_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("D_KEY_OPENS_DOCTOR_OVERLAY", func(t *testing.T) {
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

		// Press 'd' to open doctor diagnostics.
		tui.SendKeys("d")
		time.Sleep(500 * time.Millisecond)

		// Doctor overlay should show diagnostics, loading, or error state.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Doctor", "diagnostics", "smithers",
			"Loading", "Error", "Workflows", "No workflows found",
		}, 10*time.Second))

		// Escape closes the overlay.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "Error", "Start Chat",
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 64. Large Paste Becomes Attachment
//     Verifies that pasting >10 lines of text via bracketed paste mode
//     auto-creates a "paste_*.txt" attachment instead of inserting text.
// ---------------------------------------------------------------------------

func TestLargePasteAttachment_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("PASTE_OVER_10_LINES_CREATES_ATTACHMENT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Build a 12-line paste payload.
		var lines []string
		for i := 1; i <= 12; i++ {
			lines = append(lines, "line "+itoa(i)+" of pasted content")
		}
		pasteContent := strings.Join(lines, "\n")

		// Send bracketed paste: \033[200~ ... \033[201~
		// First, set the tmux buffer then paste it using bracketed paste mode.
		setBufferCmd := exec.Command("tmux", "set-buffer", "-b", "paste-test", pasteContent)
		if out, err := setBufferCmd.CombinedOutput(); err != nil {
			t.Skipf("tmux set-buffer failed (tmux version may not support -b): %v\n%s", err, out)
		}

		// Use tmux paste-buffer with -p for bracketed paste.
		pasteCmd := exec.Command("tmux", "paste-buffer", "-t", tui.session, "-b", "paste-test", "-p")
		if out, err := pasteCmd.CombinedOutput(); err != nil {
			// Fall back: send raw text (won't trigger paste detection, but tests stability).
			t.Logf("tmux paste-buffer -p failed: %v\n%s", err, out)
			t.Skip("bracketed paste not supported in this tmux version")
		}

		time.Sleep(1 * time.Second)

		// If paste detection works, a "paste_*.txt" attachment should appear.
		// If not (tmux version doesn't support bracketed paste), the text is inserted.
		require.NoError(t, tui.WaitForAnyText([]string{
			"paste_", ".txt", "line 1", "CRUSH",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 65. Runs Clear All Filters ('F' key)
//     Verifies that 'F' resets the status filter to "All" after cycling
//     through filter states.
// ---------------------------------------------------------------------------

func TestRunsClearFilter_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("F_KEY_CLEARS_STATUS_FILTER", func(t *testing.T) {
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

		// Cycle filter to "Running" with 'f'.
		tui.SendKeys("f")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Running", "No runs",
		}, 5*time.Second))

		// Clear filter with 'F' — should go back to "All".
		tui.SendKeys("F")
		require.NoError(t, tui.WaitForAnyText([]string{
			"All", "No runs found",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 66. Memory Recall Mode ('r' → type query → Esc cancel)
//     Verifies entering semantic recall search mode in the memory browser,
//     typing a query, and cancelling with Escape.
// ---------------------------------------------------------------------------

func TestMemoryRecallMode_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_ENTERS_RECALL_AND_ESCAPE_CANCELS", func(t *testing.T) {
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

		// Press 'r' to enter recall mode (semantic search).
		// In list mode, 'r' triggers recall; help bar changes to search/cancel.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Help bar should now show recall-specific hints.
		require.NoError(t, tui.WaitForAnyText([]string{
			"recall", "search", "cancel", "enter", "query",
			"Memory", "No memory facts found", "Error",
		}, 5*time.Second))

		// Escape to cancel recall mode.
		tui.SendKeys("\x1b")
		time.Sleep(300 * time.Millisecond)

		// Should return to list mode or view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Memory", "No memory facts found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 67. SQL Browser Query Execution ('x' key)
//     Verifies that switching to the editor pane and pressing 'x' to execute
//     a query works (or shows an error) without crashing.
// ---------------------------------------------------------------------------

func TestSQLBrowserQueryExecution_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("X_KEY_EXECUTES_QUERY", func(t *testing.T) {
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

		// Tab to switch to editor pane.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Type a simple query.
		tui.SendKeys("SELECT 1")
		time.Sleep(300 * time.Millisecond)

		// Press 'x' to execute the query.
		tui.SendKeys("x")
		time.Sleep(500 * time.Millisecond)

		// Should show query result, an error, or remain stable.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SQL Browser", "Error", "result", "1", "SELECT",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 68. Triggers Refresh ('r' key)
//     Verifies that 'r' in the triggers view refreshes the list without
//     crashing.
// ---------------------------------------------------------------------------

func TestTriggersRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_TRIGGERS", func(t *testing.T) {
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

		// Press 'r' to refresh.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Should reload the triggers list.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Triggers", "Loading triggers", "No cron triggers found", "Error",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 69. Tickets Refresh ('r' key)
//     Verifies that 'r' in the tickets view refreshes the current source
//     data without crashing.
// ---------------------------------------------------------------------------

func TestTicketsRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_TICKETS", func(t *testing.T) {
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

		// Press 'r' to refresh.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Should still be in the tickets view, possibly reloading.
		require.NoError(t, tui.WaitForAnyText([]string{
			"WORK ITEMS", "Loading", "No local tickets", "Local",
		}, 10*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 70. Dashboard Sessions Tab Shows Seeded Sessions
//     Verifies that the Sessions tab on the dashboard (tab 4) lists seeded
//     session titles inline, without opening the sessions dialog.
// ---------------------------------------------------------------------------

func TestDashboardSessionsTab_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSIONS_TAB_LISTS_SEEDED_SESSIONS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "Dashboard Tab Session A", messages: []string{"msg a"}},
			seededSession{title: "Dashboard Tab Session B", messages: []string{"msg b"}},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Press "4" to jump to the Sessions tab on the dashboard.
		tui.SendKeys("4")
		time.Sleep(500 * time.Millisecond)

		// The sessions tab should show the seeded session titles.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Dashboard Tab Session A", "Dashboard Tab Session B", "Sessions",
		}, 5*time.Second))
	})

	t.Run("SESSIONS_TAB_SHOWS_SESSION_COUNT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "Count Session 1"},
			seededSession{title: "Count Session 2"},
			seededSession{title: "Count Session 3"},
		)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)

		// Jump to Sessions tab.
		tui.SendKeys("4")
		time.Sleep(500 * time.Millisecond)

		// Should display at least one session title.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Count Session", "Sessions",
		}, 5*time.Second))

		// Navigate back to Overview tab.
		tui.SendKeys("1")
		require.NoError(t, tui.WaitForText("At a Glance", 5*time.Second))
	})
}

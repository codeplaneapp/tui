package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 41. Reasoning Effort Dialog
//     Verifies that "Select Reasoning Effort" in the command palette opens
//     the reasoning effort selection dialog with effort levels visible.
// ---------------------------------------------------------------------------

func TestReasoningEffortDialog_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("REASONING_DIALOG_OPENS_AND_SHOWS_LEVELS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open command palette and search for reasoning effort.
		openCommandsPalette(t, tui)
		tui.SendKeys("Reasoning")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Select Reasoning Effort", "Reasoning",
		}, 5*time.Second))
		tui.SendKeys("\r")

		// The reasoning dialog should show effort levels.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Low", "Medium", "High",
		}, 5*time.Second))

		// Escape to cancel without selecting.
		tui.SendKeys("\x1b")
		require.NoError(t, tui.WaitForNoText("Low", 5*time.Second))

		// App should still be functional.
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 42. Thinking Mode Toggle
//     Verifies that toggling Thinking Mode on/off via the command palette
//     does not crash the app and the command is discoverable.
// ---------------------------------------------------------------------------

func TestThinkingModeToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_THINKING_VIA_COMMANDS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open command palette and search for thinking mode.
		openCommandsPalette(t, tui)
		tui.SendKeys("Thinking")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Thinking Mode", "Thinking",
		}, 5*time.Second))
		tui.SendKeys("\r")

		// Dialog should close and app should be stable.
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))

		// Toggle again (should reverse the state).
		openCommandsPalette(t, tui)
		tui.SendKeys("Thinking")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Thinking Mode", "Thinking",
		}, 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 43. Initialize Project Prompt
//     Verifies the initialization prompt ("Would you like to initialize this
//     project?") can be launched via command palette, and the y/n/tab
//     buttons work correctly.
// ---------------------------------------------------------------------------

func TestInitializeProjectPrompt_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("INITIALIZE_PROMPT_OPENS_AND_CANCELS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open command palette and trigger Initialize Project.
		openCommandsPalette(t, tui)
		tui.SendKeys("Initialize Project")
		require.NoError(t, tui.WaitForText("Initialize Project", 5*time.Second))
		tui.SendKeys("\r")

		// Should show the initialization prompt.
		require.NoError(t, tui.WaitForAnyText([]string{
			"initialize", "Initialize", "Yep!", "Nope",
		}, 10*time.Second))

		// Press 'n' to skip initialization.
		tui.SendKeys("n")

		// Should return to chat/landing view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH", "MCPs",
		}, 10*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 44. Session CLI: `session list --json`
//     Verifies the non-interactive session list subcommand outputs valid
//     JSON with seeded sessions.
// ---------------------------------------------------------------------------

func TestSessionCLIList_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_LIST_JSON_OUTPUT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "CLI Session Alpha", messages: []string{"alpha content"}},
			seededSession{title: "CLI Session Beta", messages: []string{"beta content"}},
		)

		binary := buildSharedTUIBinary(t)

		// Run `session list --json` non-interactively.
		cmd := exec.Command(binary, "session", "list", "--json")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=xterm-256color",
		)
		cmd.Dir = fixture.workingDir

		output, err := cmd.Output()
		require.NoError(t, err, "session list --json failed: %s", string(output))

		// Output should be valid JSON.
		var sessions []map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &sessions), "invalid JSON: %s", string(output))

		// Should contain our seeded sessions.
		require.GreaterOrEqual(t, len(sessions), 2, "expected at least 2 sessions")
	})
}

// ---------------------------------------------------------------------------
// 45. Session CLI: `session last --json`
//     Verifies that `session last` returns the most recently updated session.
// ---------------------------------------------------------------------------

func TestSessionCLILast_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_LAST_JSON_OUTPUT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir,
			seededSession{title: "Old Session", messages: []string{"old"}},
			seededSession{title: "Latest Session", messages: []string{"latest"}},
		)

		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "session", "last", "--json")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=xterm-256color",
		)
		cmd.Dir = fixture.workingDir

		output, err := cmd.Output()
		require.NoError(t, err, "session last --json failed: %s", string(output))

		// Should be valid JSON with a title field.
		var session map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &session), "invalid JSON: %s", string(output))

		title, ok := session["title"].(string)
		require.True(t, ok, "session should have a title field")
		require.Equal(t, "Latest Session", title, "should return the most recent session")
	})
}

// ---------------------------------------------------------------------------
// 46. @ Completions Popup
//     Verifies that typing '@' opens the completions popup, which can be
//     cancelled with Escape.
// ---------------------------------------------------------------------------

func TestAtCompletionsPopup_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("AT_OPENS_COMPLETIONS_AND_ESCAPE_CLOSES", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		// Create files in the working directory for completions to find.
		writeTextFixture(t, fixture.workingDir, "readme.md", "# Test")
		writeTextFixture(t, fixture.workingDir, "main.go", "package main")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type '@' to trigger completions popup.
		tui.SendKeys("@")

		// The completions popup should appear with file names.
		require.NoError(t, tui.WaitForAnyText([]string{
			"readme.md", "main.go", ".md", ".go",
		}, 5*time.Second))

		// Escape closes the completions popup.
		tui.SendKeys("\x1b")

		// App should still be in chat with '@' in the editor.
		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})

	t.Run("AT_FILTER_NARROWS_COMPLETIONS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writeTextFixture(t, fixture.workingDir, "readme.md", "# Test")
		writeTextFixture(t, fixture.workingDir, "main.go", "package main")
		writeTextFixture(t, fixture.workingDir, "readme2.txt", "readme 2")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type '@read' to trigger completions and filter to "readme" files.
		tui.SendKeys("@read")

		// Should show readme files.
		require.NoError(t, tui.WaitForAnyText([]string{
			"readme", "readme.md", "readme2",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 47. Notifications Toggle via Command Palette
//     Verifies toggling notifications on/off via the command palette shows
//     a status message and does not crash.
// ---------------------------------------------------------------------------

func TestNotificationsToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_NOTIFICATIONS_VIA_COMMANDS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Open command palette and search for notifications toggle.
		openCommandsPalette(t, tui)
		tui.SendKeys("Notification")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Notification", "Enable", "Disable",
		}, 5*time.Second))
		tui.SendKeys("\r")

		// Should show a status message about notifications being toggled.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Notifications", "enabled", "disabled", "CRUSH",
		}, 5*time.Second))

		// Toggle back.
		openCommandsPalette(t, tui)
		tui.SendKeys("Notification")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Notification", "Enable", "Disable",
		}, 5*time.Second))
		tui.SendKeys("\r")

		require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 48. Workflows Refresh ('r' key)
//     Verifies that 'r' in the workflows view triggers a data refresh and
//     the view remains stable.
// ---------------------------------------------------------------------------

func TestWorkflowsRefresh_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("R_KEY_REFRESHES_WORKFLOWS", func(t *testing.T) {
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

		// Press 'r' to refresh.
		tui.SendKeys("r")

		// View should reload (show loading or refreshed data).
		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "Loading workflows", "No workflows found", "Error",
		}, 10*time.Second))

		// Navigation still works after refresh.
		tui.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		tui.SendKeys("k")

		require.NoError(t, tui.WaitForAnyText([]string{
			"Workflows", "No workflows found", "Error",
		}, 5*time.Second))

		tui.SendKeys("\x1b")
	})
}

// ---------------------------------------------------------------------------
// 49. Chat Ctrl+N Creates New Session from Main Focus
//     Verifies that Ctrl+N works from main (messages) focus, not just from
//     editor focus, to create a new session.
// ---------------------------------------------------------------------------

func TestCtrlNFromMainFocus_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_N_WORKS_FROM_MAIN_FOCUS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		seedSessions(t, fixture.dataDir, seededSession{
			title:    "Main Focus Session",
			messages: []string{"main focus content"},
		})
		tui := launchFixtureTUI(t, fixture, "--continue")
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)
		require.NoError(t, tui.WaitForText("main focus content", 15*time.Second))

		// Tab to main focus.
		tui.SendKeys("\t")
		time.Sleep(300 * time.Millisecond)

		// Ctrl+N to create a new session from main focus.
		tui.SendKeys("\x0e") // ctrl+n

		// Should land on a fresh view.
		require.NoError(t, tui.WaitForAnyText([]string{
			"Start Chat", "MCPs", "CRUSH",
		}, 10*time.Second))

		// Old message should be gone.
		require.NoError(t, tui.WaitForNoText("main focus content", 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 50. Multiple File Attachments
//     Verifies that multiple files can be attached and all appear in the
//     attachment indicator area.
// ---------------------------------------------------------------------------

func TestMultipleFileAttachments_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TWO_IMAGES_BOTH_SHOWN", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writePNGFixture(t, fixture.workingDir, "photo1.png")
		writePNGFixture(t, fixture.workingDir, "photo2.png")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Add first image via Ctrl+F.
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		// Navigate to select photo1.
		tui.SendKeys("photo1")
		require.NoError(t, tui.WaitForText("photo1.png", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))
		require.NoError(t, tui.WaitForText("photo1.png", 10*time.Second))

		// Add second image via Ctrl+F.
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		tui.SendKeys("photo2")
		require.NoError(t, tui.WaitForText("photo2.png", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))

		// Both attachments should be visible.
		require.NoError(t, tui.WaitForText("photo1.png", 5*time.Second))
		require.NoError(t, tui.WaitForText("photo2.png", 5*time.Second))
	})

	t.Run("TEXT_AND_IMAGE_MIXED_ATTACHMENTS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writePNGFixture(t, fixture.workingDir, "diagram.png")
		writeTextFixture(t, fixture.workingDir, "notes.txt", "important notes")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Add image via Ctrl+F.
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		tui.SendKeys("diagram")
		require.NoError(t, tui.WaitForText("diagram.png", 5*time.Second))
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))

		// Add text file via @ mention.
		tui.SendKeys("@notes")
		require.NoError(t, tui.WaitForText("notes.txt", 10*time.Second))
		tui.SendKeys("\r")

		// Both should appear as attachments.
		require.NoError(t, tui.WaitForText("diagram.png", 5*time.Second))
		require.NoError(t, tui.WaitForText("notes.txt", 5*time.Second))
	})
}

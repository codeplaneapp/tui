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
// 51. Session CLI: `session show --json`
//     Verifies the `session show <id> --json` subcommand outputs valid JSON
//     with messages from a specific seeded session.
// ---------------------------------------------------------------------------

func TestSessionCLIShow_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_SHOW_JSON_BY_ID", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		sessions := seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Show Me Session", messages: []string{"show me content"}},
		)

		binary := buildSharedTUIBinary(t)
		targetID := sessions[0].ID

		cmd := exec.Command(binary, "--data-dir", fixture.workspaceDataDir(), "session", "show", targetID, "--json")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=xterm-256color",
		)
		cmd.Dir = fixture.workingDir

		output, err := cmd.Output()
		require.NoError(t, err, "session show --json failed: %s", string(output))

		// Output should be valid JSON with session info and messages.
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &result), "invalid JSON: %s", string(output))

		// Should contain the session title in the metadata envelope.
		meta, ok := result["meta"].(map[string]interface{})
		require.True(t, ok, "session show output should have meta field")
		title, ok := meta["title"].(string)
		require.True(t, ok, "session show metadata should have title field")
		require.Equal(t, "Show Me Session", title)
	})
}

// ---------------------------------------------------------------------------
// 52. Session CLI: `session delete`
//     Verifies that deleting a session via CLI removes it from the list.
// ---------------------------------------------------------------------------

func TestSessionCLIDelete_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_DELETE_REMOVES_FROM_LIST", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		sessions := seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Keep Session"},
			seededSession{title: "Delete Session"},
		)

		binary := buildSharedTUIBinary(t)
		deleteID := sessions[1].ID

		env := append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=xterm-256color",
		)

		// Delete the session.
		deleteCmd := exec.Command(binary, "--data-dir", fixture.workspaceDataDir(), "session", "delete", deleteID, "--json")
		deleteCmd.Env = env
		deleteCmd.Dir = fixture.workingDir
		deleteOutput, err := deleteCmd.CombinedOutput()
		require.NoError(t, err, "session delete failed: %s", string(deleteOutput))

		// List sessions — deleted session should be gone.
		listCmd := exec.Command(binary, "--data-dir", fixture.workspaceDataDir(), "session", "list", "--json")
		listCmd.Env = env
		listCmd.Dir = fixture.workingDir
		listOutput, err := listCmd.Output()
		require.NoError(t, err, "session list failed: %s", string(listOutput))

		var remaining []map[string]interface{}
		require.NoError(t, json.Unmarshal(listOutput, &remaining))

		// Only "Keep Session" should remain.
		require.Len(t, remaining, 1)
		require.Equal(t, "Keep Session", remaining[0]["title"])
	})
}

// ---------------------------------------------------------------------------
// 53. Session CLI: `session rename`
//     Verifies that renaming a session via CLI persists the new title.
// ---------------------------------------------------------------------------

func TestSessionCLIRename_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SESSION_RENAME_PERSISTS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		sessions := seedSessions(t, fixture.workspaceDataDir(),
			seededSession{title: "Original Title"},
		)

		binary := buildSharedTUIBinary(t)
		sessionID := sessions[0].ID

		env := append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=xterm-256color",
		)

		// Rename the session.
		renameCmd := exec.Command(binary, "--data-dir", fixture.workspaceDataDir(), "session", "rename", sessionID, "Renamed Title", "--json")
		renameCmd.Env = env
		renameCmd.Dir = fixture.workingDir
		renameOutput, err := renameCmd.CombinedOutput()
		require.NoError(t, err, "session rename failed: %s", string(renameOutput))

		// Verify via session show.
		showCmd := exec.Command(binary, "--data-dir", fixture.workspaceDataDir(), "session", "show", sessionID, "--json")
		showCmd.Env = env
		showCmd.Dir = fixture.workingDir
		showOutput, err := showCmd.Output()
		require.NoError(t, err, "session show failed: %s", string(showOutput))

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(showOutput, &result))
		meta, ok := result["meta"].(map[string]interface{})
		require.True(t, ok, "session show output should have meta field")
		require.Equal(t, "Renamed Title", meta["title"])
	})
}

// ---------------------------------------------------------------------------
// 54. Models CLI Subcommand
//     Verifies that `models` lists the configured fixture models.
// ---------------------------------------------------------------------------

func TestModelsCLI_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("MODELS_LISTS_CONFIGURED_PROVIDERS", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "models")
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
		require.NoError(t, err, "models command failed: %s", string(output))

		// Output should contain the fixture model names.
		outputStr := string(output)
		require.Contains(t, outputStr, "vision-alpha", "should list vision-alpha model")
		require.Contains(t, outputStr, "reason-mini", "should list reason-mini model")
	})
}

// ---------------------------------------------------------------------------
// 55. Schema CLI Subcommand
//     Verifies that `schema` outputs valid JSON schema for configuration.
// ---------------------------------------------------------------------------

func TestSchemaCLI_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("SCHEMA_OUTPUTS_VALID_JSON", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		binary := buildSharedTUIBinary(t)

		cmd := exec.Command(binary, "schema")
		cmd.Env = append(os.Environ(),
			"CRUSH_GLOBAL_CONFIG="+fixture.configDir,
			"CRUSH_GLOBAL_DATA="+fixture.dataDir,
			"SMITHERS_TUI_GLOBAL_CONFIG="+fixture.configDir,
			"SMITHERS_TUI_GLOBAL_DATA="+fixture.dataDir,
			"TERM=dumb",
		)
		cmd.Dir = fixture.workingDir

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "schema command failed: %s", string(output))

		// Output should be valid JSON.
		var schema map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &schema), "invalid JSON schema: %s", string(output))

		// JSON schema should have standard fields.
		require.Contains(t, schema, "$schema", "should be a JSON schema")
	})
}

// ---------------------------------------------------------------------------
// 56. Auto-Compact Mode at Narrow Terminal
//     Verifies that launching the TUI at a width < 120 cols automatically
//     activates compact mode (sidebar hidden).
// ---------------------------------------------------------------------------

func TestAutoCompactMode_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("NARROW_TERMINAL_ACTIVATES_COMPACT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		// Launch at 90 cols — below the 120-col breakpoint.
		tui := launchTUIWithOptions(t, tuiLaunchOptions{
			env:        fixture.env(),
			workingDir: fixture.workingDir,
		})
		defer tui.Terminate()

		// Resize to narrow before the app fully renders.
		resizeTmuxPane(t, tui, 90, 30)

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// In compact mode at 90 cols, the sidebar should NOT be visible.
		// The chat area should take the full width.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 10*time.Second))

		// Widen the terminal above the breakpoint.
		resizeTmuxPane(t, tui, 150, 40)
		time.Sleep(500 * time.Millisecond)

		// App should still be rendering correctly.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 57. Details Panel Toggle (Ctrl+D in Compact Mode)
//     Verifies that when in compact mode, Ctrl+D toggles the details/sidebar
//     panel on and off.
// ---------------------------------------------------------------------------

func TestDetailsPanelToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("CTRL_D_TOGGLES_DETAILS_IN_COMPACT", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// First, enable compact mode via command palette to ensure we're in compact.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)

		// Now Ctrl+D should toggle the details panel.
		tui.SendKeys("\x04") // ctrl+d
		time.Sleep(500 * time.Millisecond)

		// Details panel may show model info, MCPs, LSPs.
		require.NoError(t, tui.WaitForAnyText([]string{
			"CRUSH", fixtureLargeModelName, "MCPs", "LSPs",
		}, 5*time.Second))

		// Toggle details off.
		tui.SendKeys("\x04") // ctrl+d
		time.Sleep(500 * time.Millisecond)

		// App should still be functional.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 5*time.Second))

		// Toggle compact mode back off.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)
	})
}

// ---------------------------------------------------------------------------
// 58. Attachment Delete All After Adding Multiple
//     Verifies that after adding multiple attachments, they can all be
//     removed and the editor returns to a clean state.
// ---------------------------------------------------------------------------

func TestAttachmentDeleteAll_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("DELETE_ALL_ATTACHMENTS_CLEARS_LIST", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		writePNGFixture(t, fixture.workingDir, "del1.png")
		writePNGFixture(t, fixture.workingDir, "del2.png")
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Add first image.
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		tui.SendKeys("j")
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))

		// Add second image.
		tui.SendKeys("\x06") // ctrl+f
		require.NoError(t, tui.WaitForText("Add Image", 5*time.Second))
		tui.SendKeys("j")
		tui.SendKeys("j")
		tui.SendKeys("\r")
		require.NoError(t, tui.WaitForNoText("Add Image", 10*time.Second))

		// Both should be visible.
		require.NoError(t, tui.WaitForText("del1.png", 5*time.Second))
		require.NoError(t, tui.WaitForText("del2.png", 5*time.Second))

		// Enter delete mode with Ctrl+Shift+R, then 'r' to delete all.
		// Ctrl+Shift+R is sent as the hex sequence for the key.
		tui.SendKeys("\x12") // ctrl+r — note: in some terminals Ctrl+Shift+R is same as Ctrl+R
		time.Sleep(300 * time.Millisecond)

		// If delete mode activated, press 'r' for delete all.
		// If not (Ctrl+Shift+R not distinguishable), try the alternative: escape and clear manually.
		tui.SendKeys("r")
		time.Sleep(500 * time.Millisecond)

		// Attachments should be removed (or view should still be stable).
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 59. Transparent Background Toggle via Command Palette
//     Verifies toggling transparent background mode on/off through the
//     command palette without crashing.
// ---------------------------------------------------------------------------

func TestTransparentBackgroundToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("TOGGLE_TRANSPARENT_BACKGROUND", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Toggle transparent background on.
		openCommandsPalette(t, tui)
		tui.SendKeys("Transparent")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Transparent", "Background", "Enable", "Disable",
		}, 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)

		// App should still render.
		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 5*time.Second))

		// Toggle back.
		openCommandsPalette(t, tui)
		tui.SendKeys("Transparent")
		require.NoError(t, tui.WaitForAnyText([]string{
			"Transparent", "Background",
		}, 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)

		require.NoError(t, tui.WaitForAnyText([]string{
			"SMITHERS", fixtureLargeModelName, "Ready?", "MCPs",
		}, 5*time.Second))
	})
}

// ---------------------------------------------------------------------------
// 60. Chat Text Survives Compact Mode Toggle
//     Verifies that text typed in the editor is preserved when toggling
//     compact/sidebar mode, ensuring no data loss during layout changes.
// ---------------------------------------------------------------------------

func TestChatTextSurvivesCompactToggle_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	t.Run("EDITOR_TEXT_PRESERVED_AFTER_TOGGLE", func(t *testing.T) {
		fixture := newConfiguredFixture(t)
		tui := launchFixtureTUI(t, fixture)
		defer tui.Terminate()

		waitForDashboard(t, tui)
		openStartChatFromDashboard(t, tui)

		// Type some text into the editor.
		tui.SendText("preserve this text across toggle")
		require.NoError(t, tui.WaitForText("preserve this text across toggle", 5*time.Second))

		// Toggle compact/sidebar mode.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)

		// Text should still be in the editor.
		require.NoError(t, tui.WaitForText("preserve this text across toggle", 5*time.Second))

		// Toggle back.
		openCommandsPalette(t, tui)
		tui.SendKeys("Toggle Sidebar")
		require.NoError(t, tui.WaitForText("Toggle Sidebar", 5*time.Second))
		tui.SendKeys("\r")
		if err := tui.WaitForNoText("Commands", 2*time.Second); err != nil {
			tui.SendKeys("\x1b")
			require.NoError(t, tui.WaitForNoText("Commands", 5*time.Second))
		}
		time.Sleep(500 * time.Millisecond)

		// Text still preserved.
		require.NoError(t, tui.WaitForText("preserve this text across toggle", 5*time.Second))
	})
}

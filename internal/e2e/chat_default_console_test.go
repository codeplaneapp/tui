package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestChatDefaultConsole verifies that the Smithers dashboard is the default
// startup view when Smithers config is present.
func TestChatDefaultConsole(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForAnyText([]string{"CRUSH", "SMITHERS"}, 15*time.Second))
	require.NoError(t, tui.WaitForText("New Chat", 10*time.Second))
	require.NoError(t, tui.WaitForText("At a Glance", 10*time.Second))
	require.NoError(t, tui.WaitForText("Run Workflow", 10*time.Second))
}

// TestEscReturnsToChat verifies that Esc from a pushed view returns to chat.
// This test is minimal since it requires mock Smithers agents/data.
// A full test would use VHS to record terminal interactions.
func TestEscReturnsToChat(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	// This test is a placeholder that would require:
	// 1. Mocking Smithers client to return agents
	// 2. Sending key presses to open agents view (Ctrl+P, then /agents)
	// 3. Verifying agents view is displayed
	// 4. Sending Esc key
	// 5. Verifying return to chat console
	//
	// For now, we rely on VHS tape recording for interactive testing:
	// vhs tests/vhs/chat-default-console.tape
	t.Log("E2E Esc-return-to-chat test requires VHS recording (interactive terminal)")
}

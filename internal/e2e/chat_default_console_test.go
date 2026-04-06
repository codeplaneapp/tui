package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestChatDefaultConsole verifies that a Smithers-configured launch skips the
// generic landing view and opens on the Smithers dashboard.
func TestChatDefaultConsole(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") == "" {
		t.Skip("Skipping E2E test: set CRUSH_TUI_E2E=1 to run")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "http://localhost:7331",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("CRUSH_GLOBAL_CONFIG", configDir)
	t.Setenv("CRUSH_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	require.NoError(t, tui.WaitForText("At a Glance", 10*time.Second),
		"smithers dashboard should render at startup; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("Start Chat", 5*time.Second),
		"dashboard quick actions should be visible; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForNoText("Unknown flag", 2*time.Second))
}

// TestEscReturnsToChat verifies that Esc from a pushed view returns to chat.
// This test is minimal since it requires mock Smithers agents/data.
// A full test would use VHS to record terminal interactions.
func TestEscReturnsToChat(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") == "" {
		t.Skip("Skipping E2E test: set CRUSH_TUI_E2E=1 to run")
	}

	// This test is a placeholder that would require:
	// 1. Mocking Smithers client to return agents
	// 2. Sending key presses to open agents view (Ctrl+P, then agents)
	// 3. Verifying agents view is displayed
	// 4. Sending Esc key
	// 5. Verifying return to chat console
	//
	// For now, we rely on VHS tape recording for interactive testing:
	// vhs tests/vhs/chat-default-console.tape
	t.Log("E2E Esc-return-to-chat test requires VHS recording (interactive terminal)")
}

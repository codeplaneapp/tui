package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAgentsView_Navigation exercises the full agents view lifecycle:
//   - Opening the command palette and navigating to the agents view.
//   - Verifying the "SMITHERS › Agents" header and agent groupings are visible.
//   - Moving the cursor with j/k.
//   - Pressing Esc to return to the chat view.
//
// Set SMITHERS_TUI_E2E=1 to run this test (it spawns a real TUI process).
func TestAgentsView_Navigation(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open the command palette with Ctrl+K (or /).
	tui.SendKeys("/")
	require.NoError(t, tui.WaitForText("agents", 5*time.Second))

	// Navigate to the agents view.
	tui.SendKeys("agents\r")
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Agents", 5*time.Second))

	// Agents should be grouped. At least one section header should be visible.
	// The view shows either "Available" or "Not Detected" depending on what's
	// installed on the test machine.
	snap := tui.Snapshot()
	hasAvailable := tui.matchesText("Available")
	hasNotDetected := tui.matchesText("Not Detected")
	_ = snap
	require.True(t, hasAvailable || hasNotDetected,
		"agents view should show at least one group section")

	// Move cursor down then up — should not crash.
	tui.SendKeys("j")
	time.Sleep(100 * time.Millisecond)
	tui.SendKeys("k")
	time.Sleep(100 * time.Millisecond)

	// Refresh.
	tui.SendKeys("r")
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Agents", 5*time.Second))

	// Escape should return to the chat/console view.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Agents", 5*time.Second))
}

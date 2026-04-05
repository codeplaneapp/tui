package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestApprovalsRecentDecisions_TUI exercises the approvals view recent decisions flow:
//   - Opening the command palette and navigating to the approvals view.
//   - Verifying the "SMITHERS › Approvals" header is visible.
//   - Pressing Tab to switch to the recent decisions view.
//   - Verifying the "RECENT DECISIONS" section header appears.
//   - Pressing Tab again to return to the pending queue.
//   - Pressing Esc to leave the view.
//
// Set SMITHERS_TUI_E2E=1 to run this test (it spawns a real TUI process).
func TestApprovalsRecentDecisions_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open the command palette.
	tui.SendKeys("/")
	require.NoError(t, tui.WaitForText("approvals", 5*time.Second))

	// Navigate to the approvals view.
	tui.SendKeys("approvals\r")
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second))

	// The pending queue is displayed first.  The mode hint should mention [Tab] History.
	snap := tui.Snapshot()
	hasPendingMode := tui.matchesText("History") || tui.matchesText("Tab")
	_ = snap
	require.True(t, hasPendingMode, "approvals view should show tab/history hint in pending mode")

	// Press Tab to switch to recent decisions.
	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForText("RECENT DECISIONS", 5*time.Second))

	// The mode hint should now mention Queue.
	require.NoError(t, tui.WaitForText("Queue", 3*time.Second))

	// Navigate down/up in the decisions list (should not crash even if empty).
	tui.SendKeys("j")
	time.Sleep(100 * time.Millisecond)
	tui.SendKeys("k")
	time.Sleep(100 * time.Millisecond)

	// Refresh the decisions list.
	tui.SendKeys("r")
	require.NoError(t, tui.WaitForText("RECENT DECISIONS", 5*time.Second))

	// Press Tab again to return to pending queue.
	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForNoText("RECENT DECISIONS", 3*time.Second))

	// Escape should return to the chat/console view.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

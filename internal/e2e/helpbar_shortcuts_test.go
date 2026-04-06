package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHelpbarShortcuts_TUI(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") != "1" {
		t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("CRUSH_GLOBAL_CONFIG", configDir)
	t.Setenv("CRUSH_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForAnyText([]string{"CRUSH", "SMITHERS"}, 15*time.Second))
	require.NoError(t, tui.WaitForText("enter", 15*time.Second))
	require.NoError(t, tui.WaitForText("select", 15*time.Second))
	require.NoError(t, tui.WaitForText("c", 15*time.Second))
	require.NoError(t, tui.WaitForText("chat", 15*time.Second))
	require.NoError(t, tui.WaitForText("r", 15*time.Second))
	require.NoError(t, tui.WaitForText("refresh", 15*time.Second))
	require.NoError(t, tui.WaitForText("ctrl+g", 15*time.Second))
	require.NoError(t, tui.WaitForText("more", 15*time.Second))

	tui.SendKeys("\x12") // ctrl+r
	require.NoError(t, tui.WaitForAnyText([]string{"CRUSH › Runs", "SMITHERS › Runs"}, 10*time.Second))

	tuiApprovals := launchTUI(t)
	defer tuiApprovals.Terminate()

	require.NoError(t, tuiApprovals.WaitForAnyText([]string{"CRUSH", "SMITHERS"}, 15*time.Second))
	tuiApprovals.SendKeys("\x01") // ctrl+a
	require.NoError(t, tuiApprovals.WaitForAnyText([]string{"CRUSH › Approvals", "SMITHERS › Approvals"}, 10*time.Second))
}

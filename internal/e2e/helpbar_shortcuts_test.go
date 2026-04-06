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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	require.NoError(t, tui.WaitForText("ctrl+r", 15*time.Second))
	require.NoError(t, tui.WaitForText("runs", 15*time.Second))
	require.NoError(t, tui.WaitForText("ctrl+a", 15*time.Second))
	require.NoError(t, tui.WaitForText("approvals", 15*time.Second))

	tui.SendKeys("\x12") // ctrl+r
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Runs", 10*time.Second))

	tui.SendKeys("\x1b") // esc
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Runs", 10*time.Second))

	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 10*time.Second))

	tui.SendKeys("\x1b") // esc
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 10*time.Second))
}

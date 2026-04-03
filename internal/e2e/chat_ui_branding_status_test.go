package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatUIBrandingStatus_TUI(t *testing.T) {
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
	require.NoError(t, tui.WaitForNoText("CRUSH", 5*time.Second))
	require.NoError(t, tui.WaitForNoText("Charm™", 5*time.Second))

	tui.SendKeys("\x03") // ctrl+c
}

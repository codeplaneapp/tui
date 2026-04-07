package e2e_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatUIBrandingStatus_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

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
	require.NoError(t, tui.WaitForNoText("Charm™", 5*time.Second))

	tui.SendKeys("\x03") // ctrl+c
}

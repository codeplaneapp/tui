package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeGlobalConfig(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "smithers-tui.json"), []byte(body), 0o644))
}

func TestSmithersDomainSystemPrompt_TUI(t *testing.T) {
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
	require.NoError(t, tui.WaitForText("Run Dashboard", 10*time.Second))
	require.NoError(t, tui.WaitForText("Workflows", 10*time.Second))
	require.NoError(t, tui.WaitForNoText("Init Smithers", 5*time.Second))
	require.NoError(t, tui.WaitForNoText("Init Crush", 5*time.Second))
}

func TestSmithersDomainSystemPrompt_CoderFallback_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUIWithOptions(t, tuiLaunchOptions{
		workingDir: t.TempDir(),
	})
	defer tui.Terminate()

	require.NoError(t, tui.WaitForAnyText([]string{"CRUSH", "SMITHERS"}, 15*time.Second))
	require.NoError(t, tui.WaitForAnyText([]string{
		"Would you like to initialize this project?",
		"Init Crush",
		"Init Smithers",
	}, 10*time.Second))
	require.NoError(t, tui.WaitForNoText("Run Dashboard", 5*time.Second))
}

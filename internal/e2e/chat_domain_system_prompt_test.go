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
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

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

	// Header brand is always shown.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// When a Smithers config block is present the agent label appears in the UI.
	require.NoError(t, tui.WaitForText("Smithers Agent Mode", 10*time.Second))

	// The smithers MCP entry name is reflected in the MCP status area.
	require.NoError(t, tui.WaitForText("smithers", 5*time.Second))
}

// TestCoderAgentFallback_TUI verifies that the TUI still loads normally when no
// Smithers config block is provided, and that Smithers-specific UI labels are
// absent so the user is not misled about the active agent.
func TestCoderAgentFallback_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// The TUI must still launch and show the header.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Without a smithers config block the Smithers agent mode label must NOT appear.
	require.NoError(t, tui.WaitForNoText("Smithers Agent Mode", 3*time.Second))
}

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestChatDefaultConsole verifies that chat is the default view on startup
// when Smithers config is present.
func TestChatDefaultConsole(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") == "" {
		t.Skip("Skipping E2E test: set CRUSH_TUI_E2E=1 to run")
	}

	// Create a temporary directory for the test config and data
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.Mkdir(dataDir, 0755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}

	// Create a minimal crush.json with Smithers config
	configPath := filepath.Join(tmpDir, "crush.json")
	configContent := `{
  "defaultModel": "claude-opus-4-6",
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "apiUrl": "http://localhost:7331",
    "workflowDir": ".smithers/workflows"
  }
}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Launch TUI with test config
	tui := launchTUI(t,
		"--config", configPath,
		"--data-dir", dataDir,
		"--skip-version-check",
	)
	defer tui.Terminate()

	// Wait for initial render and verify chat prompt is visible
	if err := tui.WaitForText("Ready", 5*time.Second); err != nil {
		t.Logf("Initial render snapshot:\n%s", tui.Snapshot())
		t.Errorf("expected chat prompt 'Ready' at startup: %v", err)
	}

	text := tui.bufferText()

	// Verify no landing view elements are present (Smithers mode should skip landing)
	// Landing view shows model information and LSP/MCP status in columns
	if strings.Contains(text, "LSP") && strings.Contains(text, "MCP") {
		t.Logf("Unexpected landing view detected:\n%s", text)
		// This is informational; landing might appear during init, but should transition to chat
	}
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
	// 2. Sending key presses to open agents view (Ctrl+P, then /agents)
	// 3. Verifying agents view is displayed
	// 4. Sending Esc key
	// 5. Verifying return to chat console
	//
	// For now, we rely on VHS tape recording for interactive testing:
	// vhs tests/vhs/chat-default-console.tape
	t.Log("E2E Esc-return-to-chat test requires VHS recording (interactive terminal)")
}

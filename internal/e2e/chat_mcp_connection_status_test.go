package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestChatMCPConnectionStatus_TUI verifies that the Smithers TUI header shows
// MCP connection status and updates dynamically.
//
// Set SMITHERS_TUI_E2E=1 to run.
func TestChatMCPConnectionStatus_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mockBin := buildMockMCPServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

	// Write a global config that wires the mock MCP binary as the "smithers" MCP.
	cfg := map[string]any{
		"mcp": map[string]any{
			"smithers": map[string]any{
				"type":    "stdio",
				"command": mockBin,
				"args":    []string{},
			},
		},
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	writeGlobalConfig(t, configDir, string(cfgBytes))

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// TUI must show SMITHERS branding.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// After the MCP server connects the header must show "smithers connected".
	// Allow up to 20 s for the MCP handshake + first render cycle.
	require.NoError(t, tui.WaitForText("smithers connected", 20*time.Second),
		"header should show smithers connected after MCP handshake\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03") // ctrl+c
}

// TestChatMCPConnectionStatus_DisconnectedOnStart_TUI verifies that when no
// Smithers MCP is configured the header shows "smithers disconnected".
//
// Set SMITHERS_TUI_E2E=1 to run.
func TestChatMCPConnectionStatus_DisconnectedOnStart_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()

	// Config that configures a smithers MCP pointing at a command that doesn't
	// exist so the MCP reaches StateError / stays disconnected.
	cfg := map[string]any{
		"mcp": map[string]any{
			"smithers": map[string]any{
				"type":    "stdio",
				"command": "/nonexistent/smithers-binary",
				"args":    []string{},
			},
		},
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	writeGlobalConfig(t, configDir, string(cfgBytes))

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Should never show "connected" because the binary is missing.
	// We allow a few seconds for the (failed) MCP startup to complete.
	require.NoError(t, tui.WaitForText("smithers disconnected", 20*time.Second),
		"header should show smithers disconnected when MCP command is missing\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03")
}

// buildMockMCPServer compiles the mock Smithers MCP server binary and returns
// its path.  The binary is placed in a t.TempDir() so it is cleaned up after
// the test completes.
func buildMockMCPServer(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	srcPkg := filepath.Join(repoRoot, "internal", "e2e", "testdata", "mock_smithers_mcp")
	binPath := filepath.Join(t.TempDir(), "mock_smithers_mcp")

	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = srcPkg
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build mock MCP server: %s", string(out))

	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("mock MCP binary not found at %s: %v", binPath, err)
	}
	fmt.Printf("mock MCP server built at %s\n", binPath)
	return binPath
}

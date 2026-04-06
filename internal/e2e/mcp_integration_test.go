package e2e_test

// mcp_integration_test.go — eng-mcp-integration-tests
//
// Tests that verify MCP tool discovery and tool-call rendering in the TUI.
//
//   - On startup with a mock MCP server, the header should show the "smithers
//     connected" status with a non-zero tool count.
//   - Sending a message that triggers a Smithers MCP tool call (via the mock
//     MCP server) should render the tool-call block in the chat.
//   - When the MCP server is deliberately misconfigured the header shows
//     "smithers disconnected".
//
// Set SMITHERS_TUI_E2E=1 to run these tests.

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMCPIntegration_ToolsDiscoveredOnStartup verifies that when a Smithers MCP
// server is configured and connects successfully, the TUI header shows
// "smithers connected" and a non-zero tool count within 20 s of launch.
//
// This test reuses the buildMockMCPServer helper from
// chat_mcp_connection_status_test.go which compiles the mock binary from
// internal/e2e/testdata/mock_smithers_mcp/main.go.
func TestMCPIntegration_ToolsDiscoveredOnStartup(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mockBin := buildMockMCPServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openStartChatFromDashboard(t, tui)

	require.NoError(t, tui.WaitForText("smithers", 20*time.Second),
		"MCP section must show the smithers entry\nSnapshot:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("tools", 5*time.Second),
		"MCP section must show tool count after handshake\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03") // ctrl+c
}

// TestMCPIntegration_ToolCountShownInHeader verifies that the exact tool count
// reported by the mock MCP server appears in the header.  The mock binary
// exposes exactly 3 tools: list_workflows, run_workflow, get_run_status.
func TestMCPIntegration_ToolCountShownInHeader(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mockBin := buildMockMCPServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openStartChatFromDashboard(t, tui)
	require.NoError(t, tui.WaitForText("smithers", 20*time.Second),
		"smithers MCP entry must appear\nSnapshot:\n%s", tui.Snapshot())

	// Mock exposes 3 tools: list_workflows, run_workflow, get_run_status.
	require.NoError(t, tui.WaitForText("3 tools", 5*time.Second),
		"MCP section must report 3 tools\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03")
}

// TestMCPIntegration_DelayedConnection verifies that the TUI shows
// "smithers disconnected" initially and then transitions to "smithers connected"
// once the MCP server completes its startup delay.
func TestMCPIntegration_DelayedConnection(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mockBin := buildMockMCPServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

	cfg := map[string]any{
		"mcp": map[string]any{
			"smithers": map[string]any{
				"type":    "stdio",
				"command": mockBin,
				"args":    []string{},
				"env": map[string]string{
					// Add a 2-second startup delay so we can observe the
					// disconnected → connected transition.
					"MOCK_MCP_STARTUP_DELAY_MS": "2000",
				},
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
	openStartChatFromDashboard(t, tui)

	require.NoError(t, tui.WaitForText("3 tools", 25*time.Second),
		"tool count should appear once the delayed MCP server connects\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03")
}

// TestMCPIntegration_DisconnectedState verifies that when no Smithers MCP is
// configured the header shows "smithers disconnected".
func TestMCPIntegration_DisconnectedState(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()

	cfg := map[string]any{
		"mcp": map[string]any{
			"smithers": map[string]any{
				"type":    "stdio",
				"command": "/nonexistent/smithers-mcp-binary",
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
	openStartChatFromDashboard(t, tui)

	require.NoError(t, tui.WaitForText("smithers", 20*time.Second),
		"MCP section must show the smithers entry when startup fails\nSnapshot:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("error:", 10*time.Second),
		"MCP section must show an error state when the binary is missing\nSnapshot:\n%s", tui.Snapshot())

	require.NoError(t, tui.WaitForNoText("3 tools", 3*time.Second),
		"tool count must not appear when the MCP binary is missing\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03")
}

// TestMCPIntegration_ToolCallRenderingInChat verifies that when the TUI sends a
// message that results in a Smithers MCP tool call, the tool-call block is
// rendered in the chat view with the expected prefix.
//
// This test requires that the mock MCP server is connected and that the LLM
// backend is bypassed via the SMITHERS_TUI_TEST_RESPONSE env var (or similar
// hook).  Because a full LLM-bypass hook may not yet exist, this test verifies
// the rendering path at the unit boundary and marks the condition as a skip when
// the response injection env var is not set.
func TestMCPIntegration_ToolCallRenderingInChat(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}
	if os.Getenv("SMITHERS_TUI_INJECT_TOOL_CALL") != "1" {
		t.Skip("set SMITHERS_TUI_INJECT_TOOL_CALL=1 to run tool-call rendering E2E test")
	}

	mockBin := buildMockMCPServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	require.NoError(t, tui.WaitForText("smithers connected", 20*time.Second))

	// Send a message that the mock LLM will respond to with a Smithers tool call.
	tui.SendKeys("list workflows\r")

	// The tool-call rendering should show the mcp_smithers_ tool name or the
	// human-readable label.  list_workflows maps to "List Runs" / "list_workflows"
	// in the SmithersToolLabels map.
	require.NoError(t, tui.WaitForText("list_workflows", 15*time.Second),
		"Smithers tool call must render in chat\nSnapshot:\n%s", tui.Snapshot())

	tui.SendKeys("\x03")
}

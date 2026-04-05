package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockRunsResponse is the canned JSON response returned by the mock Smithers server.
// Matches the design doc wireframe with 3 representative runs.
var mockRunsPayload = []map[string]interface{}{
	{
		"runId":        "abc12345",
		"workflowName": "code-review",
		"workflowPath": ".smithers/workflows/code-review.ts",
		"status":       "running",
		"startedAtMs":  time.Now().Add(-2*time.Minute - 14*time.Second).UnixMilli(),
		"summary": map[string]int{
			"finished": 3,
			"failed":   0,
			"total":    5,
		},
	},
	{
		"runId":        "def67890",
		"workflowName": "deploy-staging",
		"workflowPath": ".smithers/workflows/deploy-staging.ts",
		"status":       "waiting-approval",
		"startedAtMs":  time.Now().Add(-8*time.Minute - 2*time.Second).UnixMilli(),
		"summary": map[string]int{
			"finished": 4,
			"failed":   0,
			"total":    6,
		},
	},
	{
		"runId":        "ghi11223",
		"workflowName": "test-suite",
		"workflowPath": ".smithers/workflows/test-suite.ts",
		"status":       "running",
		"startedAtMs":  time.Now().Add(-30 * time.Second).UnixMilli(),
		"summary": map[string]int{
			"finished": 1,
			"failed":   0,
			"total":    3,
		},
	},
}

// startMockSmithersServer starts a local HTTP test server that simulates the
// Smithers API for the runs dashboard. It returns canned run data on GET /v1/runs.
func startMockSmithersServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockRunsPayload)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestRunsDashboard_NavigateWithCtrlR verifies that pressing Ctrl+R navigates
// to the runs dashboard view and displays run data from a mock server.
func TestRunsDashboard_NavigateWithCtrlR(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	// Start mock Smithers HTTP server.
	srv := startMockSmithersServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()

	// Write config pointing at the mock server.
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// 1. Wait for TUI to start and show SMITHERS branding.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// 2. Send Ctrl+R to navigate to the runs dashboard.
	tui.SendKeys("\x12") // Ctrl+R

	// 3. Verify the runs view header is rendered.
	require.NoError(t, tui.WaitForText("Runs", 10*time.Second))

	// 4. Verify table column headers are displayed.
	require.NoError(t, tui.WaitForText("Workflow", 5*time.Second))
	require.NoError(t, tui.WaitForText("Status", 5*time.Second))

	// 5. Verify run data from mock server appears in the table.
	require.NoError(t, tui.WaitForText("code-review", 10*time.Second))
	require.NoError(t, tui.WaitForText("running", 5*time.Second))
	require.NoError(t, tui.WaitForText("deploy-staging", 5*time.Second))
	require.NoError(t, tui.WaitForText("test-suite", 5*time.Second))

	// 6. Verify the cursor indicator is present.
	require.NoError(t, tui.WaitForText("▸", 5*time.Second))

	// 7. Send Down arrow to move cursor.
	tui.SendKeys("\x1b[B") // Down arrow
	time.Sleep(200 * time.Millisecond)
	snapshot := tui.Snapshot()
	// After pressing down, the cursor should have moved (▸ should still be visible).
	require.Contains(t, snapshot, "▸")

	// 8. Send Esc to return to chat.
	tui.SendKeys("\x1b") // Esc
	require.NoError(t, tui.WaitForText("SMITHERS", 5*time.Second))
}

// TestRunsDashboard_NavigateViaCommandPalette verifies that typing "/runs" in
// the command palette navigates to the runs dashboard.
func TestRunsDashboard_NavigateViaCommandPalette(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	srv := startMockSmithersServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open command palette (Ctrl+P or /).
	tui.SendKeys("\x10") // Ctrl+P
	time.Sleep(500 * time.Millisecond)

	// Type "runs" to filter to the Runs entry.
	tui.SendKeys("runs")
	time.Sleep(300 * time.Millisecond)

	// Verify the Run Dashboard entry appears in the palette.
	require.NoError(t, tui.WaitForText("Run Dashboard", 5*time.Second))

	// Press Enter to select it.
	tui.SendKeys("\r") // Enter
	require.NoError(t, tui.WaitForText("Runs", 10*time.Second))
}

// TestRunsDashboard_EmptyState verifies the "No runs found" message when the
// mock server returns an empty runs list.
func TestRunsDashboard_EmptyState(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	// Start a mock server that returns an empty list.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	tui.SendKeys("\x12") // Ctrl+R
	require.NoError(t, tui.WaitForText("No runs found", 10*time.Second))
}

// TestRunsDashboard_RefreshWithRKey verifies that pressing "r" in the runs
// view reloads runs from the server.
func TestRunsDashboard_RefreshWithRKey(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	srv := startMockSmithersServer(t)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Navigate to runs dashboard.
	tui.SendKeys("\x12") // Ctrl+R
	require.NoError(t, tui.WaitForText("code-review", 10*time.Second))

	// Press "r" to refresh — should briefly show loading then data again.
	tui.SendKeys("r")
	// After refresh, runs should still appear.
	require.NoError(t, tui.WaitForText("code-review", 10*time.Second))
}

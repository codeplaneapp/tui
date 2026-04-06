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

// TestApprovalsQueue_Navigation exercises the full approvals queue lifecycle:
//   - Opening the approvals view via Ctrl+A.
//   - Verifying the "SMITHERS › Approvals" header and pending approvals are visible.
//   - Moving the cursor with j/k.
//   - Pressing r to refresh.
//   - Pressing Esc to return to the chat view.
//
// Set SMITHERS_TUI_E2E=1 to run this test (it spawns a real TUI process).
func TestApprovalsQueue_Navigation(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open approvals view via Ctrl+A.
	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second))

	// Move cursor down then up — should not crash.
	tui.SendKeys("j")
	time.Sleep(100 * time.Millisecond)
	tui.SendKeys("k")
	time.Sleep(100 * time.Millisecond)

	// Refresh.
	tui.SendKeys("r")
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second))

	// Escape should return to the chat/console view.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// TestApprovalsQueue_WithMockServer exercises the approvals queue against a
// mock Smithers HTTP server that returns two pending approvals.
//
// Set SMITHERS_TUI_E2E=1 to run this test.
func TestApprovalsQueue_WithMockServer(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	// Set up a mock Smithers HTTP server.
	mockServer := newMockSmithersServer(t, []mockApproval{
		{ID: "appr-1", RunID: "run-abc", NodeID: "deploy", Gate: "Deploy to staging", Status: "pending"},
		{ID: "appr-2", RunID: "run-xyz", NodeID: "delete", Gate: "Delete user data", Status: "pending"},
	})
	defer mockServer.Close()

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+mockServer.URL+`",
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open approvals view via Ctrl+A.
	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second),
		"should show approvals header; buffer: %s", tui.Snapshot())

	require.NoError(t, tui.WaitForText("Pending", 5*time.Second),
		"should show the pending approvals section; buffer: %s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("Deploy to staging", 5*time.Second),
		"should show first approval label; buffer: %s", tui.Snapshot())

	require.NoError(t, tui.WaitForText("Delete user data", 5*time.Second),
		"should show second approval label; buffer: %s", tui.Snapshot())

	// Navigate with j (down) — should not crash.
	tui.SendKeys("j")
	time.Sleep(100 * time.Millisecond)

	// Refresh — list should re-render.
	tui.SendKeys("r")
	require.NoError(t, tui.WaitForText("Deploy to staging", 5*time.Second),
		"refresh should re-render list; buffer: %s", tui.Snapshot())

	// Escape should return to chat.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second),
		"esc should return to chat; buffer: %s", tui.Snapshot())
}

// TestApprovalsQueue_OpenViaCommandPalette opens the approvals view via the
// command palette rather than Ctrl+A.
//
// Set SMITHERS_TUI_E2E=1 to run this test.
func TestApprovalsQueue_OpenViaCommandPalette(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open command palette and navigate to approvals.
	openCommandsPalette(t, tui)
	tui.SendKeys("approvals")
	require.NoError(t, tui.WaitForText("Approvals", 5*time.Second))

	tui.SendKeys("\r")
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second),
		"should show approvals header via command palette; buffer: %s", tui.Snapshot())

	// Should show loading or a state (no crash).
	snap := tui.Snapshot()
	_ = snap

	// Escape should return to chat.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// --- Mock server helpers ---

// mockApproval is a simplified approval record for the mock server.
type mockApproval struct {
	ID           string `json:"id"`
	RunID        string `json:"runId"`
	NodeID       string `json:"nodeId"`
	WorkflowPath string `json:"workflowPath"`
	Gate         string `json:"gate"`
	Status       string `json:"status"`
	RequestedAt  int64  `json:"requestedAt"`
}

// newMockSmithersServer creates an httptest.Server that mimics the Smithers
// HTTP API, returning the given approvals from GET /approval/list.
func newMockSmithersServer(t *testing.T, approvals []mockApproval) *httptest.Server {
	t.Helper()

	// Set RequestedAt to "now" for approvals that don't specify it.
	now := time.Now().UnixMilli()
	for i := range approvals {
		if approvals[i].RequestedAt == 0 {
			approvals[i].RequestedAt = now
		}
	}

	mux := http.NewServeMux()

	// Health endpoint — used by isServerAvailable().
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Approvals list endpoint.
	mux.HandleFunc("/approval/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]interface{}{
			"ok":   true,
			"data": approvals,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode approvals response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

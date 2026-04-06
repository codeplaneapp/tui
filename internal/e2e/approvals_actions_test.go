package e2e_test

// approvals_actions_test.go — eng-approvals-e2e-tests
//
// Tests the approve / deny actions in the approvals queue and verifies that
// acting on a pending approval removes it from the list.  Also covers the Tab
// toggle between the pending queue and the recent decisions view using a mock
// Smithers HTTP server.
//
// Set SMITHERS_TUI_E2E=1 to run these tests.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestApprovalsApproveAction_RemovesItemFromQueue launches the TUI against a
// mock server with two pending approvals, opens the approvals view, and presses
// 'a' to approve the first item.  The approved item should disappear from the
// list, leaving only the second approval visible.
func TestApprovalsApproveAction_RemovesItemFromQueue(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	var (
		mu        sync.Mutex
		approvals = []mockApproval{
			{ID: "appr-1", RunID: "run-abc", NodeID: "deploy", Gate: "Deploy to staging", Status: "pending"},
			{ID: "appr-2", RunID: "run-xyz", NodeID: "notify", Gate: "Send notification", Status: "pending"},
		}
		approvedIDs []string
	)

	mux := http.NewServeMux()

	// Health endpoint.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Approvals list — returns current state of the slice.
	mux.HandleFunc("/approval/list", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		snapshot := make([]mockApproval, len(approvals))
		copy(snapshot, approvals)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": snapshot})
	})

	// Approve endpoint — POST /v1/runs/:runID/nodes/:nodeID/approve
	mux.HandleFunc("/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		// Extract the approvalID from the path and mark it as approved.
		// Path shape: /v1/runs/<runID>/nodes/<nodeID>/approve|deny
		parts := splitPath(r.URL.Path)
		if len(parts) < 5 {
			http.NotFound(w, r)
			return
		}
		action := parts[len(parts)-1] // "approve" or "deny"
		runID := parts[2]

		mu.Lock()
		for i, a := range approvals {
			if a.RunID == runID {
				if action == "approve" {
					approvedIDs = append(approvedIDs, a.ID)
					approvals = append(approvals[:i], approvals[i+1:]...)
				} else if action == "deny" {
					approvals = append(approvals[:i], approvals[i+1:]...)
				}
				break
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	// Wait for the TUI to start.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Open approvals view via Ctrl+A.
	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second),
		"approvals header must appear; buffer:\n%s", tui.Snapshot())

	// Both pending approvals should be visible.
	require.NoError(t, tui.WaitForText("Deploy to staging", 5*time.Second),
		"first approval must be visible; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("Send notification", 5*time.Second),
		"second approval must be visible; buffer:\n%s", tui.Snapshot())

	// Approve the first item with 'a'.
	tui.SendKeys("a")

	// After approval the first item should disappear; second should remain.
	require.NoError(t, tui.WaitForNoText("Deploy to staging", 8*time.Second),
		"approved item must be removed from queue; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("Send notification", 5*time.Second),
		"remaining approval must still be visible; buffer:\n%s", tui.Snapshot())

	// Escape returns to chat.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// TestApprovalsDenyAction_RemovesItemFromQueue launches the TUI against a mock
// server with one pending approval and verifies that pressing 'd' to deny
// removes the item and shows the empty-queue message.
func TestApprovalsDenyAction_RemovesItemFromQueue(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	var (
		mu      sync.Mutex
		pending = true
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/approval/list", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isPending := pending
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		var data []mockApproval
		if isPending {
			data = []mockApproval{
				{ID: "appr-deny-1", RunID: "run-deny", NodeID: "rm-data", Gate: "Delete user records", Status: "pending"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": data})
	})

	// Deny endpoint.
	mux.HandleFunc("/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			pending = false
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second),
		"approvals header; buffer:\n%s", tui.Snapshot())

	require.NoError(t, tui.WaitForText("Delete user records", 5*time.Second),
		"pending approval must render; buffer:\n%s", tui.Snapshot())

	// Deny with 'd'.
	tui.SendKeys("d")

	// After denial the item must disappear and the empty-queue message should show.
	require.NoError(t, tui.WaitForNoText("Delete user records", 8*time.Second),
		"denied item must be removed; buffer:\n%s", tui.Snapshot())

	// Empty state message should appear.
	require.NoError(t, tui.WaitForText("No pending approvals", 5*time.Second),
		"empty queue state must show after denial; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// TestApprovalsTabToggle_QueueToRecentAndBack verifies the full Tab-toggle
// lifecycle: pending queue → recent decisions → back to pending.
// The mock server provides both pending approvals and recent decisions.
func TestApprovalsTabToggle_QueueToRecentAndBack(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	now := time.Now().UnixMilli()
	recentDecision := map[string]interface{}{
		"id":          "dec-1",
		"runId":       "run-rec",
		"nodeId":      "build",
		"gate":        "Build artifact",
		"decision":    "approved",
		"decidedAt":   now - 60000,
		"requestedAt": now - 120000,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/approval/list", func(w http.ResponseWriter, r *http.Request) {
		data := []mockApproval{
			{ID: "appr-tab-1", RunID: "run-tab", NodeID: "test", Gate: "Run test suite", Status: "pending"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": data})
	})

	mux.HandleFunc("/approval/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": []interface{}{recentDecision}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second))

	// Pending queue should be shown first.
	require.NoError(t, tui.WaitForText("Run test suite", 5*time.Second),
		"pending approval must render initially; buffer:\n%s", tui.Snapshot())

	// Tab should switch to recent decisions.
	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForText("RECENT DECISIONS", 5*time.Second),
		"Tab must switch to recent decisions; buffer:\n%s", tui.Snapshot())

	// The mode hint should advertise the pending queue as the way back.
	require.NoError(t, tui.WaitForText("Pending", 3*time.Second),
		"mode hint should mention Pending; buffer:\n%s", tui.Snapshot())

	// Navigate in recent decisions (should not crash even if list is short).
	tui.SendKeys("j")
	time.Sleep(100 * time.Millisecond)
	tui.SendKeys("k")
	time.Sleep(100 * time.Millisecond)

	// Refresh recent decisions.
	tui.SendKeys("r")
	require.NoError(t, tui.WaitForText("RECENT DECISIONS", 5*time.Second),
		"refresh must keep recent decisions view; buffer:\n%s", tui.Snapshot())

	// Tab again → back to pending queue.
	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForNoText("RECENT DECISIONS", 3*time.Second),
		"second Tab must return to pending queue; buffer:\n%s", tui.Snapshot())

	// Escape to chat.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// TestApprovalsQueue_EmptyState verifies the empty-queue message when the mock
// server returns no pending approvals.
func TestApprovalsQueue_EmptyState(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/approval/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": []mockApproval{}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	tui.SendKeys("\x01") // ctrl+a
	require.NoError(t, tui.WaitForText("SMITHERS \u203a Approvals", 5*time.Second))

	require.NoError(t, tui.WaitForText("No pending approvals", 5*time.Second),
		"empty state message must appear; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS \u203a Approvals", 5*time.Second))
}

// splitPath splits a URL path on "/" and returns non-empty segments.
func splitPath(p string) []string {
	var parts []string
	for _, s := range splitSlash(p) {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

// splitSlash splits s on "/" without importing strings in the test file.
func splitSlash(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return result
}

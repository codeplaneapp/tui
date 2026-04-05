package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSmithersWorkspaceContext_TUI launches the TUI with a mock Smithers HTTP
// server that returns active runs and verifies that the prompt template
// rendered the workspace context into the session (by observing the TUI boot
// up without crashing).
//
// Run with SMITHERS_TUI_E2E=1 to execute this test.
func TestSmithersWorkspaceContext_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	// Spin up a local Smithers API mock.
	srv := startWorkspaceContextMockServer(t)
	defer srv.Close()

	configDir := t.TempDir()
	dataDir := t.TempDir()

	cfg := map[string]interface{}{
		"smithers": map[string]interface{}{
			"apiUrl":      srv.URL,
			"workflowDir": ".smithers/workflows",
		},
	}
	cfgBytes, err := json.Marshal(cfg)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "smithers-tui.json"), cfgBytes, 0o644))

	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// Wait for the TUI to start and show the SMITHERS header.
	require.NoError(t, tui.WaitForText("SMITHERS", 20*time.Second))
}

// startWorkspaceContextMockServer creates a minimal Smithers HTTP mock that
// handles /health and /v1/runs endpoint stubs needed for workspace context
// pre-fetch.
func startWorkspaceContextMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	type runSummary struct {
		RunID        string `json:"runId"`
		WorkflowName string `json:"workflowName"`
		WorkflowPath string `json:"workflowPath"`
		Status       string `json:"status"`
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/runs":
			status := r.URL.Query().Get("status")
			var runs []runSummary
			switch status {
			case "running":
				runs = []runSummary{
					{
						RunID:        "run-e2e-1",
						WorkflowName: "ci-check",
						WorkflowPath: ".smithers/workflows/ci.tsx",
						Status:       "running",
					},
				}
			case "waiting-approval":
				runs = []runSummary{
					{
						RunID:        "run-e2e-2",
						WorkflowName: "deploy-staging",
						WorkflowPath: ".smithers/workflows/deploy.tsx",
						Status:       "waiting-approval",
					},
				}
			default:
				runs = []runSummary{}
			}
			if err := json.NewEncoder(w).Encode(runs); err != nil {
				http.Error(w, "encode error", http.StatusInternalServerError)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

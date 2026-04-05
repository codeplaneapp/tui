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

func TestChatActiveRunSummary_TUI(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	type run struct {
		RunID        string `json:"runId"`
		WorkflowName string `json:"workflowName"`
		Status       string `json:"status"`
	}

	// Serve a minimal Smithers API that returns 2 active runs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/runs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]run{
				{RunID: "r1", WorkflowName: "code-review", Status: "running"},
				{RunID: "r2", WorkflowName: "deploy", Status: "running"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	configDir := t.TempDir()
	dataDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`"
  }
}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	// Header branding must appear first.
	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

	// Active run count must appear within two poll cycles (≤ 25 s).
	// The startup fetch fires before the 10-second tick, so the count
	// should appear within a few seconds of launch.
	require.NoError(t, tui.WaitForText("2 active", 25*time.Second))

	tui.SendKeys("\x03")
}

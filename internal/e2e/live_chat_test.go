package e2e_test

// live_chat_test.go — eng-live-chat-e2e-testing
//
// Tests the Live Chat Viewer view: opening the view via the command palette,
// verifying that messages stream in from a mock SSE server, that follow mode
// works, and that attempt navigation keys are rendered.
//
// Set SMITHERS_TUI_E2E=1 to run these tests.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockChatBlock is a simplified chat block shape for JSON encoding in the mock
// SSE server.
type mockChatBlock struct {
	ID          string `json:"id,omitempty"`
	RunID       string `json:"runId"`
	NodeID      string `json:"nodeId,omitempty"`
	Attempt     int    `json:"attempt"`
	Role        string `json:"role"`
	Content     string `json:"content"`
	TimestampMs int64  `json:"timestampMs"`
}

// newMockLiveChatServer creates a test HTTP server that provides:
//   - GET /health                               — 200 OK
//   - GET /v1/runs/:id                          — returns minimal run metadata
//   - GET /v1/runs/:id/chat                     — returns snapshot blocks
//   - GET /v1/runs/:id/chat/stream              — sends blocks over SSE then closes
//   - GET /v1/runs                              — returns the run in the list
func newMockLiveChatServer(t *testing.T, runID string, blocks []mockChatBlock) *httptest.Server {
	t.Helper()

	now := time.Now().UnixMilli()
	for i := range blocks {
		if blocks[i].RunID == "" {
			blocks[i].RunID = runID
		}
		if blocks[i].TimestampMs == 0 {
			blocks[i].TimestampMs = now + int64(i*1000)
		}
	}

	mux := http.NewServeMux()

	// Health.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Runs list (for the runs dashboard).
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"runId":        runID,
				"workflowName": "e2e-test-workflow",
				"status":       "running",
				"startedAtMs":  now - 30000,
				"summary":      map[string]int{"finished": 1, "failed": 0, "total": 2},
			},
		})
	})

	// Single run metadata.
	mux.HandleFunc("/v1/runs/"+runID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"runId":        runID,
			"workflowName": "e2e-test-workflow",
			"status":       "running",
			"startedAtMs":  now - 30000,
		})
	})

	// Chat snapshot — returns all blocks at once.
	mux.HandleFunc("/v1/runs/"+runID+"/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(blocks)
	})

	// Chat SSE stream — sends each block as an SSE event then closes.
	mux.HandleFunc("/v1/runs/"+runID+"/chat/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		for _, block := range blocks {
			data, err := json.Marshal(block)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
		// Heartbeat then close.
		fmt.Fprintf(w, ": done\n\n")
		flusher.Flush()
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeFakeSmithersCLI(t *testing.T, run map[string]any, blocks []mockChatBlock) string {
	t.Helper()

	runJSON, err := json.Marshal(run)
	require.NoError(t, err)

	blocksJSON, err := json.Marshal(blocks)
	require.NoError(t, err)

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "smithers")
	script := fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
  "run get")
    cat <<'EOF'
%s
EOF
    ;;
  "run chat")
    cat <<'EOF'
%s
EOF
    ;;
  *)
    printf 'unsupported smithers invocation: %%s\n' "$*" >&2
    exit 1
    ;;
esac
`, string(runJSON), string(blocksJSON))
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return binDir
}

// openLiveChatViaCommandPalette navigates to the Live Chat view via Ctrl+P.
// Returns after the "SMITHERS › Chat" header is visible.
func openLiveChatViaCommandPalette(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	openCommandsPalette(t, tui)
	tui.SendKeys("live")
	require.NoError(t, tui.WaitForText("Live Chat", 5*time.Second),
		"command palette must show Live Chat entry; buffer:\n%s", tui.Snapshot())
	tui.SendKeys("\r")
	require.NoError(t, tui.WaitForText("SMITHERS > Chat >", 5*time.Second),
		"live chat header must appear; buffer:\n%s", tui.Snapshot())
}

// TestLiveChat_OpenViaCommandPaletteAndRender verifies that the live chat view
// can be opened from the command palette, that the header "SMITHERS › Chat"
// appears, and that the help bar shows the expected bindings.
func TestLiveChat_OpenViaCommandPaletteAndRender(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	const runID = "livechat-e2e-run"
	blocks := []mockChatBlock{
		{RunID: runID, NodeID: "task1", Attempt: 0, Role: "user", Content: "Please deploy the service"},
		{RunID: runID, NodeID: "task1", Attempt: 0, Role: "assistant", Content: "Starting deployment sequence"},
		{RunID: runID, NodeID: "task1", Attempt: 0, Role: "tool", Content: "deploy_service({env:staging})"},
	}

	srv := newMockLiveChatServer(t, runID, blocks)

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

	openLiveChatViaCommandPalette(t, tui)

	// The help bar must show the follow binding.
	require.NoError(t, tui.WaitForText("follow", 3*time.Second),
		"help bar must show follow binding; buffer:\n%s", tui.Snapshot())

	// The help bar must show the hijack binding.
	require.NoError(t, tui.WaitForText("hijack", 3*time.Second),
		"help bar must show hijack binding; buffer:\n%s", tui.Snapshot())

	// Escape returns to the previous view.
	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second),
		"Esc must pop the live chat view; buffer:\n%s", tui.Snapshot())
}

// TestLiveChat_MessagesStreamIn verifies that when the TUI opens the live chat
// view for a run that has messages, those messages appear in the viewport.
func TestLiveChat_MessagesStreamIn(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	const runID = "stream-in-run"
	blocks := []mockChatBlock{
		{RunID: runID, NodeID: "n1", Attempt: 0, Role: "user", Content: "Hello from E2E test"},
		{RunID: runID, NodeID: "n1", Attempt: 0, Role: "assistant", Content: "E2E response received"},
	}
	fakeBin := writeFakeSmithersCLI(t, map[string]any{
		"runId":        runID,
		"workflowName": "e2e-test-workflow",
		"status":       "running",
	}, blocks)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUIWithOptions(t, tuiLaunchOptions{
		pathPrefixes: []string{fakeBin},
		workingDir:   projectDir,
	})
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openLiveChatViaCommandPalette(t, tui)

	// Wait for the message content to appear.
	require.NoError(t, tui.WaitForText("Hello from E2E test", 10*time.Second),
		"user message must render; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("E2E response received", 10*time.Second),
		"assistant message must render; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second))
}

// TestLiveChat_FollowModeToggle verifies that pressing 'f' toggles follow mode
// in the help bar between "follow: on" and "follow: off".
func TestLiveChat_FollowModeToggle(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	const runID = "follow-mode-run"
	blocks := []mockChatBlock{
		{RunID: runID, NodeID: "n1", Attempt: 0, Role: "assistant", Content: "Agent is working..."},
	}

	srv := newMockLiveChatServer(t, runID, blocks)

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
	openLiveChatViaCommandPalette(t, tui)

	// Follow mode should be ON by default.
	require.NoError(t, tui.WaitForText("follow: on", 5*time.Second),
		"follow mode must default to on; buffer:\n%s", tui.Snapshot())

	// Press 'f' — follow mode should turn off.
	tui.SendKeys("f")
	require.NoError(t, tui.WaitForText("follow: off", 3*time.Second),
		"follow mode must turn off after 'f'; buffer:\n%s", tui.Snapshot())

	// Press 'f' again — follow mode should turn on.
	tui.SendKeys("f")
	require.NoError(t, tui.WaitForText("follow: on", 3*time.Second),
		"follow mode must turn on after second 'f'; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second))
}

// TestLiveChat_UpArrowDisablesFollowMode verifies that pressing the Up arrow
// while follow mode is on disables it (the user is manually scrolling).
func TestLiveChat_UpArrowDisablesFollowMode(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]interface{}{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openLiveChatViaCommandPalette(t, tui)

	require.NoError(t, tui.WaitForText("follow: on", 5*time.Second),
		"follow mode must default to on; buffer:\n%s", tui.Snapshot())

	// Pressing Up should disable follow.
	tui.SendKeys("\x1b[A") // ANSI Up arrow
	require.NoError(t, tui.WaitForText("follow: off", 3*time.Second),
		"Up arrow must disable follow mode; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
}

// TestLiveChat_AttemptNavigation verifies that when multiple attempts exist,
// the '[' and ']' attempt navigation hints appear in the help bar and navigate
// between attempts.
func TestLiveChat_AttemptNavigation(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	const runID = "attempt-nav-run"
	// Two attempts for the same run.
	blocks := []mockChatBlock{
		{RunID: runID, NodeID: "n1", Attempt: 0, Role: "assistant", Content: "First attempt output"},
		{RunID: runID, NodeID: "n1", Attempt: 1, Role: "assistant", Content: "Second attempt output"},
	}
	fakeBin := writeFakeSmithersCLI(t, map[string]any{
		"runId":        runID,
		"workflowName": "e2e-test-workflow",
		"status":       "running",
	}, blocks)

	configDir := t.TempDir()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeGlobalConfig(t, configDir, `{
  "smithers": {
    "dbPath": ".smithers/smithers.db",
    "workflowDir": ".smithers/workflows"
  }
}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUIWithOptions(t, tuiLaunchOptions{
		pathPrefixes: []string{fakeBin},
		workingDir:   projectDir,
	})
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openLiveChatViaCommandPalette(t, tui)

	// Wait for blocks to load — the latest (attempt 1) is shown by default.
	require.NoError(t, tui.WaitForText("Second attempt output", 10*time.Second),
		"latest attempt content must render; buffer:\n%s", tui.Snapshot())

	// With multiple attempts, the attempt nav hint must appear.
	require.NoError(t, tui.WaitForText("attempt", 5*time.Second),
		"attempt navigation hint must appear; buffer:\n%s", tui.Snapshot())

	// Also verify the sub-header shows the attempt indicator.
	require.NoError(t, tui.WaitForText("Attempt", 3*time.Second),
		"sub-header must show attempt indicator; buffer:\n%s", tui.Snapshot())

	// Navigate to previous attempt with '['.
	tui.SendKeys("[")
	require.NoError(t, tui.WaitForText("First attempt output", 5*time.Second),
		"'[' must navigate to previous attempt; buffer:\n%s", tui.Snapshot())

	// Navigate back to latest attempt with ']'.
	tui.SendKeys("]")
	require.NoError(t, tui.WaitForText("Second attempt output", 5*time.Second),
		"']' must navigate to next attempt; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("q")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second))
}

// TestLiveChat_QKeyPopsView verifies that pressing 'q' pops the live chat view,
// same as Esc.
func TestLiveChat_QKeyPopsView(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]interface{}{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openLiveChatViaCommandPalette(t, tui)

	// Press 'q' — same effect as Esc.
	tui.SendKeys("q")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second),
		"'q' must pop the live chat view; buffer:\n%s", tui.Snapshot())
}

// TestLiveChat_NoServerFallback verifies that when no Smithers server is
// configured, the live chat view still opens and shows an error/empty state
// rather than crashing.
func TestLiveChat_NoServerFallback(t *testing.T) {
	if os.Getenv("SMITHERS_TUI_E2E") != "1" {
		t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
	}

	configDir := t.TempDir()
	dataDir := t.TempDir()
	// Config with no apiUrl so the client has no server to reach.
	writeGlobalConfig(t, configDir, `{}`)
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

	tui := launchTUI(t)
	defer tui.Terminate()

	require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))
	openLiveChatViaCommandPalette(t, tui)

	require.NoError(t, tui.WaitForText("Error loading run", 8*time.Second),
		"live chat must show an error state instead of crashing\nBuffer:\n%s", tui.Snapshot())

	tui.SendKeys("\x1b")
	require.NoError(t, tui.WaitForNoText("SMITHERS > Chat >", 5*time.Second))
}

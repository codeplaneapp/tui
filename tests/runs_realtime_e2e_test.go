// Package tests contains end-to-end integration tests for the Smithers TUI.
//
// runs_realtime_e2e_test.go tests the real-time SSE streaming integration in
// RunsView using a mock httptest.Server.  These tests verify the full data-flow
// without starting a real Smithers server.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/views"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================
// Mock server
// =============================================================

// mockSSEServer is a controllable httptest.Server that serves:
//   - GET /health        → 200
//   - GET /v1/runs       → JSON array of the current run slice
//   - GET /v1/events     → long-lived SSE stream driven by eventPush
type mockSSEServer struct {
	srv           *httptest.Server
	eventPush     chan string
	mu            sync.Mutex
	runs          []smithers.RunSummary
	sseConnected  chan struct{}
	sseDone       chan struct{}
	listRunsHits  atomic.Int64
	disableEvents bool // return 404 for /v1/events when true
}

func newMockSSEServer(t *testing.T, initial []smithers.RunSummary) *mockSSEServer {
	t.Helper()
	ms := &mockSSEServer{
		eventPush:    make(chan string, 16),
		runs:         initial,
		sseConnected: make(chan struct{}, 4),
		sseDone:      make(chan struct{}, 4),
	}
	ms.srv = httptest.NewServer(http.HandlerFunc(ms.handle))
	t.Cleanup(ms.srv.Close)
	return ms
}

func (ms *mockSSEServer) handle(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		w.WriteHeader(http.StatusOK)

	case "/v1/runs":
		ms.listRunsHits.Add(1)
		ms.mu.Lock()
		data, _ := json.Marshal(ms.runs)
		ms.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)

	case "/v1/events":
		if ms.disableEvents {
			http.NotFound(w, r)
			return
		}
		ms.handleSSE(w, r)

	default:
		http.NotFound(w, r)
	}
}

func (ms *mockSSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	f.Flush()

	select {
	case ms.sseConnected <- struct{}{}:
	default:
	}

	for {
		select {
		case frame, open := <-ms.eventPush:
			if !open {
				return
			}
			_, _ = fmt.Fprint(w, frame)
			f.Flush()
		case <-r.Context().Done():
			select {
			case ms.sseDone <- struct{}{}:
			default:
			}
			return
		}
	}
}

func (ms *mockSSEServer) pushEvent(eventType, runID, status string, seq int) {
	payload, _ := json.Marshal(smithers.RunEvent{
		Type:        eventType,
		RunID:       runID,
		Status:      status,
		TimestampMs: time.Now().UnixMilli(),
	})
	ms.eventPush <- fmt.Sprintf("event: smithers\ndata: %s\nid: %d\n\n", payload, seq)
}

func (ms *mockSSEServer) waitConnected(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-ms.sseConnected:
	case <-time.After(timeout):
		t.Fatal("timeout waiting for SSE connection to open")
	}
}

func (ms *mockSSEServer) waitDisconnected(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-ms.sseDone:
	case <-time.After(timeout):
		t.Fatal("timeout waiting for SSE connection to drop")
	}
}

func (ms *mockSSEServer) newClient() *smithers.Client {
	c := smithers.NewClient(
		smithers.WithAPIURL(ms.srv.URL),
		smithers.WithHTTPClient(ms.srv.Client()),
	)
	c.SetServerUp(true)
	return c
}

// =============================================================
// Drive helpers
// =============================================================

// execBatch runs cmd (possibly a tea.Batch) and collects resulting messages.
// Child cmds that block longer than perCmdTimeout are skipped so that slow
// SSE-blocking cmds do not stall the test.
func execBatch(cmd tea.Cmd, perCmdTimeout time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { done <- c() }(c)
		select {
		case m := <-done:
			out = append(out, m)
		case <-time.After(perCmdTimeout):
			// Blocking cmd (e.g. WaitForAllEvents with no events yet) — skip.
		}
	}
	return out
}

// driveView applies a slice of messages to v, collecting any cmds returned.
func driveView(v *views.RunsView, msgs []tea.Msg) (*views.RunsView, []tea.Cmd) {
	var cmds []tea.Cmd
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		updated, cmd := v.Update(msg)
		v = updated.(*views.RunsView)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return v, cmds
}

// runCmdsWithTimeout executes a slice of cmds concurrently and returns all
// messages that arrive within timeout.
func runCmdsWithTimeout(cmds []tea.Cmd, timeout time.Duration) []tea.Msg {
	if len(cmds) == 0 {
		return nil
	}
	ch := make(chan tea.Msg, len(cmds)*2)
	deadline := time.After(timeout)
	for _, cmd := range cmds {
		c := cmd
		go func() {
			select {
			case ch <- c():
			case <-deadline:
			}
		}()
	}
	var out []tea.Msg
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for range cmds {
		select {
		case msg := <-ch:
			out = append(out, msg)
		case <-timer.C:
			return out
		}
	}
	return out
}

// cancelViewAtEnd presses Esc on v at test cleanup time so the view context is
// cancelled before the httptest.Server tries to shut down.  This prevents the
// server from blocking in Close() waiting for open SSE connections.
func cancelViewAtEnd(t *testing.T, v *views.RunsView) {
	t.Helper()
	t.Cleanup(func() {
		if ctx := v.Ctx(); ctx != nil && ctx.Err() == nil {
			v.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) //nolint:errcheck
		}
		// Give the SSE goroutine a moment to see the cancellation.
		time.Sleep(50 * time.Millisecond)
	})
}

// =============================================================
// TestRunsRealtimeSSE
// =============================================================

// TestRunsRealtimeSSE verifies the full SSE round-trip:
//  1. RunsView subscribes to /v1/events on Init.
//  2. A RunStatusChanged event updates the run status in-place without
//     any user keypress.
//  3. The "● Live" indicator is visible in the rendered header.
func TestRunsRealtimeSSE(t *testing.T) {
	initial := []smithers.RunSummary{
		{RunID: "abc123", WorkflowName: "wf-alpha", Status: smithers.RunStatusRunning},
		{RunID: "def456", WorkflowName: "wf-beta", Status: smithers.RunStatusRunning},
	}
	ms := newMockSSEServer(t, initial)
	client := ms.newClient()

	v := views.NewRunsView(client)
	cancelViewAtEnd(t, v) // ensures context is cancelled before server.Close()

	// Init: loads runs and starts SSE stream.
	initMsgs := execBatch(v.Init(), 3*time.Second)
	v, pendingCmds := driveView(v, initMsgs)

	// Wait for SSE connection to be established on the server side.
	ms.waitConnected(t, 5*time.Second)

	// Initial load should be complete.
	assert.False(t, v.Loading(), "loading should be false after initial load")
	assert.Equal(t, "live", v.StreamMode(), "stream mode should be 'live'")

	// Check "● Live" appears in the header.
	v.SetSize(120, 40)
	assert.Contains(t, v.View(), "● Live")

	// Push a status-change event.
	ms.pushEvent("RunStatusChanged", "abc123", "finished", 1)

	// Run the WaitForAllEvents cmd; it should unblock now that there is an event.
	eventMsgs := runCmdsWithTimeout(pendingCmds, 3*time.Second)
	v, _ = driveView(v, eventMsgs)

	// abc123 should now be "finished".
	var found bool
	for _, r := range v.Runs() {
		if r.RunID == "abc123" {
			assert.Equal(t, smithers.RunStatusFinished, r.Status,
				"abc123 should reflect the SSE status change")
			found = true
		}
	}
	assert.True(t, found, "abc123 must still appear in the run list")
}

// =============================================================
// TestRunsView_PollFallback
// =============================================================

// TestRunsView_PollFallback verifies the auto-poll fallback path:
//   - When /v1/events returns 404, streamMode is set to "polling".
//   - The "○ Polling" indicator appears in the rendered header.
//   - GET /v1/runs is called at least once (initial load).
func TestRunsView_PollFallback(t *testing.T) {
	ms := newMockSSEServer(t, []smithers.RunSummary{
		{RunID: "run-poll", WorkflowName: "wf-poll", Status: smithers.RunStatusRunning},
	})
	ms.disableEvents = true // 404 on /v1/events → triggers poll fallback

	v := views.NewRunsView(ms.newClient())
	cancelViewAtEnd(t, v)

	initMsgs := execBatch(v.Init(), 3*time.Second)
	v, _ = driveView(v, initMsgs)

	assert.Equal(t, "polling", v.StreamMode(),
		"stream mode should be 'polling' when /v1/events returns 404")

	v.SetSize(120, 40)
	assert.Contains(t, v.View(), "○ Polling",
		"polling indicator must appear in the rendered header")

	assert.GreaterOrEqual(t, ms.listRunsHits.Load(), int64(1),
		"GET /v1/runs must have been called at least once")
}

// =============================================================
// TestRunsView_SSEContextCancelDropsConnection
// =============================================================

// TestRunsView_SSEContextCancelDropsConnection verifies that pressing Esc
// cancels the view context and causes the SSE connection to be dropped.
func TestRunsView_SSEContextCancelDropsConnection(t *testing.T) {
	ms := newMockSSEServer(t, []smithers.RunSummary{
		{RunID: "r1", WorkflowName: "wf-1", Status: smithers.RunStatusRunning},
	})

	v := views.NewRunsView(ms.newClient())
	// No cancelViewAtEnd here — we press Esc explicitly in the test body.

	initMsgs := execBatch(v.Init(), 3*time.Second)
	v, _ = driveView(v, initMsgs)

	ms.waitConnected(t, 5*time.Second)

	// Simulate pressing Esc — cancels the view context, emits PopViewMsg.
	v2, escCmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, escCmd)

	popMsg := escCmd()
	_, ok := popMsg.(views.PopViewMsg)
	require.True(t, ok, "Esc should emit PopViewMsg")

	// The SSE connection should drop within 2 s after context cancellation.
	ms.waitDisconnected(t, 2*time.Second)

	// View context should be cancelled.
	rv := v2.(*views.RunsView)
	if ctx := rv.Ctx(); ctx != nil {
		assert.Error(t, ctx.Err(), "view context should be cancelled after Esc")
	}
}

// =============================================================
// TestWaitForAllEvents_IntegrationWithStreamAllEvents
// =============================================================

// TestWaitForAllEvents_IntegrationWithStreamAllEvents verifies that
// WaitForAllEvents correctly receives events from StreamAllEvents via a
// real HTTP SSE connection to a mock server.
func TestWaitForAllEvents_IntegrationWithStreamAllEvents(t *testing.T) {
	ms := newMockSSEServer(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := ms.newClient().StreamAllEvents(ctx)
	require.NoError(t, err, "StreamAllEvents should succeed against mock server")

	ms.waitConnected(t, 2*time.Second)

	// Push two events.
	ms.pushEvent("RunStarted", "r1", "running", 1)
	ms.pushEvent("RunFinished", "r1", "finished", 2)

	// Collect two events using the WaitForAllEvents pattern.
	var received []smithers.RunEventMsg
	for i := 0; i < 2; i++ {
		cmd := smithers.WaitForAllEvents(ch)
		msg := cmd()
		switch m := msg.(type) {
		case smithers.RunEventMsg:
			received = append(received, m)
		case smithers.RunEventDoneMsg:
			t.Fatal("stream closed before expected events arrived")
		case smithers.RunEventErrorMsg:
			t.Fatalf("unexpected stream error: %v", m.Err)
		default:
			t.Fatalf("unexpected message type %T", msg)
		}
	}

	cancel() // closes the stream

	require.Len(t, received, 2)
	assert.Equal(t, "RunStarted", received[0].Event.Type)
	assert.Equal(t, "r1", received[0].RunID)
	assert.Equal(t, "RunFinished", received[1].Event.Type)
	assert.Equal(t, "finished", received[1].Event.Status)
}

// =============================================================
// TestRunsView_SSEStatusUpdateApplied (focused integration test)
// =============================================================

// TestRunsView_SSEStatusUpdateApplied directly exercises the Update loop with
// a RunEventMsg and verifies the in-place status patch.
func TestRunsView_SSEStatusUpdateApplied(t *testing.T) {
	ms := newMockSSEServer(t, []smithers.RunSummary{
		{RunID: "run-a", WorkflowName: "wf-a", Status: smithers.RunStatusRunning},
	})

	v := views.NewRunsView(ms.newClient())
	cancelViewAtEnd(t, v)

	initMsgs := execBatch(v.Init(), 3*time.Second)
	v, _ = driveView(v, initMsgs)

	ms.waitConnected(t, 5*time.Second)

	require.Len(t, v.Runs(), 1, "initial load should have 1 run")
	assert.Equal(t, smithers.RunStatusRunning, v.Runs()[0].Status)

	// Push a finish event directly to the view via a RunEventMsg.
	ev := smithers.RunEvent{Type: "RunFinished", RunID: "run-a", Status: "finished"}
	v2, _ := v.Update(smithers.RunEventMsg{RunID: "run-a", Event: ev})
	rv := v2.(*views.RunsView)

	require.Len(t, rv.Runs(), 1)
	assert.Equal(t, smithers.RunStatusFinished, rv.Runs()[0].Status,
		"status should be updated to finished by the RunEventMsg")
}

// =============================================================
// TestRunsView_SSENewRunInserted (focused integration test)
// =============================================================

// TestRunsView_SSENewRunInserted verifies that a RunStarted event for an
// unknown RunID prepends a stub entry to the runs list.
func TestRunsView_SSENewRunInserted(t *testing.T) {
	ms := newMockSSEServer(t, []smithers.RunSummary{
		{RunID: "existing", WorkflowName: "wf-x", Status: smithers.RunStatusRunning},
	})

	v := views.NewRunsView(ms.newClient())
	cancelViewAtEnd(t, v)

	initMsgs := execBatch(v.Init(), 3*time.Second)
	v, _ = driveView(v, initMsgs)

	ms.waitConnected(t, 5*time.Second)

	ev := smithers.RunEvent{Type: "RunStarted", RunID: "brand-new", Status: "running"}
	v2, _ := v.Update(smithers.RunEventMsg{RunID: "brand-new", Event: ev})
	rv := v2.(*views.RunsView)

	require.Len(t, rv.Runs(), 2, "stub run should be prepended")
	assert.Equal(t, "brand-new", rv.Runs()[0].RunID, "new run at front")
	assert.Equal(t, "existing", rv.Runs()[1].RunID, "existing run preserved")
}

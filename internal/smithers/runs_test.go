package smithers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- v1 test helpers ---

// newV1TestServer creates an httptest.Server that returns direct JSON (v1 API style,
// not the {ok,data,error} envelope used by legacy paths).
func newV1TestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(
		WithAPIURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	c.serverUp = true
	return srv, c
}

// writeV1JSON writes a successful v1 API direct-JSON response.
func writeV1JSON(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(data))
}

// writeV1Error writes a v1 API error response.
func writeV1Error(t *testing.T, w http.ResponseWriter, statusCode int, code, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	require.NoError(t, json.NewEncoder(w).Encode(v1ErrorEnvelope{
		Error: &v1ErrorBody{Code: code, Message: message},
	}))
}

// sampleRunSummary returns a RunSummary with deterministic test data.
func sampleRunSummary(id string) RunSummary {
	started := int64(1700000000000)
	return RunSummary{
		RunID:        id,
		WorkflowName: "code-review",
		WorkflowPath: ".smithers/workflows/code-review.tsx",
		Status:       RunStatusRunning,
		StartedAtMs:  &started,
		Summary:      map[string]int{"running": 1, "pending": 2},
	}
}

// --- RunStatus ---

func TestRunStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		status   RunStatus
		terminal bool
	}{
		{RunStatusRunning, false},
		{RunStatusWaitingApproval, false},
		{RunStatusWaitingEvent, false},
		{RunStatusFinished, true},
		{RunStatusFailed, true},
		{RunStatusCancelled, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.terminal, tc.status.IsTerminal())
		})
	}
}

func TestRunStatus_JSONRoundTrip(t *testing.T) {
	statuses := []RunStatus{
		RunStatusRunning,
		RunStatusWaitingApproval,
		RunStatusWaitingEvent,
		RunStatusFinished,
		RunStatusFailed,
		RunStatusCancelled,
	}
	for _, s := range statuses {
		data, err := json.Marshal(s)
		require.NoError(t, err)
		var got RunStatus
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, s, got)
	}
}

// --- RunSummary JSON round-trip ---

func TestRunSummary_JSONRoundTrip(t *testing.T) {
	started := int64(1700000000000)
	finished := int64(1700000001000)
	errJSON := `{"message":"boom"}`
	original := RunSummary{
		RunID:        "run-abc123",
		WorkflowName: "code-review",
		WorkflowPath: ".smithers/workflows/code-review.tsx",
		Status:       RunStatusFinished,
		StartedAtMs:  &started,
		FinishedAtMs: &finished,
		Summary:      map[string]int{"finished": 3},
		ErrorJSON:    &errJSON,
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got RunSummary
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, original.RunID, got.RunID)
	assert.Equal(t, original.WorkflowName, got.WorkflowName)
	assert.Equal(t, original.Status, got.Status)
	require.NotNil(t, got.StartedAtMs)
	assert.Equal(t, *original.StartedAtMs, *got.StartedAtMs)
	require.NotNil(t, got.FinishedAtMs)
	assert.Equal(t, *original.FinishedAtMs, *got.FinishedAtMs)
	assert.Equal(t, original.Summary, got.Summary)
	require.NotNil(t, got.ErrorJSON)
	assert.Equal(t, *original.ErrorJSON, *got.ErrorJSON)
}

func TestRunSummary_NullableFieldsOmitted(t *testing.T) {
	r := RunSummary{
		RunID:        "run-1",
		WorkflowName: "wf",
		Status:       RunStatusRunning,
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	// Nullable fields should not appear when nil.
	assert.NotContains(t, string(data), "startedAtMs")
	assert.NotContains(t, string(data), "finishedAtMs")
	assert.NotContains(t, string(data), "errorJson")
}

// --- RunTask JSON round-trip ---

func TestRunTask_JSONRoundTrip(t *testing.T) {
	label := "Review code"
	attempt := 2
	updated := int64(1700000002000)
	task := RunTask{
		NodeID:      "node-1",
		Label:       &label,
		Iteration:   1,
		State:       TaskStateRunning,
		LastAttempt: &attempt,
		UpdatedAtMs: &updated,
	}
	data, err := json.Marshal(task)
	require.NoError(t, err)
	var got RunTask
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, task.NodeID, got.NodeID)
	require.NotNil(t, got.Label)
	assert.Equal(t, *task.Label, *got.Label)
	assert.Equal(t, task.State, got.State)
	require.NotNil(t, got.LastAttempt)
	assert.Equal(t, *task.LastAttempt, *got.LastAttempt)
}

// --- RunEvent JSON round-trip ---

func TestRunEvent_JSONRoundTrip(t *testing.T) {
	raw := `{"type":"run_status_changed","runId":"run-1","status":"finished","timestampMs":1700000000000,"seq":42}`
	var ev RunEvent
	require.NoError(t, json.Unmarshal([]byte(raw), &ev))
	assert.Equal(t, "run_status_changed", ev.Type)
	assert.Equal(t, "run-1", ev.RunID)
	assert.Equal(t, "finished", ev.Status)
	assert.Equal(t, int64(1700000000000), ev.TimestampMs)
	assert.Equal(t, 42, ev.Seq)

	// Raw field should be excluded from the default JSON output (json:"-").
	out, err := json.Marshal(ev)
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"Raw"`)
}

// --- TaskState constants ---

func TestTaskState_Values(t *testing.T) {
	states := []TaskState{
		TaskStatePending, TaskStateRunning, TaskStateFinished,
		TaskStateFailed, TaskStateCancelled, TaskStateSkipped, TaskStateBlocked,
	}
	for _, s := range states {
		data, err := json.Marshal(s)
		require.NoError(t, err)
		var got TaskState
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, s, got)
	}
}

// --- ListRuns ---

func TestListRuns_HTTP(t *testing.T) {
	runs := []RunSummary{
		sampleRunSummary("run-1"),
		sampleRunSummary("run-2"),
	}
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "50", r.URL.Query().Get("limit"))
		writeV1JSON(t, w, runs)
	})

	got, err := c.ListRuns(context.Background(), RunFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "run-1", got[0].RunID)
	assert.Equal(t, "run-2", got[1].RunID)
}

func TestListRuns_HTTP_WithStatusFilter(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "running", r.URL.Query().Get("status"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		writeV1JSON(t, w, []RunSummary{sampleRunSummary("run-1")})
	})

	got, err := c.ListRuns(context.Background(), RunFilter{Limit: 10, Status: "running"})
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestListRuns_HTTP_BearerToken(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		writeV1JSON(t, w, []RunSummary{})
	})
	c.apiToken = "secret-token"

	_, err := c.ListRuns(context.Background(), RunFilter{})
	require.NoError(t, err)
}

func TestListRuns_HTTP_DBNotConfigured_ReturnsEmpty(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusBadRequest, "DB_NOT_CONFIGURED", "no db")
	})

	got, err := c.ListRuns(context.Background(), RunFilter{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListRuns_HTTP_MalformedJSON(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not-json{{{")
	})

	_, err := c.ListRuns(context.Background(), RunFilter{})
	require.Error(t, err)
}

func TestListRuns_Exec(t *testing.T) {
	runs := []RunSummary{sampleRunSummary("run-exec-1")}
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "ps", args[0])
		assert.Equal(t, "--format", args[1])
		assert.Equal(t, "json", args[2])
		return json.Marshal(runs)
	})

	got, err := c.ListRuns(context.Background(), RunFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "run-exec-1", got[0].RunID)
}

func TestListRuns_Exec_WithStatusFilter(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--status")
		assert.Contains(t, args, "failed")
		return json.Marshal([]RunSummary{})
	})

	got, err := c.ListRuns(context.Background(), RunFilter{Status: "failed"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListRuns_HTTP_FallsBackToExecOnConnectionError(t *testing.T) {
	// Client with a non-existent server URL and an exec fallback.
	execCalled := false
	c := NewClient(
		WithAPIURL("http://127.0.0.1:19999"), // nothing listening here
		withExecFunc(func(_ context.Context, args ...string) ([]byte, error) {
			execCalled = true
			return json.Marshal([]RunSummary{sampleRunSummary("run-fallback")})
		}),
	)
	// Don't force serverUp so the availability check runs naturally.
	c.serverUp = false
	c.serverChecked = time.Time{} // reset cache

	got, err := c.ListRuns(context.Background(), RunFilter{})
	require.NoError(t, err)
	assert.True(t, execCalled, "expected exec fallback to be called")
	assert.Len(t, got, 1)
}

// --- GetRunSummary ---

func TestGetRunSummary_HTTP(t *testing.T) {
	run := sampleRunSummary("run-abc")
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-abc", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeV1JSON(t, w, run)
	})

	got, err := c.GetRunSummary(context.Background(), "run-abc")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-abc", got.RunID)
	assert.Equal(t, RunStatusRunning, got.Status)
}

func TestGetRunSummary_HTTP_NotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
	})

	_, err := c.GetRunSummary(context.Background(), "missing-run")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestGetRunSummary_HTTP_Unauthorized(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.GetRunSummary(context.Background(), "run-1")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestGetRunSummary_EmptyRunID_ReturnsError(t *testing.T) {
	c := NewClient()
	_, err := c.GetRunSummary(context.Background(), "")
	require.Error(t, err)
}

func TestGetRunSummary_Exec(t *testing.T) {
	run := sampleRunSummary("run-exec")
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "inspect", args[0])
		assert.Equal(t, "run-exec", args[1])
		assert.Equal(t, "--format", args[2])
		assert.Equal(t, "json", args[3])
		return json.Marshal(run)
	})

	got, err := c.GetRunSummary(context.Background(), "run-exec")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-exec", got.RunID)
}

func TestGetRunSummary_Exec_WrappedResponse(t *testing.T) {
	// Some versions of the CLI return { "run": {...} } wrapper.
	run := sampleRunSummary("run-wrapped")
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(map[string]interface{}{"run": run})
	})

	got, err := c.GetRunSummary(context.Background(), "run-wrapped")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-wrapped", got.RunID)
}

func TestGetRunSummary_Exec_EmptyRunID_InWrapper(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		// Returns a wrapper with no runId — should yield ErrRunNotFound.
		return json.Marshal(map[string]interface{}{"run": map[string]interface{}{}})
	})

	_, err := c.GetRunSummary(context.Background(), "ghost-run")
	require.ErrorIs(t, err, ErrRunNotFound)
}

// --- InspectRun ---

func TestInspectRun_HTTP_WithNoTasks(t *testing.T) {
	run := sampleRunSummary("run-inspect")
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-inspect", r.URL.Path)
		writeV1JSON(t, w, run)
	})

	got, err := c.InspectRun(context.Background(), "run-inspect")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-inspect", got.RunID)
	// Tasks may be empty when exec enrichment is not available.
}

func TestInspectRun_EmptyRunID_ReturnsError(t *testing.T) {
	c := NewClient()
	_, err := c.InspectRun(context.Background(), "")
	require.Error(t, err)
}

func TestInspectRun_Exec_WithTasks(t *testing.T) {
	run := sampleRunSummary("run-with-tasks")
	label := "Review"
	attempt := 1
	updated := int64(1700000003000)
	tasks := []RunTask{
		{NodeID: "node-a", Label: &label, Iteration: 0, State: TaskStateFinished,
			LastAttempt: &attempt, UpdatedAtMs: &updated},
		{NodeID: "node-b", Iteration: 0, State: TaskStateRunning},
	}

	callCount := 0
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		callCount++
		if args[0] == "inspect" && len(args) == 4 {
			// GetRunSummary call
			return json.Marshal(run)
		}
		if args[0] == "inspect" && len(args) > 4 {
			// getRunTasks call (--nodes flag)
			return json.Marshal(tasks)
		}
		return nil, fmt.Errorf("unexpected call: %v", args)
	})

	got, err := c.InspectRun(context.Background(), "run-with-tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-with-tasks", got.RunID)
	assert.Len(t, got.Tasks, 2)
	assert.Equal(t, "node-a", got.Tasks[0].NodeID)
	require.NotNil(t, got.Tasks[0].Label)
	assert.Equal(t, "Review", *got.Tasks[0].Label)
}

func TestInspectRun_Exec_TasksFromWrapper(t *testing.T) {
	run := sampleRunSummary("run-wt")
	tasks := []RunTask{{NodeID: "n1", State: TaskStatePending}}
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) == 4 {
			return json.Marshal(run)
		}
		// Return { "tasks": [...] } wrapper form.
		return json.Marshal(map[string]interface{}{"tasks": tasks})
	})

	got, err := c.InspectRun(context.Background(), "run-wt")
	require.NoError(t, err)
	require.Len(t, got.Tasks, 1)
}

func TestInspectRun_Exec_TasksFromNodeWrapper(t *testing.T) {
	run := sampleRunSummary("run-wn")
	tasks := []RunTask{{NodeID: "n1", State: TaskStateBlocked}}
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) == 4 {
			return json.Marshal(run)
		}
		// Return { "nodes": [...] } wrapper form.
		return json.Marshal(map[string]interface{}{"nodes": tasks})
	})

	got, err := c.InspectRun(context.Background(), "run-wn")
	require.NoError(t, err)
	require.Len(t, got.Tasks, 1)
	assert.Equal(t, TaskStateBlocked, got.Tasks[0].State)
}

func TestInspectRun_TaskEnrichmentFailure_StillReturnsRun(t *testing.T) {
	// GetRunSummary succeeds but task enrichment fails — InspectRun should still
	// return the run summary (task enrichment is best-effort).
	run := sampleRunSummary("run-partial")
	callCount := 0
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		callCount++
		if callCount == 1 {
			return json.Marshal(run)
		}
		return nil, errors.New("exec: command not found")
	})

	got, err := c.InspectRun(context.Background(), "run-partial")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-partial", got.RunID)
	assert.Empty(t, got.Tasks) // task enrichment failed silently
}

// --- CancelRun ---

func TestCancelRun_HTTP(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-to-cancel/cancel", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		writeV1JSON(t, w, map[string]string{"runId": "run-to-cancel"})
	})

	err := c.CancelRun(context.Background(), "run-to-cancel")
	require.NoError(t, err)
}

func TestCancelRun_HTTP_NotActive(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusConflict, "RUN_NOT_ACTIVE", "run already finished")
	})

	err := c.CancelRun(context.Background(), "run-finished")
	require.ErrorIs(t, err, ErrRunNotActive)
}

func TestCancelRun_HTTP_NotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.CancelRun(context.Background(), "run-missing")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestCancelRun_HTTP_Unauthorized(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	err := c.CancelRun(context.Background(), "run-1")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestCancelRun_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"cancel", "run-exec-cancel"}, args)
		return nil, nil
	})

	err := c.CancelRun(context.Background(), "run-exec-cancel")
	require.NoError(t, err)
}

func TestCancelRun_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, errors.New("smithers: run not found")
	})

	err := c.CancelRun(context.Background(), "run-gone")
	require.Error(t, err)
}

func TestCancelRun_EmptyRunID_ReturnsError(t *testing.T) {
	c := NewClient()
	err := c.CancelRun(context.Background(), "")
	require.Error(t, err)
}

// --- StreamRunEvents ---

func TestStreamRunEvents_NormalFlow(t *testing.T) {
	runID := "run-stream-1"
	events := []RunEvent{
		{Type: "run_started", RunID: runID, TimestampMs: 1000},
		{Type: "node_state_changed", RunID: runID, NodeID: "node-1", Status: "running", TimestampMs: 2000},
		{Type: "run_status_changed", RunID: runID, Status: "finished", TimestampMs: 3000},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "/v1/runs/run-stream-1/events", r.URL.Path)
		assert.Equal(t, "-1", r.URL.Query().Get("afterSeq"))
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		f, ok := w.(http.Flusher)
		require.True(t, ok)

		for _, ev := range events {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", data)
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	var received []RunEvent
	var done bool
	for msg := range ch {
		switch m := msg.(type) {
		case RunEventMsg:
			received = append(received, m.Event)
		case RunEventDoneMsg:
			done = true
		case RunEventErrorMsg:
			t.Errorf("unexpected error: %v", m.Err)
		}
	}

	assert.True(t, done, "expected RunEventDoneMsg")
	assert.Len(t, received, 3)
	assert.Equal(t, "run_started", received[0].Type)
	assert.Equal(t, "node_state_changed", received[1].Type)
	assert.Equal(t, "run_status_changed", received[2].Type)
}

func TestStreamRunEvents_Heartbeat_IsIgnored(t *testing.T) {
	runID := "run-heartbeat"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		// Send a heartbeat then a real event then close.
		fmt.Fprint(w, ": keep-alive\n\n")
		data, _ := json.Marshal(RunEvent{Type: "ping", RunID: runID, TimestampMs: 1})
		fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", data)
		f.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	var events []RunEvent
	for msg := range ch {
		if m, ok := msg.(RunEventMsg); ok {
			events = append(events, m.Event)
		}
	}
	require.Len(t, events, 1)
	assert.Equal(t, "ping", events[0].Type)
}

func TestStreamRunEvents_MalformedData_SendsErrorContinues(t *testing.T) {
	runID := "run-malformed"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		// First: malformed JSON.
		fmt.Fprint(w, "event: smithers\ndata: not-valid-json\n\n")
		// Second: valid event.
		data, _ := json.Marshal(RunEvent{Type: "ok", RunID: runID, TimestampMs: 1})
		fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", data)
		f.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	var parseErrors []error
	var validEvents []RunEvent
	for msg := range ch {
		switch m := msg.(type) {
		case RunEventMsg:
			validEvents = append(validEvents, m.Event)
		case RunEventErrorMsg:
			parseErrors = append(parseErrors, m.Err)
		}
	}

	assert.Len(t, parseErrors, 1, "expected one parse error for malformed JSON")
	assert.Len(t, validEvents, 1, "expected one valid event after parse error")
}

func TestStreamRunEvents_ContextCancellation(t *testing.T) {
	runID := "run-cancel"
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		f.Flush()
		close(started)
		// Block until client disconnects.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	// Wait for server to receive the connection.
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not receive connection in time")
	}

	// Cancel the context — the goroutine should exit and the channel close.
	cancel()

	// Drain the channel with a timeout; it must close.
	closed := make(chan struct{})
	go func() {
		for range ch {
		}
		close(closed)
	}()
	select {
	case <-closed:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not close after context cancellation")
	}
}

func TestStreamRunEvents_NotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.StreamRunEvents(context.Background(), "missing-run")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestStreamRunEvents_Unauthorized(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.StreamRunEvents(context.Background(), "run-1")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestStreamRunEvents_EmptyRunID_ReturnsError(t *testing.T) {
	c := NewClient(WithAPIURL("http://localhost:7331"))
	_, err := c.StreamRunEvents(context.Background(), "")
	require.Error(t, err)
}

func TestStreamRunEvents_NoAPIURL_ReturnsError(t *testing.T) {
	c := NewClient() // no API URL
	_, err := c.StreamRunEvents(context.Background(), "run-1")
	require.ErrorIs(t, err, ErrServerUnavailable)
}

func TestStreamRunEvents_BearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()), WithAPIToken("my-token"))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, "run-1")
	require.NoError(t, err)
	for range ch {
	}
}

// --- v1 transport helpers ---

func TestDecodeV1Response_ErrorCodes(t *testing.T) {
	cases := []struct {
		code       string
		statusCode int
		wantErr    error
	}{
		{"RUN_NOT_FOUND", http.StatusNotFound, ErrRunNotFound},
		{"RUN_NOT_ACTIVE", http.StatusConflict, ErrRunNotActive},
		{"DB_NOT_CONFIGURED", http.StatusBadRequest, ErrDBNotConfigured},
		{"UNAUTHORIZED", http.StatusForbidden, ErrUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			body, _ := json.Marshal(v1ErrorEnvelope{
				Error: &v1ErrorBody{Code: tc.code, Message: "test error"},
			})
			rec := httptest.NewRecorder()
			rec.WriteHeader(tc.statusCode)
			rec.Body.Write(body)

			err := decodeV1Response(rec.Result(), nil)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestDecodeV1Response_UnknownErrorCode(t *testing.T) {
	body, _ := json.Marshal(v1ErrorEnvelope{
		Error: &v1ErrorBody{Code: "SOME_NEW_CODE", Message: "something went wrong"},
	})
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusInternalServerError)
	rec.Body.Write(body)

	err := decodeV1Response(rec.Result(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SOME_NEW_CODE")
}

func TestDecodeV1Response_200_DecodesOutput(t *testing.T) {
	run := sampleRunSummary("run-ok")
	body, _ := json.Marshal(run)
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	rec.Body.Write(body)

	var got RunSummary
	err := decodeV1Response(rec.Result(), &got)
	require.NoError(t, err)
	assert.Equal(t, "run-ok", got.RunID)
}

// --- parseRunSummaryJSON ---

func TestParseRunSummaryJSON_DirectObject(t *testing.T) {
	run := sampleRunSummary("run-direct")
	data, _ := json.Marshal(run)

	got, err := parseRunSummaryJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "run-direct", got.RunID)
}

func TestParseRunSummaryJSON_WrappedObject(t *testing.T) {
	run := sampleRunSummary("run-wrapped")
	data, _ := json.Marshal(map[string]interface{}{"run": run})

	got, err := parseRunSummaryJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "run-wrapped", got.RunID)
}

func TestParseRunSummaryJSON_WrappedEmptyRunID(t *testing.T) {
	data, _ := json.Marshal(map[string]interface{}{"run": map[string]interface{}{}})

	_, err := parseRunSummaryJSON(data)
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestParseRunSummaryJSON_MalformedJSON(t *testing.T) {
	_, err := parseRunSummaryJSON([]byte("not json"))
	require.Error(t, err)
}

// --- parseRunSummariesJSON ---

func TestParseRunSummariesJSON_ValidArray(t *testing.T) {
	runs := []RunSummary{
		sampleRunSummary("r1"),
		sampleRunSummary("r2"),
	}
	data, _ := json.Marshal(runs)

	got, err := parseRunSummariesJSON(data)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "r1", got[0].RunID)
}

func TestParseRunSummariesJSON_Empty(t *testing.T) {
	got, err := parseRunSummariesJSON([]byte("[]"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseRunSummariesJSON_MalformedJSON(t *testing.T) {
	_, err := parseRunSummariesJSON([]byte("{not an array}"))
	require.Error(t, err)
}

// --- SSE event field parsing ---

func TestStreamRunEvents_RetryLine_IsIgnored(t *testing.T) {
	runID := "run-retry"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		// Upstream servers send an initial retry hint.
		fmt.Fprint(w, "retry: 1000\n\n")
		data, _ := json.Marshal(RunEvent{Type: "started", RunID: runID, TimestampMs: 1})
		fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", data)
		f.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	var events []RunEvent
	for msg := range ch {
		if m, ok := msg.(RunEventMsg); ok {
			events = append(events, m.Event)
		}
	}
	assert.Len(t, events, 1)
	assert.Equal(t, "started", events[0].Type)
}

func TestStreamRunEvents_UnknownEventName_IsSkipped(t *testing.T) {
	// Events with an unknown event name (not "smithers") should be silently skipped.
	runID := "run-unknown-event"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		// A different event type — should be ignored.
		fmt.Fprint(w, "event: other\ndata: {\"ignored\":true}\n\n")
		// Then a real smithers event.
		data, _ := json.Marshal(RunEvent{Type: "real", RunID: runID, TimestampMs: 1})
		fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", data)
		f.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	var events []RunEvent
	for msg := range ch {
		if m, ok := msg.(RunEventMsg); ok {
			events = append(events, m.Event)
		}
	}
	assert.Len(t, events, 1, "only the 'smithers' event should be received")
	assert.Equal(t, "real", events[0].Type)
}

// --- Raw field preservation ---

func TestRunEvent_RawFieldPreserved(t *testing.T) {
	// StreamRunEvents sets ev.Raw so consumers can forward the original payload.
	runID := "run-raw"
	rawPayload := `{"type":"custom","runId":"run-raw","timestampMs":999,"extra":"data"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: smithers\ndata: %s\n\n", rawPayload)
		f.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamRunEvents(ctx, runID)
	require.NoError(t, err)

	for msg := range ch {
		if m, ok := msg.(RunEventMsg); ok {
			assert.Equal(t, rawPayload, string(m.Event.Raw),
				"Raw field should contain original SSE payload")
		}
	}
}

// --- v1ErrorEnvelope ---

func TestV1ErrorEnvelope_JSONRoundTrip(t *testing.T) {
	env := v1ErrorEnvelope{
		Error: &v1ErrorBody{Code: "RUN_NOT_FOUND", Message: "no such run"},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	var got v1ErrorEnvelope
	require.NoError(t, json.Unmarshal(data, &got))
	require.NotNil(t, got.Error)
	assert.Equal(t, "RUN_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "no such run", got.Error.Message)
}

// --- HTTP 500 generic error ---

func TestGetRunSummary_HTTP_ServerError(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"code":"INTERNAL","message":"db error"}}`)
	})

	_, err := c.GetRunSummary(context.Background(), "run-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INTERNAL")
}

// --- v1PostJSON ---

func TestV1PostJSON_BodyEncoding(t *testing.T) {
	type reqBody struct {
		Key string `json:"key"`
	}
	var received reqBody
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		writeV1JSON(t, w, map[string]string{"runId": "r1"})
	})

	err := c.v1PostJSON(context.Background(), "/v1/runs/r1/cancel", reqBody{Key: "value"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "value", received.Key)
}

func TestV1PostJSON_NilBody(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1JSON(t, w, map[string]string{"ok": "true"})
	})

	err := c.v1PostJSON(context.Background(), "/v1/runs/r1/cancel", nil, nil)
	require.NoError(t, err)
}

func TestV1GetJSON_DecodesOutput(t *testing.T) {
	run := sampleRunSummary("run-get-json")
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1JSON(t, w, run)
	})

	var got RunSummary
	err := c.v1GetJSON(context.Background(), "/v1/runs/run-get-json", &got)
	require.NoError(t, err)
	assert.Equal(t, "run-get-json", got.RunID)
}

func TestV1GetJSON_ServerUnavailable(t *testing.T) {
	c := NewClient(
		WithAPIURL("http://127.0.0.1:19998"),
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
	)
	c.serverUp = true

	err := c.v1GetJSON(context.Background(), "/v1/runs", nil)
	require.ErrorIs(t, err, ErrServerUnavailable)
}

func TestGetRunSummary_HTTP_FallsBackToExecOnConnectionError(t *testing.T) {
	execCalled := false
	run := sampleRunSummary("run-fallback-get")
	c := NewClient(
		WithAPIURL("http://127.0.0.1:19997"),
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
		withExecFunc(func(_ context.Context, args ...string) ([]byte, error) {
			execCalled = true
			return json.Marshal(run)
		}),
	)
	c.serverUp = false
	c.serverChecked = time.Time{}

	got, err := c.GetRunSummary(context.Background(), "run-fallback-get")
	require.NoError(t, err)
	assert.True(t, execCalled)
	assert.Equal(t, "run-fallback-get", got.RunID)
}

func TestCancelRun_HTTP_FallsBackToExecOnConnectionError(t *testing.T) {
	execCalled := false
	c := NewClient(
		WithAPIURL("http://127.0.0.1:19996"),
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
		withExecFunc(func(_ context.Context, args ...string) ([]byte, error) {
			execCalled = true
			assert.Equal(t, "cancel", args[0])
			return nil, nil
		}),
	)
	c.serverUp = false
	c.serverChecked = time.Time{}

	err := c.CancelRun(context.Background(), "run-cancel-fallback")
	require.NoError(t, err)
	assert.True(t, execCalled)
}

// --- Integration: ListRuns → GetRunSummary → CancelRun ---

func TestIntegration_ListGetCancel(t *testing.T) {
	runID := "run-integration"
	run := sampleRunSummary(runID)
	cancelled := false

	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/runs":
			writeV1JSON(t, w, []RunSummary{run})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/v1/runs/"+runID):
			writeV1JSON(t, w, run)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/cancel"):
			cancelled = true
			writeV1JSON(t, w, map[string]string{"runId": runID})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ctx := context.Background()

	// 1. List runs.
	runs, err := c.ListRuns(ctx, RunFilter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, runID, runs[0].RunID)

	// 2. Get individual run.
	got, err := c.GetRunSummary(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, RunStatusRunning, got.Status)

	// 3. Cancel it.
	err = c.CancelRun(ctx, runID)
	require.NoError(t, err)
	assert.True(t, cancelled)
}

// --- StreamChat ---

func TestStreamChat_SSE_ThreeBlocks(t *testing.T) {
	blocks := []ChatBlock{
		{RunID: "run-sse", NodeID: "n1", Attempt: 1, Role: ChatRoleUser, Content: "hello", TimestampMs: 100},
		{RunID: "run-sse", NodeID: "n1", Attempt: 1, Role: ChatRoleAssistant, Content: "hi there", TimestampMs: 200},
		{RunID: "run-sse", NodeID: "n1", Attempt: 1, Role: ChatRoleTool, Content: "tool output", TimestampMs: 300},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "/v1/runs/run-sse/chat/stream", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		f, ok := w.(http.Flusher)
		require.True(t, ok)

		for _, b := range blocks {
			data, _ := json.Marshal(b)
			fmt.Fprintf(w, "data: %s\n\n", data)
			f.Flush()
		}
		// Connection close signals end of stream.
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamChat(ctx, "run-sse")
	require.NoError(t, err)
	require.NotNil(t, ch)

	var received []ChatBlock
	for b := range ch {
		received = append(received, b)
	}

	require.Len(t, received, 3)
	assert.Equal(t, ChatRoleUser, received[0].Role)
	assert.Equal(t, "hello", received[0].Content)
	assert.Equal(t, ChatRoleAssistant, received[1].Role)
	assert.Equal(t, "hi there", received[1].Content)
	assert.Equal(t, ChatRoleTool, received[2].Role)
	assert.Equal(t, "tool output", received[2].Content)
}

func TestStreamChat_NoServer(t *testing.T) {
	c := NewClient() // no API URL
	_, err := c.StreamChat(context.Background(), "run-noserver")
	assert.ErrorIs(t, err, ErrServerUnavailable)
}

func TestStreamChat_EmptyRunID(t *testing.T) {
	c := NewClient(WithAPIURL("http://localhost:9999"))
	_, err := c.StreamChat(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runID")
}

func TestStreamChat_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	_, err := c.StreamChat(context.Background(), "missing-run")
	assert.ErrorIs(t, err, ErrRunNotFound)
}

func TestStreamChat_ContextCancellation(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		f.Flush()
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.StreamChat(ctx, "run-cancel")
	require.NoError(t, err)

	<-started
	cancel()

	// Drain until closed.
	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // success
			}
		case <-timeout:
			t.Fatal("channel not closed after context cancellation")
		}
	}
}

// --- WaitForChatBlock ---

func TestWaitForChatBlock_ReturnsBlock(t *testing.T) {
	ch := make(chan ChatBlock, 1)
	block := ChatBlock{RunID: "r1", NodeID: "n1", Role: ChatRoleAssistant, Content: "hello", TimestampMs: 100}
	ch <- block

	cmd := WaitForChatBlock("r1", ch)
	msg := cmd()
	cbm, ok := msg.(ChatBlockMsg)
	require.True(t, ok)
	assert.Equal(t, "r1", cbm.RunID)
	assert.Equal(t, "hello", cbm.Block.Content)
}

func TestWaitForChatBlock_ChannelClosed(t *testing.T) {
	ch := make(chan ChatBlock)
	close(ch)

	cmd := WaitForChatBlock("r1", ch)
	msg := cmd()
	done, ok := msg.(ChatStreamDoneMsg)
	require.True(t, ok)
	assert.Equal(t, "r1", done.RunID)
}

// --- HijackRun ---

func TestHijackRun_HTTP(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-hijack/hijack", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		writeV1JSON(t, w, HijackSession{
			RunID:          "run-hijack",
			AgentEngine:    "claude-code",
			AgentBinary:    "/usr/local/bin/claude",
			ResumeToken:    "sess-abc123",
			CWD:            "/home/user/project",
			SupportsResume: true,
		})
	})

	session, err := c.HijackRun(context.Background(), "run-hijack")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "run-hijack", session.RunID)
	assert.Equal(t, "claude-code", session.AgentEngine)
	assert.Equal(t, "/usr/local/bin/claude", session.AgentBinary)
	assert.Equal(t, "sess-abc123", session.ResumeToken)
	assert.Equal(t, "/home/user/project", session.CWD)
	assert.True(t, session.SupportsResume)
}

func TestHijackRun_NoServer(t *testing.T) {
	c := NewClient() // no API URL
	_, err := c.HijackRun(context.Background(), "run-noserver")
	assert.ErrorIs(t, err, ErrServerUnavailable)
}

func TestHijackRun_EmptyRunID(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server for empty runID")
	})

	_, err := c.HijackRun(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runID")
}

func TestHijackRun_RunNotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
	})

	_, err := c.HijackRun(context.Background(), "run-missing")
	assert.ErrorIs(t, err, ErrRunNotFound)
}

// --- HijackSession ---

func TestHijackSession_ResumeArgs_WithToken(t *testing.T) {
	s := &HijackSession{ResumeToken: "abc", SupportsResume: true}
	args := s.ResumeArgs()
	assert.Equal(t, []string{"--resume", "abc"}, args)
}

func TestHijackSession_ResumeArgs_NoToken(t *testing.T) {
	s := &HijackSession{ResumeToken: "", SupportsResume: true}
	assert.Nil(t, s.ResumeArgs())
}

func TestHijackSession_ResumeArgs_NotSupported(t *testing.T) {
	s := &HijackSession{ResumeToken: "abc", SupportsResume: false}
	assert.Nil(t, s.ResumeArgs())
}

// --- ApproveNode ---

func TestApproveNode_HTTP(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-1/approve/node-1", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	err := c.ApproveNode(context.Background(), "run-1", "node-1")
	require.NoError(t, err)
}

func TestApproveNode_HTTP_NotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
	})

	err := c.ApproveNode(context.Background(), "missing-run", "node-1")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestApproveNode_HTTP_Unauthorized(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	err := c.ApproveNode(context.Background(), "run-1", "node-1")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestApproveNode_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		require.Equal(t, []string{"approve", "run-1", "--node", "node-1"}, args)
		return nil, nil
	})

	err := c.ApproveNode(context.Background(), "run-1", "node-1")
	require.NoError(t, err)
}

func TestApproveNode_EmptyRunID(t *testing.T) {
	c := NewClient()
	err := c.ApproveNode(context.Background(), "", "node-1")
	require.Error(t, err)
}

func TestApproveNode_EmptyNodeID(t *testing.T) {
	c := NewClient()
	err := c.ApproveNode(context.Background(), "run-1", "")
	require.Error(t, err)
}

// --- DenyNode ---

func TestDenyNode_HTTP(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-1/deny/node-1", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	err := c.DenyNode(context.Background(), "run-1", "node-1")
	require.NoError(t, err)
}

func TestDenyNode_HTTP_NotFound(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1Error(t, w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
	})

	err := c.DenyNode(context.Background(), "missing-run", "node-1")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestDenyNode_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		require.Equal(t, []string{"deny", "run-1", "--node", "node-1"}, args)
		return nil, nil
	})

	err := c.DenyNode(context.Background(), "run-1", "node-1")
	require.NoError(t, err)
}

func TestDenyNode_EmptyRunID(t *testing.T) {
	c := NewClient()
	err := c.DenyNode(context.Background(), "", "node-1")
	require.Error(t, err)
}

func TestDenyNode_EmptyNodeID(t *testing.T) {
	c := NewClient()
	err := c.DenyNode(context.Background(), "run-1", "")
	require.Error(t, err)
}

// ============================================================
// WaitForAllEvents
// ============================================================

// TestWaitForAllEvents_EventDelivery verifies that a RunEventMsg sent on the
// channel is returned directly by the cmd function.
func TestWaitForAllEvents_EventDelivery(t *testing.T) {
	ch := make(chan interface{}, 1)
	ev := RunEventMsg{RunID: "r1", Event: RunEvent{Type: "RunStarted", RunID: "r1"}}
	ch <- ev

	cmd := WaitForAllEvents(ch)
	msg := cmd()

	require.IsType(t, RunEventMsg{}, msg)
	got := msg.(RunEventMsg)
	assert.Equal(t, "r1", got.RunID)
	assert.Equal(t, "RunStarted", got.Event.Type)
}

// TestWaitForAllEvents_ChannelClosed verifies that closing the channel causes
// the cmd to return RunEventDoneMsg{}.
func TestWaitForAllEvents_ChannelClosed(t *testing.T) {
	ch := make(chan interface{})
	close(ch)

	cmd := WaitForAllEvents(ch)
	msg := cmd()

	assert.IsType(t, RunEventDoneMsg{}, msg)
}

// TestWaitForAllEvents_ErrorMsgPassthrough verifies that a RunEventErrorMsg in
// the channel is returned unchanged.
func TestWaitForAllEvents_ErrorMsgPassthrough(t *testing.T) {
	ch := make(chan interface{}, 1)
	errMsg := RunEventErrorMsg{RunID: "r2", Err: errors.New("parse error")}
	ch <- errMsg

	cmd := WaitForAllEvents(ch)
	msg := cmd()

	require.IsType(t, RunEventErrorMsg{}, msg)
	got := msg.(RunEventErrorMsg)
	assert.Equal(t, "r2", got.RunID)
	assert.EqualError(t, got.Err, "parse error")
}

// TestWaitForAllEvents_SelfRescheduling verifies the idiomatic self-re-scheduling
// pattern: each cmd() call returns one message; the caller dispatches again to
// receive the next one.
func TestWaitForAllEvents_SelfRescheduling(t *testing.T) {
	ch := make(chan interface{}, 3)
	for i := 0; i < 3; i++ {
		ch <- RunEventMsg{
			RunID: "r1",
			Event: RunEvent{Type: fmt.Sprintf("E%d", i), RunID: "r1"},
		}
	}
	close(ch)

	var received []RunEventMsg
	for {
		cmd := WaitForAllEvents(ch)
		msg := cmd()
		switch m := msg.(type) {
		case RunEventMsg:
			received = append(received, m)
		case RunEventDoneMsg:
			goto done
		default:
			t.Fatalf("unexpected message type %T", msg)
		}
	}
done:
	require.Len(t, received, 3)
	for i, m := range received {
		assert.Equal(t, fmt.Sprintf("E%d", i), m.Event.Type)
	}
}

// TestWaitForAllEvents_DoneMsg verifies that a RunEventDoneMsg value in the
// channel is passed through directly (the channel hasn't been closed yet, but
// the stream is signalling done via a typed value).
func TestWaitForAllEvents_DoneMsg(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- RunEventDoneMsg{RunID: "r3"}

	cmd := WaitForAllEvents(ch)
	msg := cmd()

	require.IsType(t, RunEventDoneMsg{}, msg)
}

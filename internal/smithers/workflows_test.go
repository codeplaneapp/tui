package smithers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers specific to workflow tests ---

// newWorkspaceTestServer creates a test server and a client pre-configured with
// workspaceID "ws-1" so workspace-scoped routes resolve correctly.
func newWorkspaceTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
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
		WithWorkspaceID("ws-1"),
	)
	// Force server available cache so tests don't need a live /health round-trip.
	c.serverUp = true
	return srv, c
}

// writeDaemonJSON writes a daemon-style direct JSON response (no envelope).
func writeDaemonJSON(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(data))
}

// writeDaemonError writes a daemon-style error response: { "error": "msg" }.
func writeDaemonError(t *testing.T, w http.ResponseWriter, status int, msg string) {
	t.Helper()
	writeDaemonJSON(t, w, status, daemonErrorResponse{Error: msg})
}

// sampleWorkflow returns a test Workflow value.
func sampleWorkflow(id string) Workflow {
	updatedAt := "2026-01-01T00:00:00Z"
	return Workflow{
		ID:           id,
		WorkspaceID:  "ws-1",
		Name:         "My Workflow",
		RelativePath: ".smithers/workflows/" + id + ".tsx",
		Status:       WorkflowStatusActive,
		UpdatedAt:    &updatedAt,
	}
}

// sampleWorkflowDefinition returns a WorkflowDefinition for the given ID.
func sampleWorkflowDefinition(id string) WorkflowDefinition {
	return WorkflowDefinition{
		Workflow: sampleWorkflow(id),
		Source:   `import { createSmithers } from "smithers-orchestrator";\nexport default smithers((ctx) => (<Workflow name="` + id + `" />));`,
	}
}

// sampleDAGDefinition returns a DAGDefinition for the given workflowID.
func sampleDAGDefinition(workflowID string) DAGDefinition {
	entryTaskID := "plan"
	return DAGDefinition{
		WorkflowID:  workflowID,
		Mode:        "inferred",
		EntryTaskID: &entryTaskID,
		Fields: []WorkflowTask{
			{Key: "prompt", Label: "Prompt", Type: "string"},
			{Key: "ticketId", Label: "Ticket ID", Type: "string"},
		},
	}
}

// --- ListWorkflows ---

func TestListWorkflows_HTTP(t *testing.T) {
	workflows := []Workflow{sampleWorkflow("audit"), sampleWorkflow("review")}
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/workspaces/ws-1/workflows", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeDaemonJSON(t, w, http.StatusOK, workflows)
	})

	got, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "audit", got[0].ID)
	assert.Equal(t, "review", got[1].ID)
	assert.Equal(t, WorkflowStatusActive, got[0].Status)
}

func TestListWorkflows_HTTP_EmptyList(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonJSON(t, w, http.StatusOK, []Workflow{})
	})

	got, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListWorkflows_HTTP_ServerError(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonError(t, w, http.StatusInternalServerError, "internal error")
	})

	_, err := c.ListWorkflows(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
}

func TestListWorkflows_Exec(t *testing.T) {
	// Client with no server URL → falls straight to exec.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"workflow", "list", "--format", "json"}, args)
		return json.Marshal(map[string]any{
			"workflows": []DiscoveredWorkflow{
				{ID: "grill-me", DisplayName: "Grill Me", EntryFile: "/proj/.smithers/workflows/grill-me.tsx", SourceType: "seeded"},
				{ID: "plan", DisplayName: "Plan", EntryFile: "/proj/.smithers/workflows/plan.tsx", SourceType: "user"},
			},
		})
	})

	got, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "grill-me", got[0].ID)
	assert.Equal(t, "Grill Me", got[0].Name)
	assert.Equal(t, "/proj/.smithers/workflows/grill-me.tsx", got[0].RelativePath)
	assert.Equal(t, WorkflowStatusActive, got[0].Status)
	assert.Equal(t, "plan", got[1].ID)
}

func TestListWorkflows_Exec_BareArray(t *testing.T) {
	// Some CLI versions may return a bare array instead of a wrapper.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]DiscoveredWorkflow{
			{ID: "review", DisplayName: "Review", EntryFile: "/proj/.smithers/workflows/review.tsx", SourceType: "seeded"},
		})
	})

	got, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "review", got[0].ID)
}

func TestListWorkflows_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers workflow list: command not found")
	})

	_, err := c.ListWorkflows(context.Background())
	require.Error(t, err)
}

func TestListWorkflows_NoWorkspaceID_FallsToExec(t *testing.T) {
	// Client has a server URL but no workspaceID — should skip HTTP and exec.
	execCalled := false
	c := NewClient(
		WithAPIURL("http://localhost:9999"),
		withExecFunc(func(_ context.Context, args ...string) ([]byte, error) {
			execCalled = true
			return json.Marshal(map[string]any{"workflows": []DiscoveredWorkflow{}})
		}),
	)
	c.serverUp = true

	_, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	assert.True(t, execCalled, "should fall through to exec when workspaceID is not set")
}

func TestListWorkflows_ServerDown_FallsToExec(t *testing.T) {
	execCalled := false
	c := NewClient(
		WithWorkspaceID("ws-1"),
		withExecFunc(func(_ context.Context, args ...string) ([]byte, error) {
			execCalled = true
			return json.Marshal(map[string]any{"workflows": []DiscoveredWorkflow{
				{ID: "test", DisplayName: "Test", EntryFile: "/p/test.tsx", SourceType: "user"},
			}})
		}),
	)
	// serverUp = false (default) → HTTP skipped.

	got, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
	assert.True(t, execCalled)
	require.Len(t, got, 1)
	assert.Equal(t, "test", got[0].ID)
}

// --- GetWorkflowDefinition ---

func TestGetWorkflowDefinition_HTTP(t *testing.T) {
	def := sampleWorkflowDefinition("audit")
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/workspaces/ws-1/workflows/audit", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeDaemonJSON(t, w, http.StatusOK, def)
	})

	got, err := c.GetWorkflowDefinition(context.Background(), "audit")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "audit", got.ID)
	assert.Equal(t, "My Workflow", got.Name)
	assert.Contains(t, got.Source, "smithers-orchestrator")
}

func TestGetWorkflowDefinition_HTTP_NotFound(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonError(t, w, http.StatusNotFound, "Workflow not found: missing")
	})

	_, err := c.GetWorkflowDefinition(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestGetWorkflowDefinition_HTTP_MalformedJSON(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	})

	_, err := c.GetWorkflowDefinition(context.Background(), "audit")
	require.Error(t, err)
}

func TestGetWorkflowDefinition_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.GetWorkflowDefinition(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflowID is required")
}

func TestGetWorkflowDefinition_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"workflow", "path", "audit", "--format", "json"}, args)
		return json.Marshal(map[string]any{
			"id":         "audit",
			"path":       "/proj/.smithers/workflows/audit.tsx",
			"sourceType": "seeded",
		})
	})

	got, err := c.GetWorkflowDefinition(context.Background(), "audit")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "audit", got.ID)
	assert.Equal(t, "/proj/.smithers/workflows/audit.tsx", got.RelativePath)
	assert.Equal(t, WorkflowStatusActive, got.Status)
}

func TestGetWorkflowDefinition_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers workflow path missing: WORKFLOW_NOT_FOUND")
	})

	_, err := c.GetWorkflowDefinition(context.Background(), "missing")
	require.Error(t, err)
}

func TestGetWorkflowDefinition_Exec_EmptyID(t *testing.T) {
	// exec returns valid JSON but with empty ID → ErrWorkflowNotFound.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(map[string]any{"id": "", "path": ""})
	})

	_, err := c.GetWorkflowDefinition(context.Background(), "ghost")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

// --- RunWorkflow ---

func TestRunWorkflow_HTTP(t *testing.T) {
	expectedRun := RunSummary{
		RunID:        "run-abc123",
		WorkflowName: "Audit",
		Status:       RunStatusRunning,
	}
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/workspaces/ws-1/runs", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "audit", body["workflowId"])
		inputs, ok := body["input"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "fix ticket PROJ-123", inputs["prompt"])

		writeDaemonJSON(t, w, http.StatusCreated, expectedRun)
	})

	got, err := c.RunWorkflow(context.Background(), "audit", map[string]any{"prompt": "fix ticket PROJ-123"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-abc123", got.RunID)
	assert.Equal(t, RunStatusRunning, got.Status)
}

func TestRunWorkflow_HTTP_NoInputs(t *testing.T) {
	// When inputs is nil/empty, the "input" key should be omitted from the body.
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "review", body["workflowId"])
		_, hasInput := body["input"]
		assert.False(t, hasInput, "input key should be omitted when inputs is empty")

		writeDaemonJSON(t, w, http.StatusCreated, RunSummary{RunID: "run-xyz", Status: RunStatusRunning})
	})

	got, err := c.RunWorkflow(context.Background(), "review", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-xyz", got.RunID)
}

func TestRunWorkflow_HTTP_WorkflowNotFound(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonError(t, w, http.StatusNotFound, "Workflow not found: missing")
	})

	_, err := c.RunWorkflow(context.Background(), "missing", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestRunWorkflow_HTTP_ServerError(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonError(t, w, http.StatusInternalServerError, "workflow startup failed")
	})

	_, err := c.RunWorkflow(context.Background(), "audit", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow startup failed")
}

func TestRunWorkflow_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.RunWorkflow(context.Background(), "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflowID is required")
}

func TestRunWorkflow_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "workflow", args[0])
		assert.Equal(t, "run", args[1])
		assert.Equal(t, "audit", args[2])
		assert.Equal(t, "--format", args[3])
		assert.Equal(t, "json", args[4])
		assert.Equal(t, "--input", args[5])
		// args[6] is the JSON-encoded input.
		return json.Marshal(RunSummary{RunID: "run-exec-1", Status: RunStatusRunning, WorkflowName: "Audit"})
	})

	got, err := c.RunWorkflow(context.Background(), "audit", map[string]any{"prompt": "hello"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-exec-1", got.RunID)
}

func TestRunWorkflow_Exec_NoInputs(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		// --input flag should not appear when inputs is empty.
		for _, arg := range args {
			if arg == "--input" {
				t.Errorf("--input flag should be omitted when inputs is nil")
			}
		}
		return json.Marshal(RunSummary{RunID: "run-exec-2", Status: RunStatusRunning})
	})

	got, err := c.RunWorkflow(context.Background(), "review", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-exec-2", got.RunID)
}

func TestRunWorkflow_Exec_WrappedResult(t *testing.T) {
	// Some CLI versions may return { "run": {...} } wrapper.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"run": RunSummary{RunID: "run-exec-3", Status: RunStatusRunning},
		})
	})

	got, err := c.RunWorkflow(context.Background(), "plan", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-exec-3", got.RunID)
}

func TestRunWorkflow_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers workflow run: workflow not found")
	})

	_, err := c.RunWorkflow(context.Background(), "missing", nil)
	require.Error(t, err)
}

// --- GetWorkflowDAG ---

func TestGetWorkflowDAG_HTTP(t *testing.T) {
	dag := sampleDAGDefinition("audit")
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/workspaces/ws-1/workflows/audit/launch-fields", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeDaemonJSON(t, w, http.StatusOK, dag)
	})

	got, err := c.GetWorkflowDAG(context.Background(), "audit")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "audit", got.WorkflowID)
	assert.Equal(t, "inferred", got.Mode)
	assert.Equal(t, "plan", *got.EntryTaskID)
	require.Len(t, got.Fields, 2)
	assert.Equal(t, "prompt", got.Fields[0].Key)
	assert.Equal(t, "Prompt", got.Fields[0].Label)
	assert.Equal(t, "string", got.Fields[0].Type)
	assert.Equal(t, "ticketId", got.Fields[1].Key)
}

func TestGetWorkflowDAG_HTTP_NotFound(t *testing.T) {
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonError(t, w, http.StatusNotFound, "Workflow not found: ghost")
	})

	_, err := c.GetWorkflowDAG(context.Background(), "ghost")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestGetWorkflowDAG_HTTP_FallbackMode(t *testing.T) {
	msg := "static analysis failed"
	dag := DAGDefinition{
		WorkflowID: "myflow",
		Mode:       "fallback",
		Fields:     []WorkflowTask{{Key: "prompt", Label: "Prompt", Type: "string"}},
		Message:    &msg,
	}
	_, c := newWorkspaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeDaemonJSON(t, w, http.StatusOK, dag)
	})

	got, err := c.GetWorkflowDAG(context.Background(), "myflow")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "fallback", got.Mode)
	assert.NotNil(t, got.Message)
	assert.Contains(t, *got.Message, "static analysis failed")
}

func TestGetWorkflowDAG_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.GetWorkflowDAG(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflowID is required")
}

func TestGetWorkflowDAG_Exec_FallbackDAG(t *testing.T) {
	// Without a daemon, exec returns a generic fallback DAG.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"workflow", "path", "plan", "--format", "json"}, args)
		return json.Marshal(map[string]any{
			"id":   "plan",
			"path": "/proj/.smithers/workflows/plan.tsx",
		})
	})

	got, err := c.GetWorkflowDAG(context.Background(), "plan")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "plan", got.WorkflowID)
	assert.Equal(t, "fallback", got.Mode)
	require.Len(t, got.Fields, 1)
	assert.Equal(t, "prompt", got.Fields[0].Key)
	assert.NotNil(t, got.Message)
}

func TestGetWorkflowDAG_Exec_WorkflowNotFound(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers workflow path ghost: WORKFLOW_NOT_FOUND")
	})

	_, err := c.GetWorkflowDAG(context.Background(), "ghost")
	require.Error(t, err)
}

func TestGetWorkflowDAG_Exec_EmptyID(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(map[string]any{"id": "", "path": ""})
	})

	_, err := c.GetWorkflowDAG(context.Background(), "ghost")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

// --- HTTP transport: bearer token propagation ---

func TestWorkflowMethods_BearerToken(t *testing.T) {
	const token = "test-secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		writeDaemonJSON(t, w, http.StatusOK, []Workflow{})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(
		WithAPIURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithAPIToken(token),
		WithWorkspaceID("ws-1"),
	)
	c.serverUp = true

	_, err := c.ListWorkflows(context.Background())
	require.NoError(t, err)
}

// --- parseDiscoveredWorkflowsJSON ---

func TestParseDiscoveredWorkflowsJSON_Wrapped(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"workflows": []DiscoveredWorkflow{
			{ID: "a", DisplayName: "A", EntryFile: "/a.tsx", SourceType: "user"},
		},
	})
	require.NoError(t, err)

	got, err := parseDiscoveredWorkflowsJSON(data)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "A", got[0].Name)
}

func TestParseDiscoveredWorkflowsJSON_BareArray(t *testing.T) {
	data, err := json.Marshal([]DiscoveredWorkflow{
		{ID: "b", DisplayName: "B", EntryFile: "/b.tsx", SourceType: "seeded"},
	})
	require.NoError(t, err)

	got, err := parseDiscoveredWorkflowsJSON(data)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].ID)
}

func TestParseDiscoveredWorkflowsJSON_Invalid(t *testing.T) {
	_, err := parseDiscoveredWorkflowsJSON([]byte("not json"))
	require.Error(t, err)
}

// --- adaptDiscoveredWorkflows ---

func TestAdaptDiscoveredWorkflows_Empty(t *testing.T) {
	result := adaptDiscoveredWorkflows(nil)
	assert.Empty(t, result)
}

func TestAdaptDiscoveredWorkflows(t *testing.T) {
	input := []DiscoveredWorkflow{
		{ID: "x", DisplayName: "X Flow", EntryFile: "/path/x.tsx", SourceType: "generated"},
	}
	got := adaptDiscoveredWorkflows(input)
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].ID)
	assert.Equal(t, "X Flow", got[0].Name)
	assert.Equal(t, "/path/x.tsx", got[0].RelativePath)
	assert.Equal(t, WorkflowStatusActive, got[0].Status)
	// WorkspaceID should be empty (no daemon context).
	assert.Empty(t, got[0].WorkspaceID)
}

// --- decodeDaemonResponse ---

func TestDecodeDaemonResponse_Unauthorized(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusUnauthorized)
	resp := recorder.Result()

	err := decodeDaemonResponse(resp, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestDecodeDaemonResponse_NotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusNotFound)
	resp := recorder.Result()

	err := decodeDaemonResponse(resp, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestDecodeDaemonResponse_ErrorBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusBadRequest)
	require.NoError(t, json.NewEncoder(recorder).Encode(daemonErrorResponse{Error: "bad input"}))
	resp := recorder.Result()

	err := decodeDaemonResponse(resp, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad input")
}

func TestDecodeDaemonResponse_UnknownError(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusTeapot)
	resp := recorder.Result()

	err := decodeDaemonResponse(resp, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "418")
}

func TestDecodeDaemonResponse_Success_NilOut(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusNoContent)
	resp := recorder.Result()

	err := decodeDaemonResponse(resp, nil)
	require.NoError(t, err)
}

func TestDecodeDaemonResponse_Success_JSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	require.NoError(t, json.NewEncoder(recorder).Encode(Workflow{ID: "test"}))
	resp := recorder.Result()

	var w Workflow
	err := decodeDaemonResponse(resp, &w)
	require.NoError(t, err)
	assert.Equal(t, "test", w.ID)
}

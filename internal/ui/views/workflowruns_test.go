package views

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRunView_ImplementsView(t *testing.T) {
	t.Parallel()

	var _ View = (*WorkflowRunView)(nil)
}

func TestWorkflowRunView_DefaultRegistryContainsWorkflowRuns(t *testing.T) {
	t.Parallel()

	r := DefaultRegistry()
	v, ok := r.Open("workflow-runs", smithers.NewClient())
	require.True(t, ok)
	assert.Equal(t, "workflow-runs", v.Name())
}

func TestWorkflowRunView_StartStreamFallbackWithoutServer(t *testing.T) {
	t.Parallel()

	v := NewWorkflowRunView(smithers.NewClient())
	v.ctx = context.Background()

	msg := v.startStreamCmd()()
	_, ok := msg.(workflowStreamUnavailableMsg)
	assert.True(t, ok)
}

func TestWorkflowRunView_RunEventUpdatesStatusesAndTasks(t *testing.T) {
	t.Parallel()

	v := seededWorkflowRunView()

	updated, _ := v.Update(smithers.RunEventMsg{
		RunID: "run-1",
		Event: smithers.RunEvent{
			Type:        "node_state_changed",
			RunID:       "run-1",
			NodeID:      "task-1",
			Status:      "failed",
			TimestampMs: 2000,
		},
	})
	v = updated.(*WorkflowRunView)

	task := v.inspections["run-1"].Tasks[0]
	assert.Equal(t, smithers.TaskStateFailed, task.State)

	updated, _ = v.Update(smithers.RunEventMsg{
		RunID: "run-1",
		Event: smithers.RunEvent{
			Type:        "run_status_changed",
			RunID:       "run-1",
			Status:      "finished",
			TimestampMs: 3000,
		},
	})
	v = updated.(*WorkflowRunView)

	assert.Equal(t, smithers.RunStatusFinished, v.runs[0].Status)
	require.NotNil(t, v.runs[0].FinishedAtMs)
}

func TestWorkflowRunView_NavigationAcrossPanes(t *testing.T) {
	t.Parallel()

	v := seededWorkflowRunView()

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	v = updated.(*WorkflowRunView)
	assert.Equal(t, workflowPaneTasks, v.focus)
	assert.NotNil(t, cmd)

	updated, _ = v.Update(tea.KeyPressMsg{Text: "l", Code: 'l'})
	v = updated.(*WorkflowRunView)
	assert.Equal(t, workflowPaneLogs, v.focus)

	updated, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	v = updated.(*WorkflowRunView)
	assert.Equal(t, workflowPaneTasks, v.focus)
}

func TestWorkflowRunView_ResponsiveViewModes(t *testing.T) {
	t.Parallel()

	v := seededWorkflowRunView()
	v.logs[v.logKey("run-1", v.inspections["run-1"].Tasks[0])] = workflowTaskLog{
		key:    v.logKey("run-1", v.inspections["run-1"].Tasks[0]),
		runID:  "run-1",
		nodeID: "task-1",
		loaded: true,
		blocks: []smithers.ChatBlock{{RunID: "run-1", NodeID: "task-1", Role: smithers.ChatRoleAssistant, Content: "done"}},
	}
	v.syncLogViewer()

	v.SetSize(160, 20)
	wide := v.View()
	assert.Contains(t, wide, "Runs")
	assert.Contains(t, wide, "Tasks")
	assert.Contains(t, wide, "build")

	v.focus = workflowPaneLogs
	v.SetSize(120, 20)
	medium := v.View()
	assert.Contains(t, medium, "Runs")
	assert.Contains(t, medium, "build")

	v.SetSize(80, 20)
	narrow := v.View()
	assert.NotContains(t, narrow, "Build Workflow")
	assert.Contains(t, narrow, "build")
}

func TestWorkflowRunView_LoadTaskLogsCmdFiltersNodeAndAttempt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/runs/run-1/chat":
			writeEnvelopeResponse(t, w, []smithers.ChatBlock{
				{RunID: "run-1", NodeID: "task-1", Attempt: 1, Role: smithers.ChatRoleAssistant, Content: "selected"},
				{RunID: "run-1", NodeID: "task-1", Attempt: 0, Role: smithers.ChatRoleAssistant, Content: "old"},
				{RunID: "run-1", NodeID: "task-2", Attempt: 1, Role: smithers.ChatRoleAssistant, Content: "other"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := smithers.NewClient(smithers.WithAPIURL(srv.URL), smithers.WithHTTPClient(srv.Client()))
	v := NewWorkflowRunView(client)
	v.ctx = context.Background()

	attempt := 1
	msg := v.loadTaskLogsCmd("run-1", smithers.RunTask{
		NodeID:      "task-1",
		LastAttempt: &attempt,
	})()
	require.NotNil(t, msg)

	logsMsg := msg.(workflowRunLogsLoadedMsg)
	require.NoError(t, logsMsg.err)
	require.Len(t, logsMsg.blocks, 1)
	assert.Equal(t, "selected", logsMsg.blocks[0].Content)
}

func seededWorkflowRunView() *WorkflowRunView {
	started := int64(1000)
	label := "build"
	attempt := 0

	v := NewWorkflowRunView(smithers.NewClient())
	v.runs = []smithers.RunSummary{{
		RunID:        "run-1",
		WorkflowName: "Build Workflow",
		Status:       smithers.RunStatusRunning,
		StartedAtMs:  &started,
		Summary: map[string]int{
			"finished": 0,
			"total":    1,
		},
	}}
	v.loading = false
	v.inspections["run-1"] = &smithers.RunInspection{
		RunSummary: v.runs[0],
		Tasks: []smithers.RunTask{{
			NodeID:      "task-1",
			Label:       &label,
			State:       smithers.TaskStateRunning,
			LastAttempt: &attempt,
		}},
	}
	v.syncLogViewer()
	return v
}

func writeEnvelopeResponse(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": json.RawMessage(raw),
	}))
}

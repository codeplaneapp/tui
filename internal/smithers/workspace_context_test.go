package smithers

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchWorkspaceContext_NilClient(t *testing.T) {
	wsCtx := FetchWorkspaceContext(context.Background(), nil)
	assert.Empty(t, wsCtx.ActiveRuns)
	assert.Equal(t, 0, wsCtx.PendingApprovals)
}

func TestFetchWorkspaceContext_NoRunsAvailable(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeV1JSON(t, w, []RunSummary{})
	})

	wsCtx := FetchWorkspaceContext(context.Background(), c)
	assert.Empty(t, wsCtx.ActiveRuns)
	assert.Equal(t, 0, wsCtx.PendingApprovals)
}

func TestFetchWorkspaceContext_ActiveAndApprovalRuns(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		switch RunStatus(status) {
		case RunStatusRunning:
			writeV1JSON(t, w, []RunSummary{
				{RunID: "run-1", WorkflowName: "ci", Status: RunStatusRunning},
			})
		case RunStatusWaitingApproval:
			writeV1JSON(t, w, []RunSummary{
				{RunID: "run-2", WorkflowName: "deploy", Status: RunStatusWaitingApproval},
			})
		case RunStatusWaitingEvent:
			writeV1JSON(t, w, []RunSummary{})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	wsCtx := FetchWorkspaceContext(context.Background(), c)
	require.Len(t, wsCtx.ActiveRuns, 2)
	assert.Equal(t, 1, wsCtx.PendingApprovals)

	ids := make([]string, 0, len(wsCtx.ActiveRuns))
	for _, r := range wsCtx.ActiveRuns {
		ids = append(ids, r.RunID)
	}
	assert.Contains(t, ids, "run-1")
	assert.Contains(t, ids, "run-2")
}

func TestFetchWorkspaceContext_ServerUnavailable_ReturnsEmpty(t *testing.T) {
	c := NewClient(
		WithAPIURL("http://127.0.0.1:1"),
		withExecFunc(func(_ context.Context, _ ...string) ([]byte, error) {
			return nil, ErrServerUnavailable
		}),
	)

	wsCtx := FetchWorkspaceContext(context.Background(), c)
	assert.Empty(t, wsCtx.ActiveRuns)
	assert.Equal(t, 0, wsCtx.PendingApprovals)
}

func TestFetchWorkspaceContext_OnlyWaitingApproval(t *testing.T) {
	_, c := newV1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		switch RunStatus(status) {
		case RunStatusWaitingApproval:
			writeV1JSON(t, w, []RunSummary{
				{RunID: "run-ap1", WorkflowName: "gate-review", Status: RunStatusWaitingApproval},
				{RunID: "run-ap2", WorkflowName: "gate-deploy", Status: RunStatusWaitingApproval},
			})
		default:
			writeV1JSON(t, w, []RunSummary{})
		}
	})

	wsCtx := FetchWorkspaceContext(context.Background(), c)
	assert.Equal(t, 2, wsCtx.PendingApprovals)
	require.Len(t, wsCtx.ActiveRuns, 2)
}

package smithers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRecentDecisions_HTTP(t *testing.T) {
	decidedBy := "alice"
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/approval/decisions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apiEnvelope{
				OK: true,
				Data: json.RawMessage(mustMarshal(t, []ApprovalDecision{
					{
						ID:          "d1",
						RunID:       "run-1",
						NodeID:      "node-1",
						WorkflowPath: "deploy.tsx",
						Gate:        "Deploy?",
						Decision:    "approved",
						DecidedAt:   1700000010000,
						DecidedBy:   &decidedBy,
						RequestedAt: 1700000000000,
					},
					{
						ID:          "d2",
						RunID:       "run-2",
						NodeID:      "node-2",
						WorkflowPath: "ci.tsx",
						Gate:        "Continue?",
						Decision:    "denied",
						DecidedAt:   1700000020000,
						DecidedBy:   nil,
						RequestedAt: 1700000005000,
					},
				})),
			})
		}
	})

	decisions, err := c.ListRecentDecisions(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, decisions, 2)
	assert.Equal(t, "d1", decisions[0].ID)
	assert.Equal(t, "approved", decisions[0].Decision)
	assert.Equal(t, "alice", *decisions[0].DecidedBy)
	assert.Equal(t, "d2", decisions[1].ID)
	assert.Nil(t, decisions[1].DecidedBy)
}

func TestListRecentDecisions_FallbackReturnsNil(t *testing.T) {
	// When HTTP and SQLite are both unavailable, exec returns nil gracefully.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, ErrBinaryNotFound
	})
	decisions, err := c.ListRecentDecisions(context.Background(), 10)
	require.NoError(t, err)
	assert.Nil(t, decisions)
}

func TestListRecentDecisions_Empty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/approval/decisions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apiEnvelope{
				OK:   true,
				Data: json.RawMessage(`[]`),
			})
		}
	})
	decisions, err := c.ListRecentDecisions(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, decisions)
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

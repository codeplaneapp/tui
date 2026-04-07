package smithers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
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

// TestListRecentDecisions_SQLite verifies the SQLite fallback path correctly
// filters on plain "approved" / "denied" status values (not quoted).
func TestListRecentDecisions_SQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_approvals.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE _smithers_approvals (
		id TEXT, run_id TEXT, node_id TEXT, workflow_path TEXT, gate TEXT,
		status TEXT, payload TEXT, requested_at INTEGER,
		resolved_at INTEGER, resolved_by TEXT)`)
	require.NoError(t, err)

	// Insert rows with different statuses: approved, denied, and pending.
	rows := []struct {
		id, status string
		resolvedAt *int64
		resolvedBy *string
	}{
		{"a1", "approved", ptr(int64(1700000010000)), ptr("alice")},
		{"a2", "denied", ptr(int64(1700000020000)), nil},
		{"a3", "pending", nil, nil}, // should be excluded by the filter
	}
	for i, r := range rows {
		_, err = db.Exec(
			`INSERT INTO _smithers_approvals VALUES (?,?,?,?,?,?,?,?,?,?)`,
			r.id, fmt.Sprintf("run-%d", i), fmt.Sprintf("node-%d", i),
			"wf.tsx", "Continue?", r.status, "", int64(1700000000000),
			r.resolvedAt, r.resolvedBy)
		require.NoError(t, err)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	decisions, err := c.ListRecentDecisions(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, decisions, 2, "should return only approved/denied rows, not pending")

	// Results are ordered by resolved_at DESC.
	assert.Equal(t, "a2", decisions[0].ID)
	assert.Equal(t, "denied", decisions[0].Decision)

	assert.Equal(t, "a1", decisions[1].ID)
	assert.Equal(t, "approved", decisions[1].Decision)
	assert.Equal(t, "alice", *decisions[1].DecidedBy)
}

func ptr[T any](v T) *T { return &v }

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

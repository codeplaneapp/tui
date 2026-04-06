package smithers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // register "sqlite" driver for ListRecentScores tests
)

// --- Test helpers ---

// newTestServer creates an httptest.Server that returns the Smithers API envelope.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
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
	// Force server available cache.
	c.serverUp = true
	return srv, c
}

// writeEnvelope writes a successful API envelope response.
func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	dataBytes, err := json.Marshal(data)
	require.NoError(t, err)
	env := apiEnvelope{OK: true, Data: json.RawMessage(dataBytes)}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(env)
	require.NoError(t, err)
}

// newExecClient creates a Client that uses a mock exec function.
func newExecClient(fn func(ctx context.Context, args ...string) ([]byte, error)) *Client {
	return NewClient(withExecFunc(fn))
}

// --- ExecuteSQL ---

func TestExecuteSQL_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sql", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "SELECT 1", body["query"])

		writeEnvelope(t, w, map[string]interface{}{
			"results": []map[string]interface{}{
				{"result": float64(1)},
			},
		})
	})

	result, err := c.ExecuteSQL(context.Background(), "SELECT 1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"result"}, result.Columns)
	assert.Len(t, result.Rows, 1)
}

func TestExecuteSQL_Exec(t *testing.T) {
	// The smithers CLI has no `sql` subcommand. When no HTTP server is available
	// and no SQLite DB is configured, ExecuteSQL must return ErrNoTransport
	// rather than attempting an exec fallback that would always fail.
	c := NewClient() // no server, no db, no exec func
	result, err := c.ExecuteSQL(context.Background(), "SELECT 1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoTransport)
	assert.Nil(t, result)
}

func TestExecuteSQL_NoTransport(t *testing.T) {
	// Same as above via exec client: no SQL subcommand in upstream CLI.
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return nil, nil
	})
	_, err := c.ExecuteSQL(context.Background(), "SELECT 1")
	require.ErrorIs(t, err, ErrNoTransport)
	assert.False(t, execCalled, "exec should not be called for SQL queries")
}

func TestIsSelectQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"SELECT * FROM t", true},
		{"  select count(*) from t", true},
		{"PRAGMA table_info(t)", true},
		{"EXPLAIN SELECT 1", true},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET x = 1", false},
		{"DELETE FROM t", false},
		{"DROP TABLE t", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			assert.Equal(t, tt.want, isSelectQuery(tt.query))
		})
	}
}

// --- GetScores ---

func TestGetScores_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "scores", args[0])
		assert.Equal(t, "run-123", args[1])
		return json.Marshal([]ScoreRow{
			{ID: "s1", RunID: "run-123", ScorerID: "quality", ScorerName: "Quality", Score: 0.85, ScoredAtMs: 1000},
		})
	})

	scores, err := c.GetScores(context.Background(), "run-123", nil)
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, "s1", scores[0].ID)
	assert.Equal(t, 0.85, scores[0].Score)
}

func TestGetScores_ExecWithNodeFilter(t *testing.T) {
	nodeID := "node-1"
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--node")
		assert.Contains(t, args, "node-1")
		return json.Marshal([]ScoreRow{})
	})

	scores, err := c.GetScores(context.Background(), "run-123", &nodeID)
	require.NoError(t, err)
	assert.Empty(t, scores)
}

// --- GetAggregateScores ---

func TestGetAggregateScores(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]ScoreRow{
			{ScorerID: "quality", ScorerName: "Quality", Score: 0.8},
			{ScorerID: "quality", ScorerName: "Quality", Score: 0.6},
			{ScorerID: "quality", ScorerName: "Quality", Score: 1.0},
			{ScorerID: "speed", ScorerName: "Speed", Score: 0.5},
			{ScorerID: "speed", ScorerName: "Speed", Score: 0.7},
		})
	})

	aggs, err := c.GetAggregateScores(context.Background(), "run-1")
	require.NoError(t, err)
	require.Len(t, aggs, 2)

	// Results sorted by ScorerID
	assert.Equal(t, "quality", aggs[0].ScorerID)
	assert.Equal(t, 3, aggs[0].Count)
	assert.InDelta(t, 0.8, aggs[0].Mean, 0.001)
	assert.Equal(t, 0.6, aggs[0].Min)
	assert.Equal(t, 1.0, aggs[0].Max)
	assert.InDelta(t, 0.8, aggs[0].P50, 0.001)

	assert.Equal(t, "speed", aggs[1].ScorerID)
	assert.Equal(t, 2, aggs[1].Count)
	assert.InDelta(t, 0.6, aggs[1].Mean, 0.001)
}

func TestAggregateScores_Empty(t *testing.T) {
	aggs := aggregateScores(nil)
	assert.Empty(t, aggs)
}

func TestAggregateScores_SingleValue(t *testing.T) {
	aggs := aggregateScores([]ScoreRow{
		{ScorerID: "x", ScorerName: "X", Score: 0.5},
	})
	require.Len(t, aggs, 1)
	assert.Equal(t, 1, aggs[0].Count)
	assert.Equal(t, 0.5, aggs[0].Mean)
	assert.Equal(t, 0.5, aggs[0].P50)
	assert.Equal(t, 0.5, aggs[0].Min)
	assert.Equal(t, 0.5, aggs[0].Max)
	assert.True(t, math.IsNaN(aggs[0].StdDev) || aggs[0].StdDev == 0.0,
		"stddev of single value should be 0 or NaN")
}

// --- ListMemoryFacts ---

func TestListMemoryFacts_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "memory", args[0])
		assert.Equal(t, "list", args[1])
		assert.Equal(t, "default", args[2])
		return json.Marshal([]MemoryFact{
			{Namespace: "default", Key: "greeting", ValueJSON: `"hello"`, CreatedAtMs: 1000, UpdatedAtMs: 2000},
		})
	})

	facts, err := c.ListMemoryFacts(context.Background(), "default", "")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "greeting", facts[0].Key)
}

func TestListMemoryFacts_ExecWithWorkflow(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--workflow")
		assert.Contains(t, args, "my-workflow.tsx")
		return json.Marshal([]MemoryFact{})
	})

	facts, err := c.ListMemoryFacts(context.Background(), "default", "my-workflow.tsx")
	require.NoError(t, err)
	assert.Empty(t, facts)
}

// --- RecallMemory ---

func TestRecallMemory_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "memory", args[0])
		assert.Equal(t, "recall", args[1])
		assert.Equal(t, "test query", args[2])
		assert.Contains(t, args, "--topK")
		assert.Contains(t, args, "5")
		return json.Marshal([]MemoryRecallResult{
			{Score: 0.95, Content: "relevant fact"},
		})
	})

	results, err := c.RecallMemory(context.Background(), "test query", nil, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0.95, results[0].Score)
	assert.Equal(t, "relevant fact", results[0].Content)
}

func TestRecallMemory_ExecWithNamespace(t *testing.T) {
	ns := "custom-ns"
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--namespace")
		assert.Contains(t, args, "custom-ns")
		return json.Marshal([]MemoryRecallResult{})
	})

	results, err := c.RecallMemory(context.Background(), "query", &ns, 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// --- ListCrons ---

func TestListCrons_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/cron/list", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		writeEnvelope(t, w, []CronSchedule{
			{CronID: "c1", Pattern: "0 */6 * * *", WorkflowPath: "deploy.tsx", Enabled: true, CreatedAtMs: 1000},
		})
	})

	crons, err := c.ListCrons(context.Background())
	require.NoError(t, err)
	require.Len(t, crons, 1)
	assert.Equal(t, "c1", crons[0].CronID)
	assert.Equal(t, "0 */6 * * *", crons[0].Pattern)
	assert.True(t, crons[0].Enabled)
}

func TestListCrons_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"cron", "list", "--format", "json"}, args)
		return json.Marshal([]CronSchedule{
			{CronID: "c2", Pattern: "0 0 * * *", WorkflowPath: "nightly.tsx", Enabled: false},
		})
	})

	crons, err := c.ListCrons(context.Background())
	require.NoError(t, err)
	require.Len(t, crons, 1)
	assert.Equal(t, "c2", crons[0].CronID)
	assert.False(t, crons[0].Enabled)
}

// --- CreateCron ---

func TestCreateCron_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/cron/add", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "0 0 * * *", body["pattern"])
		assert.Equal(t, "nightly.tsx", body["workflowPath"])

		writeEnvelope(t, w, CronSchedule{
			CronID: "new-1", Pattern: "0 0 * * *", WorkflowPath: "nightly.tsx", Enabled: true,
		})
	})

	cron, err := c.CreateCron(context.Background(), "0 0 * * *", "nightly.tsx")
	require.NoError(t, err)
	require.NotNil(t, cron)
	assert.Equal(t, "new-1", cron.CronID)
}

func TestCreateCron_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "cron", args[0])
		assert.Equal(t, "add", args[1])
		assert.Equal(t, "0 0 * * *", args[2])
		assert.Equal(t, "nightly.tsx", args[3])
		return json.Marshal(CronSchedule{
			CronID: "new-2", Pattern: "0 0 * * *", WorkflowPath: "nightly.tsx", Enabled: true,
		})
	})

	cron, err := c.CreateCron(context.Background(), "0 0 * * *", "nightly.tsx")
	require.NoError(t, err)
	require.NotNil(t, cron)
	assert.Equal(t, "new-2", cron.CronID)
}

// --- ToggleCron ---

func TestToggleCron_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/cron/toggle/c1", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]bool
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.False(t, body["enabled"])

		writeEnvelope(t, w, nil)
	})

	err := c.ToggleCron(context.Background(), "c1", false)
	require.NoError(t, err)
}

// TestToggleCron_Exec_Enable verifies that enabling a cron uses `cron enable <id>`.
// The upstream CLI uses `cron enable` / `cron disable`, not `cron toggle --enabled <bool>`.
func TestToggleCron_Exec_Enable(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		require.Len(t, args, 3, "expected: cron enable <id>")
		assert.Equal(t, "cron", args[0])
		assert.Equal(t, "enable", args[1])
		assert.Equal(t, "c1", args[2])
		return nil, nil
	})

	err := c.ToggleCron(context.Background(), "c1", true)
	require.NoError(t, err)
}

// TestToggleCron_Exec_Disable verifies that disabling a cron uses `cron disable <id>`.
func TestToggleCron_Exec_Disable(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		require.Len(t, args, 3, "expected: cron disable <id>")
		assert.Equal(t, "cron", args[0])
		assert.Equal(t, "disable", args[1])
		assert.Equal(t, "c1", args[2])
		return nil, nil
	})

	err := c.ToggleCron(context.Background(), "c1", false)
	require.NoError(t, err)
}

// --- DeleteCron ---

func TestDeleteCron_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"cron", "rm", "c1"}, args)
		return nil, nil
	})

	err := c.DeleteCron(context.Background(), "c1")
	require.NoError(t, err)
}

// --- Transport fallback ---

func TestTransportFallback_ServerDown(t *testing.T) {
	// Client with no server URL, no DB, and a working exec func.
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return json.Marshal([]CronSchedule{
			{CronID: "c1", Pattern: "* * * * *"},
		})
	})

	crons, err := c.ListCrons(context.Background())
	require.NoError(t, err)
	assert.True(t, execCalled, "should have fallen through to exec")
	assert.Len(t, crons, 1)
}

// --- ListPendingApprovals ---

func TestListPendingApprovals_HTTP(t *testing.T) {
	now := int64(1700000000000) // fixed timestamp for deterministic test
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/approval/list", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		writeEnvelope(t, w, []Approval{
			{
				ID: "appr-1", RunID: "run-abc", NodeID: "deploy",
				WorkflowPath: "deploy.tsx", Gate: "Deploy to staging",
				Status: "pending", RequestedAt: now,
			},
			{
				ID: "appr-2", RunID: "run-xyz", NodeID: "delete",
				WorkflowPath: "cleanup.tsx", Gate: "Delete user data",
				Status: "approved", RequestedAt: now - 60000,
			},
		})
	})

	approvals, err := c.ListPendingApprovals(context.Background())
	require.NoError(t, err)
	require.Len(t, approvals, 2)

	assert.Equal(t, "appr-1", approvals[0].ID)
	assert.Equal(t, "run-abc", approvals[0].RunID)
	assert.Equal(t, "deploy", approvals[0].NodeID)
	assert.Equal(t, "Deploy to staging", approvals[0].Gate)
	assert.Equal(t, "pending", approvals[0].Status)
	assert.Equal(t, now, approvals[0].RequestedAt)

	assert.Equal(t, "appr-2", approvals[1].ID)
	assert.Equal(t, "approved", approvals[1].Status)
}

func TestListPendingApprovals_HTTP_EmptyList(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/approval/list", r.URL.Path)
		writeEnvelope(t, w, []Approval{})
	})

	approvals, err := c.ListPendingApprovals(context.Background())
	require.NoError(t, err)
	// Should return a non-nil empty slice, not nil.
	assert.NotNil(t, approvals)
	assert.Empty(t, approvals)
}

func TestListPendingApprovals_ExecFallbackReturnsNil(t *testing.T) {
	// The exec fallback returns nil because `smithers approval list` doesn't exist.
	// When both HTTP and SQLite are unavailable, we get an empty result.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, errors.New("smithers not installed")
	})

	approvals, err := c.ListPendingApprovals(context.Background())
	require.NoError(t, err)
	assert.Nil(t, approvals)
}

func TestListPendingApprovals_ParsePlainJSON(t *testing.T) {
	// Verify that parseApprovalsJSON handles plain JSON arrays correctly.
	now := int64(1700000000000)
	data, _ := json.Marshal([]Approval{
		{ID: "plain-1", RunID: "run-p", NodeID: "n1", Gate: "Plain Gate",
			Status: "pending", RequestedAt: now},
	})

	approvals, err := parseApprovalsJSON(data)
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	assert.Equal(t, "plain-1", approvals[0].ID)
}

func TestParseApprovalsJSON_ValidArray(t *testing.T) {
	data := `[{"id":"a1","runId":"r1","nodeId":"n1","gate":"G","status":"pending","requestedAt":1000}]`
	approvals, err := parseApprovalsJSON([]byte(data))
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	assert.Equal(t, "a1", approvals[0].ID)
	assert.Equal(t, "pending", approvals[0].Status)
	assert.Equal(t, int64(1000), approvals[0].RequestedAt)
}

func TestParseApprovalsJSON_EmptyArray(t *testing.T) {
	approvals, err := parseApprovalsJSON([]byte(`[]`))
	require.NoError(t, err)
	assert.NotNil(t, approvals)
	assert.Empty(t, approvals)
}

func TestParseApprovalsJSON_InvalidJSON(t *testing.T) {
	_, err := parseApprovalsJSON([]byte(`not-json`))
	assert.Error(t, err)
}

// --- convertResultMaps ---

func TestConvertResultMaps_Empty(t *testing.T) {
	r := convertResultMaps(nil)
	assert.Empty(t, r.Columns)
	assert.Empty(t, r.Rows)
}

func TestConvertResultMaps(t *testing.T) {
	input := []map[string]interface{}{
		{"a": float64(1), "b": "hello"},
		{"a": float64(2), "b": "world"},
	}
	r := convertResultMaps(input)
	assert.Equal(t, []string{"a", "b"}, r.Columns)
	assert.Len(t, r.Rows, 2)
	assert.Equal(t, float64(1), r.Rows[0][0])
	assert.Equal(t, "hello", r.Rows[0][1])
}

// --- ListAgents backward compat ---

func TestListAgents_NoOptions(t *testing.T) {
	c := NewClient() // no options — backward compat
	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)
	assert.Len(t, agents, 7)
	assert.Equal(t, "claude-code", agents[0].ID)
}

// --- GetRun ---

func TestGetRun_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-123", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, RunSummary{
			RunID:        "run-123",
			WorkflowName: "code-review",
			Status:       RunStatusRunning,
		})
	})

	run, err := c.GetRun(context.Background(), "run-123")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-123", run.RunID)
	assert.Equal(t, "code-review", run.WorkflowName)
	assert.Equal(t, RunStatusRunning, run.Status)
}

func TestGetRun_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"run", "get", "run-456", "--format", "json"}, args)
		return json.Marshal(RunSummary{
			RunID:        "run-456",
			WorkflowName: "deploy",
			Status:       RunStatusFinished,
		})
	})

	run, err := c.GetRun(context.Background(), "run-456")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-456", run.RunID)
	assert.Equal(t, RunStatusFinished, run.Status)
}

// --- GetChatOutput ---

func TestGetChatOutput_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/runs/run-789/chat", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, []ChatBlock{
			{RunID: "run-789", NodeID: "n1", Role: ChatRoleAssistant, Content: "Hello", TimestampMs: 1000},
		})
	})

	blocks, err := c.GetChatOutput(context.Background(), "run-789")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, ChatRoleAssistant, blocks[0].Role)
	assert.Equal(t, "Hello", blocks[0].Content)
}

func TestGetChatOutput_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"run", "chat", "run-abc", "--format", "json"}, args)
		return json.Marshal([]ChatBlock{
			{RunID: "run-abc", NodeID: "n1", Role: ChatRoleSystem, Content: "System prompt", TimestampMs: 500},
			{RunID: "run-abc", NodeID: "n1", Role: ChatRoleAssistant, Content: "Response", TimestampMs: 1500},
		})
	})

	blocks, err := c.GetChatOutput(context.Background(), "run-abc")
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	assert.Equal(t, ChatRoleSystem, blocks[0].Role)
	assert.Equal(t, "System prompt", blocks[0].Content)
	assert.Equal(t, ChatRoleAssistant, blocks[1].Role)
}

func TestGetChatOutput_Empty(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]ChatBlock{})
	})

	blocks, err := c.GetChatOutput(context.Background(), "run-empty")
	require.NoError(t, err)
	assert.Empty(t, blocks)
}

// --- ListAgents detection ---

// newDetectionClient creates a Client with mocked lookPath and statFunc.
func newDetectionClient(
	lp func(string) (string, error),
	sf func(string) (os.FileInfo, error),
) *Client {
	return NewClient(withLookPath(lp), withStatFunc(sf))
}

func TestListAgents_BinaryFound_WithAuthDir(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			if file == "claude" {
				return "/usr/local/bin/claude", nil
			}
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			// Simulate ~/.claude existing
			if strings.HasSuffix(name, ".claude") {
				return nil, nil // stat succeeds
			}
			return nil, os.ErrNotExist
		},
	)

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)
	require.Len(t, agents, 7)

	var claude Agent
	for _, a := range agents {
		if a.ID == "claude-code" {
			claude = a
			break
		}
	}
	assert.Equal(t, "likely-subscription", claude.Status)
	assert.True(t, claude.HasAuth)
	assert.True(t, claude.Usable)
	assert.Equal(t, "/usr/local/bin/claude", claude.BinaryPath)
}

func TestListAgents_BinaryFound_WithAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-key")

	c := newDetectionClient(
		func(file string) (string, error) {
			if file == "codex" {
				return "/usr/local/bin/codex", nil
			}
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // no auth dirs present
		},
	)

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)

	var codex Agent
	for _, a := range agents {
		if a.ID == "codex" {
			codex = a
			break
		}
	}
	assert.Equal(t, "api-key", codex.Status)
	assert.False(t, codex.HasAuth)
	assert.True(t, codex.HasAPIKey)
	assert.True(t, codex.Usable)
}

func TestListAgents_BinaryFound_NoAuth(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			if file == "gemini" {
				return "/usr/local/bin/gemini", nil
			}
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	)
	// Ensure no API key env vars are set for gemini
	t.Setenv("GEMINI_API_KEY", "")

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)

	var gemini Agent
	for _, a := range agents {
		if a.ID == "gemini" {
			gemini = a
			break
		}
	}
	assert.Equal(t, "binary-only", gemini.Status)
	assert.False(t, gemini.HasAuth)
	assert.False(t, gemini.HasAPIKey)
	assert.True(t, gemini.Usable)
}

func TestListAgents_BinaryNotFound(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	)

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)
	require.Len(t, agents, 7)

	for _, a := range agents {
		assert.Equal(t, "unavailable", a.Status, "agent %s should be unavailable", a.ID)
		assert.False(t, a.Usable, "agent %s should not be usable", a.ID)
		assert.Empty(t, a.BinaryPath, "agent %s should have no binary path", a.ID)
	}
}

func TestListAgents_AllKnownAgentsReturned(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	)

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)
	assert.Len(t, agents, 7)

	ids := make([]string, len(agents))
	for i, a := range agents {
		ids[i] = a.ID
	}
	assert.Contains(t, ids, "claude-code")
	assert.Contains(t, ids, "codex")
	assert.Contains(t, ids, "opencode")
	assert.Contains(t, ids, "gemini")
	assert.Contains(t, ids, "kimi")
	assert.Contains(t, ids, "amp")
	assert.Contains(t, ids, "forge")
}

func TestListAgents_ContextCancelled_DoesNotPanic(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Detection is synchronous; cancelled context should not cause panic or error.
	agents, err := c.ListAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 7)
}

func TestListAgents_RolesPopulated(t *testing.T) {
	c := newDetectionClient(
		func(file string) (string, error) {
			if file == "claude" {
				return "/usr/local/bin/claude", nil
			}
			return "", os.ErrNotExist
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	)

	agents, err := c.ListAgents(context.Background())
	require.NoError(t, err)

	var claude Agent
	for _, a := range agents {
		if a.ID == "claude-code" {
			claude = a
			break
		}
	}
	assert.NotEmpty(t, claude.Roles)
	assert.Contains(t, claude.Roles, "coding")
}

// --- ListTickets ---

func TestListTickets_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/list", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, []Ticket{
			{ID: "auth-bug", Content: "# Auth Bug\n\n## Summary\n\nFix the auth module."},
			{ID: "deploy-fix", Content: "# Deploy Fix\n\n## Summary\n\nFix deploys."},
		})
	})
	tickets, err := c.ListTickets(context.Background())
	require.NoError(t, err)
	require.Len(t, tickets, 2)
	assert.Equal(t, "auth-bug", tickets[0].ID)
	assert.Contains(t, tickets[0].Content, "Auth Bug")
}

func TestListTickets_Filesystem(t *testing.T) {
	// ListTickets now reads from .smithers/tickets/*.md on the filesystem.
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, ".smithers", "tickets")
	require.NoError(t, os.MkdirAll(ticketsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ticketsDir, "test-ticket.md"), []byte("# Test\n\nContent here."), 0644))

	c := NewClient(WithWorkingDir(dir))
	tickets, err := c.ListTickets(context.Background())
	require.NoError(t, err)
	require.Len(t, tickets, 1)
	assert.Equal(t, "test-ticket", tickets[0].ID)
	assert.Contains(t, tickets[0].Content, "Content here.")
}

func TestListTickets_Empty(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]Ticket{})
	})
	tickets, err := c.ListTickets(context.Background())
	require.NoError(t, err)
	assert.Empty(t, tickets)
}

// --- ListRecentScores and AggregateAllScores ---

// TestListRecentScores_SQLite seeds a temporary database and asserts ordering.
func TestListRecentScores_SQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE _smithers_scorer_results (
		id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
		scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
		meta_json TEXT, input_json TEXT, output_json TEXT,
		latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER)`)
	require.NoError(t, err)

	// Insert 3 rows with different scored_at_ms values (out of order).
	// Column order: id, run_id, node_id, iteration, attempt, scorer_id, scorer_name,
	// source, score, reason, meta_json, input_json, output_json, latency_ms, scored_at_ms, duration_ms
	for i, ts := range []int64{100, 300, 200} {
		_, err = db.Exec(`INSERT INTO _smithers_scorer_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("s%d", i), "run-1", "node-1", 0, 0,
			"relevancy", "Relevancy", "live", 0.8+float64(i)*0.05,
			nil, nil, nil, nil, nil, ts, nil)
		require.NoError(t, err)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, scores, 3)

	// Results must be ordered by scored_at_ms DESC: 300, 200, 100.
	assert.Equal(t, int64(300), scores[0].ScoredAtMs)
	assert.Equal(t, int64(200), scores[1].ScoredAtMs)
	assert.Equal(t, int64(100), scores[2].ScoredAtMs)
}

// TestListRecentScores_NoTable treats a missing table as empty (not an error).
func TestListRecentScores_NoTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_notables.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, _ = db.Exec("CREATE TABLE unrelated (id TEXT)")
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 10)
	require.NoError(t, err) // must NOT return an error
	assert.Empty(t, scores)
}

// TestListRecentScores_LimitRespected asserts limit is applied.
func TestListRecentScores_LimitRespected(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_limit.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE _smithers_scorer_results (
		id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
		scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
		meta_json TEXT, input_json TEXT, output_json TEXT,
		latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER)`)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		_, _ = db.Exec(`INSERT INTO _smithers_scorer_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("s%d", i), "run-1", "node-1", 0, 0,
			"q", "Quality", "live", 0.5, nil, nil, nil, nil, nil, int64(i), nil)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 3)
	require.NoError(t, err)
	assert.Len(t, scores, 3)
}

// TestListRecentScores_NoDB returns nil when no database is configured.
func TestListRecentScores_NoDB(t *testing.T) {
	c := NewClient() // no DB configured
	scores, err := c.ListRecentScores(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, scores)
}

// TestAggregateAllScores_NoDB returns empty aggregates when no database is configured.
func TestAggregateAllScores_NoDB(t *testing.T) {
	c := NewClient() // no DB, no exec func override
	aggs, err := c.AggregateAllScores(context.Background(), 100)
	require.NoError(t, err)
	assert.Empty(t, aggs)
}

// TestAggregateAllScores_SQLite verifies aggregation over cross-run data.
func TestAggregateAllScores_SQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_agg.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE _smithers_scorer_results (
		id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
		scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
		meta_json TEXT, input_json TEXT, output_json TEXT,
		latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER)`)
	require.NoError(t, err)
	// Two scorers, two runs.
	rows := []struct {
		id, runID, scorerID, scorerName, source string
		score                                   float64
		ts                                      int64
	}{
		{"s1", "run-a", "relevancy", "Relevancy", "live", 0.90, 100},
		{"s2", "run-a", "faithfulness", "Faithfulness", "live", 0.80, 200},
		{"s3", "run-b", "relevancy", "Relevancy", "live", 0.70, 300},
		{"s4", "run-b", "faithfulness", "Faithfulness", "live", 1.00, 400},
	}
	for _, r := range rows {
		_, err = db.Exec(`INSERT INTO _smithers_scorer_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.id, r.runID, "node-1", 0, 0,
			r.scorerID, r.scorerName, r.source, r.score,
			nil, nil, nil, nil, nil, r.ts, nil)
		require.NoError(t, err)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	aggs, err := c.AggregateAllScores(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, aggs, 2)

	// Find relevancy aggregate.
	var rel AggregateScore
	for _, a := range aggs {
		if a.ScorerID == "relevancy" {
			rel = a
		}
	}
	assert.Equal(t, 2, rel.Count)
	assert.InDelta(t, 0.80, rel.Mean, 0.01) // (0.90+0.70)/2
}

// --- Approve ---

func TestApprove_Exec(t *testing.T) {
	var capturedArgs []string
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return json.Marshal(map[string]any{"runId": "run-1", "ok": true})
	})

	err := c.Approve(context.Background(), "run-1", "node-a", 2, "looks good")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"approve", "run-1",
		"--node", "node-a",
		"--iteration", "2",
		"--format", "json",
		"--note", "looks good",
	}, capturedArgs)
}

func TestApprove_Exec_NoNote(t *testing.T) {
	var capturedArgs []string
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return nil, nil
	})

	err := c.Approve(context.Background(), "run-1", "node-a", 1, "")
	require.NoError(t, err)
	// --note should be omitted when note is empty
	assert.NotContains(t, capturedArgs, "--note")
}

func TestApprove_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, &ExecError{Command: "approve run-1", Stderr: "run not active", Exit: 1}
	})

	err := c.Approve(context.Background(), "run-1", "node-a", 1, "")
	require.Error(t, err)
	var execErr *ExecError
	require.True(t, errors.As(err, &execErr))
	assert.Equal(t, 1, execErr.Exit)
}

// --- Deny ---

func TestDeny_Exec(t *testing.T) {
	var capturedArgs []string
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return json.Marshal(map[string]any{"runId": "run-2", "ok": true})
	})

	err := c.Deny(context.Background(), "run-2", "node-b", 3, "insufficient quality")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"deny", "run-2",
		"--node", "node-b",
		"--iteration", "3",
		"--format", "json",
		"--reason", "insufficient quality",
	}, capturedArgs)
}

func TestDeny_Exec_NoReason(t *testing.T) {
	var capturedArgs []string
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return nil, nil
	})

	err := c.Deny(context.Background(), "run-2", "node-b", 1, "")
	require.NoError(t, err)
	// --reason should be omitted when reason is empty
	assert.NotContains(t, capturedArgs, "--reason")
}

func TestDeny_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, &ExecError{Command: "deny run-2", Stderr: "approval not found", Exit: 1}
	})

	err := c.Deny(context.Background(), "run-2", "node-b", 1, "")
	require.Error(t, err)
	var execErr *ExecError
	require.True(t, errors.As(err, &execErr))
	assert.Equal(t, 1, execErr.Exit)
}

// --- JSONParseError classification for existing helpers ---

func TestParseSQLResultJSON_JSONParseError(t *testing.T) {
	// parseSQLResultJSON should return *JSONParseError for unrecognised output.
	_, err := parseSQLResultJSON([]byte(`not valid json`))
	require.Error(t, err)
	var parseErr *JSONParseError
	require.True(t, errors.As(err, &parseErr), "expected *JSONParseError, got %T: %v", err, err)
	assert.Equal(t, "sql", parseErr.Command)
}

func TestListCrons_Exec_JSONParseError(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("{invalid}"), nil
	})
	_, err := c.ListCrons(context.Background())
	require.Error(t, err)
	var parseErr *JSONParseError
	require.True(t, errors.As(err, &parseErr), "expected *JSONParseError, got %T: %v", err, err)
	assert.Equal(t, "cron list", parseErr.Command)
}

func TestListPendingApprovals_NoExecFallback(t *testing.T) {
	// The exec fallback for approvals returns nil (no CLI command exists).
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		t.Fatal("exec should not be called for approvals")
		return nil, nil
	})
	approvals, err := c.ListPendingApprovals(context.Background())
	require.NoError(t, err)
	assert.Nil(t, approvals)
}

func TestListTickets_NoDirectory(t *testing.T) {
	// When .smithers/tickets/ doesn't exist, return nil (empty list)
	dir := t.TempDir()
	c := NewClient(WithWorkingDir(dir))
	tickets, err := c.ListTickets(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tickets)
}

// TestParseApprovalsJSON_JSONParseError verifies that malformed output yields *JSONParseError.
func TestParseApprovalsJSON_JSONParseError(t *testing.T) {
	_, err := parseApprovalsJSON([]byte(`{broken`))
	require.Error(t, err)
	var parseErr *JSONParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Equal(t, "approval list", parseErr.Command)
}

// TestParseCronSchedulesJSON_JSONParseError verifies that malformed cron output
// yields *JSONParseError.
func TestParseCronSchedulesJSON_JSONParseError(t *testing.T) {
	_, err := parseCronSchedulesJSON([]byte(`not-json`))
	require.Error(t, err)
	var parseErr *JSONParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Equal(t, "cron list", parseErr.Command)
}

// --- HTTPError ---

// TestHTTPError_401 verifies that httpGetJSON returns *HTTPError with StatusCode 401
// when the server responds with 401, and IsUnauthorized returns true.
func TestHTTPError_401(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	})

	// Call httpGetJSON directly — domain methods may fall through to SQLite/exec.
	var out interface{}
	err := c.httpGetJSON(context.Background(), "/some/path", &out)
	require.Error(t, err)

	var he *HTTPError
	require.True(t, errors.As(err, &he), "expected *HTTPError, got: %T %v", err, err)
	assert.Equal(t, http.StatusUnauthorized, he.StatusCode)
	assert.True(t, IsUnauthorized(err))
}

// TestHTTPError_401_Post verifies that httpPostJSON returns *HTTPError with StatusCode 401.
func TestHTTPError_401_Post(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	})

	var out interface{}
	err := c.httpPostJSON(context.Background(), "/some/path", nil, &out)
	require.Error(t, err)

	var he *HTTPError
	require.True(t, errors.As(err, &he), "expected *HTTPError, got: %T %v", err, err)
	assert.Equal(t, http.StatusUnauthorized, he.StatusCode)
	assert.True(t, IsUnauthorized(err))
}

// TestHTTPError_503 verifies that httpGetJSON returns *HTTPError and
// invalidates the server availability cache on 5xx responses.
func TestHTTPError_503(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("service unavailable"))
	})

	// Pre-condition: server is marked as up.
	c.serverUp = true

	var out interface{}
	err := c.httpGetJSON(context.Background(), "/some/path", &out)
	require.Error(t, err)

	var he *HTTPError
	require.True(t, errors.As(err, &he))
	assert.Equal(t, http.StatusServiceUnavailable, he.StatusCode)

	// Server availability cache must have been invalidated.
	c.serverMu.RLock()
	up := c.serverUp
	checked := c.serverChecked
	c.serverMu.RUnlock()
	assert.False(t, up, "server should be marked down after 503")
	assert.True(t, checked.IsZero(), "serverChecked should be zero after invalidation")
}

// TestInvalidateServerCache verifies that a transport error on httpGetJSON
// resets serverUp and serverChecked.
func TestInvalidateServerCache(t *testing.T) {
	// Create a client pointing at a URL that refuses connections.
	c := NewClient(
		WithAPIURL("http://127.0.0.1:1"), // port 1 is almost always refused
	)
	c.serverUp = true
	c.serverChecked = c.serverChecked.Add(1) // make it non-zero

	// Calling httpGetJSON directly to exercise the transport-error path.
	err := c.httpGetJSON(context.Background(), "/some/path", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrServerUnavailable)

	c.serverMu.RLock()
	up := c.serverUp
	checked := c.serverChecked
	c.serverMu.RUnlock()
	assert.False(t, up, "serverUp should be false after transport error")
	assert.True(t, checked.IsZero(), "serverChecked should be zero to force re-probe")
}

// TestIsUnauthorized verifies the IsUnauthorized helper function.
func TestIsUnauthorized(t *testing.T) {
	assert.True(t, IsUnauthorized(&HTTPError{StatusCode: http.StatusUnauthorized}))
	assert.False(t, IsUnauthorized(&HTTPError{StatusCode: http.StatusForbidden}))
	assert.False(t, IsUnauthorized(errors.New("some other error")))
	assert.False(t, IsUnauthorized(nil))
}

// TestIsServerUnavailable verifies the IsServerUnavailable helper function.
func TestIsServerUnavailable(t *testing.T) {
	assert.True(t, IsServerUnavailable(ErrServerUnavailable))
	assert.True(t, IsServerUnavailable(fmt.Errorf("wrapped: %w", ErrServerUnavailable)))
	assert.False(t, IsServerUnavailable(errors.New("other error")))
}

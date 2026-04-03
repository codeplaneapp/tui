package smithers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"sql", "--query", "SELECT 1", "--format", "json"}, args)
		return json.Marshal([]map[string]interface{}{
			{"val": float64(42)},
		})
	})

	result, err := c.ExecuteSQL(context.Background(), "SELECT 1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"val"}, result.Columns)
	assert.Len(t, result.Rows, 1)
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

func TestToggleCron_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "cron", args[0])
		assert.Equal(t, "toggle", args[1])
		assert.Equal(t, "c1", args[2])
		assert.Equal(t, "--enabled", args[3])
		assert.Equal(t, "true", args[4])
		return nil, nil
	})

	err := c.ToggleCron(context.Background(), "c1", true)
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
	assert.Len(t, agents, 6)
	assert.Equal(t, "claude-code", agents[0].ID)
}

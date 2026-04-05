package smithers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ListTables ---

func TestListTables_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sql/tables", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, []TableInfo{
			{Name: "_smithers_runs", Type: "table", RowCount: 42},
			{Name: "_smithers_nodes", Type: "table", RowCount: 100},
		})
	})

	tables, err := c.ListTables(context.Background())
	require.NoError(t, err)
	require.Len(t, tables, 2)
	assert.Equal(t, "_smithers_runs", tables[0].Name)
	assert.Equal(t, "table", tables[0].Type)
	assert.Equal(t, int64(42), tables[0].RowCount)
}

func TestListTables_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "sql", args[0])
		assert.Equal(t, "--query", args[1])
		assert.Contains(t, args[2], "sqlite_master")
		assert.Equal(t, "--format", args[3])
		assert.Equal(t, "json", args[4])

		return json.Marshal([]map[string]interface{}{
			{"name": "_smithers_crons", "type": "table"},
			{"name": "_smithers_memory_facts", "type": "table"},
		})
	})

	tables, err := c.ListTables(context.Background())
	require.NoError(t, err)
	require.Len(t, tables, 2)
	assert.Equal(t, "_smithers_crons", tables[0].Name)
}

func TestListTables_Exec_SQLResultFormat(t *testing.T) {
	// Some smithers CLI versions return columnar SQLResult format.
	c := newExecClient(func(_ context.Context, _ ...string) ([]byte, error) {
		return json.Marshal(SQLResult{
			Columns: []string{"name", "type"},
			Rows: [][]interface{}{
				{"_smithers_runs", "table"},
				{"_smithers_events", "table"},
			},
		})
	})

	tables, err := c.ListTables(context.Background())
	require.NoError(t, err)
	require.Len(t, tables, 2)
	assert.Equal(t, "_smithers_runs", tables[0].Name)
	assert.Equal(t, "_smithers_events", tables[1].Name)
}

// --- GetTableSchema ---

func TestGetTableSchema_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sql/schema/_smithers_runs", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		dflt := "pending"
		writeEnvelope(t, w, TableSchema{
			TableName: "_smithers_runs",
			Columns: []Column{
				{CID: 0, Name: "id", Type: "TEXT", NotNull: true, PrimaryKey: true},
				{CID: 1, Name: "status", Type: "TEXT", NotNull: true, DefaultValue: &dflt},
				{CID: 2, Name: "created_at_ms", Type: "INTEGER", NotNull: true},
			},
		})
	})

	schema, err := c.GetTableSchema(context.Background(), "_smithers_runs")
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, "_smithers_runs", schema.TableName)
	require.Len(t, schema.Columns, 3)
	assert.Equal(t, "id", schema.Columns[0].Name)
	assert.True(t, schema.Columns[0].PrimaryKey)
	assert.Equal(t, "TEXT", schema.Columns[0].Type)
}

func TestGetTableSchema_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "sql", args[0])
		assert.Equal(t, "--query", args[1])
		assert.Contains(t, args[2], "PRAGMA table_info")
		assert.Contains(t, args[2], "_smithers_nodes")
		return json.Marshal([]map[string]interface{}{
			{"cid": float64(0), "name": "id", "type": "TEXT", "notnull": float64(1), "dflt_value": nil, "pk": float64(1)},
			{"cid": float64(1), "name": "run_id", "type": "TEXT", "notnull": float64(1), "dflt_value": nil, "pk": float64(0)},
			{"cid": float64(2), "name": "status", "type": "TEXT", "notnull": float64(0), "dflt_value": nil, "pk": float64(0)},
		})
	})

	schema, err := c.GetTableSchema(context.Background(), "_smithers_nodes")
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, "_smithers_nodes", schema.TableName)
	require.Len(t, schema.Columns, 3)
	assert.Equal(t, "id", schema.Columns[0].Name)
	assert.True(t, schema.Columns[0].PrimaryKey)
	assert.False(t, schema.Columns[2].PrimaryKey)
}

func TestGetTableSchema_EmptyName(t *testing.T) {
	c := NewClient()
	_, err := c.GetTableSchema(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tableName must not be empty")
}

// --- GetTokenUsageMetrics ---

func TestGetTokenUsageMetrics_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/metrics/tokens", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, TokenMetrics{
			TotalInputTokens:  1000,
			TotalOutputTokens: 500,
			TotalTokens:       1500,
			CacheReadTokens:   200,
			CacheWriteTokens:  100,
		})
	})

	m, err := c.GetTokenUsageMetrics(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, int64(1000), m.TotalInputTokens)
	assert.Equal(t, int64(500), m.TotalOutputTokens)
	assert.Equal(t, int64(1500), m.TotalTokens)
}

func TestGetTokenUsageMetrics_HTTP_WithFilters(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "runId=run-abc")
		assert.Contains(t, r.URL.RawQuery, "groupBy=day")
		writeEnvelope(t, w, TokenMetrics{TotalInputTokens: 777})
	})

	filters := MetricsFilter{RunID: "run-abc", GroupBy: "day"}
	m, err := c.GetTokenUsageMetrics(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, int64(777), m.TotalInputTokens)
}

func TestGetTokenUsageMetrics_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "metrics", args[0])
		assert.Equal(t, "token-usage", args[1])
		assert.Contains(t, args, "--format")
		assert.Contains(t, args, "json")
		return json.Marshal(TokenMetrics{
			TotalInputTokens:  2000,
			TotalOutputTokens: 1000,
			TotalTokens:       3000,
		})
	})

	m, err := c.GetTokenUsageMetrics(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, int64(3000), m.TotalTokens)
}

func TestGetTokenUsageMetrics_Exec_WithFilters(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--run")
		assert.Contains(t, args, "run-xyz")
		assert.Contains(t, args, "--workflow")
		assert.Contains(t, args, "review.tsx")
		return json.Marshal(TokenMetrics{TotalInputTokens: 100})
	})

	filters := MetricsFilter{RunID: "run-xyz", WorkflowPath: "review.tsx"}
	m, err := c.GetTokenUsageMetrics(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, int64(100), m.TotalInputTokens)
}

// --- GetLatencyMetrics ---

func TestGetLatencyMetrics_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/metrics/latency", r.URL.Path)
		writeEnvelope(t, w, LatencyMetrics{
			Count:  10,
			MeanMs: 250.5,
			MinMs:  100.0,
			MaxMs:  800.0,
			P50Ms:  230.0,
			P95Ms:  750.0,
		})
	})

	m, err := c.GetLatencyMetrics(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, 10, m.Count)
	assert.InDelta(t, 250.5, m.MeanMs, 0.001)
	assert.InDelta(t, 750.0, m.P95Ms, 0.001)
}

func TestGetLatencyMetrics_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "metrics", args[0])
		assert.Equal(t, "latency", args[1])
		return json.Marshal(LatencyMetrics{
			Count:  5,
			MeanMs: 120.0,
			P50Ms:  110.0,
			P95Ms:  180.0,
		})
	})

	m, err := c.GetLatencyMetrics(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	assert.Equal(t, 5, m.Count)
	assert.InDelta(t, 120.0, m.MeanMs, 0.001)
}

func TestGetLatencyMetrics_Exec_WithFilters(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--node")
		assert.Contains(t, args, "node-1")
		assert.Contains(t, args, "--start")
		assert.Contains(t, args, "1000")
		assert.Contains(t, args, "--end")
		assert.Contains(t, args, "2000")
		return json.Marshal(LatencyMetrics{Count: 3, MeanMs: 90.0})
	})

	filters := MetricsFilter{NodeID: "node-1", StartMs: 1000, EndMs: 2000}
	m, err := c.GetLatencyMetrics(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, 3, m.Count)
}

// --- GetCostTracking ---

func TestGetCostTracking_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/metrics/cost", r.URL.Path)
		writeEnvelope(t, w, CostReport{
			TotalCostUSD:  0.042,
			InputCostUSD:  0.030,
			OutputCostUSD: 0.012,
			RunCount:      7,
		})
	})

	report, err := c.GetCostTracking(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.InDelta(t, 0.042, report.TotalCostUSD, 0.0001)
	assert.Equal(t, 7, report.RunCount)
}

func TestGetCostTracking_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "metrics", args[0])
		assert.Equal(t, "cost", args[1])
		return json.Marshal(CostReport{
			TotalCostUSD: 0.15,
			RunCount:     3,
		})
	})

	report, err := c.GetCostTracking(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	assert.InDelta(t, 0.15, report.TotalCostUSD, 0.0001)
	assert.Equal(t, 3, report.RunCount)
}

func TestGetCostTracking_Exec_WithFilters(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Contains(t, args, "--run")
		assert.Contains(t, args, "run-123")
		assert.Contains(t, args, "--group-by")
		assert.Contains(t, args, "workflow")
		return json.Marshal(CostReport{TotalCostUSD: 0.01, RunCount: 1})
	})

	filters := MetricsFilter{RunID: "run-123", GroupBy: "workflow"}
	report, err := c.GetCostTracking(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, 1, report.RunCount)
}

// --- computeLatencyMetrics unit tests ---

func TestComputeLatencyMetrics_Empty(t *testing.T) {
	m := computeLatencyMetrics(nil)
	assert.Equal(t, 0, m.Count)
	assert.Equal(t, 0.0, m.MeanMs)
}

func TestComputeLatencyMetrics_SingleValue(t *testing.T) {
	m := computeLatencyMetrics([]float64{300.0})
	assert.Equal(t, 1, m.Count)
	assert.Equal(t, 300.0, m.MeanMs)
	assert.Equal(t, 300.0, m.MinMs)
	assert.Equal(t, 300.0, m.MaxMs)
	assert.Equal(t, 300.0, m.P50Ms)
	assert.Equal(t, 300.0, m.P95Ms)
}

func TestComputeLatencyMetrics_MultipleValues(t *testing.T) {
	// Values in ascending order (as they arrive from ORDER BY).
	durations := []float64{100, 150, 200, 250, 300, 400, 500, 600, 700, 1000}
	m := computeLatencyMetrics(durations)
	assert.Equal(t, 10, m.Count)
	assert.Equal(t, 100.0, m.MinMs)
	assert.Equal(t, 1000.0, m.MaxMs)
	// Mean = (100+150+200+250+300+400+500+600+700+1000) / 10 = 4200/10 = 420
	assert.InDelta(t, 420.0, m.MeanMs, 0.001)
	// P50 of 10 values: interpolated between index 4 (300) and index 5 (400) => 350
	assert.InDelta(t, 350.0, m.P50Ms, 0.001)
	// P95 of 10 values: idx = 0.95*9 = 8.55 → interp between 700 and 1000: 700 + 0.55*300 = 865
	assert.InDelta(t, 865.0, m.P95Ms, 0.001)
}

func TestComputeLatencyMetrics_UnsortedInput(t *testing.T) {
	// Must produce same result regardless of input order.
	m := computeLatencyMetrics([]float64{500, 100, 300})
	assert.Equal(t, 100.0, m.MinMs)
	assert.Equal(t, 500.0, m.MaxMs)
	assert.InDelta(t, 300.0, m.MeanMs, 0.001)
}

// --- percentile unit tests ---

func TestPercentile_Empty(t *testing.T) {
	assert.Equal(t, 0.0, percentile(nil, 0.5))
}

func TestPercentile_SingleElement(t *testing.T) {
	assert.Equal(t, 42.0, percentile([]float64{42.0}, 0.99))
}

func TestPercentile_Median(t *testing.T) {
	// Odd number: [1,2,3] → p50 = 2
	assert.InDelta(t, 2.0, percentile([]float64{1, 2, 3}, 0.5), 0.001)
}

func TestPercentile_P95(t *testing.T) {
	// 100 values: p95 = value at index 94 (0-based)
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	// idx = 0.95 * 99 = 94.05 → interp between 95 and 96: 95 + 0.05 = 95.05
	assert.InDelta(t, 95.05, percentile(vals, 0.95), 0.001)
}

// --- buildMetricsPath unit tests ---

func TestBuildMetricsPath_NoFilters(t *testing.T) {
	assert.Equal(t, "/metrics/tokens", buildMetricsPath("/metrics/tokens", MetricsFilter{}))
}

func TestBuildMetricsPath_WithFilters(t *testing.T) {
	path := buildMetricsPath("/metrics/tokens", MetricsFilter{
		RunID:   "run-1",
		GroupBy: "day",
	})
	assert.Contains(t, path, "runId=run-1")
	assert.Contains(t, path, "groupBy=day")
}

func TestBuildMetricsPath_WithAllFilters(t *testing.T) {
	path := buildMetricsPath("/metrics/cost", MetricsFilter{
		RunID:        "run-abc",
		NodeID:       "node-1",
		WorkflowPath: "deploy.tsx",
		StartMs:      1000,
		EndMs:        2000,
		GroupBy:      "workflow",
	})
	assert.Contains(t, path, "runId=run-abc")
	assert.Contains(t, path, "nodeId=node-1")
	assert.Contains(t, path, "workflowPath=deploy.tsx")
	assert.Contains(t, path, "startMs=1000")
	assert.Contains(t, path, "endMs=2000")
	assert.Contains(t, path, "groupBy=workflow")
}

// --- metricsExecArgs unit tests ---

func TestMetricsExecArgs_NoFilters(t *testing.T) {
	args := metricsExecArgs("token-usage", MetricsFilter{})
	assert.Equal(t, []string{"metrics", "token-usage", "--format", "json"}, args)
}

func TestMetricsExecArgs_WithRunFilter(t *testing.T) {
	args := metricsExecArgs("latency", MetricsFilter{RunID: "run-1"})
	assert.Contains(t, args, "--run")
	assert.Contains(t, args, "run-1")
}

func TestMetricsExecArgs_WithGroupBy(t *testing.T) {
	args := metricsExecArgs("cost", MetricsFilter{GroupBy: "day"})
	assert.Contains(t, args, "--group-by")
	assert.Contains(t, args, "day")
}

func TestMetricsExecArgs_WithTimeRange(t *testing.T) {
	args := metricsExecArgs("latency", MetricsFilter{StartMs: 500, EndMs: 1500})
	assert.Contains(t, args, "--start")
	assert.Contains(t, args, "500")
	assert.Contains(t, args, "--end")
	assert.Contains(t, args, "1500")
}

// --- parseTableInfoJSON unit tests ---

func TestParseTableInfoJSON_ArrayFormat(t *testing.T) {
	data, _ := json.Marshal([]map[string]interface{}{
		{"name": "_smithers_runs", "type": "table"},
		{"name": "_smithers_crons", "type": "table"},
	})
	tables, err := parseTableInfoJSON(data)
	require.NoError(t, err)
	require.Len(t, tables, 2)
	assert.Equal(t, "_smithers_runs", tables[0].Name)
	assert.Equal(t, "table", tables[0].Type)
}

func TestParseTableInfoJSON_SQLResultFormat(t *testing.T) {
	data, _ := json.Marshal(SQLResult{
		Columns: []string{"name", "type"},
		Rows: [][]interface{}{
			{"_smithers_memory_facts", "table"},
		},
	})
	tables, err := parseTableInfoJSON(data)
	require.NoError(t, err)
	require.Len(t, tables, 1)
	assert.Equal(t, "_smithers_memory_facts", tables[0].Name)
}

func TestParseTableInfoJSON_InvalidJSON(t *testing.T) {
	_, err := parseTableInfoJSON([]byte("not json"))
	require.Error(t, err)
}

// --- parseTableColumnsJSON unit tests ---

func TestParseTableColumnsJSON(t *testing.T) {
	dflt := "pending"
	data, _ := json.Marshal([]map[string]interface{}{
		{"cid": float64(0), "name": "id", "type": "TEXT", "notnull": float64(1), "dflt_value": nil, "pk": float64(1)},
		{"cid": float64(1), "name": "status", "type": "TEXT", "notnull": float64(1), "dflt_value": dflt, "pk": float64(0)},
	})

	cols, err := parseTableColumnsJSON(data)
	require.NoError(t, err)
	require.Len(t, cols, 2)

	assert.Equal(t, 0, cols[0].CID)
	assert.Equal(t, "id", cols[0].Name)
	assert.True(t, cols[0].PrimaryKey)
	assert.True(t, cols[0].NotNull)
	assert.Nil(t, cols[0].DefaultValue)

	assert.Equal(t, 1, cols[1].CID)
	assert.Equal(t, "status", cols[1].Name)
	assert.False(t, cols[1].PrimaryKey)
}

func TestParseTableColumnsJSON_InvalidJSON(t *testing.T) {
	_, err := parseTableColumnsJSON([]byte("oops"))
	require.Error(t, err)
}

// --- quoteIdentifier unit tests ---

func TestQuoteIdentifier_Simple(t *testing.T) {
	assert.Equal(t, `"_smithers_runs"`, quoteIdentifier("_smithers_runs"))
}

func TestQuoteIdentifier_WithDoubleQuote(t *testing.T) {
	// Double-quotes inside identifiers must be escaped.
	assert.Equal(t, `"tab""le"`, quoteIdentifier(`tab"le`))
}

// --- buildTokenMetricsQuery unit tests ---

func TestBuildTokenMetricsQuery_NoFilters(t *testing.T) {
	q, args := buildTokenMetricsQuery(MetricsFilter{})
	assert.Contains(t, q, "_smithers_chat_attempts")
	assert.NotContains(t, q, "WHERE")
	assert.Empty(t, args)
}

func TestBuildTokenMetricsQuery_WithRunID(t *testing.T) {
	q, args := buildTokenMetricsQuery(MetricsFilter{RunID: "run-1"})
	assert.Contains(t, q, "WHERE")
	assert.Contains(t, q, "run_id = ?")
	require.Len(t, args, 1)
	assert.Equal(t, "run-1", args[0])
}

func TestBuildTokenMetricsQuery_WithTimeRange(t *testing.T) {
	q, args := buildTokenMetricsQuery(MetricsFilter{StartMs: 1000, EndMs: 2000})
	assert.Contains(t, q, "started_at_ms >= ?")
	assert.Contains(t, q, "started_at_ms <= ?")
	require.Len(t, args, 2)
	assert.Equal(t, int64(1000), args[0])
	assert.Equal(t, int64(2000), args[1])
}

// --- buildRunCountQuery unit tests ---

func TestBuildRunCountQuery_NoFilters(t *testing.T) {
	q, args := buildRunCountQuery(MetricsFilter{})
	assert.Contains(t, q, "COUNT(DISTINCT run_id)")
	assert.NotContains(t, q, "WHERE")
	assert.Empty(t, args)
}

func TestBuildRunCountQuery_WithRunID(t *testing.T) {
	q, args := buildRunCountQuery(MetricsFilter{RunID: "run-abc"})
	assert.Contains(t, q, "WHERE")
	assert.Contains(t, q, "run_id = ?")
	assert.Len(t, args, 1)
}

// --- buildLatencyQuery unit tests ---

func TestBuildLatencyQuery_NoFilters(t *testing.T) {
	q, args := buildLatencyQuery(MetricsFilter{})
	assert.Contains(t, q, "_smithers_nodes")
	assert.Contains(t, q, "duration_ms IS NOT NULL")
	assert.Empty(t, args)
}

func TestBuildLatencyQuery_WithWorkflowPath(t *testing.T) {
	q, args := buildLatencyQuery(MetricsFilter{WorkflowPath: "review.tsx"})
	assert.Contains(t, q, "workflow_path = ?")
	require.Len(t, args, 1)
	assert.Equal(t, "review.tsx", args[0])
}

// --- cost calculation unit tests ---

func TestCostPerTokenConstants(t *testing.T) {
	// Sanity-check: 1M input tokens should cost exactly $3.
	cost := float64(1_000_000) / 1_000_000 * costPerMInputTokens
	assert.InDelta(t, 3.0, cost, 0.001)

	// 1M output tokens should cost exactly $15.
	cost = float64(1_000_000) / 1_000_000 * costPerMOutputTokens
	assert.InDelta(t, 15.0, cost, 0.001)
}

func TestCostCalculation_ZeroTokens(t *testing.T) {
	// Zero tokens → zero cost.
	inputCost := float64(0) / 1_000_000 * costPerMInputTokens
	outputCost := float64(0) / 1_000_000 * costPerMOutputTokens
	assert.Equal(t, 0.0, inputCost+outputCost)
}

func TestCostCalculation_SmallRun(t *testing.T) {
	// 10k input + 2k output = (10000/1M * 3) + (2000/1M * 15)
	//   = 0.03 + 0.03 = 0.06
	inputCost := float64(10_000) / 1_000_000 * costPerMInputTokens
	outputCost := float64(2_000) / 1_000_000 * costPerMOutputTokens
	assert.InDelta(t, 0.03, inputCost, 0.0001)
	assert.InDelta(t, 0.03, outputCost, 0.0001)
	assert.InDelta(t, 0.06, inputCost+outputCost, 0.0001)
}

// --- HTTP filter passthrough ---

func TestGetTokenUsageMetrics_HTTP_AllFilters(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.RawQuery
		assert.Contains(t, q, fmt.Sprintf("startMs=%d", int64(1000)))
		assert.Contains(t, q, fmt.Sprintf("endMs=%d", int64(9000)))
		assert.Contains(t, q, "workflowPath=deploy.tsx")
		writeEnvelope(t, w, TokenMetrics{TotalTokens: 99})
	})

	filters := MetricsFilter{
		WorkflowPath: "deploy.tsx",
		StartMs:      1000,
		EndMs:        9000,
	}
	m, err := c.GetTokenUsageMetrics(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, int64(99), m.TotalTokens)
}

func TestGetLatencyMetrics_HTTP_WithRunFilter(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "runId=run-999")
		writeEnvelope(t, w, LatencyMetrics{Count: 4, MeanMs: 200})
	})

	m, err := c.GetLatencyMetrics(context.Background(), MetricsFilter{RunID: "run-999"})
	require.NoError(t, err)
	assert.Equal(t, 4, m.Count)
}

func TestGetCostTracking_HTTP_WithFilters(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "groupBy=day")
		writeEnvelope(t, w, CostReport{TotalCostUSD: 1.23, RunCount: 10})
	})

	report, err := c.GetCostTracking(context.Background(), MetricsFilter{GroupBy: "day"})
	require.NoError(t, err)
	assert.InDelta(t, 1.23, report.TotalCostUSD, 0.001)
	assert.Equal(t, 10, report.RunCount)
}

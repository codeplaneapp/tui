package smithers

// systems.go — Systems and Analytics API client methods.
//
// Provides methods consumed by:
//   - SQL Browser view (internal/ui/views/sqlbrowser.go) — PRD §6.11
//   - Scores/ROI Dashboard (internal/ui/views/scores.go)   — PRD §6.14
//
// Transport strategy: HTTP-primary → SQLite fallback → exec.Command fallback.
// ExecuteSQL, GetScores, ListCrons, etc. live in client.go.  This file adds
// the systems/analytics layer: table introspection and aggregate metrics.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// --- SQL Browser: table introspection ---

// ListTables returns the tables available in the Smithers SQLite database.
// Used by the SQL Browser table sidebar (PRD §6.11, feature SQL_TABLE_SIDEBAR).
//
// Transport cascade: HTTP GET /sql/tables → SQLite PRAGMA → exec.
func (c *Client) ListTables(ctx context.Context) ([]TableInfo, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var tables []TableInfo
		if err := c.httpGetJSON(ctx, "/sql/tables", &tables); err == nil {
			return tables, nil
		}
	}

	// 2. Try direct SQLite via PRAGMA.
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT name, type FROM sqlite_master
			 WHERE type IN ('table','view')
			   AND name NOT LIKE 'sqlite_%'
			 ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var tables []TableInfo
		for rows.Next() {
			var ti TableInfo
			if err := rows.Scan(&ti.Name, &ti.Type); err != nil {
				return nil, err
			}
			// Best-effort row count; ignore errors on individual tables.
			countRow := c.db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdentifier(ti.Name))
			_ = countRow.Scan(&ti.RowCount)
			tables = append(tables, ti)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return tables, nil
	}

	// 3. Fall back to exec.
	out, err := c.execSmithers(ctx, "sql", "--query",
		"SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name",
		"--format", "json")
	if err != nil {
		return nil, err
	}
	return parseTableInfoJSON(out)
}

// GetTableSchema returns the column schema for a named table.
// Used by the SQL Browser to show column types and constraints (PRD §6.11).
//
// Transport cascade: HTTP GET /sql/schema/{tableName} → SQLite PRAGMA → exec.
func (c *Client) GetTableSchema(ctx context.Context, tableName string) (*TableSchema, error) {
	if tableName == "" {
		return nil, fmt.Errorf("tableName must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var schema TableSchema
		if err := c.httpGetJSON(ctx, "/sql/schema/"+tableName, &schema); err == nil {
			return &schema, nil
		}
	}

	// 2. Try direct SQLite via PRAGMA table_info.
	if c.db != nil {
		rows, err := c.queryDB(ctx, "PRAGMA table_info("+quoteIdentifier(tableName)+")")
		if err != nil {
			return nil, err
		}
		cols, err := scanTableColumns(rows)
		if err != nil {
			return nil, err
		}
		return &TableSchema{TableName: tableName, Columns: cols}, nil
	}

	// 3. Fall back to exec.
	out, err := c.execSmithers(ctx, "sql", "--query",
		"PRAGMA table_info("+quoteIdentifier(tableName)+")",
		"--format", "json")
	if err != nil {
		return nil, err
	}
	cols, err := parseTableColumnsJSON(out)
	if err != nil {
		return nil, err
	}
	return &TableSchema{TableName: tableName, Columns: cols}, nil
}

// --- Scores / ROI Dashboard: metrics ---

// GetTokenUsageMetrics returns aggregated token usage statistics.
// Source: _smithers_chat_attempts table.
// Maps to SCORES_TOKEN_USAGE_METRICS feature flag (PRD §6.14).
//
// Transport cascade: HTTP GET /metrics/tokens → SQLite → exec.
func (c *Client) GetTokenUsageMetrics(ctx context.Context, filters MetricsFilter) (*TokenMetrics, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		path := buildMetricsPath("/metrics/tokens", filters)
		var m TokenMetrics
		if err := c.httpGetJSON(ctx, path, &m); err == nil {
			return &m, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		return queryTokenMetricsSQLite(ctx, c, filters)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, metricsExecArgs("token-usage", filters)...)
	if err != nil {
		return nil, err
	}
	return parseTokenMetricsJSON(out)
}

// GetLatencyMetrics returns aggregated node execution latency statistics.
// Source: _smithers_nodes table (duration_ms column).
// Maps to SCORES_LATENCY_METRICS feature flag (PRD §6.14).
//
// Transport cascade: HTTP GET /metrics/latency → SQLite → exec.
func (c *Client) GetLatencyMetrics(ctx context.Context, filters MetricsFilter) (*LatencyMetrics, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		path := buildMetricsPath("/metrics/latency", filters)
		var m LatencyMetrics
		if err := c.httpGetJSON(ctx, path, &m); err == nil {
			return &m, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		return queryLatencyMetricsSQLite(ctx, c, filters)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, metricsExecArgs("latency", filters)...)
	if err != nil {
		return nil, err
	}
	return parseLatencyMetricsJSON(out)
}

// GetCostTracking returns estimated cost information for runs.
// Costs are computed from token counts multiplied by per-model pricing constants.
// Maps to SCORES_COST_TRACKING feature flag (PRD §6.14).
//
// Transport cascade: HTTP GET /metrics/cost → SQLite → exec.
func (c *Client) GetCostTracking(ctx context.Context, filters MetricsFilter) (*CostReport, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		path := buildMetricsPath("/metrics/cost", filters)
		var r CostReport
		if err := c.httpGetJSON(ctx, path, &r); err == nil {
			return &r, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		return queryCostTrackingSQLite(ctx, c, filters)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, metricsExecArgs("cost", filters)...)
	if err != nil {
		return nil, err
	}
	return parseCostReportJSON(out)
}

// --- SQLite query helpers ---

// queryTokenMetricsSQLite computes token usage aggregates from _smithers_chat_attempts.
func queryTokenMetricsSQLite(ctx context.Context, c *Client, filters MetricsFilter) (*TokenMetrics, error) {
	q, args := buildTokenMetricsQuery(filters)
	rows, err := c.queryDB(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := &TokenMetrics{}
	for rows.Next() {
		var input, output, cacheRead, cacheWrite int64
		if err := rows.Scan(&input, &output, &cacheRead, &cacheWrite); err != nil {
			return nil, err
		}
		m.TotalInputTokens += input
		m.TotalOutputTokens += output
		m.CacheReadTokens += cacheRead
		m.CacheWriteTokens += cacheWrite
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m.TotalTokens = m.TotalInputTokens + m.TotalOutputTokens
	return m, nil
}

// buildTokenMetricsQuery builds the SQL query for token usage aggregation.
func buildTokenMetricsQuery(filters MetricsFilter) (string, []any) {
	var conditions []string
	var args []any

	if filters.RunID != "" {
		conditions = append(conditions, "run_id = ?")
		args = append(args, filters.RunID)
	}
	if filters.NodeID != "" {
		conditions = append(conditions, "node_id = ?")
		args = append(args, filters.NodeID)
	}
	if filters.StartMs > 0 {
		conditions = append(conditions, "started_at_ms >= ?")
		args = append(args, filters.StartMs)
	}
	if filters.EndMs > 0 {
		conditions = append(conditions, "started_at_ms <= ?")
		args = append(args, filters.EndMs)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	q := `SELECT
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_read_tokens), 0),
		COALESCE(SUM(cache_write_tokens), 0)
		FROM _smithers_chat_attempts` + where

	return q, args
}

// queryLatencyMetricsSQLite computes latency statistics from _smithers_nodes.
func queryLatencyMetricsSQLite(ctx context.Context, c *Client, filters MetricsFilter) (*LatencyMetrics, error) {
	q, args := buildLatencyQuery(filters)
	rows, err := c.queryDB(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		durations = append(durations, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return computeLatencyMetrics(durations), nil
}

// buildLatencyQuery builds the SQL query for latency data collection.
func buildLatencyQuery(filters MetricsFilter) (string, []any) {
	var conditions []string
	var args []any

	// Only include completed nodes with a duration.
	conditions = append(conditions, "duration_ms IS NOT NULL")

	if filters.RunID != "" {
		conditions = append(conditions, "run_id = ?")
		args = append(args, filters.RunID)
	}
	if filters.NodeID != "" {
		conditions = append(conditions, "id = ?")
		args = append(args, filters.NodeID)
	}
	if filters.WorkflowPath != "" {
		conditions = append(conditions, "workflow_path = ?")
		args = append(args, filters.WorkflowPath)
	}
	if filters.StartMs > 0 {
		conditions = append(conditions, "started_at_ms >= ?")
		args = append(args, filters.StartMs)
	}
	if filters.EndMs > 0 {
		conditions = append(conditions, "started_at_ms <= ?")
		args = append(args, filters.EndMs)
	}

	where := " WHERE " + strings.Join(conditions, " AND ")
	q := "SELECT CAST(duration_ms AS REAL) FROM _smithers_nodes" + where + " ORDER BY duration_ms"
	return q, args
}

// computeLatencyMetrics computes descriptive statistics from a sorted slice of durations.
// The input slice must already be sorted in ascending order.
func computeLatencyMetrics(durations []float64) *LatencyMetrics {
	n := len(durations)
	if n == 0 {
		return &LatencyMetrics{}
	}

	// durations arrives pre-sorted from ORDER BY duration_ms, but sort again
	// defensively so callers that build slices programmatically get correct stats.
	sort.Float64s(durations)

	sum := 0.0
	for _, d := range durations {
		sum += d
	}
	mean := sum / float64(n)

	p50 := percentile(durations, 0.50)
	p95 := percentile(durations, 0.95)

	return &LatencyMetrics{
		Count:  n,
		MeanMs: mean,
		MinMs:  durations[0],
		MaxMs:  durations[n-1],
		P50Ms:  p50,
		P95Ms:  p95,
	}
}

// percentile returns the p-th percentile (0.0–1.0) of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := p * float64(n-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	// Linear interpolation.
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// queryCostTrackingSQLite estimates cost from token usage in _smithers_chat_attempts.
// Cost model: $3/M input tokens, $15/M output tokens (Claude Sonnet approximate).
// The Smithers HTTP API applies the real per-model pricing; this is a best-effort
// estimate for the SQLite fallback path.
const (
	costPerMInputTokens  = 3.0  // USD per 1 million input tokens
	costPerMOutputTokens = 15.0 // USD per 1 million output tokens
)

func queryCostTrackingSQLite(ctx context.Context, c *Client, filters MetricsFilter) (*CostReport, error) {
	// Reuse the token metrics query — cost is derived from token counts.
	tokenMetrics, err := queryTokenMetricsSQLite(ctx, c, filters)
	if err != nil {
		return nil, err
	}

	// Count distinct runs in the result set.
	runCountQ, runArgs := buildRunCountQuery(filters)
	row := c.db.QueryRowContext(ctx, runCountQ, runArgs...)
	var runCount int
	if err := row.Scan(&runCount); err != nil {
		runCount = 0
	}

	inputCost := float64(tokenMetrics.TotalInputTokens) / 1_000_000 * costPerMInputTokens
	outputCost := float64(tokenMetrics.TotalOutputTokens) / 1_000_000 * costPerMOutputTokens
	return &CostReport{
		TotalCostUSD:  inputCost + outputCost,
		InputCostUSD:  inputCost,
		OutputCostUSD: outputCost,
		RunCount:      runCount,
	}, nil
}

// buildRunCountQuery builds the SQL query for counting distinct runs.
func buildRunCountQuery(filters MetricsFilter) (string, []any) {
	var conditions []string
	var args []any

	if filters.RunID != "" {
		conditions = append(conditions, "run_id = ?")
		args = append(args, filters.RunID)
	}
	if filters.StartMs > 0 {
		conditions = append(conditions, "started_at_ms >= ?")
		args = append(args, filters.StartMs)
	}
	if filters.EndMs > 0 {
		conditions = append(conditions, "started_at_ms <= ?")
		args = append(args, filters.EndMs)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return "SELECT COUNT(DISTINCT run_id) FROM _smithers_chat_attempts" + where, args
}

// --- Table schema helpers ---

// scanTableColumns reads rows from PRAGMA table_info() into a Column slice.
// PRAGMA table_info() returns: cid, name, type, notnull (int), dflt_value, pk (int).
func scanTableColumns(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}) ([]Column, error) {
	defer rows.Close()
	var cols []Column
	for rows.Next() {
		var col Column
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&col.CID, &col.Name, &col.Type, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		col.NotNull = notNull != 0
		col.DefaultValue = dfltValue
		col.PrimaryKey = pk > 0
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// parseTableColumnsJSON converts PRAGMA table_info JSON output into a Column slice.
// The exec output is an array of objects with keys: cid, name, type, notnull, dflt_value, pk.
func parseTableColumnsJSON(data []byte) ([]Column, error) {
	var rows []struct {
		CID          int     `json:"cid"`
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		NotNull      int     `json:"notnull"`
		DefaultValue *string `json:"dflt_value"`
		PK           int     `json:"pk"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parse table columns: %w", err)
	}
	cols := make([]Column, len(rows))
	for i, r := range rows {
		cols[i] = Column{
			CID:          r.CID,
			Name:         r.Name,
			Type:         r.Type,
			NotNull:      r.NotNull != 0,
			DefaultValue: r.DefaultValue,
			PrimaryKey:   r.PK > 0,
		}
	}
	return cols, nil
}

// parseTableInfoJSON parses exec output into a TableInfo slice.
// The exec output may be columnar SQLResult or an array of {name, type} objects.
func parseTableInfoJSON(data []byte) ([]TableInfo, error) {
	// Try direct array first.
	var rows []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &rows); err == nil && len(rows) > 0 {
		tables := make([]TableInfo, len(rows))
		for i, r := range rows {
			tables[i] = TableInfo{Name: r.Name, Type: r.Type}
		}
		return tables, nil
	}
	// Try SQLResult columnar format.
	var result SQLResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse table info: %w", err)
	}
	nameIdx, typeIdx := -1, -1
	for i, col := range result.Columns {
		switch col {
		case "name":
			nameIdx = i
		case "type":
			typeIdx = i
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("parse table info: no 'name' column in result")
	}
	tables := make([]TableInfo, 0, len(result.Rows))
	for _, row := range result.Rows {
		ti := TableInfo{}
		if nameIdx < len(row) {
			ti.Name = fmt.Sprintf("%v", row[nameIdx])
		}
		if typeIdx >= 0 && typeIdx < len(row) {
			ti.Type = fmt.Sprintf("%v", row[typeIdx])
		}
		tables = append(tables, ti)
	}
	return tables, nil
}

// --- HTTP metrics helpers ---

// buildMetricsPath appends query string filters to an HTTP path.
func buildMetricsPath(base string, f MetricsFilter) string {
	var parts []string
	if f.RunID != "" {
		parts = append(parts, "runId="+f.RunID)
	}
	if f.NodeID != "" {
		parts = append(parts, "nodeId="+f.NodeID)
	}
	if f.WorkflowPath != "" {
		parts = append(parts, "workflowPath="+f.WorkflowPath)
	}
	if f.StartMs > 0 {
		parts = append(parts, fmt.Sprintf("startMs=%d", f.StartMs))
	}
	if f.EndMs > 0 {
		parts = append(parts, fmt.Sprintf("endMs=%d", f.EndMs))
	}
	if f.GroupBy != "" {
		parts = append(parts, "groupBy="+f.GroupBy)
	}
	if len(parts) == 0 {
		return base
	}
	return base + "?" + strings.Join(parts, "&")
}

// metricsExecArgs builds smithers CLI args for a metrics subcommand.
func metricsExecArgs(subcommand string, f MetricsFilter) []string {
	args := []string{"metrics", subcommand, "--format", "json"}
	if f.RunID != "" {
		args = append(args, "--run", f.RunID)
	}
	if f.NodeID != "" {
		args = append(args, "--node", f.NodeID)
	}
	if f.WorkflowPath != "" {
		args = append(args, "--workflow", f.WorkflowPath)
	}
	if f.StartMs > 0 {
		args = append(args, "--start", fmt.Sprintf("%d", f.StartMs))
	}
	if f.EndMs > 0 {
		args = append(args, "--end", fmt.Sprintf("%d", f.EndMs))
	}
	if f.GroupBy != "" {
		args = append(args, "--group-by", f.GroupBy)
	}
	return args
}

// --- Parse helpers for exec output ---

func parseTokenMetricsJSON(data []byte) (*TokenMetrics, error) {
	var m TokenMetrics
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse token metrics: %w", err)
	}
	return &m, nil
}

func parseLatencyMetricsJSON(data []byte) (*LatencyMetrics, error) {
	var m LatencyMetrics
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse latency metrics: %w", err)
	}
	return &m, nil
}

func parseCostReportJSON(data []byte) (*CostReport, error) {
	var r CostReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse cost report: %w", err)
	}
	return &r, nil
}

// --- Identifier quoting ---

// quoteIdentifier wraps a SQLite identifier in double-quotes and escapes embedded
// double-quotes by doubling them, matching SQLite's identifier quoting rules.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

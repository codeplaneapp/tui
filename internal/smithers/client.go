package smithers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrServerUnavailable is returned when the Smithers HTTP server cannot be reached.
	ErrServerUnavailable = errors.New("smithers server unavailable")
	// ErrNoDatabase is returned when no SQLite database is available.
	ErrNoDatabase = errors.New("smithers database not available")
	// ErrNoTransport is returned when no transport (HTTP, SQLite, or exec) can handle the request.
	ErrNoTransport = errors.New("no smithers transport available")
)

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithAPIURL sets the Smithers HTTP API base URL.
func WithAPIURL(url string) ClientOption {
	return func(c *Client) { c.apiURL = url }
}

// WithAPIToken sets the bearer token for HTTP API authentication.
func WithAPIToken(token string) ClientOption {
	return func(c *Client) { c.apiToken = token }
}

// WithDBPath sets the path to the Smithers SQLite database for read-only fallback.
func WithDBPath(path string) ClientOption {
	return func(c *Client) { c.dbPath = path }
}

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// withExecFunc overrides how CLI commands are executed (for testing).
func withExecFunc(fn func(ctx context.Context, args ...string) ([]byte, error)) ClientOption {
	return func(c *Client) { c.execFunc = fn }
}

// Client provides access to the Smithers API.
// Supports three transport tiers: HTTP API, direct SQLite (read-only), and exec fallback.
type Client struct {
	apiURL    string
	apiToken  string
	dbPath    string
	db        *sql.DB
	httpClient *http.Client

	// execFunc allows overriding how CLI commands are executed (for testing).
	execFunc func(ctx context.Context, args ...string) ([]byte, error)

	// Cached server availability probe.
	serverMu      sync.RWMutex
	serverUp      bool
	serverChecked time.Time
}

// NewClient creates a new Smithers client.
// With no options, it behaves as a stub client (backward compatible).
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	// Open read-only SQLite connection if a DB path is configured.
	if c.dbPath != "" {
		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", c.dbPath))
		if err == nil {
			if err := db.Ping(); err == nil {
				c.db = db
			} else {
				db.Close()
			}
		}
	}
	return c
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// ListAgents returns CLI agents detected on the system.
// Stub returns placeholder data until HTTP/exec transport is wired.
func (c *Client) ListAgents(_ context.Context) ([]Agent, error) {
	return []Agent{
		{ID: "claude-code", Name: "Claude Code", Command: "claude", Status: "unavailable"},
		{ID: "codex", Name: "Codex", Command: "codex", Status: "unavailable"},
		{ID: "gemini", Name: "Gemini", Command: "gemini", Status: "unavailable"},
		{ID: "kimi", Name: "Kimi", Command: "kimi", Status: "unavailable"},
		{ID: "amp", Name: "Amp", Command: "amp", Status: "unavailable"},
		{ID: "forge", Name: "Forge", Command: "forge", Status: "unavailable"},
	}, nil
}

// --- Transport helpers ---

// apiEnvelope is the standard Smithers HTTP API response wrapper.
type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error,omitempty"`
}

// isServerAvailable checks if the Smithers HTTP server is reachable.
// The result is cached for 30 seconds.
func (c *Client) isServerAvailable() bool {
	if c.apiURL == "" {
		return false
	}
	c.serverMu.RLock()
	if time.Since(c.serverChecked) < 30*time.Second {
		up := c.serverUp
		c.serverMu.RUnlock()
		return up
	}
	c.serverMu.RUnlock()

	c.serverMu.Lock()
	defer c.serverMu.Unlock()

	// Double-check after acquiring write lock.
	if time.Since(c.serverChecked) < 30*time.Second {
		return c.serverUp
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+"/health", nil)
	if err != nil {
		c.serverUp = false
		c.serverChecked = time.Now()
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.serverUp = false
		c.serverChecked = time.Now()
		return false
	}
	resp.Body.Close()

	c.serverUp = resp.StatusCode == http.StatusOK
	c.serverChecked = time.Now()
	return c.serverUp
}

// httpGetJSON sends a GET request and decodes the JSON response.
func (c *Client) httpGetJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+path, nil)
	if err != nil {
		return err
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrServerUnavailable
	}
	defer resp.Body.Close()

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !env.OK {
		return fmt.Errorf("smithers API error: %s", env.Error)
	}
	if out != nil {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

// httpPostJSON sends a POST request with a JSON body and decodes the response.
func (c *Client) httpPostJSON(ctx context.Context, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrServerUnavailable
	}
	defer resp.Body.Close()

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !env.OK {
		return fmt.Errorf("smithers API error: %s", env.Error)
	}
	if out != nil {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

// queryDB executes a read-only query against the direct SQLite connection.
func (c *Client) queryDB(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if c.db == nil {
		return nil, ErrNoDatabase
	}
	return c.db.QueryContext(ctx, query, args...)
}

// execSmithers shells out to the smithers CLI and returns stdout.
func (c *Client) execSmithers(ctx context.Context, args ...string) ([]byte, error) {
	if c.execFunc != nil {
		return c.execFunc(ctx, args...)
	}
	cmd := exec.CommandContext(ctx, "smithers", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("smithers %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("smithers %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// --- Approvals ---

// ListPendingApprovals returns approvals, optionally filtered by status.
// Routes: HTTP GET /approval/list → SQLite → exec smithers approval list.
func (c *Client) ListPendingApprovals(ctx context.Context) ([]Approval, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var approvals []Approval
		err := c.httpGetJSON(ctx, "/approval/list", &approvals)
		if err == nil {
			return approvals, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT id, run_id, node_id, workflow_path, gate, status,
			payload, requested_at, resolved_at, resolved_by
			FROM _smithers_approvals ORDER BY requested_at DESC`)
		if err != nil {
			return nil, err
		}
		return scanApprovals(rows)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "approval", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseApprovalsJSON(out)
}

// --- SQL Browser ---

// ExecuteSQL executes an arbitrary SQL query against the Smithers database.
// Routes: HTTP POST /sql → SQLite (SELECT only) → exec smithers sql.
func (c *Client) ExecuteSQL(ctx context.Context, query string) (*SQLResult, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var resp struct {
			Results []map[string]interface{} `json:"results"`
		}
		err := c.httpPostJSON(ctx, "/sql", map[string]string{"query": query}, &resp)
		if err == nil {
			return convertResultMaps(resp.Results), nil
		}
	}

	// 2. Try direct SQLite for SELECT queries
	if c.db != nil && isSelectQuery(query) {
		rows, err := c.queryDB(ctx, query)
		if err != nil {
			return nil, err
		}
		return scanSQLResult(rows)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "sql", "--query", query, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseSQLResultJSON(out)
}

// isSelectQuery performs a simple prefix check to prevent mutation queries
// from reaching the read-only SQLite path.
func isSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	return strings.HasPrefix(trimmed, "SELECT") ||
		strings.HasPrefix(trimmed, "PRAGMA") ||
		strings.HasPrefix(trimmed, "EXPLAIN")
}

// --- Scores ---

// GetScores retrieves scorer evaluation results for a given run.
// Routes: SQLite (preferred, no HTTP endpoint exists) → exec smithers scores.
func (c *Client) GetScores(ctx context.Context, runID string, nodeID *string) ([]ScoreRow, error) {
	// 1. Try direct SQLite (preferred — no dedicated HTTP endpoint)
	if c.db != nil {
		query := `SELECT id, run_id, node_id, iteration, attempt, scorer_id, scorer_name,
			source, score, reason, meta_json, input_json, output_json,
			latency_ms, scored_at_ms, duration_ms
			FROM _smithers_scorer_results WHERE run_id = ?`
		args := []any{runID}
		if nodeID != nil {
			query += " AND node_id = ?"
			args = append(args, *nodeID)
		}
		query += " ORDER BY scored_at_ms DESC"
		rows, err := c.queryDB(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return scanScoreRows(rows)
	}

	// 2. Fall back to exec
	args := []string{"scores", runID, "--format", "json"}
	if nodeID != nil {
		args = append(args, "--node", *nodeID)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseScoreRowsJSON(out)
}

// GetAggregateScores computes aggregated scorer statistics for a run.
// Groups individual score rows by ScorerID and computes count, mean, min, max, p50, stddev.
func (c *Client) GetAggregateScores(ctx context.Context, runID string) ([]AggregateScore, error) {
	rows, err := c.GetScores(ctx, runID, nil)
	if err != nil {
		return nil, err
	}
	return aggregateScores(rows), nil
}

// --- Memory ---

// ListMemoryFacts lists memory facts for a namespace.
// Routes: SQLite → exec smithers memory list.
func (c *Client) ListMemoryFacts(ctx context.Context, namespace string, workflowPath string) ([]MemoryFact, error) {
	// 1. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms
			FROM _smithers_memory_facts WHERE namespace = ?`,
			namespace)
		if err != nil {
			return nil, err
		}
		return scanMemoryFacts(rows)
	}

	// 2. Fall back to exec
	args := []string{"memory", "list", namespace, "--format", "json"}
	if workflowPath != "" {
		args = append(args, "--workflow", workflowPath)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseMemoryFactsJSON(out)
}

// RecallMemory performs semantic recall (vector similarity search).
// Always exec — requires Smithers TypeScript runtime for vector search.
func (c *Client) RecallMemory(ctx context.Context, query string, namespace *string, topK int) ([]MemoryRecallResult, error) {
	args := []string{"memory", "recall", query, "--format", "json"}
	if namespace != nil {
		args = append(args, "--namespace", *namespace)
	}
	if topK > 0 {
		args = append(args, "--topK", strconv.Itoa(topK))
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseRecallResultsJSON(out)
}

// --- Cron / Triggers ---

// ListCrons lists all cron trigger schedules.
// Routes: HTTP GET /cron/list → SQLite → exec smithers cron list.
func (c *Client) ListCrons(ctx context.Context) ([]CronSchedule, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var crons []CronSchedule
		err := c.httpGetJSON(ctx, "/cron/list", &crons)
		if err == nil {
			return crons, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT cron_id, pattern, workflow_path, enabled, created_at_ms,
			last_run_at_ms, next_run_at_ms, error_json FROM _smithers_crons`)
		if err != nil {
			return nil, err
		}
		return scanCronSchedules(rows)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "cron", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseCronSchedulesJSON(out)
}

// CreateCron creates a new cron trigger schedule.
// Routes: HTTP POST /cron/add → exec smithers cron add.
func (c *Client) CreateCron(ctx context.Context, pattern string, workflowPath string) (*CronSchedule, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var cron CronSchedule
		err := c.httpPostJSON(ctx, "/cron/add", map[string]string{
			"pattern": pattern, "workflowPath": workflowPath,
		}, &cron)
		if err == nil {
			return &cron, nil
		}
	}

	// 2. Fall back to exec (no SQLite for mutations)
	out, err := c.execSmithers(ctx, "cron", "add", pattern, workflowPath, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseCronScheduleJSON(out)
}

// ToggleCron enables or disables a cron trigger.
// Routes: HTTP POST /cron/toggle/{id} → exec smithers cron toggle.
func (c *Client) ToggleCron(ctx context.Context, cronID string, enabled bool) error {
	// 1. Try HTTP
	if c.isServerAvailable() {
		err := c.httpPostJSON(ctx, "/cron/toggle/"+cronID,
			map[string]bool{"enabled": enabled}, nil)
		if err == nil {
			return nil
		}
	}

	// 2. Fall back to exec
	_, err := c.execSmithers(ctx, "cron", "toggle", cronID,
		"--enabled", strconv.FormatBool(enabled))
	return err
}

// DeleteCron removes a cron trigger.
// No HTTP endpoint exists — always exec.
func (c *Client) DeleteCron(ctx context.Context, cronID string) error {
	_, err := c.execSmithers(ctx, "cron", "rm", cronID)
	return err
}

// --- Tickets ---

// ListTickets lists all tickets discovered from .smithers/tickets/.
// Routes: HTTP GET /ticket/list → exec smithers ticket list.
func (c *Client) ListTickets(ctx context.Context) ([]Ticket, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var tickets []Ticket
		err := c.httpGetJSON(ctx, "/ticket/list", &tickets)
		if err == nil {
			return tickets, nil
		}
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "ticket", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseTicketsJSON(out)
}

// --- Scan/parse helpers ---

// scanSQLResult converts sql.Rows into an SQLResult.
func scanSQLResult(rows *sql.Rows) (*SQLResult, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &SQLResult{Columns: cols}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// Convert []byte values to strings for JSON compatibility.
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, vals)
	}
	return result, rows.Err()
}

// convertResultMaps converts HTTP response maps to a columnar SQLResult.
func convertResultMaps(results []map[string]interface{}) *SQLResult {
	if len(results) == 0 {
		return &SQLResult{}
	}
	// Extract columns from first row, sorted for deterministic order.
	var cols []string
	for k := range results[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	r := &SQLResult{Columns: cols}
	for _, m := range results {
		row := make([]interface{}, len(cols))
		for i, col := range cols {
			row[i] = m[col]
		}
		r.Rows = append(r.Rows, row)
	}
	return r
}

// parseSQLResultJSON parses exec output into an SQLResult.
func parseSQLResultJSON(data []byte) (*SQLResult, error) {
	// The CLI may return an array of objects or the SQLResult directly.
	var result SQLResult
	if err := json.Unmarshal(data, &result); err == nil && len(result.Columns) > 0 {
		return &result, nil
	}
	// Try array-of-objects format.
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse SQL result: %w", err)
	}
	return convertResultMaps(arr), nil
}

// scanScoreRows converts sql.Rows into ScoreRow slice.
func scanScoreRows(rows *sql.Rows) ([]ScoreRow, error) {
	defer rows.Close()
	var result []ScoreRow
	for rows.Next() {
		var s ScoreRow
		if err := rows.Scan(
			&s.ID, &s.RunID, &s.NodeID, &s.Iteration, &s.Attempt,
			&s.ScorerID, &s.ScorerName, &s.Source, &s.Score, &s.Reason,
			&s.MetaJSON, &s.InputJSON, &s.OutputJSON, &s.LatencyMs,
			&s.ScoredAtMs, &s.DurationMs,
		); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// parseScoreRowsJSON parses exec output into ScoreRow slice.
func parseScoreRowsJSON(data []byte) ([]ScoreRow, error) {
	var rows []ScoreRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parse score rows: %w", err)
	}
	return rows, nil
}

// aggregateScores groups ScoreRows by ScorerID and computes summary stats.
func aggregateScores(rows []ScoreRow) []AggregateScore {
	groups := make(map[string][]ScoreRow)
	names := make(map[string]string)
	for _, r := range rows {
		groups[r.ScorerID] = append(groups[r.ScorerID], r)
		names[r.ScorerID] = r.ScorerName
	}

	var result []AggregateScore
	for id, group := range groups {
		scores := make([]float64, len(group))
		sum := 0.0
		minVal := math.Inf(1)
		maxVal := math.Inf(-1)

		for i, r := range group {
			scores[i] = r.Score
			sum += r.Score
			if r.Score < minVal {
				minVal = r.Score
			}
			if r.Score > maxVal {
				maxVal = r.Score
			}
		}

		n := float64(len(group))
		mean := sum / n

		// Standard deviation
		variance := 0.0
		for _, s := range scores {
			d := s - mean
			variance += d * d
		}
		if n > 1 {
			variance /= n - 1
		}

		// P50 (median)
		sort.Float64s(scores)
		var p50 float64
		if len(scores)%2 == 0 {
			p50 = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		} else {
			p50 = scores[len(scores)/2]
		}

		result = append(result, AggregateScore{
			ScorerID:   id,
			ScorerName: names[id],
			Count:      len(group),
			Mean:       mean,
			Min:        minVal,
			Max:        maxVal,
			P50:        p50,
			StdDev:     math.Sqrt(variance),
		})
	}

	// Sort by scorer ID for deterministic output.
	sort.Slice(result, func(i, j int) bool {
		return result[i].ScorerID < result[j].ScorerID
	})
	return result
}

// scanMemoryFacts converts sql.Rows into MemoryFact slice.
func scanMemoryFacts(rows *sql.Rows) ([]MemoryFact, error) {
	defer rows.Close()
	var result []MemoryFact
	for rows.Next() {
		var f MemoryFact
		if err := rows.Scan(
			&f.Namespace, &f.Key, &f.ValueJSON, &f.SchemaSig,
			&f.CreatedAtMs, &f.UpdatedAtMs, &f.TTLMs,
		); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// parseMemoryFactsJSON parses exec output into MemoryFact slice.
func parseMemoryFactsJSON(data []byte) ([]MemoryFact, error) {
	var facts []MemoryFact
	if err := json.Unmarshal(data, &facts); err != nil {
		return nil, fmt.Errorf("parse memory facts: %w", err)
	}
	return facts, nil
}

// parseRecallResultsJSON parses exec output into MemoryRecallResult slice.
func parseRecallResultsJSON(data []byte) ([]MemoryRecallResult, error) {
	var results []MemoryRecallResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse recall results: %w", err)
	}
	return results, nil
}

// scanCronSchedules converts sql.Rows into CronSchedule slice.
func scanCronSchedules(rows *sql.Rows) ([]CronSchedule, error) {
	defer rows.Close()
	var result []CronSchedule
	for rows.Next() {
		var cs CronSchedule
		if err := rows.Scan(
			&cs.CronID, &cs.Pattern, &cs.WorkflowPath, &cs.Enabled,
			&cs.CreatedAtMs, &cs.LastRunAtMs, &cs.NextRunAtMs, &cs.ErrorJSON,
		); err != nil {
			return nil, err
		}
		result = append(result, cs)
	}
	return result, rows.Err()
}

// parseCronSchedulesJSON parses exec output into CronSchedule slice.
func parseCronSchedulesJSON(data []byte) ([]CronSchedule, error) {
	var crons []CronSchedule
	if err := json.Unmarshal(data, &crons); err != nil {
		return nil, fmt.Errorf("parse cron schedules: %w", err)
	}
	return crons, nil
}

// parseCronScheduleJSON parses exec output into a single CronSchedule.
func parseCronScheduleJSON(data []byte) (*CronSchedule, error) {
	var cron CronSchedule
	if err := json.Unmarshal(data, &cron); err != nil {
		return nil, fmt.Errorf("parse cron schedule: %w", err)
	}
	return &cron, nil
}

// parseTicketsJSON parses exec output into Ticket slice.
func parseTicketsJSON(data []byte) ([]Ticket, error) {
	var tickets []Ticket
	if err := json.Unmarshal(data, &tickets); err != nil {
		return nil, fmt.Errorf("parse tickets: %w", err)
	}
	return tickets, nil
}

// scanApprovals converts sql.Rows into Approval slice.
func scanApprovals(rows *sql.Rows) ([]Approval, error) {
	defer rows.Close()
	var result []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(
			&a.ID, &a.RunID, &a.NodeID, &a.WorkflowPath, &a.Gate,
			&a.Status, &a.Payload, &a.RequestedAt, &a.ResolvedAt, &a.ResolvedBy,
		); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// parseApprovalsJSON parses exec output into Approval slice.
func parseApprovalsJSON(data []byte) ([]Approval, error) {
	var approvals []Approval
	if err := json.Unmarshal(data, &approvals); err != nil {
		return nil, fmt.Errorf("parse approvals: %w", err)
	}
	return approvals, nil
}

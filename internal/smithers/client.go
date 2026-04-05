package smithers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

// HTTPError is returned when the Smithers legacy HTTP server responds with a
// non-2xx status code on the envelope-wrapped API paths (/sql, /cron/*, etc.).
// v1 API paths (/v1/runs, ...) use decodeV1Response which maps status codes to
// sentinel errors (ErrRunNotFound, ErrUnauthorized, etc.) instead.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("smithers API HTTP %d: %s", e.StatusCode, e.Message)
}

// IsUnauthorized returns true if err is an *HTTPError with StatusCode 401.
func IsUnauthorized(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.StatusCode == http.StatusUnauthorized
}

// IsServerUnavailable returns true if err indicates the server is unreachable.
func IsServerUnavailable(err error) bool {
	return errors.Is(err, ErrServerUnavailable)
}

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

// WithWorkspaceID sets the Smithers workspace ID for workspace-scoped API calls.
// This is required by the daemon API routes (/api/workspaces/{workspaceId}/...).
// When unset, workspace-scoped methods fall through to exec fallback.
func WithWorkspaceID(id string) ClientOption {
	return func(c *Client) { c.workspaceID = id }
}

// withExecFunc overrides how CLI commands are executed (for testing).
func withExecFunc(fn func(ctx context.Context, args ...string) ([]byte, error)) ClientOption {
	return func(c *Client) { c.execFunc = fn }
}

// withLookPath overrides binary resolution for testing.
func withLookPath(fn func(file string) (string, error)) ClientOption {
	return func(c *Client) { c.lookPath = fn }
}

// withStatFunc overrides filesystem stat for testing.
func withStatFunc(fn func(name string) (os.FileInfo, error)) ClientOption {
	return func(c *Client) { c.statFunc = fn }
}

// agentManifestEntry defines detection parameters for a single CLI agent.
type agentManifestEntry struct {
	id          string
	name        string
	command     string
	roles       []string
	authDir     string // relative to $HOME, e.g. ".claude"
	apiKeyEnv   string // env var name, e.g. "ANTHROPIC_API_KEY"
	versionFlag string // flag to pass for version output, e.g. "--version"; empty = skip version probe
	credFile    string // path relative to authDir for credentials JSON, e.g. ".credentials.json"
}

// knownAgents is the canonical detection manifest for CLI agents.
var knownAgents = []agentManifestEntry{
	{
		id: "claude-code", name: "Claude Code", command: "claude",
		roles:       []string{"coding", "review", "spec"},
		authDir:     ".claude",
		apiKeyEnv:   "ANTHROPIC_API_KEY",
		versionFlag: "--version",
		credFile:    ".credentials.json",
	},
	{
		id: "codex", name: "Codex", command: "codex",
		roles:       []string{"coding", "implement"},
		authDir:     ".codex",
		apiKeyEnv:   "OPENAI_API_KEY",
		versionFlag: "--version",
	},
	{
		id: "gemini", name: "Gemini", command: "gemini",
		roles:       []string{"coding", "research"},
		authDir:     ".gemini",
		apiKeyEnv:   "GEMINI_API_KEY",
		versionFlag: "--version",
	},
	{
		id: "kimi", name: "Kimi", command: "kimi",
		roles:     []string{"research", "plan"},
		authDir:   "",
		apiKeyEnv: "KIMI_API_KEY",
		// versionFlag intentionally omitted — kimi has no --version support
	},
	{
		id: "amp", name: "Amp", command: "amp",
		roles:       []string{"coding", "validate"},
		authDir:     ".amp",
		apiKeyEnv:   "",
		versionFlag: "--version",
	},
	{
		id: "forge", name: "Forge", command: "forge",
		roles:       []string{"coding"},
		authDir:     "",
		apiKeyEnv:   "FORGE_API_KEY",
		versionFlag: "--version",
	},
}

// Client provides access to the Smithers API.
// Supports three transport tiers: HTTP API, direct SQLite (read-only), and exec fallback.
type Client struct {
	apiURL      string
	apiToken    string
	workspaceID string // workspace ID for daemon /api/workspaces/{id}/... routes
	dbPath      string
	db          *sql.DB
	httpClient  *http.Client

	// execFunc allows overriding how CLI commands are executed (for testing).
	execFunc func(ctx context.Context, args ...string) ([]byte, error)

	// lookPath resolves a binary name to its full path (injectable for testing).
	lookPath func(file string) (string, error)
	// statFunc checks whether a filesystem path exists (injectable for testing).
	statFunc func(name string) (os.FileInfo, error)

	// Exec infrastructure fields (configured via With* options).
	binaryPath  string        // path to the smithers CLI binary; default "smithers"
	execTimeout time.Duration // default exec timeout; 0 means none
	workingDir  string        // working directory for exec; "" inherits TUI process cwd
	logger      Logger        // optional transport logger; nil = no-op

	// Cached server availability probe.
	serverMu      sync.RWMutex
	serverUp      bool
	serverChecked time.Time

	// Per-runID cache for GetRunContext results (30-second TTL).
	runSummaryCache sync.Map // map[string]*runContextCacheEntry
}

// runContextCacheEntry holds a cached RunContext with its fetch timestamp.
type runContextCacheEntry struct {
	context   *RunContext
	fetchedAt time.Time
}

// NewClient creates a new Smithers client.
// With no options, it behaves as a stub client (backward compatible).
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		lookPath:   exec.LookPath,
		statFunc:   os.Stat,
		binaryPath: "smithers",
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

// SmithersBinaryPath resolves the full path to the smithers CLI binary using
// the configured lookPath function (defaults to exec.LookPath).
// Returns ErrBinaryNotFound if smithers is not on PATH.
func (c *Client) SmithersBinaryPath() (string, error) {
	path, err := c.lookPath(c.binaryPath)
	if err != nil {
		return "", ErrBinaryNotFound
	}
	return path, nil
}

// ListAgents detects CLI agents installed on the system using pure-Go binary
// and auth-signal detection.  Results reflect the real system state; no
// subprocess is spawned.  The lookPath and statFunc fields are injectable for
// testing.
func (c *Client) ListAgents(_ context.Context) ([]Agent, error) {
	homeDir, _ := os.UserHomeDir() // empty string on error; stat checks will fail gracefully

	agents := make([]Agent, 0, len(knownAgents))
	for _, m := range knownAgents {
		a := Agent{
			ID:      m.id,
			Name:    m.name,
			Command: m.command,
			Roles:   m.roles,
		}

		// 1. Binary detection.
		binaryPath, err := c.lookPath(m.command)
		if err != nil {
			// Binary not found in PATH.
			a.Status = "unavailable"
			a.Usable = false
			agents = append(agents, a)
			continue
		}
		a.BinaryPath = binaryPath

		// 2. Auth-directory check.
		if m.authDir != "" && homeDir != "" {
			authPath := filepath.Join(homeDir, m.authDir)
			if _, err := c.statFunc(authPath); err == nil {
				a.HasAuth = true
			}
		}

		// 3. API-key env-var check.
		if m.apiKeyEnv != "" && os.Getenv(m.apiKeyEnv) != "" {
			a.HasAPIKey = true
		}

		// 4. Classify status.
		switch {
		case a.HasAuth:
			a.Status = "likely-subscription"
		case a.HasAPIKey:
			a.Status = "api-key"
		default:
			a.Status = "binary-only"
		}
		a.Usable = true

		agents = append(agents, a)
	}
	return agents, nil
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

// SetServerUp sets the cached server-availability flag directly.
// This is intended for use in tests that construct a mock server and need
// to bypass the health-check probe.
func (c *Client) SetServerUp(up bool) {
	c.serverMu.Lock()
	c.serverUp = up
	c.serverChecked = time.Now().Add(30 * time.Second) // suppress re-probe
	c.serverMu.Unlock()
}

// invalidateServerCache resets the availability cache so the next call
// re-probes the server instead of waiting up to 30 seconds.
// Call this whenever a mid-flight HTTP request fails with a transport error.
func (c *Client) invalidateServerCache() {
	c.serverMu.Lock()
	c.serverUp = false
	c.serverChecked = time.Time{} // zero time forces re-probe on next call
	c.serverMu.Unlock()
}

// httpGetJSON sends a GET request and decodes the JSON response envelope.
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
		c.invalidateServerCache()
		return fmt.Errorf("%w: %w", ErrServerUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle non-2xx status codes before attempting JSON decode.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 500 {
			c.invalidateServerCache()
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(rawBody))}
	}

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

// httpPostJSON sends a POST request with a JSON body and decodes the response envelope.
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
		c.invalidateServerCache()
		return fmt.Errorf("%w: %w", ErrServerUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle non-2xx status codes before attempting JSON decode.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 500 {
			c.invalidateServerCache()
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(rawBody))}
	}

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

// execSmithers is defined in exec.go.

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

	// 3. Fall back to exec — no `smithers approval list` command exists.
	// Approvals are embedded in run inspection output. Return empty list
	// since both HTTP and SQLite paths were already attempted.
	return nil, nil
}

// ListRecentDecisions returns a list of recently decided (approved/denied) approvals.
// Routes: HTTP GET /approval/decisions → SQLite → exec smithers approval decisions.
func (c *Client) ListRecentDecisions(ctx context.Context, limit int) ([]ApprovalDecision, error) {
	if limit <= 0 {
		limit = 20
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var decisions []ApprovalDecision
		err := c.httpGetJSON(ctx, "/approval/decisions", &decisions)
		if err == nil {
			return decisions, nil
		}
	}

	// 2. Try direct SQLite (read resolved rows from _smithers_approvals)
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT id, run_id, node_id, workflow_path, gate,
			status, resolved_at, resolved_by, requested_at
			FROM _smithers_approvals
			WHERE status IN ('"approved"', '"denied"')
			ORDER BY resolved_at DESC
			LIMIT ?`, limit)
		if err == nil {
			return scanApprovalDecisions(rows)
		}
	}

	// 3. Fall back to exec — no `smithers approval decisions` command exists.
	// Return empty list since both HTTP and SQLite paths were already attempted.
	return nil, nil
}

// Approve submits an approval decision for a pending approval gate.
// Routes: HTTP POST /v1/runs/:runID/nodes/:nodeID/approve → exec smithers approve.
func (c *Client) Approve(ctx context.Context, runID, nodeID string, iteration int, note string) error {
	// 1. Try HTTP
	if c.isServerAvailable() {
		err := c.httpPostJSON(ctx,
			fmt.Sprintf("/v1/runs/%s/nodes/%s/approve", runID, nodeID),
			map[string]any{"iteration": iteration, "note": note}, nil)
		if err == nil {
			return nil
		}
	}

	// 2. Fall back to exec (no SQLite tier for mutations)
	args := []string{"approve", runID, "--node", nodeID,
		"--iteration", strconv.Itoa(iteration), "--format", "json"}
	if note != "" {
		args = append(args, "--note", note)
	}
	_, err := c.execSmithers(ctx, args...)
	return err
}

// Deny submits a denial decision for a pending approval gate.
// Routes: HTTP POST /v1/runs/:runID/nodes/:nodeID/deny → exec smithers deny.
func (c *Client) Deny(ctx context.Context, runID, nodeID string, iteration int, reason string) error {
	// 1. Try HTTP
	if c.isServerAvailable() {
		err := c.httpPostJSON(ctx,
			fmt.Sprintf("/v1/runs/%s/nodes/%s/deny", runID, nodeID),
			map[string]any{"iteration": iteration, "reason": reason}, nil)
		if err == nil {
			return nil
		}
	}

	// 2. Fall back to exec (no SQLite tier for mutations)
	args := []string{"deny", runID, "--node", nodeID,
		"--iteration", strconv.Itoa(iteration), "--format", "json"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	_, err := c.execSmithers(ctx, args...)
	return err
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

	// 3. The smithers CLI has no `sql` subcommand — return ErrNoTransport
	// with an actionable hint. SQL queries require the HTTP server.
	return nil, fmt.Errorf("%w: SQL requires a running smithers server; start with: smithers up --serve", ErrNoTransport)
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
			FROM _smithers_scorers WHERE run_id = ?`	// upstream: smithers/src/scorers/schema.ts
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

// ListRecentScores retrieves the most recent scorer results across all runs.
// Routes: SQLite (preferred — no HTTP endpoint exists) → returns nil on exec fallback
// (smithers scores requires a runID; cross-run queries need a direct DB connection).
func (c *Client) ListRecentScores(ctx context.Context, limit int) ([]ScoreRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if c.db != nil {
		query := `SELECT id, run_id, node_id, iteration, attempt, scorer_id, scorer_name,
			source, score, reason, meta_json, input_json, output_json,
			latency_ms, scored_at_ms, duration_ms
			FROM _smithers_scorer_results ORDER BY scored_at_ms DESC LIMIT ?`
		rows, err := c.queryDB(ctx, query, limit)
		if err != nil {
			// Treat "no such table" as an empty result — older Smithers DBs may not
			// have the scoring system tables.
			if strings.Contains(err.Error(), "no such table") {
				return nil, nil
			}
			return nil, err
		}
		return scanScoreRows(rows)
	}
	// Exec fallback: smithers scores requires a runID; omit rather than error.
	// The view will show the empty state. Downstream tickets can add an HTTP endpoint.
	return nil, nil
}

// AggregateAllScores computes aggregated scorer statistics across all recent runs.
// Reuses the aggregateScores() helper already in client.go.
func (c *Client) AggregateAllScores(ctx context.Context, limit int) ([]AggregateScore, error) {
	rows, err := c.ListRecentScores(ctx, limit)
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

// ListAllMemoryFacts lists all memory facts across all namespaces.
// Routes: SQLite → exec smithers memory list --all.
func (c *Client) ListAllMemoryFacts(ctx context.Context) ([]MemoryFact, error) {
	// 1. Try direct SQLite (preferred — no dedicated HTTP endpoint)
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT namespace, key, value_json, schema_sig, created_at_ms, updated_at_ms, ttl_ms
			FROM _smithers_memory_facts ORDER BY updated_at_ms DESC`)
		if err != nil {
			return nil, err
		}
		return scanMemoryFacts(rows)
	}

	// 2. Fall back to exec
	// TODO: The --all flag requires smithers CLI support. If unavailable, the exec path will
	// return an error that MemoryView renders gracefully.
	out, err := c.execSmithers(ctx, "memory", "list", "--all", "--format", "json")
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
			last_run_at_ms, next_run_at_ms, error_json FROM _smithers_cron`)	// upstream: smithers/src/db/internal-schema.ts
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
	// The upstream CLI uses `cron enable <id>` / `cron disable <id>`.
	// The flag-based form `cron toggle --enabled <bool>` does not exist.
	subcmd := "enable"
	if !enabled {
		subcmd = "disable"
	}
	_, err := c.execSmithers(ctx, "cron", subcmd, cronID)
	return err
}

// DeleteCron removes a cron trigger.
// No HTTP endpoint exists — always exec.
func (c *Client) DeleteCron(ctx context.Context, cronID string) error {
	_, err := c.execSmithers(ctx, "cron", "rm", cronID)
	return err
}

// --- Runs ---

// GetRun fetches a single run summary by ID.
// Routes: HTTP GET /v1/runs/:id → exec smithers run get <id>.
func (c *Client) GetRun(ctx context.Context, runID string) (*RunSummary, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var run RunSummary
		err := c.httpGetJSON(ctx, "/v1/runs/"+runID, &run)
		if err == nil {
			return &run, nil
		}
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "run", "get", runID, "--format", "json")
	if err != nil {
		return nil, err
	}
	var run RunSummary
	if err := json.Unmarshal(out, &run); err != nil {
		return nil, fmt.Errorf("parse run: %w", err)
	}
	return &run, nil
}

// GetRunContext returns lightweight run metadata enriched with progress counters
// and elapsed time. Used by the approval detail pane and other views that need
// run context without full node trees.
// Routes: HTTP GET /v1/runs/{runID} → SQLite → exec smithers inspect.
// Results are cached for 30 seconds per runID.
func (c *Client) GetRunContext(ctx context.Context, runID string) (*RunContext, error) {
	if cached, ok := c.getRunContextCache(runID); ok {
		return cached, nil
	}

	var rc *RunContext

	// 1. Try HTTP
	if c.isServerAvailable() {
		var s RunContext
		err := c.httpGetJSON(ctx, "/v1/runs/"+runID, &s)
		if err == nil {
			if s.WorkflowName == "" && s.WorkflowPath != "" {
				s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
			}
			if s.ElapsedMs == 0 && s.StartedAtMs > 0 {
				s.ElapsedMs = time.Now().UnixMilli() - s.StartedAtMs
			}
			rc = &s
		}
	}

	// 2. Try direct SQLite
	if rc == nil && c.db != nil {
		row := c.db.QueryRowContext(ctx,
			`SELECT r.id, r.workflow_path, r.status, r.started_at,
			 (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id) AS node_total,
			 (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id
			  AND n.status IN ('completed', 'failed')) AS nodes_done
			 FROM _smithers_runs r WHERE r.id = ?`, runID)
		var s RunContext
		var startedAt int64
		if err := row.Scan(&s.ID, &s.WorkflowPath, &s.Status, &startedAt, &s.NodeTotal, &s.NodesDone); err == nil {
			s.StartedAtMs = startedAt
			s.ElapsedMs = time.Now().UnixMilli() - startedAt
			s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
			rc = &s
		}
	}

	// 3. Fall back to exec
	if rc == nil {
		out, err := c.execSmithers(ctx, "inspect", runID, "--format", "json")
		if err != nil {
			return nil, err
		}
		var s RunContext
		if err := json.Unmarshal(out, &s); err != nil {
			return nil, fmt.Errorf("parse run context: %w", err)
		}
		if s.WorkflowName == "" && s.WorkflowPath != "" {
			s.WorkflowName = workflowNameFromPath(s.WorkflowPath)
		}
		if s.ElapsedMs == 0 && s.StartedAtMs > 0 {
			s.ElapsedMs = time.Now().UnixMilli() - s.StartedAtMs
		}
		rc = &s
	}

	c.setRunContextCache(runID, rc)
	return rc, nil
}

// ClearRunContextCache removes the cached RunContext for runID.
func (c *Client) ClearRunContextCache(runID string) {
	c.runSummaryCache.Delete(runID)
}

// getRunContextCache returns a cached RunContext if present and not expired (30s TTL).
func (c *Client) getRunContextCache(runID string) (*RunContext, bool) {
	v, ok := c.runSummaryCache.Load(runID)
	if !ok {
		return nil, false
	}
	entry := v.(*runContextCacheEntry)
	if time.Since(entry.fetchedAt) > 30*time.Second {
		c.runSummaryCache.Delete(runID)
		return nil, false
	}
	return entry.context, true
}

// setRunContextCache stores a RunContext in the cache for runID.
func (c *Client) setRunContextCache(runID string, rc *RunContext) {
	c.runSummaryCache.Store(runID, &runContextCacheEntry{
		context:   rc,
		fetchedAt: time.Now(),
	})
}

// GetChatOutput retrieves the full chat transcript for a run (all attempts, all nodes).
// Routes: HTTP GET /v1/runs/:id/chat → SQLite → exec smithers run chat <id>.
func (c *Client) GetChatOutput(ctx context.Context, runID string) ([]ChatBlock, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var blocks []ChatBlock
		err := c.httpGetJSON(ctx, "/v1/runs/"+runID+"/chat", &blocks)
		if err == nil {
			return blocks, nil
		}
	}

	// 2. Try direct SQLite
	if c.db != nil {
		rows, err := c.queryDB(ctx,
			`SELECT id, run_id, node_id, attempt, role, content, timestamp_ms
			FROM _smithers_chat_attempts WHERE run_id = ?
			ORDER BY timestamp_ms ASC, id ASC`,
			runID)
		if err != nil {
			return nil, err
		}
		return scanChatBlocks(rows)
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "run", "chat", runID, "--format", "json")
	if err != nil {
		return nil, err
	}
	var blocks []ChatBlock
	if err := json.Unmarshal(out, &blocks); err != nil {
		return nil, fmt.Errorf("parse chat output: %w", err)
	}
	return blocks, nil
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

	// 2. Read directly from .smithers/tickets/*.md on the filesystem.
	// There is no `smithers ticket list` CLI command — tickets are plain files.
	ticketsDir := filepath.Join(".smithers", "tickets")
	if c.workingDir != "" {
		ticketsDir = filepath.Join(c.workingDir, ".smithers", "tickets")
	}
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No tickets directory — empty list
		}
		return nil, fmt.Errorf("reading tickets dir: %w", err)
	}
	var tickets []Ticket
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(ticketsDir, e.Name()))
		if err != nil {
			continue
		}
		tickets = append(tickets, Ticket{ID: id, Content: string(content)})
	}
	return tickets, nil
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
		return nil, &JSONParseError{Command: "sql", Output: data, Err: err}
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
		return nil, &JSONParseError{Command: "scores", Output: data, Err: err}
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
		return nil, &JSONParseError{Command: "memory list", Output: data, Err: err}
	}
	return facts, nil
}

// parseRecallResultsJSON parses exec output into MemoryRecallResult slice.
func parseRecallResultsJSON(data []byte) ([]MemoryRecallResult, error) {
	var results []MemoryRecallResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, &JSONParseError{Command: "memory recall", Output: data, Err: err}
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
		return nil, &JSONParseError{Command: "cron list", Output: data, Err: err}
	}
	return crons, nil
}

// parseCronScheduleJSON parses exec output into a single CronSchedule.
func parseCronScheduleJSON(data []byte) (*CronSchedule, error) {
	var cron CronSchedule
	if err := json.Unmarshal(data, &cron); err != nil {
		return nil, &JSONParseError{Command: "cron add", Output: data, Err: err}
	}
	return &cron, nil
}

// parseTicketsJSON parses exec output into Ticket slice.
func parseTicketsJSON(data []byte) ([]Ticket, error) {
	var tickets []Ticket
	if err := json.Unmarshal(data, &tickets); err != nil {
		return nil, &JSONParseError{Command: "ticket list", Output: data, Err: err}
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
		return nil, &JSONParseError{Command: "approval list", Output: data, Err: err}
	}
	return approvals, nil
}

// scanChatBlocks converts sql.Rows into ChatBlock slice.
func scanChatBlocks(rows *sql.Rows) ([]ChatBlock, error) {
	defer rows.Close()
	var result []ChatBlock
	for rows.Next() {
		var b ChatBlock
		var role string
		if err := rows.Scan(&b.ID, &b.RunID, &b.NodeID, &b.Attempt, &role, &b.Content, &b.TimestampMs); err != nil {
			return nil, err
		}
		b.Role = ChatRole(role)
		result = append(result, b)
	}
	return result, rows.Err()
}

// scanApprovalDecisions converts sql.Rows into ApprovalDecision slice.
// Expected column order: id, run_id, node_id, workflow_path, gate, status (used as decision), resolved_at, resolved_by, requested_at.
func scanApprovalDecisions(rows *sql.Rows) ([]ApprovalDecision, error) {
	defer rows.Close()
	var result []ApprovalDecision
	for rows.Next() {
		var d ApprovalDecision
		var resolvedAt *int64
		if err := rows.Scan(
			&d.ID, &d.RunID, &d.NodeID, &d.WorkflowPath, &d.Gate,
			&d.Decision, &resolvedAt, &d.DecidedBy, &d.RequestedAt,
		); err != nil {
			return nil, err
		}
		if resolvedAt != nil {
			d.DecidedAt = *resolvedAt
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// parseApprovalDecisionsJSON parses exec output into ApprovalDecision slice.
func parseApprovalDecisionsJSON(data []byte) ([]ApprovalDecision, error) {
	var decisions []ApprovalDecision
	if err := json.Unmarshal(data, &decisions); err != nil {
		return nil, fmt.Errorf("parse approval decisions: %w", err)
	}
	return decisions, nil
}

// workflowNameFromPath extracts a human-readable workflow name from a file path.
// E.g., ".smithers/workflows/deploy.ts" → "deploy".
func workflowNameFromPath(p string) string {
	base := path.Base(p)
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".yaml", ".yml"} {
		if strings.HasSuffix(base, ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

## Goal
Bring `internal/smithers/client.go` (and the surrounding package) to the state required for the run dashboard, live chat, approval queue, and time-travel views to function. This means: correct run-domain types, HTTP methods for the run lifecycle, a working SSE consumer, fixed error handling, repaired schema/CLI drift, and config wiring. The `platform-thin-frontend-layer` plan already established the SSE consumer and run types as work items; this plan focuses narrowly on the HTTP transport layer quality improvements and the run-lifecycle methods that are the direct dependency of all runtime views.

---

## Steps

### Step 1 — Expand types.go with run-domain models

**File**: `internal/smithers/types.go`

Add the following types. Keep all existing types unchanged.

**`RunStatus`** — string enum for run state:
```go
type RunStatus string

const (
    RunStatusActive    RunStatus = "active"
    RunStatusPaused    RunStatus = "paused"
    RunStatusCompleted RunStatus = "completed"
    RunStatusFailed    RunStatus = "failed"
    RunStatusCancelled RunStatus = "cancelled"
)
```

**`Run`** — top-level run summary returned by `GET /v1/runs`:
```go
type Run struct {
    ID           string    `json:"id"`
    WorkflowPath string    `json:"workflowPath"`
    Status       RunStatus `json:"status"`
    StartedAt    int64     `json:"startedAt"`    // Unix ms
    FinishedAt   *int64    `json:"finishedAt"`   // nil if not finished
    Error        *string   `json:"error"`
    ActiveNodes  []string  `json:"activeNodes"`  // node IDs currently executing
}
```

**`RunNode`** — node within a run:
```go
type RunNode struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Status      RunStatus `json:"status"`
    Iteration   int       `json:"iteration"`
    Attempt     int       `json:"attempt"`
    StartedAt   *int64    `json:"startedAt"`
    FinishedAt  *int64    `json:"finishedAt"`
}
```

**`RunDetail`** — full run inspection response:
```go
type RunDetail struct {
    Run
    Nodes    []RunNode  `json:"nodes"`
    Approvals []Approval `json:"approvals"`
}
```

**`RunEvent`** — a single event from the SSE stream:
```go
type RunEvent struct {
    Seq       int64           `json:"seq"`
    Type      string          `json:"type"`   // e.g. "run.status", "node.status", "approval.created"
    Timestamp int64           `json:"timestamp"`
    Payload   json.RawMessage `json:"payload"`
}
```

**`HijackSession`** — returned by `POST /v1/runs/:id/hijack`:
```go
type HijackSession struct {
    RunID       string   `json:"runId"`
    Engine      string   `json:"engine"`       // e.g. "claude-code"
    AgentBinary string   `json:"agentBinary"`  // e.g. "claude"
    ResumeToken string   `json:"resumeToken"`
    CWD         string   `json:"cwd"`
}

// ResumeArgs returns the CLI args to pass to the agent binary for resuming.
func (h *HijackSession) ResumeArgs() []string {
    if h.ResumeToken == "" {
        return nil
    }
    return []string{"--resume", h.ResumeToken}
}
```

**`Workflow`** — discovered workflow:
```go
type Workflow struct {
    ID          string `json:"id"`
    Path        string `json:"path"`
    Name        string `json:"name"`
    Description string `json:"description"`
}
```

**`Snapshot`** — time-travel checkpoint:
```go
type Snapshot struct {
    No          int    `json:"no"`
    RunID       string `json:"runId"`
    NodeID      string `json:"nodeId"`
    CreatedAtMs int64  `json:"createdAtMs"`
    Label       string `json:"label,omitempty"`
}
```

**`Diff`** — snapshot diff result:
```go
type Diff struct {
    From    int             `json:"from"`
    To      int             `json:"to"`
    Changes json.RawMessage `json:"changes"`
}
```

---

### Step 2 — Fix error handling in HTTP helpers

**File**: `internal/smithers/client.go`

#### 2a. Define typed HTTP errors

Add before the existing helpers:

```go
// HTTPError is returned when the server responds with a non-2xx status code.
type HTTPError struct {
    StatusCode int
    Message    string
}

func (e *HTTPError) Error() string {
    return fmt.Sprintf("smithers API HTTP %d: %s", e.StatusCode, e.Message)
}

// IsUnauthorized returns true if the error is an HTTP 401.
func IsUnauthorized(err error) bool {
    var he *HTTPError
    return errors.As(err, &he) && he.StatusCode == http.StatusUnauthorized
}

// IsServerUnavailable returns true if the error indicates the server is down.
func IsServerUnavailable(err error) bool {
    return errors.Is(err, ErrServerUnavailable)
}
```

#### 2b. Update httpGetJSON and httpPostJSON

Both helpers need two changes:

1. **Preserve transport error**: instead of discarding the underlying error, wrap it:
   ```go
   if err != nil {
       c.invalidateServerCache()
       return fmt.Errorf("%w: %w", ErrServerUnavailable, err)
   }
   ```

2. **Handle non-OK HTTP status before JSON decode**:
   ```go
   if resp.StatusCode < 200 || resp.StatusCode >= 300 {
       body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
       return &HTTPError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
   }
   ```

#### 2c. Add invalidateServerCache helper

```go
func (c *Client) invalidateServerCache() {
    c.serverMu.Lock()
    c.serverUp = false
    c.serverChecked = time.Time{} // zero time forces re-probe on next call
    c.serverMu.Unlock()
}
```

Call this from `httpGetJSON` and `httpPostJSON` when a transport error occurs.

---

### Step 3 — Add streaming HTTP client

**File**: `internal/smithers/client.go`

The existing `httpClient` has `Timeout: 10s` which terminates SSE connections. Add a second client for streaming:

In the `Client` struct, add:
```go
streamClient *http.Client // no timeout — used for SSE streams
```

In `NewClient`, after constructing the main `httpClient`:
```go
c.streamClient = &http.Client{
    Transport: c.httpClient.Transport, // share transport (and its connection pool)
    // No Timeout — SSE streams are long-lived
}
```

If `WithHTTPClient` is used (tests), derive `streamClient` from it with `Timeout: 0`:
```go
func WithHTTPClient(hc *http.Client) ClientOption {
    return func(c *Client) {
        c.httpClient = hc
        c.streamClient = &http.Client{Transport: hc.Transport}
    }
}
```

---

### Step 4 — Implement run-lifecycle HTTP methods

**File**: `internal/smithers/client.go`

Add the following methods. Each follows the same three-tier pattern as existing methods. Note that run-scope operations use path `/v1/runs` (upstream Smithers v1 API) rather than the older `/ps` / `/run` paths assumed by the engineering spec's earlier draft — implement against the actual upstream path.

#### ListRuns
```
HTTP:  GET /v1/runs
SQLite: SELECT id, workflow_path, status, started_at, finished_at, error
        FROM _smithers_runs ORDER BY started_at DESC LIMIT 100
exec:  smithers ps --format json
```

#### GetRun
```
HTTP:  GET /v1/runs/:id
SQLite: query _smithers_runs + join _smithers_nodes + _smithers_approvals
exec:  smithers inspect <runID> --format json
```

#### CancelRun
```
HTTP:  POST /v1/runs/:id/cancel
exec:  smithers cancel <runID>
(no SQLite — mutation)
```

#### ApproveNode
```
HTTP:  POST /v1/runs/:runID/approve/:nodeID
exec:  smithers approve <runID> --node <nodeID>
(no SQLite — mutation)
```

#### DenyNode
```
HTTP:  POST /v1/runs/:runID/deny/:nodeID
exec:  smithers deny <runID> --node <nodeID>
(no SQLite — mutation)
```

#### RunWorkflow
```
HTTP:  POST /v1/runs  body: {workflowPath, inputs}
exec:  smithers up <workflowPath> --input <key=val>... --detach
(no SQLite — mutation)
```

#### HijackRun
```
HTTP:  POST /v1/runs/:id/hijack
exec:  smithers hijack <runID> --format json
(no SQLite — mutation)
```

For each method, follow the existing pattern:
1. `if c.isServerAvailable() { ... HTTP ... if err == nil { return } }`
2. `if c.db != nil { ... SQLite ... }` (read-only methods only)
3. `c.execSmithers(ctx, args...)` as final fallback

---

### Step 5 — Implement SSE stream consumer

**File**: `internal/smithers/events.go` (new file)

This file handles `text/event-stream` parsing. It must be kept transport-only — no UI logic.

#### Function signature
```go
// StreamRunEvents opens an SSE stream for a specific run's events.
// Events are sent on the returned channel until ctx is cancelled or the connection drops.
// afterSeq specifies the last sequence number seen; pass -1 to receive all events.
func (c *Client) StreamRunEvents(ctx context.Context, runID string, afterSeq int64) (<-chan RunEvent, <-chan error)

// StreamWorkspaceEvents opens an SSE stream for all workspace events.
func (c *Client) StreamWorkspaceEvents(ctx context.Context, afterSeq int64) (<-chan RunEvent, <-chan error)
```

#### Implementation notes

- Use `c.streamClient` (no timeout) for the GET request.
- Set `Accept: text/event-stream` header.
- Append `?afterSeq=N` query parameter.
- Parse SSE line-by-line:
  - Lines starting with `:` are keep-alive comments — skip.
  - `event:` lines set the event type — only process `event: smithers`.
  - `data:` lines contain the JSON payload — unmarshal into `RunEvent`.
  - Blank lines delimit events (flush).
- Send parsed events on the channel; drop and log if the channel is full (non-blocking send with default case).
- On `ctx.Done()`, drain in-flight reads and close the channel.
- On connection drop (EOF or read error), send error on the error channel and close both channels — the caller decides whether to reconnect.
- Use `bufio.NewReader` on the response body, not line-by-line `Scanner`, to handle large data lines.

#### Reconnect responsibility
The `StreamRunEvents` function itself does NOT reconnect — it terminates on drop. The caller (typically a Bubble Tea view) is responsible for reconnecting by calling `StreamRunEvents` again with the last received `seq`. This keeps the function simple and testable.

---

### Step 6 — Fix SQLite table name drift

**File**: `internal/smithers/client.go`

| Location | Current (wrong) | Correct |
|---|---|---|
| `ListCrons` SQLite query | `_smithers_crons` | `_smithers_cron` |
| `GetScores` SQLite query | `_smithers_scorer_results` | `_smithers_scorers` (verify against current upstream schema first) |

Search for both strings and update. Add a comment citing the upstream schema source file (`smithers/src/db/internal-schema.ts`).

---

### Step 7 — Fix CLI fallback drift

**File**: `internal/smithers/client.go`

| Method | Current exec args (wrong) | Correct exec args |
|---|---|---|
| `ExecuteSQL` | `smithers sql --query Q --format json` | Remove this exec path; if no HTTP and no SQLite, return `ErrNoTransport` with a note that SQL requires a running server |
| `ToggleCron` | `smithers cron toggle <id> --enabled <bool>` | `smithers cron enable <id>` if true, `smithers cron disable <id>` if false |

For `ExecuteSQL`, the exec fallback cannot work (the CLI has no `sql` subcommand). Return `ErrNoTransport` with message `"SQL requires smithers server; start with: smithers up --serve"`.

---

### Step 8 — Wire config into client construction

**File**: `internal/ui/model/ui.go`

Find the line that calls `smithers.NewClient()` with no arguments (around line 332 per research doc). Replace with:

```go
smithersOpts := []smithers.ClientOption{}
if cfg.Smithers.APIURL != "" {
    smithersOpts = append(smithersOpts, smithers.WithAPIURL(cfg.Smithers.APIURL))
}
if cfg.Smithers.APIToken != "" {
    smithersOpts = append(smithersOpts, smithers.WithAPIToken(cfg.Smithers.APIToken))
} else if token := os.Getenv("SMITHERS_API_KEY"); token != "" {
    smithersOpts = append(smithersOpts, smithers.WithAPIToken(token))
}
if cfg.Smithers.DBPath != "" {
    smithersOpts = append(smithersOpts, smithers.WithDBPath(cfg.Smithers.DBPath))
}
m.smithersClient = smithers.NewClient(smithersOpts...)
```

Verify that `cfg.Smithers` field names match `internal/config/config.go`. Adjust field names as needed.

Also ensure `m.smithersClient.Close()` is called in the UI model's cleanup path (look for `Close()` / `tea.Quit` handling).

---

### Step 9 — Write tests for new methods

**File**: `internal/smithers/client_test.go`

Add tests following existing patterns. Minimum coverage:

| Test | Transport | What to assert |
|---|---|---|
| `TestListRuns_HTTP` | HTTP | path=`/v1/runs`, method=GET, returns parsed `[]Run` |
| `TestListRuns_Exec` | exec | args=`["ps", "--format", "json"]`, returns parsed `[]Run` |
| `TestGetRun_HTTP` | HTTP | path=`/v1/runs/run-1`, method=GET, returns `RunDetail` |
| `TestCancelRun_HTTP` | HTTP | path=`/v1/runs/run-1/cancel`, method=POST |
| `TestCancelRun_Exec` | exec | args=`["cancel", "run-1"]` |
| `TestApproveNode_HTTP` | HTTP | path=`/v1/runs/run-1/approve/node-1`, method=POST |
| `TestApproveNode_Exec` | exec | args=`["approve", "run-1", "--node", "node-1"]` |
| `TestDenyNode_HTTP` | HTTP | path=`/v1/runs/run-1/deny/node-1`, method=POST |
| `TestHijackRun_HTTP` | HTTP | path=`/v1/runs/run-1/hijack`, returns `HijackSession` |
| `TestHTTPError_401` | HTTP | returns `*HTTPError` with StatusCode=401 |
| `TestHTTPError_503` | HTTP | invalidates server cache (check `serverUp==false`) |
| `TestInvalidateServerCache` | HTTP | transport error → `serverUp` reset to false |
| `TestExecuteSQL_NoTransport` | none | returns `ErrNoTransport` |
| `TestToggleCron_Exec_Enable` | exec | args include `enable` not `toggle --enabled true` |
| `TestToggleCron_Exec_Disable` | exec | args include `disable` not `toggle --enabled false` |

**File**: `internal/smithers/events_test.go` (new file)

| Test | What to assert |
|---|---|
| `TestStreamRunEvents_ParsesEvent` | Given valid `event: smithers\ndata: {...}\n\n`, event arrives on channel |
| `TestStreamRunEvents_SkipsKeepAlive` | Lines starting with `:` do not produce events |
| `TestStreamRunEvents_ContextCancel` | Cancelling ctx closes both channels |
| `TestStreamRunEvents_ConnectionDrop` | EOF on body sends on error channel, closes event channel |
| `TestHijackSession_ResumeArgs` | With token → `["--resume", token]`; without → nil |

Use `httptest.NewServer` with a handler that writes SSE-formatted lines to the response writer, flushing after each event.

---

### Step 10 — Verify ListWorkflows path

**File**: `internal/smithers/client.go`

The engineering doc lists `ListWorkflows` but the upstream server (`src/server/index.ts`) may not expose a `/v1/workflows` endpoint — workflows are discovered from the filesystem. Implement as:
1. `exec smithers workflow list --format json` (primary)
2. No SQLite fallback (no workflow table in internal schema)
3. No HTTP path unless the upstream server adds it

This keeps `ListWorkflows` simple and avoids assuming a route that does not exist.

---

## File Plan

Files to modify:
- `/Users/williamcory/crush/internal/smithers/types.go` — add run-domain types (Step 1)
- `/Users/williamcory/crush/internal/smithers/client.go` — error handling, streaming client, run lifecycle methods, schema/CLI drift fixes, config wiring method (Steps 2–8, 10)
- `/Users/williamcory/crush/internal/smithers/client_test.go` — new tests (Step 9)
- `/Users/williamcory/crush/internal/ui/model/ui.go` — wire config into client construction (Step 8)

Files to create:
- `/Users/williamcory/crush/internal/smithers/events.go` — SSE stream consumer (Step 5)
- `/Users/williamcory/crush/internal/smithers/events_test.go` — SSE tests (Step 9)

---

## Validation

Run in this order:

1. Format: `gofumpt -w internal/smithers`
2. Unit tests: `go test ./internal/smithers/... -count=1 -v`
3. Targeted run-lifecycle tests: `go test ./internal/smithers/... -run 'TestListRuns|TestGetRun|TestCancelRun|TestApproveNode|TestDenyNode|TestHijackRun|TestHTTPError|TestStream' -count=1 -v`
4. Targeted error handling tests: `go test ./internal/smithers/... -run 'TestHTTPError_401|TestHTTPError_503|TestInvalidateServerCache|TestExecuteSQL_NoTransport' -count=1 -v`
5. Targeted cron fix tests: `go test ./internal/smithers/... -run 'TestToggleCron' -count=1 -v`
6. UI package still builds: `go build ./internal/ui/...`
7. Full build: `go build ./...`
8. All tests: `go test ./... -count=1`
9. Manual transport check (if Smithers server is available):
   ```
   cd /Users/williamcory/smithers && bun run src/cli/index.ts serve --root . --port 7331
   curl -s http://127.0.0.1:7331/v1/runs | jq '.[0] // "empty"'
   curl -N 'http://127.0.0.1:7331/v1/runs/<run-id>/events?afterSeq=-1'
   ```
10. Confirm config wiring: launch smithers-tui with `smithers.apiUrl` set in `smithers-tui.json`, open Agents view, verify no panic on startup.

---

## Open Questions

1. **`/v1/runs` vs `/ps`**: The engineering doc uses `/ps` in one section and `/v1/runs` in another. The upstream `src/server/index.ts` research showed `/v1/runs`. Verify the actual running server response shape before implementing `ListRuns` — the `apiEnvelope{ok, data}` wrapper may or may not apply to v1 routes.

2. **`_smithers_scorers` column names**: The research noted a possible table name mismatch. Before updating the `GetScores` SQLite query, read `smithers/src/scorers/schema.ts` to confirm column names match the existing `scanScoreRows` scan order.

3. **`cron enable` / `cron disable` CLI**: Confirm these subcommands exist in `smithers/src/cli/index.ts` before implementing Step 7's cron toggle fix.

4. **SSE reconnect policy**: Should reconnect be automatic in the SSE consumer (with backoff) or left to the caller? Leaving it to the caller (Step 5) is simpler for testing but puts burden on every view. Consider a `StreamWithReconnect` wrapper in `events.go` that handles exponential backoff reconnect using the last received `seq`, but do not implement it in this ticket unless required by the runs view.

5. **`SMITHERS_API_KEY` env var fallback**: Step 8 reads the env var as a fallback when no config token is set. Verify this matches the PRD's config priority order (`smithers-tui.json` > `~/.config/...` > env vars). The current reading order in Step 8 sets env var only when config token is empty, which is correct per the priority list.

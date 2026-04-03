# Implement HTTP Client for Runs API

## Metadata
- ID: eng-smithers-client-runs
- Group: Runs And Inspection (runs-and-inspection)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Implement the Smithers HTTP client and SSE event stream consumer to support fetching run state and real-time updates.

## Acceptance Criteria

- Client can fetch the list of runs from /api/runs
- Client can fetch specific run details and DAG from /api/runs/:id
- Client can stream SSE events for run status updates
- Client exposes Approve, Deny, Cancel, and Hijack mutations
- Unit tests cover client serialization and API errors

## Source Context

- internal/smithers/client.go
- internal/smithers/types.go
- ../smithers/src/server/routes/runs.ts

## Implementation Notes

- Ensure dual-mode access (HTTP if running, SQLite fallback if direct db exists).
- Events should be translated into tea.Msg types for easy consumption by the TUI loop.

---

## Objective

Build the `internal/smithers/` Go package into a production-ready HTTP and SSE client that can list runs, fetch run details (including nodes/attempts/approvals), stream real-time events, and execute mutations (approve, deny, cancel, hijack). This client is the data backbone for the Runs Dashboard view (`internal/ui/views/runs.go`) and every downstream feature that consumes run state. The architecture must support dual-mode access: HTTP API when the Smithers server is running, and direct SQLite reads when only the local database is available.

## Scope

### In scope

1. **Run list endpoint** — `GET /v1/runs` with optional `status` and `limit` query params.
2. **Run detail endpoint** — `GET /v1/runs/:runId` returning run metadata, node summary, and DAG structure.
3. **SSE event stream** — `GET /v1/runs/:runId/events?afterSeq=N` consuming `event: smithers` frames, including heartbeat handling and automatic reconnection.
4. **Mutations** — `POST /v1/runs/:runId/nodes/:nodeId/approve`, `.../deny`, `POST /v1/runs/:runId/cancel`, hijack request via `POST /v1/runs/:runId/cancel` (hijack is modeled as a cancel with hijack target in upstream — verify and adapt).
5. **Go type definitions** — `Run`, `Node`, `Attempt`, `Approval`, `SmithersEvent` (union type via tagged discriminator), `RunFilter`, `RunDetail`.
6. **Bubble Tea integration** — Every async result (HTTP response, SSE event, error) wrapped as a `tea.Msg` so the TUI Update loop can consume it directly.
7. **Dual-mode transport** — HTTP-first, SQLite-read fallback for `ListRuns` and `GetRun` when no server is reachable.
8. **Unit tests** — Serialization round-trips, error mapping, SSE line parsing, reconnection logic.
9. **Terminal E2E test** — At least one test modeled on the upstream `@microsoft/tui-test`-style harness that spawns the TUI binary, sends keystrokes, and asserts rendered text.
10. **VHS happy-path tape** — One `.tape` file demonstrating the runs-list flow end-to-end.

### Out of scope

- The Runs Dashboard UI view (`runs.go`) — that is a separate ticket.
- Time-travel endpoints (diff/fork/replay) — separate ticket `eng-time-travel-api-and-model`.
- Chat streaming (`GET /v1/runs/:runId/frames`) — separate ticket under live-chat.
- Workflow and agent client methods — separate tickets `eng-smithers-workflows-client` and agent tickets.
- MCP tool integration — consumed by the chat agent, not by the runs client.

## Implementation Plan

### Slice 1: Go type definitions (`internal/smithers/types.go`)

**Goal**: Define all types needed to represent Smithers run state in Go, grounded in the upstream Drizzle schema (`smithers/src/db/internal-schema.ts`) and the upstream event types (`smithers/src/SmithersEvent.ts`).

**Files**: `internal/smithers/types.go`

**Types to add** (alongside the existing `Agent` struct):

```go
// RunStatus enumerates the possible states of a Smithers run.
type RunStatus string

const (
    RunStatusRunning         RunStatus = "running"
    RunStatusWaitingApproval RunStatus = "waiting-approval"
    RunStatusWaitingEvent    RunStatus = "waiting-event"
    RunStatusFinished        RunStatus = "finished"
    RunStatusFailed          RunStatus = "failed"
    RunStatusCancelled       RunStatus = "cancelled"
)

// Run mirrors smithersRuns from internal-schema.ts.
type Run struct {
    RunID        string    `json:"runId"`
    WorkflowName string    `json:"workflowName"`
    WorkflowPath string    `json:"workflowPath,omitempty"`
    Status       RunStatus `json:"status"`
    CreatedAtMs  int64     `json:"createdAtMs"`
    StartedAtMs  *int64    `json:"startedAtMs,omitempty"`
    FinishedAtMs *int64    `json:"finishedAtMs,omitempty"`
    ErrorJSON    string    `json:"errorJson,omitempty"`
    ConfigJSON   string    `json:"configJson,omitempty"`
}

// NodeState enumerates the possible states of a workflow node.
type NodeState string

const (
    NodeStatePending         NodeState = "pending"
    NodeStateInProgress      NodeState = "in-progress"
    NodeStateFinished        NodeState = "finished"
    NodeStateFailed          NodeState = "failed"
    NodeStateCancelled       NodeState = "cancelled"
    NodeStateSkipped         NodeState = "skipped"
    NodeStateWaitingApproval NodeState = "waiting-approval"
)

// Node mirrors smithersNodes from internal-schema.ts.
type Node struct {
    RunID       string    `json:"runId"`
    NodeID      string    `json:"nodeId"`
    Iteration   int       `json:"iteration"`
    State       NodeState `json:"state"`
    LastAttempt *int      `json:"lastAttempt,omitempty"`
    UpdatedAtMs int64     `json:"updatedAtMs"`
    Label       string    `json:"label,omitempty"`
}

// Attempt mirrors smithersAttempts from internal-schema.ts.
type Attempt struct {
    RunID        string  `json:"runId"`
    NodeID       string  `json:"nodeId"`
    Iteration    int     `json:"iteration"`
    Attempt      int     `json:"attempt"`
    State        string  `json:"state"`
    StartedAtMs  int64   `json:"startedAtMs"`
    FinishedAtMs *int64  `json:"finishedAtMs,omitempty"`
    ErrorJSON    string  `json:"errorJson,omitempty"`
    Cached       bool    `json:"cached"`
    ResponseText string  `json:"responseText,omitempty"`
}

// Approval mirrors smithersApprovals from internal-schema.ts.
type Approval struct {
    RunID         string `json:"runId"`
    NodeID        string `json:"nodeId"`
    Iteration     int    `json:"iteration"`
    Status        string `json:"status"`
    RequestedAtMs *int64 `json:"requestedAtMs,omitempty"`
    DecidedAtMs   *int64 `json:"decidedAtMs,omitempty"`
    Note          string `json:"note,omitempty"`
    DecidedBy     string `json:"decidedBy,omitempty"`
}

// RunDetail is the enriched response from GET /v1/runs/:runId.
type RunDetail struct {
    Run
    Summary map[string]int `json:"summary,omitempty"` // state → count
    Nodes   []Node         `json:"nodes,omitempty"`
}

// RunFilter controls query parameters for ListRuns.
type RunFilter struct {
    Status RunStatus
    Limit  int
}

// SmithersEvent is the envelope for all SSE events.
// The Type field is the discriminator; Payload holds the decoded sub-type.
type SmithersEvent struct {
    Type        string `json:"type"`
    TimestampMs int64  `json:"timestampMs"`
    RunID       string `json:"runId"`
    // Flattened fields present on node-level events:
    NodeID    string `json:"nodeId,omitempty"`
    Iteration int    `json:"iteration,omitempty"`
    Attempt   int    `json:"attempt,omitempty"`
    // Additional payload fields (error, status, etc.) stored as raw JSON
    // for type-specific handling.
    Raw json.RawMessage `json:"-"`
}
```

**Verification**: `go vet ./internal/smithers/...` passes; JSON round-trip tests for every struct.

---

### Slice 2: Client struct and HTTP transport (`internal/smithers/client.go`)

**Goal**: Replace the stub `Client` struct with a real HTTP client that can reach the Smithers server.

**Files**: `internal/smithers/client.go`

**Design**:

```go
type Client struct {
    httpClient *http.Client
    baseURL    string        // e.g. "http://localhost:7331/v1"
    apiToken   string        // Bearer token for auth
    dbPath     string        // path to smithers.db for fallback reads
    mu         sync.RWMutex  // guards cachedRuns for status bar
    cachedRuns []Run
}

type ClientConfig struct {
    APIURL   string
    APIToken string
    DBPath   string
}

func NewClient(cfg ClientConfig) *Client
```

**Key behaviors**:
- `NewClient` validates the base URL (strips trailing slash, appends `/v1` if missing).
- Stores a pre-configured `*http.Client` with a 10-second default timeout for non-streaming requests.
- All HTTP requests attach `Authorization: Bearer <token>` when `apiToken` is non-empty.
- The `doRequest` helper handles JSON decoding, maps HTTP 4xx/5xx to typed Go errors (`ErrNotFound`, `ErrConflict`, `ErrServerDown`).
- A `Ping(ctx) error` method hits `GET /metrics` (lightweight) to check server reachability, used to decide HTTP-vs-SQLite mode.

**Config injection**: The `ClientConfig` is populated from the smithers section of the TUI config (`internal/config/config.go`). The existing config loader already reads `smithers.apiUrl`, `smithers.apiToken`, and `smithers.dbPath` from `smithers-tui.json`.

---

### Slice 3: ListRuns and GetRun (`internal/smithers/client.go`)

**Goal**: Implement the two read endpoints that the Runs Dashboard needs on initial load.

**Methods**:

```go
// ListRuns fetches runs from GET /v1/runs.
// Falls back to direct SQLite SELECT if the server is unreachable.
func (c *Client) ListRuns(ctx context.Context, f RunFilter) ([]Run, error)

// GetRun fetches a single run with node summary from GET /v1/runs/:runId.
func (c *Client) GetRun(ctx context.Context, runID string) (*RunDetail, error)
```

**HTTP path**: `GET /v1/runs?status=<f.Status>&limit=<f.Limit>`, `GET /v1/runs/<runID>`.

**SQLite fallback** (read-only, `ListRuns` only):
- Open `dbPath` with `?mode=ro&_journal_mode=WAL`.
- `SELECT * FROM _smithers_runs ORDER BY createdAtMs DESC LIMIT ?`.
- Rows are scanned into `[]Run`. No node or event data in fallback mode.
- Fallback is only attempted when `Ping` returns `ErrServerDown`.

**Bubble Tea messages**:

```go
// RunsLoadedMsg carries the result of ListRuns into the TUI Update loop.
type RunsLoadedMsg struct {
    Runs []Run
    Err  error
}

// RunDetailMsg carries the result of GetRun.
type RunDetailMsg struct {
    Detail *RunDetail
    Err    error
}
```

A helper `FetchRunsCmd` returns a `tea.Cmd` that performs the HTTP call and wraps the result:

```go
func FetchRunsCmd(client *Client, filter RunFilter) tea.Cmd {
    return func() tea.Msg {
        runs, err := client.ListRuns(context.Background(), filter)
        return RunsLoadedMsg{Runs: runs, Err: err}
    }
}
```

---

### Slice 4: SSE event stream (`internal/smithers/events.go`)

**Goal**: Consume the `GET /v1/runs/:runId/events?afterSeq=N` SSE stream and emit `tea.Msg` values for each event.

**File**: `internal/smithers/events.go`

**Design**:

```go
// StreamEvents opens an SSE connection and sends parsed events to the
// returned channel. The channel is closed when the stream ends (run
// reaches terminal state) or the context is cancelled.
func (c *Client) StreamEvents(ctx context.Context, runID string, afterSeq int) (<-chan SmithersEvent, error)
```

**SSE parsing**:
- Read response body line-by-line (`bufio.Scanner`).
- Lines starting with `event:` set the current event type (expect `smithers`).
- Lines starting with `data:` contain the JSON payload — unmarshal into `SmithersEvent`.
- Lines starting with `:` are comments/heartbeats — reset a 30-second inactivity timer.
- Blank lines delimit events.
- `retry:` lines update the reconnection interval.

**Reconnection**:
- On EOF or network error, wait `retryMs` (default 1000ms from server), then reconnect with `afterSeq` set to the last received `seq`.
- Cap reconnection attempts at 10 before closing the channel with a final error event.
- Respect `ctx.Done()` for clean cancellation.

**Bubble Tea integration**:
- `SubscribeEventsCmd(client *Client, runID string, afterSeq int) tea.Cmd` — returns a `tea.Cmd` that opens the stream and returns the first event. Subsequent events use `tea.Batch` chaining (each event handler re-subscribes for the next event).
- Alternatively, use a background goroutine that calls `p.Send(msg)` on the `tea.Program` — this is the simpler pattern used in Charm examples for long-lived subscriptions. Document both options; prefer the `p.Send` approach for SSE since it avoids recursive `tea.Cmd` chaining.

**Event message type**:

```go
// RunEventMsg wraps a single SSE event for the TUI.
type RunEventMsg struct {
    Event SmithersEvent
    Err   error  // non-nil on stream error / disconnect
    Done  bool   // true when stream closed (terminal run state)
}
```

---

### Slice 5: Mutations — Approve, Deny, Cancel (`internal/smithers/client.go`)

**Goal**: Implement the three mutation endpoints needed for quick actions from the Runs Dashboard.

**Methods**:

```go
// Approve approves a waiting-approval node.
// POST /v1/runs/:runId/nodes/:nodeId/approve
func (c *Client) Approve(ctx context.Context, runID, nodeID string, opts ApproveOpts) error

// Deny denies a waiting-approval node.
// POST /v1/runs/:runId/nodes/:nodeId/deny
func (c *Client) Deny(ctx context.Context, runID, nodeID string, opts DenyOpts) error

// Cancel cancels an active run.
// POST /v1/runs/:runId/cancel
func (c *Client) Cancel(ctx context.Context, runID string) error
```

**`ApproveOpts` / `DenyOpts`**:

```go
type ApproveOpts struct {
    Iteration int    `json:"iteration,omitempty"`
    Note      string `json:"note,omitempty"`
    DecidedBy string `json:"decidedBy,omitempty"`
}

type DenyOpts = ApproveOpts // same shape in upstream
```

**Bubble Tea messages**:

```go
type MutationResultMsg struct {
    Action string // "approve", "deny", "cancel"
    RunID  string
    NodeID string
    Err    error
}
```

**Mutations have no SQLite fallback** — they always require the HTTP server. If the server is down, return a clear error: `ErrServerRequired`.

---

### Slice 6: Hijack request (`internal/smithers/client.go`)

**Goal**: Send a hijack request so the TUI can later hand off to the agent's native CLI.

The upstream models hijack as two phases:
1. **Request hijack** — sets `hijackRequestedAtMs` and `hijackTarget` on the run record. The running agent sees this flag and pauses.
2. **Launch agent CLI** — the TUI uses `tea.ExecProcess` to hand off (handled by the UI layer, not the client).

**Method**:

```go
// HijackRun requests that the Smithers engine pause the agent on the given
// run so the user can take over. Returns the session token or resume
// identifier needed to launch the agent's CLI.
func (c *Client) HijackRun(ctx context.Context, runID string) (*HijackSession, error)

type HijackSession struct {
    RunID        string `json:"runId"`
    NodeID       string `json:"nodeId"`
    Engine       string `json:"engine"`       // "claude-code", "codex", etc.
    ResumeToken  string `json:"resumeToken"`  // session token for --resume
    Cwd          string `json:"cwd"`          // working directory
}
```

**Upstream endpoint**: This maps to the `RunHijackRequested` event flow. The exact REST endpoint may be `POST /v1/runs/:runId/hijack` or may piggyback on the cancel endpoint with a hijack target. Verify against the upstream server routes at implementation time — the server index (`smithers/src/server/index.ts`) is authoritative.

**Mismatch note**: The upstream server has `hijackRequestedAtMs` and `hijackTarget` columns on the runs table but the REST surface for triggering hijack may not be a dedicated endpoint yet. If no endpoint exists, implement via shell-out to `smithers hijack <runId>` as a temporary bridge, returning parsed JSON output.

---

### Slice 7: Unit tests (`internal/smithers/client_test.go`, `events_test.go`, `types_test.go`)

**Goal**: Thorough unit test coverage for serialization, transport, error handling, and SSE parsing.

**Files**:
- `internal/smithers/types_test.go` — JSON marshal/unmarshal round-trips for all types.
- `internal/smithers/client_test.go` — HTTP client tests using `httptest.NewServer` for mock responses.
- `internal/smithers/events_test.go` — SSE line parser tests, reconnection logic, heartbeat timeout.

**Test cases**:

| Test | What it verifies |
|------|-----------------|
| `TestRunJSONRoundTrip` | All Run fields survive marshal→unmarshal |
| `TestNodeStateConstants` | NodeState values match upstream strings |
| `TestSmithersEventParsing` | Raw SSE `data:` line → `SmithersEvent` |
| `TestListRuns_HTTP` | Mock server returns JSON array → `[]Run` |
| `TestListRuns_FilterByStatus` | Query param correctly appended |
| `TestListRuns_ServerDown_SQLiteFallback` | Unreachable server → SQLite read |
| `TestGetRun_NotFound` | 404 → `ErrNotFound` |
| `TestApprove_Success` | 200 → nil error, correct POST body |
| `TestApprove_ServerDown` | Unreachable → `ErrServerRequired` |
| `TestStreamEvents_ParseLines` | Multi-line SSE frame → event + seq tracking |
| `TestStreamEvents_Heartbeat` | Comment-only line resets inactivity timer |
| `TestStreamEvents_Reconnect` | EOF → reconnect with updated afterSeq |
| `TestStreamEvents_ContextCancel` | ctx.Done → channel closed cleanly |

---

### Slice 8: Terminal E2E test (`tests/tui_runs_e2e_test.go`)

**Goal**: One end-to-end test that spawns the Smithers TUI binary, navigates to the runs view, and asserts rendered output — modeled on the upstream `@microsoft/tui-test`-style harness in `smithers/tests/tui-helpers.ts`.

**File**: `tests/tui_runs_e2e_test.go`

**Harness design** (Go port of `tui-helpers.ts:BunSpawnBackend`):

```go
type TUITestInstance struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bytes.Buffer
    mu     sync.Mutex
}

func launchTUI(t *testing.T, args ...string) *TUITestInstance
func (t *TUITestInstance) SendKeys(text string)
func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error
func (t *TUITestInstance) WaitForNoText(text string, timeout time.Duration) error
func (t *TUITestInstance) Snapshot() string
func (t *TUITestInstance) Terminate()
```

Key details from upstream patterns:
- Set `TERM=xterm-256color`, `COLORTERM=truecolor`, `LANG=en_US.UTF-8` in process env.
- Strip ANSI escape sequences before text matching: `regexp.MustCompile(\x1B\[[0-9;]*[a-zA-Z])`.
- Poll interval: 100ms. Default timeout: 10s.

**Test case**: `TestTUI_RunsDashboard_E2E`

1. Start a mock Smithers HTTP server (httptest) returning canned `/v1/runs` JSON with 2 runs (one running, one waiting-approval).
2. Launch TUI binary with `SMITHERS_API_URL` pointed at mock server.
3. `WaitForText("SMITHERS")` — confirm TUI launched.
4. `SendKeys("\x12")` — Ctrl+R to open runs view.
5. `WaitForText("Runs")` — confirm view switch.
6. `WaitForText("running")` — confirm run data rendered.
7. `WaitForText("waiting-approval")` — confirm second run visible.
8. `SendKeys("\x1b")` — Esc to return to chat.
9. `WaitForNoText("Runs")` — confirm back navigation.
10. `Terminate()`.

---

### Slice 9: VHS happy-path tape (`tests/tapes/runs_dashboard.tape`)

**Goal**: One VHS recording that demonstrates the runs dashboard flow for visual regression.

**File**: `tests/tapes/runs_dashboard.tape`

```tape
# Smithers TUI — Runs Dashboard happy path
Output runs_dashboard.gif
Set Shell "bash"
Set Width 120
Set Height 40
Set Theme "Catppuccin Mocha"

# Start TUI with mock server
Type "SMITHERS_API_URL=http://localhost:17331 ./smithers-tui"
Enter
Sleep 2s

# Navigate to runs
Ctrl+R
Sleep 1s

# Scroll through runs
Down
Sleep 500ms
Down
Sleep 500ms

# Open run details
Enter
Sleep 1s

# Return to runs list
Escape
Sleep 500ms

# Return to chat
Escape
Sleep 500ms
```

**CI integration**: Run `vhs tests/tapes/runs_dashboard.tape` in CI. Compare output GIF against a golden reference or simply verify the command exits 0 (basic smoke test).

## Validation

### Automated checks

| Check | Command | What it verifies |
|-------|---------|-----------------|
| Type correctness | `go vet ./internal/smithers/...` | No type errors in new code |
| Unit tests | `go test ./internal/smithers/... -v` | All Slice 7 tests pass |
| Race detection | `go test -race ./internal/smithers/...` | No data races in SSE goroutines |
| Build | `go build ./...` | Full project compiles with new types |
| Terminal E2E | `go test ./tests/ -run TestTUI_RunsDashboard_E2E -v -timeout 30s` | Slice 8 test passes: TUI launches, navigates to runs, renders data, returns to chat |
| VHS recording | `vhs tests/tapes/runs_dashboard.tape` | Tape renders without error (exit 0) |

### Manual verification paths

1. **Start Smithers server**: `cd ../smithers && smithers up --serve` (or use a known project with runs).
2. **Launch TUI**: `go run . --smithers-api-url http://localhost:7331`
3. **Verify run list fetch**: Press `Ctrl+R` → confirm run list loads with real data from the server.
4. **Verify real-time updates**: Start a workflow run in another terminal (`smithers up workflow.tsx`) → confirm the runs view updates without manual refresh.
5. **Verify approve mutation**: Find a run in `waiting-approval` status → press `a` → confirm the run status changes to `running` (or the next state).
6. **Verify cancel mutation**: Select a running run → press `x` → confirm status changes to `cancelled`.
7. **Verify SQLite fallback**: Stop the Smithers server → press `Ctrl+R` → confirm run list still loads (from local DB), but mutations show "server required" error.
8. **Verify SSE reconnection**: Open runs view → kill and restart the Smithers server → confirm events resume after ~1 second without user action.

### Terminal E2E coverage (modeled on upstream harness)

The E2E test in Slice 8 directly mirrors the upstream pattern in `smithers/tests/tui-helpers.ts`:
- **Process spawning**: `exec.Command` with piped stdin/stdout (Go equivalent of `Bun.spawn()`)
- **Text assertions**: `WaitForText`/`WaitForNoText` with ANSI stripping and polling (same as `BunSpawnBackend.waitForText`)
- **Keystroke injection**: `SendKeys` writing to stdin (same as `BunSpawnBackend.sendKeys`)
- **Cleanup**: `t.Cleanup()` ensures process termination (Go equivalent of `onTestFinished`)

The test covers the critical path: launch → navigate to runs → verify data renders → navigate back.

### VHS happy-path coverage

The `.tape` file in Slice 9 provides a visual recording of the same flow. This serves as:
- A **regression detector** for layout/styling changes.
- A **documentation artifact** (the output GIF shows exactly what the user sees).
- A **smoke test** in CI (tape execution failure = broken TUI).

## Risks

### 1. Upstream hijack endpoint may not exist as REST

**Risk**: The Smithers server has `hijackRequestedAtMs` and `hijackTarget` columns, and the `RunHijackRequested` event type, but the v1 REST route for triggering hijack may not be implemented yet. The GUI likely used a different transport or the hijack was CLI-only.

**Mitigation**: Slice 6 includes a shell-out fallback (`smithers hijack <runId> --json`). If the REST endpoint doesn't exist, use this bridge and file an upstream ticket to add `POST /v1/runs/:runId/hijack`.

### 2. GUI transport divergence

**Risk**: The GUI's `transport.ts` uses different endpoints (`/ps`, `/node/:runId`) than the v1 server API (`/v1/runs`, `/v1/runs/:runId`). The TUI targets the v1 API, but some v1 routes may have been added after the GUI was built and could have subtle behavior differences.

**Mitigation**: Ground all implementation on the actual server route handlers in `smithers/src/server/index.ts`, not on the GUI transport. Write integration tests against a real Smithers server instance during development to catch mismatches early.

### 3. SSE stream reliability

**Risk**: The upstream SSE implementation uses 500ms DB polling internally. Under high event volume, the client may receive bursts of events that need to be processed without blocking the TUI render loop.

**Mitigation**: The SSE consumer runs in a background goroutine. Events are sent via `p.Send()` (Bubble Tea's thread-safe message injection). The `RunEventMsg` is a lightweight struct — the TUI can process dozens per render frame without jank.

### 4. SQLite fallback WAL locking

**Risk**: If the Smithers server is actively writing to the database, a concurrent read-only SQLite connection from the TUI could encounter WAL checkpoint contention on macOS.

**Mitigation**: Open the fallback connection with `?mode=ro&_journal_mode=WAL&_busy_timeout=5000`. The 5-second busy timeout handles brief contention. For sustained contention, surface a user-visible warning suggesting they start the HTTP server instead.

### 5. Missing `uiSmithersView` Draw() in root model

**Risk**: The Crush UI model has `uiSmithersView` state handling in `Update()` (lines 1721-1733 of `ui.go`) but the agent research indicates the `Draw()` case for rendering Smithers views to screen may be incomplete or missing. If so, the runs view would update state but render nothing.

**Mitigation**: This is a prerequisite fix, not part of this ticket's scope, but must be verified before integration testing. If `Draw()` doesn't handle `uiSmithersView`, add a minimal case that calls `m.viewRouter.Current().View()` and renders the result. File as a blocker if not already tracked.

### 6. Client initialization timing

**Risk**: The current `smithers.NewClient()` is called locally in the UI model constructor. The new `NewClient(cfg)` requires config values that may not be available until after config loading completes in `internal/app/app.go`.

**Mitigation**: Wire `ClientConfig` from the app's config store during `app.New()` initialization, and pass the constructed `*Client` into the UI model constructor — matching how other services (sessions, agent coordinator) are already injected.

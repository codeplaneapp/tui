# Implement HTTP Client for Runs API

## Metadata
- ID: eng-smithers-client-runs
- Group: Runs And Inspection (runs-and-inspection)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Objective

Extend `internal/smithers/client.go` and `internal/smithers/types.go` to provide a complete Go HTTP client for the Smithers server's runs API surface. The client must support listing runs, fetching run details, consuming SSE event streams for real-time status updates, and issuing approve/deny/cancel mutations. All responses must be translated into Go types that can be consumed by the TUI's Bubble Tea update loop as `tea.Msg` values, following the pattern established by the existing `AgentsView` in `internal/ui/views/agents.go`.

This is the data layer prerequisite for the Runs Dashboard view (`RUNS_DASHBOARD`, `RUNS_REALTIME_STATUS_UPDATES`, `RUNS_QUICK_APPROVE`, `RUNS_QUICK_DENY`, `RUNS_QUICK_CANCEL`) and for every downstream feature that needs run state: live chat, hijack, approvals, and time-travel.

## Scope

### In scope
- Go types mirroring the upstream Smithers run model (`Run`, `RunNode`, `SmithersEvent`, `RunFilter`, `NodeStateSummary`)
- HTTP client methods for all `/v1/runs` endpoints exposed by `../smithers/src/server/index.ts`
- SSE consumer that translates the `smithers` event stream into `tea.Msg` values
- Shell-out fallback for `smithers ps --json` when no HTTP server is available
- Unit tests covering JSON deserialization, error handling, and SSE parsing
- Terminal E2E test skeleton modeled on upstream `../smithers/tests/tui.e2e.test.ts`
- VHS happy-path recording test

### Out of scope
- The Runs Dashboard UI view itself (separate ticket `runs-dashboard`)
- Direct SQLite access (deferred; the PRD §11 lists it as a fallback, but the server API is the primary channel)
- Time-travel frame endpoints (`/v1/runs/:id/frames`) — covered by `eng-time-travel-api-and-model`
- MCP tool wrappers for runs — covered by `feat-mcp-runs-tools`

## Implementation Plan

### Slice 1: Go types for the runs domain

**File**: `internal/smithers/types.go`

Add types that mirror the upstream server's response shapes. The canonical source is:
- `../smithers/src/RunStatus.ts` → `RunStatus` string enum
- `../smithers/src/SmithersEvent.ts` → `SmithersEvent` discriminated union
- `../smithers/src/cli/tui-v2/shared/types.ts` → `RunSummary`, `RunNodeSummary`, `ApprovalSummary`, `TokenUsage`
- `../smithers/src/server/index.ts` lines 858–869 → GET `/v1/runs/:id` response shape

```go
// RunStatus matches ../smithers/src/RunStatus.ts
type RunStatus string

const (
    RunStatusRunning         RunStatus = "running"
    RunStatusWaitingApproval RunStatus = "waiting-approval"
    RunStatusWaitingEvent    RunStatus = "waiting-event"
    RunStatusFinished        RunStatus = "finished"
    RunStatusFailed          RunStatus = "failed"
    RunStatusCancelled       RunStatus = "cancelled"
)

// Run is the top-level run object returned by GET /v1/runs and GET /v1/runs/:id.
type Run struct {
    RunID        string            `json:"runId"`
    WorkflowName string            `json:"workflowName"`
    WorkflowPath string            `json:"workflowPath,omitempty"`
    Status       RunStatus         `json:"status"`
    StartedAtMs  *int64            `json:"startedAtMs"`
    FinishedAtMs *int64            `json:"finishedAtMs"`
    Summary      map[string]int    `json:"summary,omitempty"` // node-state → count
    ErrorJSON    *string           `json:"errorJson,omitempty"`
}

// RunNode mirrors _smithers_nodes rows and RunNodeSummary.
type RunNode struct {
    NodeID      string  `json:"nodeId"`
    Label       *string `json:"label"`
    Iteration   int     `json:"iteration"`
    State       string  `json:"state"`
    LastAttempt *int    `json:"lastAttempt"`
    UpdatedAtMs *int64  `json:"updatedAtMs"`
}

// RunFilter is the query parameters for GET /v1/runs.
type RunFilter struct {
    Limit  int    // default 50
    Status string // optional status filter
}

// SmithersEvent is the envelope for SSE payloads.
// The Type field is the discriminator.
type SmithersEvent struct {
    Type        string `json:"type"`
    RunID       string `json:"runId"`
    NodeID      string `json:"nodeId,omitempty"`
    Iteration   int    `json:"iteration,omitempty"`
    Attempt     int    `json:"attempt,omitempty"`
    Status      string `json:"status,omitempty"`
    TimestampMs int64  `json:"timestampMs"`
    // Catch-all for fields we don't model explicitly.
    Raw json.RawMessage `json:"-"`
}
```

The `SmithersEvent` type uses a flat struct with optional fields rather than a full discriminated-union hierarchy. This keeps deserialization simple; consumers switch on `Type`. The `Raw` field preserves the original JSON for debugging or forwarding.

**Validation**: `go vet ./internal/smithers/...` passes; a table-driven unit test round-trips every `RunStatus` value and a representative subset of event types through `json.Marshal`/`json.Unmarshal`.

---

### Slice 2: HTTP client — list and get runs

**File**: `internal/smithers/client.go`

Expand the existing stub `Client` to hold connection config and implement `ListRuns` and `GetRun`.

```go
type Client struct {
    apiURL     string       // e.g. "http://localhost:7331"
    apiToken   string       // Bearer token (optional)
    httpClient *http.Client
}

type ClientOption func(*Client)

func WithAPIURL(url string) ClientOption   { return func(c *Client) { c.apiURL = url } }
func WithAPIToken(tok string) ClientOption  { return func(c *Client) { c.apiToken = tok } }

func NewClient(opts ...ClientOption) *Client {
    c := &Client{
        apiURL:     "http://localhost:7331",
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
    for _, o := range opts {
        o(c)
    }
    return c
}
```

Key methods:

- `ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)` — `GET /v1/runs?limit=N&status=S`
- `GetRun(ctx context.Context, runID string) (*Run, error)` — `GET /v1/runs/:id`

Both methods:
1. Build the URL from `c.apiURL`.
2. Set `Authorization: Bearer <token>` if `c.apiToken` is non-empty.
3. Decode JSON into the Go types from Slice 1.
4. Map HTTP errors (401, 404, 409, 500) to typed Go errors (`ErrUnauthorized`, `ErrRunNotFound`, `ErrServerError`).

The upstream server (lines 956–969 of `../smithers/src/server/index.ts`) returns an array directly for `GET /v1/runs` and an object with `runId`, `workflowName`, `status`, `startedAtMs`, `finishedAtMs`, `summary` for `GET /v1/runs/:id`. The error envelope is `{ error: { code, message } }`.

**Mismatch note**: The GUI's `transport.ts` uses legacy endpoints (`/ps`, `/node/:runId`). The TUI must use the v1 API (`/v1/runs`, `/v1/runs/:id`) which is the canonical server surface. The legacy endpoints are not present in the current server code (`../smithers/src/server/index.ts`)—they were served by an older Hono layer in `../smithers/src/server/serve.ts`. The v1 endpoints are authoritative.

**Validation**: Unit test with `httptest.Server` returning canned JSON for both endpoints; assert Go structs match. Error path test: 404, 401, malformed JSON.

---

### Slice 3: HTTP client — mutations (approve, deny, cancel)

**File**: `internal/smithers/client.go`

Add mutation methods that map to POST endpoints:

- `Approve(ctx, runID, nodeID string, iteration int, note, decidedBy string) error` → `POST /v1/runs/:id/nodes/:nodeId/approve`
- `Deny(ctx, runID, nodeID string, iteration int, note, decidedBy string) error` → `POST /v1/runs/:id/nodes/:nodeId/deny`
- `Cancel(ctx, runID string) error` → `POST /v1/runs/:id/cancel`

Request bodies mirror the upstream server expectations (lines 900–954 of `../smithers/src/server/index.ts`):
- Approve/Deny: `{ "iteration": N, "note": "...", "decidedBy": "..." }`
- Cancel: empty body

All return `{ runId }` on success. Non-200 responses decode the error envelope.

**Validation**: Unit test with `httptest.Server`; assert correct HTTP method, path, body, and header for each mutation. Test 409 (RUN_NOT_ACTIVE) for cancel on a finished run.

---

### Slice 4: SSE event stream consumer

**File**: `internal/smithers/events.go` (new)

Implement an SSE consumer for `GET /v1/runs/:id/events?afterSeq=N`.

The upstream server (lines 765–841 of `../smithers/src/server/index.ts`):
- Returns `Content-Type: text/event-stream`
- Sends `retry: 1000\n\n` initially
- Polls the DB every 500ms for new events
- Each event: `event: smithers\ndata: <JSON>\n\n`
- Heartbeat: `: keep-alive\n\n` every 10s
- Auto-closes when run reaches terminal state and event queue is drained

Go implementation:

```go
// RunEventMsg is a tea.Msg carrying a SmithersEvent from the SSE stream.
type RunEventMsg struct {
    RunID string
    Event SmithersEvent
}

// RunEventErrorMsg is sent when the SSE stream encounters an error.
type RunEventErrorMsg struct {
    RunID string
    Err   error
}

// RunEventDoneMsg is sent when the SSE stream closes (run reached terminal state).
type RunEventDoneMsg struct {
    RunID string
}

// StreamRunEvents returns a tea.Cmd that opens an SSE connection and sends
// RunEventMsg values to the Bubble Tea program until the stream closes or
// the context is cancelled.
func (c *Client) StreamRunEvents(ctx context.Context, runID string, afterSeq int) tea.Cmd {
    return func() tea.Msg {
        // Implementation: open HTTP GET, parse SSE lines, decode JSON,
        // send messages via tea.Program.Send or return batch.
        // On stream close → RunEventDoneMsg
        // On error → RunEventErrorMsg
    }
}
```

The SSE parser reads line-by-line:
- Lines starting with `event:` set the current event name (always "smithers")
- Lines starting with `data:` append to the data buffer
- Empty lines dispatch the accumulated event
- Lines starting with `:` are comments (heartbeats), ignored
- Lines starting with `retry:` update reconnect interval (stored but not acted on in v1)

Because the SSE stream is long-lived and Bubble Tea expects `tea.Cmd` functions to return a single `tea.Msg`, the stream must use the program reference pattern: the `StreamRunEvents` method takes a `*tea.Program` and sends events via `program.Send()` in a goroutine, returning `nil` immediately. This matches how `app.Subscribe` works in `internal/app/app.go:550–579`.

Alternative: return a `tea.Cmd` that blocks reading the first event, then returns it along with a continuation `tea.Cmd` for the next event (recursive cmd chaining). This is simpler but means one event per Bubble Tea cycle.

**Decision**: Use the `program.Send()` goroutine pattern for consistency with the existing codebase. The `StreamRunEvents` method signature becomes:

```go
func (c *Client) StreamRunEvents(ctx context.Context, program *tea.Program, runID string, afterSeq int)
```

Called from a `tea.Cmd` that launches the goroutine.

**Validation**: Unit test with a test HTTP server that writes canned SSE frames. Assert:
1. Normal events are decoded and sent as `RunEventMsg`
2. Heartbeat comments are silently consumed
3. Stream close sends `RunEventDoneMsg`
4. Context cancellation terminates the goroutine cleanly
5. Malformed JSON in data frame sends `RunEventErrorMsg` but continues reading

---

### Slice 5: Shell-out fallback

**File**: `internal/smithers/exec.go` (new)

When the Smithers HTTP server is not running (connection refused), provide a fallback that shells out to the `smithers` CLI.

```go
// ExecListRuns shells out to `smithers ps --json` and parses the output.
func (c *Client) ExecListRuns(ctx context.Context) ([]Run, error)

// ExecGetRun shells out to `smithers inspect <runID> --json`.
func (c *Client) ExecGetRun(ctx context.Context, runID string) (*Run, error)
```

These are called internally by `ListRuns`/`GetRun` when the HTTP call returns a connection error. The client tries HTTP first, falls back to exec. SSE streaming has no exec fallback (it requires a running server).

The exec path uses `exec.CommandContext` with the provided context for timeout control. Output is expected as JSON on stdout. Non-zero exit codes map to `ErrSmithersCLI`.

**Validation**: Unit test that mocks `exec.Command` via a test helper binary pattern (Go's standard `TestHelperProcess` approach). Verify JSON parsing and error mapping.

---

### Slice 6: Wire client into UI and config

**Files**:
- `internal/smithers/client.go` — update `NewClient` signature
- `internal/ui/model/ui.go` — update `smithersClient` initialization
- `internal/config/config.go` — add Smithers config section

Currently `NewClient()` takes no arguments (`internal/smithers/client.go:10`). Update it to accept `ClientOption` values as designed in Slice 2.

In `internal/ui/model/ui.go:332`, the client is created as `smithers.NewClient()`. Update to pass config:

```go
smithersClient: smithers.NewClient(
    smithers.WithAPIURL(com.Config().SmithersAPIURL()),
    smithers.WithAPIToken(com.Config().SmithersAPIToken()),
),
```

Add to `internal/config/config.go`'s `Options` struct:

```go
type SmithersOptions struct {
    APIURL   string `json:"api_url,omitempty"`   // default "http://localhost:7331"
    APIToken string `json:"api_token,omitempty"` // supports ${ENV_VAR} expansion
    DBPath   string `json:"db_path,omitempty"`   // for future direct-DB fallback
}
```

This matches the config shape described in PRD §9.

**Validation**: Build succeeds. Existing agents view still works (the `ListAgents` method signature is unchanged, just `NewClient` gains optional params).

---

### Slice 7: Integration smoke test

Create `internal/smithers/client_test.go` with a full integration test that:
1. Starts an `httptest.Server` simulating the Smithers v1 API
2. Creates a `Client` pointed at the test server
3. Calls `ListRuns`, `GetRun`, `Approve`, `Deny`, `Cancel`
4. Opens an SSE stream and verifies event delivery
5. Verifies the exec fallback fires when the HTTP server is down

This test uses no external dependencies and runs in `go test ./internal/smithers/...`.

## Validation

### Unit tests

```bash
go test ./internal/smithers/... -v -count=1
```

Expected coverage:
- `types.go`: JSON round-trip for all types, RunStatus enum coverage
- `client.go`: ListRuns, GetRun, Approve, Deny, Cancel against httptest.Server
- `events.go`: SSE parsing (normal events, heartbeats, stream close, malformed data, context cancellation)
- `exec.go`: Shell-out fallback parsing and error handling

### Terminal E2E test (modeled on upstream @microsoft/tui-test harness)

The upstream Smithers TUI E2E tests in `../smithers/tests/tui.e2e.test.ts` use a pattern where:
1. A background workflow is started in `beforeAll` to seed the database
2. The TUI binary is launched as a child process via `launchTUI()` from `../smithers/tests/tui-helpers.ts`
3. The helper class (`BunSpawnBackend`) captures stdout/stderr, strips ANSI codes, and provides `waitForText(text, timeout)`, `waitForNoText(text, timeout)`, `sendKeys(text)`, and `snapshot()` methods
4. Tests navigate views by sending keys and asserting on rendered text

For Crush, create an equivalent Go-based E2E test:

**File**: `tests/e2e/runs_client_e2e_test.go`

```go
func TestRunsClientE2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }
    // 1. Start a mock Smithers HTTP server with canned run data
    srv := startMockSmithersServer(t)
    defer srv.Close()
    
    // 2. Build and launch the TUI binary with SMITHERS_API_URL pointed at mock
    tui := launchTUI(t, "--smithers-api-url", srv.URL)
    defer tui.Terminate()
    
    // 3. Navigate to runs view
    tui.SendKeys("\x12") // Ctrl+R for runs
    tui.WaitForText("SMITHERS › Runs", 5*time.Second)
    
    // 4. Verify run data appears
    tui.WaitForText("code-review", 5*time.Second)
    tui.WaitForText("running", 5*time.Second)
    
    // 5. Navigate and approve
    tui.SendKeys("a") // approve
    tui.WaitForText("Approved", 5*time.Second)
}
```

The `launchTUI` helper mirrors `../smithers/tests/tui-helpers.ts`'s `BunSpawnBackend`:
- Spawns the binary as `exec.Command`
- Captures stdout/stderr into a buffer
- Strips ANSI escape sequences
- Provides `WaitForText(text, timeout)`, `WaitForNoText(text, timeout)`, `SendKeys(text)`, `Snapshot() string`, `Terminate()`

**File**: `tests/e2e/tui_helpers_test.go`

### VHS happy-path recording test

**File**: `tests/vhs/runs_client.tape`

```vhs
Output runs_client.gif
Set Shell bash
Set FontSize 14
Set Width 120
Set Height 40

# Start mock smithers server in background
Type "SMITHERS_API_URL=http://localhost:17331 ./smithers-tui"
Enter
Sleep 2s

# Navigate to runs
Ctrl+R
Sleep 1s

# Verify runs dashboard renders
Sleep 2s

# Select a run and inspect
Down
Enter
Sleep 1s

# Back out
Escape
Sleep 500ms

# Quit
Ctrl+C
```

Run with:
```bash
vhs tests/vhs/runs_client.tape
```

The VHS test produces a GIF recording and exits 0 if the TUI doesn't crash. It validates the happy path: launch → navigate to runs → select run → back → quit.

### Manual verification

1. Start a real Smithers server: `cd ../smithers && bun run src/cli/index.ts up --serve`
2. Start a workflow: `cd ../smithers && bun run src/cli/index.ts up examples/fan-out-fan-in.tsx -d`
3. Launch the TUI: `go run . --smithers-api-url http://localhost:7331`
4. Verify: `ListRuns` returns the running workflow, `GetRun` shows node summary, SSE events stream in real-time, approve/cancel mutations work

## Risks

### 1. Smithers v1 API vs legacy GUI endpoints

**Risk**: The GUI's `transport.ts` uses legacy endpoints (`/ps`, `/node/:runId`) that don't exist in the current `server/index.ts`. The v1 endpoints (`/v1/runs`, `/v1/runs/:id`) have a different response shape.

**Mitigation**: The TUI exclusively targets the v1 API. The types in Slice 1 are derived from the actual server code, not the GUI transport. If the server changes, only `internal/smithers/types.go` needs updating.

### 2. SSE stream lifecycle management

**Risk**: Long-lived SSE connections can leak goroutines if the TUI navigates away from the runs view without cancelling the context.

**Mitigation**: Each `StreamRunEvents` call takes a `context.Context`. The runs view must cancel its context in its cleanup path (when popped from the router stack). Add a `Close()` or `Cancel()` method to the view that cancels the SSE context. Document this contract in the `View` interface.

### 3. No smithers server running (exec fallback reliability)

**Risk**: The shell-out fallback depends on `smithers` being on `$PATH` and supporting `--json` output. If the CLI output format changes, the fallback breaks silently.

**Mitigation**: Pin the expected JSON shape in unit tests. The exec fallback is explicitly best-effort; the primary path is always HTTP. Log a warning when falling back to exec so users know the server isn't running.

### 4. NewClient signature change is a breaking change

**Risk**: `internal/ui/model/ui.go:332` currently calls `smithers.NewClient()` with no arguments. Changing the signature to accept options could break the build if not done atomically.

**Mitigation**: Use the variadic options pattern (`NewClient(opts ...ClientOption)`) so the zero-argument call still compiles. The existing `ListAgents` method continues to work as-is since it doesn't use HTTP.

### 5. Server DB requirement for GET /v1/runs

**Risk**: The list-all-runs endpoint (`GET /v1/runs`, line 956–968 of server/index.ts) requires a server-level DB (`serverAdapter`). If the server was started without `--db`, this endpoint returns 400 `DB_NOT_CONFIGURED`. Individual run endpoints (`GET /v1/runs/:id`) work because they resolve the adapter from the in-memory run record.

**Mitigation**: Handle the `DB_NOT_CONFIGURED` error code gracefully in the client—return an empty list with a logged warning rather than surfacing an error to the user. Document that `smithers up --serve --db smithers.db` is required for the full runs dashboard experience.

### 6. Auth token handling

**Risk**: The server checks `Authorization: Bearer <token>` or `x-smithers-key` header (lines 181–196). If the TUI config has an API token but the server doesn't, or vice versa, requests fail with 401.

**Mitigation**: The client sends the token only when configured. The 401 error is surfaced clearly in the TUI. The config supports `${SMITHERS_API_KEY}` environment variable expansion, matching the PRD §9 config example.
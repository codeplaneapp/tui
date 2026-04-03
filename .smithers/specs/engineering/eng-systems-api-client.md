# Engineering Spec: Systems and Analytics API Client Methods

**Ticket**: eng-systems-api-client
**Group**: Systems And Analytics
**Status**: Draft
**Date**: 2026-04-02

---

## Objective

Expand the Smithers Go client (`internal/smithers/client.go`) with the API bindings required by the Systems and Analytics views: SQL query execution, scorer/metrics retrieval, memory fact listing and semantic recall, and full cron/trigger CRUD. Each method must support dual-mode transport — HTTP API when the Smithers server is running, with fallback to direct SQLite reads or `exec.Command("smithers", ...)` shell-out when it is not.

This client surface is consumed by:
- **SQL Browser view** (`internal/ui/views/sqlbrowser.go`) — PRD §6.11, Design §3.10
- **Triggers/Cron Manager view** (`internal/ui/views/triggers.go`) — PRD §6.12
- **Scores/ROI Dashboard view** (`internal/ui/views/scores.go`) — PRD §6.14, Design §3.16
- **Memory Browser view** (`internal/ui/views/memory.go`) — PRD §6.15

**Feature coverage** (from `features.ts`):
- `SQL_BROWSER`, `SQL_TABLE_SIDEBAR`, `SQL_QUERY_EDITOR`, `SQL_RESULTS_TABLE`
- `TRIGGERS_LIST`, `TRIGGERS_TOGGLE`, `TRIGGERS_CREATE`, `TRIGGERS_EDIT`, `TRIGGERS_DELETE`
- `SCORES_AND_ROI_DASHBOARD`, `SCORES_RUN_EVALUATIONS`, `SCORES_TOKEN_USAGE_METRICS`, `SCORES_TOOL_CALL_METRICS`, `SCORES_LATENCY_METRICS`, `SCORES_CACHE_EFFICIENCY_METRICS`, `SCORES_DAILY_AND_WEEKLY_SUMMARIES`, `SCORES_COST_TRACKING`
- `MEMORY_BROWSER`, `MEMORY_FACT_LIST`, `MEMORY_SEMANTIC_RECALL`, `MEMORY_CROSS_RUN_MESSAGE_HISTORY`

---

## Scope

### In scope

1. **Type definitions** in `internal/smithers/types.go` for: `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, `CronSchedule`.
2. **Client methods** on the `Client` struct in `internal/smithers/client.go`:
   - `ExecuteSQL(ctx, query string) (*SQLResult, error)`
   - `GetScores(ctx, runID string, nodeID *string) ([]ScoreRow, error)`
   - `GetAggregateScores(ctx, runID string) ([]AggregateScore, error)`
   - `ListMemoryFacts(ctx, namespace string, workflowPath string) ([]MemoryFact, error)`
   - `RecallMemory(ctx, query string, namespace *string, topK int) ([]MemoryRecallResult, error)`
   - `ListCrons(ctx) ([]CronSchedule, error)`
   - `CreateCron(ctx, pattern string, workflowPath string) (*CronSchedule, error)`
   - `ToggleCron(ctx, cronID string, enabled bool) error`
   - `DeleteCron(ctx, cronID string) error`
3. **Dual-mode transport** per method: HTTP-primary, with SQLite fallback for reads and `exec.Command` fallback for mutations.
4. **Unit tests** confirming transport routing and response parsing for each method.
5. **Terminal E2E tests** and **VHS recording tests** validating the end-to-end flow from TUI navigation through to rendered results.

### Out of scope

- View-layer rendering (handled by separate view tickets).
- SSE event streaming for real-time updates (handled by `events.go` in `eng-smithers-client-runs`).
- Smithers MCP server side (TypeScript, lives in `../smithers/src/`).
- Direct Drizzle ORM access (the TUI never imports Smithers' TypeScript DB layer).

### Implementation status

The core client methods, type definitions, and unit tests are **already implemented**:
- `internal/smithers/client.go` — all 9 methods with transport helpers (`httpGetJSON`, `httpPostJSON`, `queryDB`, `execSmithers`, `isServerAvailable`)
- `internal/smithers/types.go` — all 6 type definitions
- `internal/smithers/client_test.go` — 20+ tests covering HTTP, exec, and aggregation paths

**Remaining work**: Terminal E2E tests, VHS recording test, and verification against live Smithers server.

---

## Implementation Plan

### Slice 1: Client struct plumbing — dual-mode transport helpers (DONE)

**Goal**: Extend the `Client` struct with the fields and private helpers needed for dual-mode transport, so each API method can simply call `c.httpGetJSON()`, `c.httpPostJSON()`, `c.execSmithers()`, or `c.queryDB()`.

**Files**:
- `internal/smithers/client.go:59-96` — `Client` struct with `apiURL`, `apiToken`, `dbPath`, `db *sql.DB`, `httpClient`, `execFunc` fields. Functional options pattern via `ClientOption` funcs (`WithAPIURL`, `WithAPIToken`, `WithDBPath`, `WithHTTPClient`). Constructor `NewClient(opts ...ClientOption)` opens read-only SQLite if DB path configured.

**Implemented transport helpers**:
- `httpGetJSON(ctx, path string, out any) error` — GET `{apiURL}{path}`, parse JSON envelope (`{ok, data, error}`), unmarshal `data` into `out`. Returns `ErrServerUnavailable` on connection failure. (`client.go:174-200`)
- `httpPostJSON(ctx, path string, body any, out any) error` — POST with JSON body, same envelope parsing. (`client.go:203-237`)
- `execSmithers(ctx, args ...string) ([]byte, error)` — runs `exec.CommandContext(ctx, "smithers", args...)`, captures stdout. Supports test override via `execFunc` field. (`client.go:248-262`)
- `queryDB(ctx, query string, args ...any) (*sql.Rows, error)` — wraps `c.db.QueryContext`. Returns `ErrNoDatabase` if `c.db` is nil. (`client.go:240-245`)
- `isServerAvailable() bool` — cached probe via `GET {apiURL}/health` with 1s timeout; result cached for 30s with mutex-protected double-check locking. (`client.go:130-171`)

The routing decision in each method follows:
```
1. If server available → HTTP
2. If read-only operation and DB open → SQLite
3. If mutation or no SQLite path → exec.Command("smithers", ...) as fallback
4. Otherwise → return clear error (ErrNoTransport)
```

**Upstream reference**: The GUI transport layer in `smithers/gui/src/ui/api/transport.ts` uses a similar `getApiBaseUrl()` + `unwrap<T>()` pattern. The daemon HTTP client in `smithers/gui-ref/apps/daemon/src/integrations/smithers/http-client.ts` shows the `{ok, data}` envelope format.

### Slice 2: Type definitions for Systems & Analytics domain (DONE)

**Goal**: Go struct types that map to the upstream Smithers TypeScript types.

**File**: `internal/smithers/types.go`

**Types implemented** (all with JSON tags matching upstream wire format):

| Go Type | Upstream TypeScript | Table | Notes |
|---------|-------------------|-------|-------|
| `SQLResult` | ad-hoc `{results: any[]}` | N/A | `Columns []string`, `Rows [][]interface{}` |
| `ScoreRow` | `ScoreRow` in `smithers/src/scorers/types.ts` | `_smithers_scorer_results` | 16 fields including `Score float64` (0-1 normalized) |
| `AggregateScore` | computed client-side | N/A | `Count`, `Mean`, `Min`, `Max`, `P50`, `StdDev` |
| `MemoryFact` | `MemoryFact` in `smithers/src/memory/types.ts` | `_smithers_memory_facts` | `Namespace`, `Key`, `ValueJSON`, optional `TTLMs` |
| `MemoryRecallResult` | vector search result | N/A | `Score float64`, `Content string`, `Metadata interface{}` |
| `CronSchedule` | cron row in `smithers/src/db/internal-schema.ts` | `_smithers_crons` | `CronID`, `Pattern`, `WorkflowPath`, `Enabled`, optional timing fields |

### Slice 3: ExecuteSQL method (DONE)

**Goal**: Execute an arbitrary SQL query against the Smithers database.

**File**: `internal/smithers/client.go:268-295`

**Upstream endpoints**:
- HTTP: `POST /sql` with body `{"query": "..."}` → `{results: any[]}` (see `smithers/src/cli/index.ts:2735-2763`)
- CLI: `smithers sql --query "..." --format json` → JSON to stdout
- SQLite fallback: direct `c.queryDB(ctx, query)` for read-only queries (SELECT/PRAGMA/EXPLAIN only)

**Transport cascade**: HTTP → SQLite (SELECT only) → exec

**Key implementation details**:
- `isSelectQuery()` (`client.go:299-304`): prefix check allowing `SELECT`, `PRAGMA`, `EXPLAIN`. Defense-in-depth alongside read-only SQLite mode (`?mode=ro`).
- `convertResultMaps()` (`client.go:509-529`): converts HTTP `[]map[string]interface{}` to columnar `SQLResult` with sorted, deterministic column order.
- `scanSQLResult()` (`client.go:481-506`): converts `*sql.Rows` to `SQLResult`, handling `[]byte` → `string` conversion for JSON compatibility.
- `parseSQLResultJSON()` (`client.go:532-544`): handles both `SQLResult` and `[]map[string]interface{}` formats from CLI output.

### Slice 4: GetScores and GetAggregateScores methods (DONE)

**Goal**: Retrieve scorer evaluation results for a given run.

**File**: `internal/smithers/client.go:310-350`

**Upstream endpoints**:
- CLI: `smithers scores <runId> [--node <nodeId>] --format json` → JSON to stdout (see `smithers/src/cli/index.ts:2416-2449`)
- SQLite: `_smithers_scorer_results` table is queryable directly.

**Mismatch note**: The upstream Smithers CLI server (`src/cli/server.ts`) may expose a `/scores/{runId}` route via its `.fetch()` handler, but the GUI transport layer (`gui/src/ui/api/transport.ts`) does not reference it. The current implementation conservatively treats scores as a SQLite-first, exec-fallback path. If an HTTP route is confirmed upstream, `GetScores` should add HTTP as the first transport tier.

**Transport cascade**: SQLite (preferred) → exec

**Key implementation details**:
- `GetScores` queries `_smithers_scorer_results` with required `run_id` filter and optional `node_id` filter, ordered by `scored_at_ms DESC`.
- `GetAggregateScores` calls `GetScores` then computes per-scorer statistics via `aggregateScores()` (`client.go:575-640`): groups by `ScorerID`, computes count, mean, min, max, p50 (median), stddev (sample standard deviation, `n-1` denominator).
- `scanScoreRows()` (`client.go:547-563`): scans all 16 columns from `_smithers_scorer_results` into `ScoreRow` struct.

### Slice 5: ListMemoryFacts and RecallMemory methods (DONE)

**Goal**: List memory facts and perform semantic recall queries.

**File**: `internal/smithers/client.go:356-396`

**Upstream endpoints**:
- CLI: `smithers memory list <namespace> [--workflow <path>] --format json` (see `smithers/src/cli/index.ts:851-933`)
- CLI: `smithers memory recall <query> [--namespace <ns>] [--topK <k>] --format json`
- SQLite: `_smithers_memory_facts` table for listing; semantic recall requires vector search (not feasible from Go directly).

**Mismatch note**: Memory recall uses vector similarity search (`createSqliteVectorStore()`) which requires the Smithers TypeScript runtime with an embedding model (OpenAI `text-embedding-3-small`). The Go client cannot replicate this. Therefore:
- `ListMemoryFacts()`: SQLite fallback works (simple SELECT on `_smithers_memory_facts` filtered by `namespace`).
- `RecallMemory()`: Must always shell out to `smithers memory recall` — there is no SQLite-only path for semantic search.

**Transport cascade**:
- `ListMemoryFacts`: SQLite → exec
- `RecallMemory`: exec only (always — vector search requires TypeScript runtime)

### Slice 6: Cron CRUD methods (DONE)

**Goal**: Full CRUD for cron trigger schedules.

**File**: `internal/smithers/client.go:402-476`

**Upstream endpoints**:
- HTTP: `GET /cron/list`, `POST /cron/add`, `POST /cron/toggle/{id}`, `POST /cron/rm/{cronId}` (see `smithers/src/cli/index.ts:1040-1121`, `smithers/gui/src/ui/api/transport.ts:64-83`)
- CLI: `smithers cron list`, `cron add <pattern> <workflowPath>`, `cron rm <cronId>`, `cron toggle <cronId> --enabled <bool>`
- SQLite: `_smithers_crons` table for read operations.

**Transport cascades per method**:

| Method | HTTP | SQLite | Exec |
|--------|------|--------|------|
| `ListCrons` | `GET /cron/list` → `[]CronSchedule` | `SELECT FROM _smithers_crons` | `smithers cron list --format json` |
| `CreateCron` | `POST /cron/add` | N/A (mutation) | `smithers cron add <pattern> <path> --format json` |
| `ToggleCron` | `POST /cron/toggle/{id}` | N/A (mutation) | `smithers cron toggle <id> --enabled <bool>` |
| `DeleteCron` | N/A | N/A (mutation) | `smithers cron rm <id>` |

**Mismatch note**: `DeleteCron` is exec-only because the upstream CLI server does not expose a dedicated `DELETE /cron/{id}` HTTP route. The `POST /cron/rm/{cronId}` route exists in the CLI's `.fetch()` handler but is not consumed by the GUI transport layer. If confirmed stable, `DeleteCron` should try HTTP first. For now, exec is the safe path. This is low-impact since cron deletion is infrequent.

### Slice 7: Unit tests (DONE)

**Goal**: Unit tests confirming each method routes to the correct transport layer and parses responses correctly.

**File**: `internal/smithers/client_test.go`

**Implemented tests** (20+ test functions):

```
TestExecuteSQL_HTTP            — Mock httptest.Server validates POST /sql, body, returns envelope
TestExecuteSQL_Exec            — Mock execFunc validates args ["sql", "--query", ..., "--format", "json"]
TestIsSelectQuery              — Table-driven: SELECT/PRAGMA/EXPLAIN → true; INSERT/UPDATE/DELETE/DROP → false

TestGetScores_Exec             — Mock exec validates ["scores", "run-123", "--format", "json"]
TestGetScores_ExecWithNodeFilter — Validates --node flag passed through
TestGetAggregateScores         — 5 scores across 2 scorers: validates count, mean, min, max, p50
TestAggregateScores_Empty      — Empty input returns empty output
TestAggregateScores_SingleValue — Single value: mean = p50 = min = max = value

TestListMemoryFacts_Exec       — Validates ["memory", "list", "default", "--format", "json"]
TestListMemoryFacts_ExecWithWorkflow — Validates --workflow flag
TestRecallMemory_Exec          — Validates ["memory", "recall", "test query", "--format", "json", "--topK", "5"]
TestRecallMemory_ExecWithNamespace — Validates --namespace flag

TestListCrons_HTTP             — Mock server at /cron/list, validates GET, returns CronSchedule array
TestListCrons_Exec             — Mock exec validates ["cron", "list", "--format", "json"]
TestCreateCron_HTTP            — Mock server at /cron/add, validates POST body {pattern, workflowPath}
TestCreateCron_Exec            — Mock exec validates ["cron", "add", pattern, path, "--format", "json"]
TestToggleCron_HTTP            — Mock server at /cron/toggle/c1, validates POST body {enabled}
TestToggleCron_Exec            — Mock exec validates ["cron", "toggle", "c1", "--enabled", "true"]
TestDeleteCron_Exec            — Mock exec validates ["cron", "rm", "c1"]

TestTransportFallback_ServerDown — No server URL, no DB → falls through to exec
TestConvertResultMaps_Empty    — Nil input returns empty SQLResult
TestConvertResultMaps          — 2-row, 2-column map → columnar SQLResult with sorted columns
TestListAgents_NoOptions       — Backward compat: NewClient() with no options returns stub agents
```

**Test infrastructure**:
- `newTestServer(t, handler)`: creates `httptest.Server` with `/health` auto-responding 200, returns `(server, client)` pair with server availability cache pre-warmed.
- `writeEnvelope(t, w, data)`: writes `{ok: true, data: ...}` JSON envelope.
- `newExecClient(fn)`: creates `Client` with mock `execFunc` (no HTTP, no DB).

### Slice 8: Terminal E2E tests (TODO)

**Goal**: End-to-end tests that launch the full TUI binary, navigate to Systems views, and assert on rendered output.

**File**: `tests/tui_e2e_test.go` (new)

**Upstream reference**: The Smithers TUI E2E harness in `smithers/tests/tui-helpers.ts` implements a `BunSpawnBackend` class that:
- Spawns the TUI binary via `Bun.spawn()` with piped stdin/stdout/stderr
- Sets environment: `TERM=xterm-256color`, `COLORTERM=truecolor`, `LANG=en_US.UTF-8`
- Implements `waitForText(text, timeoutMs)` with 100ms polling interval and 10s default timeout
- Strips ANSI escape sequences before text matching
- Takes snapshot of terminal buffer on test failure for debugging
- Provides `sendKeys(text)` that writes directly to stdin pipe

The Crush Go equivalent must replicate this pattern:

```go
// tests/tui_harness_test.go
type TUIHarness struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  io.ReadCloser
    mu      sync.Mutex
    buf     bytes.Buffer // accumulated stdout with ANSI stripped
    done    chan struct{}
}

func launchTUI(t *testing.T, args ...string) *TUIHarness {
    t.Helper()
    cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
    cmd.Env = append(os.Environ(),
        "TERM=xterm-256color",
        "COLORTERM=truecolor",
        "LANG=en_US.UTF-8",
    )
    stdin, _ := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    cmd.Start()
    h := &TUIHarness{cmd: cmd, stdin: stdin, stdout: stdout, done: make(chan struct{})}
    go h.readLoop() // continuously read stdout, strip ANSI, append to buf
    t.Cleanup(h.Terminate)
    return h
}

func (h *TUIHarness) SendKeys(s string)  { h.stdin.Write([]byte(s)) }

func (h *TUIHarness) WaitForText(text string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        h.mu.Lock()
        found := strings.Contains(h.buf.String(), text)
        h.mu.Unlock()
        if found { return nil }
        time.Sleep(100 * time.Millisecond) // match upstream polling interval
    }
    return fmt.Errorf("text %q not found within %v\nSnapshot:\n%s", text, timeout, h.Snapshot())
}

func (h *TUIHarness) WaitForNoText(text string, timeout time.Duration) error { /* inverse */ }
func (h *TUIHarness) Snapshot() string { /* return stripped buffer */ }
func (h *TUIHarness) Terminate()       { h.cmd.Process.Kill(); <-h.done }
```

**E2E test for Systems views**:

```go
// tests/tui_e2e_test.go
func TestSystemsViews_E2E(t *testing.T) {
    if testing.Short() { t.Skip("skipping E2E in short mode") }

    // Setup: seed a test SQLite DB with fixture data
    testDBPath := seedTestDB(t) // creates temp DB with runs, scorer_results, memory_facts, crons
    tui := launchTUI(t, "--db", testDBPath)

    // 1. Navigate to SQL Browser via command palette
    tui.SendKeys("/")
    require.NoError(t, tui.WaitForText("sql", 5*time.Second))
    tui.SendKeys("sql")
    tui.SendKeys("\r")
    require.NoError(t, tui.WaitForText("Tables", 5*time.Second))
    require.NoError(t, tui.WaitForText("_smithers_runs", 5*time.Second))

    // 2. Execute a query
    tui.SendKeys("SELECT count(*) FROM _smithers_runs;")
    tui.SendKeys("\x0d") // Enter to execute
    require.NoError(t, tui.WaitForText("count", 5*time.Second)) // column header in results

    // 3. Navigate to Triggers via Esc + command palette
    tui.SendKeys("\x1b") // Esc back
    time.Sleep(200 * time.Millisecond)
    tui.SendKeys("/")
    require.NoError(t, tui.WaitForText("triggers", 5*time.Second))
    tui.SendKeys("trig")
    tui.SendKeys("\r")
    require.NoError(t, tui.WaitForText("0 */6 * * *", 5*time.Second)) // fixture cron pattern

    // 4. Esc back, test scores via chat agent
    tui.SendKeys("\x1b")
    time.Sleep(200 * time.Millisecond)
}

func TestSQLBrowser_E2E(t *testing.T) {
    if testing.Short() { t.Skip("skipping E2E in short mode") }
    testDBPath := seedTestDB(t)
    tui := launchTUI(t, "--db", testDBPath)

    // Navigate to SQL Browser
    tui.SendKeys("/")
    require.NoError(t, tui.WaitForText("sql", 5*time.Second))
    tui.SendKeys("sql\r")

    // Verify table sidebar lists known tables
    require.NoError(t, tui.WaitForText("_smithers_runs", 5*time.Second))
    require.NoError(t, tui.WaitForText("_smithers_crons", 5*time.Second))
    require.NoError(t, tui.WaitForText("_smithers_memory", 5*time.Second))

    // Execute PRAGMA query
    tui.SendKeys("PRAGMA table_info(_smithers_runs);")
    tui.SendKeys("\x0d")
    require.NoError(t, tui.WaitForText("name", 5*time.Second)) // PRAGMA returns name column
}

func TestTriggersView_E2E(t *testing.T) {
    if testing.Short() { t.Skip("skipping E2E in short mode") }
    testDBPath := seedTestDB(t)
    tui := launchTUI(t, "--db", testDBPath)

    // Navigate to Triggers
    tui.SendKeys("/")
    require.NoError(t, tui.WaitForText("triggers", 5*time.Second))
    tui.SendKeys("trig\r")

    // Verify cron list renders with fixture data
    require.NoError(t, tui.WaitForText("0 */6 * * *", 5*time.Second))
    require.NoError(t, tui.WaitForText("deploy.tsx", 5*time.Second))

    // Toggle a cron (fixture has an enabled cron)
    tui.SendKeys("\r") // select first cron
    require.NoError(t, tui.WaitForText("disabled", 5*time.Second)) // toggled state
}
```

### Slice 9: VHS happy-path recording test (TODO)

**Goal**: A VHS tape that exercises the systems API client happy path, producing a visual recording for documentation and CI smoke testing.

**File**: `tests/tapes/systems-api-client.tape` (new)

**Upstream reference**: Smithers VHS demo tapes in `demo/smithers/tapes/` use this pattern:
- `Output` directive for `.gif` output path
- `Set Shell "bash"`, `Set FontSize 18`, `Set Width 900`, `Set Height 500`, `Set TypingSpeed 50ms`, `Set Theme "Catppuccin Frappe"`
- `Hide`/`Show` blocks for setup commands that shouldn't appear in recording
- `Type` + `Sleep` + `Enter` for user interactions
- `Sleep` after commands to let output render

```tape
# Systems API Client — Happy Path
Output tests/recordings/systems-api-client.gif
Set Shell "bash"
Set FontSize 16
Set Width 120
Set Height 40
Set TypingSpeed 50ms
Set Theme "Catppuccin Frappe"

# Setup: seed test DB (hidden from recording)
Hide
Type "export SMITHERS_DB=.smithers/test-fixtures/systems.db"
Enter
Sleep 500ms
Show

# Launch Smithers TUI
Type "smithers-tui --db $SMITHERS_DB"
Enter
Sleep 2s

# Navigate to SQL Browser via command palette
Type "/"
Sleep 500ms
Type "sql"
Sleep 300ms
Enter
Sleep 1s

# Show table sidebar
Sleep 500ms

# Execute a query
Type "SELECT id, status, workflow_path FROM _smithers_runs LIMIT 5;"
Sleep 500ms
Enter
Sleep 2s

# Go back to chat
Escape
Sleep 500ms

# Navigate to Triggers view
Type "/"
Sleep 500ms
Type "triggers"
Sleep 300ms
Enter
Sleep 1s

# View cron schedule list
Sleep 2s

# Exit
Ctrl+C
Sleep 500ms
```

**CI integration**:
- `vhs tests/tapes/systems-api-client.tape` — exit code 0 confirms TUI launches, navigates, and renders without panic.
- The `.gif` output is an artifact for documentation. CI does not diff against a golden image (too fragile); it asserts exit code only.

---

## Validation

### Automated checks

1. **Unit tests**: `go test ./internal/smithers/ -v` — all 20+ tests pass, covering HTTP, SQLite, and exec paths for each method.
   - Per-method: `go test ./internal/smithers/ -run TestExecuteSQL -v`, `TestGetScores`, `TestListMemoryFacts`, `TestRecallMemory`, `TestListCrons`, `TestCreateCron`, `TestToggleCron`, `TestDeleteCron`
2. **Aggregation math**: `go test ./internal/smithers/ -run TestAggregateScores -v` — validates mean, p50, min, max, stddev computation.
3. **Race detection**: `go test -race ./internal/smithers/` — no data races in the cached `isServerAvailable()` probe (uses `sync.RWMutex` with double-check locking).
4. **Select guard**: `go test ./internal/smithers/ -run TestIsSelectQuery -v` — table-driven test covering SELECT, PRAGMA, EXPLAIN (allowed) and INSERT, UPDATE, DELETE, DROP (rejected).
5. **Transport fallback**: `go test ./internal/smithers/ -run TestTransportFallback -v` — confirms graceful degradation when server is down.
6. **Lint**: `golangci-lint run ./internal/smithers/` — no new warnings.

### Terminal E2E tests (modeled on upstream @microsoft/tui-test harness)

The upstream Smithers TUI E2E harness in `smithers/tests/tui.e2e.test.ts` and `smithers/tests/tui-helpers.ts` uses a `BunSpawnBackend` class with:
- `Bun.spawn()` with piped stdio
- `waitForText(text, timeoutMs)`: polls buffer every 100ms, default 10s timeout, strips ANSI escapes
- `waitForNoText(text, timeoutMs)`: inverse — waits for text to disappear
- `sendKeys(text)`: writes to stdin pipe
- `snapshot()`: returns current terminal buffer (used for failure diagnostics)
- `terminate()`: kills the process
- Environment: `TERM=xterm-256color`, `COLORTERM=truecolor`, `LANG=en_US.UTF-8`

Crush replicates this in Go with a `TUIHarness` struct (see Slice 8 above). The key E2E tests:

| Test | What it validates | Key assertions |
|------|------------------|----------------|
| `TestSystemsViews_E2E` | Full navigation: chat → SQL Browser → Triggers | Table sidebar renders, query results appear, cron patterns visible |
| `TestSQLBrowser_E2E` | SQL Browser table sidebar + PRAGMA query | Known tables listed, PRAGMA returns column metadata |
| `TestTriggersView_E2E` | Trigger list + toggle interaction | Fixture cron pattern + workflow path visible, toggle changes state |

Run: `go test ./tests/ -run TestSystems -v -timeout 60s`

### VHS happy-path recording test

A VHS tape at `tests/tapes/systems-api-client.tape` (see Slice 9) exercises: launch TUI → open SQL Browser → execute SELECT → navigate to Triggers → view cron list → exit.

Run: `vhs tests/tapes/systems-api-client.tape`

CI asserts: exit code 0 (TUI renders without panic). The `.gif` artifact is published for documentation.

### Manual verification paths

1. **With server running**: Start `smithers up --serve`, then launch Smithers TUI. Navigate to `/sql`, run `SELECT * FROM _smithers_runs LIMIT 3`. Verify results render with column headers and row data. Navigate to `/triggers`, verify cron list populates from HTTP. Toggle a cron via `Enter` key and verify the change persists on re-navigating to `/triggers`.

2. **Without server (SQLite fallback)**: Stop the Smithers server. Launch TUI with `--db .smithers/smithers.db`. Navigate to `/sql`, run a SELECT. Verify results come from direct SQLite (check that non-SELECT queries are rejected with an error). Navigate to `/triggers`, verify list loads from SQLite. Toggle a cron — verify it falls back to `smithers cron toggle` exec (visible in verbose logging).

3. **Without server and no DB (exec-only)**: Launch TUI without server or DB but with `smithers` binary in PATH. Navigate to `/sql`, execute `SELECT 1`. Verify results come from `smithers sql` exec. Navigate to `/triggers`, verify list from `smithers cron list` exec.

4. **Without server, no DB, no binary**: Launch TUI with no transport available. Navigate to `/sql`, attempt a query. Verify a clear error message renders (e.g., "No Smithers transport available — start server with `smithers up --serve` or ensure smithers is in PATH").

---

## Risks

### 1. Missing upstream HTTP endpoints for scores and memory

**Impact**: Medium. The Smithers CLI server (`src/cli/server.ts`) may not expose dedicated `/scores/<runId>` or `/memory/*` HTTP routes (the GUI transport layer at `gui/src/ui/api/transport.ts` does not reference them). Scores and memory methods fall back to `exec.Command("smithers", ...)` or direct SQLite, which is slower than HTTP (~200ms exec vs ~20ms HTTP).

**Mitigation**: The current implementation handles this correctly — `GetScores` tries SQLite first (fast), `RecallMemory` uses exec (necessary for vector search). If HTTP endpoints are confirmed upstream, add HTTP as the first transport tier. File a PR upstream to add `GET /scores/<runId>` and `GET /memory/list/<namespace>` to the CLI server for performance parity.

### 2. SQLite schema drift between Smithers versions

**Impact**: Medium. The direct SQLite queries (`_smithers_scorer_results`, `_smithers_memory_facts`, `_smithers_crons`) depend on specific column names from the Smithers internal schema (`smithers/src/db/internal-schema.ts`). Schema changes in Smithers would silently break the Go client's `scanScoreRows()`, `scanMemoryFacts()`, and `scanCronSchedules()` helpers.

**Mitigation**: Add a startup check that queries `PRAGMA table_info(...)` for each table the client reads and logs a warning if columns don't match expectations. Consider versioning the schema check against the Smithers binary version (`smithers --version`). The exec fallback path is schema-agnostic (it parses CLI JSON output), so graceful degradation works.

### 3. Memory recall requires Smithers runtime

**Impact**: Low. Semantic recall uses vector similarity search (`createSqliteVectorStore()` with OpenAI `text-embedding-3-small`) which is implemented in the TypeScript runtime. There is no pure-SQLite or pure-Go fallback for `RecallMemory()` — it always needs the `smithers memory recall` CLI command.

**Mitigation**: Acceptable. The exec path is correct and well-tested. If the `smithers` binary is not in PATH, the method returns a clear `exec.Error`. The Memory Browser view should display an informative message like "Install Smithers CLI for semantic memory recall" when the binary is unavailable.

### 4. DeleteCron has no confirmed HTTP API

**Impact**: Low. `DeleteCron` is currently exec-only. The upstream CLI server has a `POST /cron/rm/{cronId}` handler in its `.fetch()` method, but the GUI transport layer does not consume it, making its stability uncertain.

**Mitigation**: Exec works. Cron deletion is low-frequency. If the HTTP route is confirmed stable, add HTTP as a first-try transport. For now, exec-only is the safe, tested path.

### 5. SQL injection via ExecuteSQL SQLite fallback

**Impact**: High if not mitigated. The `ExecuteSQL` method accepts arbitrary user SQL. The SQLite connection is opened read-only (`?mode=ro`), which prevents writes at the database driver level. Additionally, the `isSelectQuery()` guard (`client.go:299-304`) rejects non-SELECT statements on the direct-SQLite path.

**Mitigation**: Defense in depth is in place:
1. SQLite opened with `?mode=ro` — driver-level write prevention.
2. `isSelectQuery()` — application-level prefix check.
3. HTTP and exec paths delegate sanitization to the Smithers server/CLI.
4. The SQL Browser view is a power-user feature for debugging, documented as accepting raw SQL.

### 6. Parallel work with eng-smithers-client-runs

**Impact**: Low. The `Client` struct, transport helpers, and constructor are already implemented and shared by both this ticket and `eng-smithers-client-runs`. Run-related methods (`ListRuns`, `GetRun`, `StreamEvents`, etc.) will be added to the same `client.go` file.

**Mitigation**: The type definitions (Slice 2) and method implementations (Slices 3-6) occupy distinct sections of `client.go` and `types.go`. The transport helpers (Slice 1) are already landed and stable. Merge conflicts should be minimal — limited to import blocks. Coordinate by landing type additions first, methods second.

### 7. E2E test reliability in CI

**Impact**: Medium. Terminal E2E tests that spawn the TUI binary and poll for text are inherently timing-sensitive. Slow CI runners may cause `WaitForText` timeouts.

**Mitigation**: Use generous timeouts (10s default, matching upstream), poll at 100ms intervals, and capture `Snapshot()` on failure for debugging. Mark E2E tests with `if testing.Short() { t.Skip() }` so they can be skipped in fast iteration. VHS tape tests are more forgiving since they use explicit `Sleep` durations.

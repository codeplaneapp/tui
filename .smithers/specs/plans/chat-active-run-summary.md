## Goal

Wire a live active-run count into the Smithers TUI header and status bar. On startup and on a periodic 10-second tick the client calls `GET /v1/runs` (HTTP → SQLite → exec fallback), counts the runs whose status is `running`, `waiting-approval`, or `waiting-event`, and stores the result in `UI.smithersStatus`. The header segment `X active` (and `⚠ N pending approval`) then renders from that field, which already exists in `SmithersStatus` and in both `renderHeaderDetails` and `Status.drawSmithersSummary`. The ticket acceptance criteria ("The header displays 'X active' when there are running workflows. The run count updates dynamically.") are satisfied once the data-fetch loop and the client-config wiring are in place — no rendering changes are required.

## Steps

### 1. Add `ListRuns` to the Smithers client

`GetRun` (single-run fetch) already exists. The summary poll needs `ListRuns`, which is not yet implemented.

Add to `internal/smithers/client.go` immediately after `GetRun`:

```go
// ListRuns returns runs matching the optional filter.
// Routes: HTTP GET /v1/runs → SQLite → exec smithers ps --format json.
func (c *Client) ListRuns(ctx context.Context, f RunFilter) ([]Run, error) {
    // 1. Try HTTP
    if c.isServerAvailable() {
        path := "/v1/runs"
        if f.Status != "" {
            path += "?status=" + url.QueryEscape(f.Status)
        }
        var runs []Run
        if err := c.httpGetJSON(ctx, path, &runs); err == nil {
            return runs, nil
        }
    }

    // 2. Try direct SQLite
    if c.db != nil {
        query := `SELECT run_id, workflow_name, workflow_path, status,
                  started_at_ms, finished_at_ms, error_json
                  FROM _smithers_runs`
        var args []any
        if f.Status != "" {
            query += " WHERE status = ?"
            args = append(args, f.Status)
        }
        query += " ORDER BY started_at_ms DESC"
        if f.Limit > 0 {
            query += fmt.Sprintf(" LIMIT %d", f.Limit)
        }
        rows, err := c.queryDB(ctx, query, args...)
        if err != nil {
            return nil, err
        }
        return scanRuns(rows)
    }

    // 3. Fall back to exec
    args := []string{"ps", "--format", "json"}
    out, err := c.execSmithers(ctx, args...)
    if err != nil {
        return nil, err
    }
    var runs []Run
    if err := json.Unmarshal(out, &runs); err != nil {
        return nil, fmt.Errorf("parse runs: %w", err)
    }
    return runs, nil
}
```

Add `scanRuns` helper alongside the other scan functions at the bottom of `client.go`:

```go
func scanRuns(rows *sql.Rows) ([]Run, error) {
    defer rows.Close()
    var result []Run
    for rows.Next() {
        var r Run
        if err := rows.Scan(
            &r.RunID, &r.WorkflowName, &r.WorkflowPath,
            &r.Status, &r.StartedAtMs, &r.FinishedAtMs, &r.ErrorJSON,
        ); err != nil {
            return nil, err
        }
        result = append(result, r)
    }
    return result, rows.Err()
}
```

Add `"net/url"` to the import block (already importing `"net/http"`, so the stdlib group already exists).

### 2. Add `RunStatusSummary` and a client helper to derive it

Add to `internal/smithers/types_runs.go`:

```go
// RunStatusSummary is the aggregate view used by the header and status bar.
type RunStatusSummary struct {
    ActiveRuns       int // running + waiting-approval + waiting-event
    PendingApprovals int // waiting-approval only
}

// SummariseRuns derives a RunStatusSummary from a run list.
// Active = running | waiting-approval | waiting-event.
// PendingApprovals = waiting-approval only (subset of Active).
func SummariseRuns(runs []Run) RunStatusSummary {
    var s RunStatusSummary
    for _, r := range runs {
        switch r.Status {
        case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingEvent:
            s.ActiveRuns++
        }
        if r.Status == RunStatusWaitingApproval {
            s.PendingApprovals++
        }
    }
    return s
}
```

`SummariseRuns` is pure and easily unit-tested without any network or file I/O.

### 3. Wire client config at construction time in `ui.go`

Currently `ui.go` constructs the client with no options (`smithers.NewClient()`), so it falls through to exec on every call even when the server URL and DB path are configured.

In `internal/ui/model/ui.go`, replace:

```go
smithersClient: smithers.NewClient(),
```

with:

```go
smithersClient: buildSmithersClient(com.Config()),
```

Add the helper in `ui.go` (or a new `internal/ui/model/smithers_client.go` if the file grows large):

```go
// buildSmithersClient constructs a Smithers client from TUI config.
// Falls back to a no-op stub client when smithers config is absent.
func buildSmithersClient(cfg *config.Config) *smithers.Client {
    if cfg.Smithers == nil {
        return smithers.NewClient()
    }
    var opts []smithers.ClientOption
    if cfg.Smithers.APIURL != "" {
        opts = append(opts, smithers.WithAPIURL(cfg.Smithers.APIURL))
    }
    if cfg.Smithers.APIToken != "" {
        opts = append(opts, smithers.WithAPIToken(cfg.Smithers.APIToken))
    }
    if cfg.Smithers.DBPath != "" {
        opts = append(opts, smithers.WithDBPath(cfg.Smithers.DBPath))
    }
    return smithers.NewClient(opts...)
}
```

### 4. Define the Tea message and the poll command

Add to `internal/ui/model/ui.go` (near the other private message types):

```go
// smithersRunSummaryMsg is delivered to the update loop after a background
// run-summary refresh completes.
type smithersRunSummaryMsg struct {
    Summary smithers.RunStatusSummary
    Err     error
}
```

Add the command that fires the background fetch:

```go
// refreshSmithersRunSummaryCmd fetches active runs in the background and
// returns a smithersRunSummaryMsg. It is safe to call when smithersClient
// has no transport configured — errors are silently swallowed so the header
// simply stays blank.
func (m *UI) refreshSmithersRunSummaryCmd() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        runs, err := m.smithersClient.ListRuns(ctx, smithers.RunFilter{Limit: 200})
        if err != nil {
            return smithersRunSummaryMsg{Err: err}
        }
        return smithersRunSummaryMsg{Summary: smithers.SummariseRuns(runs)}
    }
}
```

Add the recurring tick command:

```go
const smithersRunSummaryPollInterval = 10 * time.Second

func smithersRunSummaryTickCmd() tea.Cmd {
    return tea.Tick(smithersRunSummaryPollInterval, func(t time.Time) tea.Msg {
        return smithersRunSummaryTickMsg(t)
    })
}

type smithersRunSummaryTickMsg time.Time
```

### 5. Seed the poll on startup

In `UI.Init()`, after the existing `cmds`, add:

```go
// Only poll if Smithers mode is active (config.Smithers != nil).
if m.com.Config().Smithers != nil {
    cmds = append(cmds, m.refreshSmithersRunSummaryCmd())
    cmds = append(cmds, smithersRunSummaryTickCmd())
}
```

### 6. Handle messages in `UI.Update()`

In the `switch msg.(type)` block in `Update` (in `internal/ui/model/ui.go`), add two cases:

```go
case smithersRunSummaryMsg:
    if msg.Err == nil {
        m.smithersStatus = &SmithersStatus{
            ActiveRuns:       msg.Summary.ActiveRuns,
            PendingApprovals: msg.Summary.PendingApprovals,
            // Preserve MCP connection state already tracked elsewhere.
            MCPConnected:  m.smithersStatus != nil && m.smithersStatus.MCPConnected,
            MCPServerName: func() string {
                if m.smithersStatus != nil {
                    return m.smithersStatus.MCPServerName
                }
                return ""
            }(),
        }
    }
    // Always re-arm the tick so the loop continues.
    cmds = append(cmds, smithersRunSummaryTickCmd())

case smithersRunSummaryTickMsg:
    if m.com.Config().Smithers != nil {
        cmds = append(cmds, m.refreshSmithersRunSummaryCmd())
    }
```

Errors from the fetch are ignored (the count stays at zero / last known value). This keeps the header stable when Smithers server is not running.

### 7. Verify rendering paths (no changes needed)

Both rendering sites already consume `SmithersStatus` and produce the correct output:

- `internal/ui/model/header.go` `renderHeaderDetails` at line 174: renders `fmt.Sprintf("%d active", smithersStatus.ActiveRuns)` when `ActiveRuns > 0`, and the pending-approvals warning when `PendingApprovals > 0`. No changes required.
- `internal/ui/model/status.go` `formatSmithersSummary` at line 148: renders `"N run(s) · M approval(s)"` aligned to the right of the status bar. No changes required.
- `drawHeader` at line 2101 in `ui.go` calls `m.header.SetSmithersStatus(m.smithersStatus)` on every draw cycle, so as soon as `m.smithersStatus` is updated the next render frame picks it up. No changes required.

### 8. Add `ListRuns` tests in `internal/smithers/client_test.go`

Add to the existing test file, following the patterns of `TestListCrons_HTTP` and `TestListCrons_Exec`:

```go
// --- ListRuns ---

func TestListRuns_HTTP(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/v1/runs", r.URL.Path)
        assert.Equal(t, "GET", r.Method)
        writeEnvelope(t, w, []Run{
            {RunID: "r1", WorkflowName: "code-review", Status: RunStatusRunning},
            {RunID: "r2", WorkflowName: "deploy",      Status: RunStatusWaitingApproval},
        })
    })

    runs, err := c.ListRuns(context.Background(), RunFilter{})
    require.NoError(t, err)
    require.Len(t, runs, 2)
    assert.Equal(t, RunStatusRunning, runs[0].Status)
    assert.Equal(t, RunStatusWaitingApproval, runs[1].Status)
}

func TestListRuns_Exec(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        assert.Equal(t, []string{"ps", "--format", "json"}, args)
        return json.Marshal([]Run{
            {RunID: "r3", WorkflowName: "test-suite", Status: RunStatusFinished},
        })
    })

    runs, err := c.ListRuns(context.Background(), RunFilter{})
    require.NoError(t, err)
    require.Len(t, runs, 1)
    assert.Equal(t, RunStatusFinished, runs[0].Status)
}

func TestListRuns_HTTPWithStatusFilter(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "running", r.URL.Query().Get("status"))
        writeEnvelope(t, w, []Run{
            {RunID: "r1", Status: RunStatusRunning},
        })
    })

    runs, err := c.ListRuns(context.Background(), RunFilter{Status: "running"})
    require.NoError(t, err)
    require.Len(t, runs, 1)
}
```

Add `SummariseRuns` tests in a new `internal/smithers/types_runs_test.go`:

```go
package smithers

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestSummariseRuns_Mixed(t *testing.T) {
    runs := []Run{
        {RunID: "a", Status: RunStatusRunning},
        {RunID: "b", Status: RunStatusWaitingApproval},
        {RunID: "c", Status: RunStatusWaitingEvent},
        {RunID: "d", Status: RunStatusFinished},
        {RunID: "e", Status: RunStatusFailed},
    }
    s := SummariseRuns(runs)
    assert.Equal(t, 3, s.ActiveRuns)
    assert.Equal(t, 1, s.PendingApprovals)
}

func TestSummariseRuns_Empty(t *testing.T) {
    s := SummariseRuns(nil)
    assert.Equal(t, 0, s.ActiveRuns)
    assert.Equal(t, 0, s.PendingApprovals)
}

func TestSummariseRuns_AllTerminal(t *testing.T) {
    runs := []Run{
        {Status: RunStatusFinished},
        {Status: RunStatusCancelled},
        {Status: RunStatusFailed},
    }
    s := SummariseRuns(runs)
    assert.Equal(t, 0, s.ActiveRuns)
    assert.Equal(t, 0, s.PendingApprovals)
}

func TestSummariseRuns_MultipleApprovals(t *testing.T) {
    runs := []Run{
        {Status: RunStatusWaitingApproval},
        {Status: RunStatusWaitingApproval},
        {Status: RunStatusRunning},
    }
    s := SummariseRuns(runs)
    assert.Equal(t, 3, s.ActiveRuns)
    assert.Equal(t, 2, s.PendingApprovals)
}
```

### 9. Add an E2E test

Add `internal/e2e/chat_active_run_summary_test.go`. The test starts a fake HTTP server that serves a fixed run list, sets `smithers.apiUrl` in the TUI config to point at it, launches the TUI, and asserts the header text appears within the poll window.

```go
package e2e_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestChatActiveRunSummary_TUI(t *testing.T) {
    if os.Getenv("CRUSH_TUI_E2E") != "1" {
        t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
    }

    // Serve a minimal Smithers API that returns 2 active runs.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/health":
            w.WriteHeader(http.StatusOK)
        case "/v1/runs":
            type run struct {
                RunID        string `json:"runId"`
                WorkflowName string `json:"workflowName"`
                Status       string `json:"status"`
            }
            type envelope struct {
                OK   bool  `json:"ok"`
                Data []run `json:"data"`
            }
            json.NewEncoder(w).Encode(envelope{
                OK: true,
                Data: []run{
                    {RunID: "r1", WorkflowName: "code-review", Status: "running"},
                    {RunID: "r2", WorkflowName: "deploy",      Status: "running"},
                },
            })
        default:
            http.NotFound(w, r)
        }
    }))
    defer srv.Close()

    configDir := t.TempDir()
    dataDir := t.TempDir()
    writeGlobalConfig(t, configDir, `{
  "smithers": {
    "apiUrl": "`+srv.URL+`"
  }
}`)
    t.Setenv("CRUSH_GLOBAL_CONFIG", configDir)
    t.Setenv("CRUSH_GLOBAL_DATA", dataDir)

    tui := launchTUI(t)
    defer tui.Terminate()

    // Header branding must appear first.
    require.NoError(t, tui.WaitForText("SMITHERS", 15*time.Second))

    // Active run count must appear within two poll cycles (≤ 25 s).
    require.NoError(t, tui.WaitForText("2 active", 25*time.Second))

    tui.SendKeys("\x03")
}
```

Key details:
- The fake server responds to `/health` (used by `isServerAvailable`) and `/v1/runs`.
- `WaitForText("2 active", 25*time.Second)` gives the first poll (fires at startup in `Init`) plus a safety margin. The startup fetch fires before the 10-second tick, so the count should appear within a few seconds of launch.
- The test uses the existing `writeGlobalConfig` and `launchTUI` helpers from `tui_helpers_test.go`.

### 10. Add a VHS tape

Add `tests/vhs/active-run-summary.tape`. VHS cannot spin up a live fake API, so this tape uses a smithers instance with a pre-seeded DB fixture or simply records the zero-state startup (no active runs, blank summary) to lock the visual baseline. The primary behavioral assertion lives in the E2E test above.

```vhs
# active-run-summary.tape — records the header in Smithers mode with no active runs.
# Set CRUSH_TUI_E2E=1 and a valid Smithers config to record with live data.

Output tests/vhs/output/active-run-summary.gif

Set Shell "bash"
Set FontSize 14
Set Width 220
Set Height 50

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures/smithers-config go run . --no-session" Sleep 100ms Enter
Sleep 5s

# Verify SMITHERS header is present (zero-state: no active runs, summary blank).
Screenshot tests/vhs/output/active-run-summary-startup.png

Type "" # ctrl+c
Sleep 500ms
```

Add `tests/vhs/fixtures/smithers-config/config.json` with the same shape used in E2E tests:

```json
{
  "smithers": {
    "dbPath": ".smithers/smithers.db"
  }
}
```

## File Plan

- [`internal/smithers/client.go`](/Users/williamcory/crush/internal/smithers/client.go) — add `ListRuns` method and `scanRuns` helper; add `"net/url"` import
- [`internal/smithers/types_runs.go`](/Users/williamcory/crush/internal/smithers/types_runs.go) — add `RunStatusSummary` struct and `SummariseRuns` function
- [`internal/smithers/types_runs_test.go`](/Users/williamcory/crush/internal/smithers/types_runs_test.go) (new) — unit tests for `SummariseRuns`
- [`internal/smithers/client_test.go`](/Users/williamcory/crush/internal/smithers/client_test.go) — add `TestListRuns_HTTP`, `TestListRuns_Exec`, `TestListRuns_HTTPWithStatusFilter`
- [`internal/ui/model/ui.go`](/Users/williamcory/crush/internal/ui/model/ui.go) — replace `smithers.NewClient()` call with `buildSmithersClient(com.Config())`; add `buildSmithersClient` helper; add `smithersRunSummaryMsg`, `smithersRunSummaryTickMsg` types; add `refreshSmithersRunSummaryCmd` and `smithersRunSummaryTickCmd`; extend `Init` and `Update` to seed and handle the poll loop
- [`internal/e2e/chat_active_run_summary_test.go`](/Users/williamcory/crush/internal/e2e/chat_active_run_summary_test.go) (new) — E2E test with fake HTTP server
- [`tests/vhs/active-run-summary.tape`](/Users/williamcory/crush/tests/vhs/active-run-summary.tape) (new) — VHS visual baseline tape
- [`tests/vhs/fixtures/smithers-config/config.json`](/Users/williamcory/crush/tests/vhs/fixtures/smithers-config/config.json) (new, if fixture dir doesn't exist) — minimal smithers config for VHS tape

No changes are required to `header.go`, `status.go`, or the `SmithersStatus` struct — the rendering already handles `ActiveRuns` and `PendingApprovals` correctly.

## Validation

```sh
# 1. Format
gofumpt -w internal/smithers internal/ui/model internal/e2e

# 2. Smithers package unit tests (includes new ListRuns + SummariseRuns tests)
go test ./internal/smithers/... -count=1 -v

# 3. Targeted test run
go test ./internal/smithers/... -run 'TestListRuns|TestSummariseRuns' -count=1 -v

# 4. Full test suite (must stay green)
go test ./... -count=1

# 5. E2E — requires CRUSH_TUI_E2E=1 and the fake server embedded in the test
CRUSH_TUI_E2E=1 go test ./internal/e2e/... -run TestChatActiveRunSummary_TUI -v -timeout 60s

# 6. VHS visual record (optional, documents zero-state baseline)
vhs tests/vhs/active-run-summary.tape

# 7. Manual live-server smoke check
#    a. Start a run in the smithers repo:
#       cd /Users/williamcory/smithers && bun run src/cli/index.ts up examples/fan-out-fan-in.tsx -d
#    b. Start the server:
#       bun run src/cli/index.ts serve --root . --port 7331
#    c. Verify the endpoint:
#       curl -s http://127.0.0.1:7331/v1/runs | jq '.'
#    d. Launch the TUI with server config:
#       SMITHERS_API_URL=http://127.0.0.1:7331 go run . --smithers
#    e. Confirm the header shows "N active" within 1–2 seconds of launch.
```

## Open Questions

1. **`/v1/runs` vs `/api/runs`**: The research doc notes the ticket acceptance criteria still references `/api/runs` but the upstream server uses `/v1/runs`. This plan targets `/v1/runs` to match actual server behavior. Confirm this is correct before implementation.
2. **Limit on the summary poll**: The plan uses `Limit: 200` to cap the list-runs query for the header poll. If there can realistically be more than 200 active runs simultaneously, consider omitting the limit or using a server-side `status=running,waiting-approval,waiting-event` filter instead.
3. **`smithersStatus` ownership on error**: The plan leaves `smithersStatus` unchanged on a fetch error (the previous count persists). An alternative is to zero the struct on error. Preference should be confirmed with the product owner.
4. **Poll interval**: 10 seconds is borrowed from the upstream tui-v2 broker sync cadence. If the header needs sub-10s freshness (e.g. for approval gate notifications), the interval should be tuned — possibly down to 5 seconds while active runs exist and back to 30 seconds when counts are zero.
5. **Config wiring scope**: The research doc flags a separate `platform-config-namespace` ticket for full config wiring. Step 3 here does the minimum needed for this feature (client construction only). Confirm whether that is sufficient or whether the broader config ticket must land first.

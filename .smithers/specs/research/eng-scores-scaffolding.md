# Research Document: eng-scores-scaffolding

## Summary

This research audits the existing score methods in `internal/smithers/client.go` and `internal/smithers/systems.go`, catalogues the data types available in `internal/smithers/types.go`, examines the wireframe requirements in `docs/smithers-tui/02-DESIGN.md:806-839`, and documents the gaps between what exists and what the scaffolding ticket must deliver.

---

## Existing Client Surface

### Score methods in `internal/smithers/client.go`

Two methods exist for fetching scorer evaluation data, both in the Scores section (lines 340–384):

**`GetScores(ctx, runID, nodeID *string) ([]ScoreRow, error)`** (line 344)

- Routes: SQLite first (prefers direct DB access — no HTTP endpoint exists upstream), exec fallback (`smithers scores <runID> --format json`).
- Requires a `runID`. For a cross-run dashboard view this is a gap: the method can only retrieve scores for a single run. The exec fallback also hard-codes a per-run invocation.
- Optional `nodeID` filter narrows results to a single node within the run.
- SQLite query: `SELECT ... FROM _smithers_scorer_results WHERE run_id = ? [AND node_id = ?] ORDER BY scored_at_ms DESC`.

**`GetAggregateScores(ctx, runID) ([]AggregateScore, error)`** (line 378)

- Wraps `GetScores(ctx, runID, nil)` and pipes results through `aggregateScores()`.
- Inherits the same per-run constraint.
- Returns `[]AggregateScore` grouped by `ScorerID`.

### Aggregation helper: `aggregateScores(rows []ScoreRow) []AggregateScore` (line 694)

- Groups `ScoreRow` entries by `ScorerID`.
- Computes: `Count`, `Mean`, `Min`, `Max`, `P50` (median via sort + midpoint interpolation), `StdDev` (sample variance).
- Returns results sorted by `ScorerID` for deterministic output.
- This helper is package-private (lowercase) but reusable within `internal/smithers`.

### Scan/parse helpers already in place (lines 666–691)

- `scanScoreRows(rows *sql.Rows) ([]ScoreRow, error)` — scans 16 columns from `_smithers_scorer_results`.
- `parseScoreRowsJSON(data []byte) ([]ScoreRow, error)` — deserialises exec output.

### Gap: no cross-run scores method

Neither `GetScores` nor `GetAggregateScores` can fetch scores across all runs. The dashboard requires a "Recent Evaluations" section and a "Scorer Summary" section that aggregate over all available data — not scoped to a single `runID`. A new method is required.

---

## Systems Layer (`internal/smithers/systems.go`)

The `eng-systems-api-client` dependency has landed. `systems.go` provides:

- `ListTables`, `GetTableSchema` — SQL browser table introspection.
- `GetTokenUsageMetrics(ctx, MetricsFilter) (*TokenMetrics, error)` — token counts, cache read/write, estimated cost. Routes: HTTP `/metrics/tokens` → SQLite `_smithers_chat_attempts` aggregate → exec `smithers metrics token-usage`.
- `GetLatencyMetrics(ctx, MetricsFilter) (*LatencyMetrics, error)` — count, mean, min, max, P50, P95 in ms. Routes: HTTP `/metrics/latency` → SQLite `_smithers_nodes` `duration_ms` → exec.
- `GetCostTracking(ctx, MetricsFilter) (*CostReport, error)` — total/input/output cost in USD, run count. Routes: HTTP `/metrics/cost` → SQLite → exec.

The `MetricsFilter` struct supports `RunID`, `NodeID`, `WorkflowPath`, `StartMs`, `EndMs`, `GroupBy` filtering.

These methods are directly relevant to the downstream tickets (`feat-scores-token-usage-metrics`, `feat-scores-cost-tracking`, `feat-scores-latency-metrics`) but are NOT needed for the scaffolding ticket — the design doc wireframe shows them as placeholder lines ("Tokens: — · Avg duration: — · Cache hit rate: —") until those downstream tickets land.

The key takeaway: the `eng-systems-api-client` dependency is satisfied. The infrastructure exists. The only missing client method for this ticket is a cross-run score list.

---

## Data Types (`internal/smithers/types.go`)

### `ScoreRow` (line 26)

```go
type ScoreRow struct {
    ID         string  // UUID row identifier
    RunID      string  // parent run
    NodeID     string  // parent node within the run
    Iteration  int     // node iteration number
    Attempt    int     // attempt within the node
    ScorerID   string  // stable scorer identifier (e.g. "relevancy")
    ScorerName string  // human display name (e.g. "Relevancy")
    Source     string  // "live" (inline) or "batch" (offline evaluation)
    Score      float64 // 0–1 normalised score
    Reason     *string // optional text explanation
    MetaJSON   *string // scorer-specific metadata
    InputJSON  *string // snapshot of node input at scoring time
    OutputJSON *string // snapshot of node output at scoring time
    LatencyMs  *int64  // time scorer took to produce this result
    ScoredAtMs int64   // Unix ms timestamp
    DurationMs *int64  // node execution duration at scoring time
}
```

For the dashboard the useful display fields are: `RunID` (truncated), `NodeID`, `ScorerName`, `Score`, `Source`, `ScoredAtMs`.

### `AggregateScore` (line 46)

```go
type AggregateScore struct {
    ScorerID   string
    ScorerName string
    Count      int
    Mean       float64
    Min        float64
    Max        float64
    P50        float64
    StdDev     float64
}
```

All fields map directly to columns in the "Scorer Summary" table wireframe: `Scorer | Count | Mean | Min | Max | P50`.

---

## Dashboard Requirements (`02-DESIGN.md:806-839`)

Section 3.16 defines three layout sections:

### Section 1 — "Today's Summary" (line 815)

```
Today's Summary
─────────────────────────────────────────
Runs: 16 total  │  12 ✓  │  3 running  │  1 ✗
Tokens: 847K input · 312K output · $4.23 est. cost
Avg duration: 4m 12s
Cache hit rate: 73%
```

For the scaffolding ticket only total evaluations count and overall mean score are populated. Token/cost/duration/cache fields render as `"—"` (downstream tickets populate them via `GetTokenUsageMetrics`, `GetCostTracking`, `GetLatencyMetrics`).

### Section 2 — "Top Workflows by Efficiency" (line 822)

The wireframe shows: `Workflow | Runs | Avg Time | Avg Cost | Success | Score`. For the scaffolding ticket the design doc spec narrows this to "Scorer Summary": `Scorer | Count | Mean | Min | Max | P50` — populated directly from `[]AggregateScore`. The "Top Workflows" table with run/cost/success requires join data not available until downstream tickets land. The scaffolding delivers the scorer-aggregated version, named "Scorer Summary".

### Section 3 — "Recent Evaluations" (line 831)

```
abc123  code-review  │ relevancy: 0.94 │ faithfulness: 0.88
jkl012  code-review  │ relevancy: 0.91 │ faithfulness: 0.95
```

For the scaffolding: last 10 `ScoreRow` entries across all runs. Columns: `Run` (8-char truncated ID), `Node`, `Scorer`, `Score`, `Source`.

### Navigation

The command palette must include a "Scores" entry. Design doc section 3.13 (line 723) shows `Scores` in the Actions group of the palette. No dedicated keyboard shortcut is specified (unlike Runs `ctrl+r` or Approvals `ctrl+a`).

---

## View Architecture Patterns (from existing views)

All Smithers views follow the pattern established in `internal/ui/views/agents.go` and `internal/ui/views/approvals.go`:

1. **Compile-time interface check**: `var _ View = (*ScoresView)(nil)`
2. **Private message types**: `scoresLoadedMsg`, `scoresErrorMsg` (unexported, package-local)
3. **Struct fields**: `client`, data slices, `width`, `height`, `loading bool`, `err error`
4. **Constructor**: `NewScoresView(client *smithers.Client) *ScoresView` — sets `loading: true`
5. **`Init()`**: returns async `tea.Cmd` that calls the client and returns typed message
6. **`Update(msg)`**: handles loaded/error/window-size/key messages; `esc`/`alt+esc` returns `PopViewMsg{}`; `r` re-calls `Init()`
7. **`View()`**: renders header line (`SMITHERS › Scores` left, `[Esc] Back` faint right), then loading/error/empty/data states
8. **`Name()`**: returns `"scores"`
9. **`ShortHelp()`**: returns `[]string{"[r] Refresh", "[Esc] Back"}`

The header pattern (left/right alignment using `lipgloss.Width` and space-padding) is identical across `agents.go:104-113` and `approvals.go:103-112`.

---

## Navigation Wiring (existing pattern)

From `internal/ui/dialog/actions.go:92-97`:

```go
ActionOpenAgentsView    struct{}
ActionOpenTicketsView   struct{}
ActionOpenApprovalsView struct{}
```

`ActionOpenScoresView` follows as the next entry in this group.

From `internal/ui/dialog/commands.go:528-533`:

```go
commands = append(commands,
    NewCommandItem(c.com.Styles, "agents",    "Agents",    "", ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals", "Approvals", "", ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets",   "Tickets",   "", ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "quit",      "Quit",      "ctrl+c", tea.QuitMsg{}),
)
```

A `"scores"` entry is inserted into this block before `"quit"`.

From `internal/ui/model/ui.go:1458-1477` — the action switch handles each view:

```go
case dialog.ActionOpenApprovalsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    approvalsView := views.NewApprovalsView(m.smithersClient)
    cmd := m.viewRouter.Push(approvalsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

`ActionOpenScoresView` follows the same 5-line pattern.

The `smithersClient` is initialised as `smithers.NewClient()` (no options) at `ui.go:342`. The `WithDBPath` option is not yet wired from config — this is a known gap mentioned in the approvals-queue plan. The scores view tolerates this: with no DB path, `c.db` is nil, `ListRecentScores` returns nil (empty), and the view shows "No score data available."

---

## SQLite Schema

The `_smithers_scorer_results` table has the 16 columns scanned by `scanScoreRows`:

```sql
CREATE TABLE _smithers_scorer_results (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL,
    node_id     TEXT NOT NULL,
    iteration   INTEGER,
    attempt     INTEGER,
    scorer_id   TEXT NOT NULL,
    scorer_name TEXT,
    source      TEXT,       -- "live" | "batch"
    score       REAL,       -- 0–1
    reason      TEXT,
    meta_json   TEXT,
    input_json  TEXT,
    output_json TEXT,
    latency_ms  INTEGER,
    scored_at_ms INTEGER,
    duration_ms INTEGER
);
```

The `scanScoreRows` helper already handles this schema. `ListRecentScores` can query it without a `run_id` filter by dropping the `WHERE run_id = ?` clause.

---

## Testing Infrastructure

### Unit test patterns (`internal/smithers/client_test.go`)

- `newExecClient(fn)` — creates a client with a mock exec function.
- `newTestServer(t, handler)` — creates an httptest server.
- `writeEnvelope(t, w, data)` — writes the `{ok: true, data: ...}` envelope.

For `ListRecentScores` the unit test will use a temporary SQLite file (created with `database/sql` + `mattn/go-sqlite3` or the existing sqlite driver) populated with seeded `_smithers_scorer_results` rows, asserting that results come back ordered by `scored_at_ms DESC` with the correct field values.

### VHS tape patterns (`tests/vhs/*.tape`)

All existing tapes use:
- `Set Shell zsh`
- `CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-<name> go run .`
- `Sleep 3s` after launch
- `Screenshot` + `Ctrl+c`

The scores tape adds a fixture SQLite DB path, delivered via a config key or environment variable.

---

## Gaps Summary

| Gap | Impact | Resolution |
|-----|--------|------------|
| No cross-run score method | Cannot populate dashboard without a `runID` | Add `ListRecentScores(ctx, limit)` in `client.go` |
| No `AggregateAllScores` method | Must re-aggregate over cross-run data | Add `AggregateAllScores(ctx, limit)` calling `aggregateScores(ListRecentScores(...))` |
| `ActionOpenScoresView` not declared | TUI cannot navigate to view | Add to `dialog/actions.go` |
| No command palette entry for scores | User cannot open view | Add to `dialog/commands.go` |
| No `ui.go` case for `ActionOpenScoresView` | Action dispatched but unhandled | Add case to `ui.go` action switch |
| No `internal/ui/views/scores.go` | View does not exist | Create file implementing `views.View` |
| No `_smithers_scorer_results` table guard | SQLite error if table absent on old DB | Treat "no such table" as empty result |
| No VHS fixture DB for scores | VHS tape cannot demonstrate data | Create `tests/vhs/fixtures/scores-test.db` |
| `smithersClient` not wired with `WithDBPath` | SQLite path not read from config | Out of scope for this ticket; view handles nil DB gracefully |

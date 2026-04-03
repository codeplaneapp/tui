# Engineering Spec: Scaffold Scores Dashboard View

## Metadata
- Ticket: `eng-scores-scaffolding`
- Feature: `SCORES_AND_ROI_DASHBOARD` (`docs/smithers-tui/features.ts:139`)
- Group: Systems And Analytics
- Dependencies: `eng-systems-api-client` (provides `GetScores`, `GetAggregateScores` on `smithers.Client`)
- Target files:
  - `internal/ui/views/scores.go` (new)
  - `internal/ui/dialog/actions.go` (modify — add `ActionOpenScoresView`)
  - `internal/ui/dialog/commands.go` (modify — add Scores entry to command palette)
  - `internal/ui/model/ui.go` (modify — wire `ActionOpenScoresView`)
  - `tests/scores_scaffolding_e2e_test.go` (new)
  - `tests/vhs/scores-scaffolding.tape` (new — VHS recording)

---

## Objective

Deliver the foundational Scores / ROI dashboard view — a static, data-driven summary panel that users access via `/scores` in the command palette. The view fetches aggregate scorer evaluation data from the Smithers client and renders three layout sections: a daily summary header, a workflow efficiency table, and a recent evaluations list. This scaffolding provides the structural shell that downstream feature tickets (`feat-scores-run-evaluations`, `feat-scores-token-usage-metrics`, `feat-scores-cost-tracking`, `feat-scores-latency-metrics`, `feat-scores-daily-and-weekly-summaries`, `feat-scores-cache-efficiency-metrics`) populate with richer data and interactivity.

This corresponds to Design Doc section 3.16 (`docs/smithers-tui/02-DESIGN.md:806-839`) and the `SCORES_AND_ROI_DASHBOARD` feature in `docs/smithers-tui/features.ts:139`. In the upstream GUI, the closest analog is the planned ROI dashboard; no shipped GUI component exists — making this a TUI-first surface.

---

## Scope

### In scope
- A `ScoresView` struct implementing the `views.View` interface (`internal/ui/views/router.go:6-12`)
- Three static layout sections rendered with lipgloss: "Today's Summary" header, "Top Workflows by Efficiency" table, and "Recent Evaluations" list — matching the wireframe at `02-DESIGN.md:810-839`
- Async data loading via `smithers.Client.GetScores()` (`internal/smithers/client.go:344`) and `smithers.Client.GetAggregateScores()` (`internal/smithers/client.go:378`)
- `ActionOpenScoresView` dialog action wired into `ui.go`
- Command palette entry "Scores" so typing `/scores` navigates to the view
- Loading, error, and empty states matching the established pattern in `AgentsView` (`internal/ui/views/agents.go:44-53, 116-129`)
- Manual refresh via `r` key
- Terminal E2E test using upstream tui-test harness patterns
- VHS happy-path recording test

### Out of scope
- Token usage metrics (`SCORES_TOKEN_USAGE_METRICS` — separate ticket `feat-scores-token-usage-metrics`)
- Tool call metrics (`SCORES_TOOL_CALL_METRICS`)
- Latency distribution and percentiles (`SCORES_LATENCY_METRICS`)
- Cache efficiency metrics (`SCORES_CACHE_EFFICIENCY_METRICS`)
- Daily/weekly time-series aggregation (`SCORES_DAILY_AND_WEEKLY_SUMMARIES`)
- Cost tracking (`SCORES_COST_TRACKING`)
- Drill-in to individual workflow or run detail
- Real-time SSE streaming of live scores
- Split-pane layout

---

## Implementation Plan

### Slice 1: `ActionOpenScoresView` dialog action and command palette entry

**Files**: `internal/ui/dialog/actions.go`, `internal/ui/dialog/commands.go`, `internal/ui/model/ui.go`

Add the navigation plumbing so the view can be reached before implementing the view itself.

1. Add `ActionOpenScoresView struct{}` to `internal/ui/dialog/actions.go:93` inside the existing grouped type declaration, next to `ActionOpenAgentsView`, `ActionOpenTicketsView`, and `ActionOpenApprovalsView`:

```go
// ActionOpenScoresView is a message to navigate to the scores view.
ActionOpenScoresView struct{}
```

2. Add a "Scores" entry in the command palette (`internal/ui/dialog/commands.go`). Insert it into the `commands = append(commands, ...)` block at line 526, alongside the existing "Agents", "Approvals", and "Tickets" entries:

```go
NewCommandItem(c.com.Styles, "scores", "Scores", "", ActionOpenScoresView{}),
```

3. In `internal/ui/model/ui.go`, handle `ActionOpenScoresView` in the dialog-action switch block (around line 1455, after the `ActionOpenApprovalsView` case):

```go
case dialog.ActionOpenScoresView:
    m.dialog.CloseDialog(dialog.CommandsID)
    scoresView := views.NewScoresView(m.smithersClient)
    cmd := m.viewRouter.Push(scoresView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

This follows the identical pattern used for Agents (`ui.go:1436-1441`), Tickets (`ui.go:1443-1448`), and Approvals (`ui.go:1450-1455`).

**Verification**: Build compiles (`go build ./...`). Selecting "Scores" from the command palette transitions to `uiSmithersView`. Since the view doesn't exist yet, wire it to a temporary stub that renders "Loading scores…".

---

### Slice 2: `ScoresView` implementing `views.View`

**File**: `internal/ui/views/scores.go`

The view struct follows the established pattern from `AgentsView` (`internal/ui/views/agents.go`):

```go
package views

// Compile-time interface check.
var _ View = (*ScoresView)(nil)

type scoresLoadedMsg struct {
    scores []smithers.ScoreRow
    agg    []smithers.AggregateScore
}

type scoresErrorMsg struct {
    err error
}

type ScoresView struct {
    client  *smithers.Client
    scores  []smithers.ScoreRow
    agg     []smithers.AggregateScore
    width   int
    height  int
    loading bool
    err     error
}
```

**Lifecycle**:

1. `Init()` — dispatches an async `tea.Cmd` that calls `client.GetScores(ctx, "", nil)` to fetch all recent score rows (empty `runID` fetches all — or uses a direct SQLite `SELECT ... ORDER BY scored_at_ms DESC LIMIT 100` query), and `client.GetAggregateScores(ctx, "")` for aggregate stats. Returns `scoresLoadedMsg` or `scoresErrorMsg`.

2. `Update(msg)` — handles:
   - `scoresLoadedMsg`: stores scores and aggregates, sets `loading = false`
   - `scoresErrorMsg`: stores error, sets `loading = false`
   - `tea.WindowSizeMsg`: updates width/height
   - `tea.KeyPressMsg`:
     - `esc` / `alt+esc` → returns `PopViewMsg{}`
     - `r` → sets `loading = true`, re-dispatches `Init()`

3. `View()` — renders three sections using lipgloss:
   - **Header line**: `SMITHERS › Scores` left-aligned, `[Esc] Back` right-aligned (matching `AgentsView.View()` pattern at `agents.go:100-113`)
   - **Loading state**: `"  Loading scores..."` (matching agents pattern at `agents.go:116-118`)
   - **Error state**: `"  Error: <msg>"` (matching agents pattern at `agents.go:121-123`)
   - **Empty state**: `"  No score data available."`
   - **Section 1 — "Today's Summary"**: Rendered from aggregated score data. Shows total evaluations count, mean score across all scorers. Placeholder text for token/cost metrics that downstream tickets will populate.
   - **Section 2 — "Scorer Summary"**: A table of aggregate scores grouped by scorer name. Columns: Scorer, Count, Mean, Min, Max, P50. Rendered from `[]AggregateScore`. Uses lipgloss for column alignment and faint header styling.
   - **Section 3 — "Recent Evaluations"**: Last 10 `ScoreRow` entries. Columns: Run ID (truncated 8 chars), Node, Scorer, Score (formatted `.2f`), Source. Uses lipgloss for layout.

   Section separators use a faint horizontal line matching the wireframe at `02-DESIGN.md:816,822,831`.

4. `Name()` → `"scores"`

5. `ShortHelp()` → `[]string{"[r] Refresh", "[Esc] Back"}`

**Data flow**:
```
User selects "Scores" from command palette
  → ui.go handles ActionOpenScoresView, pushes ScoresView onto viewRouter
  → ScoresView.Init() fires async tea.Cmd
  → tea.Cmd calls smithersClient.GetScores(ctx, "", nil) + GetAggregateScores(ctx, "")
  → smithers.Client hits SQLite _smithers_scorer_results table (preferred)
      or falls back to exec("smithers scores --format json")
  → Returns scoresLoadedMsg{scores: [...], agg: [...]}
  → ScoresView.Update() stores data, triggers re-render
  → ScoresView.View() renders three-section layout
  → User views dashboard; refreshes with r, exits with Esc
```

**Note on data loading**: The ticket's dependency `eng-systems-api-client` provides `GetScores` and `GetAggregateScores`. These methods already exist in `internal/smithers/client.go:342-384`. The current API accepts a `runID` parameter. For the scores dashboard (which shows cross-run data), we need either:
- A new `ListAllScores(ctx, limit)` method that queries without a `runID` filter, or
- Composing results from `ListRuns` + per-run `GetScores` calls

The simplest path for scaffolding is adding a `ListRecentScores(ctx context.Context, limit int) ([]ScoreRow, error)` method that queries `_smithers_scorer_results ORDER BY scored_at_ms DESC LIMIT ?` without a run filter.

**Verification**: Build and run manually. Open the TUI, press `/`, type "scores", select it. Verify the header renders. If a Smithers database exists, verify score sections populate. If not, verify the error/empty state renders.

---

### Slice 3: `ListRecentScores` client method

**File**: `internal/smithers/client.go`

Add a new method that fetches recent scores across all runs, needed for the dashboard's cross-run view:

```go
// ListRecentScores retrieves the most recent scorer results across all runs.
// Routes: SQLite (preferred) → exec fallback.
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
            return nil, err
        }
        return scanScoreRows(rows)
    }

    // Exec fallback — smithers scores without a run ID is not supported,
    // so return empty for now; downstream tickets will add HTTP endpoint.
    return nil, nil
}
```

Also add `AggregateAllScores` that computes aggregates over the recent results:

```go
// AggregateAllScores computes aggregated scorer statistics across all recent runs.
func (c *Client) AggregateAllScores(ctx context.Context, limit int) ([]AggregateScore, error) {
    rows, err := c.ListRecentScores(ctx, limit)
    if err != nil {
        return nil, err
    }
    return aggregateScores(rows), nil
}
```

This reuses the existing `aggregateScores()` helper and `scanScoreRows()` already in `client.go`.

**Verification**: Unit test in `internal/smithers/client_test.go` that passes a test SQLite database with seeded `_smithers_scorer_results` rows and asserts `ListRecentScores` returns them ordered by `scored_at_ms DESC`.

---

### Slice 4: Section rendering helpers

**File**: `internal/ui/views/scores.go` (within the `View()` method)

Implement the three-section layout as private helper functions on `ScoresView`:

```go
func (v *ScoresView) renderSummary() string      // Section 1: Today's Summary
func (v *ScoresView) renderScorerTable() string   // Section 2: Scorer Summary table
func (v *ScoresView) renderRecentScores() string  // Section 3: Recent Evaluations
```

**Section 1 — `renderSummary()`**:
- Total evaluations: `len(v.scores)`
- Overall mean: average of all `v.scores[*].Score`
- Placeholder lines for "Tokens", "Avg duration", "Cache hit rate" (rendered as `"—"` until downstream tickets provide the data)
- Layout: key-value pairs with lipgloss dim label + bold value, pipe-separated on one line

**Section 2 — `renderScorerTable()`**:
- Header row: `Scorer │ Count │ Mean │ Min │ Max │ P50` with faint styling
- Data rows from `v.agg` slice, each formatted with fixed-width columns using lipgloss
- Score values formatted as `.2f`
- Handles terminal width: truncate scorer name with ellipsis if needed (follow `approvals.go` `truncate()` pattern)
- Graceful fallback below 60 columns: hide P50 column

**Section 3 — `renderRecentScores()`**:
- Header row: `Run │ Node │ Scorer │ Score │ Source` with faint styling
- Last 10 entries from `v.scores`
- Run ID truncated to 8 chars
- Score formatted as `.2f`
- Source rendered as-is ("live" or "batch")

**Verification**: Build compiles. Manual visual check that sections render with correct alignment at 80, 120, and 200 column widths.

---

### Slice 5: Terminal E2E test (tui-test harness)

**File**: `tests/scores_scaffolding_e2e_test.go`

Model this test on the upstream `@microsoft/tui-test` harness patterns from `smithers_tmp/tests/tui.e2e.test.ts` and `smithers_tmp/tests/tui-helpers.ts`.

The upstream harness:
- Spawns the TUI as a child process (`BunSpawnBackend` in `tui-helpers.ts`)
- Strips ANSI escape sequences from output (regex: `/\x1B\[[0-9;]*[a-zA-Z]/g`)
- Sends keystrokes via stdin
- Captures buffer snapshots for assertion
- Uses 15-second timeout with buffer dump on failure

For the Go E2E test, we adapt this pattern:

```go
package tests

import (
    "os/exec"
    "testing"
    "time"
)

func TestScoresScaffolding_E2E(t *testing.T) {
    // 1. Seed a test SQLite database with _smithers_scorer_results rows
    dbPath := seedTestScoresDB(t)
    defer os.Remove(dbPath)

    // 2. Launch the TUI binary with SMITHERS_DB_PATH pointed at the test DB
    cmd := exec.Command("go", "run", ".")
    cmd.Env = append(os.Environ(), "SMITHERS_DB_PATH="+dbPath)
    // ... set up PTY via creack/pty

    // 3. Wait for initial render (chat view)
    waitForOutput(t, pty, "SMITHERS", 5*time.Second)

    // 4. Open command palette and navigate to scores
    sendKey(pty, "/")
    waitForOutput(t, pty, "Type to filter", 3*time.Second)
    sendString(pty, "scores")
    sendKey(pty, enter)

    // 5. Assert header renders
    waitForOutput(t, pty, "SMITHERS › Scores", 5*time.Second)

    // 6. Assert section headers render
    waitForOutput(t, pty, "Summary", 3*time.Second)
    waitForOutput(t, pty, "Scorer", 3*time.Second)

    // 7. Assert score data from seeded DB renders
    waitForOutput(t, pty, "relevancy", 3*time.Second) // seeded scorer name

    // 8. Press r to refresh
    sendKey(pty, 'r')
    waitForOutput(t, pty, "Loading", 2*time.Second)
    waitForOutput(t, pty, "Scorer", 5*time.Second)

    // 9. Press Esc, verify return to chat
    sendKey(pty, escape)
    waitForOutput(t, pty, "Ready", 3*time.Second)
}
```

**Test DB fixture** (`seedTestScoresDB`): Creates a temporary SQLite database with `_smithers_scorer_results` table containing 5 seeded rows across 2 runs and 2 scorers ("relevancy", "faithfulness"), matching the schema at `smithers_tmp/src/scorers/schema.ts`.

**Helper utilities** (in `tests/helpers_test.go` — shared with other E2E tests):
- `waitForOutput(t, pty, text, timeout)` — polls PTY output with ANSI stripping
- `sendKey(pty, key)` — writes escape sequences for special keys
- `sendString(pty, s)` — writes string characters sequentially
- `captureBuffer(pty)` — reads current terminal buffer

These helpers mirror the upstream `tui-helpers.ts` functions: `BunSpawnBackend.waitForText()` (poll at 100ms intervals, 10s default timeout), ANSI stripping regex, and `snapshot()` for debugging.

**Failure diagnostics**: On test failure, dump the terminal buffer to `tui-buffer.txt` (matching the upstream pattern at `tui.e2e.test.ts` where `require("fs").writeFileSync("tui-buffer.txt", tui.snapshot())` is called in catch blocks).

**Verification**: `go test ./tests/ -run TestScoresScaffolding_E2E -v -timeout 30s` passes.

---

### Slice 6: VHS happy-path recording test

**File**: `tests/vhs/scores-scaffolding.tape`

A [VHS](https://github.com/charmbracelet/vhs) tape file that records the happy path of opening the scores dashboard and returning to chat. This follows the pattern established by the existing `tests/vhs/smithers-domain-system-prompt.tape`.

```tape
# scores-scaffolding.tape — Happy-path recording for the Scores Dashboard view
Output tests/vhs/output/scores-scaffolding.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"
Set TypingSpeed 50ms

# Start the TUI with test database
Type "SMITHERS_DB_PATH=tests/vhs/fixtures/scores-test.db CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Open command palette
Type "/"
Sleep 1s

# Type "scores" to filter
Type "scores"
Sleep 500ms

# Select Scores entry
Enter
Sleep 2s

# Verify scores dashboard is visible
Screenshot tests/vhs/output/scores-scaffolding.png

# Refresh the dashboard
Type "r"
Sleep 2s

# Return to chat
Escape
Sleep 1s

Ctrl+c
Sleep 500ms
```

**VHS fixture**: `tests/vhs/fixtures/scores-test.db` — a pre-seeded SQLite database with `_smithers_scorer_results` containing representative score rows. Created by a setup script or committed as a binary fixture.

**CI integration**:
```bash
# Validate the tape parses (no syntax errors)
vhs validate tests/vhs/scores-scaffolding.tape

# Generate recording (optional, for documentation)
vhs tests/vhs/scores-scaffolding.tape
```

**Verification**: `vhs validate tests/vhs/scores-scaffolding.tape` exits 0. Running `vhs tests/vhs/scores-scaffolding.tape` produces `tests/vhs/output/scores-scaffolding.gif` showing the navigation flow.

---

## Validation

### Automated checks

| Check | Command | What it verifies |
|-------|---------|-----------------|
| Build | `go build ./...` | All new files compile, no import cycles |
| Unit tests | `go test ./internal/smithers/ -run TestListRecentScores -v` | `ListRecentScores` returns rows from SQLite ordered by `scored_at_ms DESC` |
| Unit tests | `go test ./internal/ui/views/ -run TestScoresView -v` | `ScoresView` handles loaded/error/empty states correctly |
| E2E test | `go test ./tests/ -run TestScoresScaffolding_E2E -v -timeout 30s` | Full flow: launch → palette → "scores" → dashboard renders → refresh → Esc → chat |
| VHS validate | `vhs validate tests/vhs/scores-scaffolding.tape` | Tape file syntax is valid |
| VHS record | `vhs tests/vhs/scores-scaffolding.tape` | Produces `tests/vhs/output/scores-scaffolding.gif` showing happy path |

### Manual verification paths

1. **With Smithers database present** (`.smithers/smithers.db` with scorer results):
   - Launch the TUI
   - Press `/` or `Ctrl+P` to open command palette
   - Type "scores" — "Scores" entry should appear in filtered results
   - Select it — should see "SMITHERS › Scores" header
   - Verify three sections: "Today's Summary", "Scorer Summary" table, "Recent Evaluations"
   - Verify scorer table shows correct Count/Mean/Min/Max/P50 values
   - Press `r` — "Loading scores…" flashes, then dashboard refreshes
   - Press `Esc` — returns to chat view

2. **Without Smithers database** (no `.smithers/smithers.db`):
   - Launch the TUI
   - Navigate to Scores via command palette
   - Should display "No score data available." or a graceful error
   - Should NOT crash the TUI

3. **Empty database** (database exists but `_smithers_scorer_results` table has zero rows):
   - Navigate to Scores — should display "No score data available."
   - All three section areas render without panics

4. **Narrow terminal** (< 60 columns):
   - Navigate to Scores — layout should degrade gracefully
   - P50 column hides; scorer names truncate with ellipsis

### E2E test coverage mapping (tui-test harness)

| Acceptance criterion | E2E assertion |
|---------------------|---------------|
| `internal/ui/views/scores.go` created and functional | `waitForOutput("SMITHERS › Scores")` after navigation |
| Routing to `/scores` enabled | Command palette filter → select → view transition |
| View renders score data | `waitForOutput("relevancy")` with seeded test DB |
| Refresh works | `sendKey(pty, 'r')` + `waitForOutput("Loading")` + data re-renders |
| Esc navigates back | `sendKey(pty, escape)` + `waitForOutput("Ready")` |
| VHS recording | `vhs validate tests/vhs/scores-scaffolding.tape` exits 0 |

---

## Risks

### 1. `eng-systems-api-client` dependency not yet landed

**Impact**: The `ScoresView.Init()` calls `client.GetScores()` and `client.GetAggregateScores()`, which depend on the systems API client ticket.

**Mitigation**: Both methods already exist in `internal/smithers/client.go:342-384` with working SQLite and exec fallback transport. The new `ListRecentScores` method added in Slice 3 follows the same pattern. If the dependency ticket changes the method signatures, adaptation is minimal. The view already handles nil/error returns gracefully.

### 2. No cross-run scores endpoint in upstream Smithers CLI

**Impact**: The upstream `smithers scores <run_id>` command requires a specific `runID`. The dashboard needs cross-run data (all recent scores regardless of run).

**Mitigation**: Slice 3 introduces `ListRecentScores` which queries the SQLite `_smithers_scorer_results` table directly without a `runID` filter. This is valid because Crush's `smithers.Client` already has a direct SQLite connection (`client.go` `c.db` field). The exec fallback returns `nil` (empty) rather than erroring — the view shows "No score data available." This is an acceptable degradation for the scaffolding ticket; downstream tickets can add an HTTP endpoint.

### 3. `_smithers_scorer_results` table may not exist

**Impact**: If the Smithers database exists but was created by an older version that predates the scoring system, the table won't exist and the SQLite query will fail.

**Mitigation**: Wrap the SQLite query in an error handler that treats "no such table" as an empty result rather than a hard error. The `queryDB` helper in `client.go` already returns errors; the view's `scoresErrorMsg` handler displays a graceful message.

### 4. Scores view is static — no run context

**Impact**: Unlike `AgentsView` (which lists agents) or `ApprovalsView` (which lists pending gates), the scores dashboard doesn't focus on a single entity. It aggregates data across all runs. This is a different data pattern than existing views.

**Mitigation**: The design doc wireframe at `02-DESIGN.md:810-839` explicitly defines this as a summary dashboard with three static sections. The implementation renders aggregate data once on load and refreshes on `r`. No cursor-based navigation is needed for the scaffolding — downstream tickets add drill-in capabilities.

### 5. PTY-based E2E tests are flaky on CI

**Impact**: Terminal E2E tests that rely on PTY output timing can be brittle across CI environments with varying CPU/IO speeds.

**Mitigation**: Use generous timeouts (5s per assertion), retry-with-backoff on assertion polling (100ms intervals matching upstream `POLL_INTERVAL_MS` in `tui-helpers.ts`), and dump the terminal buffer to `tui-buffer.txt` on failure for debugging. The upstream test harness (`tui-helpers.ts:waitForText`) demonstrates this exact pattern.

### 6. Upstream Smithers has no shipped GUI scores component

**Impact**: Unlike runs, agents, or tickets (which have GUI reference implementations in `gui-ref/apps/web/src/`), the scores dashboard has no shipped GUI counterpart. The design doc wireframe is the only authoritative source.

**Mitigation**: This is actually lower risk than it appears — the wireframe at `02-DESIGN.md:806-839` is detailed and prescriptive. The data types (`ScoreRow`, `AggregateScore`) are well-defined in `internal/smithers/types.go:24-55` and map directly to the upstream `smithers/src/scorers/types.ts` schema. There is no GUI behavior to reverse-engineer, only a layout to implement.

### 7. File location mismatch with ticket

**Impact**: The ticket specifies `internal/ui/scores.go` but the established pattern (agents, tickets, approvals) places view files in `internal/ui/views/`. Using the wrong path would break convention.

**Mitigation**: Create the file at `internal/ui/views/scores.go` following the convention established by `agents.go`, `tickets.go`, and `approvals.go` in the same directory. Update the ticket's source context reference if needed.

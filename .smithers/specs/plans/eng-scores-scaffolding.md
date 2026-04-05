# Implementation Plan: eng-scores-scaffolding

## Goal

Deliver the foundational Scores / ROI dashboard view. Users can navigate to it via `/scores` in the command palette, see a "Today's Summary" header, a "Scorer Summary" table, and a "Recent Evaluations" list populated from the Smithers SQLite database. The view is read-only, refreshable, and exits cleanly to chat on `Esc`. It provides the structural shell that downstream tickets populate with token usage, cost, latency, and cache efficiency metrics.

---

## Pre-flight: dependency check

Before writing any code, verify the `eng-systems-api-client` dependency has landed:

```bash
grep -n "ListTables\|GetTokenUsageMetrics\|GetLatencyMetrics\|GetCostTracking" internal/smithers/systems.go
```

Expected: all four methods present. As of the current codebase state, `internal/smithers/systems.go` exists and these methods are implemented. If the file is absent, do not proceed — block on the dependency first.

---

## Step 1: Add `ListRecentScores` and `AggregateAllScores` to the client

**File**: `/Users/williamcory/crush/internal/smithers/client.go`

The existing `GetScores` and `GetAggregateScores` methods are scoped to a single `runID`. The dashboard needs cross-run data. Add two new methods immediately after `GetAggregateScores` (after line 384):

```go
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
```

The `strings` package is already imported. No other imports needed.

**Verification**: `go build ./internal/smithers/...` passes.

---

## Step 2: Unit tests for `ListRecentScores`

**File**: `/Users/williamcory/crush/internal/smithers/client_test.go`

Add after the existing `TestGetAggregateScores` block. The test must use a real SQLite database because `ListRecentScores` only routes through SQLite (exec returns nil). Use the `database/sql` + sqlite driver already present in the module.

```go
// TestListRecentScores_SQLite seeds a temporary database and asserts ordering.
func TestListRecentScores_SQLite(t *testing.T) {
	// Create a temp SQLite file.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE _smithers_scorer_results (
		id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
		scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
		meta_json TEXT, input_json TEXT, output_json TEXT,
		latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER)`)
	require.NoError(t, err)

	// Insert 3 rows with different scored_at_ms values.
	for i, ts := range []int64{100, 300, 200} {
		_, err = db.Exec(`INSERT INTO _smithers_scorer_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("s%d", i), "run-1", "node-1", 0, 0,
			"relevancy", "Relevancy", "live", 0.8+float64(i)*0.05,
			nil, nil, nil, nil, ts, nil, nil)
		require.NoError(t, err)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, scores, 3)

	// Results must be ordered by scored_at_ms DESC: 300, 200, 100.
	assert.Equal(t, int64(300), scores[0].ScoredAtMs)
	assert.Equal(t, int64(200), scores[1].ScoredAtMs)
	assert.Equal(t, int64(100), scores[2].ScoredAtMs)
}

// TestListRecentScores_NoTable treats a missing table as empty (not an error).
func TestListRecentScores_NoTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_notables.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, _ = db.Exec("CREATE TABLE unrelated (id TEXT)")
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 10)
	require.NoError(t, err) // must NOT return an error
	assert.Empty(t, scores)
}

// TestListRecentScores_LimitRespected asserts limit is applied.
func TestListRecentScores_LimitRespected(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smithers_limit.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE _smithers_scorer_results (
		id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
		scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
		meta_json TEXT, input_json TEXT, output_json TEXT,
		latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER)`)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		_, _ = db.Exec(`INSERT INTO _smithers_scorer_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("s%d", i), "run-1", "node-1", 0, 0,
			"q", "Quality", "live", 0.5, nil, nil, nil, nil, int64(i), nil, nil)
	}
	db.Close()

	c := NewClient(WithDBPath(dbPath))
	defer c.Close()

	scores, err := c.ListRecentScores(context.Background(), 3)
	require.NoError(t, err)
	assert.Len(t, scores, 3)
}

// TestAggregateAllScores_CrossRun tests aggregation over multi-run data.
func TestAggregateAllScores_CrossRun(t *testing.T) {
	// Exec fallback returns nil; test aggregation of empty input.
	c := NewClient() // no DB, no exec func override
	aggs, err := c.AggregateAllScores(context.Background(), 100)
	require.NoError(t, err)
	assert.Empty(t, aggs)
}
```

The test file already imports `context`, `encoding/json`, `testing`, `github.com/stretchr/testify/assert`, and `github.com/stretchr/testify/require`. Add `database/sql`, `fmt`, `os`, and `path/filepath` to the import block.

**Verification**: `go test ./internal/smithers/ -run TestListRecentScores -v` passes. `go test ./internal/smithers/ -run TestAggregateAllScores_CrossRun -v` passes.

---

## Step 3: Add `ActionOpenScoresView` dialog action

**File**: `/Users/williamcory/crush/internal/ui/dialog/actions.go`

In the grouped type declaration at line 46 (`ActionNewSession`, `ActionToggleHelp`, etc.), add after `ActionOpenApprovalsView` at line 96:

```go
// ActionOpenScoresView is a message to navigate to the scores/ROI dashboard.
ActionOpenScoresView struct{}
```

**Verification**: `go build ./internal/ui/dialog/...` passes.

---

## Step 4: Add "Scores" to the command palette

**File**: `/Users/williamcory/crush/internal/ui/dialog/commands.go`

In the `append` block at line 528–533, insert a "Scores" entry before `"quit"`:

```go
commands = append(commands,
    NewCommandItem(c.com.Styles, "agents",    "Agents",    "", ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals", "Approvals", "", ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets",   "Tickets",   "", ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "scores",    "Scores",    "", ActionOpenScoresView{}),
    NewCommandItem(c.com.Styles, "quit",      "Quit",      "ctrl+c", tea.QuitMsg{}),
)
```

No keyboard shortcut is assigned to scores in v1 — the design doc places it in the Actions group without a direct binding (unlike Runs `ctrl+r` and Approvals `ctrl+a`).

**Verification**: Build passes. Open command palette (`/`), type "scores" — the Scores entry appears in filtered results.

---

## Step 5: Wire `ActionOpenScoresView` in `ui.go`

**File**: `/Users/williamcory/crush/internal/ui/model/ui.go`

In the dialog-action switch block, add a case after `ActionOpenApprovalsView` (line 1477). Following the identical 5-line pattern used by Agents, Tickets, and Approvals:

```go
case dialog.ActionOpenScoresView:
    m.dialog.CloseDialog(dialog.CommandsID)
    scoresView := views.NewScoresView(m.smithersClient)
    cmd := m.viewRouter.Push(scoresView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

The `views` package is already imported at the top of `ui.go`.

**Verification**: `go build ./...` passes. Selecting "Scores" from the command palette transitions to `uiSmithersView` state. Until Step 6 creates the view, add a temporary stub in `internal/ui/views/scores.go` that satisfies the `View` interface (see below).

---

## Step 6: Implement `ScoresView`

**File**: `/Users/williamcory/crush/internal/ui/views/scores.go` (new file)

Full implementation of the `views.View` interface. The view renders three sections.

```go
package views

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*ScoresView)(nil)

type scoresLoadedMsg struct {
	scores []smithers.ScoreRow
	agg    []smithers.AggregateScore
}

type scoresErrorMsg struct {
	err error
}

// ScoresView renders the Scores / ROI dashboard (PRD §6.14, Design §3.16).
// Three sections: Today's Summary, Scorer Summary table, Recent Evaluations.
type ScoresView struct {
	client  *smithers.Client
	scores  []smithers.ScoreRow
	agg     []smithers.AggregateScore
	width   int
	height  int
	loading bool
	err     error
}

// NewScoresView creates a new scores view.
func NewScoresView(client *smithers.Client) *ScoresView {
	return &ScoresView{
		client:  client,
		loading: true,
	}
}
```

### `Init()`

```go
// Init loads scores from the client asynchronously.
func (v *ScoresView) Init() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		scores, err := v.client.ListRecentScores(ctx, 100)
		if err != nil {
			return scoresErrorMsg{err: err}
		}
		agg := smithers.AggregateScores(scores) // see note below
		return scoresLoadedMsg{scores: scores, agg: agg}
	}
}
```

Note: `aggregateScores` is package-private. The view calls `client.AggregateAllScores` instead, or receives both scores and aggregates from a single combined call. The cleanest approach: call `ListRecentScores` to get raw rows and pass them to a new exported helper, or call `AggregateAllScores` separately. Use two sequential calls (both are synchronous and fast against local SQLite):

```go
func (v *ScoresView) Init() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		scores, err := v.client.ListRecentScores(ctx, 100)
		if err != nil {
			return scoresErrorMsg{err: err}
		}
		agg, err := v.client.AggregateAllScores(ctx, 100)
		if err != nil {
			return scoresErrorMsg{err: err}
		}
		return scoresLoadedMsg{scores: scores, agg: agg}
	}
}
```

### `Update(msg)`

```go
func (v *ScoresView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case scoresLoadedMsg:
		v.scores = msg.scores
		v.agg = msg.agg
		v.loading = false
		return v, nil

	case scoresErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.err = nil
			return v, v.Init()
		}
	}
	return v, nil
}
```

### `View()`

```go
func (v *ScoresView) View() string {
	var b strings.Builder

	// Header line: "SMITHERS › Scores" left, "[Esc] Back" right.
	header   := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Scores")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine + "\n\n")

	if v.loading {
		b.WriteString("  Loading scores...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}
	if len(v.scores) == 0 && len(v.agg) == 0 {
		b.WriteString("  No score data available.\n")
		return b.String()
	}

	b.WriteString(v.renderSummary())
	b.WriteString("\n")
	b.WriteString(v.renderScorerTable())
	b.WriteString("\n")
	b.WriteString(v.renderRecentScores())

	return b.String()
}

func (v *ScoresView) Name() string     { return "scores" }
func (v *ScoresView) ShortHelp() []string {
	return []string{"[r] Refresh", "[Esc] Back"}
}
```

### Section helpers

#### `renderSummary()` — Section 1

```go
func (v *ScoresView) renderSummary() string {
	var b strings.Builder
	bold  := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)
	sep   := faint.Render("─")

	b.WriteString(bold.Render("Today's Summary") + "\n")
	if v.width > 0 {
		b.WriteString(strings.Repeat(sep, v.width-2) + "\n")
	} else {
		b.WriteString(strings.Repeat(sep, 40) + "\n")
	}

	// Total evaluations and mean score from loaded data.
	total := len(v.scores)
	mean  := 0.0
	if total > 0 {
		sum := 0.0
		for _, s := range v.scores {
			sum += s.Score
		}
		mean = sum / float64(total)
	}

	b.WriteString(fmt.Sprintf("  Evaluations: %d   Mean score: %.2f\n", total, mean))

	// Placeholder lines for downstream tickets.
	b.WriteString(faint.Render("  Tokens: —   Avg duration: —   Cache hit rate: —") + "\n")
	b.WriteString(faint.Render("  Est. cost: —") + "\n")

	return b.String()
}
```

#### `renderScorerTable()` — Section 2

```go
func (v *ScoresView) renderScorerTable() string {
	var b strings.Builder
	bold  := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)
	sep   := faint.Render("─")

	b.WriteString(bold.Render("Scorer Summary") + "\n")
	if v.width > 0 {
		b.WriteString(strings.Repeat(sep, v.width-2) + "\n")
	} else {
		b.WriteString(strings.Repeat(sep, 40) + "\n")
	}

	if len(v.agg) == 0 {
		b.WriteString(faint.Render("  No scorer data.") + "\n")
		return b.String()
	}

	// Column widths: Scorer(20) Count(6) Mean(6) Min(6) Max(6) [P50(6) if width >= 60]
	showP50 := v.width == 0 || v.width >= 60
	header := fmt.Sprintf("  %-20s  %5s  %5s  %5s  %5s", "Scorer", "Count", "Mean", "Min", "Max")
	if showP50 {
		header += fmt.Sprintf("  %5s", "P50")
	}
	b.WriteString(faint.Render(header) + "\n")

	for _, a := range v.agg {
		name := a.ScorerName
		if name == "" {
			name = a.ScorerID
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		row := fmt.Sprintf("  %-20s  %5d  %5.2f  %5.2f  %5.2f",
			name, a.Count, a.Mean, a.Min, a.Max)
		if showP50 {
			row += fmt.Sprintf("  %5.2f", a.P50)
		}
		b.WriteString(row + "\n")
	}

	return b.String()
}
```

#### `renderRecentScores()` — Section 3

```go
func (v *ScoresView) renderRecentScores() string {
	var b strings.Builder
	bold  := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)
	sep   := faint.Render("─")

	b.WriteString(bold.Render("Recent Evaluations") + "\n")
	if v.width > 0 {
		b.WriteString(strings.Repeat(sep, v.width-2) + "\n")
	} else {
		b.WriteString(strings.Repeat(sep, 40) + "\n")
	}

	if len(v.scores) == 0 {
		b.WriteString(faint.Render("  No evaluations.") + "\n")
		return b.String()
	}

	// Header row
	b.WriteString(faint.Render(fmt.Sprintf("  %-8s  %-16s  %-16s  %5s  %s",
		"Run", "Node", "Scorer", "Score", "Source")) + "\n")

	// Last 10 entries (scores are already DESC-ordered from ListRecentScores).
	limit := 10
	if len(v.scores) < limit {
		limit = len(v.scores)
	}
	for _, s := range v.scores[:limit] {
		runID := s.RunID
		if len(runID) > 8 {
			runID = runID[:8]
		}
		nodeID := s.NodeID
		if len(nodeID) > 16 {
			nodeID = nodeID[:13] + "..."
		}
		scorer := s.ScorerName
		if scorer == "" {
			scorer = s.ScorerID
		}
		if len(scorer) > 16 {
			scorer = scorer[:13] + "..."
		}
		b.WriteString(fmt.Sprintf("  %-8s  %-16s  %-16s  %5.2f  %s\n",
			runID, nodeID, scorer, s.Score, s.Source))
	}

	return b.String()
}
```

The `time` and `math` imports are used only if the timestamp display is added in a downstream ticket. For the scaffolding, omit them if unused — the compiler will catch it. Remove unused imports before building.

**Verification**: `go build ./internal/ui/views/...` passes. Launch the TUI, open command palette, type "scores", select it. Verify the header, section titles, and loading/empty states render correctly.

---

## Step 7: Unit tests for `ScoresView`

**File**: `/Users/williamcory/crush/internal/ui/views/scores_test.go` (new file)

```go
package views

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoresView_InterfaceCompliance(t *testing.T) {
	// Compile-time check is already in scores.go; this is belt-and-suspenders.
	var _ View = (*ScoresView)(nil)
}

func TestScoresView_InitialState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	assert.True(t, v.loading)
	assert.Nil(t, v.err)
	assert.Empty(t, v.scores)
	assert.Empty(t, v.agg)
}

func TestScoresView_LoadedMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true

	scores := []smithers.ScoreRow{
		{ID: "s1", RunID: "run-abc123", NodeID: "node-1", ScorerID: "relevancy",
		 ScorerName: "Relevancy", Source: "live", Score: 0.92, ScoredAtMs: 1000},
	}
	agg := []smithers.AggregateScore{
		{ScorerID: "relevancy", ScorerName: "Relevancy", Count: 1, Mean: 0.92,
		 Min: 0.92, Max: 0.92, P50: 0.92},
	}

	v2, _ := v.Update(scoresLoadedMsg{scores: scores, agg: agg})
	sv := v2.(*ScoresView)

	assert.False(t, sv.loading)
	assert.Nil(t, sv.err)
	require.Len(t, sv.scores, 1)
	require.Len(t, sv.agg, 1)
}

func TestScoresView_ErrorMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true

	v2, _ := v.Update(scoresErrorMsg{err: errors.New("db error")})
	sv := v2.(*ScoresView)

	assert.False(t, sv.loading)
	require.NotNil(t, sv.err)
	assert.Contains(t, sv.err.Error(), "db error")
}

func TestScoresView_WindowSizeMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v2, _ := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	sv := v2.(*ScoresView)
	assert.Equal(t, 120, sv.width)
	assert.Equal(t, 40, sv.height)
}

func TestScoresView_EscReturnsPopMsg(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok)
}

func TestScoresView_RefreshTriggersInit(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyRunes, Text: "r"})
	// After pressing r, loading should be set and a cmd returned.
	assert.True(t, v.loading)
	assert.NotNil(t, cmd)
}

func TestScoresView_ViewLoadingState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = true
	out := v.View()
	assert.Contains(t, out, "SMITHERS › Scores")
	assert.Contains(t, out, "Loading scores")
}

func TestScoresView_ViewErrorState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.err = errors.New("connection refused")
	out := v.View()
	assert.Contains(t, out, "Error: connection refused")
}

func TestScoresView_ViewEmptyState(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	out := v.View()
	assert.Contains(t, out, "No score data available")
}

func TestScoresView_ViewWithData(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.scores = []smithers.ScoreRow{
		{RunID: "abc12345", NodeID: "review", ScorerID: "rel", ScorerName: "Relevancy",
		 Source: "live", Score: 0.95, ScoredAtMs: 9999},
	}
	v.agg = []smithers.AggregateScore{
		{ScorerID: "rel", ScorerName: "Relevancy", Count: 1, Mean: 0.95,
		 Min: 0.95, Max: 0.95, P50: 0.95},
	}
	v.width = 120
	out := v.View()
	assert.Contains(t, out, "Today's Summary")
	assert.Contains(t, out, "Scorer Summary")
	assert.Contains(t, out, "Recent Evaluations")
	assert.Contains(t, out, "Relevancy")
	assert.Contains(t, out, "0.95")
	assert.Contains(t, out, "abc12345")
}

func TestScoresView_NarrowTerminalHidesP50(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.agg = []smithers.AggregateScore{
		{ScorerID: "q", ScorerName: "Quality", Count: 5, Mean: 0.8, Min: 0.6, Max: 1.0, P50: 0.82},
	}
	v.width = 55 // below 60-column threshold
	out := v.renderScorerTable()
	// P50 column should not appear at narrow widths.
	assert.NotContains(t, out, "P50")
}

func TestScoresView_ScorerNameTruncation(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	v.loading = false
	v.agg = []smithers.AggregateScore{
		{ScorerID: "x", ScorerName: "A Very Long Scorer Name That Exceeds Limit",
		 Count: 1, Mean: 0.5, Min: 0.5, Max: 0.5, P50: 0.5},
	}
	v.width = 120
	out := v.renderScorerTable()
	assert.Contains(t, out, "...")
}

func TestScoresView_Name(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	assert.Equal(t, "scores", v.Name())
}

func TestScoresView_ShortHelp(t *testing.T) {
	v := NewScoresView(smithers.NewClient())
	help := v.ShortHelp()
	assert.Contains(t, help, "[r] Refresh")
	assert.Contains(t, help, "[Esc] Back")
}
```

**Verification**: `go test ./internal/ui/views/ -run TestScoresView -v` passes.

---

## Step 8: Create VHS fixture database

**File**: `tests/vhs/fixtures/scores-test.db` (binary SQLite database)

Create a setup script (run once, commit the resulting DB):

```bash
#!/usr/bin/env bash
# tests/vhs/fixtures/create-scores-db.sh
set -e
DB="tests/vhs/fixtures/scores-test.db"
rm -f "$DB"
sqlite3 "$DB" <<'SQL'
CREATE TABLE _smithers_scorer_results (
    id TEXT, run_id TEXT, node_id TEXT, iteration INTEGER, attempt INTEGER,
    scorer_id TEXT, scorer_name TEXT, source TEXT, score REAL, reason TEXT,
    meta_json TEXT, input_json TEXT, output_json TEXT,
    latency_ms INTEGER, scored_at_ms INTEGER, duration_ms INTEGER
);

INSERT INTO _smithers_scorer_results VALUES
    ('s1','run-abc12345','review-auth',0,0,'relevancy','Relevancy','live',0.94,'Highly relevant',NULL,NULL,NULL,150,1743800000100,3200),
    ('s2','run-abc12345','review-auth',0,0,'faithfulness','Faithfulness','live',0.88,'Mostly faithful',NULL,NULL,NULL,160,1743800000200,3200),
    ('s3','run-def67890','lint-check',0,0,'relevancy','Relevancy','live',0.91,NULL,NULL,NULL,NULL,140,1743800000300,1100),
    ('s4','run-def67890','lint-check',0,0,'faithfulness','Faithfulness','live',0.95,NULL,NULL,NULL,NULL,155,1743800000400,1100),
    ('s5','run-ghi11223','test-runner',0,0,'relevancy','Relevancy','batch',0.87,NULL,NULL,NULL,NULL,200,1743800000500,5000);
SQL
echo "Created $DB"
```

Commit `tests/vhs/fixtures/scores-test.db` as a binary file. The script is for documentation; run it to regenerate if needed.

**Verification**: `sqlite3 tests/vhs/fixtures/scores-test.db "SELECT COUNT(*) FROM _smithers_scorer_results"` returns `5`.

---

## Step 9: VHS recording tape

**File**: `/Users/williamcory/crush/tests/vhs/scores-scaffolding.tape` (new)

```tape
# scores-scaffolding.tape — Happy-path smoke recording for the Scores dashboard.
Output tests/vhs/output/scores-scaffolding.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with fixture DB and clean config/data dirs.
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-scores-scaffolding SMITHERS_DB_PATH=tests/vhs/fixtures/scores-test.db go run ."
Enter
Sleep 3s

# Open command palette.
Type "/"
Sleep 1s

# Filter to scores entry.
Type "scores"
Sleep 500ms

# Select Scores.
Enter
Sleep 2s

# Screenshot scores dashboard.
Screenshot tests/vhs/output/scores-scaffolding.png

# Refresh the dashboard.
Type "r"
Sleep 2s

# Return to chat.
Escape
Sleep 1s

Ctrl+c
Sleep 500ms
```

Note on `SMITHERS_DB_PATH`: the TUI config must read this env var and pass it to `smithers.NewClient(WithDBPath(...))`. Check whether `internal/config/config.go` or `internal/ui/model/ui.go` already reads `SMITHERS_DB_PATH`. If not, the tape can still run — the view will show "No score data available." which is a valid smoke test. Reading the DB path from config is the responsibility of the config wiring ticket, not this scaffolding ticket.

**Verification**: `vhs validate tests/vhs/scores-scaffolding.tape` exits 0. Running `vhs tests/vhs/scores-scaffolding.tape` produces `tests/vhs/output/scores-scaffolding.gif`.

---

## File Plan

| File | Action | Description |
|------|--------|-------------|
| `/Users/williamcory/crush/internal/smithers/client.go` | Modify | Add `ListRecentScores` and `AggregateAllScores` after line 384 |
| `/Users/williamcory/crush/internal/smithers/client_test.go` | Modify | Add `TestListRecentScores_*` and `TestAggregateAllScores_CrossRun` tests |
| `/Users/williamcory/crush/internal/ui/dialog/actions.go` | Modify | Add `ActionOpenScoresView struct{}` after line 96 |
| `/Users/williamcory/crush/internal/ui/dialog/commands.go` | Modify | Add `"scores"` command palette entry before `"quit"` at line 531 |
| `/Users/williamcory/crush/internal/ui/model/ui.go` | Modify | Add `case dialog.ActionOpenScoresView:` case after line 1477 |
| `/Users/williamcory/crush/internal/ui/views/scores.go` | Create | Full `ScoresView` implementing `views.View` |
| `/Users/williamcory/crush/internal/ui/views/scores_test.go` | Create | Unit tests for `ScoresView` state machine and rendering |
| `/Users/williamcory/crush/tests/vhs/scores-scaffolding.tape` | Create | VHS happy-path recording |
| `/Users/williamcory/crush/tests/vhs/fixtures/scores-test.db` | Create | Pre-seeded SQLite fixture database |

---

## Validation

| Check | Command | Passes when |
|-------|---------|-------------|
| Build | `go build ./...` | All files compile, no import cycles |
| Client unit tests | `go test ./internal/smithers/ -run TestListRecentScores -v` | `ListRecentScores` orders by `scored_at_ms DESC`, respects limit, returns empty (not error) when table absent |
| Client unit tests | `go test ./internal/smithers/ -run TestAggregateAllScores -v` | Empty input returns empty slice, no error |
| View unit tests | `go test ./internal/ui/views/ -run TestScoresView -v` | All state machine transitions correct; rendering contains expected strings; narrow terminal hides P50; truncation works |
| VHS validate | `vhs validate tests/vhs/scores-scaffolding.tape` | Exits 0 |
| VHS record | `vhs tests/vhs/scores-scaffolding.tape` | Produces `tests/vhs/output/scores-scaffolding.gif` |
| Manual — with DB | Launch TUI, `/`, type `scores`, Enter → see header + three sections | Three sections visible, scorer table populated, recent evaluations listed |
| Manual — no DB | Launch TUI, navigate to scores | "No score data available." renders, TUI does not crash |
| Manual — empty DB | DB exists but table has zero rows | "No score data available." renders cleanly |
| Manual — narrow terminal | Resize to < 60 cols, navigate to scores | P50 column hidden, scorer names truncated with `...`, no panic |
| Manual — refresh | Press `r` from scores view | "Loading scores..." flashes briefly, then data reloads |
| Manual — exit | Press `Esc` from scores view | Returns to chat view ("Ready..." visible in input area) |

---

## Open Questions

1. **`SMITHERS_DB_PATH` config wiring**: The `smithersClient` is constructed as `smithers.NewClient()` with no options at `ui.go:342`. The `WithDBPath` option is not yet wired from config. For the VHS tape to show real data, this wiring is needed. The scoring view tolerates nil DB gracefully (shows empty state), but documenting this gap is important. If config wiring is blocked, the VHS tape validates the empty/loading/navigation flow rather than the data path.

2. **`smithers scores` exec cross-run support**: The `ListRecentScores` exec fallback returns `nil` because the upstream CLI requires a `runID`. If a future Smithers CLI version adds `smithers scores --all --format json` (cross-run listing), the exec path in `ListRecentScores` should be updated. Leave a `// TODO(scores-http): add exec fallback when smithers scores supports --all flag` comment.

3. **Section 2 "Top Workflows by Efficiency" vs. "Scorer Summary"**: The wireframe at `02-DESIGN.md:822-829` shows a per-workflow table with `Runs | Avg Time | Avg Cost | Success | Score`. That table requires join data across `_smithers_runs` and `_smithers_scorer_results` plus the metrics methods. The scaffolding delivers a simpler "Scorer Summary" grouped by scorer. Confirm with product whether the wireframe's full table is expected in the scaffolding or is a downstream responsibility.

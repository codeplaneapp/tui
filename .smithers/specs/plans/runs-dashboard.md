## Goal

Deliver the foundational Run Dashboard view (`internal/ui/views/runs.go`) — a tabular list of Smithers runs accessible via `Ctrl+R` or the `/runs` command palette entry. Users see Run ID, Workflow Name, Status (color-coded), Node Progress, and Elapsed Time with cursor-based navigation. The view follows the established `AgentsView` / `TicketsView` / `ApprovalsView` pattern and lays the groundwork for inline details, quick actions, filtering, and live SSE updates in downstream tickets.

This corresponds to `RUNS_DASHBOARD` in `docs/smithers-tui/features.ts` and the engineering spec at `.smithers/specs/engineering/runs-dashboard.md`.

---

## Steps

### Step 1: Verify keybinding availability for Ctrl+R

Before writing any code, confirm `ctrl+r` is unused in the current codebase.

```bash
grep -rn '"ctrl+r"' internal/ui/
grep -rn 'ctrl+r' internal/ui/model/ui.go
```

Expected: zero hits (the `ctrl+shift+r` binding for attachment delete at [keys.go:146](/Users/williamcory/crush/internal/ui/model/keys.go#L146) is different). If `ctrl+r` is found elsewhere, document the conflict and proceed with command palette as the only entry point for this ticket.

---

### Step 2: Add run domain types (if eng-smithers-client-runs has not landed)

**File**: [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go)

If the `eng-smithers-client-runs` ticket has already landed, skip this step — the types will be present. If not, add these stubs to unblock compilation:

```go
// RunFilter controls which runs are returned by ListRuns.
type RunFilter struct {
    Status       string // "" = all; or "running", "finished", etc.
    WorkflowPath string
    Limit        int
    Offset       int
}

// RunSummary holds node completion counts for a run.
type RunSummary struct {
    Completed int `json:"completed"`
    Failed    int `json:"failed"`
    Cancelled int `json:"cancelled"`
    Total     int `json:"total"`
}

// Run represents a Smithers workflow run.
// Maps to run rows from GET /v1/runs and _smithers_runs SQLite table.
type Run struct {
    RunID        string     `json:"runId"`
    WorkflowPath string     `json:"workflowPath"`
    WorkflowName string     `json:"workflowName"`
    Status       string     `json:"status"`       // RunStatus values
    StartedAtMs  int64      `json:"startedAtMs"`
    FinishedAtMs *int64     `json:"finishedAtMs"`
    Summary      RunSummary `json:"summary"`
    AgentID      *string    `json:"agentId"`
}
```

Mark any type additions with a `// TODO(runs-dashboard): move to eng-smithers-client-runs` comment so they are easy to find when the dependency lands and deduplication is needed.

---

### Step 3: Add ListRuns stub to client (if eng-smithers-client-runs has not landed)

**File**: [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go)

Add immediately after `ListAgents` (around line 117):

```go
// ListRuns returns a list of Smithers runs matching the given filter.
// TODO(runs-dashboard): stub — replace with real HTTP/SQLite/exec implementation
// from eng-smithers-client-runs when that ticket lands.
func (c *Client) ListRuns(_ context.Context, _ RunFilter) ([]Run, error) {
    return nil, ErrNoTransport
}
```

**Verification**: `go build ./...` passes.

---

### Step 4: Add ActionOpenRunsView dialog action

**File**: [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go)

Add after `ActionOpenApprovalsView` (line 96):

```go
// ActionOpenRunsView is a message to navigate to the runs dashboard.
ActionOpenRunsView struct{}
```

**Verification**: `go build ./internal/ui/dialog/...` passes.

---

### Step 5: Add "Runs" entry to the command palette

**File**: [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go)

In the block starting at line 528 that adds Agents/Approvals/Tickets entries, add a "Runs" entry before "Agents" (runs is listed first in the PRD's workspace section):

```go
commands = append(commands,
    NewCommandItem(c.com.Styles, "runs", "Runs", "ctrl+r", ActionOpenRunsView{}),
    NewCommandItem(c.com.Styles, "agents", "Agents", "", ActionOpenAgentsView{}),
    NewCommandItem(c.com.Styles, "approvals", "Approvals", "", ActionOpenApprovalsView{}),
    NewCommandItem(c.com.Styles, "tickets", "Tickets", "", ActionOpenTicketsView{}),
    NewCommandItem(c.com.Styles, "quit", "Quit", "ctrl+c", tea.QuitMsg{}),
)
```

The `"ctrl+r"` string in `NewCommandItem` is the keyboard shortcut hint displayed in the palette; it does not bind the key — that is wired separately in Step 7.

**Verification**: Build passes. Open the command palette with `/` or `Ctrl+P`, type "runs" — the entry appears in filtered results.

---

### Step 6: Build RunTable stateless component

**File**: `/Users/williamcory/crush/internal/ui/components/runtable.go` (new)

```go
package components

import (
    "fmt"
    "strings"
    "time"

    "charm.land/lipgloss/v2"
    "github.com/charmbracelet/crush/internal/smithers"
)

// RunTable renders a tabular list of runs as a string.
// Stateless: call View() any time data or cursor changes.
type RunTable struct {
    Runs   []smithers.Run
    Cursor int
    Width  int // available terminal columns
}

// statusStyle returns the lipgloss style for a run status.
func statusStyle(status string) lipgloss.Style {
    switch status {
    case "running":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
    case "waiting-approval":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
    case "finished":
        return lipgloss.NewStyle().Faint(true)
    case "failed":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
    case "cancelled":
        return lipgloss.NewStyle().Faint(true).Strikethrough(true)
    case "paused":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
    default:
        return lipgloss.NewStyle()
    }
}

// progressStr formats the node completion ratio, e.g. "3/5".
func progressStr(s smithers.RunSummary) string {
    if s.Total == 0 {
        return "–/–"
    }
    done := s.Completed + s.Failed + s.Cancelled
    return fmt.Sprintf("%d/%d", done, s.Total)
}

// elapsedStr returns a human-readable elapsed time, e.g. "2m 14s".
func elapsedStr(startedAtMs int64, finishedAtMs *int64) string {
    start := time.UnixMilli(startedAtMs)
    var end time.Time
    if finishedAtMs != nil {
        end = time.UnixMilli(*finishedAtMs)
    } else {
        end = time.Now()
    }
    d := end.Sub(start).Round(time.Second)
    if d < 0 {
        d = 0
    }
    h := int(d.Hours())
    m := int(d.Minutes()) % 60
    s := int(d.Seconds()) % 60
    if h > 0 {
        return fmt.Sprintf("%dh %dm", h, m)
    }
    if m > 0 {
        return fmt.Sprintf("%dm %ds", m, s)
    }
    return fmt.Sprintf("%ds", s)
}

// View renders the table as a string. Returns an empty string if Runs is nil.
func (t RunTable) View() string {
    if len(t.Runs) == 0 {
        return ""
    }

    // Column widths — degrade gracefully on narrow terminals.
    showProgress := t.Width == 0 || t.Width >= 80
    showTime := t.Width == 0 || t.Width >= 80

    idWidth := 8
    statusWidth := 18
    progressWidth := 7
    timeWidth := 8
    cursorWidth := 2

    // Workflow column fills remaining space.
    workflowWidth := t.Width - idWidth - statusWidth - cursorWidth - 2 // 2 for padding
    if showProgress {
        workflowWidth -= progressWidth + 2
    }
    if showTime {
        workflowWidth -= timeWidth + 2
    }
    if workflowWidth < 10 {
        workflowWidth = 10
    }

    faint := lipgloss.NewStyle().Faint(true)

    var b strings.Builder

    // Header row
    cursor := "  "
    idH := padTo("ID", idWidth)
    wfH := padTo("Workflow", workflowWidth)
    stH := padTo("Status", statusWidth)
    b.WriteString(faint.Render(cursor + idH + "  " + wfH + "  " + stH))
    if showProgress {
        b.WriteString(faint.Render("  " + padTo("Nodes", progressWidth)))
    }
    if showTime {
        b.WriteString(faint.Render("  " + padTo("Time", timeWidth)))
    }
    b.WriteString("\n")

    for i, run := range t.Runs {
        if i == t.Cursor {
            cursor = "▸ "
        } else {
            cursor = "  "
        }

        id := truncateTo(run.RunID, idWidth)
        wf := truncateTo(run.WorkflowName, workflowWidth)
        if wf == "" {
            // Derive from path if WorkflowName not populated.
            parts := strings.Split(run.WorkflowPath, "/")
            wf = truncateTo(parts[len(parts)-1], workflowWidth)
        }
        st := statusStyle(run.Status).Render(padTo(run.Status, statusWidth))

        line := cursor + padTo(id, idWidth) + "  " + padTo(wf, workflowWidth) + "  " + st
        if showProgress {
            line += "  " + padTo(progressStr(run.Summary), progressWidth)
        }
        if showTime {
            line += "  " + padTo(elapsedStr(run.StartedAtMs, run.FinishedAtMs), timeWidth)
        }
        b.WriteString(line + "\n")
    }

    return b.String()
}

// padTo pads or truncates s to exactly n display columns.
func padTo(s string, n int) string {
    w := lipgloss.Width(s)
    if w >= n {
        return s[:n] // safe for ASCII; lipgloss.Width handles ANSI
    }
    return s + strings.Repeat(" ", n-w)
}

// truncateTo truncates s to at most n display columns, adding "..." if truncated.
func truncateTo(s string, n int) string {
    if lipgloss.Width(s) <= n {
        return s
    }
    if n <= 3 {
        return s[:n]
    }
    return s[:n-3] + "..."
}
```

**Verification**: Unit test below (`runtable_test.go`) passes.

---

### Step 7: Build RunTable unit tests

**File**: `/Users/williamcory/crush/internal/ui/components/runtable_test.go` (new)

```go
package components

import (
    "strings"
    "testing"

    "github.com/charmbracelet/crush/internal/smithers"
)

func makeRun(id, workflow, status string, completed, total int) smithers.Run {
    return smithers.Run{
        RunID:        id,
        WorkflowPath: ".smithers/workflows/" + workflow + ".tsx",
        WorkflowName: workflow,
        Status:       status,
        StartedAtMs:  1700000000000,
        Summary: smithers.RunSummary{
            Completed: completed,
            Total:     total,
        },
    }
}

func TestRunTable_RendersHeaders(t *testing.T) {
    rt := RunTable{
        Runs:   []smithers.Run{makeRun("abc12345", "code-review", "running", 3, 5)},
        Cursor: 0,
        Width:  120,
    }
    out := rt.View()
    if !strings.Contains(out, "Workflow") {
        t.Errorf("expected header 'Workflow', got:\n%s", out)
    }
    if !strings.Contains(out, "Status") {
        t.Errorf("expected header 'Status', got:\n%s", out)
    }
    if !strings.Contains(out, "Nodes") {
        t.Errorf("expected header 'Nodes', got:\n%s", out)
    }
}

func TestRunTable_RendersCursorIndicator(t *testing.T) {
    rt := RunTable{
        Runs: []smithers.Run{
            makeRun("aaa", "wf-a", "running", 1, 3),
            makeRun("bbb", "wf-b", "finished", 3, 3),
        },
        Cursor: 1,
        Width:  120,
    }
    lines := strings.Split(rt.View(), "\n")
    // Line 0 = header, line 1 = row 0, line 2 = row 1 (cursor)
    if len(lines) < 3 {
        t.Fatalf("expected at least 3 lines, got %d", len(lines))
    }
    if strings.Contains(lines[1], "▸") {
        t.Errorf("expected no cursor on row 0, got: %s", lines[1])
    }
    if !strings.Contains(lines[2], "▸") {
        t.Errorf("expected cursor on row 1 (index 1), got: %s", lines[2])
    }
}

func TestRunTable_RendersProgressRatio(t *testing.T) {
    rt := RunTable{
        Runs:   []smithers.Run{makeRun("x", "wf", "running", 3, 5)},
        Cursor: 0,
        Width:  120,
    }
    if !strings.Contains(rt.View(), "3/5") {
        t.Errorf("expected progress '3/5'")
    }
}

func TestRunTable_NarrowTerminalHidesProgressAndTime(t *testing.T) {
    rt := RunTable{
        Runs:   []smithers.Run{makeRun("x", "wf", "running", 3, 5)},
        Cursor: 0,
        Width:  70, // below 80-column threshold
    }
    out := rt.View()
    if strings.Contains(out, "Nodes") {
        t.Errorf("expected Nodes column hidden at width 70")
    }
}

func TestRunTable_EmptyReturnsEmptyString(t *testing.T) {
    rt := RunTable{Runs: nil, Width: 120}
    if rt.View() != "" {
        t.Errorf("expected empty string for nil runs")
    }
}
```

**Verification**: `go test ./internal/ui/components/ -run TestRunTable -v` — all tests pass.

---

### Step 8: Build RunsView

**File**: `/Users/williamcory/crush/internal/ui/views/runs.go` (new)

Follows the `AgentsView` pattern exactly, substituting the runs domain.

```go
package views

import (
    "context"
    "fmt"
    "strings"
    "time"

    "charm.land/bubbles/v2/key"
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*RunsView)(nil)

type runsLoadedMsg struct {
    runs      []smithers.Run
    loadedAt  time.Time
}

type runsErrorMsg struct {
    err error
}

// RunsView displays a tabular list of Smithers runs.
type RunsView struct {
    client   *smithers.Client
    runs     []smithers.Run
    cursor   int
    width    int
    height   int
    loading  bool
    err      error
    loadedAt time.Time // when runs were last fetched
}

// NewRunsView creates a new runs view.
func NewRunsView(client *smithers.Client) *RunsView {
    return &RunsView{
        client:  client,
        loading: true,
    }
}

// Init loads runs from the client.
func (v *RunsView) Init() tea.Cmd {
    return func() tea.Msg {
        runs, err := v.client.ListRuns(context.Background(), smithers.RunFilter{Limit: 50})
        if err != nil {
            return runsErrorMsg{err: err}
        }
        return runsLoadedMsg{runs: runs, loadedAt: time.Now()}
    }
}

// Update handles messages for the runs view.
func (v *RunsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case runsLoadedMsg:
        v.runs = msg.runs
        v.loadedAt = msg.loadedAt
        v.loading = false
        v.err = nil
        return v, nil

    case runsErrorMsg:
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

        case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
            if v.cursor > 0 {
                v.cursor--
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
            if v.cursor < len(v.runs)-1 {
                v.cursor++
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
            v.loading = true
            return v, v.Init()

        case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
            // No-op for now; future: drill into run inspector (RUNS_INLINE_RUN_DETAILS).
        }
    }
    return v, nil
}

// View renders the runs dashboard.
func (v *RunsView) View() string {
    var b strings.Builder

    // Header: "SMITHERS › Runs" left, "[Esc] Back" right.
    header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Runs")
    helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
    headerLine := header
    if v.width > 0 {
        gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
        if gap > 0 {
            headerLine = header + strings.Repeat(" ", gap) + helpHint
        }
    }
    b.WriteString(headerLine)
    b.WriteString("\n\n")

    if v.loading {
        b.WriteString("  Loading runs...\n")
        return b.String()
    }

    if v.err != nil {
        b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
        return b.String()
    }

    if len(v.runs) == 0 {
        b.WriteString("  No runs found.\n")
        return b.String()
    }

    // Render the table, clipped to available height.
    table := components.RunTable{
        Runs:   v.runs,
        Cursor: v.cursor,
        Width:  v.width,
    }.View()
    b.WriteString(table)

    // Footer: last-updated timestamp + count.
    if !v.loadedAt.IsZero() {
        elapsed := time.Since(v.loadedAt).Round(time.Second)
        footer := fmt.Sprintf("  %d runs · updated %s ago  [r] Refresh", len(v.runs), elapsed)
        b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(footer) + "\n")
    }

    return b.String()
}

// Name returns the view name.
func (v *RunsView) Name() string {
    return "runs"
}

// ShortHelp returns keybinding hints for the help bar.
func (v *RunsView) ShortHelp() []string {
    return []string{"[Enter] Inspect", "[r] Refresh", "[Esc] Back"}
}
```

**Verification**: `go build ./internal/ui/views/...` passes. The view renders loading/error/empty states without panicking.

---

### Step 9: Wire RunsView into ui.go

**File**: [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)

**Part A — dialog action handler** (around line 1472, after `ActionOpenApprovalsView` block):

```go
case dialog.ActionOpenRunsView:
    m.dialog.CloseDialog(dialog.CommandsID)
    runsView := views.NewRunsView(m.smithersClient)
    cmd := m.viewRouter.Push(runsView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

**Part B — Ctrl+R direct keybinding** (in the key-handling section where `uiChat`/`uiLanding` state is active, near the area where other global shortcuts are checked):

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
    // Only fire when not already in a Smithers view and editor is not focused.
    if m.state == uiChat || m.state == uiLanding {
        runsView := views.NewRunsView(m.smithersClient)
        cmd := m.viewRouter.Push(runsView)
        m.setState(uiSmithersView, uiFocusMain)
        cmds = append(cmds, cmd)
    }
```

Locate the right position by searching for the `ActionOpenAgentsView` handler (line 1453) and add the `ActionOpenRunsView` case immediately after the `ActionOpenApprovalsView` block. For the keybinding, search for the `uiChat` key-handling switch and add `ctrl+r` near other view-opening shortcuts.

**Verification**:
1. From chat view, press `Ctrl+R` → runs view appears with header "SMITHERS › Runs".
2. Press `Esc` → returns to chat.
3. Open command palette, type "runs", select → runs view appears.

---

### Step 10: Add E2E test

**File**: `/Users/williamcory/crush/tests/tui/runs_dashboard_e2e_test.go` (new)

Place in the `tests/tui/` directory created by `eng-smithers-client-runs`. If that directory does not exist, create it with `package tui_test`.

```go
package tui_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

// mockRuns returns a JSON array of fixture runs matching the design wireframe.
var mockRuns = []map[string]interface{}{
    {
        "runId": "abc12345", "workflowPath": ".smithers/workflows/code-review.tsx",
        "workflowName": "code-review", "status": "running",
        "startedAtMs": time.Now().Add(-2 * time.Minute).UnixMilli(),
        "summary": map[string]int{"completed": 3, "failed": 0, "cancelled": 0, "total": 5},
    },
    {
        "runId": "def45678", "workflowPath": ".smithers/workflows/deploy-staging.tsx",
        "workflowName": "deploy-staging", "status": "waiting-approval",
        "startedAtMs": time.Now().Add(-8 * time.Minute).UnixMilli(),
        "summary": map[string]int{"completed": 4, "failed": 0, "cancelled": 0, "total": 6},
    },
    {
        "runId": "ghi78901", "workflowPath": ".smithers/workflows/test-suite.tsx",
        "workflowName": "test-suite", "status": "running",
        "startedAtMs": time.Now().Add(-30 * time.Second).UnixMilli(),
        "summary": map[string]int{"completed": 1, "failed": 0, "cancelled": 0, "total": 3},
    },
}

func TestRunsDashboard_E2E(t *testing.T) {
    // 1. Start mock Smithers HTTP server returning canned /v1/runs data.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/health":
            w.WriteHeader(http.StatusOK)
        case "/v1/runs":
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(mockRuns)
        default:
            http.NotFound(w, r)
        }
    }))
    defer srv.Close()

    // 2. Launch TUI with mock server URL.
    h := newTUIHarness(t, srv.URL)
    defer h.terminate()

    // 3. Wait for initial render (chat/landing view).
    h.waitForText(t, "SMITHERS", 10*time.Second)

    // 4. Navigate to runs view via Ctrl+R.
    h.sendKeys(t, "\x12") // Ctrl+R

    // 5. Assert runs view header appears.
    h.waitForText(t, "SMITHERS \u203a Runs", 5*time.Second)

    // 6. Assert table column headers.
    h.waitForText(t, "Workflow", 3*time.Second)
    h.waitForText(t, "Status", 3*time.Second)

    // 7. Assert fixture run data appears.
    h.waitForText(t, "code-review", 3*time.Second)
    h.waitForText(t, "running", 3*time.Second)

    // 8. Navigate down — cursor moves to second row.
    h.sendKeys(t, "\x1b[B") // Down arrow
    snap := h.snapshot(t)
    if !contains(snap, "▸") {
        t.Errorf("expected cursor indicator '▸' after Down arrow, got:\n%s", snap)
    }

    // 9. Navigate up — cursor returns to first row.
    h.sendKeys(t, "\x1b[A") // Up arrow

    // 10. Press Esc — returns to chat view.
    h.sendKeys(t, "\x1b")
    h.waitForNoText(t, "SMITHERS \u203a Runs", 3*time.Second)
}
```

The `newTUIHarness`, `waitForText`, `waitForNoText`, `sendKeys`, `snapshot`, `terminate`, and `contains` helpers are defined in `tests/tui/helpers_test.go` (from `eng-smithers-client-runs`). If that file does not exist, create a minimal version following the pattern documented in the engineering spec (Section "Slice 5").

**Verification**: `go test ./tests/tui/ -run TestRunsDashboard_E2E -v -timeout 60s` passes.

---

### Step 11: Add VHS tape recording

**File**: `/Users/williamcory/crush/tests/vhs/runs-dashboard.tape` (new)

```tape
# runs-dashboard.tape — Happy-path recording for the Runs Dashboard view.
Output tests/vhs/output/runs-dashboard.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Start the TUI with mock server (requires smithers server at port 7331 or mock)
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Navigate to runs dashboard via Ctrl+R
Ctrl+R
Sleep 2s

Screenshot tests/vhs/output/runs-dashboard.png

# Navigate through runs
Down
Sleep 500ms
Down
Sleep 500ms
Up
Sleep 500ms

# Manual refresh
Type "r"
Sleep 2s

# Return to chat
Escape
Sleep 1s
```

**Verification**: `vhs validate tests/vhs/runs-dashboard.tape` exits 0.

---

### Step 12: Final integration check

Run all checks in sequence:

```bash
# Format
gofumpt -w internal/smithers internal/ui/views/runs.go internal/ui/components/runtable.go

# Build
go build ./...

# Unit tests
go test ./internal/ui/components/ -run TestRunTable -v
go test ./internal/ui/views/ -v

# E2E test
go test ./tests/tui/ -run TestRunsDashboard_E2E -v -timeout 60s

# VHS validate
vhs validate tests/vhs/runs-dashboard.tape
```

---

## File Plan

| File | Action | Owner step |
|------|--------|------------|
| [internal/smithers/types.go](/Users/williamcory/crush/internal/smithers/types.go) | Add `Run`, `RunSummary`, `RunFilter` (stub if eng-smithers-client-runs not landed) | Step 2 |
| [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go) | Add `ListRuns` stub | Step 3 |
| [internal/ui/dialog/actions.go](/Users/williamcory/crush/internal/ui/dialog/actions.go) | Add `ActionOpenRunsView struct{}` | Step 4 |
| [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go) | Add "Runs" palette entry | Step 5 |
| `/Users/williamcory/crush/internal/ui/components/runtable.go` (new) | `RunTable` stateless component | Step 6 |
| `/Users/williamcory/crush/internal/ui/components/runtable_test.go` (new) | Unit tests for `RunTable` | Step 7 |
| `/Users/williamcory/crush/internal/ui/views/runs.go` (new) | `RunsView` implementing `views.View` | Step 8 |
| [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go) | Handle `ActionOpenRunsView`, add `Ctrl+R` binding | Step 9 |
| `/Users/williamcory/crush/tests/tui/runs_dashboard_e2e_test.go` (new) | PTY E2E test | Step 10 |
| `/Users/williamcory/crush/tests/vhs/runs-dashboard.tape` (new) | VHS happy-path tape | Step 11 |

---

## Data Model for the View

```go
// RunsView state fields (internal to the view struct)
type RunsView struct {
    client   *smithers.Client   // injected; never nil
    runs     []smithers.Run     // nil until first load; empty slice = "no runs found"
    cursor   int                // index into runs; clamped to [0, len(runs)-1]
    width    int                // terminal width from tea.WindowSizeMsg
    height   int                // terminal height from tea.WindowSizeMsg
    loading  bool               // true during Init() and manual r-refresh
    err      error              // non-nil if last fetch failed
    loadedAt time.Time          // when the current runs slice was fetched
}
```

**State transitions**:

| From | Event | To |
|------|-------|----|
| `loading=true, runs=nil` | `runsLoadedMsg` | `loading=false, runs=[...]` |
| `loading=true, runs=nil` | `runsErrorMsg` | `loading=false, err=<error>` |
| `loading=false, runs=[...]` | `r` keypress | `loading=true, runs=[...]` (stale shown until reload) |
| `loading=false` | `runsLoadedMsg` | `loading=false, runs=[new]` |
| any | `tea.WindowSizeMsg` | `width=msg.Width, height=msg.Height` |
| any | `esc` / `alt+esc` | `PopViewMsg{}` emitted |
| `loading=false, len(runs)>0` | `up`/`k` | `cursor = max(0, cursor-1)` |
| `loading=false, len(runs)>0` | `down`/`j` | `cursor = min(len(runs)-1, cursor+1)` |

---

## Testing Strategy

### Unit tests (RunTable component)
- Test file: `/Users/williamcory/crush/internal/ui/components/runtable_test.go`
- Coverage: header rendering, cursor indicator placement, progress ratio calculation, elapsed time format, narrow-terminal column hiding, empty input.
- These tests are purely string-based (no PTY) and run instantly.

### Integration tests (RunsView states)
- Test file: `/Users/williamcory/crush/internal/ui/views/runs_test.go` (optional, can be added alongside main implementation)
- Mock `smithers.Client` by injecting a mock `execFunc` that returns canned JSON.
- Test `Init()` dispatches a Cmd; test `Update()` handles loaded/error/size/key messages correctly.
- Use `tea.NewTestModel` pattern if available in the Crush test suite; otherwise test via direct struct manipulation.

### E2E test (PTY harness)
- Test file: `/Users/williamcory/crush/tests/tui/runs_dashboard_e2e_test.go`
- Launches the TUI binary against a `httptest.Server` returning fixture runs.
- Assertions map directly to acceptance criteria (see table below).
- Timeout: 60 seconds total; 5–15 seconds per `waitForText` call.

### VHS recording test
- Tape: `/Users/williamcory/crush/tests/vhs/runs-dashboard.tape`
- `vhs validate` runs in CI (syntax check only, no rendering).
- `vhs runs-dashboard.tape` produces GIF for documentation/review.

---

## Acceptance Criteria → Implementation Mapping

| Acceptance Criterion | Implementation | Test Assertion |
|----------------------|----------------|----------------|
| Accessible via `Ctrl+R` from chat view | `Ctrl+R` keybinding in `ui.go` (Step 9B) | `h.sendKeys("\x12")` + `h.waitForText("SMITHERS › Runs")` |
| Accessible via `/runs` in command palette | `NewCommandItem("runs", ...)` in `commands.go` (Step 5) | Manual: open palette, type "runs", verify entry |
| Displays tabular list with columns: ID, Workflow, Status, Progress, Time | `RunTable.View()` (Step 6) | `strings.Contains(out, "Workflow")` + `"Status"` + `"Nodes"` |
| Status rendered with color (green=running, yellow=waiting-approval, red=failed) | `statusStyle()` in `runtable.go` (Step 6) | Visual check in VHS recording |
| Progress shows `n/m` nodes completed | `progressStr(run.Summary)` in `runtable.go` (Step 6) | `TestRunTable_RendersProgressRatio` |
| Elapsed time shows human-readable duration | `elapsedStr(startedAtMs, finishedAtMs)` in `runtable.go` (Step 6) | Visual check in `TestRunTable_RendersHeaders` |
| Up/Down (and j/k) navigate cursor | `key.Matches(up/k)`, `key.Matches(down/j)` in `runs.go` (Step 8) | `h.sendKeys(Down)` + `contains(snap, "▸")` |
| `r` key refreshes the list | Re-dispatches `Init()` in `Update()` (Step 8) | Visual: "Loading runs..." flashes, list reloads |
| `Esc` returns to chat view | `PopViewMsg{}` on `esc`/`alt+esc` (Step 8) | `h.sendKeys("\x1b")` + `h.waitForNoText("SMITHERS › Runs")` |
| Loading state renders gracefully | `if v.loading { b.WriteString("  Loading runs...") }` (Step 8) | Visible during slow init |
| Error state renders gracefully | `if v.err != nil { b.WriteString("  Error: ...") }` (Step 8) | Inject failing mock server |
| Empty state renders gracefully | `if len(v.runs) == 0 { b.WriteString("  No runs found.") }` (Step 8) | Mock server returning `[]` |
| Data fetched via Smithers client | `client.ListRuns(ctx, RunFilter{Limit:50})` in `Init()` (Step 8) | Mock server receives `GET /v1/runs` |
| Narrow terminal degrades gracefully | Column hiding at `< 80` cols in `RunTable` (Step 6) | `TestRunTable_NarrowTerminalHidesProgressAndTime` |

---

## Open Questions

1. **eng-smithers-client-runs dependency status**: Has this ticket landed? If yes, skip Steps 2–3. If no, the stubs in Steps 2–3 are required and must be removed (or replaced) when that ticket is integrated.

2. **`tests/tui/helpers_test.go` existence**: Does the PTY harness from `eng-smithers-client-runs` already exist at `/Users/williamcory/crush/tests/tui/helpers_test.go`? If not, Step 10 requires a minimal harness to be built alongside the E2E test.

3. **`httpGetDirect` availability**: The `ListRuns` real implementation in `eng-smithers-client-runs` adds a `httpGetDirect[T]()` helper to bypass the `{ok,data,error}` envelope. If this helper is not present when Step 3's stub is replaced, the mock server in Step 10 must return envelope-wrapped JSON as a temporary workaround.

4. **Ctrl+R conflict check**: Confirm `grep -rn '"ctrl+r"' internal/ui/` returns zero hits before wiring the keybinding in Step 9. If a conflict is found, the keybinding step should be skipped and noted as a follow-up.

5. **VHS server availability**: The VHS tape currently expects a Smithers server at `http://localhost:7331`. Should the tape use a fixture server started by a `Taskfile.yaml` task, or is a `SMITHERS_MOCK=1` env-var injection approach preferred for CI reproducibility?

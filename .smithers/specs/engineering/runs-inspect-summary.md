# Engineering Spec: Run Inspector Base View

## Metadata
- Ticket: `runs-inspect-summary`
- Feature: `RUNS_INSPECT_SUMMARY`
- Group: Runs And Inspection
- Dependencies: `runs-dashboard` (RunsView with Enter handler hook)
- Downstream consumers: `runs-dag-overview`, `runs-node-inspector`, `runs-task-*` tab tickets
- Target files:
  - `internal/ui/views/runinspect.go` (new)
  - `internal/ui/views/runinspect_test.go` (new)
  - `internal/ui/views/runs.go` (modify — wire Enter key to push RunInspectView)
  - `internal/ui/model/ui.go` (modify — handle OpenRunInspectMsg)
  - `tests/tui/runs_inspect_e2e_test.go` (new)
  - `tests/vhs/runs-inspect-summary.tape` (new)

---

## Objective

Deliver the Run Inspector base view — a detailed view of a single Smithers run that opens when the user presses Enter on a run in the Run Dashboard. The inspector shows run metadata (ID, workflow, status, elapsed time) and a scrollable node list with per-node state indicators. It also provides a `c` keybinding to open the existing `LiveChatView` for the active node, giving users a path to the live agent conversation.

This view is the parent container for all downstream inspection tickets (`runs-dag-overview`, `runs-node-inspector`, `runs-task-*`). The base view must be designed so those features can be layered in without rework.

This corresponds to `NodeInspector.tsx` in the GUI reference at `../smithers/gui/src/routes/runs/NodeInspector.tsx`.

---

## Scope

### In scope
- A `RunInspectView` struct implementing `views.View`
- Async data loading via `client.InspectRun(ctx, runID)` (already implemented in `internal/smithers/runs.go:198`)
- Run metadata header: run ID, workflow name, status (color-coded), elapsed time
- Scrollable node list with state glyphs, NodeID/Label, and elapsed time since last update
- Cursor-based Up/Down navigation through the node list
- `c` keybinding to open `LiveChatView` for the cursor-selected node
- `r` keybinding for manual refresh
- `Esc`/`q` keybinding to pop back to `RunsView`
- Wire `Enter` in `RunsView.Update()` to push `RunInspectView` for the selected run
- `OpenRunInspectMsg{RunID string}` message type for the ui.go dispatch path
- Loading, error, and empty-tasks states
- E2E test verifying inspector navigation
- VHS happy-path recording

### Out of scope
- DAG / graph visualization (`runs-dag-overview`)
- Node selection driving detail tabs (`runs-node-inspector`)
- Input/Output/Config/Chat tabs (`runs-task-*`)
- Real-time SSE updates of node states (follow-on)
- Approval gate actions from inspector (handled in `RUNS_QUICK_*`)
- Split-pane layout (deferred — base ticket uses full-width node list)

---

## Implementation Plan

### Slice 1: OpenRunInspectMsg and ui.go wiring

**Files**: `internal/ui/views/runinspect.go` (stub), `internal/ui/model/ui.go`

Before the view exists, establish the navigation plumbing.

1. In `internal/ui/views/runinspect.go`, declare the message type:
   ```go
   // OpenRunInspectMsg signals ui.go to push a RunInspectView for the given run.
   type OpenRunInspectMsg struct {
       RunID string
   }
   ```

2. In `internal/ui/model/ui.go`, add a case in the main `Update` switch (near the `PopViewMsg` handler, around line 1474) to handle `OpenRunInspectMsg`:
   ```go
   case views.OpenRunInspectMsg:
       inspectView := views.NewRunInspectView(m.smithersClient, msg.RunID)
       cmd := m.viewRouter.PushView(inspectView)
       m.setState(uiSmithersView, uiFocusMain)
       cmds = append(cmds, cmd)
   ```

3. Wire the `Enter` key in `RunsView.Update()` at `internal/ui/views/runs.go:94`:
   ```go
   case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
       if len(v.runs) > 0 && v.cursor < len(v.runs) {
           runID := v.runs[v.cursor].RunID
           return v, func() tea.Msg { return OpenRunInspectMsg{RunID: runID} }
       }
   ```

**Verification**: Build compiles. Pressing Enter on a run in the dashboard triggers `OpenRunInspectMsg` in `ui.go`. The stub inspector (renders "Loading...") is pushed onto the view stack. Esc returns to the runs list.

---

### Slice 2: RunInspectView struct and lifecycle

**File**: `internal/ui/views/runinspect.go`

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
)

// Compile-time interface check.
var _ View = (*RunInspectView)(nil)

type runInspectLoadedMsg struct {
    inspection *smithers.RunInspection
}

type runInspectErrorMsg struct {
    err error
}

// RunInspectView shows detailed run metadata and a per-node task list.
type RunInspectView struct {
    client     *smithers.Client
    runID      string
    inspection *smithers.RunInspection

    cursor  int
    width   int
    height  int
    loading bool
    err     error
}

// NewRunInspectView constructs a new inspector for the given run ID.
func NewRunInspectView(client *smithers.Client, runID string) *RunInspectView {
    return &RunInspectView{
        client:  client,
        runID:   runID,
        loading: true,
    }
}
```

**Init()**: Dispatches an async `tea.Cmd` that calls `client.InspectRun(ctx, runID)` and returns `runInspectLoadedMsg` or `runInspectErrorMsg`.

```go
func (v *RunInspectView) Init() tea.Cmd {
    runID := v.runID
    client := v.client
    return func() tea.Msg {
        inspection, err := client.InspectRun(context.Background(), runID)
        if err != nil {
            return runInspectErrorMsg{err: err}
        }
        return runInspectLoadedMsg{inspection: inspection}
    }
}
```

**Update()**: Handles:
- `runInspectLoadedMsg`: stores inspection, sets `loading = false`, clamps cursor to `len(tasks)-1`
- `runInspectErrorMsg`: stores error, sets `loading = false`
- `tea.WindowSizeMsg`: updates width/height
- `tea.KeyPressMsg`:
  - `esc`/`q`/`alt+esc` → `func() tea.Msg { return PopViewMsg{} }`
  - `up`/`k` → decrement cursor, clamped to 0
  - `down`/`j` → increment cursor, clamped to `len(tasks)-1`
  - `r` → set `loading = true`, re-dispatch `Init()`
  - `c` → if inspection loaded and cursor valid, push `LiveChatView` for the selected node

The `c` keypress emits `OpenRunInspectMsg` is **not** used here; instead it emits a new `OpenLiveChatMsg`:
```go
case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
    if v.inspection != nil && len(v.inspection.Tasks) > 0 {
        task := v.inspection.Tasks[v.cursor]
        return v, func() tea.Msg {
            return OpenLiveChatMsg{
                RunID:     v.runID,
                TaskID:    task.NodeID,
                AgentName: "",
            }
        }
    }
```

`OpenLiveChatMsg` is defined alongside `OpenRunInspectMsg` in `runinspect.go` and handled in `ui.go` similarly (push `NewLiveChatView`). This mirrors the existing `LiveChatView` constructor at `livechat.go:82`.

**Verification**: Build compiles. Unit test passes (see Slice 5).

---

### Slice 3: RunInspectView rendering

**File**: `internal/ui/views/runinspect.go`

The `View()` method renders three zones separated by dividers:

#### Zone 1: Header

```
SMITHERS › Runs › abc12345 (code-review)              [Esc] Back
```

Implementation follows the `LiveChatView.renderHeader()` pattern:
- Title: `"SMITHERS › Runs › " + truncate(runID, 8)`, with workflow name appended if loaded
- Right hint: `"[Esc] Back"` (faint)
- Gap filled with spaces using `lipgloss.Width` for ANSI-safe measurement

#### Zone 2: Run metadata sub-header

```
Status: running  │  Started: 2m 14s ago  │  Nodes: 3/5  │  ● LIVE
```

- `Status`: colored via lipgloss using the same status-style mapping as `components.RunTable`
- `Started`: elapsed time since `inspection.StartedAtMs` using `time.Since(time.UnixMilli(*startedAtMs)).Round(time.Second)`
- `Nodes`: completed+failed / total from `inspection.Summary` map
- `● LIVE` / `✓ DONE` / `✗ FAILED` terminal indicator based on `inspection.Status.IsTerminal()`

Render as faint pipe-separated items (same as `LiveChatView.renderSubHeader()`).

#### Zone 3: Node list

```
─────────────────────────────────────────────────────────────────────
  ● fetch-deps        finished  ✓   0m 12s
  ● parse-config      finished  ✓   0m 08s
▸ ● review-auth       running   ●   2m 14s
  ○ deploy            pending       —
```

Column layout:

| Column       | Source                   | Width | Notes                              |
|--------------|--------------------------|-------|------------------------------------|
| Cursor       | (computed)               | 2     | `▸ ` or `  `                       |
| State glyph  | `task.State`             | 2     | See glyph table below              |
| Label/NodeID | `task.Label` or NodeID   | flex  | Prefer Label; fall back to NodeID  |
| State text   | `task.State`             | 12    | Color-coded (see below)            |
| Attempt      | `task.LastAttempt`       | 4     | `#N` or blank if nil/0             |
| Elapsed      | `task.UpdatedAtMs`       | 8     | Time since last update, or `—`     |

**State glyph and color mapping**:

| TaskState   | Glyph | lipgloss style                         |
|-------------|-------|----------------------------------------|
| `running`   | `●`   | Green foreground                       |
| `finished`  | `●`   | Faint/dim                              |
| `failed`    | `●`   | Red foreground                         |
| `pending`   | `○`   | Faint/dim                              |
| `cancelled` | `–`   | Faint/dim, strikethrough on state text |
| `skipped`   | `↷`   | Faint/dim                              |
| `blocked`   | `⏸`  | Yellow foreground                      |

**Selected row**: `lipgloss.NewStyle().Reverse(true)` on the full row for the cursor position.

**Scroll**: For the base ticket, the node list is unscrolled if tasks fit within `visibleHeight()`. `visibleHeight()` = `height - 5` (header + sub-header + divider + help bar + 1 margin). If tasks exceed visible height, clamp display to a window starting at `max(0, cursor - visibleHeight/2)`.

**Error state**: `"  Error: <msg>"` in the body zone.
**Empty state**: `"  No nodes found."` in the body zone.
**Loading state**: `"  Loading run..."` in the body zone with no zones 2/3.

#### Zone 4: Help bar

```
[↑↓/jk] navigate  [c] chat  [r] refresh  [q/esc] back
```

Rendered via `ShortHelp()`:
```go
func (v *RunInspectView) ShortHelp() []key.Binding {
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
        key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
        key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
    }
}
```

**Verification**: Build and render test with fixture data. Verify all four zones render without truncation at 80-col and 120-col widths.

---

### Slice 4: OpenLiveChatMsg and ui.go wiring

**Files**: `internal/ui/views/runinspect.go`, `internal/ui/model/ui.go`

1. In `runinspect.go` (same file as `OpenRunInspectMsg`):
   ```go
   // OpenLiveChatMsg signals ui.go to push a LiveChatView for the given run/node.
   type OpenLiveChatMsg struct {
       RunID     string
       TaskID    string // optional: filter to a single node's chat
       AgentName string // optional display hint
   }
   ```

2. In `ui.go`, add a case near the `OpenRunInspectMsg` handler:
   ```go
   case views.OpenLiveChatMsg:
       chatView := views.NewLiveChatView(m.smithersClient, msg.RunID, msg.TaskID, msg.AgentName)
       cmd := m.viewRouter.PushView(chatView)
       m.setState(uiSmithersView, uiFocusMain)
       cmds = append(cmds, cmd)
   ```

**Verification**: From the inspector, press `c` on a running node → `LiveChatView` opens showing chat for that node. Press `Esc` → returns to inspector. Press `Esc` again → returns to runs list.

---

### Slice 5: Unit tests

**File**: `internal/ui/views/runinspect_test.go`

Test cases (using table-driven Go tests):

| Test | Setup | Assertion |
|------|-------|-----------|
| `TestRunInspectView_Loading` | Loading state | `View()` contains "Loading run..." |
| `TestRunInspectView_Error` | `err = errors.New("connection refused")` | `View()` contains "Error: connection refused" |
| `TestRunInspectView_EmptyTasks` | Loaded inspection, no tasks | `View()` contains "No nodes found." |
| `TestRunInspectView_NodeList` | 3 tasks (running, finished, failed) | `View()` contains all three NodeIDs, correct glyphs `●`, `✓`, `✗` |
| `TestRunInspectView_Cursor` | 3 tasks, cursor=1 | `View()` has `▸` on second row only |
| `TestRunInspectView_Header` | Loaded, `RunID="abc12345"`, `WorkflowName="code-review"` | Header contains `"abc12345"` and `"code-review"` |
| `TestRunInspectView_EnterEmitsMsg` | Loaded, press `enter` on RunsView with run selected | `Update()` returns `OpenRunInspectMsg{RunID: "abc12345"}` |
| `TestRunInspectView_ChatEmitsMsg` | Loaded, cursor on node, press `c` | `Update()` returns `OpenLiveChatMsg{RunID: "...", TaskID: "..."}` |
| `TestRunInspectView_EscEmitsPopMsg` | Loaded, press `esc` | `Update()` returns `PopViewMsg{}` |

Fixture data helper:
```go
func fixtureInspection() *smithers.RunInspection {
    label1 := "review-auth"
    label2 := "fetch-deps"
    label3 := "deploy"
    attempt1 := 1
    now := time.Now().UnixMilli()
    return &smithers.RunInspection{
        RunSummary: smithers.RunSummary{
            RunID:        "abc12345",
            WorkflowName: "code-review",
            Status:       smithers.RunStatusRunning,
            StartedAtMs:  &now,
        },
        Tasks: []smithers.RunTask{
            {NodeID: "fetch-deps",  Label: &label2, State: smithers.TaskStateFinished},
            {NodeID: "review-auth", Label: &label1, State: smithers.TaskStateRunning, LastAttempt: &attempt1},
            {NodeID: "deploy",      Label: &label3, State: smithers.TaskStatePending},
        },
    }
}
```

**Verification**: `go test ./internal/ui/views/ -run TestRunInspect -v` passes.

---

### Slice 6: E2E test

**File**: `tests/tui/runs_inspect_e2e_test.go`

Model on `tests/tui/runs_dashboard_e2e_test.go`. The mock server must handle two endpoints:

1. `GET /v1/runs` → returns the canned runs list (for RunsView)
2. `GET /v1/runs/:id` → returns a single `RunSummary` for the inspected run

Node tasks fall back to exec since SQLite is not available in the E2E mock. Add exec mock support or use a pre-seeded SQLite fixture file.

Test flow:
```
1. Start mock Smithers HTTP server + mock exec binary
2. Launch TUI binary with SMITHERS_API_URL pointing at mock
3. Wait for "SMITHERS" header → confirm chat view loaded
4. Send Ctrl+R → wait for "SMITHERS › Runs" header
5. Send Down (optional, move to first run) → cursor on abc12345
6. Send Enter → wait for "SMITHERS › Runs › abc12345"
7. Assert run metadata visible: "code-review", "running"
8. Assert node list visible: "review-auth", "fetch-deps"
9. Assert cursor indicator "▸" is visible
10. Send Down → assert cursor moves to next node
11. Send Esc → wait for "SMITHERS › Runs" header (returned to runs list)
12. Send Esc → wait for "SMITHERS" (returned to chat)
```

**Mock data** (same 3 runs as `runs-dashboard` E2E, plus node data for `abc12345`):
- Run `abc12345`: `code-review`, `running`, nodes: `fetch-deps` (finished), `review-auth` (running), `deploy` (pending)

**Acceptance criterion mapping**:

| Acceptance criterion | E2E assertion |
|---------------------|---------------|
| Enter opens Run Inspector | `sendKey(Enter)` → `waitForOutput("SMITHERS › Runs › abc12345")` |
| Displays run metadata | `waitForOutput("code-review")` + `waitForOutput("running")` |
| E2E test verifying inspector navigation | Steps 9-11 above |

**Verification**: `go test ./tests/tui/ -run TestRunsInspect_E2E -v -timeout 30s` passes.

---

### Slice 7: VHS recording

**File**: `tests/vhs/runs-inspect-summary.tape`

```tape
# runs-inspect-summary.tape — Happy-path recording for the Run Inspector view
Output tests/vhs/output/runs-inspect-summary.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"
Set TypingSpeed 50ms
Set Env SMITHERS_API_URL "http://localhost:7331"
Set Env CRUSH_GLOBAL_CONFIG "tests/vhs/fixtures"

Type "go run ."
Enter
Sleep 3s

# Navigate to runs dashboard
Ctrl+R
Sleep 2s

# Move cursor to first run and open inspector
Down
Sleep 300ms
Enter
Sleep 2s

# Verify inspector is visible
# Navigate through nodes
Down
Sleep 500ms
Down
Sleep 500ms
Up
Sleep 500ms

# Press 'r' to refresh
Type "r"
Sleep 2s

# Return to runs list
Escape
Sleep 1s

# Return to chat
Escape
Sleep 1s
```

**Verification**: `vhs validate tests/vhs/runs-inspect-summary.tape` exits 0.

---

## Data Flow

```
User presses Enter on a run in RunsView
  │
  ▼
RunsView.Update() returns OpenRunInspectMsg{RunID: "abc12345"}
  │
  ▼
ui.go handles OpenRunInspectMsg:
  creates RunInspectView("abc12345"), calls m.viewRouter.PushView(inspectView)
  Push calls inspectView.Init()
  │
  ▼  (goroutine: Bubble Tea runs the Cmd)
smithers.Client.InspectRun(ctx, "abc12345")
  │
  ├─[1] GetRunSummary: HTTP GET /v1/runs/abc12345
  │     + getRunTasks: SQLite _smithers_nodes WHERE run_id = "abc12345"
  │
  ├─[2] GetRunSummary: SQLite _smithers_runs WHERE run_id = "abc12345"
  │     + getRunTasks: SQLite _smithers_nodes
  │
  └─[3] exec: smithers inspect abc12345 --format json
         exec: smithers inspect abc12345 --nodes --format json
  │
  ▼
runInspectLoadedMsg{inspection: *RunInspection}
  │
  ▼
RunInspectView.Update(runInspectLoadedMsg)
  stores inspection, loading=false
  │
  ▼
RunInspectView.View()
  renders header + sub-header + divider + node list
  │
  ▼
User presses 'c' on "review-auth" node
  │
  ▼
RunInspectView.Update() returns OpenLiveChatMsg{RunID: "abc12345", TaskID: "review-auth"}
  │
  ▼
ui.go handles OpenLiveChatMsg:
  creates LiveChatView("abc12345", "review-auth", ""), pushes it
  │
  ▼
User sees live chat for the "review-auth" node
User presses Esc → returns to inspector
User presses Esc → returns to runs list
```

---

## Risks

### 1. No `PushViewMsg` pattern in existing views
**Impact**: Views cannot directly push sibling views. `RunInspectView` needs to push `LiveChatView` on `c` press. The solution (emit `OpenLiveChatMsg`, handled in `ui.go`) requires adding a new message type and a handler in `ui.go`.
**Mitigation**: This pattern is clean and matches how `ActionOpenRunsView` works. One new message type (`OpenLiveChatMsg`) and one new `ui.go` case. Low risk.

### 2. Node task data unavailable without running server or SQLite
**Impact**: `InspectRun` enriches `RunSummary` with `RunTask` from SQLite or exec. If neither is available, `inspection.Tasks` is empty (best-effort, no error). The inspector will show the run metadata but display "No nodes found." in the node list.
**Mitigation**: This is the documented behavior of `InspectRun` — task enrichment is best-effort. The metadata header still renders correctly. The E2E test uses a server + exec mock to supply task data.

### 3. Wide node IDs exceeding terminal width
**Impact**: Node IDs like `very-long-workflow-node-name-with-context` will overflow the flex column and break alignment.
**Mitigation**: `truncate(label, flexWidth)` from `helpers.go` caps the label column at the available width. Use `lipgloss.Width` for ANSI-safe measurement of rendered row width.

### 4. `runs-dashboard` Enter key currently no-op
**Impact**: The `runs-dashboard` ticket shipped `enter` as a no-op. This ticket replaces that no-op with the inspector push. If `runs-dashboard` and `runs-inspect-summary` are developed concurrently, there may be a merge conflict at that line.
**Mitigation**: The change is a single case arm in `runs.go:94`. Trivial to resolve. The comment "future: drill into run inspector" documents the intent.

### 5. LiveChatView currently only fetched by run, not filtered by node
**Impact**: `NewLiveChatView(client, runID, taskID, agentName)` accepts a `taskID` but the underlying `client.GetChatOutput` may return all chat blocks for the run, not just the selected node. The `LiveChatView` filters display by `taskID` (checked at `livechat.go:466`) but the fetch is unfiltered.
**Mitigation**: The display filtering in `LiveChatView` is sufficient for the base ticket. Node-specific chat fetching is a follow-on concern for the `runs-task-chat-tab` ticket.

---

## Validation

### Automated checks

| Check | Command | What it verifies |
|-------|---------|-----------------|
| Build | `go build ./...` | New files compile, no import cycles |
| Unit tests | `go test ./internal/ui/views/ -run TestRunInspect -v` | All 9 unit test cases pass |
| E2E test | `go test ./tests/tui/ -run TestRunsInspect_E2E -v -timeout 30s` | Full navigation flow: dashboard → inspect → nodes → chat → back |
| VHS validate | `vhs validate tests/vhs/runs-inspect-summary.tape` | Tape file syntax is valid |

### Manual verification paths

1. **With Smithers server running** (`smithers up --serve`):
   - Press `Ctrl+R` → runs list appears
   - Navigate to a run, press Enter → inspector opens with run metadata and node list
   - Verify node states are color-coded correctly
   - Press `c` on a running node → live chat opens for that node
   - Press Esc → returns to inspector
   - Press Esc → returns to runs list

2. **Without server** (exec fallback):
   - Press Ctrl+R → error state or empty state (no server)
   - If a `smithers ps` binary is available: runs appear, Enter → inspector opens
   - Node list may be empty ("No nodes found.") if `smithers inspect --nodes` is unavailable

3. **Narrow terminal** (< 80 cols):
   - Inspector renders header and node list without overflow
   - Long node labels are truncated with `...`

### E2E test coverage mapping

| Acceptance criterion | E2E assertion |
|---------------------|---------------|
| Pressing Enter on a run opens the Run Inspector | `sendKey(Enter)` + `waitForOutput("SMITHERS › Runs › abc12345")` |
| Displays run metadata (time, status, overall progress) | `waitForOutput("code-review")` + `waitForOutput("running")` |
| E2E test verifying inspector navigation | Full flow test: navigate nodes, Esc returns to runs list |

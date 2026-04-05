## Goal

Deliver the Run Inspector view (`internal/ui/views/runinspect.go`) — a detailed single-run view that opens when the user presses Enter on any run in the Run Dashboard. The inspector renders run metadata (ID, workflow, status, elapsed time) and a scrollable per-node task list with state glyphs. It also provides a `c` shortcut to open the existing `LiveChatView` for the selected node.

This corresponds to `RUNS_INSPECT_SUMMARY` in `docs/smithers-tui/features.ts` and the engineering spec at `.smithers/specs/engineering/runs-inspect-summary.md`. The view is the parent container for the downstream DAG, node selection, and task-tabs tickets.

---

## Steps

### Step 1: Declare OpenRunInspectMsg and OpenLiveChatMsg

**File**: `internal/ui/views/runinspect.go` (new, initially just the message types)

Add two message types that `ui.go` will handle to push views:

```go
package views

// OpenRunInspectMsg signals ui.go to push a RunInspectView for the given run.
type OpenRunInspectMsg struct {
    RunID string
}

// OpenLiveChatMsg signals ui.go to push a LiveChatView for the given run/node.
type OpenLiveChatMsg struct {
    RunID     string
    TaskID    string // optional: filter display to a single node's chat
    AgentName string // optional display hint for the sub-header
}
```

**Verification**: `go build ./internal/ui/views/...` passes with just these declarations.

---

### Step 2: Wire OpenRunInspectMsg in ui.go

**File**: `internal/ui/model/ui.go`

Locate the `PopViewMsg` handler (around line 1474). Immediately below it, add:

```go
case views.OpenRunInspectMsg:
    inspectView := views.NewRunInspectView(m.smithersClient, msg.RunID)
    cmd := m.viewRouter.PushView(inspectView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)

case views.OpenLiveChatMsg:
    chatView := views.NewLiveChatView(m.smithersClient, msg.RunID, msg.TaskID, msg.AgentName)
    cmd := m.viewRouter.PushView(chatView)
    m.setState(uiSmithersView, uiFocusMain)
    cmds = append(cmds, cmd)
```

**Verification**: Build passes. (The view doesn't exist yet; add a stub `NewRunInspectView` that panics("not implemented") to keep the compiler happy during this step.)

---

### Step 3: Wire Enter in RunsView

**File**: `internal/ui/views/runs.go`

Find the `enter` case at line 94 (currently a no-op with the comment "future: drill into run inspector"). Replace it:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
    if len(v.runs) > 0 && v.cursor < len(v.runs) {
        runID := v.runs[v.cursor].RunID
        return v, func() tea.Msg { return OpenRunInspectMsg{RunID: runID} }
    }
```

**Verification**: Build passes. From the runs list, pressing Enter with a run selected triggers `OpenRunInspectMsg`. The stub inspector view is pushed. Esc returns to the runs list.

---

### Step 4: Implement RunInspectView

**File**: `internal/ui/views/runinspect.go`

Replace the stub with the full implementation. Key implementation points (full signatures in engineering spec):

#### 4a: Struct and constructor
```go
type RunInspectView struct {
    client     *smithers.Client
    runID      string
    inspection *smithers.RunInspection
    cursor     int
    width      int
    height     int
    loading    bool
    err        error
}

func NewRunInspectView(client *smithers.Client, runID string) *RunInspectView {
    return &RunInspectView{client: client, runID: runID, loading: true}
}
```

#### 4b: Init()
Dispatch an async `tea.Cmd` calling `client.InspectRun(ctx, runID)`. Return `runInspectLoadedMsg` or `runInspectErrorMsg`.

#### 4c: Update()
Handle `runInspectLoadedMsg`, `runInspectErrorMsg`, `tea.WindowSizeMsg`, and key presses:
- `esc`/`q`/`alt+esc` → `PopViewMsg{}`
- `up`/`k` → cursor--
- `down`/`j` → cursor++
- `r` → reload
- `c` → emit `OpenLiveChatMsg` for selected node

#### 4d: View()
Four rendering zones:
1. **Header**: `"SMITHERS › Runs › <runID[:8]>"` left, `"[Esc] Back"` right
2. **Sub-header**: `Status: <colored> │ Started: <elapsed> │ Nodes: <n>/<total>` (faint)
3. **Divider**: `strings.Repeat("─", v.width)` (faint)
4. **Node list**: cursor indicator + state glyph + label + state text + attempt + elapsed

Node list state glyphs and colors (from engineering spec state table):
- `running` → `●` green
- `finished` → `●` faint
- `failed` → `●` red
- `pending` → `○` faint
- `cancelled` → `–` faint strikethrough
- `skipped` → `↷` faint
- `blocked` → `⏸` yellow

Selected row uses `lipgloss.NewStyle().Reverse(true)` for the full row.

Loading state: render only the header + "  Loading run..."
Error state: render only the header + "  Error: <msg>"
Empty tasks: render header + sub-header + divider + "  No nodes found."

#### 4e: ShortHelp()
Return bindings: `↑↓/jk navigate`, `c chat`, `r refresh`, `q/esc back`.

**Verification**: `go build ./...` passes. Manually launch TUI, Ctrl+R, Enter → inspector renders with header and (if server available) node list.

---

### Step 5: Unit tests

**File**: `internal/ui/views/runinspect_test.go`

Write table-driven tests for all 9 test cases from the engineering spec. Use the `fixtureInspection()` helper:

```go
func fixtureInspection() *smithers.RunInspection {
    label1, label2, label3 := "review-auth", "fetch-deps", "deploy"
    attempt1 := 1
    now := time.Now().UnixMilli()
    return &smithers.RunInspection{
        RunSummary: smithers.RunSummary{
            RunID: "abc12345", WorkflowName: "code-review",
            Status: smithers.RunStatusRunning, StartedAtMs: &now,
        },
        Tasks: []smithers.RunTask{
            {NodeID: "fetch-deps",  Label: &label2, State: smithers.TaskStateFinished},
            {NodeID: "review-auth", Label: &label1, State: smithers.TaskStateRunning, LastAttempt: &attempt1},
            {NodeID: "deploy",      Label: &label3, State: smithers.TaskStatePending},
        },
    }
}
```

For message-emission tests, call `v.Update(tea.KeyPressMsg{...})` and assert the returned `tea.Cmd` produces the expected message when invoked.

**Verification**: `go test ./internal/ui/views/ -run TestRunInspect -v` — all 9 cases pass.

---

### Step 6: E2E test

**File**: `tests/tui/runs_inspect_e2e_test.go`

Build on the `runs_dashboard_e2e_test.go` harness. The mock server needs two endpoints:
- `GET /v1/runs` → canned list (same as runs-dashboard E2E)
- `GET /v1/runs/abc12345` → single `RunSummary` JSON for `abc12345`

For node data, add a mock exec binary or SQLite fixture so `InspectRun`'s task enrichment returns the 3-node fixture.

Test sequence:
1. Start mock server, launch TUI, wait for "SMITHERS"
2. `Ctrl+R` → wait for "SMITHERS › Runs"
3. `Down` (optional cursor move), `Enter` → wait for "SMITHERS › Runs › abc12345"
4. Assert "code-review" and "running" visible
5. Assert "review-auth" and "fetch-deps" visible in node list
6. Assert `▸` cursor indicator visible
7. `Down` → assert cursor moves (different node highlighted)
8. `Esc` → wait for "SMITHERS › Runs"
9. `Esc` → wait for "SMITHERS" (back at chat)

**Verification**: `go test ./tests/tui/ -run TestRunsInspect_E2E -v -timeout 30s` passes.

---

### Step 7: VHS recording

**File**: `tests/vhs/runs-inspect-summary.tape`

```tape
Output tests/vhs/output/runs-inspect-summary.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"
Set Env CRUSH_GLOBAL_CONFIG "tests/vhs/fixtures"

Type "go run ."
Enter
Sleep 3s

Ctrl+R
Sleep 2s

Down
Sleep 300ms
Enter
Sleep 2s

Down
Sleep 500ms
Down
Sleep 500ms
Up
Sleep 500ms

Type "r"
Sleep 2s

Escape
Sleep 1s

Escape
Sleep 1s
```

**Verification**: `vhs validate tests/vhs/runs-inspect-summary.tape` exits 0.

---

## Checklist

- [ ] `OpenRunInspectMsg` and `OpenLiveChatMsg` declared in `runinspect.go`
- [ ] `ui.go` handles `OpenRunInspectMsg` (push `RunInspectView`) and `OpenLiveChatMsg` (push `LiveChatView`)
- [ ] `RunsView.Update()` Enter case emits `OpenRunInspectMsg` for selected run
- [ ] `RunInspectView` struct, constructor, `Init()`, `Update()`, `View()`, `Name()`, `SetSize()`, `ShortHelp()` all implemented
- [ ] Node list renders state glyphs and colors for all 7 `TaskState` values
- [ ] Selected row renders with reverse-video highlight
- [ ] `c` key emits `OpenLiveChatMsg` → `LiveChatView` opens for selected node
- [ ] Loading, error, and empty-tasks states render correctly
- [ ] `go build ./...` passes
- [ ] Unit tests: 9 cases in `runinspect_test.go` all green
- [ ] E2E test: `TestRunsInspect_E2E` passes with 30s timeout
- [ ] VHS tape: `vhs validate tests/vhs/runs-inspect-summary.tape` exits 0
- [ ] `runs-dag-overview`, `runs-node-inspector`, `runs-task-*` tickets can proceed (this view is their parent container)

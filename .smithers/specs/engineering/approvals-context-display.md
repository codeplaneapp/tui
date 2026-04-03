# Engineering Specification: approvals-context-display

**Ticket**: `.smithers/tickets/approvals-context-display.md`
**Feature**: `APPROVALS_CONTEXT_DISPLAY`
**Dependencies**: `approvals-queue`
**Date**: 2026-04-03

---

## Objective

Enrich the approval detail pane so it shows the full task context — run metadata, node position in the workflow, elapsed wait time, and a pretty-printed payload — updating live as the operator moves the cursor through the queue.

The `ApprovalsView` already exists (`internal/ui/views/approvals.go`) with an inline split-pane layout (`renderList` + `renderDetail`) and reads fields from the `Approval` struct (`Gate`, `Status`, `WorkflowPath`, `RunID`, `NodeID`, `Payload`). This ticket improves the detail pane in three ways:

1. **Enriched context**: Fetch run-level metadata (status, elapsed time, node count / step position) from the Smithers API so the detail pane shows more than what the `Approval` record alone carries.
2. **Richer rendering**: Color-coded wait-time SLA badges, word-wrapped context text, and a scrollable payload area that handles large JSON without clipping.
3. **Cursor-driven fetching**: When the cursor moves, fire an async command to fetch enriched context for the newly selected approval. Cache results to avoid redundant API calls during rapid scrolling.

This directly supports PRD §6.5 ("Context display: Show the task that needs approval, its inputs, and the workflow context") and the wireframe in `02-DESIGN.md` §3.5 which shows per-approval context blocks including run info, node step position, and a context summary paragraph.

---

## Scope

### In scope

1. **`RunSummary` type** — Add to `internal/smithers/types.go` for run-level metadata consumed by the detail pane.
2. **`GetRunSummary` client method** — Add to `internal/smithers/client.go` with the standard three-tier transport (HTTP → SQLite → exec).
3. **Client-level caching** — Cache `RunSummary` results keyed by `runID` with a 30-second TTL to avoid per-cursor-move API calls.
4. **Client unit tests** — HTTP, exec, cache, and error paths.
5. **Detail pane upgrade** — Rewrite `renderDetail()` in `internal/ui/views/approvals.go` to consume both `Approval` and `RunSummary`, rendering: SLA-colored wait time, run status and progress, node step position, word-wrapped gate description, and pretty-printed payload.
6. **Cursor-driven async fetch** — On cursor change, emit a `tea.Cmd` that fetches `RunSummary` for the selected approval's `RunID`.
7. **View unit tests** — Enriched detail rendering, cursor-driven updates, loading/error states.
8. **Terminal E2E test** — Verify the enriched detail pane renders and updates on cursor movement.
9. **VHS happy-path recording** — Visual regression test showing the context display.

### Out of scope

- Inline approve/deny actions (`approvals-inline-approve`, `approvals-inline-deny`).
- Shared `SplitPane` component extraction (`eng-split-pane-component`) — the existing hand-rolled split in `approvals.go` is adequate for this feature.
- Notification badges, toast overlays.
- SSE-driven live approval updates (already spec'd in `approvals-queue`).

---

## Implementation Plan

### Slice 1: `RunSummary` type (`internal/smithers/types.go`)

Add a lightweight struct for the run metadata needed by the detail pane. The upstream GUI's `RunsList.tsx` and `NodeInspector.tsx` render equivalent data from the daemon's run routes (`run-routes.ts`).

```go
// RunSummary holds run-level metadata used by the approval detail pane
// and other views that need run context without full node trees.
// Maps to the shape returned by GET /v1/runs/{id} in the Smithers server.
type RunSummary struct {
    ID           string `json:"id"`
    WorkflowPath string `json:"workflowPath"`
    WorkflowName string `json:"workflowName"` // Derived from path basename
    Status       string `json:"status"`       // "running" | "paused" | "completed" | "failed"
    NodeTotal    int    `json:"nodeTotal"`    // Total nodes in the DAG
    NodesDone    int    `json:"nodesDone"`    // Nodes that have finished
    StartedAtMs  int64  `json:"startedAtMs"`  // Unix ms
    ElapsedMs    int64  `json:"elapsedMs"`    // Duration since start
}
```

This is intentionally small — it carries just enough data for the context pane header. A full `Run` type (with node trees, events, etc.) will come with the runs dashboard ticket.

### Slice 2: `GetRunSummary` client method (`internal/smithers/client.go`)

Follow the three-tier transport pattern established by `ListPendingApprovals`, `GetScores`, etc.

```go
// GetRunSummary returns lightweight metadata for a single run.
// Routes: HTTP GET /v1/runs/{runID} → SQLite → exec smithers inspect.
func (c *Client) GetRunSummary(ctx context.Context, runID string) (*RunSummary, error) {
    // Check cache first
    if cached, ok := c.getRunSummaryCache(runID); ok {
        return cached, nil
    }

    var summary *RunSummary

    // 1. Try HTTP
    if c.isServerAvailable() {
        var s RunSummary
        err := c.httpGetJSON(ctx, "/v1/runs/"+runID, &s)
        if err == nil {
            summary = &s
        }
    }

    // 2. Try SQLite
    if summary == nil && c.db != nil {
        s, err := c.getRunSummaryDB(ctx, runID)
        if err == nil {
            summary = s
        }
    }

    // 3. Fall back to exec
    if summary == nil {
        s, err := c.getRunSummaryExec(ctx, runID)
        if err != nil {
            return nil, err
        }
        summary = s
    }

    c.setRunSummaryCache(runID, summary)
    return summary, nil
}
```

**SQLite helper** — `getRunSummaryDB`:
```sql
SELECT r.id, r.workflow_path, r.status, r.started_at,
       (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id) AS node_total,
       (SELECT COUNT(*) FROM _smithers_nodes n WHERE n.run_id = r.id
        AND n.status IN ('completed', 'failed')) AS nodes_done
FROM _smithers_runs r WHERE r.id = ?
```

Derive `WorkflowName` from `WorkflowPath` using `path.Base()`. Compute `ElapsedMs` as `time.Now().UnixMilli() - StartedAtMs`.

**Exec helper** — `getRunSummaryExec`: Run `smithers inspect <runID> --format json`, parse the JSON output (which includes the full run state), and extract the summary fields.

**Cache**: Add `runSummaryCache sync.Map` on the `Client` struct. Each entry stores `{summary *RunSummary, fetchedAt time.Time}`. `getRunSummaryCache` returns the cached value if `fetchedAt` is within 30 seconds. This prevents redundant HTTP calls when the user scrolls back and forth through approvals referencing the same run.

### Slice 3: Client unit tests (`internal/smithers/client_test.go`)

Add tests following the existing patterns (`TestExecuteSQL_HTTP`, `TestListPendingApprovals_HTTP`, etc.):

```go
func TestGetRunSummary_HTTP(t *testing.T)
    // httptest.Server returns RunSummary JSON for run "run-abc"
    // Verify GET /v1/runs/run-abc called
    // Assert fields mapped correctly: WorkflowName derived from WorkflowPath

func TestGetRunSummary_Exec(t *testing.T)
    // withExecFunc mock returns smithers inspect JSON
    // Assert args == ["inspect", "run-abc", "--format", "json"]
    // Assert RunSummary parsed from nested output

func TestGetRunSummary_NotFound(t *testing.T)
    // HTTP returns 404 → exec returns error
    // Assert error returned

func TestGetRunSummary_CacheHit(t *testing.T)
    // Fetch "run-abc" twice within 30s
    // Assert HTTP called only once (count requests on mock server)

func TestGetRunSummary_CacheExpiry(t *testing.T)
    // Fetch, advance clock past 30s, fetch again
    // Assert HTTP called twice
```

### Slice 4: New message types and async fetch wiring (`internal/ui/views/approvals.go`)

Add message types for the enriched context flow:

```go
type runSummaryLoadedMsg struct {
    runID   string
    summary *smithers.RunSummary
}

type runSummaryErrorMsg struct {
    runID string
    err   error
}
```

Add fields to `ApprovalsView`:

```go
type ApprovalsView struct {
    client       *smithers.Client
    approvals    []smithers.Approval
    cursor       int
    width        int
    height       int
    loading      bool
    err          error

    // Enriched context for the selected approval
    selectedRun  *smithers.RunSummary // nil until fetched
    contextLoading bool               // true while fetching RunSummary
    contextErr   error                // non-nil if fetch failed
    lastFetchRun string               // RunID of the last fetch (dedup)
}
```

**Cursor change triggers fetch**: Modify the `Update` handler for up/down/j/k keys. After moving the cursor, if the new approval's `RunID` differs from `lastFetchRun`, return a `tea.Cmd` that calls `GetRunSummary`:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
    if v.cursor < len(v.approvals)-1 {
        v.cursor++
        return v, v.fetchRunContext()
    }
```

```go
func (v *ApprovalsView) fetchRunContext() tea.Cmd {
    a := v.approvals[v.cursor]
    if a.RunID == v.lastFetchRun {
        return nil // Already fetched
    }
    v.contextLoading = true
    v.contextErr = nil
    v.lastFetchRun = a.RunID
    runID := a.RunID
    return func() tea.Msg {
        summary, err := v.client.GetRunSummary(context.Background(), runID)
        if err != nil {
            return runSummaryErrorMsg{runID: runID, err: err}
        }
        return runSummaryLoadedMsg{runID: runID, summary: summary}
    }
}
```

Handle the result messages in `Update`:

```go
case runSummaryLoadedMsg:
    if msg.runID == v.lastFetchRun {
        v.selectedRun = msg.summary
        v.contextLoading = false
    }
    return v, nil

case runSummaryErrorMsg:
    if msg.runID == v.lastFetchRun {
        v.contextErr = msg.err
        v.contextLoading = false
    }
    return v, nil
```

**Initial load**: After `approvalsLoadedMsg` arrives, if there are approvals, trigger `fetchRunContext()` for the first item:

```go
case approvalsLoadedMsg:
    v.approvals = msg.approvals
    v.loading = false
    if len(v.approvals) > 0 {
        return v, v.fetchRunContext()
    }
    return v, nil
```

### Slice 5: Enriched detail pane rendering (`internal/ui/views/approvals.go`)

Rewrite `renderDetail()` to consume both `v.approvals[v.cursor]` (the `Approval` record) and `v.selectedRun` (the fetched `RunSummary`). The detail pane matches the wireframe in `02-DESIGN.md` §3.5.

**Layout** (right pane, variable width):

```
Deploy to staging
Status: ● pending · ⏱ 8m 23s

Run: def456 (deploy-staging)
Step 4 of 6 · running · started 10m ago

Workflow: .smithers/workflows/deploy.ts
Node:     deploy

Context:
  The deploy workflow has completed build, test,
  and lint steps. All passed. Ready to deploy
  commit a1b2c3d to staging environment.

Payload:
  {
    "commit": "a1b2c3d",
    "environment": "staging",
    "changes": {
      "files": 3,
      "insertions": 47,
      "deletions": 12
    }
  }
```

**Sections**:

1. **Gate header**: Bold `a.Gate` (falls back to `a.NodeID` if empty). Below: status badge + wait time. Wait time computed as `time.Since(time.UnixMilli(a.RequestedAt))`, formatted as `Xm Ys`. SLA color: green <10m, yellow 10-30m, red ≥30m (matching upstream `approval-ui.ts` thresholds from `gui-ref/apps/web/src/features/approvals/lib/approval-ui.ts`).

2. **Run context** (only rendered if `v.selectedRun != nil`): Run ID + workflow name, step progress (`Step {NodesDone} of {NodeTotal}`), run status, started-at relative time. If `v.contextLoading`, show `Loading run details...` in faint text. If `v.contextErr != nil`, show `Could not load run details` in faint red.

3. **Static metadata**: Workflow path from `a.WorkflowPath`, Node ID from `a.NodeID`. Always available from the `Approval` record.

4. **Payload**: If `a.Payload != ""`, attempt JSON pretty-print via the existing `formatPayload()` helper. Word-wrap non-JSON text via `wrapText()`. Limit rendered lines to `(v.height - headerLines)` and append `... (N more lines)` if truncated.

5. **Resolution info** (only for decided approvals): If `a.ResolvedAt != nil`, show `Resolved by: {a.ResolvedBy}` and `Resolved at: {relative time}`.

**State handling**:
- **No selection** (empty list): `Select an approval to view details.`
- **Loading context**: Gate/metadata from `Approval` render immediately; run context section shows `Loading run details...`
- **Error**: Gate/metadata render; run context section shows `Could not load run details: {err}`
- **No payload**: Omit the Payload section entirely (don't show an empty header).

### Slice 6: Improve list pane with wait-time indicators (`internal/ui/views/approvals.go`)

Update `renderListItem()` to show the wait time next to each pending approval, using the same SLA color scheme:

```go
func (v *ApprovalsView) renderListItem(idx, width int) string {
    a := v.approvals[idx]
    // ... existing cursor/icon logic ...

    // Add wait time for pending items
    var waitStr string
    if a.Status == "pending" {
        wait := time.Since(time.UnixMilli(a.RequestedAt))
        waitStr = formatWait(wait)
        waitStr = slaStyle(wait).Render(waitStr)
    }

    // Lay out: cursor + icon + label + right-aligned wait
    // ...
}
```

Helper functions:

```go
// formatWait formats a duration as "Xm" or "Xh Ym".
func formatWait(d time.Duration) string {
    if d < time.Minute {
        return "<1m"
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm", int(d.Minutes()))
    }
    return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// slaStyle returns a lipgloss.Style with SLA-appropriate color.
// Green <10m, yellow 10-30m, red ≥30m.
func slaStyle(d time.Duration) lipgloss.Style {
    switch {
    case d < 10*time.Minute:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
    case d < 30*time.Minute:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
    default:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
    }
}
```

### Slice 7: Improve compact mode (`internal/ui/views/approvals.go`)

Update `renderListCompact()` to include wait time and the same enriched context from `v.selectedRun` when the cursor is on an item. In compact mode (<80 cols), context is shown inline below the selected item instead of in a side pane:

```go
if i == v.cursor {
    faint := lipgloss.NewStyle().Faint(true)
    b.WriteString(faint.Render("    Workflow: "+a.WorkflowPath) + "\n")
    b.WriteString(faint.Render("    Run: "+a.RunID) + "\n")

    // Show run context if available
    if v.selectedRun != nil && v.selectedRun.ID == a.RunID {
        b.WriteString(faint.Render(fmt.Sprintf("    Step %d of %d · %s",
            v.selectedRun.NodesDone, v.selectedRun.NodeTotal,
            v.selectedRun.Status)) + "\n")
    }

    if a.Payload != "" {
        b.WriteString(faint.Render("    "+truncate(a.Payload, 60)) + "\n")
    }
}
```

### Slice 8: View unit tests (`internal/ui/views/approvals_test.go`)

```go
func TestApprovalsView_DetailShowsRunContext(t *testing.T)
    // Load approvals, send runSummaryLoadedMsg with step 4/6
    // Assert View() output contains "Step 4 of 6"
    // Assert View() output contains workflow name

func TestApprovalsView_DetailWaitTimeSLA(t *testing.T)
    // Create approval with RequestedAt = 5 min ago
    // Assert wait time rendered in green (ANSI code 32 or color "2")
    // Create approval with RequestedAt = 45 min ago
    // Assert wait time rendered in red

func TestApprovalsView_CursorChangeTriggersContextFetch(t *testing.T)
    // Load 2 approvals with different RunIDs
    // Send "j" key → assert tea.Cmd returned (non-nil)
    // Verify lastFetchRun updated to second approval's RunID

func TestApprovalsView_SameRunIDSkipsFetch(t *testing.T)
    // Load 2 approvals with SAME RunID
    // Send "j" key → assert no tea.Cmd returned (nil, already fetched)

func TestApprovalsView_DetailLoadingState(t *testing.T)
    // Move cursor (triggers fetch), don't send result yet
    // Assert View() output contains "Loading run details..."

func TestApprovalsView_DetailErrorState(t *testing.T)
    // Send runSummaryErrorMsg
    // Assert View() output contains "Could not load run details"

func TestApprovalsView_DetailNoPayload(t *testing.T)
    // Load approval with empty Payload
    // Assert View() does NOT contain "Payload:"

func TestApprovalsView_ResolvedApprovalShowsDecision(t *testing.T)
    // Load approval with ResolvedAt and ResolvedBy set
    // Assert View() contains "Resolved by:"

func TestApprovalsView_ListItemShowsWaitTime(t *testing.T)
    // Load pending approval with RequestedAt = 12 min ago
    // Assert list item in View() contains "12m"

func TestApprovalsView_CompactModeShowsRunContext(t *testing.T)
    // Set width=60, load approvals + runSummaryLoadedMsg
    // Assert View() contains "Step N of M" inline under selected item
    // Assert no split-pane divider "│" present

func TestApprovalsView_InitialLoadTriggersContextFetch(t *testing.T)
    // Send approvalsLoadedMsg with 1 approval
    // Assert returned cmd is non-nil (fetches context for first item)

func TestApprovalsView_SplitPaneDividerPresent(t *testing.T)
    // Set width=120, load approvals
    // Assert View() contains " │ " divider
```

### Slice 9: Terminal E2E test

**File**: `internal/e2e/approvals_context_display_test.go`

Modeled on the existing Go E2E harness in `internal/e2e/tui_helpers_test.go` and the upstream TypeScript patterns in `smithers_tmp/tests/tui-helpers.ts` + `smithers_tmp/tests/tui.e2e.test.ts`.

```go
func TestApprovalsContextDisplayE2E(t *testing.T) {
    if os.Getenv("SMITHERS_TUI_E2E") != "1" {
        t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
    }

    // Start mock Smithers HTTP server:
    // - GET /approval/list → 2 pending approvals with Payload JSON
    // - GET /v1/runs/run-abc → RunSummary (deploy-staging, 4/6 nodes, running)
    // - GET /v1/runs/run-xyz → RunSummary (gdpr-cleanup, 3/4 nodes, running)
    mockServer := startMockSmithersContextServer(t)
    defer mockServer.Close()

    configDir := t.TempDir()
    dataDir := t.TempDir()
    writeGlobalConfig(t, configDir, fmt.Sprintf(`{
        "smithers": { "apiUrl": %q }
    }`, mockServer.URL))

    t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", configDir)
    t.Setenv("SMITHERS_TUI_GLOBAL_DATA", dataDir)

    tui := launchTUI(t)
    defer tui.Terminate()

    // Wait for TUI to start
    require.NoError(t, tui.WaitForText("CRUSH", 15*time.Second))

    // Navigate to approvals via ctrl+a
    tui.SendKeys("\x01")

    // Verify the split-pane approval list renders
    require.NoError(t, tui.WaitForText("Pending", 5*time.Second),
        "should show pending section; buffer: %s", tui.Snapshot())

    // Verify context pane shows enriched details for first approval
    require.NoError(t, tui.WaitForText("deploy-staging", 5*time.Second),
        "should show workflow name in detail pane; buffer: %s", tui.Snapshot())
    require.NoError(t, tui.WaitForText("Step 4 of 6", 5*time.Second),
        "should show step progress from RunSummary; buffer: %s", tui.Snapshot())

    // Move cursor down to second approval
    tui.SendKeys("j")

    // Verify context updates to second approval's run
    require.NoError(t, tui.WaitForText("gdpr-cleanup", 5*time.Second),
        "should show second workflow name; buffer: %s", tui.Snapshot())
    require.NoError(t, tui.WaitForText("Step 3 of 4", 5*time.Second),
        "should show second run's step progress; buffer: %s", tui.Snapshot())

    // Move back up
    tui.SendKeys("k")
    require.NoError(t, tui.WaitForText("deploy-staging", 5*time.Second),
        "should return to first approval context; buffer: %s", tui.Snapshot())

    // Return to chat
    tui.SendKeys("\x1b") // esc
    require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second),
        "esc should return to chat; buffer: %s", tui.Snapshot())
}
```

**Mock server helper** (`startMockSmithersContextServer`): Uses `net/http/httptest` to serve:
- `GET /approval/list` → JSON array with 2 `Approval` records
- `GET /v1/runs/run-abc` → `RunSummary{ID: "run-abc", WorkflowName: "deploy-staging", NodeTotal: 6, NodesDone: 4, Status: "running"}`
- `GET /v1/runs/run-xyz` → `RunSummary{ID: "run-xyz", WorkflowName: "gdpr-cleanup", NodeTotal: 4, NodesDone: 3, Status: "running"}`

This test follows:
- **`internal/e2e/tui_helpers_test.go`**: `launchTUI()` process spawning with `go run .`, `WaitForText()` polling at 100ms intervals, `SendKeys()` for stdin, `Snapshot()` ANSI-stripped buffer dump on failure, `Terminate()` with SIGINT + 2s kill timeout. Same env var isolation as `chat_domain_system_prompt_test.go`.
- **`smithers_tmp/tests/tui.e2e.test.ts`**: Multi-level navigation (enter view → verify content → navigate within view → verify update → Esc back), assertion messages include buffer snapshot, 15s initial timeout then 5s for assertions.

### Slice 10: VHS happy-path recording

**File**: `tests/vhs/approvals-context-display.tape`

```tape
# Approvals context display — enriched detail pane with cursor-driven updates
Output tests/vhs/output/approvals-context-display.gif
Set FontSize 14
Set Width 120
Set Height 35
Set Shell zsh

# Start TUI with mock server
Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Navigate to approvals
Ctrl+a
Sleep 2s

# Capture the split-pane with enriched context for first approval
Screenshot tests/vhs/output/approvals-context-first.png

# Navigate to second approval
Down
Sleep 1s

# Capture context update for second approval
Screenshot tests/vhs/output/approvals-context-second.png

# Navigate back to first
Up
Sleep 500ms

# Refresh the list
Type "r"
Sleep 1s

# Return to chat
Escape
Sleep 1s

Screenshot tests/vhs/output/approvals-context-back.png
```

The VHS tape validates:
- Split-pane renders at 120 columns with enriched context.
- Context pane updates on cursor movement (visible in GIF).
- No crashes during navigation or refresh.
- Exits non-zero if the TUI crashes at any point.
- Produces `approvals-context-display.gif` + PNG screenshots for review.

---

## Validation

### Automated checks

| Check | Command | What it proves |
|-------|---------|----------------|
| `RunSummary` type compiles | `go build ./internal/smithers/...` | New type and fetch/cache helpers are valid Go |
| Client unit tests pass | `go test ./internal/smithers/ -run TestGetRunSummary -v` | HTTP, exec, cache, cache-expiry, and 404 paths work correctly |
| View unit tests pass | `go test ./internal/ui/views/ -run TestApprovalsView -v` | Enriched detail, SLA colors, cursor-driven fetch, loading/error states, compact mode, split pane divider |
| Full build succeeds | `go build ./...` | No import cycles, all new code integrates cleanly |
| Existing tests pass | `go test ./...` | No regressions in approval list, chat, router, agents, tickets |
| Terminal E2E: context display | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsContextDisplayE2E -timeout 30s -v` | `Ctrl+A` shows enriched detail, cursor movement updates context, `Esc` returns to chat |
| VHS recording | `vhs tests/vhs/approvals-context-display.tape` (exit code 0) | Happy-path flow completes visually; GIF + screenshots produced |

### Terminal E2E coverage (modeled on upstream harness)

The E2E test in Slice 9 directly models these upstream patterns:

- **`internal/e2e/tui_helpers_test.go`** (Go harness, 178 lines): `launchTUI(t)` spawns `go run .` with `TERM=xterm-256color`, `COLORTERM=truecolor`, `LANG=en_US.UTF-8`. `WaitForText()` polls at `100ms` intervals using `ansiPattern.ReplaceAllString()` for ANSI stripping and `normalizeTerminalText()` for box-drawing character handling. `SendKeys()` writes to stdin via `io.WriteString`. `Snapshot()` returns ANSI-stripped buffer. `Terminate()` sends `os.Interrupt` with a 2-second kill timeout.

- **`smithers_tmp/tests/tui-helpers.ts`** (upstream TypeScript harness): Same `waitForText` / `sendKeys` / `snapshot` API surface. ANSI stripping via regex. Space normalization. Default 10s timeout, 100ms poll.

- **`smithers_tmp/tests/tui.e2e.test.ts`** (upstream test pattern): Multi-level view navigation (navigate into view → verify rendered content → drill into subview / move cursor → verify update → Esc back). Assertion messages include buffer snapshot for CI debugging. 15s initial timeout, 5s for subsequent assertions.

The Go test preserves: `SMITHERS_TUI_E2E=1` opt-in gate, per-test mock servers for isolation, `require.NoError` with snapshot context in failure messages, `defer tui.Terminate()` for cleanup.

### VHS recording test

The VHS tape in Slice 10 provides a visual regression test:
- Launches the real TUI binary at 120×35 (wide enough for the split-pane).
- Navigates to the approvals view via `Ctrl+A`.
- Captures enriched context for the first approval (step progress, workflow name, SLA wait time).
- Moves cursor down, captures updated context for the second approval.
- Returns to chat via `Escape`.
- Produces `approvals-context-display.gif` + PNG screenshots for visual review.
- Exits non-zero if the TUI crashes at any point.

### Manual verification

1. **Build**: `go build -o smithers-tui . && ./smithers-tui`
2. **With live server**: Start `smithers up --serve`, create a workflow with `<ApprovalGate>`, run until it pauses. Press `Ctrl+A`. Verify the detail pane shows gate name, workflow path, run ID, step progress (`Step N of M`), run status, wait time with SLA color.
3. **Cursor movement**: Press `↓`/`j` to move through list. Verify right pane updates to show context for the newly selected approval. Brief "Loading run details..." may flash during fetch.
4. **Payload rendering**: Verify JSON payloads are pretty-printed with indentation. Verify non-JSON payloads display as word-wrapped text.
5. **Wait time colors**: Verify <10m → green, 10-30m → yellow, ≥30m → red in both list pane and detail pane.
6. **Resolved approvals**: Navigate to a recently decided approval. Verify detail pane shows "Resolved by:" and relative timestamp.
7. **Wide terminal** (120+ cols): Verify split pane with `│` divider, list on left, detail on right.
8. **Narrow terminal** (< 80 cols): Verify compact mode — inline context below selected item, no divider.
9. **Resize**: Resize terminal while view is open. Verify no crash, layout switches between split/compact correctly at the 80-column breakpoint.
10. **Without server**: Press `Ctrl+A` with no Smithers server. Verify list shows error or empty state gracefully; detail pane shows "Could not load run details" if fetch fails.
11. **Return to chat**: Press `Esc` — verify return to chat with all state intact.

---

## Risks

### 1. No run endpoint on the Smithers server

**Risk**: `GetRunSummary` depends on `GET /v1/runs/{runID}` existing on the Smithers HTTP server. The upstream GUI used the Burns daemon's `GET /api/workspaces/{wid}/runs/{rid}` which is a different path. The direct Smithers server may not have a per-run GET endpoint — it may only expose `GET /ps` for listing all runs.

**Mitigation**: The three-tier transport provides fallbacks. If no per-run HTTP endpoint exists: (1) the SQLite path queries `_smithers_runs` directly — this table exists in all Smithers projects; (2) the exec path uses `smithers inspect <runID> --format json` which is a standard CLI command. At implementation time, probe the Smithers server routes in `../smithers/src/server/index.ts` to determine the correct HTTP path. If the endpoint exists but at a different path (e.g., `/ps/{runID}`), adjust `GetRunSummary` accordingly.

### 2. `_smithers_nodes` table may lack step counts

**Risk**: The SQLite query in `getRunSummaryDB` counts nodes via `SELECT COUNT(*) FROM _smithers_nodes WHERE run_id = ?`. If the Smithers DB doesn't create node rows until they start executing (lazy initialization), `NodeTotal` and `NodesDone` will be inaccurate for runs that haven't reached all nodes yet.

**Mitigation**: The step progress display (`Step N of M`) is supplementary information — the detail pane still renders the gate, status, workflow, payload, and wait time without it. If node counts are unavailable or clearly wrong (e.g., `NodeTotal == 0`), skip the step progress line entirely. The exec fallback (`smithers inspect`) returns the full DAG including unstarted nodes, providing accurate counts.

### 3. Cache invalidation on approve/deny

**Risk**: When `approvals-inline-approve` or `approvals-inline-deny` are implemented (future tickets), approving a gate changes the run status. The cached `RunSummary` will be stale — it may still show "running" when the run has moved to the next node.

**Mitigation**: The cache has a 30-second TTL, which naturally expires. Additionally, when approve/deny tickets are implemented, they should invalidate the cache for the affected `runID` by calling a `ClearRunSummaryCache(runID)` method (which this ticket should expose on the client). The approval list also refreshes after mutations, which triggers a new `fetchRunContext()`.

### 4. Large JSON payloads overflow the detail pane

**Risk**: Some approval gates may carry large input payloads (hundreds of lines of JSON). The current `formatPayload()` renders all of it, which can overflow the visible terminal height and push the gate/run context off-screen.

**Mitigation**: Slice 5 adds height-aware truncation: count the lines used by header/metadata sections, compute remaining lines for payload, and truncate with `... (N more lines)`. The existing `wrapText()` helper already handles line-level wrapping; the new code adds a max-lines cap. A future ticket could add a scrollable viewport within the detail pane using `bubbles/viewport`.

### 5. Crush-Smithers field name mismatch

**Impact**: The `Approval` type in Crush uses `Gate` (not `Label`), `Payload` (not `InputJSON`), `RequestedAt` (not `WaitMinutes`), `ResolvedBy`/`ResolvedAt` (not `DecidedBy`/`DecidedAt`). The upstream GUI reference uses the older field names. All code in this spec uses the Crush field names from the actual `internal/smithers/types.go` (lines 83-96).

**Consequence**: Any upstream Smithers API changes that rename these fields will require updates to the `Approval` struct and the `scanApprovals` SQL column list. The JSON tags on the struct serve as the contract — if the wire format changes, only `types.go` and the scan helper need updating.

### 6. Concurrent approval list refresh and context fetch

**Risk**: If the approval list refreshes (via `r` key or SSE) while a `GetRunSummary` fetch is in-flight, the `runSummaryLoadedMsg` may arrive for an approval that's no longer at the cursor position or no longer in the list.

**Mitigation**: The `runSummaryLoadedMsg` carries `runID`. The `Update` handler checks `msg.runID == v.lastFetchRun` before applying the result — if the cursor has moved or the list has refreshed, stale results are silently discarded. After a list refresh (`approvalsLoadedMsg`), `fetchRunContext()` is called again for the current cursor position, which resets `lastFetchRun`.

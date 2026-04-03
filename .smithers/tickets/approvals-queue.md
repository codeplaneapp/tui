# Build the pending approvals queue

## Metadata
- ID: approvals-queue
- Group: Approvals And Notifications (approvals-and-notifications)
- Type: feature
- Feature: APPROVALS_QUEUE
- Dependencies: eng-approvals-view-scaffolding

## Summary

Fetch and display a list of all pending approval gates from the Smithers API/DB.

## Acceptance Criteria

- The view shows a selectable list/table of pending approvals.
- List dynamically updates if new approvals arrive via SSE.

## Source Context

- internal/ui/views/approvals.go
- internal/smithers/client.go

## Implementation Notes

- Use Bubble Tea's `list` or `table` component. Fetch data via the Smithers API client.

---

## Objective

Wire the empty `ApprovalsView` (created by `eng-approvals-view-scaffolding`) to real data from the Smithers API so that operators can see every pending approval gate in a single, navigable list that updates in real-time. After this ticket, pressing `ctrl+a` shows a live queue of pending approvals with labels, run IDs, node IDs, wait durations, and cursor-based selection — the foundation that `approvals-context-display`, `approvals-inline-approve`, and `approvals-inline-deny` build on.

This ticket adds:
1. The `Approval` Go type in `internal/smithers/types.go`.
2. `ListPendingApprovals()` and supporting transport methods on the Smithers client.
3. A selectable approval list rendered in the `ApprovalsView`.
4. SSE-driven live updates so new approval gates appear without manual refresh.

## Scope

### In scope

1. **`internal/smithers/types.go`** — Add the `Approval` struct matching the upstream schema from `smithers_tmp/gui-ref/packages/shared/src/schemas/approval.ts`.
2. **`internal/smithers/client.go`** — Add `ListPendingApprovals(ctx) ([]Approval, error)` with the standard three-tier transport: HTTP GET → SQLite SELECT → exec fallback.
3. **`internal/smithers/client_test.go`** — Unit tests for `ListPendingApprovals` covering HTTP, exec, and error paths.
4. **`internal/ui/views/approvals.go`** — Extend the skeleton `ApprovalsView` to:
   - Accept a `*smithers.Client` dependency.
   - Fetch approvals on `Init()`.
   - Render a cursor-navigable list of pending approvals with label, run ID, node, and wait time.
   - Show a "RECENT DECISIONS" section below pending items (approved/denied approvals).
   - Handle `tea.WindowSizeMsg` for responsive layout.
   - Handle `r` key to manually refresh.
5. **SSE integration** — Subscribe to the Smithers event stream and push new `ApprovalRequested` events into the view as `tea.Msg` values, causing the list to update without polling.
6. **`internal/ui/views/approvals_test.go`** — Unit tests for list rendering, cursor navigation, and message handling.
7. **Terminal E2E test** — Verify the queue renders with mock data and supports cursor navigation.
8. **VHS happy-path recording** — Visual test showing the populated approval queue.

### Out of scope

- Inline approve/deny actions (ticket `approvals-inline-approve`, `approvals-inline-deny`).
- Approval context/detail pane (ticket `approvals-context-display`).
- Notification badges on other views (ticket `approvals-pending-badges`).
- Toast notifications for new approvals (ticket `notifications-approval-requests`).
- The `approvalcard.go` reusable component (`internal/ui/components/approvalcard.go`) — the list in this ticket uses simple row rendering. The card component is introduced when `approvals-context-display` needs a richer detail panel.

## Implementation Plan

### Slice 1: Approval type (`internal/smithers/types.go`)

Add the `Approval` struct to the existing types file. The canonical upstream shape is defined in `smithers_tmp/gui-ref/packages/shared/src/schemas/approval.ts`.

```go
// Approval represents a pending or decided approval gate.
// Maps to Approval in smithers/gui-ref/packages/shared/src/schemas/approval.ts
// and the row shape in approval-repository.ts.
type Approval struct {
    ID          string  `json:"id"`
    RunID       string  `json:"runId"`
    NodeID      string  `json:"nodeId"`
    Label       string  `json:"label"`
    Status      string  `json:"status"`      // "pending" | "approved" | "denied"
    WaitMinutes int     `json:"waitMinutes"`
    Note        *string `json:"note,omitempty"`
    DecidedBy   *string `json:"decidedBy,omitempty"`
    DecidedAt   *string `json:"decidedAt,omitempty"`
}
```

This matches the upstream schema field-for-field. `Status` is a string (not a typed enum) to stay aligned with the JSON wire format; the view layer interprets the values.

### Slice 2: Client method — `ListPendingApprovals` (`internal/smithers/client.go`)

Add `ListPendingApprovals` following the three-tier transport pattern established by `ListCrons`, `ExecuteSQL`, and `GetScores`.

**HTTP path** (primary): `GET /api/workspaces/{workspaceId}/approvals` as defined in `smithers_tmp/gui-ref/apps/daemon/src/server/routes/approval-routes.ts`. For the TUI, the workspace ID is either configured or defaults to the current project root. The Smithers HTTP server also exposes `GET /v1/approvals` as a convenience endpoint that returns approvals across all workspaces.

```go
// ListPendingApprovals returns all approval gates, optionally filtered to pending only.
// Routes: HTTP GET /v1/approvals → SQLite → exec smithers approve --list.
func (c *Client) ListPendingApprovals(ctx context.Context) ([]Approval, error) {
    // 1. Try HTTP
    if c.isServerAvailable() {
        var approvals []Approval
        err := c.httpGetJSON(ctx, "/v1/approvals", &approvals)
        if err == nil {
            return approvals, nil
        }
    }

    // 2. Try direct SQLite
    if c.db != nil {
        rows, err := c.queryDB(ctx,
            `SELECT id, run_id, node_id, label, status, wait_minutes,
                note, decided_by, decided_at
            FROM _smithers_approvals
            ORDER BY
                CASE WHEN status = 'pending' THEN 0 ELSE 1 END,
                wait_minutes DESC`)
        if err != nil {
            return nil, err
        }
        return scanApprovals(rows)
    }

    // 3. Fall back to exec
    out, err := c.execSmithers(ctx, "approve", "--list", "--format", "json")
    if err != nil {
        return nil, err
    }
    return parseApprovalsJSON(out)
}
```

Add corresponding scan/parse helpers:

```go
func scanApprovals(rows *sql.Rows) ([]Approval, error) {
    defer rows.Close()
    var result []Approval
    for rows.Next() {
        var a Approval
        if err := rows.Scan(
            &a.ID, &a.RunID, &a.NodeID, &a.Label, &a.Status,
            &a.WaitMinutes, &a.Note, &a.DecidedBy, &a.DecidedAt,
        ); err != nil {
            return nil, err
        }
        result = append(result, a)
    }
    return result, rows.Err()
}

func parseApprovalsJSON(data []byte) ([]Approval, error) {
    var approvals []Approval
    if err := json.Unmarshal(data, &approvals); err != nil {
        return nil, fmt.Errorf("parse approvals: %w", err)
    }
    return approvals, nil
}
```

**Transport priority rationale**: The HTTP path is preferred because the Smithers server maintains the approval lifecycle (sync from events, track decisions). SQLite provides read-only access to the local `_smithers_approvals` table for offline inspection. The exec fallback uses `smithers approve --list` which queries the same DB through the CLI.

### Slice 3: Client unit tests (`internal/smithers/client_test.go`)

Add tests following the patterns established by `TestExecuteSQL_HTTP` and `TestExecuteSQL_Exec`.

```go
func TestListPendingApprovals_HTTP(t *testing.T) {
    // Set up httptest.Server returning two approvals (one pending, one approved)
    // Verify correct path (/v1/approvals), method (GET)
    // Assert returned slice length, field mapping, ordering
}

func TestListPendingApprovals_Exec(t *testing.T) {
    // Use newExecClient with mock returning JSON array
    // Assert args == ["approve", "--list", "--format", "json"]
    // Assert correct deserialization
}

func TestListPendingApprovals_EmptyList(t *testing.T) {
    // HTTP returns empty array
    // Assert non-nil empty slice (not nil)
}

func TestListPendingApprovals_ServerError(t *testing.T) {
    // HTTP returns ok:false with error message
    // Assert error returned, not nil approvals
}
```

### Slice 4: ApprovalsView data integration (`internal/ui/views/approvals.go`)

Extend the skeleton view created by `eng-approvals-view-scaffolding`. The existing `AgentsView` in `internal/ui/views/agents.go` provides the exact pattern to follow: accept a `*smithers.Client`, define loaded/error message types, fetch on `Init()`, render a cursor-navigable list.

**Message types**:

```go
type approvalsLoadedMsg struct {
    approvals []smithers.Approval
}

type approvalsErrorMsg struct {
    err error
}

type approvalsRefreshMsg struct{} // Triggered by SSE or manual refresh
```

**View struct changes** (extending the skeleton from `eng-approvals-view-scaffolding`):

```go
type ApprovalsView struct {
    client    *smithers.Client
    approvals []smithers.Approval  // all approvals (pending + recent)
    cursor    int
    width     int
    height    int
    loading   bool
    err       error
}

func NewApprovals(client *smithers.Client) *ApprovalsView {
    return &ApprovalsView{
        client:  client,
        loading: true,
    }
}
```

**Init**: Fetch approvals asynchronously, same pattern as `AgentsView.Init()`.

```go
func (v *ApprovalsView) Init() tea.Cmd {
    return func() tea.Msg {
        approvals, err := v.client.ListPendingApprovals(context.Background())
        if err != nil {
            return approvalsErrorMsg{err: err}
        }
        return approvalsLoadedMsg{approvals: approvals}
    }
}
```

**Update**: Handle loaded/error messages, cursor navigation (up/down/j/k), refresh (`r`), and esc (pop via `PopViewMsg`).

**View rendering**: Two sections, matching the wireframe in `02-DESIGN.md` §3.5:

1. **Pending approvals** — Filtered to `status == "pending"`. Each row shows:
   - Cursor indicator (`▸` or space)
   - Approval label (bold when selected)
   - Wait duration (formatted as "Xm ago" or "Xh ago"), color-coded: green <10m, yellow 10-30m, red ≥30m (matching upstream `approval-ui.ts` SLA thresholds)
   - Run ID and node ID in faint text
   - Section header: `⚠ N PENDING APPROVALS`

2. **Recent decisions** — Filtered to `status != "pending"`. Each row shows:
   - Status icon: `✓` for approved, `✗` for denied
   - Label, run ID, relative time
   - Section header: `RECENT DECISIONS`
   - Limit to last 10 decisions to keep the view compact

**Empty state**: When no pending approvals exist, show `No pending approvals.` (matching the scaffolding placeholder). Recent decisions are still shown if they exist.

**Layout**: The header follows the established pattern: `SMITHERS › Approvals` with `[Esc] Back` right-aligned. Help bar at the bottom: `[↑/↓] Navigate  [r] Refresh  [Esc] Back`.

### Slice 5: SSE live updates

The Smithers server emits SSE events when approval gates are created or resolved. The engineering doc (`03-ENGINEERING.md` §4.3) specifies that `StreamEvents` emits typed events including `ApprovalRequested`.

**Approach**: When the `ApprovalsView` initializes, it starts a background goroutine (via `tea.Cmd`) that listens on the SSE stream. When an approval-related event arrives (`ApprovalRequested`, `approval.approved`, `approval.denied`), it emits an `approvalsRefreshMsg`, causing the view to re-fetch the full approval list.

```go
func (v *ApprovalsView) subscribeToEvents() tea.Cmd {
    return func() tea.Msg {
        ctx := context.Background()
        events, err := v.client.StreamEvents(ctx, 0)
        if err != nil {
            // SSE not available — fall back to no live updates
            return nil
        }
        for evt := range events {
            if evt.Type == "ApprovalRequested" ||
                evt.Type == "approval.approved" ||
                evt.Type == "approval.denied" {
                return approvalsRefreshMsg{}
            }
        }
        return nil
    }
}
```

**On `approvalsRefreshMsg`**: Re-run the fetch command (`v.Init()`), which will update the list. Also restart the SSE subscription (since the goroutine returned after emitting one message — this is the Bubble Tea command pattern where each event re-subscribes).

**Graceful degradation**: If the SSE endpoint is unavailable (server not running, no HTTP transport), the view still works — it just doesn't auto-update. The `r` key provides manual refresh as a fallback.

**Note on `StreamEvents` availability**: The `StreamEvents` method is specified in `03-ENGINEERING.md` §3.1.3 and §4.3 but is not yet implemented in `internal/smithers/client.go`. If this method does not exist at implementation time, the SSE subscription code should be gated behind a nil check and the view should degrade to manual refresh only. This is not a blocker — the core acceptance criteria ("shows a selectable list/table") is satisfied by the initial fetch.

### Slice 6: View unit tests (`internal/ui/views/approvals_test.go`)

```go
func TestApprovalsView_LoadedRendersListCorrectly(t *testing.T)
    // Send approvalsLoadedMsg with 2 pending + 1 approved
    // Assert View() output contains "2 PENDING APPROVALS"
    // Assert first pending label visible
    // Assert "RECENT DECISIONS" section visible

func TestApprovalsView_CursorNavigation(t *testing.T)
    // Load 3 pending approvals
    // Send down key → cursor moves to 1
    // Send down key → cursor moves to 2
    // Send up key → cursor moves to 1
    // Assert cursor bounds (does not go below 0 or above len-1)

func TestApprovalsView_EmptyState(t *testing.T)
    // Send approvalsLoadedMsg with empty slice
    // Assert View() contains "No pending approvals"

func TestApprovalsView_ErrorState(t *testing.T)
    // Send approvalsErrorMsg
    // Assert View() contains "Error:"

func TestApprovalsView_RefreshReloads(t *testing.T)
    // Send 'r' key → assert Init() cmd returned (loading restarted)

func TestApprovalsView_WaitTimeColor(t *testing.T)
    // Assert <10 min renders green, 10-30 yellow, ≥30 red
    // Use lipgloss style inspection or string matching on ANSI codes

func TestApprovalsView_PopOnEsc(t *testing.T)
    // Send esc key → assert PopViewMsg emitted
```

### Slice 7: Terminal E2E test

Extend the E2E harness from `eng-approvals-view-scaffolding` (Slice 6 of that spec). The test launches the TUI with a mock Smithers server that returns approval data.

**File**: `tests/tui_approvals_queue_test.go` (or appended to the existing E2E test file with build tag `e2e`)

```go
func TestApprovalsQueueE2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // Start a mock Smithers HTTP server returning 2 pending approvals
    mockServer := startMockSmithersServer(t, mockApprovals{
        {ID: "appr-1", RunID: "run-abc", NodeID: "deploy", Label: "Deploy to staging", Status: "pending", WaitMinutes: 8},
        {ID: "appr-2", RunID: "run-xyz", NodeID: "delete", Label: "Delete user data", Status: "pending", WaitMinutes: 2},
    })
    defer mockServer.Close()

    tui, err := launchTUI("--smithers-api", mockServer.URL)
    require.NoError(t, err)
    defer tui.Terminate()

    // Wait for chat view
    err = tui.WaitForText("Ready", 10*time.Second)
    require.NoError(t, err)

    // Navigate to approvals
    tui.SendKeys("\x01") // ctrl+a

    // Verify pending approvals render
    err = tui.WaitForText("PENDING APPROVALS", 5*time.Second)
    require.NoError(t, err, "should show pending header; buffer: %s", tui.Snapshot())

    err = tui.WaitForText("Deploy to staging", 5*time.Second)
    require.NoError(t, err, "should show first approval label; buffer: %s", tui.Snapshot())

    err = tui.WaitForText("Delete user data", 5*time.Second)
    require.NoError(t, err, "should show second approval label; buffer: %s", tui.Snapshot())

    // Test cursor navigation
    tui.SendKeys("j") // down
    // (visual verification via snapshot — cursor position check)

    // Test refresh
    tui.SendKeys("r")
    err = tui.WaitForText("PENDING APPROVALS", 5*time.Second)
    require.NoError(t, err, "refresh should re-render list; buffer: %s", tui.Snapshot())

    // Return to chat
    tui.SendKeys("\x1b") // esc
    err = tui.WaitForText("Ready", 5*time.Second)
    require.NoError(t, err, "esc should return to chat; buffer: %s", tui.Snapshot())
}
```

This test follows the upstream `tui-helpers.ts` pattern: spawn process, `WaitForText` with timeout, `SendKeys` for input, `Snapshot()` dump on failure.

### Slice 8: VHS happy-path recording

**File**: `tests/vhs/approvals-queue.tape`

```tape
# Approvals queue — happy path with pending approvals
Output tests/vhs/output/approvals-queue.gif
Set FontSize 14
Set Width 120
Set Height 35
Set Shell zsh

# Start mock server and TUI (assumes test fixture helper)
Type "SMITHERS_API_URL=http://localhost:7331 smithers-tui"
Enter
Sleep 3s

# Navigate to approvals
Ctrl+a
Sleep 2s

# Capture the populated queue
Screenshot tests/vhs/output/approvals-queue-populated.png

# Navigate down through the list
Down
Sleep 500ms
Down
Sleep 500ms

# Capture with cursor moved
Screenshot tests/vhs/output/approvals-queue-cursor.png

# Refresh
Type "r"
Sleep 1s

# Return to chat
Escape
Sleep 1s

Screenshot tests/vhs/output/approvals-queue-back-to-chat.png
```

The VHS tape validates the visual flow end-to-end. If the TUI crashes or the view fails to render, VHS exits non-zero.

## Validation

### Automated checks

| Check | Command | What it proves |
|-------|---------|----------------|
| Approval type compiles | `go build ./internal/smithers/...` | `Approval` struct and scan/parse helpers are valid Go |
| Client unit tests pass | `go test ./internal/smithers/ -run TestListPendingApprovals -v` | HTTP, exec, empty, and error paths correctly deserialize approvals |
| View unit tests pass | `go test ./internal/ui/views/ -run TestApprovalsView -v` | List rendering, cursor navigation, empty/error states, refresh, esc behavior |
| Full build succeeds | `go build ./...` | No import cycles, all new code integrates cleanly |
| Existing tests pass | `go test ./...` | No regressions in chat, router, agents, or other views |
| Terminal E2E: queue renders | `go test ./tests/ -run TestApprovalsQueueE2E -timeout 30s` | `ctrl+a` shows populated list with mock data, cursor navigates, `r` refreshes, `esc` returns to chat |
| VHS recording test | `vhs tests/vhs/approvals-queue.tape` (exit code 0) | Happy-path flow completes visually without crash; GIF + screenshots produced |

### Manual verification

1. **Build**: `go build -o smithers-tui . && ./smithers-tui`
2. **Without server**: Press `ctrl+a` — should show "No pending approvals." or an error message (graceful degradation when no Smithers API is available).
3. **With mock server**: Start a Smithers server (`smithers up --serve`), create a workflow with an `<ApprovalGate>` node, run it until it pauses. Press `ctrl+a` — verify the pending approval appears with correct label, run ID, node ID, and wait time.
4. **Cursor navigation**: Use `↑`/`↓`/`j`/`k` to move through the list. Verify the `▸` cursor indicator moves. Verify bounds are respected (no crash at top/bottom).
5. **Wait time colors**: Verify that approvals waiting <10m show green, 10-30m show yellow, ≥30m show red.
6. **Recent decisions**: Approve or deny a gate via CLI, then check that it appears in the "RECENT DECISIONS" section.
7. **Manual refresh**: Press `r` — verify the list re-fetches (loading indicator flashes briefly).
8. **Live updates (SSE)**: While the approvals view is open, trigger a new approval gate from another terminal. Verify the new approval appears in the list without pressing `r`.
9. **Resize**: Resize the terminal while the approvals view is open. Verify no crash, header re-renders correctly.
10. **Return to chat**: Press `esc` — verify return to chat with all state intact.

### Terminal E2E coverage (modeled on upstream harness)

The E2E test in Slice 7 directly models the patterns from:
- **`../smithers/tests/tui-helpers.ts`**: `launchTUI()` process spawning, `waitForText()` polling at 100ms intervals, `sendKeys()` stdin writes, `snapshot()` ANSI-stripped buffer dump on failure.
- **`../smithers/tests/tui.e2e.test.ts`**: assertion structure with `require.NoError` + snapshot context, cleanup via `defer tui.Terminate()`, test isolation via per-test mock servers.

The Go implementation preserves: `TERM=xterm-256color` environment, ANSI stripping for text matching, configurable timeouts, and CI-friendly snapshot dumps in assertion messages.

### VHS recording test

The VHS tape in Slice 8 provides a visual happy-path test that:
- Launches the real TUI binary against a server with approval data.
- Navigates to the approvals view, captures the populated queue.
- Exercises cursor navigation and refresh.
- Returns to chat, captures final state.
- Produces GIF + PNG artifacts for visual inspection.
- Exits non-zero if the TUI crashes at any point.

## Risks

### 1. `StreamEvents` not yet implemented

**Risk**: The SSE subscription (Slice 5) depends on `client.StreamEvents()`, which is specified in `03-ENGINEERING.md` §4.3 but not yet present in `internal/smithers/client.go`. If this method doesn't exist when this ticket is picked up, the SSE path cannot be implemented.

**Mitigation**: Gate the SSE subscription behind a check. If `StreamEvents` is not available, the view works with initial fetch + manual refresh (`r` key). The core acceptance criteria ("shows a selectable list/table") are fully satisfied without SSE. Add a `// TODO: wire SSE when StreamEvents is available` comment. The second acceptance criterion ("dynamically updates if new approvals arrive via SSE") becomes a follow-up if the dependency isn't ready.

### 2. Approval HTTP endpoint mismatch between GUI-ref and Smithers server

**Risk**: The upstream GUI used workspace-scoped endpoints (`GET /api/workspaces/{workspaceId}/approvals`) via the Burns daemon, but the Smithers TUI talks directly to the Smithers server (not Burns). The Smithers server's approval endpoints may differ — they may use `/v1/runs/{runId}/nodes/{nodeId}/approve` for mutations but lack a dedicated "list all approvals" endpoint.

**Mitigation**: The `ListPendingApprovals` implementation provides three fallback tiers. If no `/v1/approvals` HTTP endpoint exists on the Smithers server, the SQLite fallback queries `_smithers_approvals` directly (the table exists in the Smithers DB schema, populated by the approval-gate component). The exec fallback uses `smithers approve --list` which is a standard CLI pattern. At least one tier will work. At implementation time, verify which endpoint the running Smithers server actually exposes by checking `../smithers/src/server/index.ts`.

### 3. Approval table may not exist in Smithers SQLite DB

**Risk**: The `_smithers_approvals` table is used in the Burns daemon's SQLite DB (`approval-repository.ts`), but the core Smithers DB (`_smithers_runs`, `_smithers_nodes`, etc.) may not have a dedicated approvals table. Approval state may instead be derived from node status (`waiting-approval`) and events.

**Mitigation**: If no `_smithers_approvals` table exists, the SQLite fallback should query approval state from the nodes table: `SELECT * FROM _smithers_nodes WHERE status = 'waiting-approval'` and construct `Approval` structs from the node data. Alternatively, skip the SQLite tier entirely and rely on HTTP + exec. The transport tier pattern already handles graceful fallthrough — if `queryDB` returns an error, it falls through to exec.

### 4. No workspace concept in TUI

**Risk**: The upstream GUI is workspace-scoped (each workspace has its own set of approvals), but the TUI operates on a single project directory. If the Smithers API requires a `workspaceId` parameter, the TUI needs to know its workspace ID.

**Mitigation**: The TUI's Smithers client should derive the workspace context from the project directory (the directory where `.smithers/` exists). If the HTTP API requires a workspace ID, use the project directory path or a hash of it as the identifier, matching how Burns generates workspace IDs from file paths. For the exec fallback, `smithers approve --list` runs in the project directory and implicitly scopes to the current workspace.

### 5. Crush-Smithers mismatch: `ApprovalsView` constructor signature change

**Impact**: The `eng-approvals-view-scaffolding` spec creates `NewApprovals()` with no arguments. This ticket changes the signature to `NewApprovals(client *smithers.Client)`. The call site in `ui.go` (where `ctrl+a` pushes the view) must be updated to pass the client.

**Consequence**: This is a trivial change but affects the integration point established by the scaffolding ticket. If the scaffolding is already merged, this ticket modifies the same line: `m.router.Push(views.NewApprovals())` becomes `m.router.Push(views.NewApprovals(m.smithersClient))`. The `smithersClient` field on the `UI` struct must exist — it's specified in `03-ENGINEERING.md` §3.1.3 and may already be present from `eng-smithers-client-runs` or the agents view.

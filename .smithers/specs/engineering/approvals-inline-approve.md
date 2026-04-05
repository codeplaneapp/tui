# Engineering Specification: approvals-inline-approve

**Ticket**: `.smithers/tickets/approvals-inline-approve.md`
**Feature**: `APPROVALS_INLINE_APPROVE`
**Dependencies**: `approvals-context-display`
**Date**: 2026-04-05

---

## Objective

Allow the operator to approve a pending gate directly from the `ApprovalsView` by pressing `a`. The action fires an HTTP/exec API call, shows a loading indicator while inflight, removes the approved item from the pending list on success, and surfaces an inline error with retry capability on failure.

This satisfies PRD §6.5 ("Inline approve/deny: Act on gates without leaving current view") and the wireframe in `02-DESIGN.md §3.5` which shows `[a] Approve` as an inline action on each pending approval card.

---

## Scope

### In scope

1. **`ApproveGate` client method** — `internal/smithers/client.go`: two-tier transport (HTTP POST → exec fallback).
2. **Client unit tests** — HTTP success, exec success, HTTP error fallback to exec, exec error, no-op on wrong approval ID.
3. **`a` key handler** — `ApprovalsView.Update`: guard on `Status == "pending"` and no inflight request.
4. **Inflight state** — `approvingIdx int` (index of item being approved, `-1` when idle) + `spinner.Model` for the animated indicator.
5. **Success handling** — `approveSuccessMsg`: filter approved item from `v.approvals`, clamp cursor, stop spinner, clear error.
6. **Error handling** — `approveErrorMsg`: stop spinner, store `v.approveErr`, render inline in detail pane with retry hint.
7. **Help bar update** — Add `a` binding to `ShortHelp()`.
8. **View unit tests** — approve flow, guard conditions, success removal, error rendering.
9. **Terminal E2E test** — Press `a`, verify item removed from list.
10. **VHS happy-path recording** — Approve flow animated.

### Out of scope

- Deny action (`approvals-inline-deny` is a separate ticket).
- Undo/rollback after approval.
- Batch approve (multi-select).
- SSE-driven queue updates (already spec'd in `approvals-queue`).
- Run summary cache invalidation on approve (the 30-second TTL from `approvals-context-display` handles this naturally; explicit cache clearing is a future enhancement).

---

## Implementation Plan

### Slice 1: `ApproveGate` client method (`internal/smithers/client.go`)

Add after the `ListPendingApprovals` block (around line 416):

```go
// ApproveGate sends an approve decision for the given approval ID.
// Routes: HTTP POST /approval/{id}/approve → exec smithers approval approve {id}.
func (c *Client) ApproveGate(ctx context.Context, approvalID string) error {
    // 1. Try HTTP
    if c.isServerAvailable() {
        err := c.httpPostJSON(ctx, "/approval/"+approvalID+"/approve", nil, nil)
        if err == nil {
            return nil
        }
        if c.logger != nil {
            c.logger.Warn("ApproveGate HTTP failed, falling back to exec", "approvalID", approvalID, "err", err)
        }
    }

    // 2. Fall back to exec (no SQLite path — approve is a mutation)
    _, err := c.execSmithers(ctx, "approval", "approve", approvalID)
    return err
}
```

**HTTP path note**: At implementation time, verify the exact approve endpoint path against `../smithers/src/server/index.ts`. Two candidate paths exist:
- `POST /approval/{id}/approve` (approval-scoped route)
- `POST /v1/runs/{runId}/nodes/{nodeId}/approve` (run/node-scoped route)

If the server uses the run/node-scoped route, the method signature must accept `runID` and `nodeID` as well, or the client must look them up. Since `Approval.ID`, `Approval.RunID`, and `Approval.NodeID` are all available in the view, the view can pass whichever the method requires. Prefer the shorter `POST /approval/{id}/approve` if available; it matches the existing `GET /approval/list` pattern.

The exec fallback uses `smithers approval approve <approvalID>`. If the CLI uses a different subcommand (e.g., `smithers approve <runID> <nodeID>`), update the args accordingly. The exec path is the safety net; correctness of the HTTP path is the primary goal.

**No SQLite path**: Mutations cannot go through the read-only SQLite connection. The two-tier HTTP → exec pattern is consistent with `ToggleCron` and `CreateCron`.

---

### Slice 2: Client unit tests (`internal/smithers/client_test.go`)

Add alongside the existing `TestToggleCron_*` tests:

```go
func TestApproveGate_HTTP(t *testing.T)
    // httptest.Server expects POST /approval/appr-123/approve
    // Server returns {ok: true}
    // Assert no error returned

func TestApproveGate_HTTPFallbackToExec(t *testing.T)
    // Server returns {ok: false, error: "not found"}
    // Exec func asserts args == ["approval", "approve", "appr-123"]
    // Assert no error returned

func TestApproveGate_ExecError(t *testing.T)
    // No server; exec returns errors.New("smithers binary not found")
    // Assert error propagated

func TestApproveGate_Exec(t *testing.T)
    // newExecClient; exec func asserts args, returns nil error
    // Assert no error returned

func TestApproveGate_ContextCancelled(t *testing.T)
    // Cancel context before call
    // Assert error returned (context.Canceled or deadline exceeded)
```

---

### Slice 3: Inflight state and spinner (`internal/ui/views/approvals.go`)

**New imports** (add to import block):
```go
"github.com/charmbracelet/crush/internal/smithers"
"charm.land/bubbles/v2/spinner"
```

**New message types** (add after `approvalsErrorMsg`):
```go
type approveSuccessMsg struct {
    approvalID string
}

type approveErrorMsg struct {
    approvalID string
    err        error
}
```

**Updated `ApprovalsView` struct**:
```go
type ApprovalsView struct {
    client       *smithers.Client
    approvals    []smithers.Approval
    cursor       int
    width        int
    height       int
    loading      bool
    err          error

    // Inline approve state
    approvingIdx int            // index of item being approved; -1 when idle
    approveErr   error          // last approve error; cleared on next successful approve or navigation
    spinner      spinner.Model  // animated indicator during inflight request
}
```

**Updated `NewApprovalsView`**:
```go
func NewApprovalsView(client *smithers.Client) *ApprovalsView {
    s := spinner.New()
    s.Spinner = spinner.Dot
    return &ApprovalsView{
        client:       client,
        loading:      true,
        approvingIdx: -1,
        spinner:      s,
    }
}
```

**Updated `Init`**: Return `tea.Batch(v.spinner.Tick, v.loadApprovals())` so the spinner is pre-warmed (it only renders during approve, but Bubble Tea requires at least one tick to start the animation loop).

```go
func (v *ApprovalsView) Init() tea.Cmd {
    return tea.Batch(v.spinner.Tick, func() tea.Msg {
        approvals, err := v.client.ListPendingApprovals(context.Background())
        if err != nil {
            return approvalsErrorMsg{err: err}
        }
        return approvalsLoadedMsg{approvals: approvals}
    })
}
```

**`doApprove` helper** (new method):
```go
func (v *ApprovalsView) doApprove(approvalID string) tea.Cmd {
    return func() tea.Msg {
        err := v.client.ApproveGate(context.Background(), approvalID)
        if err != nil {
            return approveErrorMsg{approvalID: approvalID, err: err}
        }
        return approveSuccessMsg{approvalID: approvalID}
    }
}
```

---

### Slice 4: `Update` — `a` key handler and message handling (`internal/ui/views/approvals.go`)

Add to the `tea.KeyPressMsg` switch block (after the `r` case):

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
    if v.approvingIdx == -1 && v.cursor < len(v.approvals) {
        selected := v.approvals[v.cursor]
        if selected.Status == "pending" {
            v.approvingIdx = v.cursor
            v.approveErr = nil
            return v, tea.Batch(v.spinner.Tick, v.doApprove(selected.ID))
        }
    }
```

**Guard conditions**:
1. `v.approvingIdx == -1`: No other approval is inflight. Prevent double-firing.
2. `v.cursor < len(v.approvals)`: Cursor in bounds.
3. `selected.Status == "pending"`: Only pending items can be approved.

Add message handlers to the `Update` switch (after `approvalsErrorMsg`):

```go
case spinner.TickMsg:
    if v.approvingIdx != -1 {
        var cmd tea.Cmd
        v.spinner, cmd = v.spinner.Update(msg)
        return v, cmd
    }
    return v, nil

case approveSuccessMsg:
    // Find the approval by ID (index may have shifted if list refreshed)
    for i, a := range v.approvals {
        if a.ID == msg.approvalID {
            v.approvals = append(v.approvals[:i], v.approvals[i+1:]...)
            if v.cursor >= len(v.approvals) && v.cursor > 0 {
                v.cursor = len(v.approvals) - 1
            }
            break
        }
    }
    v.approvingIdx = -1
    v.approveErr = nil
    return v, nil

case approveErrorMsg:
    v.approvingIdx = -1
    v.approveErr = msg.err
    return v, nil
```

**Why match by ID not index in success handler**: The approve command is async. By the time `approveSuccessMsg` arrives, the list may have been refreshed (via `r` key) or the cursor may have moved. Matching by `Approval.ID` is safe; index matching is not.

---

### Slice 5: `View` — spinner and error rendering (`internal/ui/views/approvals.go`)

**List item rendering** — update `renderListItem` and `renderListCompact` to show the spinner on the inflight item:

```go
func (v *ApprovalsView) renderListItem(idx, width int) string {
    a := v.approvals[idx]
    cursor := "  "
    nameStyle := lipgloss.NewStyle()
    if idx == v.cursor {
        cursor = "▸ "
        nameStyle = nameStyle.Bold(true)
    }

    label := a.Gate
    if label == "" {
        label = a.NodeID
    }
    if len(label) > width-6 {
        label = label[:width-9] + "..."
    }

    // Spinner replaces status icon while inflight
    statusIcon := "○"
    switch {
    case idx == v.approvingIdx:
        statusIcon = v.spinner.View()
    case a.Status == "approved":
        statusIcon = "✓"
    case a.Status == "denied":
        statusIcon = "✗"
    }

    return cursor + statusIcon + " " + nameStyle.Render(label) + "\n"
}
```

**Detail pane** — update `renderDetail` to show the approve error below the payload:

```go
// After the existing payload section in renderDetail:
if v.approveErr != nil && v.cursor < len(v.approvals) &&
    v.approvals[v.cursor].Status == "pending" {
    errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
    b.WriteString("\n" + errStyle.Render("⚠ Approve failed: "+v.approveErr.Error()) + "\n")
    b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Press [a] to retry") + "\n")
}
```

**Help line** — update `View()` header help hint to include `a` key when current item is pending:

```go
helpParts := []string{"[Esc] Back", "[r] Refresh"}
if v.cursor < len(v.approvals) && v.approvals[v.cursor].Status == "pending" {
    if v.approvingIdx == -1 {
        helpParts = append([]string{"[a] Approve"}, helpParts...)
    } else {
        helpParts = append([]string{"Approving..."}, helpParts...)
    }
}
helpHint := lipgloss.NewStyle().Faint(true).Render(strings.Join(helpParts, "  "))
```

---

### Slice 6: `ShortHelp()` update (`internal/ui/views/approvals.go`)

```go
func (v *ApprovalsView) ShortHelp() []key.Binding {
    bindings := []key.Binding{
        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "navigate")),
        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
    }
    if v.cursor < len(v.approvals) && v.approvals[v.cursor].Status == "pending" {
        bindings = append([]key.Binding{
            key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
        }, bindings...)
    }
    return bindings
}
```

---

### Slice 7: View unit tests (`internal/ui/views/approvals_test.go`)

Add alongside the tests from `approvals-context-display`:

```go
func TestApprovalsView_AKeyApprovesPendingItem(t *testing.T)
    // Load 1 pending approval
    // Send "a" key → assert Update returns non-nil Cmd
    // Assert approvingIdx == 0

func TestApprovalsView_AKeyIgnoresNonPending(t *testing.T)
    // Load 1 approval with Status == "approved"
    // Send "a" key → assert no Cmd returned (already resolved)
    // Assert approvingIdx remains -1

func TestApprovalsView_AKeyIgnoredWhileInflight(t *testing.T)
    // Load 2 pending approvals
    // Send "a" → approvingIdx = 0
    // Send "a" again → assert still only 1 inflight (approvingIdx still 0)

func TestApprovalsView_ApproveSuccessRemovesItem(t *testing.T)
    // Load 2 pending approvals
    // Send approveSuccessMsg{approvalID: approvals[0].ID}
    // Assert len(v.approvals) == 1
    // Assert approvingIdx == -1
    // Assert approveErr == nil

func TestApprovalsView_ApproveSuccessClampscursor(t *testing.T)
    // Load 1 pending approval, cursor at 0
    // Send approveSuccessMsg{approvalID: approvals[0].ID}
    // Assert len(v.approvals) == 0
    // Assert cursor == 0 (or clamped to max 0, view shows empty state)

func TestApprovalsView_ApproveErrorSetsErrorField(t *testing.T)
    // Send approveErrorMsg{err: errors.New("network timeout")}
    // Assert approvingIdx == -1
    // Assert approveErr.Error() == "network timeout"

func TestApprovalsView_ApproveErrorRenderedInDetail(t *testing.T)
    // Load pending approval, set approveErr = errors.New("timeout")
    // Render wide view (> 80 cols)
    // Assert View() output contains "Approve failed: timeout"
    // Assert View() output contains "Press [a] to retry"

func TestApprovalsView_SpinnerShownWhileApproving(t *testing.T)
    // Load pending approval, set approvingIdx = 0
    // Assert list item does NOT contain "○" (replaced by spinner)

func TestApprovalsView_ApproveKeyOnlyShownForPending(t *testing.T)
    // Load approved item, call ShortHelp()
    // Assert no binding with key "a" in result

func TestApprovalsView_ApproveKeyShownForPending(t *testing.T)
    // Load pending item, call ShortHelp()
    // Assert binding with key "a" and help "approve" is present
```

---

### Slice 8: Terminal E2E test (`internal/e2e/approvals_inline_approve_test.go`)

Modeled on `internal/e2e/chat_domain_system_prompt_test.go` and the `approvals-context-display` E2E spec.

```go
func TestApprovalsInlineApprove_TUI(t *testing.T) {
    if os.Getenv("SMITHERS_TUI_E2E") != "1" {
        t.Skip("set SMITHERS_TUI_E2E=1 to run terminal E2E tests")
    }

    // Mock server:
    // - GET /health → 200
    // - GET /approval/list → [{id:"appr-1", runId:"run-1", ..., status:"pending", gate:"Deploy to staging?"}]
    // - POST /approval/appr-1/approve → {ok: true}
    // - GET /approval/list (after approve) → [] (empty, item consumed)
    mockServer := startMockApproveServer(t)
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

    require.NoError(t, tui.WaitForText("CRUSH", 15*time.Second))

    // Open approvals view via ctrl+a
    tui.SendKeys("\x01")

    // Wait for the approval to load
    require.NoError(t, tui.WaitForText("Deploy to staging?", 5*time.Second),
        "should show pending approval; buffer: %s", tui.Snapshot())

    // Press 'a' to approve
    tui.SendKeys("a")

    // Verify the item disappears from the list
    require.NoError(t, tui.WaitForText("No pending approvals", 5*time.Second),
        "approved item should be removed; buffer: %s", tui.Snapshot())

    // Return to chat
    tui.SendKeys("\x1b")
    require.NoError(t, tui.WaitForText("CRUSH", 5*time.Second),
        "esc should return to chat; buffer: %s", tui.Snapshot())
}
```

**Mock server note**: The mock must track whether `POST /approval/appr-1/approve` was called and serve an empty list on subsequent `GET /approval/list` calls. Use a `sync.Mutex`-protected `approved bool` flag.

A second test covers the error path:

```go
func TestApprovalsInlineApprove_Error_TUI(t *testing.T) {
    // Mock server: approve endpoint returns {ok: false, error: "rate limited"}
    // Press 'a' → wait for "Approve failed" error in view
    // Press 'a' again → retry fires another POST (server now returns ok: true)
    // Verify item removed
}
```

---

### Slice 9: VHS happy-path recording (`tests/vhs/approvals-inline-approve.tape`)

```tape
# Approvals inline approve — press 'a' to approve a pending gate
Output tests/vhs/output/approvals-inline-approve.gif
Set FontSize 14
Set Width 120
Set Height 35
Set Shell zsh

# Start TUI with mock server fixture
Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Open approvals view
Ctrl+a
Sleep 2s

# Capture pending approval card with [a] Approve hint
Screenshot tests/vhs/output/approvals-inline-approve-before.png

# Press 'a' to approve — spinner appears briefly
Type "a"
Sleep 1s

# Capture post-approve state (item removed or "No pending approvals")
Screenshot tests/vhs/output/approvals-inline-approve-after.png

# Return to chat
Escape
Sleep 1s

Screenshot tests/vhs/output/approvals-inline-approve-back.png
```

---

## Validation

### Automated checks

| Check | Command | What it proves |
|---|---|---|
| Client compiles | `go build ./internal/smithers/...` | `ApproveGate` method signature valid |
| Client unit tests pass | `go test ./internal/smithers/ -run TestApproveGate -v` | HTTP, exec, fallback, error paths |
| View unit tests pass | `go test ./internal/ui/views/ -run TestApprovalsView -v` | Approve key guard, success removal, cursor clamp, error rendering, spinner, ShortHelp |
| Full build succeeds | `go build ./...` | No import cycles; spinner import resolves |
| Existing tests pass | `go test ./...` | No regressions in list load, chat, navigation |
| Terminal E2E: approve flow | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsInlineApprove_TUI -timeout 30s -v` | `a` key removes item from live view |
| Terminal E2E: error+retry | `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsInlineApprove_Error_TUI -timeout 30s -v` | Error displayed; `a` retries successfully |
| VHS recording | `vhs tests/vhs/approvals-inline-approve.tape` | Happy path animated; exit code 0 |

### Manual verification

1. **Build**: `go build -o smithers-tui . && ./smithers-tui`
2. **With live server**: Start `smithers up --serve`, create a workflow with `<ApprovalGate>`, run until it pauses. Press `Ctrl+A`. Verify `[a] Approve` appears in help bar. Press `a`. Verify spinner on list item, then item disappears.
3. **Cursor clamp**: With 1 pending approval, press `a`. Verify cursor stays at 0 (or view shows "No pending approvals") — no index-out-of-bounds.
4. **Already-resolved item**: Navigate to an "approved" or "denied" item. Press `a`. Verify nothing happens (no API call, no state change).
5. **Double-press**: Press `a` twice quickly. Verify only one API call fires (inflight guard).
6. **Error state**: Kill the Smithers server mid-approve. Verify `⚠ Approve failed:` appears in detail pane. Restart server, press `a` again, verify retry succeeds.
7. **No server**: With no Smithers server, press `a`. Verify exec fallback fires (`smithers approval approve <id>`); if `smithers` binary absent, verify graceful error.
8. **Narrow terminal**: Resize to < 80 cols (compact mode). Verify spinner and error still render correctly in the compact list.
9. **ShortHelp**: Navigate to a pending item — verify `[a] approve` in help bar. Navigate to a resolved item — verify `[a]` absent.

---

## Risks

### 1. HTTP approve endpoint path

**Risk**: The exact HTTP path for approving a gate may differ from `POST /approval/{id}/approve`. The GUI reference used `POST /v1/runs/{runId}/nodes/{nodeId}/approve`, which requires two IDs. If the server only exposes the run/node-scoped path, `ApproveGate(approvalID)` cannot be called without also passing `RunID` and `NodeID`.

**Mitigation**: The `Approval` struct carries `RunID`, `NodeID`, and `ID`. If the node-scoped route is required, change the method signature to `ApproveGate(ctx, approvalID, runID, nodeID string)` and update the view to pass all three. The exec fallback (`smithers approval approve <id>`) only needs `ID` and remains unaffected. Probe `../smithers/src/server/index.ts` at implementation time to confirm.

### 2. List refresh during inflight approve

**Risk**: If the user presses `r` to refresh the list while an approve is inflight, `approvalsLoadedMsg` arrives and replaces `v.approvals`. The `approveSuccessMsg` then searches for the approval by ID but may not find it (if the refreshed list no longer includes it because the server already recorded the approval). The item has been approved, but the success handler's `for i, a := range v.approvals` loop finds no match and the cursor is not adjusted.

**Mitigation**: The `approveSuccessMsg` handler iterates by ID, not index. If no match is found (the refreshed list already excluded the approved item), the handler silently clears `approvingIdx` and `approveErr` without crashing. The view is in a consistent state: the item is absent (correct) and inflight state is cleared. No special handling is required.

### 3. Spinner tick and non-approve state

**Risk**: `spinner.TickMsg` messages arrive continuously once the spinner is started. If `Init()` starts the spinner unconditionally, tick messages will arrive even when no approve is inflight, wasting CPU and causing unnecessary re-renders.

**Mitigation**: The `spinner.TickMsg` handler guards on `v.approvingIdx != -1`. When idle, the tick is consumed without returning a new `Cmd`, so the loop stops. The spinner restarts on the next `doApprove` call which batches `spinner.Tick`. This is the standard Bubble Tea pattern.

### 4. Dependency on `approvals-context-display` state fields

**Risk**: If `approvals-context-display` adds fields like `approvingIdx int` or modifies `ApprovalsView` struct in a way that conflicts, there will be a merge conflict.

**Mitigation**: `approvals-context-display` only adds `selectedRun`, `contextLoading`, `contextErr`, `lastFetchRun` and new message types. None of these conflict with the fields added here (`approvingIdx`, `approveErr`, `spinner`). The `approvals-context-display` plan must be merged first (it is a listed dependency); this plan then adds on top.

### 5. Spinner import

**Risk**: `charm.land/bubbles/v2/spinner` may not be imported in `internal/ui/views/` yet.

**Mitigation**: Check existing view files for spinner usage. The chat view and run dashboard likely already import it. If not, add to `go.mod`/`go.sum` — the package is part of the Charm Bubbles suite already present in the repo.

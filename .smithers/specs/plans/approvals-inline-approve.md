## Goal

Add inline approval of pending gates to the `ApprovalsView`: pressing `a` fires `ApproveGate` on the selected pending item, animates a spinner while inflight, removes the item from the list on success, and surfaces an inline error with retry hint on failure.

This directly satisfies PRD §6.5 ("Inline approve/deny: Act on gates without leaving current view") and the `[a] Approve` action shown in the `02-DESIGN.md §3.5` approval card wireframe.

---

## Steps

### Step 1: Add `ApproveGate` client method

**File**: `internal/smithers/client.go`

Add after the `ListPendingApprovals` block:

```go
// ApproveGate sends an approve decision for the given approval ID.
// Routes: HTTP POST /approval/{id}/approve → exec smithers approval approve {id}.
func (c *Client) ApproveGate(ctx context.Context, approvalID string) error {
    if c.isServerAvailable() {
        err := c.httpPostJSON(ctx, "/approval/"+approvalID+"/approve", nil, nil)
        if err == nil {
            return nil
        }
        if c.logger != nil {
            c.logger.Warn("ApproveGate HTTP failed, falling back to exec",
                "approvalID", approvalID, "err", err)
        }
    }
    _, err := c.execSmithers(ctx, "approval", "approve", approvalID)
    return err
}
```

Before committing this path, check `../smithers/src/server/index.ts` for the exact approve endpoint. If the server uses `POST /v1/runs/{runId}/nodes/{nodeId}/approve` instead of `/approval/{id}/approve`, change the method signature to accept `runID` and `nodeID` as well (the `Approval` struct carries both). The exec fallback only needs `approvalID` regardless.

**Verification**: `go build ./internal/smithers/...`

---

### Step 2: Add client unit tests

**File**: `internal/smithers/client_test.go`

Add five tests following the `TestToggleCron_*` pattern:

```go
func TestApproveGate_HTTP(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/approval/appr-1/approve", r.URL.Path)
        assert.Equal(t, "POST", r.Method)
        writeEnvelope(t, w, nil)
    })
    err := c.ApproveGate(context.Background(), "appr-1")
    require.NoError(t, err)
}

func TestApproveGate_Exec(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        assert.Equal(t, []string{"approval", "approve", "appr-2"}, args)
        return nil, nil
    })
    err := c.ApproveGate(context.Background(), "appr-2")
    require.NoError(t, err)
}

func TestApproveGate_HTTPFallbackToExec(t *testing.T) { ... }
func TestApproveGate_ExecError(t *testing.T)           { ... }
func TestApproveGate_ContextCancelled(t *testing.T)    { ... }
```

**Verification**: `go test ./internal/smithers/ -run TestApproveGate -v`

---

### Step 3: Add inflight state and message types to `ApprovalsView`

**File**: `internal/ui/views/approvals.go`

Add two new message types after `approvalsErrorMsg`:

```go
type approveSuccessMsg struct{ approvalID string }
type approveErrorMsg   struct{ approvalID string; err error }
```

Add three new fields to `ApprovalsView`:

```go
approvingIdx int           // index being approved; -1 when idle
approveErr   error         // last approve error; nil when idle or on retry
spinner      spinner.Model // animated dot while inflight
```

Update `NewApprovalsView` to initialize them:

```go
s := spinner.New()
s.Spinner = spinner.Dot
return &ApprovalsView{
    client:       client,
    loading:      true,
    approvingIdx: -1,
    spinner:      s,
}
```

Update `Init` to batch the spinner tick with the list fetch:

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

Add the `doApprove` command helper:

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

**Verification**: `go build ./internal/ui/views/...`

---

### Step 4: Wire `a` key and message handlers in `Update`

**File**: `internal/ui/views/approvals.go`

In the `tea.KeyPressMsg` switch, add after the `r` case:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
    if v.approvingIdx == -1 && v.cursor < len(v.approvals) {
        if v.approvals[v.cursor].Status == "pending" {
            v.approvingIdx = v.cursor
            v.approveErr = nil
            return v, tea.Batch(v.spinner.Tick, v.doApprove(v.approvals[v.cursor].ID))
        }
    }
```

In the outer `msg` switch, add after `approvalsErrorMsg`:

```go
case spinner.TickMsg:
    if v.approvingIdx != -1 {
        var cmd tea.Cmd
        v.spinner, cmd = v.spinner.Update(msg)
        return v, cmd
    }
    return v, nil

case approveSuccessMsg:
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

**Verification**: `go build ./...` — no compilation errors.

---

### Step 5: Update `renderListItem` to show spinner

**File**: `internal/ui/views/approvals.go`

In `renderListItem`, replace the `statusIcon` block:

```go
statusIcon := "○"
switch {
case idx == v.approvingIdx:
    statusIcon = v.spinner.View()
case a.Status == "approved":
    statusIcon = "✓"
case a.Status == "denied":
    statusIcon = "✗"
}
```

Apply the same spinner substitution in `renderListCompact`:

```go
statusIcon := "○"
switch {
case i == v.approvingIdx:
    statusIcon = v.spinner.View()
case a.Status == "approved":
    statusIcon = "✓"
case a.Status == "denied":
    statusIcon = "✗"
}
```

**Verification**: `go test ./internal/ui/views/ -run TestApprovalsView -v`

---

### Step 6: Update `renderDetail` to show approve error

**File**: `internal/ui/views/approvals.go`

At the end of `renderDetail`, after the payload section, add:

```go
if v.approveErr != nil && v.cursor < len(v.approvals) &&
    v.approvals[v.cursor].Status == "pending" {
    errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
    b.WriteString("\n" + errStyle.Render("⚠ Approve failed: "+v.approveErr.Error()) + "\n")
    b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Press [a] to retry") + "\n")
}
```

---

### Step 7: Update `ShortHelp` and header hint

**File**: `internal/ui/views/approvals.go`

Update `ShortHelp()`:

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

Update the help hint in `View()`. Replace the static `"[Esc] Back"` with a dynamic hint:

```go
var hintParts []string
if v.cursor < len(v.approvals) && v.approvals[v.cursor].Status == "pending" {
    if v.approvingIdx != -1 {
        hintParts = append(hintParts, "Approving...")
    } else {
        hintParts = append(hintParts, "[a] Approve")
    }
}
hintParts = append(hintParts, "[Esc] Back")
helpHint := lipgloss.NewStyle().Faint(true).Render(strings.Join(hintParts, "  "))
```

**Verification**: `go test ./internal/ui/views/ -run TestApprovalsView -v`

---

### Step 8: View unit tests

**File**: `internal/ui/views/approvals_test.go`

Add 10 tests. Key cases:

1. `TestApprovalsView_AKeyApprovesPendingItem` — `a` key on pending item sets `approvingIdx`, returns non-nil Cmd.
2. `TestApprovalsView_AKeyIgnoresNonPending` — `a` key on approved/denied item returns nil Cmd, `approvingIdx` stays -1.
3. `TestApprovalsView_AKeyIgnoredWhileInflight` — `a` key while `approvingIdx != -1` is a no-op.
4. `TestApprovalsView_ApproveSuccessRemovesItem` — `approveSuccessMsg` filters list by ID.
5. `TestApprovalsView_ApproveSuccessCursorClamped` — last item approved, cursor clamped to 0.
6. `TestApprovalsView_ApproveErrorSetsField` — `approveErrorMsg` sets `approveErr`, clears `approvingIdx`.
7. `TestApprovalsView_ApproveErrorRenderedInDetail` — error text appears in `View()` output.
8. `TestApprovalsView_SpinnerShownOnInflightItem` — `approvingIdx == 0`, list item does not contain `"○"`.
9. `TestApprovalsView_ShortHelpIncludesApproveForPending` — `ShortHelp()` contains `a` binding when cursor on pending.
10. `TestApprovalsView_ShortHelpNoApproveForResolved` — `ShortHelp()` has no `a` binding on non-pending item.

**Verification**: `go test ./internal/ui/views/ -run TestApprovalsView -v`

---

### Step 9: Terminal E2E test

**File**: `internal/e2e/approvals_inline_approve_test.go`

Follow the existing pattern in `internal/e2e/chat_domain_system_prompt_test.go`.

Mock server provides:
- `GET /health` → 200
- `GET /approval/list` → 1 pending approval (or empty after approve)
- `POST /approval/appr-1/approve` → `{ok: true}` (sets approved flag)

After the first POST, subsequent `GET /approval/list` calls return `[]`.

Test flow:
1. Launch TUI; wait for `"CRUSH"`.
2. `ctrl+a` → wait for `"Deploy to staging?"`.
3. `"a"` → wait for `"No pending approvals"` (item removed).
4. `esc` → wait for `"CRUSH"`.

Add a second test for the error+retry path:
- Mock returns `{ok: false, error: "rate limited"}` on first POST.
- Test waits for `"Approve failed"` in view.
- Press `a` again → mock returns `{ok: true}` → item disappears.

**Verification**: `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsInlineApprove -timeout 60s -v`

---

### Step 10: VHS recording

**File**: `tests/vhs/approvals-inline-approve.tape`

```tape
Output tests/vhs/output/approvals-inline-approve.gif
Set FontSize 14
Set Width 120
Set Height 35
Set Shell zsh

Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

Ctrl+a
Sleep 2s
Screenshot tests/vhs/output/approvals-inline-approve-before.png

Type "a"
Sleep 1s
Screenshot tests/vhs/output/approvals-inline-approve-after.png

Escape
Sleep 1s
Screenshot tests/vhs/output/approvals-inline-approve-back.png
```

VHS fixtures directory must have a config pointing to a mock server (or stub data) that returns one pending approval. See `tests/vhs/fixtures/` in the existing VHS setup for reference.

**Verification**: `vhs tests/vhs/approvals-inline-approve.tape` exits 0.

---

## File Plan

- [`internal/smithers/client.go`](/Users/williamcory/crush/internal/smithers/client.go) — add `ApproveGate`
- [`internal/smithers/client_test.go`](/Users/williamcory/crush/internal/smithers/client_test.go) — add `TestApproveGate_*`
- [`internal/ui/views/approvals.go`](/Users/williamcory/crush/internal/ui/views/approvals.go) — inflight state, `a` key handler, spinner, success/error msgs, updated render methods, updated `ShortHelp`
- [`internal/ui/views/approvals_test.go`](/Users/williamcory/crush/internal/ui/views/approvals_test.go) — 10 new view tests
- [`internal/e2e/approvals_inline_approve_test.go`](/Users/williamcory/crush/internal/e2e/approvals_inline_approve_test.go) — new E2E test (2 cases)
- [`tests/vhs/approvals-inline-approve.tape`](/Users/williamcory/crush/tests/vhs/approvals-inline-approve.tape) — new VHS tape

---

## Validation

1. `gofumpt -w internal/smithers internal/ui/views internal/e2e`
2. `go build ./...`
3. `go test ./internal/smithers/ -run TestApproveGate -v`
4. `go test ./internal/ui/views/ -run TestApprovalsView -v`
5. `go test ./... ` (no regressions)
6. `SMITHERS_TUI_E2E=1 go test ./internal/e2e/ -run TestApprovalsInlineApprove -timeout 60s -v`
7. `vhs tests/vhs/approvals-inline-approve.tape`
8. Manual: start `smithers up --serve`, navigate to approvals (`ctrl+a`), press `a` on a pending gate, confirm spinner then removal; press `a` on a resolved item and confirm no-op.

## Open Questions

1. Exact HTTP path for the approve endpoint — `/approval/{id}/approve` vs `/v1/runs/{runId}/nodes/{nodeId}/approve`. Verify in `../smithers/src/server/index.ts` before implementing Slice 1.
2. Does the exec CLI accept `smithers approval approve <id>` or `smithers approve <runId> <nodeId>`? Check `../smithers/src/cli/approve.ts`.
3. Should a successful approve also trigger a list refresh (`v.Init()`) after removing the item, to pick up any other state changes the server made (e.g., gate auto-resolved others)? The current plan filters in-place; discuss with the team.

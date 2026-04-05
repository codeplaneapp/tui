## Existing Surface

### `ApprovalsView` key handling (`internal/ui/views/approvals.go`, lines 74–93)

The view's `Update` method handles `tea.KeyPressMsg` with three bindings today:

| Key | Action |
|---|---|
| `esc`, `alt+esc` | Pop view (emit `PopViewMsg{}`) |
| `up`, `k` | Move cursor up |
| `down`, `j` | Move cursor down |
| `r` | Reload (set `loading = true`, return `v.Init()`) |

No `a` key binding exists. No approve or deny mutation is wired anywhere in `ApprovalsView`.

**State fields on `ApprovalsView`**:
```go
type ApprovalsView struct {
    client    *smithers.Client
    approvals []smithers.Approval
    cursor    int
    width     int
    height    int
    loading   bool
    err       error
}
```

There is no `approving bool`, `approveErr error`, or per-item inflight state. The view currently has no mutation path at all.

**`ShortHelp()` returns** (lines 334–339):
```go
[]key.Binding{
    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "navigate")),
    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}
```

The `a` key is absent from the help bar.

---

### `Approval` struct (`internal/smithers/types.go`, lines 83–96)

```go
type Approval struct {
    ID           string  `json:"id"`
    RunID        string  `json:"runId"`
    NodeID       string  `json:"nodeId"`
    WorkflowPath string  `json:"workflowPath"`
    Gate         string  `json:"gate"`
    Status       string  `json:"status"`       // "pending" | "approved" | "denied"
    Payload      string  `json:"payload"`
    RequestedAt  int64   `json:"requestedAt"`
    ResolvedAt   *int64  `json:"resolvedAt"`
    ResolvedBy   *string `json:"resolvedBy"`
}
```

The `ID` field is the approval record's primary key used in the approve API call. `Status` is the field that must transition from `"pending"` to `"approved"` on success.

---

### `Client` — no `ApproveGate` method exists

Searching `internal/smithers/client.go` and all `*.go` files in `internal/smithers/` confirms there is **no `ApproveGate`, `Approve`, `DenyGate`, or `Deny` method** on the client. The only mutation methods currently present are:

| Method | HTTP endpoint |
|---|---|
| `CreateCron` | `POST /cron/add` |
| `ToggleCron` | `POST /cron/toggle/{id}` |
| `DeleteCron` | exec only |

`ApproveGate` needs to be added as a new client method.

---

### HTTP API — approve endpoint

The upstream research for `approvals-context-display` identified (from `smithers_tmp/tests/tui.e2e.test.ts` and the GUI reference):

```
POST /v1/runs/:id/nodes/:nodeId/approve
POST /v1/runs/:id/nodes/:nodeId/deny
```

These are confirmed in `smithers_tmp/` source references cited in the `approvals-context-display` research doc (`src/server/index.ts`). The approve endpoint takes a JSON body and the approval's run ID + node ID are available directly on the `Approval` struct (`RunID`, `NodeID`).

**Alternative route**: Some Smithers versions also expose:
```
POST /approval/{approvalId}/approve
```

At implementation time, confirm which path the running server exposes by checking `../smithers/src/server/index.ts`. If neither HTTP path is available, the exec fallback is:
```
smithers approve <approvalId>
```
or equivalently:
```
smithers approval approve <approvalId>
```

The three-tier transport pattern (HTTP → SQLite → exec) applies: approve is a mutation so SQLite is skipped; the transport order is HTTP → exec.

---

### Tea message/command pattern for async mutations

The `approvals-context-display` engineering spec establishes the pattern for async operations in the approvals view:

```go
// 1. Add message types
type runSummaryLoadedMsg struct { ... }
type runSummaryErrorMsg  struct { ... }

// 2. Dispatch a tea.Cmd that blocks until the API returns
func (v *ApprovalsView) fetchRunContext() tea.Cmd {
    return func() tea.Msg {
        result, err := v.client.SomeMethod(context.Background(), ...)
        if err != nil {
            return someErrorMsg{err: err}
        }
        return someLoadedMsg{result: result}
    }
}

// 3. Handle the result in Update
case someLoadedMsg:
    v.field = msg.result
    return v, nil
```

This same pattern applies for the approve action:
- `approveSuccessMsg{approvalID string}` — remove from list, stop spinner
- `approveErrorMsg{approvalID string, err error}` — surface error, allow retry

---

### Loading indicator pattern

The `ApprovalsView.loading` field is a boolean that triggers `"  Loading approvals...\n"` in `View()`. The same approach applies for per-item inflight state. Two options:

**Option A — view-level `approving bool`**: Simple; only one approval can be approved at a time. The user presses `a`, the view sets `approving = true`, renders a spinner, and the key handler ignores additional `a` presses until the command completes.

**Option B — per-item `approvingIdx int`**: Stores the index of the approval currently being approved (`-1` when idle). The list renders a spinner only on that row. More precise, avoids blocking cursor movement during the inflight request.

Option B is preferred: it matches the design wireframe (inline approve action per row) and is consistent with the enriched list rendering added by `approvals-context-display`.

**Spinner**: Crush ships `charm.land/bubbles/v2/spinner`. The existing Crush codebase uses `spinner.Model` in chat and run views. The approvals view can embed a `spinner.Model` and tick it via `spinner.Tick` while the approve command is inflight.

---

### Success: remove from pending queue

The acceptance criterion says "upon success, the item is removed from the pending queue." Two strategies:

**Strategy A — filter in-place**: After `approveSuccessMsg`, filter `v.approvals` to remove the item with `Approval.ID == msg.approvalID`. Clamp the cursor if it's now out of bounds. No additional API call needed.

**Strategy B — full refresh**: After `approveSuccessMsg`, trigger `v.Init()` (re-fetch the full list). Simpler to reason about, but causes a brief "Loading..." flash and discards any enriched `selectedRun` context.

Strategy A is preferred for snappier UX: the approved item can be removed immediately, the list narrows, and the cursor is clamped. The server-side truth is confirmed by the success message itself.

---

### Error handling and retry

The ticket requires error handling and retry. Patterns from the codebase:

- **View-level `err error`**: Already present on `ApprovalsView`. Renders as `"  Error: %v\n"` in the error branch of `View()`. This field is currently used only for list-load errors.
- **Per-approve error**: The view needs a separate `approveErr error` field (or `approveErrMsg string`) so the error appears inline next to the failing item rather than replacing the entire view with an error screen.
- **Retry**: The user presses `a` again. Since the view removes the inflight-flag on error (`approvingIdx = -1`, `approveErr = msg.err`), the `a` key will fire again on the next press. The UI renders the error inline (e.g., `"⚠ Approve failed: <msg> — press [a] to retry"`) in the detail pane.

---

### Key binding: `a` vs `ctrl+a`

The help bar in `02-DESIGN.md §3.5` shows `[a] Approve` as an inline action in the approval card. `ctrl+a` is already used to open the approvals view itself (from `ui.go` or the global keybindings). Inside the view, `a` (lowercase, no modifier) is the correct binding for approve. This is consistent with the run dashboard design (`02-DESIGN.md §3.2`) which also shows `[a] Approve`.

There is no conflict: `ctrl+a` navigates to the approvals view; once inside, plain `a` approves the selected item.

---

### Guard: only approve pending items

The `a` key handler must guard against approving non-pending items:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
    if v.cursor < len(v.approvals) {
        selected := v.approvals[v.cursor]
        if selected.Status == "pending" && v.approvingIdx == -1 {
            v.approvingIdx = v.cursor
            v.approveErr = nil
            return v, v.doApprove(selected.ID)
        }
    }
```

Without this guard, pressing `a` on an already-approved/denied item would fire a redundant API call.

---

### `Client` field: `binaryPath`

The `exec.go` file (line 113+) confirms the exec transport uses `c.binaryPath` (defaulting to `"smithers"`) rather than the hardcoded string `"smithers"` used in the older `client.go` section. Any new `execSmithers` calls must use the updated `execSmithers` method from `exec.go`, not the deprecated pattern.

---

### Gaps Summary

| Gap | Severity | How to close |
|---|---|---|
| No `ApproveGate` client method | High — required | New method in `internal/smithers/client.go` |
| No HTTP route confirmed for approve | Medium — needs probe | Check `../smithers/src/server/index.ts`; fallback to exec |
| No `a` key binding in `ApprovalsView` | High — core feature | Add case in `Update`, guard on `Status == "pending"` |
| No inflight state | High | Add `approvingIdx int` + `approveErr error` to struct |
| No spinner | Medium — per spec | Embed `spinner.Model`, tick while `approvingIdx != -1` |
| No success handler removing from list | High — acceptance criterion | Filter `v.approvals` on `approveSuccessMsg` |
| No per-item error display | Medium | Render `approveErr` in detail pane below context |
| No help bar update | Low | Add `a` binding to `ShortHelp()` for pending items |
| No test for approve action | High | Add tests to `internal/smithers/client_test.go` + `approvals_test.go` |

---

## Files To Touch

- [`internal/smithers/client.go`](/Users/williamcory/crush/internal/smithers/client.go) — add `ApproveGate(ctx, approvalID string) error`
- [`internal/smithers/client_test.go`](/Users/williamcory/crush/internal/smithers/client_test.go) — add `TestApproveGate_*` tests
- [`internal/ui/views/approvals.go`](/Users/williamcory/crush/internal/ui/views/approvals.go) — add `a` key handler, inflight state, spinner, success/error handling
- [`internal/ui/views/approvals_test.go`](/Users/williamcory/crush/internal/ui/views/approvals_test.go) — add view tests for approve flow

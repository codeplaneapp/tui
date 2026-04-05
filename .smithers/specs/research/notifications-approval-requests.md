# Research: notifications-approval-requests

**Ticket**: notifications-approval-requests
**Depends on**: notifications-toast-overlays
**Feature flag**: `NOTIFICATIONS_APPROVAL_REQUESTS`

---

## 1. What this ticket does

`notifications-toast-overlays` wires the `ToastManager` into the root Bubble
Tea loop and handles `status_changed` SSE events generically, including a
basic `waiting-approval` toast with a short run ID and a `[ctrl+a]` action
hint.

This ticket upgrades that approval toast with:

1. **Gate question / context** from the `Approval.Gate` field — so the toast
   body reads "Deploy to staging?" rather than "run-abc12345 is waiting for
   approval."
2. **Approve and View action hints**: `[a] approve`, `[ctrl+a] view approvals`
   rendered in the toast footer.
3. **Optional bell/terminal alert**: a `\a` (BEL) byte written to stdout when
   a new approval toast fires, giving an audible / visual terminal-dock signal.
4. **Deduplication** per approval ID (not just per run ID + status), so a
   re-polled `waiting-approval` status does not produce duplicate toasts.

---

## 2. Current state after `notifications-toast-overlays`

### 2.1 What already exists

After the parent ticket lands, `notifications.go` contains:

```go
case smithers.RunStatusWaitingApproval:
    return &components.ShowToastMsg{
        Title: "Approval needed",
        Body:  shortID + " is waiting for approval",
        Level: components.ToastLevelWarning,
        ActionHints: []components.ActionHint{
            {Key: "ctrl+a", Label: "view approvals"},
        },
    }
```

`notificationTracker.shouldToastApproval(approvalID string) bool` exists but
is never called from `runEventToToast` — it was stubbed for this ticket.

`keys.go` already defines `Approvals` as `ctrl+a` → navigates to approvals
view.

### 2.2 What is missing

| Missing | Why |
|---|---|
| Gate question in toast body | `RunEvent` carries `RunID` + `Status` but no `Gate` field |
| Per-approval-ID dedup | `runEventToToast` deduplicates on `(runID, status)` — a single run that enters `waiting-approval` twice for different gates would be suppressed |
| "approve" action hint | Currently only `[ctrl+a] view approvals`; ticket requires `[a] approve` too |
| Bell/BEL alert | No audio signal exists |

---

## 3. SSE event shape and the gate-context problem

### 3.1 `RunEvent` struct (current)

```go
type RunEvent struct {
    Type        string
    RunID       string
    NodeID      string
    Iteration   int
    Attempt     int
    Status      string
    TimestampMs int64
    Seq         int
    Raw         json.RawMessage `json:"-"`
}
```

The `Type == "status_changed"` event that triggers `waiting-approval` does not
carry the gate label, the approval ID, or the gate question.  The
`RunEvent.Raw` field preserves the full JSON frame, which may contain
additional fields emitted by the TypeScript server — but `Raw` is tagged
`json:"-"` (excluded from the standard unmarshal).

### 3.2 Two strategies for obtaining gate context

#### Strategy A — Enrich `RunEvent` with optional approval fields

Extend the struct with optional fields that the TypeScript server already
sends (or should send) in the `status_changed` frame:

```go
type RunEvent struct {
    // ... existing fields ...
    ApprovalID   string `json:"approvalId,omitempty"`
    ApprovalGate string `json:"approvalGate,omitempty"` // gate question / label
}
```

**Pros**: One SSE frame contains everything; no extra HTTP call.
**Cons**: Requires confirming the TypeScript server actually sends these fields;
may need a Smithers-side change if it doesn't.

#### Strategy B — Fetch the pending approval on `waiting-approval` status change

When `runEventToToast` receives a `waiting-approval` status event, fire an
async `tea.Cmd` that calls `client.ListPendingApprovals` and filters by
`RunID`.  The first matching pending approval provides the gate label and ID.

```go
case smithers.RunStatusWaitingApproval:
    // Fire a Cmd to fetch the pending approval details.
    return fetchApprovalAndToastCmd(ev.RunID, m.smithersClient)
```

**Pros**: Zero changes to the SSE wire format or TypeScript server; uses the
existing `ListPendingApprovals` HTTP/SQLite/exec path.
**Cons**: Adds one extra HTTP round-trip per approval notification; slight
latency (typically < 100 ms against local server).

**Recommendation**: Use Strategy B as the primary path. It is zero-risk for
the backend and self-contained in the TUI. Strategy A can be revisited as an
optimization if latency becomes observable.

### 3.3 `Approval` struct (existing)

```go
type Approval struct {
    ID           string  // dedup key
    RunID        string
    NodeID       string
    WorkflowPath string
    Gate         string  // "Deploy to staging?" — the human-readable question
    Status       string  // "pending" | "approved" | "denied"
    Payload      string  // JSON context for the gate
    RequestedAt  int64
    ResolvedAt   *int64
    ResolvedBy   *string
}
```

`Gate` is exactly the field needed for the toast body.

### 3.4 `ListPendingApprovals` availability

`client.ListPendingApprovals` (in `client.go`) follows the standard transport
cascade:

1. HTTP `GET /approval/list`
2. SQLite `SELECT * FROM _smithers_approvals ORDER BY requested_at DESC`
3. `exec smithers approval list --format json`

Filter: only `Status == "pending"` and `RunID == ev.RunID`.

---

## 4. Toast design

### 4.1 Target appearance

```
┌──────────────────────────────────────────┐
│ Approval needed                          │
│ Deploy to staging?                       │
│ run: def456 · workflow: deploy-staging   │
│                                          │
│ [a] approve  [ctrl+a] view approvals     │
└──────────────────────────────────────────┘
```

Fields:
- **Title**: "Approval needed" (unchanged, consistent with existing toasts)
- **Body line 1**: `gate.Gate` (the question, e.g. "Deploy to staging?")
- **Body line 2**: `run: {shortID} · {workflowName}` for context
- **Action hints**: `{Key: "a", Label: "approve"}`, `{Key: "ctrl+a", Label: "view approvals"}`

If the gate question is empty (server didn't populate it), fall back to the
existing text: `shortID + " is waiting for approval"`.

### 4.2 Toast TTL

Approval toasts should persist longer than the default 5 s because they require
user action. Proposed TTL: `15 * time.Second` (configurable).

### 4.3 `MaxToastWidth` constraint

`MaxToastWidth = 48` columns. Gate questions that exceed `innerW` will be
word-wrapped by `ToastManager.renderToast`. The `Gate` field on real Smithers
workflows is typically a short imperative question ("Deploy to staging?",
"Delete user data?") that fits within 48 columns.

---

## 5. Deduplication: per-approval-ID vs per-(runID,status)

### 5.1 Current dedup gap

`runEventToToast` uses `shouldToastRunStatus(runID, "waiting-approval")`.  If
a run's approval gate is resolved and a second gate is raised (both produce
`waiting-approval` status), the second toast is suppressed because the
`(runID, "waiting-approval")` pair is already in `seenRunStates`.

A run with sequential gate patterns (gate A → approve → gate B → ...) will
silently drop all toasts after the first.

### 5.2 Correct dedup for approval requests

After Strategy B fetches the `Approval`, use `shouldToastApproval(approval.ID)`
as the dedup guard.  Since each gate has a distinct `Approval.ID`, sequential
gates on the same run each produce a toast.

The `(runID, "waiting-approval")` run-level dedup should be removed or changed
to `(runID, approvalID)` when the approval detail is available.

### 5.3 Fallback dedup

If the `ListPendingApprovals` call fails (server unavailable, exec error), fall
back to `shouldToastRunStatus(runID, "waiting-approval")`.  Log a warning.

---

## 6. Bell / BEL alert

### 6.1 What the terminal BEL character does

Writing `\a` (ASCII 0x07) to stdout causes:
- Audible beep in terminals that have system sound enabled (xterm, iTerm2 with
  bell enabled, etc.)
- Visual bell (flash) in some terminals
- Dock badge / taskbar notification in macOS Terminal.app and iTerm2

This is the canonical way to produce an attention signal from a TUI without
relying on OS notification APIs.

### 6.2 Implementation

```go
// bellCmd returns a tea.Cmd that writes the BEL character to stdout.
// Used as an optional audio/visual alert for new approval requests.
func bellCmd() tea.Cmd {
    return func() tea.Msg {
        _, _ = os.Stdout.Write([]byte("\a"))
        return nil
    }
}
```

Bubble Tea uses Ultraviolet for rendering and takes control of stdout. Writing
directly to `os.Stdout` from a Cmd goroutine while Bubble Tea is running is
safe for a single byte — the renderer does not buffer individual raw bytes
written outside its render pass. However, if Ultraviolet provides a first-class
bell API, that should be preferred.

Check `charm.land/bubbletea/v2` and `github.com/charmbracelet/ultraviolet` for
a `tea.Bell` command or `uv.Bell()` function. If one exists, use it. If not,
use the `os.Stdout.Write([]byte("\a"))` approach.

### 6.3 Config gate

The bell is opt-out. Add to `smithers-tui.json` config:

```jsonc
"notifications": {
    "approval_bell": true   // default: true; set false to suppress
}
```

Guard call: `if !cfg.Notifications.DisableApprovalBell { cmds = append(cmds, bellCmd()) }`.

Since `Config` struct changes are out of scope for this ticket per the thin
frontend principle, the initial implementation uses an env-var gate:
`SMITHERS_APPROVAL_BELL=0` to disable.

---

## 7. Action hint: `[a] approve`

### 7.1 What pressing `[a]` should do

In Smithers TUI, `a` in the run dashboard view already means "approve the
selected run's pending gate" (`[a] Approve` in §3.2 of the design doc).

For the toast, `[a] approve` should:
1. Call `smithersClient.ApproveRun(runID)` directly from the `UI.Update` key
   handler, or
2. Navigate to the approvals view (`ctrl+a`) and let the user approve from
   there.

Option 2 is simpler and avoids wiring up approval write path from `UI.Update`
(which currently only reads Smithers state). For v1, the action hint reads
`[a] approve` but pressing `a` navigates to the approvals view — same target
as `ctrl+a`. A future iteration can wire direct inline approval.

### 7.2 Key conflict analysis

`a` is used in `km.Chat.HalfPageDown = key.NewBinding(key.WithKeys("d"))` —
no conflict. `a` is not currently in `KeyMap`. The `Approvals` binding is
`ctrl+a`, not `a`.

Adding a global `a` key that navigates to approvals is safe when the editor
is not focused (same guard used for `d`, `f`, etc. in the chat scroll
bindings).

---

## 8. Files involved

| File | Change |
|---|---|
| `internal/ui/model/notifications.go` | Upgrade `runEventToToast` for `waiting-approval`: add `fetchApprovalAndToastCmd`, use approval ID for dedup, add gate question to toast body, add `[a] approve` action hint, adjust TTL |
| `internal/ui/model/ui.go` | Handle `approvalFetchedMsg` (new message type); emit bell Cmd on approval toast |
| `internal/ui/model/keys.go` | Add `ViewApprovals` key binding for `a` (global, outside editor) |
| `internal/smithers/client.go` | No changes needed; `ListPendingApprovals` already exists |
| `internal/ui/model/notifications_test.go` | Add tests for enriched approval toast, dedup by approval ID, bell suppression |

---

## 9. Open questions

1. **TypeScript SSE shape**: Does the Smithers server's `status_changed` event
   include `approvalId` and `approvalGate` fields in its JSON payload? If yes,
   Strategy A (extend `RunEvent`) is preferable over the extra fetch. Needs
   verification against `smithers/src/SmithersEvent.ts` or a live server trace.

2. **`tea.Bell` primitive**: Does `charm.land/bubbletea/v2` expose a `tea.Bell`
   command? Check the Bubble Tea v2 changelog and API. If it does, use it
   instead of raw `os.Stdout.Write`.

3. **Direct approval from toast**: The `[a] approve` hint currently navigates
   rather than approves. A future ticket (`notifications-approval-inline`)
   could wire `ApproveRun` directly from the toast key handler, allowing
   < 3-keystroke approval from any view. Leave as a TODO comment.

4. **Config structure**: The bell is gated by env-var in v1. The proper home
   is `config.Notifications.ApprovalBell bool`. Coordinate with the config
   namespace ticket (`platform-config-namespace`) before adding config fields.

5. **Approval TTL**: 15 s is a guess. What is a sensible default? The user
   should not miss an approval, but 15 s may be too long if many approvals
   arrive in succession (the stack cap is 3 toasts). Consider making it
   configurable via `ToastTTL` or hardcode a longer TTL only for Warning-level
   toasts in `ToastManager`.

# Research: notifications-toast-overlays

**Ticket**: notifications-toast-overlays
**Depends on**: eng-in-terminal-toast-component (toast rendering primitive)
**Feature flag**: NOTIFICATIONS_TOAST_OVERLAYS

---

## 1. What this ticket does

`eng-in-terminal-toast-component` builds a self-contained `ToastManager` that
knows how to render a bounded stack of toasts at the bottom-right of any
`uv.Screen` / `uv.Rectangle`.  This ticket wires that component into the global
Bubble Tea loop so that:

1. Any part of the app can fire a `components.ShowToastMsg` and a toast appears
   on top of every view (chat, runs, tickets, etc.).
2. Toasts triggered by Smithers operational events (new approval request, run
   finished, run failed) are generated automatically by subscribing to the SSE
   stream and the pubsub bus.
3. The user can dismiss toasts with a keybinding and opt out in config.

---

## 2. Toast component contract (from dependency)

The component that lands in `internal/ui/components/toast.go` exposes:

```go
// ShowToastMsg — broadcast this on the tea.Program bus to show a toast.
type ShowToastMsg struct {
    Title       string
    Body        string
    ActionHints []ActionHint  // [{Key: "a", Label: "approve"}, ...]
    Level       ToastLevel    // Info | Success | Warning | Error
    TTL         time.Duration // 0 → DefaultToastTTL (5s)
}

// DismissToastMsg — dismiss one toast by ID (generated internally).
type DismissToastMsg struct { ID uint64 }

// ToastManager — imperative-subcomponent pattern, same as dialog.Overlay.
func NewToastManager(st *styles.Styles) *ToastManager
func (m *ToastManager) Update(msg tea.Msg) tea.Cmd   // call in root Update
func (m *ToastManager) Draw(scr uv.Screen, area uv.Rectangle) // call in Draw, after dialogs
func (m *ToastManager) Len() int
func (m *ToastManager) Clear()
```

`DefaultToastTTL = 5 * time.Second`, `MaxVisibleToasts = 3`,
`MaxToastWidth = 48` (columns).

---

## 3. Integration point: `internal/ui/model/ui.go`

### 3.1 The `UI` struct

The root Bubble Tea model is `UI` in `internal/ui/model/ui.go`.  It already
holds:
- `dialog *dialog.Overlay` — the modal dialog stack.
- `notifyBackend notification.Backend` — native OS desktop notification handler.

The `ToastManager` follows the same imperative-subcomponent pattern as
`dialog.Overlay`: it is stored on the struct, its `Update` is called from `UI.Update`,
and its `Draw` is called from `UI.Draw`.

### 3.2 `UI.Draw` render order

`Draw` (line 2113) currently:

1. Renders the active view state (`uiChat`, `uiSmithersView`, `uiLanding`, etc.)
   onto the screen.
2. Renders the status/help bar (`m.status.Draw`).
3. Renders the completions popup.
4. Renders dialogs — **always last** (`m.dialog.Draw(scr, scr.Bounds())`).
5. Returns the cursor position.

Toasts must appear **after dialogs return `nil`** (i.e., no active dialog)
or **before dialog rendering** so a dialog still occludes them.  The correct
order is:

```
... status bar ...
... completions ...
(toasts rendered here — bottom-right, beneath dialogs)
... dialogs — rendered last, highest z-order ...
```

Practically: call `m.toasts.Draw(scr, scr.Bounds())` just before the dialog
block.  If a dialog is open, it visually covers the toast if they overlap in
the bottom-right, which is the desired behavior (dialog has focus).

### 3.3 Key routing in `UI.Update`

`UI.Update` is a single large switch.  Messages handled today include
`pubsub.Event[notify.Notification]`, `pubsub.Event[session.Session]`,
`pubsub.Event[message.Message]`, etc.

Two additions are needed:

1. Forward all messages to `m.toasts.Update(msg)` — it only reacts to
   `ShowToastMsg`, `DismissToastMsg`, and the internal `toastTimedOutMsg`
   tick, so passing all messages is cheap.
2. In the `tea.KeyPressMsg` handler (existing `handleKeyPressMsg`): add a
   branch for the dismiss keybinding that fires `DismissToastMsg` for the
   oldest/topmost visible toast.

---

## 4. Event sources for notification triggers

### 4.1 Smithers SSE run events

`internal/smithers/types_runs.go` defines `RunEvent`:

```go
type RunEvent struct {
    Type        string    // "status_changed", "node_started", "node_finished",
                          // "approval_requested", "chat_message", etc.
    RunID       string
    NodeID      string
    Status      string    // for status_changed: new RunStatus value
    TimestampMs int64
    Seq         int
}
```

Run status values (`RunStatus` in `types_runs.go`):

| Value | Meaning |
|---|---|
| `"running"` | normal execution |
| `"waiting-approval"` | gate paused, needs human action |
| `"waiting-event"` | waiting for an external trigger |
| `"finished"` | successful terminal state |
| `"failed"` | error terminal state |
| `"cancelled"` | user-cancelled terminal state |

Toast triggers from SSE:

| `RunEvent.Type` | Condition | Toast level | Toast content |
|---|---|---|---|
| `"status_changed"` | `Status == "waiting-approval"` | Warning | "Approval needed" / run + gate name |
| `"status_changed"` | `Status == "failed"` | Error | "Run failed" / workflow name + short error |
| `"status_changed"` | `Status == "finished"` | Success | "Run finished" / workflow name |
| `"status_changed"` | `Status == "cancelled"` | Info | "Run cancelled" / workflow name |

These are produced by `smithers.StreamRunEvents`, which returns a
`<-chan interface{}` carrying `RunEventMsg`, `RunEventErrorMsg`,
`RunEventDoneMsg`.  The SSE stream is per-run, so a global SSE subscriber
needs to either poll or listen to a multiplexed global event endpoint.

**Alternative for the global event bus**: the Smithers HTTP server exposes
`GET /v1/events` (a global SSE feed for all runs).  This is simpler for
toast purposes: one SSE connection covers all runs.  If the server is not
available, fall back to polling `GET /v1/runs` at a configurable interval
(default 10s) and comparing state.

### 4.2 Approval requests via pubsub

`internal/ui/model/ui.go` already subscribes to
`pubsub.Event[permission.PermissionRequest]` and calls
`m.sendNotification(...)` (native OS notification) when a permission dialog
opens.  The Smithers approval pathway is separate — it comes from the
HTTP/SSE layer rather than the Go pubsub broker.

However, the approval queue view (`internal/ui/views/approvals.go`) already
has a refresh loop.  We can reuse its polling cadence to detect new approvals
and fire `ShowToastMsg` when the count increases.

### 4.3 Internal pubsub events

The existing `pubsub.Broker[T]` in `internal/pubsub` is generic.  We can
add a `pubsub.Broker[smithers.RunEvent]` field to `app.App` (or to `UI`
directly) and publish run events from the SSE goroutine into it.  This lets
other UI components (approval view, runs dashboard) also subscribe without
owning the SSE connection.

Alternatively, since `UI` is the sole root model, keeping the SSE subscription
on `UI` and converting events to `tea.Cmd` return values (which inject
`ShowToastMsg` into the Bubble Tea bus) is sufficient and simpler.

### 4.4 Agent notifications via `notify.Notification`

The existing `pubsub.Event[notify.Notification]` path (line 513 in `ui.go`)
handles agent-level notifications (permission prompts, tool completions).  We
can intercept these in the same `case` block and also fire `ShowToastMsg`, so
the user sees an in-terminal toast even when the window is focused (the native
backend only fires when unfocused).

---

## 5. Notification priority and deduplication

### 5.1 Priority

Toast levels map to visual urgency:

| Level | Color signal | Use case |
|---|---|---|
| `ToastLevelError` | Red border | Run failed |
| `ToastLevelWarning` | Yellow border | Approval needed, run blocked |
| `ToastLevelSuccess` | Green border | Run finished successfully |
| `ToastLevelInfo` | Muted border | Run cancelled, informational |

`ToastManager` renders up to `MaxVisibleToasts = 3` simultaneously.  When the
cap is reached, the oldest toast is evicted (FIFO).  For the notification
overlay, we should insert high-priority toasts (Error, Warning) at the top of
the visible stack rather than FIFO.  This requires a small extension to the
component — see the plan section.

### 5.2 Deduplication

Without dedup, a long-running approval gate would fire a toast on every SSE
heartbeat.  Dedup rules:

1. **Per-run, per-status deduplication**: track `(runID, status)` pairs that
   have already produced a toast.  On a new SSE event, skip if the pair is
   already in the seen set.  Clear the seen set when the run reaches a
   terminal state.

2. **Approval dedup**: only toast once per approval ID (`Approval.ID`).  Track
   seen approval IDs in a `map[string]struct{}` on the `UI` struct or in a
   dedicated `notificationTracker`.

3. **TTL reset on repeat**: if the same logical notification fires again before
   the toast has dismissed, refresh its TTL rather than adding a duplicate.
   The `ToastManager.add` path can check for an existing toast with the same
   title+body and reset its timer.

---

## 6. Overlay patterns in Bubble Tea apps

### 6.1 Crush's existing pattern (`dialog.Overlay`)

`internal/ui/dialog/dialog.go` defines the `Overlay` struct:

```go
type Overlay struct { dialogs []Dialog }

func (d *Overlay) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
    for _, dialog := range d.dialogs {
        cur = dialog.Draw(scr, area)  // each dialog draws itself onto scr
    }
    return cur
}
```

Each `Dialog` implements `Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor`.
Positioning helpers (`DrawCenter`, `DrawOnboardingCursor`) use
`common.CenterRect` and `common.BottomLeftRect` to place content.

`BottomRightRect` was added to `internal/ui/common/common.go` as part of
`eng-in-terminal-toast-component`:

```go
func BottomRightRect(area uv.Rectangle, width, height int) uv.Rectangle {
    maxX := area.Max.X
    minX := maxX - width
    maxY := area.Max.Y
    minY := maxY - height
    return image.Rect(minX, minY, maxX, maxY)
}
```

### 6.2 `ToastManager.Draw` vs `dialog.Overlay.Draw`

`ToastManager.Draw` uses `BottomRightRect` and iterates toasts
bottom-to-top, stacking them upward.  It writes directly to the provided
`uv.Screen` via `uv.NewStyledString(view).Draw(scr, rect)`.  This is the same
pattern used by all other overlays — no special Bubble Tea plumbing needed.

### 6.3 Z-order

Because `scr` is a retained-mode screen (Ultraviolet), drawing order determines
z-order: later writes appear on top.  The correct paint order:

```
1. Active view (chat / runs / tickets / ...)
2. Status bar
3. Completions popup
4. Toast stack         ← new, drawn here
5. Dialog overlay      ← always last / highest z
```

When a dialog is open, it covers any toasts that happen to overlap in the
corner.  This is acceptable: if the user is in a dialog, they should handle
the dialog before the toast action.

### 6.4 Cursor passthrough

`dialog.Overlay.Draw` returns a `*tea.Cursor`.  `ToastManager.Draw` does not
(toasts are passive, non-interactive overlays in v1).  The root `Draw` method
returns the cursor from the dialog if a dialog is present, or from the editor
textarea otherwise.  Toasts do not capture focus or cursor position.

---

## 7. User preferences

### 7.1 Existing config

`internal/config/config.go` already has:

```go
DisableNotifications bool `json:"disable_notifications,omitempty"`
```

This controls native OS notifications (`m.shouldSendNotification()`).  We
should **reuse this flag** to also suppress in-terminal toasts, keeping the
user model simple: one setting turns off all notification modalities.

### 7.2 Per-category mute (future consideration)

v1 should not add per-category mute.  If needed in a future iteration:

```jsonc
"notifications": {
  "disable": false,
  "mute_approvals": false,
  "mute_run_finished": false,
  "mute_run_failed": false,
  "toast_ttl_seconds": 5
}
```

For now, a single `DisableNotifications` toggle covers the requirement.

### 7.3 Dismiss keybinding

A single `Esc`-like binding to dismiss the topmost toast.  Since `Esc` is
already used for canceling the editor, canceling agent work, and navigating
back, we should use a distinct key — `ctrl+d` (clear/dismiss) is available
in Smithers TUI context.  Alternatively, `d` in a non-editor focus state
(matching the approval queue `d` for deny is unfortunate; use `n` for
notification dismiss instead).

The binding should only fire when:
- There is at least one active toast.
- The editor is not focused (to avoid hijacking normal typing).

---

## 8. SSE subscription approach

### 8.1 Global event stream

The Smithers HTTP API exposes a global SSE feed at `GET /v1/events` (all
runs, all event types).  This is the primary subscription point for the
notification overlay.

Subscription lifecycle:
1. On `UI.Init`, if `smithersClient.IsServerAvailable()`, start an SSE
   subscriber goroutine.
2. The goroutine reads events and injects them as tea messages via a `tea.Cmd`
   channel pump (standard BubbleTea pattern: `func listenForSSE(ch <-chan interface{}) tea.Cmd`).
3. On `UI.Update`, handle `smithers.RunEventMsg` and translate to
   `ShowToastMsg` based on the event type and dedup state.

### 8.2 Polling fallback

When no server is running (SQLite-only or exec mode), the SSE stream is
unavailable.  In this case:
- Spin up a `tea.Tick`-based poll at 10-second intervals.
- On each tick, call `smithersClient.ListPendingApprovals` and
  `smithersClient.ListRuns(RunFilter{Status: "failed"})`.
- Compare against the last known state to detect new events.
- This is a degraded mode: notification latency is up to 10 seconds, not real-time.

### 8.3 Channel pump pattern

```go
// startSSEListener returns a tea.Cmd that pumps SSE messages into the tea bus.
func startSSEListener(ch <-chan interface{}) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok {
            return sseStreamClosedMsg{}
        }
        return msg // RunEventMsg | RunEventErrorMsg | RunEventDoneMsg
    }
}
```

This follows the standard Bubble Tea pattern for long-running I/O: the Cmd
function blocks until one message arrives, returns it, and the Update handler
re-queues the next pump command.

---

## 9. Files involved

| File | Change |
|---|---|
| `internal/ui/model/ui.go` | Add `toasts *components.ToastManager`; wire `Update` and `Draw`; add SSE subscription init; add event→toast translation logic; add dedup state |
| `internal/ui/model/keys.go` | Add `DismissToast` key binding |
| `internal/ui/components/toast.go` | Dependency (built by `eng-in-terminal-toast-component`) |
| `internal/ui/common/common.go` | `BottomRightRect` (built by dependency) |
| `internal/ui/styles/styles.go` | Toast styles (built by dependency) |
| `internal/smithers/runs.go` | `StreamRunEvents` already exists; may need `StreamAllEvents` for global feed |
| `internal/smithers/types_runs.go` | `RunEventMsg` etc. already defined |
| `internal/config/config.go` | Reuse `DisableNotifications`; optionally add `ToastTTL` |

---

## 10. Open questions

1. **Global SSE endpoint**: Does `GET /v1/events` exist in the Smithers server?
   If not, we need per-run subscriptions keyed to active runs from the runs
   dashboard, or we add the endpoint.  See open question 5 in
   `eng-in-terminal-toast-component` plan.

2. **Approval action hints**: Should the approval toast include `[a] approve /
   [d] deny` action hints?  This requires wiring keypress routing from `UI.Update`
   to call `smithersClient.ApproveApproval` / `DenyApproval` directly from the
   toast handler.  This is high-value but scope-creep for v1.  Recommendation:
   show the hint, navigate to the approvals view on press rather than approving
   inline.

3. **Run event type strings**: The `RunEvent.Type` field values come from the
   Smithers TypeScript server.  The relevant values are assumed to be
   `"status_changed"`, `"node_started"`, `"node_finished"`,
   `"approval_requested"`.  These should be confirmed against
   `smithers/src/SmithersEvent.ts` before implementation.

4. **`DisableNotifications` semantics**: Currently gated only to native OS
   notifications.  Extending to in-terminal toasts may surprise users who
   want native notifications off but still want in-terminal feedback.
   Consider a separate `DisableToastNotifications` flag or a two-level config.

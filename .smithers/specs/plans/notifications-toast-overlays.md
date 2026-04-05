# Implementation Plan: notifications-toast-overlays

**Ticket**: notifications-toast-overlays
**Feature flag**: `NOTIFICATIONS_TOAST_OVERLAYS`
**Depends on**: `eng-in-terminal-toast-component` (must land first)
**Integration point**: `internal/ui/model/ui.go`

---

## Goal

Wire the `ToastManager` from `eng-in-terminal-toast-component` into the root
Bubble Tea model so that:

1. In-terminal toasts overlay every view (chat, runs, any routed view).
2. Smithers operational events — new approval request, run finished, run failed
   — automatically produce toasts via the SSE stream.
3. Users can dismiss toasts with a keybinding.
4. The feature respects `DisableNotifications` and is guard-flagged by
   `NOTIFICATIONS_TOAST_OVERLAYS`.

---

## Steps

### Step 1 — Confirm dependency is merged

Before writing any code, verify `eng-in-terminal-toast-component` has landed:

- `internal/ui/components/toast.go` exports `ToastManager`, `ShowToastMsg`,
  `DismissToastMsg`, `ToastLevel*`, `ActionHint`.
- `internal/ui/common/common.go` exports `BottomRightRect`.
- `internal/ui/styles/styles.go` has `Styles.Toast` sub-struct.

If any of these are absent, block on that ticket.

---

### Step 2 — Add `ToastManager` to the `UI` struct

**File**: `internal/ui/model/ui.go`

Add the field to the `UI` struct immediately after the existing `dialog` and
`notifyBackend` fields (lines ~185-244):

```go
// toasts is the in-terminal toast notification manager.
// Guarded by NOTIFICATIONS_TOAST_OVERLAYS feature flag.
toasts *components.ToastManager
```

In `New(...)` (line ~280), initialize it:

```go
ui := &UI{
    ...
    toasts: components.NewToastManager(com.Styles),
    ...
}
```

Add import: `"github.com/charmbracelet/crush/internal/ui/components"`.

---

### Step 3 — Wire `ToastManager.Update` into `UI.Update`

**File**: `internal/ui/model/ui.go`

At the **top** of `UI.Update`, before the type switch, add:

```go
if cmd := m.toasts.Update(msg); cmd != nil {
    cmds = append(cmds, cmd)
}
```

`ToastManager.Update` only reacts to `ShowToastMsg`, `DismissToastMsg`, and
the internal `toastTimedOutMsg` — all other message types pass through as
no-ops.  Calling it unconditionally on every message is correct and cheap.

---

### Step 4 — Wire `ToastManager.Draw` into `UI.Draw`

**File**: `internal/ui/model/ui.go`

In `UI.Draw` (line ~2113), insert the toast draw call just **before** the
dialog block (line ~2217):

```go
// Draw toast notifications (below dialogs in z-order).
m.toasts.Draw(scr, scr.Bounds())

// This needs to come last to overlay on top of everything.
if m.dialog.HasDialogs() {
    return m.dialog.Draw(scr, scr.Bounds())
}
```

This gives dialogs the highest z-order.  When no dialog is present, toasts
are the topmost layer.

---

### Step 5 — Define notification event types

**File**: `internal/ui/model/notifications.go` (new file)

Create a small file to hold the notification-specific types, dedup state, and
translation logic, keeping `ui.go` from growing further.

```go
package model

import (
    "sync"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/charmbracelet/crush/internal/ui/components"
)

// notificationTracker deduplicates Smithers event → toast mappings.
// The zero value is usable.
type notificationTracker struct {
    mu          sync.Mutex
    seenRunStates map[string]smithers.RunStatus // runID → last toasted status
    seenApprovals map[string]struct{}            // approvalID → seen
}

func newNotificationTracker() *notificationTracker {
    return &notificationTracker{
        seenRunStates: make(map[string]smithers.RunStatus),
        seenApprovals: make(map[string]struct{}),
    }
}

// shouldToastRunStatus returns true if this (runID, status) pair has not
// previously produced a toast. Records the pair on first call.
func (t *notificationTracker) shouldToastRunStatus(runID string, status smithers.RunStatus) bool {
    t.mu.Lock()
    defer t.mu.Unlock()
    if t.seenRunStates[runID] == status {
        return false
    }
    t.seenRunStates[runID] = status
    return true
}

// forgetRun removes a run from the dedup set (called when run reaches terminal state).
func (t *notificationTracker) forgetRun(runID string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    delete(t.seenRunStates, runID)
}

// shouldToastApproval returns true if this approvalID has not been toasted.
func (t *notificationTracker) shouldToastApproval(approvalID string) bool {
    t.mu.Lock()
    defer t.mu.Unlock()
    if _, seen := t.seenApprovals[approvalID]; seen {
        return false
    }
    t.seenApprovals[approvalID] = struct{}{}
    return true
}

// runEventToToast translates a RunEvent into a ShowToastMsg.
// Returns nil if the event should not produce a toast.
func runEventToToast(ev smithers.RunEvent, tracker *notificationTracker) *components.ShowToastMsg {
    if ev.Type != "status_changed" {
        return nil
    }
    status := smithers.RunStatus(ev.Status)
    if !tracker.shouldToastRunStatus(ev.RunID, status) {
        return nil
    }

    shortID := ev.RunID
    if len(shortID) > 8 {
        shortID = shortID[:8]
    }

    switch status {
    case smithers.RunStatusWaitingApproval:
        return &components.ShowToastMsg{
            Title: "Approval needed",
            Body:  shortID + " is waiting for approval",
            Level: components.ToastLevelWarning,
            ActionHints: []components.ActionHint{
                {Key: "ctrl+r", Label: "view runs"},
            },
        }
    case smithers.RunStatusFailed:
        tracker.forgetRun(ev.RunID) // allow re-toast on future failure
        return &components.ShowToastMsg{
            Title: "Run failed",
            Body:  shortID + " encountered an error",
            Level: components.ToastLevelError,
        }
    case smithers.RunStatusFinished:
        tracker.forgetRun(ev.RunID)
        return &components.ShowToastMsg{
            Title: "Run finished",
            Body:  shortID + " completed successfully",
            Level: components.ToastLevelSuccess,
        }
    case smithers.RunStatusCancelled:
        tracker.forgetRun(ev.RunID)
        return &components.ShowToastMsg{
            Title: "Run cancelled",
            Body:  shortID,
            Level: components.ToastLevelInfo,
        }
    }
    return nil
}
```

Add `notificationTracker` field to `UI` struct and initialize it in `New`:

```go
notifTracker *notificationTracker
```

```go
ui := &UI{
    ...
    notifTracker: newNotificationTracker(),
    ...
}
```

---

### Step 6 — Add SSE subscription for run events

**File**: `internal/ui/model/ui.go`

#### 6a. Define internal messages for the SSE pump

Add to the message type block (near `cancelTimerExpiredMsg`):

```go
// sseStreamClosedMsg is sent when the global Smithers SSE stream closes.
sseStreamClosedMsg struct{}

// sseStartMsg requests the SSE listener goroutine to start.
sseStartMsg struct{}
```

#### 6b. SSE listener command

Add a private method to `UI`:

```go
// listenSSE returns a Cmd that reads one message from the SSE channel and
// returns it as a tea.Msg, then re-queues itself.
func listenSSE(ch <-chan interface{}) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok {
            return sseStreamClosedMsg{}
        }
        return msg
    }
}

// startSSESubscription opens the global SSE stream and returns the first pump Cmd.
// Returns nil if the server is unavailable.
func (m *UI) startSSESubscription(ctx context.Context) tea.Cmd {
    // Only attempt if feature is enabled and client has an API URL configured.
    if m.smithersClient == nil {
        return nil
    }
    ch, err := m.smithersClient.StreamAllEvents(ctx)
    if err != nil {
        return nil // server not running — fall through to poll fallback
    }
    m.sseEventCh = ch
    return listenSSE(ch)
}
```

Add `sseEventCh chan interface{}` to the `UI` struct.

#### 6c. Initialize SSE in `UI.Init`

In `UI.Init` (line ~383), add:

```go
if cmd := m.startSSESubscription(context.Background()); cmd != nil {
    cmds = append(cmds, cmd)
}
```

#### 6d. Handle SSE messages in `UI.Update`

In the `switch msg := msg.(type)` block, add:

```go
case smithers.RunEventMsg:
    // Re-queue the SSE listener pump.
    if m.sseEventCh != nil {
        cmds = append(cmds, listenSSE(m.sseEventCh))
    }
    // Translate event to toast if applicable.
    if !m.isNotificationsDisabled() {
        if toast := runEventToToast(msg.Event, m.notifTracker); toast != nil {
            cmds = append(cmds, func() tea.Msg { return *toast })
        }
    }

case smithers.RunEventErrorMsg:
    // Re-queue pump to keep listening even after errors.
    if m.sseEventCh != nil {
        cmds = append(cmds, listenSSE(m.sseEventCh))
    }

case smithers.RunEventDoneMsg:
    // Run stream closed; re-queue pump (channel stays open for other runs on global feed).
    if m.sseEventCh != nil {
        cmds = append(cmds, listenSSE(m.sseEventCh))
    }

case sseStreamClosedMsg:
    // Global SSE stream closed (server restart, etc.).  Schedule reconnect.
    m.sseEventCh = nil
    cmds = append(cmds, tea.Tick(10*time.Second, func(time.Time) tea.Msg {
        return sseStartMsg{}
    }))

case sseStartMsg:
    if cmd := m.startSSESubscription(context.Background()); cmd != nil {
        cmds = append(cmds, cmd)
    }
```

#### 6e. Helper: `isNotificationsDisabled`

```go
func (m *UI) isNotificationsDisabled() bool {
    cfg := m.com.Config()
    return cfg != nil && cfg.Options != nil && cfg.Options.DisableNotifications
}
```

---

### Step 7 — Intercept `permission.PermissionRequest` for in-terminal toast

**File**: `internal/ui/model/ui.go`

The existing handler at line ~658 fires native OS notifications.  Extend it
to also produce an in-terminal toast:

```go
case pubsub.Event[permission.PermissionRequest]:
    if cmd := m.openPermissionsDialog(msg.Payload); cmd != nil {
        cmds = append(cmds, cmd)
    }
    // Native OS notification (existing, unchanged)
    if cmd := m.sendNotification(notification.Notification{...}); cmd != nil {
        cmds = append(cmds, cmd)
    }
    // In-terminal toast (new)
    if !m.isNotificationsDisabled() {
        cmds = append(cmds, func() tea.Msg {
            return components.ShowToastMsg{
                Title: "Permission required",
                Body:  msg.Payload.ToolName,
                Level: components.ToastLevelWarning,
                ActionHints: []components.ActionHint{
                    {Key: "enter", Label: "allow"},
                    {Key: "esc", Label: "deny"},
                },
            }
        })
    }
```

---

### Step 8 — Add dismiss keybinding

**File**: `internal/ui/model/keys.go`

Add to the `GlobalKeyMap` or the existing `KeyMap`:

```go
// DismissToast dismisses the topmost in-terminal toast notification.
DismissToast key.Binding
```

Initialize in `DefaultKeyMap()`:

```go
DismissToast: key.NewBinding(
    key.WithKeys("ctrl+n"),
    key.WithHelp("ctrl+n", "dismiss toast"),
),
```

**File**: `internal/ui/model/ui.go`

In `handleKeyPressMsg`, add a guard near the top (before routing to dialog or
editor):

```go
if key.Matches(msg, m.keyMap.Global.DismissToast) && m.toasts.Len() > 0 {
    cmds = append(cmds, func() tea.Msg {
        return components.DismissToastMsg{ID: m.toasts.FrontID()}
    })
    return m, tea.Batch(cmds...)
}
```

This requires `ToastManager.FrontID() uint64` to be added to the component
(returns the ID of the topmost/newest toast), or we expose `DismissNewest()`.
Add to `internal/ui/components/toast.go`:

```go
// FrontID returns the ID of the newest (bottom-most in visual stack) toast,
// or 0 if the stack is empty.
func (m *ToastManager) FrontID() uint64 {
    if len(m.toasts) == 0 {
        return 0
    }
    return m.toasts[len(m.toasts)-1].id
}
```

---

### Step 9 — Add `StreamAllEvents` to `smithers.Client`

**File**: `internal/smithers/runs.go`

`StreamRunEvents` already handles per-run SSE.  We need a global variant that
connects to `GET /v1/events` (the server-wide event feed):

```go
// StreamAllEvents opens the global SSE stream at GET /v1/events and returns
// a channel carrying RunEventMsg, RunEventErrorMsg, RunEventDoneMsg.
// This is the primary feed for the notification overlay.
func (c *Client) StreamAllEvents(ctx context.Context) (<-chan interface{}, error) {
    if c.apiURL == "" {
        return nil, ErrServerUnavailable
    }
    eventURL := c.apiURL + "/v1/events"
    // ... same SSE consumer logic as StreamRunEvents but without a runID filter
}
```

If the Smithers HTTP server does not yet expose `GET /v1/events`, fall back to
opening `GET /v1/runs` on a ticker and converting state diffs to synthetic
`RunEventMsg` values — but flag this as a degraded mode with a `slog.Warn`.

---

### Step 10 — Feature flag guard

The feature flag `NOTIFICATIONS_TOAST_OVERLAYS` should gate the entire
initialization path.  Add to `UI.Init` and `New`:

```go
if !featureEnabled("NOTIFICATIONS_TOAST_OVERLAYS") {
    m.toasts = nil // component is no-op when nil
}
```

Guard all call sites with a nil check:

```go
if m.toasts != nil {
    if cmd := m.toasts.Update(msg); cmd != nil {
        cmds = append(cmds, cmd)
    }
}
```

Alternatively, use a no-op `ToastManager` pattern so nil checks are not
needed everywhere.  The simpler nil-check approach is fine for v1.

For now, the feature flag is an env-var check:

```go
func featureEnabled(name string) bool {
    return os.Getenv(name) != "" && os.Getenv(name) != "0" && os.Getenv(name) != "false"
}
```

---

## File plan

| File | Change |
|---|---|
| `internal/ui/model/ui.go` | Add `toasts`, `sseEventCh`, `notifTracker` fields; wire `Update` and `Draw`; handle SSE messages; intercept `PermissionRequest` for in-terminal toast; add feature-flag guard |
| `internal/ui/model/notifications.go` | New file: `notificationTracker`, `runEventToToast` |
| `internal/ui/model/keys.go` | Add `DismissToast` binding |
| `internal/ui/components/toast.go` | Add `FrontID()` method (minor addition to dependency) |
| `internal/smithers/runs.go` | Add `StreamAllEvents` method |
| `internal/ui/model/ui_test.go` | Unit tests for toast wire-up (see testing strategy) |
| `tests/e2e/toast_overlay_e2e_test.go` | E2E test for overlay rendering across views |

---

## Testing strategy

### Unit tests (`internal/ui/model/ui_test.go`)

1. **Toast wire-up test**: Construct a minimal `UI` with a mock `ToastManager`.
   Send `ShowToastMsg` through `Update`. Assert `Len() == 1`.

2. **SSE event → toast translation** (in `notifications_test.go`):
   - `runEventToToast` with `status_changed` + `waiting-approval` → `ToastLevelWarning`.
   - `runEventToToast` with `status_changed` + `failed` → `ToastLevelError`.
   - `runEventToToast` with `status_changed` + `finished` → `ToastLevelSuccess`.
   - Duplicate call with same (runID, status) → nil (dedup).
   - Two calls with different statuses → both produce toasts.

3. **Dedup tests** (`notificationTracker`):
   - `shouldToastRunStatus` — idempotent for same pair.
   - `forgetRun` — allows re-toast after terminal state.
   - `shouldToastApproval` — idempotent for same approvalID.

4. **Dismiss keybinding test**:
   Send a `ShowToastMsg` then a `ctrl+n` `tea.KeyPressMsg`.
   Assert `toasts.Len() == 0`.

### E2E tests (`tests/e2e/toast_overlay_e2e_test.go`)

Using the TUI test harness from `eng-in-terminal-toast-component`:

1. **Overlay across views**: Start TUI with `NOTIFICATIONS_TOAST_OVERLAYS=1`.
   Send `ShowToastMsg` (via env-gated test hook).  Navigate to `/runs` view.
   Assert toast is still visible.  Navigate back to chat.  Assert toast still
   visible.  Wait for TTL.  Assert toast is gone.

2. **Dismiss keybinding**: Show toast.  Send `ctrl+n`.  Assert toast gone
   before TTL.

3. **Max stack**: Show 4 toasts rapidly.  Assert only 3 visible (FIFO eviction
   of oldest).

4. **Feature flag off**: Start without `NOTIFICATIONS_TOAST_OVERLAYS`.  Fire
   SSE `RunEventMsg` with `status_changed/failed`.  Assert no toast appears.

5. **DisableNotifications config**: Set `"disable_notifications": true` in
   config.  Fire SSE event.  Assert no toast appears.

### VHS recording

Add `tests/vhs/toast_overlay_happy_path.tape`:

```
Set Width 120
Set Height 30
Type "NOTIFICATIONS_TOAST_OVERLAYS=1 go run ."
# ... trigger toast via test hook, show overlay appearing bottom-right ...
Sleep 2s
Screenshot toast_overlay_appears.png
# ... wait for auto-dismiss ...
Sleep 6s
Screenshot toast_overlay_dismissed.png
```

---

## Validation commands

```bash
task fmt
go test ./internal/ui/model/... -run Toast -count=1
go test ./internal/ui/model/... -run Notification -count=1
go test ./internal/smithers/... -run Stream -count=1
go test ./tests/e2e/... -run TestToastOverlay -count=1
NOTIFICATIONS_TOAST_OVERLAYS=1 go run .
# Manual: verify toast appears bottom-right, does not block typing, dismisses
```

---

## Open questions

1. **`GET /v1/events` endpoint**: Does the Smithers HTTP server expose a global
   SSE feed, or only per-run feeds (`GET /v1/runs/:id/events`)?  If not, we
   need to: (a) add it to the server, or (b) multiplex per-run SSE connections
   keyed to the active run list (polling runs list + subscribing per run).
   This is a blocking dependency — open question must be resolved before Step 9.

2. **`RunEvent.Type` values**: The SSE event type strings are TypeScript-side
   constants.  Confirm the exact strings from `smithers/src/SmithersEvent.ts`
   before finalizing the `runEventToToast` switch statement.  Likely values:
   `"status_changed"`, `"node_started"`, `"node_finished"`,
   `"approval_requested"` — but these must be verified.

3. **Toast dismiss keybinding conflict**: `ctrl+n` may conflict with Crush's
   existing bindings.  Check `internal/ui/model/keys.go` for conflicts.
   Alternative: `ctrl+shift+d`, but terminal support varies.  A safe fallback
   is `alt+d`.

4. **Approval action hint navigation**: Should pressing `ctrl+r` inside a
   toast navigate to the runs/approvals view?  This requires routing a key
   event back into the UI while a toast is visible — non-trivial.  For v1,
   the action hints are display-only; pressing the key navigates regardless of
   whether a toast is showing.

5. **`DisableNotifications` scope**: Whether to reuse the existing flag to
   suppress in-terminal toasts (proposed), or introduce a separate
   `disable_toast_notifications` flag.  The concern is that users disabling
   native OS notifications (focus-based) may still want in-terminal feedback.
   Recommendation: separate flags, but default `disable_toast_notifications`
   to `false` so behavior is unchanged unless opted out.

6. **Feature flag mechanism**: The env-var approach is provisional.  A proper
   feature flag system (perhaps `config.Options.FeatureFlags []string`) would
   be cleaner.  For now, env-var is acceptable.

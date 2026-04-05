# Implementation Plan: notifications-approval-requests

**Ticket**: notifications-approval-requests
**Feature flag**: `NOTIFICATIONS_APPROVAL_REQUESTS`
**Depends on**: `notifications-toast-overlays` (must be merged first)
**Integration points**: `internal/ui/model/notifications.go`, `internal/ui/model/ui.go`, `internal/ui/model/keys.go`

---

## Goal

Enrich the approval-gate toast with: gate question from `Approval.Gate`,
per-approval-ID deduplication, `[a] approve` action hint, and an opt-out
terminal bell.

---

## Steps

### Step 1 — Confirm dependency is merged

Before writing any code verify `notifications-toast-overlays` has landed:

- `internal/ui/model/notifications.go` exports `notificationTracker`,
  `runEventToToast`, `shouldToastApproval`.
- `internal/ui/model/ui.go` has `toasts *components.ToastManager`,
  `notifTracker *notificationTracker`, `sseEventCh`.
- `internal/ui/model/keys.go` defines `DismissToast` (`alt+d`) and
  `Approvals` (`ctrl+a`).

If any are absent, block on `notifications-toast-overlays`.

---

### Step 2 — Add `approvalFetchedMsg` and `smithersClient` interface

**File**: `internal/ui/model/notifications.go`

At the top of the file, after the existing imports, add the internal message
type and the minimal interface used by the async fetch command:

```go
// approvalFetchedMsg is an internal tea.Msg returned by fetchApprovalAndToastCmd.
// Approval is nil if no matching pending approval was found or the fetch failed.
type approvalFetchedMsg struct {
    RunID    string
    Approval *smithers.Approval
    Err      error
}

// smithersClient is the subset of smithers.Client used by notification helpers.
// Scoped to keep notification logic testable without a live HTTP server.
type smithersClient interface {
    ListPendingApprovals(ctx context.Context) ([]smithers.Approval, error)
}
```

Add `"context"` to the import block.

---

### Step 3 — Add `fetchApprovalAndToastCmd`

**File**: `internal/ui/model/notifications.go`

```go
// fetchApprovalAndToastCmd returns a tea.Cmd that fetches the pending
// approval for runID from the Smithers client and returns an approvalFetchedMsg.
// Used when a waiting-approval status event arrives, to enrich the toast with
// the gate question and a per-approval dedup key.
func fetchApprovalAndToastCmd(ctx context.Context, runID string, client smithersClient) tea.Cmd {
    return func() tea.Msg {
        approvals, err := client.ListPendingApprovals(ctx)
        if err != nil {
            return approvalFetchedMsg{RunID: runID, Err: err}
        }
        for _, a := range approvals {
            if a.RunID == runID && a.Status == "pending" {
                aa := a
                return approvalFetchedMsg{RunID: runID, Approval: &aa}
            }
        }
        return approvalFetchedMsg{RunID: runID} // no match; Approval stays nil
    }
}
```

---

### Step 4 — Add `approvalEventToToast` and `workflowBaseName`

**File**: `internal/ui/model/notifications.go`

Add after `fetchApprovalAndToastCmd`:

```go
// approvalEventToToast builds a ShowToastMsg from a fetched Approval.
// If approval is nil (fetch failed or no pending approval found for the run),
// returns a fallback toast using the short run ID.
// Returns nil if the approval has already been toasted (dedup).
func approvalEventToToast(runID string, approval *smithers.Approval, tracker *notificationTracker) *components.ShowToastMsg {
    shortID := runID
    if len(shortID) > 8 {
        shortID = shortID[:8]
    }

    // Dedup: prefer per-approval-ID; fall back to per-(runID,status).
    if approval != nil {
        if !tracker.shouldToastApproval(approval.ID) {
            return nil
        }
    } else {
        if !tracker.shouldToastRunStatus(runID, smithers.RunStatusWaitingApproval) {
            return nil
        }
    }

    var body string
    if approval != nil && approval.Gate != "" {
        body = approval.Gate
        if approval.WorkflowPath != "" {
            body += "\nrun: " + shortID + " · " + workflowBaseName(approval.WorkflowPath)
        }
    } else {
        body = shortID + " is waiting for approval"
    }

    return &components.ShowToastMsg{
        Title: "Approval needed",
        Body:  body,
        Level: components.ToastLevelWarning,
        TTL:   15 * time.Second,
        ActionHints: []components.ActionHint{
            {Key: "a", Label: "approve"},
            {Key: "ctrl+a", Label: "view approvals"},
        },
    }
}

// workflowBaseName returns a short display name from a workflow file path.
// ".smithers/workflows/deploy-staging.tsx" → "deploy-staging"
func workflowBaseName(path string) string {
    base := filepath.Base(path)
    if ext := filepath.Ext(base); ext != "" {
        base = base[:len(base)-len(ext)]
    }
    return base
}
```

Add `"path/filepath"` and `"time"` to the import block if not already present.

---

### Step 5 — Change `runEventToToast` waiting-approval case to no-op

**File**: `internal/ui/model/notifications.go`

In `runEventToToast`, change:

```go
// Before (returns a basic toast):
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

To:

```go
// After (async path owns approval toasts):
case smithers.RunStatusWaitingApproval:
    // Approval toasts are handled asynchronously via fetchApprovalAndToastCmd
    // to include the gate question. The caller (UI.Update) emits the Cmd.
    return nil
```

This change means `runEventToToast` is now a pure no-op for the
`waiting-approval` status. The caller must detect this status separately.

---

### Step 6 — Update `UI.Update` to branch on `waiting-approval`

**File**: `internal/ui/model/ui.go`

Find the existing `case smithers.RunEventMsg:` handler (added by
`notifications-toast-overlays`). Change the inner notification block from:

```go
if !m.isNotificationsDisabled() {
    if toast := runEventToToast(msg.Event, m.notifTracker); toast != nil {
        cmds = append(cmds, func() tea.Msg { return *toast })
    }
}
```

To:

```go
if !m.isNotificationsDisabled() {
    ev := msg.Event
    if ev.Type == "status_changed" &&
        smithers.RunStatus(ev.Status) == smithers.RunStatusWaitingApproval {
        // Async path: fetch approval detail to include gate question.
        cmds = append(cmds, fetchApprovalAndToastCmd(
            context.Background(), ev.RunID, m.smithersClient,
        ))
    } else {
        if toast := runEventToToast(ev, m.notifTracker); toast != nil {
            cmds = append(cmds, func() tea.Msg { return *toast })
        }
    }
}
```

---

### Step 7 — Handle `approvalFetchedMsg` in `UI.Update`

**File**: `internal/ui/model/ui.go`

In the `switch msg := msg.(type)` block, add a new case after
`sseStartMsg`:

```go
case approvalFetchedMsg:
    if !m.isNotificationsDisabled() {
        if toast := approvalEventToToast(msg.RunID, msg.Approval, m.notifTracker); toast != nil {
            cmds = append(cmds, func() tea.Msg { return *toast })
            if approvalBellEnabled() {
                cmds = append(cmds, bellCmd())
            }
        }
    }
```

---

### Step 8 — Add `bellCmd` and `approvalBellEnabled`

**File**: `internal/ui/model/ui.go`

Add near the other small helper functions (e.g. near `isNotificationsDisabled`):

```go
// bellCmd writes the BEL character (ASCII 0x07) to stdout, triggering an
// audible beep or visual bell in terminals that support it.
func bellCmd() tea.Cmd {
    return func() tea.Msg {
        _, _ = os.Stdout.Write([]byte("\a"))
        return nil
    }
}

// approvalBellEnabled returns true unless the SMITHERS_APPROVAL_BELL
// environment variable is set to "0" or "false".
func approvalBellEnabled() bool {
    v := os.Getenv("SMITHERS_APPROVAL_BELL")
    return v != "0" && v != "false"
}
```

Add `"os"` to the import block if not already imported.

---

### Step 9 — Add `ViewApprovalsShort` key binding

**File**: `internal/ui/model/keys.go`

Add to the `KeyMap` struct (after `DismissToast`):

```go
// ViewApprovalsShort is a bare 'a' shortcut that navigates to the approvals
// view when the editor is not focused. Mirrors the [a] hint shown in approval
// toasts.
ViewApprovalsShort key.Binding
```

Initialize in `DefaultKeyMap()` (after the `DismissToast` binding):

```go
km.ViewApprovalsShort = key.NewBinding(
    key.WithKeys("a"),
    key.WithHelp("a", "approvals"),
)
```

**File**: `internal/ui/model/ui.go`

In `handleKeyPressMsg`, add a guard before editor-focused key routing:

```go
// Navigate to approvals view via bare 'a' (mirrors [a] toast hint).
if key.Matches(msg, m.keyMap.ViewApprovalsShort) &&
    m.focusState != uiFocusEditor {
    cmds = append(cmds, m.navigateToView("approvals"))
    return m, tea.Batch(cmds...)
}
```

Adjust `"approvals"` to match whatever string the view router accepts (check
`internal/ui/views/` for the exact view identifier).

---

### Step 10 — Write unit tests

**File**: `internal/ui/model/notifications_test.go`

Add the following test functions to the existing file:

```
TestApprovalEventToToast_WithGate
TestApprovalEventToToast_FallbackOnNilApproval
TestApprovalEventToToast_DedupByApprovalID
TestApprovalEventToToast_DifferentIDsSameRun
TestApprovalEventToToast_EmptyGateFallback
TestApprovalEventToToast_WorkflowBaseNameExtraction
TestFetchApprovalAndToastCmd_MatchesRunID
TestFetchApprovalAndToastCmd_NoMatchReturnsNilApproval
TestFetchApprovalAndToastCmd_ErrorReturnsErr
TestWorkflowBaseName
```

Full test bodies are specified in the engineering spec
(`engineering/notifications-approval-requests.md`, §9.1).

Each test follows the `t.Parallel()` convention established in the file.

---

## File plan

| File | Change type | Notes |
|---|---|---|
| `internal/ui/model/notifications.go` | Modify + extend | Add `approvalFetchedMsg`, `smithersClient`, `fetchApprovalAndToastCmd`, `approvalEventToToast`, `workflowBaseName`; no-op `waiting-approval` in `runEventToToast` |
| `internal/ui/model/ui.go` | Modify | Branch `RunEventMsg` on `waiting-approval`; add `approvalFetchedMsg` case; add `bellCmd`, `approvalBellEnabled` |
| `internal/ui/model/keys.go` | Modify | Add `ViewApprovalsShort` binding (`a`) |
| `internal/ui/model/notifications_test.go` | Extend | 10 new test functions |

No changes to `internal/smithers/`, `internal/ui/components/toast.go`, or any
design documents.

---

## Validation commands

```bash
task fmt
go test ./internal/ui/model/... -run TestApproval -count=1 -v
go test ./internal/ui/model/... -run TestWorkflowBaseName -count=1 -v
go test ./internal/ui/model/... -run TestFetchApproval -count=1 -v
go test ./internal/ui/model/... -count=1
# Manual: NOTIFICATIONS_APPROVAL_REQUESTS=1 go run .
# Trigger a waiting-approval run event via a test fixture or live Smithers run.
# Assert: toast appears with gate question, [a] approve hint visible.
# Assert: pressing a navigates to approvals view.
# Assert: SMITHERS_APPROVAL_BELL=0 suppresses BEL.
```

---

## Open questions (carry-forward from research)

1. **SSE wire format**: Does the Smithers server embed `approvalId` /
   `approvalGate` in the `status_changed` SSE frame? If yes, extend `RunEvent`
   and use Strategy A (inline, no extra fetch). Verify before shipping.

2. **`tea.Bell` primitive**: Check `charm.land/bubbletea/v2` for a first-class
   `tea.Bell()` command. If it exists, replace the `os.Stdout.Write([]byte("\a"))`
   approach.

3. **Direct inline approval**: The `[a] approve` hint navigates to the
   approvals view in v1. A follow-on ticket (`notifications-approval-inline`)
   could wire `smithersClient.ApproveGate(approvalID)` directly from the
   toast key handler for < 3-keystroke approval. Leave a `// TODO: inline
   approval` comment at the key handler call site.

4. **TTL of 15 s**: Verify this is appropriate with real workflows. If approval
   toasts stack up (3 simultaneous gates), oldest toast is evicted by
   `MaxVisibleToasts = 3`. Consider extending `MaxVisibleToasts` for approval
   toasts specifically, or raising the cap in a future ticket.

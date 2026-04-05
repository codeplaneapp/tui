# Engineering Spec: notifications-approval-requests

**Ticket**: notifications-approval-requests
**Feature flag**: `NOTIFICATIONS_APPROVAL_REQUESTS`
**Depends on**: `notifications-toast-overlays` (must be merged first)
**Phase**: P4 — Live Chat + Hijack

---

## Goal

Upgrade the existing `waiting-approval` toast (delivered by
`notifications-toast-overlays`) to include the gate question, correct
per-approval-ID deduplication, an actionable `[a] approve` hint, and an
optional terminal bell alert.

---

## Acceptance criteria

1. When an `ApprovalRequested` SSE event is received (via `status_changed` +
   `waiting-approval` status), a toast appears with the gate question as the
   body.
2. Toast includes `[a] approve` and `[ctrl+a] view approvals` action hints.
3. If gate question is unavailable (fetch failed), falls back to the existing
   body text (`{shortID} is waiting for approval`).
4. Each unique approval ID toasts exactly once; a second gate on the same run
   produces a second toast.
5. Bell/BEL character is written to stdout on each new approval toast unless
   suppressed by `SMITHERS_APPROVAL_BELL=0`.
6. Pressing `a` (global, outside editor focus) navigates to the approvals view.
7. All new paths are covered by unit tests; dedup logic is fully tested.

---

## 1. New message type: `approvalFetchedMsg`

**File**: `internal/ui/model/notifications.go`

The `waiting-approval` path becomes asynchronous: `runEventToToast` returns
`nil` for this status and instead returns a command that fetches the pending
approval, then returns a message carrying the fetched data.

```go
// approvalFetchedMsg carries the result of fetching a pending approval
// after receiving a waiting-approval status event.
type approvalFetchedMsg struct {
    RunID    string
    Approval *smithers.Approval // nil if fetch failed
    Err      error
}
```

---

## 2. `fetchApprovalAndToastCmd`

**File**: `internal/ui/model/notifications.go`

```go
// fetchApprovalAndToastCmd returns a tea.Cmd that fetches the pending
// approval for runID and returns an approvalFetchedMsg.
func fetchApprovalAndToastCmd(
    ctx context.Context,
    runID string,
    client smithersClient,
) tea.Cmd {
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
        // No matching pending approval found — return msg with nil Approval.
        return approvalFetchedMsg{RunID: runID}
    }
}
```

### 2.1 `smithersClient` interface

To keep `notifications.go` testable without a full HTTP server, add a minimal
interface:

```go
// smithersClient is the subset of smithers.Client used by notification helpers.
type smithersClient interface {
    ListPendingApprovals(ctx context.Context) ([]smithers.Approval, error)
}
```

`*smithers.Client` already satisfies this interface since `ListPendingApprovals`
exists in `client.go`.

---

## 3. Update `runEventToToast`

**File**: `internal/ui/model/notifications.go`

Change the `RunStatusWaitingApproval` case to return `nil` (the caller will
emit a Cmd instead):

```go
case smithers.RunStatusWaitingApproval:
    // Approval toasts are handled asynchronously via fetchApprovalAndToastCmd
    // to include gate context. Return nil here; the caller emits the Cmd.
    return nil
```

The caller (`UI.Update`) handles the transition.

---

## 4. `approvalEventToToast`

**File**: `internal/ui/model/notifications.go`

A separate function converts a fetched `Approval` into the enriched toast:

```go
// approvalEventToToast builds a ShowToastMsg from a fetched Approval.
// If approval is nil (fetch failed or no match), returns a fallback toast
// using shortRunID.
func approvalEventToToast(
    runID string,
    approval *smithers.Approval,
    tracker *notificationTracker,
) *components.ShowToastMsg {
    shortID := runID
    if len(shortID) > 8 {
        shortID = shortID[:8]
    }

    // Dedup by approval ID when available; fall back to run-level dedup.
    if approval != nil {
        if !tracker.shouldToastApproval(approval.ID) {
            return nil
        }
    } else {
        if !tracker.shouldToastRunStatus(runID, smithers.RunStatusWaitingApproval) {
            return nil
        }
    }

    // Build body: gate question + run context.
    var body string
    if approval != nil && approval.Gate != "" {
        body = approval.Gate
        if approval.WorkflowPath != "" {
            // Extract just the filename without path/ext for brevity.
            wf := workflowBaseName(approval.WorkflowPath)
            body += "\nrun: " + shortID + " · " + wf
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

// workflowBaseName extracts a short display name from a workflow path.
// ".smithers/workflows/deploy-staging.tsx" → "deploy-staging"
func workflowBaseName(path string) string {
    base := filepath.Base(path)
    if ext := filepath.Ext(base); ext != "" {
        base = base[:len(base)-len(ext)]
    }
    return base
}
```

---

## 5. Wire into `UI.Update`

**File**: `internal/ui/model/ui.go`

### 5.1 Modify `RunEventMsg` handler

In the existing `case smithers.RunEventMsg:` block (added by
`notifications-toast-overlays`), intercept `waiting-approval` before calling
`runEventToToast`:

```go
case smithers.RunEventMsg:
    // Re-queue the SSE pump.
    if m.sseEventCh != nil {
        cmds = append(cmds, listenSSE(m.sseEventCh))
    }

    if !m.isNotificationsDisabled() {
        ev := msg.Event
        // Approval requests: fetch gate context asynchronously.
        if smithers.RunStatus(ev.Status) == smithers.RunStatusWaitingApproval &&
            ev.Type == "status_changed" {
            cmds = append(cmds, fetchApprovalAndToastCmd(
                context.Background(), ev.RunID, m.smithersClient,
            ))
        } else {
            // All other status changes go through the synchronous path.
            if toast := runEventToToast(ev, m.notifTracker); toast != nil {
                cmds = append(cmds, func() tea.Msg { return *toast })
            }
        }
    }
```

### 5.2 Handle `approvalFetchedMsg`

Add a new case to the `switch msg := msg.(type)` block:

```go
case approvalFetchedMsg:
    if m.isNotificationsDisabled() {
        break
    }
    if toast := approvalEventToToast(msg.RunID, msg.Approval, m.notifTracker); toast != nil {
        cmds = append(cmds, func() tea.Msg { return *toast })
        // Optional bell alert.
        if approvalBellEnabled() {
            cmds = append(cmds, bellCmd())
        }
    }
```

### 5.3 `bellCmd`

```go
// bellCmd writes the BEL character to stdout, producing an audible or
// visual terminal alert. Used for approval request notifications.
func bellCmd() tea.Cmd {
    return func() tea.Msg {
        _, _ = os.Stdout.Write([]byte("\a"))
        return nil
    }
}

// approvalBellEnabled returns true unless SMITHERS_APPROVAL_BELL=0.
func approvalBellEnabled() bool {
    v := os.Getenv("SMITHERS_APPROVAL_BELL")
    return v != "0" && v != "false"
}
```

---

## 6. `ViewApprovals` key binding

**File**: `internal/ui/model/keys.go`

The existing `Approvals` binding (`ctrl+a`) navigates to approvals. The toast
also shows `[a] approve`. Add a global `a` key binding that navigates to the
approvals view when the editor is not focused:

```go
// In KeyMap struct — no new field needed. The toast hint [a] approve
// relies on the existing Approvals (ctrl+a) navigation. However, to
// support pressing bare `a` as a shortcut outside the editor, add:

ViewApprovalsShort key.Binding
```

Initialize in `DefaultKeyMap()`:

```go
km.ViewApprovalsShort = key.NewBinding(
    key.WithKeys("a"),
    key.WithHelp("a", "approvals"),
)
```

Guard in `handleKeyPressMsg` (only when editor not focused and no dialog
active):

```go
if key.Matches(msg, m.keyMap.ViewApprovalsShort) && m.focusState != uiFocusEditor {
    // Navigate to approvals view.
    cmds = append(cmds, m.navigateToApprovals())
    return m, tea.Batch(cmds...)
}
```

**Key conflict check** (from `keys.go`):
- `Chat.HalfPageDown` = `d` — no conflict.
- `Chat.Copy` = `c`, `y`, `C`, `Y` — no conflict.
- `a` is not currently bound anywhere in `KeyMap`.
- When the editor is focused, `a` is normal text input — the guard
  `m.focusState != uiFocusEditor` prevents the conflict.

---

## 7. Changes to `runEventToToast`

The `waiting-approval` case in `runEventToToast` is changed to a no-op
(`return nil`) because the async fetch path now owns that status. All other
cases (`failed`, `finished`, `cancelled`) remain unchanged.

The function signature does not change; the caller detects the
`waiting-approval` status before calling `runEventToToast` and routes to
`fetchApprovalAndToastCmd` instead.

---

## 8. Struct / interface additions summary

| Symbol | File | Description |
|---|---|---|
| `approvalFetchedMsg` | `notifications.go` | Tea message carrying fetched `*Approval` |
| `smithersClient` | `notifications.go` | Interface scoping `ListPendingApprovals` |
| `fetchApprovalAndToastCmd` | `notifications.go` | Cmd that fetches approval by run ID |
| `approvalEventToToast` | `notifications.go` | Builds enriched `ShowToastMsg` from `*Approval` |
| `workflowBaseName` | `notifications.go` | Extracts display name from workflow path |
| `bellCmd` | `ui.go` | Cmd that writes `\a` to stdout |
| `approvalBellEnabled` | `ui.go` | Checks `SMITHERS_APPROVAL_BELL` env |
| `KeyMap.ViewApprovalsShort` | `keys.go` | Bare `a` binding for approval navigation |

No changes to `internal/smithers/` — the existing `ListPendingApprovals` and
`Approval` struct are used as-is.

---

## 9. Testing strategy

### 9.1 Unit tests — `internal/ui/model/notifications_test.go`

Add to the existing test file:

```
TestApprovalEventToToast_WithGate
    Input: Approval{ID: "appr-1", Gate: "Deploy to staging?", WorkflowPath: "deploy.tsx"}
    Assert: toast.Body contains "Deploy to staging?"
    Assert: toast.Body contains "deploy"
    Assert: toast.ActionHints contains {Key: "a", Label: "approve"}
    Assert: toast.ActionHints contains {Key: "ctrl+a", Label: "view approvals"}
    Assert: toast.Level == ToastLevelWarning
    Assert: toast.TTL == 15*time.Second

TestApprovalEventToToast_FallbackOnNilApproval
    Input: nil approval, runID "run-abc12345"
    Assert: toast.Body == "run-abc1 is waiting for approval"

TestApprovalEventToToast_DedupByApprovalID
    First call with Approval{ID: "appr-1"} → toast returned
    Second call with same ID → nil

TestApprovalEventToToast_DifferentIDsSameRun
    Call with Approval{ID: "appr-1", RunID: "run-1"} → toast
    Call with Approval{ID: "appr-2", RunID: "run-1"} → toast (different ID)

TestApprovalEventToToast_EmptyGateFallback
    Approval{ID: "appr-1", Gate: ""} → falls back to shortID body

TestApprovalEventToToast_WorkflowBaseNameExtraction
    WorkflowPath: ".smithers/workflows/deploy-staging.tsx"
    Assert body contains "deploy-staging" not the full path

TestFetchApprovalAndToastCmd_MatchesRunID
    Mock client returns [Approval{RunID:"run-1", Status:"pending"}, Approval{RunID:"run-2"}]
    fetchApprovalAndToastCmd("run-1", mockClient) → approvalFetchedMsg{RunID:"run-1", Approval: &a}
    Assert approval.RunID == "run-1"

TestFetchApprovalAndToastCmd_NoMatchReturnsNilApproval
    Mock client returns []Approval (empty)
    Result: approvalFetchedMsg{Approval: nil, Err: nil}

TestFetchApprovalAndToastCmd_ErrorReturnsErr
    Mock client returns error
    Result: approvalFetchedMsg{Err: <non-nil>}

TestWorkflowBaseName
    ".smithers/workflows/deploy.tsx"  → "deploy"
    "deploy-staging.tsx"              → "deploy-staging"
    "/abs/path/to/ci-checks.workflow" → "ci-checks"
    ""                                → ""
```

### 9.2 Integration test notes

The `UI.Update` path for `approvalFetchedMsg` requires a more integrated test.
Add a test in `ui_test.go` that:
1. Constructs a `UI` with a mock `smithersClient`.
2. Sends a `smithers.RunEventMsg` with `status_changed/waiting-approval`.
3. Executes the returned Cmd (which calls the mock and returns
   `approvalFetchedMsg`).
4. Sends the resulting `approvalFetchedMsg` to `Update`.
5. Asserts `m.toasts.Len() == 1`.

---

## 10. File change summary

| File | Nature of change |
|---|---|
| `internal/ui/model/notifications.go` | Add `approvalFetchedMsg`, `smithersClient` interface, `fetchApprovalAndToastCmd`, `approvalEventToToast`, `workflowBaseName`; modify `runEventToToast` (no-op for `waiting-approval`) |
| `internal/ui/model/ui.go` | Update `RunEventMsg` handler to branch on `waiting-approval`; add `approvalFetchedMsg` handler; add `bellCmd`; add `approvalBellEnabled` |
| `internal/ui/model/keys.go` | Add `ViewApprovalsShort key.Binding` (`a`); initialize in `DefaultKeyMap` |
| `internal/ui/model/notifications_test.go` | Add 9 new test functions covering enriched toast, dedup, fallback, bell, workflow name extraction |

No changes to `internal/smithers/` or `internal/ui/components/`.

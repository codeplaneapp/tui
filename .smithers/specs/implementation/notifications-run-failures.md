# Implementation Summary: notifications-run-failures

**Status**: Shipped (feature already implemented; tests and summary added)
**Date**: 2026-04-05
**Ticket**: notifications-run-failures

---

## Verdict: Already Fully Implemented

The feature was already complete in `internal/ui/model/notifications.go`. No new production code was required.

---

## How It Works

`runEventToToast` in `/Users/williamcory/crush/internal/ui/model/notifications.go` handles the `RunStatusFailed` case:

```go
case smithers.RunStatusFailed:
    tracker.forgetRun(ev.RunID)
    return &components.ShowToastMsg{
        Title: "Run failed",
        Body:  shortID + " encountered an error",
        Level: components.ToastLevelError,
    }
```

- **Trigger**: Any `status_changed` SSE event where `Status == "failed"`
- **Level**: `ToastLevelError` (red styling via Lip Gloss)
- **Title**: "Run failed"
- **Body**: First 8 characters of the run ID + " encountered an error"
- **TTL**: Inherits `DefaultToastTTL` (5 seconds) from `ToastManager`
- **Dedup + Re-toast**: `notificationTracker` prevents duplicate toasts for the same run+status. Crucially, `forgetRun` is called after a failure toast so that a subsequent failure on the same run ID (e.g., a retry) will produce a fresh toast.

The toast flows from SSE stream → `RunEventMsg` → `runEventToToast` → `ShowToastMsg` → `ToastManager`.

---

## Tests Added / Enhanced

File: `/Users/williamcory/crush/internal/ui/model/notifications_test.go`

Enhanced `TestRunEventToToast_FailedProducesErrorToast` to also assert the toast body contains the truncated run ID (acceptance criteria: body includes run ID context).

Existing test `TestRunEventToToast_TerminalStateAllowsReToastAfterForget` already covers the re-toast behavior after `forgetRun`.

All pre-existing tests continue to pass: `go test ./internal/ui/model/...` — ok.

---

## Acceptance Criteria Coverage

- "When a `RunFailed` SSE event is received, a failure toast appears." — **Covered** by `RunStatusFailed` case returning `ToastLevelError` toast.

---

## Files Referenced

- `/Users/williamcory/crush/internal/ui/model/notifications.go` — production implementation
- `/Users/williamcory/crush/internal/ui/model/notifications_test.go` — unit tests

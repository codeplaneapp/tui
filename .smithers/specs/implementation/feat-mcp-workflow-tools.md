# Implementation: feat-mcp-workflow-tools

**Ticket**: feat-mcp-workflow-tools
**Feature**: MCP_WORKFLOW_DOCTOR_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

The `workflow_list`, `workflow_run`, and `workflow_up` renderers were already
implemented. This ticket adds the missing `workflow_doctor` renderer, which displays
ESLint/diagnostics-style output: a list of severity-leveled messages with optional
file and line information.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

- Added `"workflow_doctor"` to `smithersToolLabels` ("Workflow Doctor").
- Added `WorkflowDiagnostic` struct: `Level` (string), `Message` (string),
  `File` (string, optional), `Line` (int, optional).
- Added `renderWorkflowDoctorOutput` method:
  - Parses bare array or `{"data":[...]}` envelope.
  - Renders each diagnostic as `<badge> <message> [file:line]`.
  - Badge styling: `error` → `CardDenied` (red), `warn`/`warning` → `StatusApproval`
    (yellow), anything else → `Subtle` (muted).
  - Empty diagnostics list renders "No issues found." in `StatusComplete` style.
  - Falls back to `renderFallback` on parse error.
- Added `styleDiagnosticLevel` helper method for badge styling.
- Added `renderBody` switch case: `"workflow_doctor"` → `renderWorkflowDoctorOutput`.

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:
- `TestRenderWorkflowDoctor_ValidDiagnostics` — all three levels rendered, messages present
- `TestRenderWorkflowDoctor_EnvelopeShape` — envelope unwrapping
- `TestRenderWorkflowDoctor_NoDiagnostics` — "No issues found." message
- `TestRenderWorkflowDoctor_InvalidJSONFallback` — malformed JSON falls back gracefully

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

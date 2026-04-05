# Implementation: feat-mcp-control-tools

**Ticket**: feat-mcp-control-tools
**Feature**: MCP_CONTROL_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete (was already done at scaffolding)

---

## Summary

The `approve`, `deny`, `cancel`, and `hijack` renderers were implemented as part of
`eng-mcp-renderer-scaffolding`. No new code was added by this ticket's implementation
pass; this document records the existing state for completeness.

---

## What Exists

### `internal/ui/chat/smithers_mcp.go`

- `"approve"`, `"deny"`, `"cancel"`, `"hijack"` in `smithersToolLabels` and
  `smithersPrimaryKeys` (all keyed on `"runId"`).
- `ActionConfirmation` struct: `Success`, `RunID`, `GateID`, `Message`.
- `HijackConfirmation` struct: `Success`, `RunID`, `Agent`, `Instructions`.
- `renderActionCard`: renders badge, run ID, optional gate ID and message.
  - Falls back to `renderFallback` when `success` is false or JSON is malformed.
- `renderHijackCard`: extends action card with agent and instructions fields.
- `renderBody` switch cases:
  - `"approve"` → APPROVED badge (CardApproved)
  - `"deny"` → DENIED badge (CardDenied)
  - `"cancel"` → CANCELED badge (CardCanceled)
  - `"hijack"` → renderHijackCard (HIJACKED badge, CardStarted)

### `internal/ui/chat/smithers_mcp_test.go`

Existing tests:
- `TestRenderApproveCard`
- `TestRenderDenyCard`
- `TestRenderCancelCard`
- `TestRenderActionCard_InvalidJSONFallback`
- `TestRenderActionCard_SuccessFalseFallback`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

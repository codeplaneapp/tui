# Implementation: feat-mcp-observability-tools

**Ticket**: feat-mcp-observability-tools
**Feature**: MCP_OBSERVABILITY_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete (was already done at scaffolding)

---

## Summary

The `inspect`, `chat`, and `logs` renderers were implemented as part of
`eng-mcp-renderer-scaffolding`. No new code was added by this ticket's implementation
pass; this document records the existing state for completeness.

---

## What Exists

### `internal/ui/chat/smithers_mcp.go`

- `"inspect"`, `"chat"`, `"logs"` in `smithersToolLabels` and `smithersPrimaryKeys`
  (all keyed on `"runId"`).
- `NodeEntry` struct: `Name`, `Status`, `Output`, `Children` (recursive).
- `InspectResult` struct: `RunID`, `Workflow`, `Status`, `Nodes`.
- `renderInspectTree`: builds a lipgloss tree from the node hierarchy.
  - Node icons: `●` running, `✓` completed, `×` failed, `○` pending.
- `renderBody` switch cases:
  - `"inspect"` → `renderInspectTree`
  - `"chat"`, `"logs"` → plain-text renderer

### `internal/ui/chat/smithers_mcp_test.go`

Existing tests:
- `TestRenderInspectTree_ValidJSON`
- `TestRenderInspectTree_InvalidJSONFallback`
- `TestRenderChat_PlainText`
- `TestRenderLogs_PlainText`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

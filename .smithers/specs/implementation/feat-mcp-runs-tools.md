# Implementation: feat-mcp-runs-tools

**Ticket**: feat-mcp-runs-tools
**Feature**: MCP_RUNS_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete (was already done at scaffolding)

---

## Summary

The `runs_list` renderer was implemented as part of `eng-mcp-renderer-scaffolding`.
No new code was added by this ticket's implementation pass; this document records the
existing state for completeness.

---

## What Exists

### `internal/ui/chat/smithers_mcp.go`

- `"runs_list"` in `smithersToolLabels` ("Runs List").
- `RunEntry` struct: `ID`, `Workflow`, `Status`, `Step`, `Elapsed`.
- `renderRunsTable`: 5-column table (ID, Workflow, Status, Step, Time).
  - Status column styled via `styleStatus`.
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Truncates at `maxTableRows` with "… and N more" suffix when not expanded.
  - Falls back to `renderFallback` on parse error.
- `renderBody` switch case: `"runs_list"` → `renderRunsTable`.

### `internal/ui/chat/smithers_mcp_test.go`

Existing tests:
- `TestRenderRunsTable_ValidJSON`
- `TestRenderRunsTable_EnvelopeShape`
- `TestRenderRunsTable_InvalidJSONFallback`
- `TestRenderRunsTable_EmptyList`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

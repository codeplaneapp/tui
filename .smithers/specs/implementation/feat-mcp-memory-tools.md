# Implementation: feat-mcp-memory-tools

**Ticket**: feat-mcp-memory-tools
**Feature**: MCP_MEMORY_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete (was already done at scaffolding)

---

## Summary

The `memory_list` and `memory_recall` renderers were implemented as part of
`eng-mcp-renderer-scaffolding`. No new code was added by this ticket's implementation
pass; this document records the existing state for completeness.

---

## What Exists

### `internal/ui/chat/smithers_mcp.go`

- `"memory_list"`, `"memory_recall"` in `smithersToolLabels`.
- `"memory_recall"` in `smithersPrimaryKeys` (keyed on `"query"`).
- `MemoryEntry` struct: `Key`, `Value`, `RunID`, `Relevance` (float64).
- `renderMemoryTable`: dual-mode table.
  - If any entry has a non-zero `Relevance`, renders 3 columns: Relevance, Key, Value.
  - Otherwise renders 3 columns: Key, Value, RunID.
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Truncates at `maxTableRows` with "… and N more" suffix.
- `renderBody` switch case: `"memory_list"`, `"memory_recall"` → `renderMemoryTable`.

### `internal/ui/chat/smithers_mcp_test.go`

Existing tests:
- `TestRenderMemoryList`
- `TestRenderMemoryRecall_HasRelevance`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

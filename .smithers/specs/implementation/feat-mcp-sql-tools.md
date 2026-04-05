# Implementation: feat-mcp-sql-tools

**Ticket**: feat-mcp-sql-tools
**Feature**: MCP_SQL_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete (was already done at scaffolding)

---

## Summary

The `sql` renderer was implemented as part of `eng-mcp-renderer-scaffolding`.
No new code was added by this ticket's implementation pass; this document records
the existing state for completeness.

---

## What Exists

### `internal/ui/chat/smithers_mcp.go`

- `"sql"` in `smithersToolLabels` ("SQL") and `smithersPrimaryKeys`
  (keyed on `"query"`).
- `SQLResult` struct: `Columns` ([]string), `Rows` ([][]interface{}).
- `renderSQLTable`: dual-shape renderer.
  - Primary shape: `{"columns":[...],"rows":[[...],...]}` — uses structured columns.
  - Fallback shape: `[{col:val,...}]` array of objects — derives columns from first row.
  - Truncates at `maxTableRows` with "… and N more" suffix.
  - Returns "No results." / "No rows returned." on empty input.
  - Falls back to `renderFallback` on unrecognized shape.
- `renderBody` switch case: `"sql"` → `renderSQLTable`.

### `internal/ui/chat/smithers_mcp_test.go`

Existing tests:
- `TestRenderSQLTable_StructuredShape`
- `TestRenderSQLTable_ObjectArrayShape`
- `TestRenderSQLTable_Empty`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

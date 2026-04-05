# Implementation: feat-mcp-ticket-tools

**Ticket**: feat-mcp-ticket-tools
**Feature**: MCP_TICKET_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

Added renderer cases for all six Smithers ticket MCP tools: `ticket_list`,
`ticket_search`, `ticket_get`, `ticket_create`, `ticket_update`, `ticket_delete`.
All previously fell through to the JSON fallback renderer.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

- Added all six tools to `smithersToolLabels`.
- Added primary keys to `smithersPrimaryKeys`:
  - `ticket_get`, `ticket_update`, `ticket_delete` → `"ticketId"`
  - `ticket_create` → `"id"`
  - `ticket_search` → `"query"`
- Added `TicketEntry` struct: `ID`, `Title`, `Status`, `Content`, `CreatedAt`.
- Added `renderTicketTable` method: 3-column table (ID, Title, Status).
  - Status is styled via `styleStatus`.
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Falls back to `renderFallback` on parse error.
- Added `renderBody` switch cases:
  - `"ticket_list"`, `"ticket_search"` → `renderTicketTable`
  - `"ticket_create"`, `"ticket_update"`, `"ticket_delete"` → `renderActionCard` with "DONE" badge
  - `"ticket_get"` → `renderFallback` (markdown/JSON content fits existing fallback)

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:
- `TestRenderTicketTable_ValidJSON` — happy path list table
- `TestRenderTicketSearch_ValidJSON` — search uses same renderer
- `TestRenderTicketTable_EnvelopeShape` — envelope unwrapping
- `TestRenderTicketTable_Empty` — "No tickets found."
- `TestRenderTicketTable_InvalidJSONFallback` — malformed JSON graceful fallback
- `TestRenderTicketCreate_Card` — DONE badge
- `TestRenderTicketUpdate_Card` — DONE badge
- `TestRenderTicketDelete_Card` — DONE badge
- `TestRenderTicketGet_Fallback` — JSON/markdown content rendered via fallback

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

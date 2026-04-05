# Implementation: feat-mcp-cron-tools

**Ticket**: feat-mcp-cron-tools
**Feature**: MCP_CRON_MUTATION_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

The `cron_list` renderer was already implemented. This ticket adds renderer cases
for the three mutation tools: `cron_add`, `cron_rm`, and `cron_toggle`. All three
previously fell through to the JSON fallback renderer.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

- Added `"cron_add"`, `"cron_rm"`, `"cron_toggle"` to `smithersToolLabels`.
- Added primary keys to `smithersPrimaryKeys`:
  - `cron_add` → `"workflow"` (the workflow being scheduled)
  - `cron_rm` → `"cronId"`
  - `cron_toggle` → `"cronId"`
- Added `renderBody` switch cases:
  - `"cron_add"` → `renderActionCard` with "SCHEDULED" badge and `CardStarted` style
  - `"cron_rm"` → `renderActionCard` with "REMOVED" badge and `CardCanceled` style
  - `"cron_toggle"` → `renderActionCard` with "TOGGLED" badge and `CardDone` style

All three use the existing `ActionConfirmation` struct (`{success, runId, gateId, message}`).
If `success` is false or the JSON is malformed, `renderActionCard` falls back to `renderFallback`.

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:
- `TestRenderCronAdd_Card` — "SCHEDULED" badge appears in output
- `TestRenderCronRm_Card` — "REMOVED" badge appears in output
- `TestRenderCronToggle_Card` — "TOGGLED" badge appears in output
- `TestRenderCronAdd_InvalidJSONFallback` — malformed JSON falls back gracefully

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

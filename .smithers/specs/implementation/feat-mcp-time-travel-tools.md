# Implementation: feat-mcp-time-travel-tools

**Ticket**: feat-mcp-time-travel-tools
**Feature**: MCP_TIME_TRAVEL_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

Completed all three outstanding time-travel gaps:

1. Replaced the `renderDiffFallback` stub with a real `renderDiff` renderer that
   parses `SnapshotDiff` content and displays each change as `op  path  before → after`.
2. Added the `timeline` tool renderer as a sortable snapshot table.
3. Added `"revert"` to `smithersPrimaryKeys` so the run ID appears in the header.

The `fork`, `replay`, and `revert` action card cases were already wired; only the
primary key was missing for `revert`.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

**Diff renderer:**
- Replaced `renderDiffFallback` (which was a pure TODO stub) with `renderDiff`.
- Added `DiffEntry` struct: `Path`, `Before` (any), `After` (any), `Op` (string).
- Added `SnapshotDiff` struct: `FromID`, `ToID`, `Changes` ([]DiffEntry).
- `renderDiff` parses content into `SnapshotDiff`; if parse fails or `Changes` is
  empty, falls back to `renderFallback`.
- Each change renders as: `<styled-op>  <path>  <before> → <after>`.
- Added `styleDiffOp` helper: `add` → green `StatusRunning`, `remove` → red
  `StatusFailed`, `change` → yellow `StatusApproval`.
- Updated `renderBody` switch: `"diff"` now calls `renderDiff` instead of `renderDiffFallback`.

**Timeline renderer:**
- Added `"timeline"` to `smithersToolLabels` and `smithersPrimaryKeys` (`"runId"`).
- Added `SnapshotSummary` struct: `ID`, `SnapshotNo` (int), `Label`, `NodeID`, `CreatedAt`.
- Added `renderTimelineTable` method: 4-column table (No., Node, Label, Created At).
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Falls back to `renderFallback` on parse error.
- Added `renderBody` switch case: `"timeline"` → `renderTimelineTable`.

**Revert primary key:**
- Added `"revert": "runId"` to `smithersPrimaryKeys`.

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:

Diff renderer:
- `TestRenderDiff_ValidChanges` — 2-change diff renders path strings
- `TestRenderDiff_EmptyChangesFallback` — valid shape but no changes → fallback
- `TestRenderDiff_InvalidJSONFallback` — malformed JSON → fallback

(Replaced original `TestRenderDiff_Fallback` stub with the three cases above.)

Timeline renderer:
- `TestRenderTimelineTable_ValidJSON` — happy path, 2-row table; node and label present
- `TestRenderTimelineTable_EnvelopeShape` — envelope unwrapping
- `TestRenderTimelineTable_Empty` — "No snapshots found."
- `TestRenderTimelineTable_InvalidJSONFallback` — malformed JSON graceful fallback

Revert primary key:
- `TestRevertPrimaryKey_InMap` — asserts `"revert"` maps to `"runId"` in `smithersPrimaryKeys`

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

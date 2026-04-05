# Implementation: feat-mcp-agent-tools

**Ticket**: feat-mcp-agent-tools
**Feature**: MCP_AGENT_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

Added renderer cases for the `agent_list` and `agent_chat` Smithers MCP tools in
`internal/ui/chat/smithers_mcp.go`. Previously both tools fell through to the JSON
fallback renderer.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

- Added `"agent_list"` and `"agent_chat"` to `smithersToolLabels`.
- Added `"agent_chat": "agentId"` to `smithersPrimaryKeys` so the agent ID appears
  in the tool header when known.
- Added `AgentEntry` struct: `ID`, `Name`, `Available` (bool), `Roles` ([]string).
- Added `renderAgentTable` method: 3-column table (Name, Available, Roles).
  - Available is styled green "yes" or subtle "no", matching the cron_list pattern.
  - Roles are joined with ", "; shows "—" when empty.
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Falls back to `renderFallback` on parse error.
- Added `renderBody` switch cases:
  - `"agent_list"` → `renderAgentTable`
  - `"agent_chat"` → plain-text renderer (same as `chat`/`logs`)

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:
- `TestRenderAgentTable_ValidJSON` — happy path, 2-row table
- `TestRenderAgentTable_EnvelopeShape` — `{"data":[...]}` unwrapping
- `TestRenderAgentTable_Empty` — empty array returns "No agents found."
- `TestRenderAgentTable_InvalidJSONFallback` — malformed JSON falls back gracefully
- `TestRenderAgentChat_PlainText` — prose content rendered as plain text

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

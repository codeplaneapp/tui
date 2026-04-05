# Implementation: feat-mcp-prompt-tools

**Ticket**: feat-mcp-prompt-tools
**Feature**: MCP_PROMPT_TOOLS_RENDERER
**Group**: MCP Integration
**Date**: 2026-04-05
**Status**: Complete

---

## Summary

Added renderer cases for the `prompt_list`, `prompt_get`, `prompt_render`, and
`prompt_update` Smithers MCP tools. All previously fell through to the JSON
fallback renderer.

---

## Changes

### `internal/ui/chat/smithers_mcp.go`

- Added all four tools to `smithersToolLabels`.
- Added primary keys to `smithersPrimaryKeys`:
  - `prompt_get`, `prompt_render`, `prompt_update` → `"promptId"`
- Added `PromptEntry` struct: `ID`, `EntryFile`.
- Added `renderPromptTable` method: 2-column table (ID, Entry File).
  - Supports both bare array and `{"data":[...]}` envelope shapes.
  - Falls back to `renderFallback` on parse error.
- Added `renderBody` switch cases:
  - `"prompt_list"` → `renderPromptTable`
  - `"prompt_render"` → plain-text renderer (same path as `chat`/`logs`/`agent_chat`)
  - `"prompt_update"` → `renderActionCard` with "UPDATED" badge and `CardDone` style
  - `"prompt_get"` → `renderFallback` (markdown content fits existing fallback)

### `internal/ui/chat/smithers_mcp_test.go`

Tests added:
- `TestRenderPromptTable_ValidJSON` — happy path, 2-row table with ID and entry file
- `TestRenderPromptTable_EnvelopeShape` — envelope unwrapping
- `TestRenderPromptTable_Empty` — "No prompts found."
- `TestRenderPromptTable_InvalidJSONFallback` — malformed JSON graceful fallback
- `TestRenderPromptRender_PlainText` — prose content rendered as plain text
- `TestRenderPromptUpdate_Card` — UPDATED badge rendered

---

## Verification

```
go build ./internal/ui/chat/...  # clean
go test ./internal/ui/chat/...   # 70 PASS, 0 FAIL
```

# Implementation Summary: eng-live-chat-scaffolding

**Date**: 2026-04-05
**Status**: Complete

---

## What Was Built

### New Files

**`internal/ui/views/livechat.go`** — The primary deliverable. A Bubble Tea v2 model implementing the `views.View` interface.

- `LiveChatView` struct with fields: `client`, `runID`, `taskID`, `agentName`, `run`, `blocks`, `loadingRun`, `loadingBlocks`, `runErr`, `blocksErr`, `width`, `height`, `scrollLine`, `follow`, `lines`, `linesDirty`
- `NewLiveChatView(client, runID, taskID, agentName)` constructor — follow defaults to `true`
- `Init()` — fires two concurrent `tea.Cmd`s: `fetchRun` and `fetchBlocks`
- `Update(msg)` — handles: `liveChatRunLoadedMsg`, `liveChatRunErrorMsg`, `liveChatBlocksLoadedMsg`, `liveChatBlocksErrorMsg`, `liveChatNewBlockMsg`, `smithers.ChatStreamDoneMsg`, `smithers.ChatStreamErrorMsg`, `tea.WindowSizeMsg`, `tea.KeyPressMsg`
- `View()` — renders: header (`SMITHERS › Chat › <runID> (<workflowName>)`), sub-header (agent, node, elapsed time, LIVE indicator), divider, scrollable body, streaming indicator
- Keyboard: `q`/`Esc` → `PopViewMsg`; `f` → toggle follow; `h` → placeholder (no-op); `↑`/`k` → scroll up; `↓`/`j` → scroll down; `PgUp`/`PgDn` → page scroll; `r` → refresh
- `renderedLines()` — cached line buffer with timestamp prefix (`[mm:ss]`) and `┊` gutter, rebuilt when `linesDirty = true`
- `ShortHelp()` → `["[↑↓] Scroll", "[f] Follow: on/off", "[h] Hijack", "[r] Refresh", "[q/Esc] Back"]`
- Compile-time interface check: `var _ View = (*LiveChatView)(nil)`

**`internal/ui/views/livechat_test.go`** — 37 unit tests covering: interface compliance, constructor defaults, Init, all Update message types, all keyboard bindings, all View() render states, ShortHelp, scroll clamping, `fmtDuration` helper, and fetch command safety.

### Modified Files

**`internal/smithers/types_runs.go`** — Added:
- `ChatRole` string type with constants: `ChatRoleSystem`, `ChatRoleUser`, `ChatRoleAssistant`, `ChatRoleTool`
- `ChatBlock` struct: `ID`, `RunID`, `NodeID`, `Attempt`, `Role`, `Content`, `TimestampMs`
- `ChatBlockMsg`, `ChatStreamDoneMsg`, `ChatStreamErrorMsg` tea message types

**`internal/smithers/client.go`** — Added:
- `GetRun(ctx, runID) (*RunSummary, error)` — HTTP → exec transport chain
- `GetChatOutput(ctx, runID) ([]ChatBlock, error)` — HTTP → SQLite → exec transport chain
- `scanChatBlocks(rows)` — SQLite row scanner for `ChatBlock`

**`internal/smithers/client_test.go`** — Added tests:
- `TestGetRun_HTTP`, `TestGetRun_Exec`
- `TestGetChatOutput_HTTP`, `TestGetChatOutput_Exec`, `TestGetChatOutput_Empty`

**`internal/ui/dialog/actions.go`** — Added `ActionOpenLiveChatView{RunID, TaskID, AgentName}` action type.

**`internal/ui/dialog/commands.go`** — Added `"live_chat"` command item (opens `ActionOpenLiveChatView{}`).

**`internal/ui/model/ui.go`** — Added `case dialog.ActionOpenLiveChatView` handler that calls `views.NewLiveChatView`, pushes to router, and sets `uiSmithersView` state.

---

## Design Decisions

1. **`RunSummary` not `Run`**: The linter renamed the old `Run` type to `RunSummary` (with `Run = ForkReplayRun` alias for backward compat). `LiveChatView` uses `*smithers.RunSummary` for the header metadata.

2. **Follow mode on by default**: Matches the design doc's "auto-scroll" default. Users disable with `f`. Scrolling with arrow keys automatically disables follow.

3. **Line cache**: `renderedLines()` rebuilds only when `linesDirty = true` (set on block arrival, resize, or refresh). This avoids O(n) rerender on every key event.

4. **`h` as no-op placeholder**: The hijack handoff (TUI suspend + exec agent CLI) is a separate ticket. The key is handled so the shortcut hint is visible and the key doesn't bubble up unexpectedly.

5. **Relative timestamps**: When `run.StartedAtMs` is available, block timestamps render as `[mm:ss]` relative to run start (matching the design doc mockup). Falls back to `[HH:MM:SS]` absolute when no run metadata yet.

6. **Streaming support**: `liveChatNewBlockMsg` can be injected by a future SSE-streaming ticket (alongside `smithers.ChatStreamDoneMsg` / `ChatStreamErrorMsg`). The view is ready to receive these without modification.

---

## Test Results

```
go test ./internal/ui/views/... -run "LiveChat|FmtDuration" -v
# 37 tests — all PASS

go test ./internal/smithers/... -run "TestGetRun|TestGetChatOutput" -v
# 5 tests — all PASS

go test ./internal/smithers/... -v
# All existing tests — PASS

go test ./internal/ui/views/... -v
# All tests — PASS
```

---

## Open Items (deferred to follow-on tickets)

1. `h` hijack — implement TUI handoff (exec agent CLI, suspend/resume Smithers TUI)
2. `/chat <run-id>` slash command parsing in the chat editor
3. SSE live-streaming: wire `smithers.Client.StreamChat` (future) to emit `liveChatNewBlockMsg`
4. Attempt navigation (if a node has retries, allow switching between attempt N)
5. VHS tape recording (`tests/vhs/livechat-happy-path.tape`)

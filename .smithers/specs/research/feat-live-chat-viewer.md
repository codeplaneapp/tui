# Research: feat-live-chat-viewer

## Overview

This document records findings from a deep read of the Crush codebase and Smithers
architecture in preparation for implementing the Live Chat Viewer feature. It covers the
existing chat rendering pipeline, how Smithers streams chat data, the data model for live
chat, viewport/scrolling patterns in Bubble Tea, and the integration points for the hijack
button.

---

## 1. Existing Chat Rendering Pipeline

### 1.1 Message Types

Crush's chat system is built around the `internal/message` package (not `internal/ui/chat`).
The `message.Message` struct has a `Role` field (`User`, `Assistant`, `Tool`) and a slice of
`ContentPart` values. The relevant content part types for a live chat viewer are:

- `TextPart` — plain text (agent prose output)
- `ReasoningPart` — extended thinking content (Claude only)
- `ToolCallPart` — a tool invocation with name, ID, and JSON input
- `ToolResultPart` — the result of a tool call, linked by tool call ID
- `FinishPart` — signals the message is complete, carries finish reason and timestamp

The UI layer in `internal/ui/chat/messages.go` converts `*message.Message` values into
`MessageItem` implementations via `ExtractMessageItems()`. Each `MessageItem` knows how to
render itself to a string at a given width.

### 1.2 The MessageItem Hierarchy

```
MessageItem (interface)
  ├── UserMessageItem        — user's prompt text
  ├── AssistantMessageItem   — agent prose, streaming animation, extended thinking
  ├── AssistantInfoItem      — model/provider/latency footer shown after completion
  └── ToolMessageItem (interface)
        ├── BashToolMessageItem
        ├── FileToolMessageItem   (read, write, edit, glob, grep, ls)
        ├── FetchToolMessageItem
        ├── SearchToolMessageItem
        ├── DiagnosticsToolMessageItem
        ├── MCPToolMessageItem
        ├── DockerMCPToolMessageItem
        └── GenericToolMessageItem  — fallback for unknown tools
```

All tool items are created through `newBaseToolMessageItem()` in `tools.go`, which embeds
a `baseToolMessageItem` struct. The base struct holds the raw `message.ToolCall`, an optional
`*message.ToolResult`, a `ToolStatus` (AwaitingPermission / Running / Success / Error /
Canceled), and an `anim.Anim` for the spinning animation.

### 1.3 Tool Renderer Pattern

Each tool type provides a `ToolRenderContext` that implements a single method:

```go
type ToolRenderer interface {
    RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string
}
```

`ToolRenderOpts` carries the tool call, result, animation state, expanded/compact flags,
and status. The `baseToolMessageItem.RawRender(width)` method calls its renderer and caches
the output. Renderers are selected at construction time — there is no dynamic dispatch
registry. Tool items are created in `ExtractMessageItems()` by matching `tc.Name` against
known tool names (bash, view, edit, write, glob, grep, ls, fetch, diagnostics, mcp, etc.).

### 1.4 Rendering Chain

```
*message.Message
    └─ ExtractMessageItems(sty, msg, toolResults) → []MessageItem
            Each item: RawRender(width) → string (cached)
                       Render(width)   → RawRender + left border prefix
```

The `list.List` component in `internal/ui/list/` assembles `MessageItem.Render()` outputs
into a scrollable viewport. `AssistantMessageItem` and `ToolMessageItem` both support
expand/collapse toggling and focus states.

### 1.5 Streaming Animation

While an agent message is being generated, `AssistantMessageItem.isSpinning()` returns true
(the message has no finish part yet). The `anim.Anim` struct (internal ticker-based
animation) drives a gradient shimmer across the message text. When a `FinishPart` arrives
the animation stops and the message is re-rendered statically.

The same animation pattern applies to running tool calls: a `ToolStatus == ToolStatusRunning`
item shows a spinner via its `anim.Anim` instance.

---

## 2. How Smithers Streams Chat Data

### 2.1 Transport Architecture (Thin Frontend)

Smithers TUI is a thin frontend. It does not access the SQLite database directly for
mutations; reads can fall back to the DB if no HTTP server is running. Live chat streaming
requires the Smithers HTTP server to be running (`smithers up --serve`) because the DB does
not have a push mechanism.

```
Smithers TUI (Go)
    │
    ├── HTTP GET  /agent/chat/{runID}         → snapshot of all ChatAttempts for a run
    ├── HTTP GET  /agent/chat/{runID}/stream  → SSE stream of new ChatBlock events
    └── SQLite    _smithers_chat_attempts      → read-only fallback for past runs
```

### 2.2 SSE Event Format

The engineering doc (`03-ENGINEERING.md §4.3`) specifies SSE delivery for real-time events.
The Smithers HTTP server sends `text/event-stream` responses. Each event carries a JSON
payload. For chat output the relevant event types are:

| SSE Event Type     | Payload                              | Description                           |
|--------------------|--------------------------------------|---------------------------------------|
| `chat.block`       | `ChatBlock`                          | One rendered output unit from an agent |
| `run.status`       | `RunStatusEvent`                     | Run status changed                    |
| `chat.attempt.new` | `{ runID, nodeID, attempt }`         | New attempt started (retry)           |
| `chat.done`        | `{ runID }`                          | Agent finished for this run           |

### 2.3 ChatBlock Data Model

The engineering doc and GUI reference establish the following structure. These types are not
yet in `internal/smithers/types.go` and must be added:

```go
// ChatAttempt holds all output for one agent attempt on a node.
// Maps to _smithers_chat_attempts in smithers/src/db/internal-schema.ts
type ChatAttempt struct {
    ID           string  `json:"id"`
    RunID        string  `json:"runId"`
    NodeID       string  `json:"nodeId"`
    AttemptNo    int     `json:"attemptNo"`    // 1-based
    AgentEngine  string  `json:"agentEngine"`  // "claude-code" | "codex" | etc.
    Prompt       string  `json:"prompt"`       // System prompt sent to agent
    ResponseText string  `json:"responseText"` // Plain text response (streaming partial)
    ToolCallsJSON string `json:"toolCallsJson"` // NDJSON: one ToolCallRecord per line
    Status       string  `json:"status"`       // "running" | "complete" | "failed" | "retrying"
    StartedAtMs  int64   `json:"startedAtMs"`
    EndedAtMs    *int64  `json:"endedAtMs"`
}

// ChatBlock is a single displayable unit emitted from a streaming agent session.
// The SSE endpoint emits one ChatBlock per streamed event.
type ChatBlock struct {
    RunID       string  `json:"runId"`
    NodeID      string  `json:"nodeId"`
    AttemptNo   int     `json:"attemptNo"`
    Role        string  `json:"role"`        // "user" | "assistant" | "tool_call" | "tool_result"
    Content     string  `json:"content"`     // Text content or JSON-encoded tool input/result
    ToolName    string  `json:"toolName,omitempty"`
    ToolCallID  string  `json:"toolCallId,omitempty"`
    TimestampMs int64   `json:"timestampMs"`
}
```

### 2.4 Existing Client Methods Needed

The `internal/smithers/client.go` currently has no chat-specific methods. Per the
engineering spec (§3.1.3), the following must be added:

```go
// GetChatOutput returns a snapshot of all ChatAttempts for a run.
// Routes: HTTP GET /agent/chat/{runID} → SQLite fallback.
func (c *Client) GetChatOutput(ctx context.Context, runID string) ([]ChatAttempt, error)

// StreamChat opens an SSE connection and delivers ChatBlock events as they arrive.
// Returns a channel the caller reads from; cancel ctx to close.
func (c *Client) StreamChat(ctx context.Context, runID string) (<-chan ChatBlock, error)

// GetRun returns metadata for a single run (agent engine, node, elapsed time).
func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error)
```

The `Run` type is also absent from `internal/smithers/types.go`. The engineering spec
(§3.1.3) documents it:

```go
type Run struct {
    ID           string    `json:"id"`
    WorkflowPath string    `json:"workflowPath"`
    Status       string    `json:"status"`         // "running" | "completed" | "failed" | "paused"
    CurrentNode  string    `json:"currentNode"`
    AgentEngine  string    `json:"agentEngine"`
    StartedAtMs  int64     `json:"startedAtMs"`
    EndedAtMs    *int64    `json:"endedAtMs"`
    AttemptNo    int       `json:"attemptNo"`
}
```

### 2.5 SSE Consumer Pattern

The SSE consumer follows the same pattern already used in `client.go` for HTTP reads. A
goroutine is spawned inside `StreamChat`. It reads `bufio.Scanner` lines from the response
body, parses `data: {...}` lines, unmarshals each as a `ChatBlock`, and sends it to a
channel. The Bubble Tea `Init` command returns a recursive tea.Cmd that reads one block at
a time and reissues itself — or uses a `tea.Batch` with a long-running goroutine that posts
messages back via a channel wrapped in a `tea.Cmd`.

The recommended pattern (matching how Crush's agent loop handles streaming) is:

```go
// Init returns the first StreamCmd; each StreamCmd reads one block and emits
// chatBlockMsg, then re-issues the next StreamCmd. This keeps all messages on
// the Bubble Tea event bus without spawning an independent goroutine.
func (v *LiveChatView) nextBlockCmd(ch <-chan ChatBlock) tea.Cmd {
    return func() tea.Msg {
        block, ok := <-ch
        if !ok {
            return chatStreamDoneMsg{}
        }
        return chatBlockMsg{block: block}
    }
}
```

An alternative is a single goroutine that posts via a `tea.Cmd` wrapper — both are valid
Bubble Tea patterns. The channel-based approach integrates more cleanly with context
cancellation.

---

## 3. Data Model for Live Chat

### 3.1 Attempt Tracking

A single run may have multiple attempts on a node if the agent retried (e.g., due to rate
limits or explicit retry logic in the workflow). Each `ChatAttempt` has an `AttemptNo`
(1-based). The Live Chat Viewer must:

1. Show the current (latest) attempt by default.
2. Allow navigating between attempts (`[` and `]` keys, or a tab bar).
3. Display a "Attempt N of M" indicator in the header when M > 1.

### 3.2 Mapping ChatBlocks to Crush MessageItems

`ChatBlock.Role` maps to `message.Role`:

| ChatBlock Role  | message.Role | MessageItem type                          |
|-----------------|--------------|-------------------------------------------|
| `user`          | `User`       | `UserMessageItem` (the prompt sent to agent) |
| `assistant`     | `Assistant`  | `AssistantMessageItem` (agent text output) |
| `tool_call`     | `Assistant`  | `ToolMessageItem` (tool being called)      |
| `tool_result`   | `Tool`       | Attached to preceding `ToolMessageItem`    |

The `ToolName` field in `ChatBlock` is used to select the right `ToolMessageItem` constructor
(same switch-on-name logic as `ExtractMessageItems()`). The `ToolCallID` links tool calls to
their results.

### 3.3 Timestamp Markers

Each `ChatBlock` carries `TimestampMs`. The Live Chat Viewer converts this to a relative
offset from the run's `StartedAtMs`: e.g. `[00:02]`. These are displayed as faint prefixes
before each message group.

### 3.4 State Machine

```
LiveChatView state:
  loading       → fetching initial snapshot + opening SSE connection
  streaming     → SSE active, blocks arriving
  paused        → SSE active, follow=false (user scrolled up)
  hijacking     → waiting for HijackRun() response
  hijacked      → tea.ExecProcess in flight (TUI suspended)
  refreshing    → post-hijack, fetching updated state
  done          → agent finished or run ended
  error         → transport failure
```

---

## 4. Viewport and Scrolling Patterns in Bubble Tea

### 4.1 Bubble Tea v2 Rendering

The existing Smithers views (`agents.go`, `approvals.go`, `tickets.go`) all implement the
`View() string` contract defined in `internal/ui/views/router.go`. The research for
`eng-live-chat-scaffolding` notes a potential migration to `Draw(scr, area)` but the
engineering spec for this ticket defers that and keeps `View() string`. This avoids
introducing a rendering model refactor mid-feature.

### 4.2 Viewport Component

`charm.land/bubbles/v2/viewport` provides a scrollable viewport model. Key API:

```go
vp := viewport.New(width, height)
vp.SetContent(rendered string)   // replace the full content string
vp.GotoBottom()                  // scroll to end (follow mode)
vp.LineDown(n)                   // scroll down n lines
vp.LineUp(n)                     // scroll up n lines
vp.AtBottom() bool               // true when viewport is scrolled to bottom
```

`viewport.Model.View()` renders only the visible slice of the content string, accounting for
scroll offset. The TUI calls `vp.SetContent()` each time new blocks arrive, then calls
`vp.GotoBottom()` only when `follow == true`.

### 4.3 Follow Mode

Follow mode is a boolean in `LiveChatView`. When `true`:
- On every new `chatBlockMsg`, after appending the block, call `vp.GotoBottom()`.
- When the user presses `↑`, `k`, or `PageUp`, set `follow = false`.
- Press `f` to re-enable follow mode and snap to bottom.

The help bar shows: `[f] Follow` when not following, `[f] Unfollow` when following.

### 4.4 Content Accumulation

Because `viewport.SetContent()` replaces the entire string, the view must maintain a
`strings.Builder` (or a `[]string` slice of rendered lines) that accumulates all rendered
block strings. On each new block:

1. Convert the `ChatBlock` to a `MessageItem` (or append to an in-progress item if the block
   is a streaming continuation of the current assistant message).
2. Call `item.Render(contentWidth)`.
3. Append to the accumulated content string.
4. Call `vp.SetContent(accumulated)`.
5. If following, call `vp.GotoBottom()`.

For large chats this re-render of the full string is O(n). A future optimization is to use
`viewport.SetYOffset` and only append new lines, but the simple approach is correct for v1.

### 4.5 Window Resize

`tea.WindowSizeMsg` must update both the viewport dimensions and the content width:

```go
case tea.WindowSizeMsg:
    v.width = msg.Width
    v.height = msg.Height
    headerHeight := 3
    helpBarHeight := 1
    v.viewport.Width = msg.Width
    v.viewport.Height = msg.Height - headerHeight - helpBarHeight
    v.contentWidth = msg.Width - 2  // 1 char padding each side
    v.rebuildContent()              // re-render all blocks at new width
    return v, nil
```

---

## 5. Hijack Integration Points

### 5.1 The Handoff Mechanism

Session hijacking uses `tea.ExecProcess` — the same mechanism Crush uses for `Ctrl+O` editor
handoff (implemented around line 2630 of `internal/ui/model/ui.go`). Research for
`eng-hijack-handoff-util` (`.smithers/specs/research/eng-hijack-handoff-util.md`) recommends
a `HandoffToProgram` utility in `internal/ui/util/handoff.go` that handles binary validation,
environment merging, and return message routing.

### 5.2 HijackSession Type

The engineering spec (§3.2.2) defines:

```go
type HijackSession struct {
    RunID       string
    AgentEngine string   // "claude-code" | "codex" | "amp" | "gemini" | ...
    AgentBinary string   // resolved path, e.g. "/usr/local/bin/claude"
    ResumeToken string   // session ID to pass to --resume
    CWD         string   // working directory at time of hijack
    SupportsResume bool  // whether --resume is supported
}

func (h *HijackSession) ResumeArgs() []string {
    if h.SupportsResume && h.ResumeToken != "" {
        return []string{"--resume", h.ResumeToken}
    }
    return nil
}
```

This type must be added to `internal/smithers/types.go` and a corresponding `HijackRun`
method added to `client.go`:

```go
// HijackRun pauses the agent on the given run and returns session metadata
// for native TUI handoff.
// Routes: HTTP POST /hijack/{runID}
func (c *Client) HijackRun(ctx context.Context, runID string) (*HijackSession, error)
```

### 5.3 Integration Point in LiveChatView

The `h` keypress in `LiveChatView.Update()` triggers the hijack flow:

```
User presses 'h'
    → v.hijackRun() tea.Cmd  (calls HijackRun via HTTP)
    → receives hijackSessionMsg{session}
    → tea.ExecProcess(exec.Command(session.AgentBinary, session.ResumeArgs()...), callback)
    → Bubble Tea suspends; agent TUI takes terminal
    → User exits agent TUI
    → callback fires: hijackReturnMsg{runID, err}
    → v.refreshRunState() — reload run metadata and chat history
    → optional: insert "HIJACK SESSION ENDED" divider block
```

The `LiveChatView` does not need a separate `HijackView` — the handoff is a transient state,
not a full view. The view handles `hijackReturnMsg` the same way it handles `chatBlockMsg`
after reconnecting.

### 5.4 `h` Key Entry Point from RunsView

The `h` key is also available in the runs list view (`internal/ui/views/runs.go`, not yet
implemented as of the scaffold). When pressed, the runs view should call `HijackRun` for the
selected run and emit `tea.ExecProcess` directly — the same pattern as `LiveChatView`. Both
views reach the same underlying client method.

### 5.5 Navigation Entry Point

The Live Chat Viewer is opened via:

1. **Runs view**: Pressing `c` on a selected run calls
   `router.Push(views.NewLiveChatView(client, run.ID))`.
2. **Command palette**: `/chat <run_id>` action parses the run ID and pushes the view.
3. **Chat agent tool result**: When the MCP `smithers_chat` tool is invoked, the result
   renderer (future `chat/smithers_chat.go`) can include an inline action that opens the
   Live Chat Viewer for the referenced run.

---

## 6. Gaps Summary

| Gap | Location | Priority |
|-----|----------|----------|
| `Run`, `ChatAttempt`, `ChatBlock`, `HijackSession` types missing | `internal/smithers/types.go` | Must-have |
| `GetRun`, `GetChatOutput`, `StreamChat`, `HijackRun` methods missing | `internal/smithers/client.go` | Must-have |
| `livechat.go` view missing (scaffold ticket `eng-live-chat-scaffolding` addressed initial shell) | `internal/ui/views/livechat.go` | Must-have |
| SSE consumer implementation | `internal/smithers/client.go` | Must-have |
| `HandoffToProgram` utility | `internal/ui/util/handoff.go` | Must-have for hijack |
| `viewport.Model` integration in a Smithers view | `internal/ui/views/livechat.go` | Must-have |
| Attempt navigation (tab bar or `[`/`]` keys) | `internal/ui/views/livechat.go` | Should-have |
| Smithers-specific MCP tool renderer for `smithers_chat` | `internal/ui/chat/smithers_chat.go` | Nice-to-have |
| E2E terminal test for streaming chat | `tests/tui/livechat_e2e_test.go` | Should-have |
| VHS tape for happy path | `tests/vhs/livechat-happy-path.tape` | Should-have |

---

## 7. Files to Touch

| File | Action |
|------|--------|
| `internal/smithers/types.go` | Add `Run`, `ChatAttempt`, `ChatBlock`, `HijackSession` |
| `internal/smithers/client.go` | Add `GetRun`, `GetChatOutput`, `StreamChat`, `HijackRun` |
| `internal/smithers/client_test.go` | Tests for new methods |
| `internal/ui/views/livechat.go` | Full implementation (see plan doc) |
| `internal/ui/views/livechat_test.go` | Unit tests |
| `internal/ui/util/handoff.go` | `HandoffToProgram` utility (if not done by eng-hijack-handoff-util ticket) |
| `internal/ui/model/ui.go` | Wire `c` key in runs view + return from hijack |
| `internal/ui/dialog/actions.go` | `/chat <run_id>` action |
| `internal/ui/dialog/commands.go` | Command palette entry |
| `tests/tui/livechat_e2e_test.go` | Terminal E2E tests |
| `tests/vhs/livechat-happy-path.tape` | VHS recording |

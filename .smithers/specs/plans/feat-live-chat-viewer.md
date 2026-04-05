# Plan: feat-live-chat-viewer

## Goal

Implement the Live Chat Viewer feature on top of the existing `eng-live-chat-scaffolding`
work. The viewer must stream a running agent's chat output in real-time, render tool calls
using Crush's existing tool renderers, support follow mode with auto-scroll, track attempts
and retry history, and provide a hijack button that hands off the terminal to the agent's
native TUI via `tea.ExecProcess`.

---

## Prerequisites

The following tickets must be complete or their deliverables merged before this work starts:

| Ticket | Required deliverable |
|--------|----------------------|
| `eng-live-chat-scaffolding` | `LiveChatView` skeleton in `internal/ui/views/livechat.go`, router push/pop wired, `ChatBlock` + `Run` stub types present |
| `eng-hijack-handoff-util` | `HandoffToProgram` in `internal/ui/util/handoff.go`, `HandoffReturnMsg` type defined |
| `eng-smithers-client-runs` | `GetRun` in `internal/smithers/client.go`, `Run` type in `types.go` |

If any of these are not shipped, the corresponding work must be done inline in this ticket.

---

## Steps

### Step 1 — Extend Smithers Types and Client

**1a. Add types to `internal/smithers/types.go`**

Add `ChatAttempt`, `ChatBlock`, and `HijackSession` if not already present from the
scaffolding ticket:

```go
// ChatAttempt holds all output for one agent attempt on a node.
type ChatAttempt struct {
    ID            string `json:"id"`
    RunID         string `json:"runId"`
    NodeID        string `json:"nodeId"`
    AttemptNo     int    `json:"attemptNo"`
    AgentEngine   string `json:"agentEngine"`
    Prompt        string `json:"prompt"`
    ResponseText  string `json:"responseText"`
    ToolCallsJSON string `json:"toolCallsJson"` // NDJSON
    Status        string `json:"status"`        // "running"|"complete"|"failed"|"retrying"
    StartedAtMs   int64  `json:"startedAtMs"`
    EndedAtMs     *int64 `json:"endedAtMs"`
}

// ChatBlock is one streamed display unit from a running agent session.
type ChatBlock struct {
    RunID       string `json:"runId"`
    NodeID      string `json:"nodeId"`
    AttemptNo   int    `json:"attemptNo"`
    Role        string `json:"role"`                  // "user"|"assistant"|"tool_call"|"tool_result"
    Content     string `json:"content"`
    ToolName    string `json:"toolName,omitempty"`
    ToolCallID  string `json:"toolCallId,omitempty"`
    TimestampMs int64  `json:"timestampMs"`
}

// HijackSession carries metadata for a native TUI handoff to an agent CLI.
type HijackSession struct {
    RunID          string `json:"runId"`
    AgentEngine    string `json:"agentEngine"`
    AgentBinary    string `json:"agentBinary"`
    ResumeToken    string `json:"resumeToken"`
    CWD            string `json:"cwd"`
    SupportsResume bool   `json:"supportsResume"`
}

func (h *HijackSession) ResumeArgs() []string {
    if h.SupportsResume && h.ResumeToken != "" {
        return []string{"--resume", h.ResumeToken}
    }
    return nil
}
```

**1b. Add client methods to `internal/smithers/client.go`**

```go
// GetChatOutput returns a snapshot of all chat attempts for a run.
// Routes: HTTP GET /agent/chat/{runID} → SQLite _smithers_chat_attempts → exec.
func (c *Client) GetChatOutput(ctx context.Context, runID string) ([]ChatAttempt, error)

// StreamChat opens an SSE connection to /agent/chat/{runID}/stream.
// Returns a channel; caller must cancel ctx to close the stream.
func (c *Client) StreamChat(ctx context.Context, runID string) (<-chan ChatBlock, error)

// HijackRun pauses the agent on runID and returns session metadata for handoff.
// Routes: HTTP POST /hijack/{runID}
func (c *Client) HijackRun(ctx context.Context, runID string) (*HijackSession, error)
```

The SSE consumer in `StreamChat` must:
1. Issue `GET {apiURL}/agent/chat/{runID}/stream` with `Accept: text/event-stream`.
2. Read lines with `bufio.Scanner`; parse `data: {...}` lines into `ChatBlock`.
3. Send each block to the returned channel.
4. Return `ErrServerUnavailable` immediately if `!c.isServerAvailable()` — SSE requires
   a live server, there is no SQLite fallback for streaming.

**1c. Add tests to `internal/smithers/client_test.go`**

- `TestGetChatOutput_HTTP`: Mock server returns two `ChatAttempt` records, assert correct
  deserialization.
- `TestStreamChat_SSE`: Mock server sends three `data: {...}` lines, assert three
  `ChatBlock` values on the channel before close.
- `TestHijackRun_HTTP`: Mock server returns `HijackSession`, assert fields.

---

### Step 2 — Implement LiveChatView

Replace the scaffold skeleton in `internal/ui/views/livechat.go` with the full
implementation.

**2a. Struct definition**

```go
type LiveChatView struct {
    // Configuration
    runID  string
    client *smithers.Client

    // Run metadata (loaded once on init)
    run       *smithers.Run
    metaErr   error
    metaReady bool

    // Streaming state
    blockCh    <-chan smithers.ChatBlock
    blockCancel context.CancelFunc
    blocks     []smithers.ChatBlock   // all blocks received, all attempts
    streamDone bool

    // Attempt display
    attempts    map[int][]smithers.ChatBlock  // attemptNo → blocks
    currentAttempt int

    // Viewport
    viewport    viewport.Model
    follow      bool
    content     string  // accumulated rendered content for viewport
    contentWidth int

    // Hijack state
    hijacking   bool
    hijackErr   error

    // Layout
    width  int
    height int
    err    error
}
```

**2b. Message types (private to livechat.go)**

```go
type liveChatMetaMsg  struct{ run *smithers.Run; err error }
type liveChatSnapshotMsg struct{ attempts []smithers.ChatAttempt; err error }
type chatBlockMsg     struct{ block smithers.ChatBlock }
type chatStreamDoneMsg struct{}
type hijackSessionMsg struct{ session *smithers.HijackSession; err error }
type hijackReturnMsg  struct{ runID string; err error }
```

**2c. Init**

```go
func (v *LiveChatView) Init() tea.Cmd {
    return tea.Batch(
        v.fetchMetaCmd(),
        v.fetchSnapshotCmd(),
    )
}
```

`fetchMetaCmd` calls `GetRun` and emits `liveChatMetaMsg`. `fetchSnapshotCmd` calls
`GetChatOutput`, converts the snapshot to `ChatBlock` slices, and emits
`liveChatSnapshotMsg`. After the snapshot is loaded, `Init` also opens the SSE stream via
`openStreamCmd`.

`openStreamCmd` creates a `context.WithCancel` (storing `blockCancel` on the view), calls
`StreamChat`, stores the returned channel in `blockCh`, and returns `v.nextBlockCmd()`.

**2d. Update**

```go
func (v *LiveChatView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        // Update viewport dimensions, rebuildContent at new width.

    case liveChatMetaMsg:
        // Store run metadata; set header fields.

    case liveChatSnapshotMsg:
        // Populate v.attempts with historical blocks; rebuildContent; open SSE stream.

    case chatBlockMsg:
        // Append block to v.blocks and v.attempts[block.AttemptNo].
        // Update current attempt if this is a newer attempt.
        // appendRenderedBlock(block) to v.content.
        // vp.SetContent(v.content).
        // If v.follow: vp.GotoBottom().
        // Return nextBlockCmd() to read the next SSE block.

    case chatStreamDoneMsg:
        v.streamDone = true

    case hijackSessionMsg:
        if msg.err != nil {
            v.hijackErr = msg.err
            v.hijacking = false
            return v, nil
        }
        s := msg.session
        cmd := exec.Command(s.AgentBinary, s.ResumeArgs()...)
        cmd.Dir = s.CWD
        return v, tea.ExecProcess(cmd, func(err error) tea.Msg {
            return hijackReturnMsg{runID: v.runID, err: err}
        })

    case hijackReturnMsg:
        v.hijacking = false
        // Insert divider block and refresh run state.
        return v, tea.Batch(v.fetchMetaCmd(), v.openStreamCmd())

    case tea.KeyPressMsg:
        // Esc: cancel SSE, return PopViewMsg.
        // h:   start hijack.
        // f:   toggle follow.
        // [:   previous attempt.
        // ]:   next attempt.
        // ↑/k/PageUp: scroll up, disable follow.
        // ↓/j/PageDown: scroll down.
        // Delegate scroll keys to viewport.
    }
    return v, nil
}
```

**2e. View**

```go
func (v *LiveChatView) View() string {
    var b strings.Builder
    b.WriteString(v.renderHeader())
    b.WriteString(v.viewport.View())
    b.WriteString(v.renderHelpBar())
    return b.String()
}
```

Header layout (matches design doc §3.3):
```
SMITHERS › Chat › {runID} ({workflowName})              [Esc] Back
Agent: {engine} │ Node: {nodeID} │ Attempt: {N} of {M} │ ⏱ {elapsed}
─────────────────────────────────────────────────────────────────────
```

Help bar:
```
[h] Hijack  [f] Follow/Unfollow  [↑↓] Scroll  [[ / ]] Attempt  [Esc] Back
```

**2f. Block rendering (appendRenderedBlock)**

Convert a `ChatBlock` into a rendered string fragment and append it to `v.content`. The
timestamp prefix is `[MM:SS]` relative to run start (faint style). Then:

- `role == "user"`: Render as `UserMessageItem`. The prompt content may be long; truncate
  with expand hint if it exceeds 20 lines.
- `role == "assistant"`: Accumulate into an in-progress assistant message. If a previous
  assistant block for the same `ToolCallID == ""` exists, append text to it. When a
  `chat.done` event arrives, finalize.
- `role == "tool_call"`: Create the appropriate `ToolMessageItem` using the same name-switch
  as `ExtractMessageItems()`. Render with `ToolStatusRunning`.
- `role == "tool_result"`: Find the pending `ToolMessageItem` with matching `ToolCallID`,
  call `SetResult(result)` and `SetStatus(ToolStatusSuccess)`, re-render the item, replace
  its rendered string in `v.content`.

Because the viewport holds a single flat string, tool call + result pairs require either:
1. Re-rendering the full content string on each result (simplest, correct for v1), or
2. Maintaining a `[]renderedBlock` slice and joining on `SetContent`.

Use option 2: maintain `v.renderedBlocks []string` and call
`v.viewport.SetContent(strings.Join(v.renderedBlocks, "\n"))`.

**2g. Attempt navigation**

`v.attempts` is a `map[int][]smithers.ChatBlock`. `v.currentAttempt` tracks which attempt
is displayed. `[` decrements (min 1), `]` increments (max = max key in map). On change:
- Filter `v.renderedBlocks` to only blocks for `v.currentAttempt`.
- Rebuild `v.content` and call `vp.SetContent`.
- Show "Attempt N of M" in header.
- If `M == 1`, hide the attempt navigation hints from the help bar.

When `v.currentAttempt == max(attempts)` (the latest), the SSE stream continues to append.
When viewing a past attempt, new SSE blocks (which arrive for the latest attempt) still get
stored in `v.attempts[block.AttemptNo]` but are not displayed until the user navigates to
that attempt. Display an unobtrusive badge: `(N new blocks in latest attempt)`.

---

### Step 3 — Wire Navigation Entry Points

**3a. `internal/ui/dialog/actions.go`**

Add a `LiveChatAction` that accepts a run ID string and emits a `PushViewMsg` (or equivalent
router command). The action is reachable via `/chat <run_id>` in the command palette.

**3b. `internal/ui/dialog/commands.go`**

Add a `liveChatCommand` entry with name `"chat"`, description `"View live agent chat output"`,
and argument hint `"<run-id>"`. Wire it to call the `LiveChatAction` with the provided run ID.

**3c. `internal/ui/model/ui.go`**

In the key handler that already pushes Smithers views onto the router, add:

```go
case actionLiveChat:
    cmd := m.viewRouter.Push(views.NewLiveChatView(m.smithersClient, msg.RunID))
    m.state = uiSmithersView
    return m, cmd
```

The `c` key in `RunsView` (when a run is selected) should emit this same action message with
the selected run's ID.

---

### Step 4 — Hijack Button Integration

**4a. `h` key in LiveChatView**

The `h` key handler:
1. Sets `v.hijacking = true` (renders a brief "Hijacking…" overlay on next `View()` call).
2. Returns `v.hijackRunCmd()`:

```go
func (v *LiveChatView) hijackRunCmd() tea.Cmd {
    return func() tea.Msg {
        session, err := v.client.HijackRun(context.Background(), v.runID)
        return hijackSessionMsg{session: session, err: err}
    }
}
```

**4b. `h` key in RunsView**

The runs view also needs a `h` binding. It follows the same pattern:
1. Gets the selected run's ID.
2. Calls `HijackRun` via a tea.Cmd.
3. On `hijackSessionMsg`, fires `tea.ExecProcess`.
4. On `hijackReturnMsg`, refreshes the run list.

This keeps hijack accessible without requiring the user to first open the chat viewer.

**4c. Binary resolution**

Before calling `tea.ExecProcess`, validate the agent binary with `exec.LookPath`. If the
binary is not found, show a toast notification:
`"Cannot hijack: {engine} binary not found. Install it or check PATH."` and abort.

If `HandoffToProgram` from `eng-hijack-handoff-util` is available, use it instead of
constructing `exec.Command` inline. The utility handles `LookPath`, env merging, and the
`HandoffReturnMsg` type.

**4d. Post-hijack refresh**

On `hijackReturnMsg`:
1. Cancel any active SSE stream.
2. Call `fetchMetaCmd()` to reload run status.
3. Call `fetchSnapshotCmd()` to get the complete updated chat history (the agent may have
   made progress during the hijack session).
4. Reopen the SSE stream if the run is still in `"running"` state.
5. Append a divider to `v.renderedBlocks`:
   `"─ ─ ─ ─ ─ ─ HIJACK SESSION ENDED ─ ─ ─ ─ ─ ─"` (faint style).

---

### Step 5 — Testing

**5a. Unit tests — `internal/smithers/client_test.go`**

- `TestGetChatOutput_HTTP` — two attempts returned, parsed correctly.
- `TestStreamChat_SSE_ThreeBlocks` — verify three blocks arrive in order.
- `TestStreamChat_SSE_Reconnect` — server closes connection; client returns `ErrServerUnavailable`.
- `TestHijackRun_HTTP` — returns `HijackSession`, fields match.
- `TestHijackRun_NoServer` — returns `ErrServerUnavailable`.

**5b. Unit tests — `internal/ui/views/livechat_test.go`**

- `TestLiveChatView_InitDispatchesMetaAndSnapshot` — `Init()` returns a batch of two commands.
- `TestLiveChatView_PopOnEsc` — Esc key emits `PopViewMsg`.
- `TestLiveChatView_FollowMode` — new block received, viewport at bottom when follow=true.
- `TestLiveChatView_UnfollowOnScroll` — UpArrow disables follow.
- `TestLiveChatView_AttemptNavigation` — `]` key advances attempt, `[` key decrements.
- `TestLiveChatView_HijackFlow` — `h` key → `hijackSessionMsg` → `hijackReturnMsg` → view
  refreshes. Verify no `tea.ExecProcess` call if binary not found (mocked `LookPath`).
- `TestLiveChatView_RenderHeader_TwoAttempts` — header shows "Attempt 2 of 2".

**5c. Terminal E2E test — `tests/tui/livechat_e2e_test.go`**

Modeled on the Smithers upstream E2E harness (`../smithers/tests/tui-helpers.ts`) which uses
`os/exec` + piped stdin/stdout. The Go harness provides:
- `waitForText(t, buf, text, timeout)` — polls stripped terminal output.
- `sendKeys(stdin, keys)` — writes keystrokes.
- `snapshot(buf) string` — ANSI-stripped current buffer content.

Test cases:
1. `TestLiveChatView_OpenAndPop` — launch TUI, open live chat via command palette `/chat
   demo-run`, assert header text appears, press Esc, assert header gone.
2. `TestLiveChatView_BlocksAppear` — with a mock SSE server, assert streamed blocks appear
   in the viewport.
3. `TestLiveChatView_FollowToggle` — press `f`, assert help bar text changes.

**5d. VHS tape — `tests/vhs/livechat-happy-path.tape`**

```
Output tests/vhs/livechat-happy-path.gif
Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800

Type "go run . "
Sleep 1s
Type "/chat demo-run"
Enter
Sleep 2s
Screenshot tests/vhs/livechat-open.png
Type "f"
Sleep 500ms
Screenshot tests/vhs/livechat-follow-off.png
Type "f"
Sleep 500ms
Escape
Sleep 500ms
Screenshot tests/vhs/livechat-back.png
```

---

## File Plan

| # | File | Action |
|---|------|--------|
| 1 | `internal/smithers/types.go` | Add `ChatAttempt`, `ChatBlock`, `HijackSession` |
| 2 | `internal/smithers/client.go` | Add `GetChatOutput`, `StreamChat`, `HijackRun` |
| 3 | `internal/smithers/client_test.go` | Tests for new client methods |
| 4 | `internal/ui/views/livechat.go` | Full `LiveChatView` implementation |
| 5 | `internal/ui/views/livechat_test.go` | Unit tests |
| 6 | `internal/ui/util/handoff.go` | `HandoffToProgram` (if not from prereq ticket) |
| 7 | `internal/ui/model/ui.go` | Wire `actionLiveChat`; wire `c` key in runs list |
| 8 | `internal/ui/dialog/actions.go` | `LiveChatAction` |
| 9 | `internal/ui/dialog/commands.go` | `chat` command palette entry |
| 10 | `tests/tui/livechat_e2e_test.go` | Terminal E2E tests |
| 11 | `tests/vhs/livechat-happy-path.tape` | VHS recording |

---

## Validation

```bash
# 1. Unit tests
go test ./internal/smithers -run TestGetChatOutput -v
go test ./internal/smithers -run TestStreamChat -v
go test ./internal/smithers -run TestHijackRun -v
go test ./internal/ui/views -run TestLiveChatView -v
go test ./...

# 2. E2E test
go test ./tests/tui -run TestLiveChatView -v -timeout 60s

# 3. VHS recording
vhs tests/vhs/livechat-happy-path.tape

# 4. Manual smoke
go run .
# Type: /chat demo-run
# Observe: header shows run metadata, blocks stream in
# Press: ↑ (unfollow), f (re-follow, snaps to bottom)
# Press: ] (next attempt if multiple), [ (previous)
# Press: h (hijack — binary not found path should show toast)
# Press: Esc (return to previous screen)
```

---

## Open Questions

1. **Streaming fallback**: If the SSE stream is unavailable (no server, only SQLite), should
   the viewer show a static snapshot with a "Live streaming unavailable" banner, or show an
   error and pop back? Recommendation: show static snapshot with banner.

2. **Tool renderer reuse**: The live chat blocks need to construct `message.ToolCall` and
   `message.ToolResult` structs to pass to existing `NewBashToolMessageItem`,
   `NewFileToolMessageItem`, etc. Confirm that `message.ToolCall` and `message.ToolResult`
   can be constructed without a live agent session (they are plain data structs, not tied to
   the Crush agent loop). Based on the code review this is true — they hold only JSON/string
   fields.

3. **Attempt tab display**: For runs with many attempts (10+), a `[` / `]` key navigation
   scheme is unwieldy. Consider a numeric indicator `1 2 ●3 4` that is clickable or
   navigable with number keys. Defer to v2 unless design specifies otherwise.

4. **SSE reconnection**: If the SSE connection drops mid-stream (network hiccup, server
   restart), should `StreamChat` transparently reconnect from the last-seen block
   (tracking sequence IDs), or surface the disconnect to the view? Recommendation: surface
   it — show a "Reconnecting…" status in the header and retry with exponential backoff.
   Cap retries at 5 before showing a permanent error state.

5. **Performance for long sessions**: Agent runs with hundreds of tool calls will produce
   large content strings. The v1 `strings.Join(v.renderedBlocks, "\n")` approach re-renders
   the full string on each new block. For sessions with >500 blocks, consider appending
   directly to the viewport's internal buffer or implementing a virtual scroll. Defer to v2
   unless benchmarks show >100ms per block render.

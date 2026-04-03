# Scaffold Live Chat View Structure

## Metadata
- ID: eng-live-chat-scaffolding
- Group: Live Chat And Hijack (live-chat-and-hijack)
- Type: engineering
- Feature: n/a
- Dependencies: none

## Summary

Create the baseline Bubble Tea view model for the Live Chat Viewer to be routed by the new view stack manager.

## Acceptance Criteria

- LiveChatView struct implements the View interface.
- Can be pushed to the Router stack.

## Source Context

- internal/ui/views/livechat.go
- internal/ui/views/router.go

## Implementation Notes

- Define LiveChatView which will hold the runID and SmithersClient.
- Ensure Init() returns a command to start streaming.

---

## Objective

Introduce the foundational `View` interface, `Router` view-stack manager, and a working `LiveChatView` scaffold into the Crush codebase. After this ticket, the TUI can push a Live Chat view for a given run ID onto a navigation stack, stream placeholder data, render it with Crush's existing message infrastructure, and pop back to the default chat. This establishes the view-routing pattern that every subsequent Smithers view (runs, workflows, agents, etc.) will follow.

## Scope

### In scope

1. **`internal/ui/views/` package** — new directory with three files: `view.go` (interface), `router.go` (stack manager), `livechat.go` (first concrete view).
2. **`View` interface** — `Init`, `Update`, `Draw`, `Name`, `ShortHelp` matching Crush's Bubble Tea v2 rendering model (the `Draw(scr uv.Screen, area uv.Rectangle)` pattern, not legacy `View() string`).
3. **`Router` struct** — push/pop view stack with the existing chat always at index 0.
4. **`LiveChatView` struct** — holds `runID`, `*smithers.Client` (initially nil-safe / stubbed), `viewport`, `following` flag, `cancelFn`; implements the `View` interface.
5. **`LiveChatView.Init()`** — returns a `tea.Cmd` that starts an SSE/polling subscription for chat blocks (initially via a stub channel returning sample data).
6. **Integration point in `internal/ui/model/ui.go`** — add a `router *views.Router` field to the `UI` struct and wire `Update`/`Draw` delegation so that when the router's current view is not chat, the router's view gets the screen.
7. **Keybinding** — a provisional key (e.g., `/chat <id>` via command dialog, or a direct `c` key when a run ID is in context) that pushes a `LiveChatView` onto the stack, and `Esc` pops it.
8. **Stub Smithers client type** — minimal `internal/smithers/types.go` defining `Run`, `ChatBlock`, and `Client` interface / struct with `GetChatOutput` and `StreamChat` stubs returning hardcoded data so the view can render without a real Smithers server.
9. **Terminal E2E test** — one test in the `@microsoft/tui-test` harness style verifying the view can be pushed and popped.
10. **VHS tape** — one `.tape` file showing the happy path: launch → push live chat → see messages → pop back.

### Out of scope

- Real Smithers HTTP client implementation (`internal/smithers/client.go` with live API calls).
- SSE event streaming from a running Smithers server.
- Hijack/handoff (`tea.ExecProcess` to agent CLIs).
- Tool-call rendering of Smithers chat blocks (ticket `feat-live-chat-tool-call-rendering`).
- Follow-mode auto-scroll (ticket `feat-live-chat-follow-mode`).
- Attempt tracking and retry history UI.
- Side-by-side split-pane layout.
- Run dashboard integration (pressing `c` on a run row).

## Implementation Plan

### Slice 1: View interface and Router (`internal/ui/views/view.go`, `router.go`)

**Files**: `internal/ui/views/view.go`, `internal/ui/views/router.go`

1. Create `internal/ui/views/view.go`:
   ```go
   package views

   import (
       tea "charm.land/bubbletea/v2"
       uv "github.com/charmbracelet/ultraviolet"
   )

   // View is the interface every routable Smithers view implements.
   // It follows Crush's Bubble Tea v2 Draw-based rendering model.
   type View interface {
       Init() tea.Cmd
       Update(msg tea.Msg) (View, tea.Cmd)
       Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor
       Name() string
       ShortHelp() []string
   }
   ```

2. Create `internal/ui/views/router.go`:
   ```go
   type Router struct {
       stack []View
   }

   func NewRouter() *Router
   func (r *Router) Push(v View) tea.Cmd   // appends to stack, calls v.Init()
   func (r *Router) Pop()                   // removes top if len > 0
   func (r *Router) Current() View          // returns stack[len-1], nil if empty
   func (r *Router) Depth() int             // len(stack)
   func (r *Router) Clear()                 // empties entire stack
   ```

   The Router does NOT hold the chat view — the existing `UI` model owns chat rendering. The router stack is empty when the user is in chat, and non-empty when a Smithers view is active. This avoids wrapping Crush's 2,850-line `UI` model behind the `View` interface (which would be a massive, risky refactor).

**Why this approach**: Crush's `UI` struct uses `Draw(scr, area)` with `uv.Screen`, not `View() string`. The new `View` interface must match this pattern so views compose into the same Ultraviolet rendering pipeline. Keeping the router as a sidecar to `UI` (rather than replacing it) means zero changes to the existing chat/session/dialog stack — the router only activates when a Smithers view is pushed.

**Rationale for not wrapping chat as a View**: The `UI` model manages focus state, textarea, completions, dialog overlay, sidebar, header, attachments, and 20+ message types. Extracting it into a `View` would touch hundreds of lines for no scaffolding benefit. Instead, `UI.Draw()` checks `router.Depth() > 0` and delegates to the router's current view.

### Slice 2: Stub Smithers types (`internal/smithers/types.go`)

**File**: `internal/smithers/types.go`

Define the minimal domain types the LiveChatView needs:

```go
package smithers

type ChatBlock struct {
    Role      string    // "system", "user", "assistant", "tool_call", "tool_result"
    Content   string
    Timestamp int64     // Unix ms, relative to run start
    ToolName  string    // populated for tool_call / tool_result
    ToolID    string
}

type Run struct {
    ID         string
    WorkflowID string
    Status     string   // "running", "paused", "completed", "failed"
    AgentName  string
    NodeName   string
    Attempt    int
    StartedAt  int64
}

// Client is the interface for Smithers data access.
// Scaffolding provides a StubClient; real HTTP/SQLite client comes later.
type Client interface {
    GetRun(ctx context.Context, id string) (*Run, error)
    GetChatOutput(ctx context.Context, runID string) ([]ChatBlock, error)
    StreamChat(ctx context.Context, runID string) (<-chan ChatBlock, error)
}
```

**File**: `internal/smithers/stub.go`

A `StubClient` implementing `Client` that returns hardcoded data: a sample `Run` and a slice of `ChatBlock` items showing a system prompt, an assistant message, and a tool-call/result pair. `StreamChat` returns a channel that sends the blocks with 200ms delays, then closes. This lets the view render real-looking content without a Smithers server.

### Slice 3: LiveChatView model (`internal/ui/views/livechat.go`)

**File**: `internal/ui/views/livechat.go`

```go
type LiveChatView struct {
    runID     string
    client    smithers.Client
    run       *smithers.Run       // fetched metadata (agent, node, attempt, elapsed)
    blocks    []smithers.ChatBlock
    viewport  viewport.Model      // charm.land/bubbles/v2/viewport
    following bool                // auto-scroll (on by default)
    width     int
    height    int
    styles    *styles.Styles
    cancelFn  context.CancelFunc  // cancels StreamChat goroutine
    err       error
}
```

**Methods**:

| Method | Behavior |
|--------|----------|
| `NewLiveChatView(runID string, client smithers.Client, styles *styles.Styles) *LiveChatView` | Constructor; sets `following = true` |
| `Init() tea.Cmd` | Returns a batch of two commands: (1) fetch run metadata, (2) start chat stream subscription |
| `Update(msg tea.Msg) (View, tea.Cmd)` | Handles `chatBlockMsg` (append block, re-render viewport), `runMetaMsg` (set run header), `tea.KeyPressMsg` (`f` toggles follow, `Esc` signals pop via `popViewMsg`), `tea.WindowSizeMsg` (resize viewport), `errMsg` |
| `Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor` | Renders: (1) header bar with breadcrumb + run metadata, (2) viewport with rendered chat blocks, (3) footer with keybinding hints |
| `Name() string` | Returns `"chat"` |
| `ShortHelp() []string` | Returns `["[h] Hijack", "[f] Follow", "[Esc] Back"]` |

**Custom messages**:

```go
type chatBlockMsg struct{ Block smithers.ChatBlock }
type runMetaMsg   struct{ Run *smithers.Run }
type popViewMsg   struct{}   // signals Router to pop
type errMsg       struct{ Err error }
```

**Streaming pattern**: `Init()` returns a `tea.Cmd` that:
1. Creates a child context from `context.Background()`.
2. Calls `client.StreamChat(ctx, runID)`.
3. Returns a `tea.BatchMsg` of commands — one per block received — using the tea command pattern for long-running subscriptions.
4. Stores `cancelFn` so the view can stop streaming when popped.

**Rendering**: Each `ChatBlock` is rendered as plain styled text in the viewport (not as full Crush `MessageItem` objects — that mapping is a separate ticket). Format:

```
[00:02] Assistant
  I'll start by reading the auth middleware files.

[00:05] Tool Call: read
  src/auth/middleware.ts

[00:05] Tool Result: read
  142 lines read
```

Timestamps are relative to `run.StartedAt`, formatted as `[MM:SS]`.

### Slice 4: Wire Router into UI model (`internal/ui/model/ui.go`)

**Changes to `internal/ui/model/ui.go`**:

1. Add field `router *views.Router` to the `UI` struct (after line ~175).

2. Initialize in `New()`:
   ```go
   m.router = views.NewRouter()
   ```

3. In `Update()`, before existing key handling, intercept `popViewMsg`:
   ```go
   case views.PopViewMsg:
       m.router.Pop()
       return m, nil
   ```

4. In `Update()`, when a Smithers view is active (`m.router.Depth() > 0`), delegate to it:
   ```go
   if m.router.Depth() > 0 {
       updated, cmd := m.router.Current().Update(msg)
       m.router.ReplaceCurrent(updated)
       return m, cmd
   }
   ```

5. In `Draw()`, when a Smithers view is active, delegate the main content area:
   ```go
   if m.router.Depth() > 0 {
       cursor = m.router.Current().Draw(scr, m.layout.main)
   } else {
       // existing chat/landing/onboarding draw logic
   }
   ```
   The status bar, dialog overlay, and notifications still render on top — they are not owned by the routed view.

6. Add a temporary way to push a LiveChatView for testing. Options:
   - Register a `/chat` command in the command palette dialog that accepts a run ID argument.
   - Or add a keybinding `ctrl+l` (provisional, to be replaced by run dashboard `c` key later).

**Key design decision**: The router delegates `Update` only when depth > 0. When depth is 0, the existing UI update loop runs unchanged. This means zero behavioral change to the chat experience — the router is inert until explicitly used.

### Slice 5: Terminal E2E test

**File**: `tests/livechat_e2e_test.go` (or `tests/tui_livechat_test.go`)

Model the test on the upstream `@microsoft/tui-test` harness pattern from `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`. The upstream pattern:

```typescript
// From tui-helpers.ts
interface TUITestInstance {
    waitForText(text: string, timeoutMs?: number): Promise<void>;
    waitForNoText(text: string, timeoutMs?: number): Promise<void>;
    sendKeys(text: string): void;
    snapshot(): string;
    terminate(): Promise<void>;
}
```

For Go, implement a minimal test helper in `tests/tui_helpers_test.go`:

```go
type TUITestInstance struct {
    program *tea.Program
    buf     *screenBuffer   // captures Draw output
}

func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error
func (t *TUITestInstance) SendKey(key tea.KeyPressMsg)
func (t *TUITestInstance) Snapshot() string
func (t *TUITestInstance) Terminate()
```

Alternatively, spawn the binary as a subprocess (matching the BunSpawn pattern) and read stdout with ANSI stripping.

**Test cases**:

1. **Push LiveChatView**: Launch TUI → trigger `/chat stub-run-id` → `WaitForText("SMITHERS › Chat")` → assert header contains run ID and agent name.
2. **Chat blocks render**: After push, `WaitForText("[00:00]")` → `WaitForText("Assistant")` → verify at least one chat block is visible.
3. **Pop back to chat**: Send `Esc` key → `WaitForNoText("SMITHERS › Chat")` → verify we're back at the main chat input.
4. **Snapshot correctness**: Take a `Snapshot()` after blocks render and verify it contains the expected structure (breadcrumb, timestamps, message content).

### Slice 6: VHS recording tape

**File**: `tests/tapes/livechat-happy-path.tape`

```
# Live Chat Viewer — Happy Path
Output tests/tapes/livechat-happy-path.gif
Set Shell "bash"
Set FontSize 14
Set Width 120
Set Height 40
Set Theme "Catppuccin Mocha"

# Launch the TUI
Type "smithers-tui"
Enter
Sleep 2s

# Open command palette and navigate to live chat
Type "/chat stub-run-id"
Enter
Sleep 1s

# Verify the live chat view renders
Sleep 3s

# Toggle follow mode
Type "f"
Sleep 1s

# Pop back to chat
Escape
Sleep 1s
```

This produces a GIF recording that serves as both a visual regression artifact and documentation. CI runs `vhs tests/tapes/livechat-happy-path.tape` and fails if the binary exits non-zero.

## Validation

### Automated checks

| Check | Command | Expected |
|-------|---------|----------|
| Package compiles | `go build ./internal/ui/views/...` | Exit 0, no errors |
| Smithers stub compiles | `go build ./internal/smithers/...` | Exit 0, no errors |
| Full binary builds | `go build -o smithers-tui .` | Exit 0 |
| Unit tests | `go test ./internal/ui/views/... -v` | Router push/pop/depth tests pass |
| Unit tests | `go test ./internal/smithers/... -v` | StubClient returns expected data |
| Terminal E2E: push | `go test ./tests/ -run TestLiveChatPush -v` | `WaitForText("SMITHERS › Chat")` succeeds within 5s |
| Terminal E2E: render | `go test ./tests/ -run TestLiveChatRender -v` | `WaitForText("[00:00]")` and `WaitForText("Assistant")` succeed |
| Terminal E2E: pop | `go test ./tests/ -run TestLiveChatPop -v` | `WaitForNoText("SMITHERS › Chat")` succeeds after Esc |
| VHS recording | `vhs tests/tapes/livechat-happy-path.tape` | Exit 0, GIF produced at expected path |
| Lint | `golangci-lint run ./internal/ui/views/... ./internal/smithers/...` | No lint errors |

### Terminal E2E coverage (modeled on upstream `@microsoft/tui-test`)

The E2E tests use a Go adaptation of the `TUITestInstance` pattern from `../smithers/tests/tui-helpers.ts`:

- **Process spawning**: Launch the compiled binary as a subprocess with `TERM=xterm-256color`, pipe stdout/stderr into a buffer.
- **ANSI stripping**: Strip escape sequences with the same regex pattern: `\x1B\[[0-9;]*[a-zA-Z]`.
- **Text matching**: `WaitForText` polls the stripped buffer at 100ms intervals with a 10s default timeout (matching upstream defaults).
- **Key input**: Write to the process's stdin pipe.
- **Snapshot**: Return the current stripped buffer contents for assertion.

This matches the upstream `BunSpawnBackend` implementation but in Go using `os/exec` + `io.Pipe`.

### VHS happy-path recording test

The VHS tape in `tests/tapes/livechat-happy-path.tape` exercises the full flow: launch → push LiveChatView → observe streamed blocks → toggle follow → pop back. CI runs this tape; failure means the TUI crashed or produced no output. The resulting GIF is archived as a test artifact.

### Manual verification

1. Build and run: `go build -o smithers-tui . && ./smithers-tui`
2. Type `/chat stub-run-id` in the command palette → verify the Live Chat view appears with the breadcrumb header `SMITHERS › Chat › stub-run-id`.
3. Verify chat blocks stream in with timestamps and role labels.
4. Press `f` → verify the footer shows follow mode toggled.
5. Press `Esc` → verify return to normal chat.
6. Resize terminal while in Live Chat view → verify the viewport reflows without panic.

## Risks

### 1. Bubble Tea v2 `Draw()` integration complexity

**Risk**: Crush uses `Draw(scr uv.Screen, area uv.Rectangle)` with Ultraviolet screen buffers, not the legacy `View() string` API. The new `View` interface must use `Draw()` too, but integrating a second `Draw`-based component into `UI.Draw()` requires understanding exactly how Ultraviolet composites screen regions.

**Mitigation**: The `Draw()` call already receives a sub-rectangle (`m.layout.main`). Passing this same rectangle to `router.Current().Draw(scr, area)` is the identical pattern used by `m.chat.Draw()`, `m.header.Draw()`, etc. No new Ultraviolet concepts needed — just delegation.

### 2. Message routing when Router is active

**Risk**: When a Smithers view is on the stack, the `UI.Update()` method must route messages to the view instead of the normal chat/editor/dialog handlers. If routing is wrong, key presses could leak to the textarea or dialog stack.

**Mitigation**: Gate on `m.router.Depth() > 0` at the top of `Update()`, before any existing key handling. The dialog overlay still gets first crack (it already intercepts at the very top of `Update()`), so modal dialogs work regardless of router state. The textarea and chat handlers are skipped entirely when a view is active.

### 3. Crush module path not yet renamed

**Risk**: The engineering doc specifies renaming the Go module from `github.com/charmbracelet/crush` to `github.com/anthropic/smithers-tui`. If this rename hasn't happened yet (current imports show `github.com/charmbracelet/crush`), the new `internal/ui/views/` and `internal/smithers/` packages must use the current module path, then be updated during the rebrand ticket (`platform-smithers-rebrand`).

**Mitigation**: Use the current module path (`github.com/charmbracelet/crush/internal/ui/views` and `github.com/charmbracelet/crush/internal/smithers`). The rebrand's `sed` pass will catch these.

### 4. StubClient divergence from eventual real Client

**Risk**: The `smithers.Client` interface defined in scaffolding may not match the final HTTP API shape, leading to rework when the real client lands.

**Mitigation**: Keep the interface minimal — only `GetRun`, `GetChatOutput`, `StreamChat` for now. These three methods map directly to known Smithers server endpoints (`GET /v1/runs/{id}`, `GET /v1/runs/{id}/events` for chat). The interface is easy to extend; existing methods are unlikely to change shape since they mirror the upstream API.

### 5. No real Smithers server for E2E tests

**Risk**: The E2E tests run against the `StubClient`, not a real Smithers server. This validates view plumbing but not actual data integration.

**Mitigation**: This is acceptable for a scaffolding ticket. The stub provides predictable data for deterministic tests. Integration with a real server is covered by `eng-live-chat-e2e-testing` which depends on `eng-smithers-client-runs`.

### 6. VHS dependency in CI

**Risk**: VHS (`charmbracelet/vhs`) must be installed in CI to run tape tests. If CI doesn't have it, the recording test is skipped or fails.

**Mitigation**: Gate the VHS test behind a build tag (`//go:build vhs`) or check for the `vhs` binary at test start and skip with `t.Skip("vhs not found")`. Document the CI setup requirement. VHS is a single Go binary, easy to add to CI.

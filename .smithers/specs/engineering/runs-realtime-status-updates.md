# Engineering Spec: Stream Real-time Run Updates

## Metadata
- Ticket: `runs-realtime-status-updates`
- Feature: `RUNS_REALTIME_STATUS_UPDATES`
- Group: Runs And Inspection
- Dependencies: `runs-dashboard` (RunsView scaffold, `components.RunTable`, `smithers.Client.ListRuns`)
- Target files:
  - `internal/ui/views/runs.go` (modify — add SSE subscription, context management, event handling)
  - `internal/smithers/runs.go` (modify — add `WaitForAllEvents` tea.Cmd helper)
  - `tests/runs_realtime_e2e_test.go` (new)
  - `tests/runs_realtime.tape` (new — VHS recording)

---

## Objective

Subscribe `RunsView` to the Smithers global SSE event stream so that run status changes propagate to the dashboard in real-time without requiring the user to press `r` to refresh. When the SSE stream is unavailable (no server, endpoint missing), fall back to a 5-second auto-poll and show a "Polling" indicator.

This corresponds to the GUI's real-time behavior in `RunsList.tsx`, which subscribes to the global event feed via the transport layer and applies status patches to its React state on each event.

---

## Scope

### In scope
- SSE subscription in `RunsView.Init()` using `client.StreamAllEvents(ctx)`
- Context management: per-view `context.Context` created on `Init()`, cancelled on `PopViewMsg`
- `WaitForAllEvents` `tea.Cmd` helper in `internal/smithers/runs.go` (wraps `StreamAllEvents` channel)
- In-place status update: when a `RunEventMsg` arrives, patch the matching `RunSummary` in `v.runs`
- New run insertion: when `RunStarted` event arrives for an unknown `RunID`, insert a stub `RunSummary`; enrich asynchronously via `GetRunSummary`
- Terminal-run handling: when a run reaches a terminal state, stop tracking it as active (but keep it in the list for display)
- Auto-poll fallback: when `StreamAllEvents` returns `ErrServerUnavailable`, start a `time.Ticker` at 5 s
- Mode indicator: render `● Live` (green) or `○ Polling` (dim) in the header based on active connection type
- SSE reconnect on disconnect: when `RunEventDoneMsg` arrives and the view context is still alive, re-launch `StreamAllEvents`
- Graceful teardown: cancel the view context in the `PopViewMsg` handler before returning `PopViewMsg`

### Out of scope
- Per-run SSE subscriptions (`StreamRunEventsWithReconnect`) — used by the Live Chat Viewer, not the dashboard
- Optimistic status mutations (no write operations in this ticket)
- Status sectioning / grouping rows by Active/Completed/Failed (`RUNS_STATUS_SECTIONING` — separate ticket)
- Approval badge / `⚠` indicator in the row (`RUNS_INLINE_RUN_DETAILS`)
- Progress bar visualization (`RUNS_PROGRESS_VISUALIZATION`)

---

## Data Flow

```
RunsView.Init()
  ├── dispatch loadRunsCmd             → smithers.Client.ListRuns(ctx) → runsLoadedMsg
  └── dispatch WaitForAllEvents(ctx)   → smithers.Client.StreamAllEvents(ctx) → ch

On runsLoadedMsg:
  v.runs = msg.runs
  v.streamMode = "live" | "polling"

On RunEventMsg (from WaitForAllEvents):
  applyRunEvent(v.runs, event)
  re-dispatch WaitForAllEvents(ctx)   ← self-re-scheduling

On RunEventDoneMsg:
  if v.ctx.Err() == nil:
    re-dispatch WaitForAllEvents(ctx) ← reconnect

On runsStreamUnavailableMsg:
  start v.pollTicker (5 s)
  v.streamMode = "polling"

On tickMsg (from pollTicker):
  re-dispatch loadRunsCmd

On PopViewMsg:
  v.cancel()                          ← cancels context, unblocks goroutines
  return PopViewMsg{}
```

---

## Implementation Plan

### Slice 1: Add `WaitForAllEvents` cmd helper to `internal/smithers/runs.go`

**File**: `internal/smithers/runs.go`

Add a function that reads one value from the `StreamAllEvents` channel and returns the appropriate `tea.Msg`. This mirrors the `WaitForRunEvent` pattern in `events.go` but operates on the `interface{}` channel from `StreamAllEvents`.

```go
// WaitForAllEvents returns a tea.Cmd that blocks on the next message from the
// global event channel returned by StreamAllEvents.
//
// When an event arrives it returns RunEventMsg.
// When a parse error occurs it returns RunEventErrorMsg (stream continues —
// re-dispatch this cmd to keep receiving).
// When the stream closes it returns RunEventDoneMsg.
//
// Typical usage in RunsView:
//
//   // In Init:
//   ch, err := client.StreamAllEvents(ctx)
//   if err != nil {
//       return runsStreamUnavailableMsg{}
//   }
//   v.allEventsCh = ch
//   return smithers.WaitForAllEvents(v.allEventsCh)
//
//   // In Update:
//   case smithers.RunEventMsg:
//       v.applyRunEvent(msg.Event)
//       return v, smithers.WaitForAllEvents(v.allEventsCh)
//   case smithers.RunEventDoneMsg:
//       // Stream closed; reconnect if view context is still live.
func WaitForAllEvents(ch <-chan interface{}) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok {
            return RunEventDoneMsg{}
        }
        return msg // already typed RunEventMsg / RunEventErrorMsg / RunEventDoneMsg
    }
}
```

**Note**: `StreamAllEvents` sends pre-typed values (`RunEventMsg`, `RunEventErrorMsg`, `RunEventDoneMsg`) into the `interface{}` channel. `WaitForAllEvents` returns the value directly — Bubble Tea's runtime receives the concrete type and routes it to the correct `case` in `Update`.

**Verification**: `go build ./internal/smithers/...` — no new dependencies introduced (the package already imports `tea`).

---

### Slice 2: Extend `RunsView` state for streaming

**File**: `internal/ui/views/runs.go`

Add context management and stream state fields to `RunsView`:

```go
// New internal message types
type runsStreamUnavailableMsg struct{}
type runsEnrichRunMsg struct {
    run smithers.RunSummary
}

// Updated RunsView struct
type RunsView struct {
    client     *smithers.Client
    runs       []smithers.RunSummary
    cursor     int
    width      int
    height     int
    loading    bool
    err        error
    // Streaming state
    ctx        context.Context
    cancel     context.CancelFunc
    allEventsCh <-chan interface{}   // global SSE channel
    streamMode string               // "live" | "polling" | "offline"
    pollTicker *time.Ticker
}
```

**Constructor update** — `NewRunsView` no longer calls `context.Background()` in the struct literal; the context is created in `Init()`:

```go
func NewRunsView(client *smithers.Client) *RunsView {
    return &RunsView{
        client:  client,
        loading: true,
    }
}
```

**Verification**: Build compiles. Existing `runsLoadedMsg` and `runsErrorMsg` are unchanged.

---

### Slice 3: Update `Init()` — start SSE or fall back to poll

**File**: `internal/ui/views/runs.go`

```go
func (v *RunsView) Init() tea.Cmd {
    v.ctx, v.cancel = context.WithCancel(context.Background())

    cmds := []tea.Cmd{
        // Always load the initial list.
        v.loadRunsCmd(),
        // Start the SSE stream (or fall back to polling).
        v.startStreamCmd(),
    }
    return tea.Batch(cmds...)
}

func (v *RunsView) loadRunsCmd() tea.Cmd {
    ctx := v.ctx
    return func() tea.Msg {
        runs, err := v.client.ListRuns(ctx, smithers.RunFilter{Limit: 50})
        if err != nil {
            if ctx.Err() != nil {
                return nil // view was popped; discard
            }
            return runsErrorMsg{err: err}
        }
        return runsLoadedMsg{runs: runs}
    }
}

func (v *RunsView) startStreamCmd() tea.Cmd {
    ctx := v.ctx
    client := v.client
    return func() tea.Msg {
        ch, err := client.StreamAllEvents(ctx)
        if err != nil {
            return runsStreamUnavailableMsg{}
        }
        // Store the channel on the view — returned via a special init msg.
        // (See note below on channel storage.)
        return runsStreamReadyMsg{ch: ch}
    }
}
```

**Note on channel storage**: Because `tea.Cmd` functions run off the main goroutine and return a `tea.Msg`, they cannot directly mutate `v.allEventsCh`. The solution is to define a `runsStreamReadyMsg` that carries the channel, store it in `Update`, then dispatch `smithers.WaitForAllEvents(v.allEventsCh)`:

```go
type runsStreamReadyMsg struct {
    ch <-chan interface{}
}
```

In `Update`:

```go
case runsStreamReadyMsg:
    v.allEventsCh = msg.ch
    v.streamMode = "live"
    return v, smithers.WaitForAllEvents(v.allEventsCh)

case runsStreamUnavailableMsg:
    v.streamMode = "polling"
    v.pollTicker = time.NewTicker(5 * time.Second)
    return v, v.pollTickCmd()
```

**Verification**: Build compiles. The `StreamAllEvents` call succeeds on a running server and returns a valid channel.

---

### Slice 4: Handle `RunEventMsg` in `Update()` — status patch

**File**: `internal/ui/views/runs.go`

```go
case smithers.RunEventMsg:
    v.applyRunEvent(msg.Event)
    return v, smithers.WaitForAllEvents(v.allEventsCh)

case smithers.RunEventErrorMsg:
    // Parse error on one event — log (or ignore) and keep listening.
    return v, smithers.WaitForAllEvents(v.allEventsCh)

case smithers.RunEventDoneMsg:
    // Stream closed. If our context is still live, reconnect.
    if v.ctx.Err() == nil {
        return v, v.startStreamCmd()
    }
    return v, nil
```

**`applyRunEvent` implementation**:

```go
func (v *RunsView) applyRunEvent(ev smithers.RunEvent) {
    // Find the run in v.runs by RunID.
    idx := -1
    for i, r := range v.runs {
        if r.RunID == ev.RunID {
            idx = i
            break
        }
    }

    switch ev.Type {
    case "RunStatusChanged", "RunFinished", "RunFailed", "RunCancelled", "RunStarted":
        if ev.Status != "" {
            if idx >= 0 {
                v.runs[idx].Status = smithers.RunStatus(ev.Status)
            } else if ev.Type == "RunStarted" {
                // New run — insert a stub and enrich asynchronously.
                stub := smithers.RunSummary{
                    RunID:  ev.RunID,
                    Status: smithers.RunStatus(ev.Status),
                }
                v.runs = append([]smithers.RunSummary{stub}, v.runs...)
                // Fire enrichment cmd
                // (handled by returning a separate cmd — see note below)
            }
        }

    case "NodeWaitingApproval":
        if idx >= 0 {
            v.runs[idx].Status = smithers.RunStatusWaitingApproval
        }
    }
}
```

**New-run enrichment**: Because `applyRunEvent` is a pure state mutation (no cmd dispatch), returning a `GetRunSummary` cmd for newly inserted runs requires returning it from the `Update` case:

```go
case smithers.RunEventMsg:
    newRun := v.applyRunEvent(msg.Event) // returns *RunSummary if new run was inserted
    cmds := []tea.Cmd{smithers.WaitForAllEvents(v.allEventsCh)}
    if newRun != nil {
        cmds = append(cmds, v.enrichRunCmd(newRun.RunID))
    }
    return v, tea.Batch(cmds...)
```

Where `applyRunEvent` returns `*RunSummary` (the inserted stub) when it adds a new entry, or `nil` otherwise. The `enrichRunCmd` fires a `GetRunSummary` call and returns `runsEnrichRunMsg` which replaces the stub.

**Verification**: Unit test: create a `RunsView` with 2 runs, call `applyRunEvent` with a `RunStatusChanged` event for run[0], assert `v.runs[0].Status` changed. Call with an unknown `RunID`, assert a new entry was prepended.

---

### Slice 5: Handle `PopViewMsg` — teardown

**File**: `internal/ui/views/runs.go`

```go
case tea.KeyPressMsg:
    switch {
    case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
        v.cancel()   // cancel ctx → closes SSE goroutine, stops poll ticker
        if v.pollTicker != nil {
            v.pollTicker.Stop()
        }
        return v, func() tea.Msg { return PopViewMsg{} }
    ...
    }
```

**Verification**: After pressing `Esc`, the SSE goroutine exits within one backoff cycle. No goroutine leak. (Verify with `-race` flag and `goleak` in the unit test teardown.)

---

### Slice 6: Mode indicator in `View()`

**File**: `internal/ui/views/runs.go`

Update the header rendering to include the stream mode indicator:

```go
func (v *RunsView) View() string {
    // ...
    modeStr := ""
    switch v.streamMode {
    case "live":
        modeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Live")
    case "polling":
        modeStr = lipgloss.NewStyle().Faint(true).Render("○ Polling")
    }
    header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Runs")
    helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
    // Pack modeStr between header and helpHint.
    ...
}
```

**Verification**: Run the TUI with a server → header shows `● Live`. Run without server → header shows `○ Polling`.

---

### Slice 7: Auto-poll fallback

**File**: `internal/ui/views/runs.go`

```go
func (v *RunsView) pollTickCmd() tea.Cmd {
    ch := v.pollTicker.C
    return func() tea.Msg {
        <-ch
        return tickMsg{}
    }
}

type tickMsg struct{}
```

In `Update`:

```go
case tickMsg:
    if v.ctx.Err() != nil {
        return v, nil // view popped
    }
    return v, tea.Batch(v.loadRunsCmd(), v.pollTickCmd())
```

**Verification**: With no server, open the runs view. Verify that `loadRunsCmd` fires every 5 s and `v.runs` updates. The `tickMsg` cycle continues until `Esc` is pressed (which cancels `v.ctx` and stops the ticker).

---

### Slice 8: E2E test

**File**: `tests/runs_realtime_e2e_test.go`

The test exercises the full round-trip: TUI opens, loads initial runs, server pushes an SSE status-change event, TUI re-renders without user input.

```
Test scenario:
1. Start a mock httptest.Server that:
   - Serves GET /v1/runs → 2 runs: "abc123" (running), "def456" (running)
   - Serves GET /v1/events → SSE stream; holds open

2. Launch TUI with SMITHERS_API_URL pointing at mock

3. Wait for table to appear with "running" for both rows

4. Server pushes SSE event:
   event: smithers
   data: {"type":"RunStatusChanged","runId":"abc123","status":"finished","timestampMs":1000}
   id: 1

5. Assert within 2 s that the TUI re-renders "finished" for abc123
   without any user keypress

6. Server pushes SSE event:
   data: {"type":"RunStarted","runId":"new999","status":"running","timestampMs":2000}
   id: 2

7. Assert within 2 s that "new999" appears in the run list

8. Press Esc
9. Assert TUI closes SSE connection (verify mock server sees connection drop)
```

**Assertions**:
| AC | Assertion |
|----|-----------|
| Dashboard subscribes to stream when active | Mock `/v1/events` handler sees a connection immediately after TUI opens |
| Status changes reflect instantly | `waitForOutput(t, pty, "finished")` resolves within 2 s of server push |
| SSE connection closed on navigate away | `waitForConnectionDrop(t, srv)` resolves within 1 s of Esc |

**Fallback test** (`TestRunsView_PollFallback`):
- Mock server returns 404 on `/v1/events`
- Assert `○ Polling` indicator appears in header
- Fast-forward time (via mock ticker injection) and assert `GET /v1/runs` is re-called

---

### Slice 9: VHS recording

**File**: `tests/runs_realtime.tape`

```tape
Output tests/runs_realtime.gif
Set FontSize 14
Set Width 120
Set Height 40

# Start TUI with a running Smithers server
Type "SMITHERS_API_URL=http://localhost:7331 go run ."
Enter
Sleep 3s

# Open runs dashboard
Ctrl+R
Sleep 2s

# Live indicator visible in header
# (status changes stream in automatically — no user input needed)
Sleep 5s

# Return to chat
Escape
Sleep 1s
```

---

## Validation

| Check | Command |
|-------|---------|
| Build | `go build ./...` |
| Unit tests | `go test ./internal/ui/views/ -run TestRunsView -v` |
| E2E | `go test ./tests/ -run TestRunsRealtime -v -timeout 30s` |
| Race detector | `go test -race ./internal/ui/views/ ./internal/smithers/...` |
| VHS | `vhs validate tests/runs_realtime.tape` |
| Manual live | Open TUI with Smithers server, navigate to runs, start a workflow, observe status update without pressing `r` |

---

## Risks

### 1. `StreamAllEvents` endpoint unavailable on older Smithers server versions
**Impact**: Server running but `/v1/events` returns 404. The view would show no live updates.
**Mitigation**: `StreamAllEvents` already returns `ErrServerUnavailable` on 404. The fallback to 5-second auto-poll kicks in automatically and the user sees `○ Polling`.

### 2. RunEvent status field absent on some event types
**Impact**: `RunEvent.Status` is empty for `NodeOutput`, `AgentEvent`, etc. Applying a blank status would corrupt the run's state.
**Mitigation**: `applyRunEvent` only updates status when `ev.Status != ""` and `ev.Type` is in the known status-change set. Other event types are silently ignored by the dashboard (they are relevant to the Live Chat Viewer, not the run list).

### 3. Goroutine leak if `cancel()` is not called
**Impact**: `StreamAllEvents` goroutine blocks on `scanner.Scan()` with no deadline, leaking forever.
**Mitigation**: `cancel()` is called in the `Esc` handler and in any future `Destroy()` hook. The unit test uses `goleak.VerifyNone` after view teardown to catch leaks during CI.

### 4. Race between initial `ListRuns` and first SSE event
**Impact**: The SSE stream may deliver a `RunStarted` event for a run that `ListRuns` also returns, causing a duplicate entry.
**Mitigation**: In `applyRunEvent`, before inserting a new run, scan `v.runs` for an existing entry with the same `RunID`. Deduplication by `RunID` before insert.

### 5. Cursor position drift after status updates
**Impact**: When `v.runs` is re-sorted by status (in future `RUNS_STATUS_SECTIONING` work), the cursor index may point to a different run.
**Mitigation**: For this ticket, `v.runs` is not re-sorted on event receipt — rows are updated in-place. Sorting is deferred to `RUNS_STATUS_SECTIONING`. Add a comment noting the deferred dependency.

### 6. `WaitForAllEvents` blocks on closed channel
**Impact**: If `v.allEventsCh` is nil when `WaitForAllEvents` is dispatched (e.g., before `runsStreamReadyMsg` is processed), the cmd would block forever on a nil channel.
**Mitigation**: A nil channel receive blocks forever in Go — which would silently hang the view. Guard: only dispatch `WaitForAllEvents` after `v.allEventsCh` is non-nil (i.e., from within the `runsStreamReadyMsg` handler).

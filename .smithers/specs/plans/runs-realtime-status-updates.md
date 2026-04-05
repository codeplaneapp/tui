## Goal
Wire the `RunsView` (`internal/ui/views/runs.go`) to the Smithers global SSE event stream so that run status changes propagate to the dashboard in real-time without polling. Subscribe via `client.StreamAllEvents(ctx)` when the view activates; patch `RunSummary.Status` in-place on each incoming `RunEventMsg`; self-re-schedule the listening cmd after each event; reconnect automatically when the stream closes. Fall back to a 5-second auto-poll ticker when the SSE endpoint is unavailable. Show a `● Live` / `○ Polling` mode indicator in the view header. Cancel the stream context when the user presses `Esc`.

---

## Steps

### 1. Add `WaitForAllEvents` cmd helper to `internal/smithers/runs.go`

Add this function at the bottom of `internal/smithers/runs.go`, after the existing scan helpers. No new imports are needed — `tea` is already imported by the package.

```go
// WaitForAllEvents returns a tea.Cmd that blocks on the next message from
// the channel returned by Client.StreamAllEvents.
//
// StreamAllEvents sends pre-typed values into an interface{} channel:
//   RunEventMsg, RunEventErrorMsg, or RunEventDoneMsg.
//
// WaitForAllEvents returns the value directly so Bubble Tea routes it
// to the correct case in the calling view's Update method.
//
// Self-re-scheduling pattern:
//
//   case smithers.RunEventMsg:
//       applyEvent(msg.Event)
//       return v, smithers.WaitForAllEvents(v.allEventsCh)
func WaitForAllEvents(ch <-chan interface{}) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok {
            return RunEventDoneMsg{}
        }
        return msg
    }
}
```

**Important**: If `ch` is nil, `<-ch` blocks forever — only dispatch this cmd after `v.allEventsCh` has been assigned from a `runsStreamReadyMsg`. See Step 3.

---

### 2. Add new internal message types to `internal/ui/views/runs.go`

Add these type declarations at the top of the file, alongside the existing `runsLoadedMsg` and `runsErrorMsg`:

```go
// runsStreamReadyMsg carries the channel returned by StreamAllEvents.
// It is returned by startStreamCmd so the channel can be stored on the view
// before WaitForAllEvents is first dispatched.
type runsStreamReadyMsg struct {
    ch <-chan interface{}
}

// runsStreamUnavailableMsg is returned when StreamAllEvents fails immediately
// (no server, 404 on endpoint). Triggers the auto-poll fallback.
type runsStreamUnavailableMsg struct{}

// runsEnrichRunMsg replaces a stub RunSummary with a fully-fetched one.
type runsEnrichRunMsg struct {
    run smithers.RunSummary
}

// tickMsg is sent by the poll ticker every 5 seconds when in polling mode.
type tickMsg struct{}
```

---

### 3. Extend `RunsView` struct with streaming fields

Replace the current `RunsView` struct with:

```go
type RunsView struct {
    client      *smithers.Client
    runs        []smithers.RunSummary
    cursor      int
    width       int
    height      int
    loading     bool
    err         error
    // Streaming state
    ctx         context.Context
    cancel      context.CancelFunc
    allEventsCh <-chan interface{}
    streamMode  string        // "live" | "polling" | "" (before first connect)
    pollTicker  *time.Ticker
}
```

Add `"context"` and `"time"` to the import block. The `NewRunsView` constructor is unchanged — context is created in `Init()`.

---

### 4. Update `Init()` to start the stream

Replace the current `Init()` with:

```go
func (v *RunsView) Init() tea.Cmd {
    v.ctx, v.cancel = context.WithCancel(context.Background())
    return tea.Batch(
        v.loadRunsCmd(),
        v.startStreamCmd(),
    )
}
```

Add these two helper cmd methods:

```go
func (v *RunsView) loadRunsCmd() tea.Cmd {
    ctx := v.ctx
    client := v.client
    return func() tea.Msg {
        runs, err := client.ListRuns(ctx, smithers.RunFilter{Limit: 50})
        if ctx.Err() != nil {
            return nil // view was popped while loading; discard silently
        }
        if err != nil {
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
        return runsStreamReadyMsg{ch: ch}
    }
}
```

The existing `Init()` body (which called `v.client.ListRuns` inline) moves into `loadRunsCmd`. The single `tea.Cmd` becomes `tea.Batch(loadRunsCmd, startStreamCmd)`.

---

### 5. Update `Update()` — handle all new message types

In the existing `Update` switch statement, add handlers for the five new message types alongside the existing `runsLoadedMsg` and `runsErrorMsg` cases:

```go
case runsStreamReadyMsg:
    v.allEventsCh = msg.ch
    v.streamMode = "live"
    return v, smithers.WaitForAllEvents(v.allEventsCh)

case runsStreamUnavailableMsg:
    v.streamMode = "polling"
    v.pollTicker = time.NewTicker(5 * time.Second)
    return v, v.pollTickCmd()

case smithers.RunEventMsg:
    newRunID := v.applyRunEvent(msg.Event)
    cmds := []tea.Cmd{smithers.WaitForAllEvents(v.allEventsCh)}
    if newRunID != "" {
        cmds = append(cmds, v.enrichRunCmd(newRunID))
    }
    return v, tea.Batch(cmds...)

case smithers.RunEventErrorMsg:
    // Non-fatal parse error; keep listening.
    return v, smithers.WaitForAllEvents(v.allEventsCh)

case smithers.RunEventDoneMsg:
    // Stream closed. Reconnect if our context is still alive.
    if v.ctx.Err() == nil {
        return v, v.startStreamCmd()
    }
    return v, nil

case runsEnrichRunMsg:
    for i, r := range v.runs {
        if r.RunID == msg.run.RunID {
            v.runs[i] = msg.run
            break
        }
    }
    return v, nil

case tickMsg:
    if v.ctx.Err() != nil {
        return v, nil // view popped; stop ticking
    }
    return v, tea.Batch(v.loadRunsCmd(), v.pollTickCmd())
```

Add the two new cmd helpers:

```go
func (v *RunsView) pollTickCmd() tea.Cmd {
    ch := v.pollTicker.C
    return func() tea.Msg {
        <-ch
        return tickMsg{}
    }
}

func (v *RunsView) enrichRunCmd(runID string) tea.Cmd {
    ctx := v.ctx
    client := v.client
    return func() tea.Msg {
        run, err := client.GetRunSummary(ctx, runID)
        if err != nil || run == nil {
            return nil
        }
        return runsEnrichRunMsg{run: *run}
    }
}
```

---

### 6. Implement `applyRunEvent`

Add this method to `RunsView`. It mutates `v.runs` in-place and returns the `RunID` of a newly inserted stub (empty string if no new run was added):

```go
func (v *RunsView) applyRunEvent(ev smithers.RunEvent) (newRunID string) {
    // Find existing entry.
    idx := -1
    for i, r := range v.runs {
        if r.RunID == ev.RunID {
            idx = i
            break
        }
    }

    switch ev.Type {
    case "RunStatusChanged", "RunFinished", "RunFailed", "RunCancelled", "RunStarted":
        if ev.Status == "" {
            return ""
        }
        if idx >= 0 {
            v.runs[idx].Status = smithers.RunStatus(ev.Status)
        } else {
            // Unknown run — insert a stub at the top, enrich asynchronously.
            stub := smithers.RunSummary{
                RunID:  ev.RunID,
                Status: smithers.RunStatus(ev.Status),
            }
            v.runs = append([]smithers.RunSummary{stub}, v.runs...)
            return ev.RunID
        }

    case "NodeWaitingApproval":
        if idx >= 0 {
            v.runs[idx].Status = smithers.RunStatusWaitingApproval
        }
    }
    return ""
}
```

---

### 7. Update `Esc` key handler — cancel context before pop

Replace the current `Esc` key case with:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
    v.cancel()
    if v.pollTicker != nil {
        v.pollTicker.Stop()
    }
    return v, func() tea.Msg { return PopViewMsg{} }
```

The existing `r` key case stays identical but should now call `v.loadRunsCmd()` instead of `v.Init()` (to avoid creating a second context+cancel pair):

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
    v.loading = true
    v.err = nil
    return v, v.loadRunsCmd()
```

---

### 8. Update `View()` — add mode indicator to header

Update the header rendering section:

```go
func (v *RunsView) View() string {
    var b strings.Builder

    header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Runs")
    helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")

    modeIndicator := ""
    switch v.streamMode {
    case "live":
        modeIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Live")
    case "polling":
        modeIndicator = lipgloss.NewStyle().Faint(true).Render("○ Polling")
    }

    headerLine := header
    if v.width > 0 {
        right := modeIndicator
        if right != "" && helpHint != "" {
            right = right + "  " + helpHint
        } else if right == "" {
            right = helpHint
        }
        gap := v.width - lipgloss.Width(header) - lipgloss.Width(right) - 2
        if gap > 0 {
            headerLine = header + strings.Repeat(" ", gap) + right
        }
    }
    b.WriteString(headerLine)
    b.WriteString("\n\n")
    // ... rest unchanged
```

---

### 9. Create E2E test `tests/runs_realtime_e2e_test.go`

The test uses an `httptest.Server` with a handler that:
1. Serves `GET /v1/runs` with a JSON array of 2 runs.
2. Serves `GET /v1/events` as a long-lived SSE stream, controlled by a `chan string` the test writes to.

```go
func TestRunsRealtimeSSE(t *testing.T) {
    // Setup mock server with SSE control channel.
    eventPush := make(chan string, 4)
    srv := startRunsMockServer(t, eventPush)
    defer srv.Close()

    // Launch TUI.
    pty := launchTUI(t, srv.URL)

    // Navigate to runs.
    sendKey(pty, ctrlR)
    waitForOutput(t, pty, "SMITHERS › Runs", 5*time.Second)
    waitForOutput(t, pty, "running", 5*time.Second)

    // Server pushes a status-change event.
    eventPush <- `event: smithers
data: {"type":"RunStatusChanged","runId":"abc123","status":"finished","timestampMs":1000}
id: 1

`
    // TUI should re-render without any keypress.
    waitForOutput(t, pty, "finished", 2*time.Second)

    // Press Esc — verify server sees connection drop within 1 s.
    sendKey(pty, escapeKey)
    waitForConnectionDrop(t, srv, "/v1/events", 1*time.Second)
}
```

Add a `TestRunsView_PollFallback` that returns 404 on `/v1/events` and asserts:
- `○ Polling` appears in header.
- `GET /v1/runs` is called again within 6 s (auto-poll).

---

### 10. Create VHS tape `tests/runs_realtime.tape`

```tape
Output tests/runs_realtime.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"

Type "SMITHERS_API_URL=http://localhost:7331 go run ."
Enter
Sleep 3s

Ctrl+R
Sleep 2s

# Header shows "● Live" when server is running.
# Status changes appear automatically as runs progress.
Sleep 8s

Escape
Sleep 1s
```

---

## File Plan

- [`internal/smithers/runs.go`](/Users/williamcory/crush/internal/smithers/runs.go) — add `WaitForAllEvents` at bottom of file
- [`internal/ui/views/runs.go`](/Users/williamcory/crush/internal/ui/views/runs.go) — add stream fields, new msg types, update `Init`/`Update`/`View`, add `applyRunEvent`/`loadRunsCmd`/`startStreamCmd`/`enrichRunCmd`/`pollTickCmd`
- [`tests/runs_realtime_e2e_test.go`](/Users/williamcory/crush/tests/runs_realtime_e2e_test.go) (new) — SSE e2e test + poll fallback test
- [`tests/runs_realtime.tape`](/Users/williamcory/crush/tests/runs_realtime.tape) (new) — VHS recording

No changes to `internal/smithers/events.go`, `internal/smithers/types_runs.go`, or `internal/pubsub/`.

---

## Validation

```bash
# Format
gofumpt -w internal/ui/views/runs.go internal/smithers/runs.go

# Vet
go vet ./internal/ui/views/... ./internal/smithers/...

# Unit tests — RunsView message handling
go test ./internal/ui/views/ -run TestRunsView -v -count=1

# Unit tests — WaitForAllEvents
go test ./internal/smithers/ -run TestWaitForAllEvents -v -count=1

# E2E tests — SSE and poll fallback
go test ./tests/ -run TestRunsRealtime -v -timeout 30s -count=1

# Race detector
go test -race ./internal/ui/views/ ./internal/smithers/... -count=1

# VHS
vhs validate tests/runs_realtime.tape

# Full build sanity
go build ./...
```

Manual verification:
```bash
# 1. With live Smithers server
cd /Users/williamcory/smithers
bun run src/cli/index.ts up --serve  # starts the HTTP API
cd /Users/williamcory/crush
go run . # open TUI
# Ctrl+R → runs view; "● Live" in header
# In another terminal: start a workflow run
bun run src/cli/index.ts up examples/fan-out-fan-in.tsx -d
# Observe status change in TUI without pressing r

# 2. Without server (poll fallback)
go run .  # no SMITHERS_API_URL set
# Ctrl+R → runs view; "○ Polling" in header if DB present; error otherwise
```

---

## Open Questions

1. **`r` key in polling mode**: Should manual `r` refresh reset the 5-second poll timer? Currently the timer and manual refresh are independent. They both call `loadRunsCmd` — the first one to complete wins. This is fine for v1 but could cause two rapid refreshes in close succession.

2. **Section re-sort on status change**: `RUNS_STATUS_SECTIONING` (future ticket) will sort `v.runs` into Active / Completed / Failed groups. When that lands, `applyRunEvent` may need to trigger a re-sort after each status patch. This ticket deliberately does not sort — add a comment in `applyRunEvent` flagging the dependency.

3. **Multiple views subscribing to `StreamAllEvents`**: If the approval queue overlay also opens `StreamAllEvents`, two HTTP connections are opened. Consider adding a `pubsub.Broker[RunEvent]` in the `smithers.Client` to fan-out a single connection. Defer to the approval overlay ticket (`approvals-pending-badges`).

4. **`GetRunSummary` fallback for new runs**: The `enrichRunCmd` calls `GetRunSummary`, which has HTTP + SQLite + exec fallbacks. On exec fallback, `smithers inspect <runID>` may be slow (> 1 s). The stub row will show until the enrichment completes — this is acceptable for v1.

```json
{
  "document": "Implementation plan for runs-realtime-status-updates: SSE subscription in RunsView via StreamAllEvents + WaitForAllEvents, in-place status patching via applyRunEvent, auto-poll fallback, mode indicator, context teardown on Esc."
}
```

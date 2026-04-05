# Research: Runs Real-time Status Updates

## Ticket
`runs-realtime-status-updates` — Stream Real-time Run Updates

## Findings

### 1. What already exists

#### SSE transport layer — `internal/smithers/events.go`
Two implementations coexist in this file:

- **`StreamRunEventsWithReconnect`** (from `platform-sse-streaming` ticket) — the production-grade path. Opens `GET /v1/runs/:id/events?afterSeq=N`, reconnects on transient failures with exponential backoff (1 s → 30 s), tracks the sequence cursor so already-delivered events are never replayed. Returns `(<-chan RunEvent, <-chan error)`. The companion `WaitForRunEvent` function converts this into a self-re-scheduling `tea.Cmd` returning typed `RunEventMsg` / `RunEventDoneMsg` / `RunEventErrorMsg` values. `BridgeRunEvents` pipes the channel into a `pubsub.Broker[RunEvent]` for fan-out.

- **`StreamRunEvents`** (from `eng-smithers-client-runs` ticket) — single-shot connection, no reconnection, returns `<-chan interface{}`. Used by the approval overlay and other early consumers. Does not support cursor-based resumption.

- **`StreamAllEvents`** — global feed at `GET /v1/events`, all runs, same channel-of-interface pattern. Used by the notification system.

The `StreamRunEventsWithReconnect` + `WaitForRunEvent` path is the correct one for `RunsView`. It is already implemented and tested.

#### Run types — `internal/smithers/types_runs.go`
All necessary types exist:
- `RunSummary` — `RunID`, `WorkflowName`, `Status RunStatus`, `StartedAtMs`, `FinishedAtMs`, `Summary map[string]int`
- `RunStatus` — `running`, `waiting-approval`, `waiting-event`, `finished`, `failed`, `cancelled`
- `RunStatus.IsTerminal()` — returns true for `finished`, `failed`, `cancelled`
- `RunEvent` — `Type`, `RunID`, `NodeID`, `Status`, `Seq`, `Raw json.RawMessage`
- `RunEventMsg`, `RunEventDoneMsg`, `RunEventErrorMsg` — the three `tea.Msg` types

The event type discriminator values that matter for `RunsView` status updates:
- `"RunStarted"` — run transitions to `running`
- `"RunFinished"` — run transitions to `finished`
- `"RunFailed"` — run transitions to `failed`
- `"RunCancelled"` — run transitions to `cancelled`
- `"RunStatusChanged"` — generic status transition (may carry a `status` field directly on `RunEvent`)
- `"NodeWaitingApproval"` — a node paused for approval; run is `waiting-approval`

`RunEvent.Status` is already a field on the struct and is populated from `"status"` in the JSON payload. For status-change events the `Status` field carries the new status string directly.

#### RunsView — `internal/ui/views/runs.go`
Current state: static poll on `Init()`, calls `client.ListRuns()`, no streaming. The view holds `[]smithers.RunSummary` and renders via `components.RunTable`. A manual `r` key refresh is the only way to update. No `context.Context`, no cancel function, no `eventCh` field.

The view uses `runsLoadedMsg` / `runsErrorMsg` as the only internal message types.

#### pubsub broker — `internal/pubsub/broker.go`
Generic `Broker[T]` with `Subscribe(ctx) <-chan Event[T]` and `Publish(EventType, T)`. Channels are context-lifetime-scoped; a cancelled context automatically unsubscribes. `Publish` is non-blocking — slow subscribers are dropped (backpressure by design). Suitable for fan-out when multiple views subscribe to the same run stream. `BridgeRunEvents` in `events.go` connects an SSE channel to a broker.

#### API endpoint design
- **Run-scoped**: `GET /v1/runs/:id/events?afterSeq=N` — per-run stream. `RunsView` must either open one stream per visible active run or use the global feed.
- **Global**: `GET /v1/events` — all runs, all event types. This is the simpler subscription model for the dashboard: one SSE connection, parse `runId` from each event, update the corresponding `RunSummary` in the view's local state.

### 2. The two subscription strategies

**Option A: Global feed (`GET /v1/events`)**
- One SSE connection for the entire dashboard.
- Parse `RunEvent.RunID` on each event; look up the run in `v.runs` slice; apply the status change.
- Simpler connection lifecycle — one context to manage.
- Requires the server to have the global `/v1/events` endpoint. `StreamAllEvents` already implements this. The endpoint may not exist on all server versions; `StreamAllEvents` handles `404` as `ErrServerUnavailable`.
- No per-run fan-out complexity.

**Option B: Per-run streams**
- Open one `StreamRunEventsWithReconnect` per active (non-terminal) run.
- More precise — only receive events for runs currently visible.
- Complex lifecycle: must cancel streams for runs that go terminal, open streams for new runs that appear.
- May create many simultaneous HTTP connections.

**Recommendation**: Start with Option A (global feed via `StreamAllEvents` with reconnect wrapping). If the global endpoint is unavailable, fall back to polling (`r` key manual refresh + a `time.Ticker` auto-poll at 5 s). Add per-run stream support as a separate enhancement once the approval queue and live chat viewer need it.

### 3. Status update semantics

When a `RunEventMsg` arrives with `Event.Type` of `"RunStatusChanged"` (or `"RunStarted"`, `"RunFinished"`, etc.):

1. Find the `RunSummary` in `v.runs` where `RunID == event.RunID`.
2. Set `RunSummary.Status = RunStatus(event.Status)`.
3. If `event.Status` is terminal and the run was previously active, decrement active-run count.
4. Re-sort the slice so active runs appear first (matching the design wireframe's section layout).

For `"NodeWaitingApproval"` events:
- The run's status may already be `waiting-approval` (server sends the status change before the node event), but set it explicitly to be safe.

For **new runs** that appear via the stream (a run started that was not in the initial `ListRuns` snapshot):
- `RunEventMsg` with `Type == "RunStarted"` and an unknown `RunID` — add a new `RunSummary` with the available fields.
- A full `GetRunSummary(runID)` call can enrich it asynchronously.

For **runs that go terminal** and the view previously had no knowledge of them:
- Can be ignored (terminal runs are already in the initial list or will appear on next refresh).

### 4. Context lifecycle

The SSE stream goroutine must be tied to `RunsView`'s view lifetime, not the program lifetime. The view must:
1. Create a `context.Context` with cancel in `Init()` (or in a `teardown` / `Destroy` phase).
2. Cancel it when the view is popped (i.e., when `PopViewMsg` is dispatched).
3. The channel close propagates to `WaitForRunEvent` which returns `RunEventDoneMsg`, allowing the view to clean up.

The `views.View` interface currently has no `Destroy()` hook. The safe pattern is: cancel via the `PopViewMsg` handler before returning it, and let the goroutine drain naturally.

### 5. Optimistic UI updates

For approval and cancellation actions (not in scope of this ticket but relevant for design): when the user presses `a` (approve) or `x` (cancel), the view can optimistically update `RunSummary.Status` before the server confirms. If the SSE stream later delivers a conflicting status, the stream value wins.

For this ticket: no user mutations are in scope. Status updates are read-only reflections of server state.

### 6. Reconnection handling

`StreamRunEventsWithReconnect` handles transient disconnects transparently with backoff (1 s → 30 s). However, it is run-scoped; the global `StreamAllEvents` does not have a reconnect wrapper.

**Gap**: `StreamAllEvents` has no reconnect logic. To use Option A reliably, either:
1. Add `StreamAllEventsWithReconnect` (mirrors `StreamRunEventsWithReconnect`), or
2. Re-launch `StreamAllEvents` in a `RunsView`-level reconnect loop when `RunEventDoneMsg` is received and the view is still active.

Option 2 is simpler and keeps the reconnect logic in the view layer where context cancellation is already managed.

### 7. Fallback strategy

When `StreamAllEvents` returns `ErrServerUnavailable` (no API URL, server not running, 404 on endpoint):
- Do not show an error. The view already loaded its initial list via `ListRuns`, which has SQLite and exec fallbacks.
- Enable a background `time.Ticker` poll every 5 seconds (auto-refresh) instead of SSE.
- Show a "Live" / "Polling" indicator in the header to signal the update mode to the user.

### 8. Design doc alignment

The design doc wireframe (`02-DESIGN.md:143-172`) shows:
- Active runs grouped at the top with a `● ACTIVE (3)` section header.
- Completed runs below with `● COMPLETED TODAY (12)`.
- Failed runs at the bottom with `● FAILED (1)`.
- Each row shows a progress bar (`████████░░`), node progress (`3/5 nodes`), elapsed time, and the cursor indicator (`▸`).

Real-time updates must maintain this sorted/sectioned layout. When a run's status changes (e.g. `running → waiting-approval`), its position in the section list may need to change (the run stays in ACTIVE but its row indicator changes from plain to `⚠ 1`).

### 9. WaitForRunEvent pattern vs. direct channel receive

`WaitForRunEvent(runID, ch, errCh)` is designed for single-run subscription. For the global feed (Option A), the `StreamAllEvents` channel carries `interface{}` values, not typed `RunEvent` values directly. The view must type-switch on `interface{}` values in `Update`.

A cleaner approach: wrap `StreamAllEvents` with a thin adapter that converts the `interface{}` channel into the same typed `RunEventMsg` / `RunEventDoneMsg` / `RunEventErrorMsg` pattern using a `tea.Cmd`. This is already how `StreamRunEvents` is consumed elsewhere in the codebase.

### 10. The `events.go` dual-implementation gap

The `events.go` file currently has two independently written SSE parsers:
- The `parseSSEStream` function (from `platform-sse-streaming`) used by `StreamRunEventsWithReconnect`
- The inline scanner loop inside `StreamRunEvents` (from `eng-smithers-client-runs`)
- The inline scanner loop inside `StreamAllEvents` (from `eng-smithers-client-runs`)

The `RunsView` ticket should use the `StreamAllEvents` path (single connection for the dashboard). No refactoring of `events.go` is required for this ticket — the dual implementation is a technical debt item for a separate cleanup.

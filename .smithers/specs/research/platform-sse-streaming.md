Research Report: platform-sse-streaming

## Existing Crush Surface

### Smithers Client (`internal/smithers/client.go`)
The existing client has a mature three-tier transport structure (HTTP → SQLite → exec) with `Client` struct fields `apiURL`, `apiToken`, `dbPath`, `httpClient`, and `execFunc`. Transport helpers `httpGetJSON` and `httpPostJSON` handle standard `{ok, data, error}` JSON envelopes. The `isServerAvailable()` probe is cached for 30 seconds via `sync.RWMutex`. There is **no SSE consumer** of any kind — the entire SSE feature is new work.

### PubSub Package (`internal/pubsub/`)
`internal/pubsub/broker.go` implements `Broker[T any]` with `Subscribe(ctx) <-chan Event[T]` (buffered at 64) and `Publish(EventType, T)`. On context cancellation the subscriber channel is closed and removed. `Publish` drops events for slow consumers (non-blocking send). `internal/pubsub/events.go` defines `EventType` string with constants `CreatedEvent`, `UpdatedEvent`, `DeletedEvent`, and the generic `Event[T]` struct. This pub/sub system is the integration target for SSE bridge consumers.

### MCP Package
`internal/mcp/` does not exist in this checkout — the MCP path in the engineering doc references upstream MCP transport (stdio/http/sse for MCP protocol purposes), not for Smithers SSE. There is no reusable SSE parsing code anywhere in the codebase.

### Bubble Tea Integration Pattern (`internal/ui/model/ui.go`)
Async work flows through `tea.Cmd` functions that return `tea.Msg` structs. The `UI.Update(msg tea.Msg)` switch dispatches on concrete message types. The pattern for delivering async results is: launch a goroutine returning a channel, then poll via a `tea.Tick`-style command, or send back a single `tea.Msg` when the goroutine completes. The recommended SSE integration pattern is a `tea.Cmd` that wraps a goroutine reading from the SSE channel and posts typed `tea.Msg` values back to the Bubble Tea runtime.

### Smithers Types (`internal/smithers/types.go`)
Defines `Agent`, `SQLResult`, `ScoreRow`, `AggregateScore`, `MemoryFact`, `MemoryRecallResult`, `Ticket`, `Approval`, `CronSchedule`. There are **no run-lifecycle or event types**. `SmithersEvent` Go structs must be added as part of this ticket's work.

---

## Upstream Smithers Server Contract

### SSE Endpoint — Run-Scoped (Hono App, `src/server/serve.ts`)

```
GET /events?afterSeq={n}
```

The run-scoped Hono app (mounted under `/v1/runs/:runId` by `index.ts`) registers this endpoint. The Hono `streamSSE` helper writes:

```
event: smithers
data: {payloadJson}
id: {seq}
```

Each SSE frame carries `event: smithers`, the `data:` field is the raw `payloadJson` string already serialized (it is stored as JSON in the DB and passed through verbatim — no double-encoding). The `id:` field is the database `seq` integer, which serves as the cursor.

Query param `afterSeq` (default `-1`) controls cursor position. The server polls the DB every 500 ms for new rows. When the run reaches a terminal status (`finished`, `failed`, `cancelled`, `continued`) and no new events are available, the stream is closed gracefully from the server side.

### SSE Endpoint — Global (Node HTTP server, `src/server/index.ts`)

```
GET /v1/runs/:runId/events?afterSeq={n}
```

The raw Node HTTP implementation in `index.ts` does the same polling loop but also emits:
- `retry: 1000\n\n` — SSE retry hint once on open.
- `: keep-alive\n\n` — SSE comment heartbeat every 10 seconds (`DEFAULT_SSE_HEARTBEAT_MS = 10_000`).

Both implementations write `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, and `X-Accel-Buffering: no`.

**Key distinction**: The data line is already valid JSON (the raw `payloadJson` string from the DB). The Go client must decode this string as `json.RawMessage` and then unmarshal into the concrete `SmithersEvent` union to dispatch on `type`.

### Full SmithersEvent Union (`src/SmithersEvent.ts`)

Every event carries `runId string` and `timestampMs int64`. The full union discriminated by `type` includes:

**Run lifecycle**: `RunStarted`, `RunStatusChanged`, `RunFinished`, `RunFailed`, `RunCancelled`, `RunContinuedAsNew`, `RunHijackRequested`, `RunHijacked`, `RunAutoResumed`, `RunAutoResumeSkipped`, `RunForked`, `ReplayStarted`

**Supervisor**: `SupervisorStarted`, `SupervisorPollCompleted`

**Node lifecycle**: `NodePending`, `NodeStarted`, `NodeFinished`, `NodeFailed`, `NodeCancelled`, `NodeSkipped`, `NodeRetrying`, `NodeWaitingApproval`, `NodeWaitingTimer`, `NodeOutput`, `TaskHeartbeat`, `TaskHeartbeatTimeout`

**Agent**: `AgentEvent` (wraps `AgentCliEvent` from `src/agents/BaseCliAgent.ts` — contains `AgentCliStartedEvent`, `AgentCliActionEvent`, `AgentCliCompletedEvent` sub-types with level/text/delta/engine fields)

**Approval**: `ApprovalRequested`, `ApprovalGranted`, `ApprovalDenied`

**Tools**: `ToolCallStarted`, `ToolCallFinished`

**Time-travel/Frames**: `FrameCommitted`, `SnapshotCaptured`, `RevertStarted`, `RevertFinished`

**Sandbox**: `SandboxCreated`, `SandboxShipped`, `SandboxHeartbeat`, `SandboxBundleReceived`, `SandboxCompleted`, `SandboxFailed`, `SandboxDiffReviewRequested`, `SandboxDiffAccepted`, `SandboxDiffRejected`

**Workflow**: `WorkflowReloadDetected`, `WorkflowReloaded`, `WorkflowReloadFailed`, `WorkflowReloadUnsafe`

**Scoring**: `ScorerStarted`, `ScorerFinished`, `ScorerFailed`, `TokenUsageReported`

For the initial implementation, the Go consumer needs full fidelity for: `RunStarted`, `RunStatusChanged`, `RunFinished`, `RunFailed`, `RunCancelled`, `NodeStarted`, `NodeFinished`, `NodeFailed`, `NodeWaitingApproval`, `NodeOutput`, `ApprovalRequested`, `AgentEvent`. Other types should be preserved as `json.RawMessage` but may be passed through without full struct decoding.

---

## SSE Wire Format Analysis

### Frame Structure
```
event: smithers\r\n
data: {"type":"NodeOutput","runId":"abc","nodeId":"n1","iteration":0,"attempt":0,"text":"hello","stream":"stdout","timestampMs":1234}\r\n
id: 42\r\n
\r\n
```

The SSE spec requires parsing:
- `event:` field → event name (always `"smithers"` here; comment lines `":"` are heartbeats)
- `data:` field → payload (may theoretically span multiple `data:` lines; concatenate with `\n`)
- `id:` field → last event ID for resumption tracking
- blank line → dispatch event

### Heartbeat format
```
: keep-alive\r\n
\r\n
```
Comment lines starting with `:` must be ignored and never dispatched as events.

### Retry hint
```
retry: 1000\r\n
\r\n
```
The `retry:` field (milliseconds) is a browser hint. The Go client should use it to override the reconnect backoff initial delay if present (optional optimization).

---

## Go SSE Parsing Patterns

### Standard Library Approach (Recommended)
Use `bufio.Scanner` with a line-by-line loop. The default 64 KB token buffer is adequate for most events, but `NodeOutput` events carrying large agent outputs may exceed it. Call `scanner.Buffer(buf, maxTokenSize)` with a 1 MB limit to be safe.

```go
scanner := bufio.NewScanner(r)
scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
```

### Alternative: bufio.Reader.ReadString
`ReadString('\n')` is an alternative but requires more boilerplate for the dispatch logic. Scanner is cleaner for line-oriented protocols.

### Third-Party Libraries
`github.com/r3labs/sse/v2` and `github.com/tmaxmax/go-sse` are available but not necessary. The Smithers SSE format is simple enough that stdlib parsing is preferable — no external dependency, full control over reconnect and cursor tracking.

---

## Reconnection Strategy Analysis

### Server-Side Behavior
The server emits `retry: 1000` on connect, meaning it suggests a 1 s base delay. When the run is terminal and no events remain, the server closes the connection cleanly (EOF). The client must distinguish between:
1. **Clean EOF on terminal run** — do not reconnect; run is done.
2. **Error disconnect** — reconnect with backoff.
3. **Context cancellation** — stop entirely.

Detecting a terminal-run EOF is difficult without application-level knowledge. The recommended approach: always reconnect with backoff unless the context is cancelled. If the run has ended, the server will again cleanly close with no new events, and the caller can stop subscribing when it sees a `RunFinished`/`RunFailed`/`RunCancelled` event.

### Cursor Tracking with `afterSeq`
The `id:` field in each SSE frame carries the `seq` integer. The client must track the last received `id` and use it as `afterSeq` on reconnect. This avoids replaying events already processed. Initial value is `-1` (fetch all events from the beginning). After reconnect, use the last seen `seq`.

The `Last-Event-ID` HTTP header is the W3C standard mechanism for this, but the Smithers server reads `afterSeq` as a query param. On reconnect the client must construct a new URL with `?afterSeq={lastSeq}`.

### Backoff Parameters
- Initial delay: 1 s (matches server `retry:` hint)
- Multiplier: 2×
- Maximum delay: 30 s
- Reset: on successful event delivery (not just connection establishment), to avoid rapid reconnect storms when the server connects but immediately drops

---

## Bubble Tea Integration Patterns

### Option A: Polling via tea.Tick
The SSE goroutine sends events to a buffered Go channel. A `tea.Cmd` polls the channel on each tick (e.g., 100 ms) and returns a batch of events as a `tea.Msg`. This matches how Crush handles async agent output today.

**Pros**: Simple, no Bubble Tea internals needed.
**Cons**: Up to 100 ms latency; tick fires even when nothing is available.

### Option B: Blocking Channel Read as tea.Cmd
Each `tea.Cmd` blocks on a single channel read. When an event arrives, the cmd returns a `tea.Msg` and immediately dispatches a new cmd to wait for the next event. This creates a self-re-scheduling event loop.

```go
func waitForEvent(ch <-chan SSEEvent) tea.Cmd {
    return func() tea.Msg {
        evt, ok := <-ch
        if !ok {
            return sseStreamClosedMsg{}
        }
        return sseEventMsg{Event: evt}
    }
}
```

**Pros**: Zero latency; no polling overhead; idiomatic Bubble Tea pattern.
**Cons**: Requires careful goroutine lifecycle management; cmd must be re-dispatched in `Update`.

Option B is recommended — it mirrors how Crush handles streaming AI model output.

### Option C: tea.Program.Send from goroutine
Use `program.Send(msg)` directly from the SSE goroutine. This requires holding a `*tea.Program` reference, which is an anti-pattern in Crush's architecture (views don't hold program refs).

**Verdict**: Option B is the canonical approach.

---

## Memory Management

### Event Buffer Sizing
The SSE channel buffer of 64 items matches the pubsub `Broker` default. This is appropriate. A slow Bubble Tea render loop will drop events if the buffer overflows — the pubsub `Publish` non-blocking pattern deliberately drops for slow consumers.

For SSE, dropping events is not acceptable (it breaks sequence tracking and live chat viewer completeness). The SSE goroutine must **block** on channel send (or use a large buffer). Recommendation: use a blocking send with the context as escape hatch:

```go
select {
case ch <- evt:
case <-ctx.Done():
    return ctx.Err()
}
```

### Run-Scoped vs. Global Streaming
Each run gets its own SSE connection at `/v1/runs/:runId/events`. The `StreamRunEvents(ctx, runID)` method should open a per-run connection. Multiple views subscribing to the same run should share one underlying connection through the pubsub bridge, not open duplicate HTTP streams.

### Cleanup
The `Client` must close the HTTP response body when the context is cancelled. The `defer resp.Body.Close()` call inside `consumeStream` handles this correctly because `http.NewRequestWithContext` ties the request to the context; when the context is cancelled, the transport unblocks the read.

---

## Gaps

- `internal/smithers/events.go` does not exist — full SSE implementation is new.
- `internal/smithers/types.go` has no `SmithersEvent` Go type, no `RunStatus`, no run-lifecycle types. These must be added (likely in a separate `types_events.go` to keep the file manageable, or inline in `events.go`).
- The existing `eng-smithers-client-runs` plan already targets `events.go` for SSE; this ticket specialises on the SSE transport layer specifically. The two must be coordinated to avoid the runs-client ticket pre-empting this ticket's design.
- The `Client` struct's `httpClient` has a 10-second timeout — this is **incompatible with SSE** (the stream runs for minutes to hours). The `StreamRunEvents` method must either use a separate `http.Client` with no timeout, or set `http.Client{Timeout: 0}` for stream requests. The existing `httpClient` field should not be changed; instead, the SSE method should construct its own client or override the timeout per-request via `http.Transport`.

## Recommended Direction

1. Implement `internal/smithers/events.go` with: `SmithersEvent` Go struct (discriminated union via `json.RawMessage` Data + string `Type` — full type parsing is done by consumers), `StreamRunEvents(ctx, runID string) (<-chan RunEventMsg, error)` returning a message channel where `RunEventMsg` is a `tea.Msg`-compatible struct, and `sseStream` internal type handling parse/reconnect/cursor.
2. Keep SSE client transport separate from the main `httpClient` (use `http.Client{Timeout: 0}` for streaming).
3. Track `afterSeq` from SSE `id:` fields; pass on reconnect as `?afterSeq={n}`.
4. Implement Option B (self-re-scheduling `tea.Cmd`) for Bubble Tea integration — a single exported `WaitForRunEvent(ch)` cmd function that views call in their `Init()` and re-dispatch in `Update()`.
5. Add `pubsub.Broker[SmithersEvent]` bridge for components that need broadcast fan-out without polling.
6. `bufio.Scanner` with 1 MB buffer, `event: smithers` as expected event name, `: keep-alive` heartbeats ignored.
7. Test with `httptest.Server` emitting controlled SSE frames.

## Files To Touch
- [`internal/smithers/events.go`](/Users/williamcory/crush/internal/smithers/events.go) (new)
- [`internal/smithers/events_test.go`](/Users/williamcory/crush/internal/smithers/events_test.go) (new)
- [`internal/smithers/types.go`](/Users/williamcory/crush/internal/smithers/types.go) (add `SmithersEvent`, `RunEventMsg`, `RunStatus`)
- [`internal/smithers/client.go`](/Users/williamcory/crush/internal/smithers/client.go) (no struct changes; `StreamRunEvents` method added in `events.go` as a method on `*Client`)

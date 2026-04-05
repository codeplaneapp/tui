## Goal
Implement a production-quality SSE consumer in `internal/smithers/events.go` that connects to the Smithers server's run-scoped event stream (`GET /v1/runs/:runId/events?afterSeq={n}`), parses the `event: smithers` / `data:` / `id:` wire format, tracks sequence cursors for reconnection, recovers transparently on disconnect with exponential backoff, and delivers events to Bubble Tea via a self-re-scheduling `tea.Cmd` pattern. Add Go type coverage for the `SmithersEvent` union. Add a `pubsub.Broker` bridge for multi-subscriber fan-out. Cover all behaviors with table-driven unit tests against `httptest.Server`.

---

## Steps

### 1. Add `SmithersEvent` types to `internal/smithers/types.go`

Add these new types alongside the existing definitions. Do not modify any existing type.

```go
// RunStatus mirrors RunStatus in smithers/src/RunStatus.ts
type RunStatus string

const (
    RunStatusPending   RunStatus = "pending"
    RunStatusRunning   RunStatus = "running"
    RunStatusFinished  RunStatus = "finished"
    RunStatusFailed    RunStatus = "failed"
    RunStatusCancelled RunStatus = "cancelled"
    RunStatusContinued RunStatus = "continued"
    RunStatusWaitingApproval RunStatus = "waiting-approval"
    RunStatusWaitingTimer    RunStatus = "waiting-timer"
)

// IsTerminal returns true if this status means the run has ended.
func (s RunStatus) IsTerminal() bool {
    switch s {
    case RunStatusFinished, RunStatusFailed, RunStatusCancelled, RunStatusContinued:
        return true
    }
    return false
}

// SmithersEvent is a parsed run-scoped event from the SSE stream.
// Type is the discriminator (e.g. "RunStarted", "NodeOutput").
// Data is the raw JSON payload — callers unmarshal into the specific struct they need.
// Seq is the database sequence number from the SSE id: field.
type SmithersEvent struct {
    Type      string          `json:"type"`
    RunID     string          `json:"runId"`
    Seq       int64           `json:"-"` // populated from SSE id: field, not payload
    TimestampMs int64         `json:"timestampMs"`
    Data      json.RawMessage `json:"-"` // full raw JSON of the payload
}

// Typed payload structs for the event types the TUI acts on directly:

type RunStatusChangedPayload struct {
    RunID       string    `json:"runId"`
    Status      RunStatus `json:"status"`
    TimestampMs int64     `json:"timestampMs"`
}

type NodeOutputPayload struct {
    RunID       string `json:"runId"`
    NodeID      string `json:"nodeId"`
    Iteration   int    `json:"iteration"`
    Attempt     int    `json:"attempt"`
    Text        string `json:"text"`
    Stream      string `json:"stream"` // "stdout" | "stderr"
    TimestampMs int64  `json:"timestampMs"`
}

type NodeWaitingApprovalPayload struct {
    RunID       string `json:"runId"`
    NodeID      string `json:"nodeId"`
    Iteration   int    `json:"iteration"`
    TimestampMs int64  `json:"timestampMs"`
}

type AgentEventPayload struct {
    RunID       string          `json:"runId"`
    NodeID      string          `json:"nodeId"`
    Iteration   int             `json:"iteration"`
    Attempt     int             `json:"attempt"`
    Engine      string          `json:"engine"`
    Event       json.RawMessage `json:"event"` // AgentCliEvent — opaque to the TUI transport layer
    TimestampMs int64           `json:"timestampMs"`
}

// RunEventMsg is the tea.Msg delivered to the Bubble Tea runtime for each SSE event.
// It is the value returned by the WaitForRunEvent cmd.
type RunEventMsg struct {
    RunID string
    Event SmithersEvent
}

// RunStreamErrorMsg is delivered when the SSE stream encounters a non-recoverable error
// (context cancelled, exhausted retries).
type RunStreamErrorMsg struct {
    RunID string
    Err   error
}

// RunStreamDoneMsg is delivered when the SSE stream closes cleanly
// (run reached terminal state and server closed the connection).
type RunStreamDoneMsg struct {
    RunID string
}
```

### 2. Create `internal/smithers/events.go`

New file. This is the SSE transport layer. It must not import any Bubble Tea packages — the `tea.Cmd` integration lives in step 3.

```go
package smithers

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"
)

const (
    sseInitialDelay = 1 * time.Second
    sseMaxDelay     = 30 * time.Second
)

// StreamRunEvents connects to the Smithers server's run-scoped SSE endpoint and
// returns a channel of SmithersEvent values. The channel is closed when:
//   - ctx is cancelled (clean shutdown)
//   - the run reaches terminal state and the server closes the stream with no new events
//   - reconnect attempts are exhausted (channel closed with a final error delivered via errCh)
//
// The errCh receives at most one error. If the stream ends cleanly, errCh is closed without
// sending. Callers may ignore errCh if they don't need error distinctions.
//
// The afterSeq parameter sets the starting cursor (-1 = from beginning).
// Sequence numbers are tracked internally and advanced with each event.
func (c *Client) StreamRunEvents(ctx context.Context, runID string, afterSeq int64) (<-chan SmithersEvent, <-chan error) {
    ch := make(chan SmithersEvent, 128)
    errCh := make(chan error, 1)
    go func() {
        defer close(ch)
        defer close(errCh)
        c.runSSELoop(ctx, runID, afterSeq, ch, errCh)
    }()
    return ch, errCh
}

// runSSELoop is the reconnect loop. It calls consumeRunStream in a loop, applying
// exponential backoff between attempts. It stops when:
//   - ctx is done
//   - consumeRunStream returns errStreamDone (clean server close on terminal run)
func (c *Client) runSSELoop(
    ctx context.Context,
    runID string,
    initialSeq int64,
    ch chan<- SmithersEvent,
    errCh chan<- error,
) {
    delay := sseInitialDelay
    lastSeq := initialSeq

    for {
        lastSeq, err := c.consumeRunStream(ctx, runID, lastSeq, ch)
        if ctx.Err() != nil {
            return // context cancelled: clean exit, no error
        }
        if err == errStreamDone {
            return // server closed cleanly: run is terminal
        }
        // transient error: wait and reconnect
        _ = err // log here if needed
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }
        delay = min(delay*2, sseMaxDelay)
        _ = lastSeq
    }
}

// errStreamDone is a sentinel returned by consumeRunStream when the server
// closes the connection cleanly (terminal run, no more events).
var errStreamDone = fmt.Errorf("sse: stream closed cleanly")

// consumeRunStream opens one SSE connection and reads until EOF or error.
// Returns (lastSeq, err). If the server closes cleanly, returns errStreamDone.
// lastSeq is the seq of the last successfully delivered event.
func (c *Client) consumeRunStream(
    ctx context.Context,
    runID string,
    afterSeq int64,
    ch chan<- SmithersEvent,
) (int64, error) {
    url := fmt.Sprintf("%s/v1/runs/%s/events?afterSeq=%d", c.apiURL, runID, afterSeq)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return afterSeq, err
    }
    req.Header.Set("Accept", "text/event-stream")
    req.Header.Set("Cache-Control", "no-cache")
    if c.apiToken != "" {
        req.Header.Set("Authorization", "Bearer "+c.apiToken)
    }

    // Use a streaming-safe HTTP client (no timeout on body read).
    streamClient := &http.Client{
        Transport: c.httpClient.Transport, // reuse transport (connection pooling, TLS)
        Timeout:   0,                      // no timeout on streaming body
    }

    resp, err := streamClient.Do(req)
    if err != nil {
        return afterSeq, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return afterSeq, fmt.Errorf("sse: unexpected status %d", resp.StatusCode)
    }

    return parseSSEStream(ctx, resp.Body, afterSeq, ch)
}

// parseSSEStream reads SSE frames from r and sends SmithersEvent values to ch.
// Returns (lastSeq, nil) on clean EOF (server closed), or (lastSeq, err) on error.
// Returns (lastSeq, errStreamDone) only when we detect the stream has ended naturally.
//
// The Smithers server sends:
//   event: smithers\n
//   data: {payloadJson}\n
//   id: {seq}\n
//   \n
// Heartbeats are: ": keep-alive\n\n" (comment lines, ignored).
// The "retry: 1000\n\n" hint is parsed but currently informational only.
func parseSSEStream(
    ctx context.Context,
    r io.Reader,
    initialSeq int64,
    ch chan<- SmithersEvent,
) (int64, error) {
    scanner := bufio.NewScanner(r)
    // Increase buffer to 1 MB to handle large NodeOutput/AgentEvent payloads.
    scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

    lastSeq := initialSeq
    var (
        currentEventName string
        currentData      strings.Builder
        currentID        string
    )

    for scanner.Scan() {
        if ctx.Err() != nil {
            return lastSeq, ctx.Err()
        }

        line := scanner.Text()

        switch {
        case line == "":
            // Blank line: dispatch event if we have data.
            if currentData.Len() > 0 {
                rawData := currentData.String()

                // Parse the base fields for discrimination.
                var base struct {
                    Type        string `json:"type"`
                    RunID       string `json:"runId"`
                    TimestampMs int64  `json:"timestampMs"`
                }
                if err := json.Unmarshal([]byte(rawData), &base); err != nil {
                    // Malformed event — skip without crashing.
                    currentEventName = ""
                    currentData.Reset()
                    currentID = ""
                    continue
                }

                seq := lastSeq
                if currentID != "" {
                    if n, err := strconv.ParseInt(currentID, 10, 64); err == nil {
                        seq = n
                    }
                }

                evt := SmithersEvent{
                    Type:        base.Type,
                    RunID:       base.RunID,
                    Seq:         seq,
                    TimestampMs: base.TimestampMs,
                    Data:        json.RawMessage(rawData),
                }

                select {
                case ch <- evt:
                    lastSeq = seq
                case <-ctx.Done():
                    return lastSeq, ctx.Err()
                }
            }
            currentEventName = ""
            currentData.Reset()
            currentID = ""

        case strings.HasPrefix(line, ":"):
            // Comment / heartbeat — ignore.

        case strings.HasPrefix(line, "event:"):
            currentEventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

        case strings.HasPrefix(line, "data:"):
            data := strings.TrimPrefix(line, "data:")
            // SSE spec: strip exactly one leading space after the colon.
            if len(data) > 0 && data[0] == ' ' {
                data = data[1:]
            }
            if currentData.Len() > 0 {
                currentData.WriteByte('\n')
            }
            currentData.WriteString(data)

        case strings.HasPrefix(line, "id:"):
            currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))

        case strings.HasPrefix(line, "retry:"):
            // Could parse and store for backoff tuning — currently ignored.

        default:
            // Unknown field — ignore per SSE spec.
        }
        _ = currentEventName // used for validation/future filtering
    }

    if err := scanner.Err(); err != nil {
        return lastSeq, err
    }
    // Clean EOF — server closed the connection (terminal run).
    return lastSeq, errStreamDone
}
```

**Important implementation notes:**

- The `runSSELoop` must shadow the `lastSeq` properly — the returned `lastSeq` from `consumeRunStream` must be the outer loop's cursor. The code sketch above has a shadowing bug in the `lastSeq, err :=` line; the implementor must use `lastSeq, err =` (assignment, not declaration) after declaring `lastSeq` and `err` before the loop.
- `streamClient.Transport` reuse avoids creating a new transport per reconnect (which would lose connection pooling and TLS session cache). If `c.httpClient.Transport` is nil, the default transport is used automatically.
- The `currentEventName` guard: the Smithers server always sends `event: smithers` before `data:`. The parser should optionally validate that `currentEventName == "smithers"` before dispatching, and skip events with unexpected names — but do not fail; just drop and continue.

### 3. Create Bubble Tea cmd helpers in `internal/smithers/events.go`

Add at the bottom of `events.go` (imports `github.com/charmbracelet/bubbletea` as `tea`):

```go
import tea "github.com/charmbracelet/bubbletea"

// WaitForRunEvent returns a tea.Cmd that blocks on the next event from ch.
// The view calls this in Init() to start receiving and re-dispatches it in Update()
// after each RunEventMsg, creating a self-re-scheduling event loop.
//
//   func (v *RunsView) Init() tea.Cmd {
//       v.eventCh, v.errCh = client.StreamRunEvents(ctx, runID, -1)
//       return smithers.WaitForRunEvent(runID, v.eventCh, v.errCh)
//   }
//
//   case smithers.RunEventMsg:
//       // handle event
//       return smithers.WaitForRunEvent(runID, v.eventCh, v.errCh)
func WaitForRunEvent(runID string, ch <-chan SmithersEvent, errCh <-chan error) tea.Cmd {
    return func() tea.Msg {
        select {
        case evt, ok := <-ch:
            if !ok {
                // Channel closed — check error channel.
                select {
                case err, hasErr := <-errCh:
                    if hasErr && err != nil {
                        return RunStreamErrorMsg{RunID: runID, Err: err}
                    }
                default:
                }
                return RunStreamDoneMsg{RunID: runID}
            }
            return RunEventMsg{RunID: runID, Event: evt}
        case err, ok := <-errCh:
            if ok && err != nil {
                return RunStreamErrorMsg{RunID: runID, Err: err}
            }
            return RunStreamDoneMsg{RunID: runID}
        }
    }
}
```

**Note on import**: If adding `tea` as a direct import in `events.go` creates a dependency cycle (unlikely but possible), move `WaitForRunEvent` and the `tea.Msg` types to a separate `internal/smithers/teamsg.go` file.

### 4. Add pubsub bridge for multi-subscriber fan-out

When multiple views need to subscribe to the same run's events (e.g., RunsView + ApprovalQueue both watching the same run), they should share one HTTP connection. Add a bridge function:

```go
// BridgeRunEvents pipes events from an SSE channel into a pubsub.Broker[SmithersEvent].
// Launch once per run; multiple subscribers call broker.Subscribe(ctx).
// The goroutine exits when ch is closed.
func BridgeRunEvents(
    ctx context.Context,
    runID string,
    ch <-chan SmithersEvent,
    broker *pubsub.Broker[SmithersEvent],
) {
    go func() {
        for evt := range ch {
            broker.Publish(pubsub.UpdatedEvent, evt)
        }
    }()
}
```

This sits in `events.go` with `import "github.com/anthropic/smithers-tui/internal/pubsub"`.

### 5. Create `internal/smithers/events_test.go`

All tests use `net/http/httptest.Server` — no live server required.

**Test: `TestParseSSEStream_BasicDispatch`**
Feed a multi-event SSE body through `parseSSEStream`. Assert that three events are dispatched with correct `Type`, `RunID`, `Seq`, and `Data` fields. Verify that a `": keep-alive\n\n"` heartbeat between events does not produce an event.

**Test: `TestParseSSEStream_MultilineData`**
Feed a `data:` field split across two lines (SSE spec allows `\n` continuation). Assert the two lines are concatenated with `\n` before JSON decode.

**Test: `TestParseSSEStream_LargePayload`**
Feed a `data:` field that is 200 KB of JSON. Assert no panic or scan error. Assert the event is dispatched. (Verifies the 1 MB scanner buffer increase.)

**Test: `TestParseSSEStream_MalformedJSON`**
Feed a `data:` line that is not valid JSON. Assert the event is silently dropped (no panic, no channel send). Assert the subsequent valid event is delivered normally.

**Test: `TestParseSSEStream_ContextCancellation`**
Create a context, start parseSSEStream, cancel the context mid-stream, verify the function returns promptly and the channel receives no further events.

**Test: `TestStreamReconnection`**
Use `httptest.NewServer` with a handler that sends 3 events then closes the connection. The handler counts connections. Assert:
- First connection delivers 3 events.
- Client reconnects (second connection) and delivers 3 more events from the second response.
- `afterSeq` on reconnect equals the `id:` of the last received event from the first connection.

**Test: `TestStreamContextCancelStopsReconnect`**
Use `httptest.NewServer` that immediately closes each connection. Cancel the context after 2 reconnect cycles. Assert the loop exits cleanly and `errCh` is closed (not sent to).

**Test: `TestStreamTerminalClose`**
Use `httptest.NewServer` that sends 3 events and then closes cleanly (EOF). Assert `errCh` closes without sending an error, and `ch` is also closed. Assert `RunStreamDoneMsg` is returned by `WaitForRunEvent`.

**Test: `TestWaitForRunEvent_EventDelivery`**
Create a buffered channel, send one `SmithersEvent`, call `WaitForRunEvent(...)()`; assert it returns a `RunEventMsg` with the correct event.

**Test: `TestWaitForRunEvent_ChannelClosed`**
Close the event channel and error channel, call `WaitForRunEvent(...)()`. Assert it returns `RunStreamDoneMsg`.

**Test: `TestWaitForRunEvent_ErrorDelivery`**
Close the event channel, send an error to `errCh`, call `WaitForRunEvent(...)()`. Assert it returns `RunStreamErrorMsg` with the error.

**Test: `TestStreamHTTPTimeout`**
Use `httptest.NewServer` that sends 1 event then hangs for 5 s. Verify the first event is delivered promptly (within 200 ms). This guards against accidentally using the 10 s `httpClient` timeout.

---

## File Plan

- [`internal/smithers/types.go`](/Users/williamcory/crush/internal/smithers/types.go) — add `RunStatus`, `SmithersEvent`, payload structs, `RunEventMsg`, `RunStreamErrorMsg`, `RunStreamDoneMsg`
- [`internal/smithers/events.go`](/Users/williamcory/crush/internal/smithers/events.go) (new) — `StreamRunEvents`, `runSSELoop`, `consumeRunStream`, `parseSSEStream`, `WaitForRunEvent`, `BridgeRunEvents`, `errStreamDone`
- [`internal/smithers/events_test.go`](/Users/williamcory/crush/internal/smithers/events_test.go) (new) — all tests above

No changes to `internal/smithers/client.go` — `StreamRunEvents` is defined as a method on `*Client` in the new file; Go allows methods on a type across multiple files in the same package.

---

## Validation

```bash
# Format
gofumpt -w internal/smithers/

# Vet
go vet ./internal/smithers/...

# Unit tests (all SSE tests, no live server needed)
go test ./internal/smithers/... -count=1 -v -run 'TestParseSSE|TestStream|TestWaitForRunEvent'

# Full package suite (regression guard)
go test ./internal/smithers/... -count=1

# Race detector (SSE involves goroutines)
go test -race ./internal/smithers/... -count=1

# Build sanity
go build ./...
```

Manual live-server check (requires Smithers server running):
```bash
# Start a run
cd /Users/williamcory/smithers && bun run src/cli/index.ts up examples/fan-out-fan-in.tsx -d

# Verify raw SSE wire format
curl -N -H "Accept: text/event-stream" \
  "http://127.0.0.1:7331/v1/runs/<run-id>/events?afterSeq=-1"

# Expected output:
# retry: 1000
#
# event: smithers
# data: {"type":"RunStarted","runId":"...","timestampMs":...}
# id: 0
#
# event: smithers
# data: {"type":"NodePending",...}
# id: 1
#
# : keep-alive
#
```

---

## Open Questions

1. **Bubble Tea import in events.go**: Should `WaitForRunEvent` live in `events.go` (requires `tea` import) or in a separate `internal/smithers/teamsg.go` to keep the transport layer pure? Check whether the existing package already imports `bubbletea` anywhere; if not, it may be cleaner to separate.

2. **Multi-run streaming**: Should the `Client` maintain a map of open run streams to prevent duplicate connections from multiple views? Or leave deduplication to the caller (via `BridgeRunEvents`)? Deduplication at the client level is safer but adds complexity.

3. **afterSeq persistence**: Should the SSE stream support resuming from where it left off across TUI restarts (i.e., persist `afterSeq` to disk)? The current design only resumes within a single process lifetime. Likely not needed for v1.

4. **Global event feed**: The engineering doc mentions a global `/events` endpoint (run-scoped Hono app exposes `/events`; the global node server has `/v1/runs/:id/events`). The `serve.ts` Hono app exposes a flat `/events` under its mount point. Should `StreamRunEvents` construct the full run-scoped path, or accept a raw path string to allow both patterns? Use the full path (`/v1/runs/:runId/events`) as the primary; document the Hono-app path (`/events`) as an alternative for direct run-serve mode.

5. **`eng-smithers-client-runs` coordination**: That ticket also targets `events.go`. Confirm whether `platform-sse-streaming` (this ticket) is the owner of `events.go`, with `eng-smithers-client-runs` consuming it, or whether they are concurrent. Recommendation: `platform-sse-streaming` owns the SSE transport layer; `eng-smithers-client-runs` adds the typed accessor methods that use it.

```json
{
  "document": "Implementation plan for platform-sse-streaming: SSE transport in internal/smithers/events.go with run-scoped streaming, cursor tracking, exponential backoff reconnection, Bubble Tea cmd integration, pubsub bridge, and comprehensive httptest-based test suite."
}
```

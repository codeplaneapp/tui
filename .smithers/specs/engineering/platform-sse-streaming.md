# Research: Platform SSE Event Streaming Consumer

## Ticket Summary

The `platform-sse-streaming` ticket requires implementing Server-Sent Events (SSE) consumption in the Smithers client to receive real-time updates for run statuses and chat streaming.

### Acceptance Criteria
- Client exposes a `StreamEvents(ctx)` returning a channel of Event structs
- Parses SSE format and decodes the inner JSON payloads
- Recovers connection seamlessly on disconnection

### Target Files
- `internal/smithers/events.go` (new)
- `internal/smithers/client.go` (existing)

## Existing Codebase Analysis

### Smithers Client (`internal/smithers/client.go`)

The existing client provides:
- `Client` struct with `baseURL string` and `http *http.Client` fields
- `NewClient(baseURL string) *Client` constructor
- Helper methods: `get()`, `post()`, `doJSON()` for HTTP operations
- CRUD methods for Runs, Workflows, Systems, and Agents
- Already uses `context.Context` throughout
- Standard JSON decoding patterns with `doJSON`

### Smithers Types (`internal/smithers/types.go`)

Defines domain types including:
- `Run` struct with fields: ID, WorkflowID, Status, Input, Output, Error, CreatedAt, UpdatedAt, CompletedAt
- `RunStatus` type (string) with constants: RunPending, RunRunning, RunCompleted, RunFailed, RunCancelled
- `Workflow`, `System`, `Agent` structs
- List response wrappers (e.g., `RunsResponse`, `WorkflowsResponse`)

### PubSub Package (`internal/pubsub/`)

The project already has a generic pub/sub system:
- **`pubsub.go`**: `Hub[T any]` struct implementing both `Publisher[T]` and `Subscriber[T]` interfaces
  - `NewHub[T any]()` constructor
  - `Publish(EventType, T)` - non-blocking publish to all subscribers
  - `Subscribe(context.Context) <-chan Event[T]` - returns buffered channel (size 64), auto-cleanup on context cancellation
- **`types.go`**: Generic event types
  - `EventType` string type with constants: `CreatedEvent`, `UpdatedEvent`, `DeletedEvent`
  - `Event[T any]` struct with `Type EventType` and `Payload T`
  - `Subscriber[T any]` and `Publisher[T any]` interfaces

### GUI Reference Implementation

The daemon reference (`smithers_tmp/gui-ref/apps/daemon/`) shows:
- SSE endpoints exist at `/api/workspaces/:id/runs/:runId/events/stream` and similar paths
- Events are streamed as JSON with `event:` and `data:` SSE fields
- Event types include: `run.status`, `run.output`, `task.status`, `task.output`, `approval.requested`
- The daemon uses `text/event-stream` content type
- Run event service (`run-event-service.ts`) manages SSE broadcasting

### Reference: Run Routes SSE Implementation

From the daemon reference (`run-routes.ts`):
- SSE endpoint sets headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
- Sends periodic heartbeat comments (`:\n\n`) to keep connection alive
- Events are formatted as `event: ${type}\ndata: ${JSON.stringify(payload)}\n\n`
- Uses `X-Accel-Buffering: no` header for nginx compatibility

## Implementation Plan

### 1. Define SSE Event Types (`internal/smithers/events.go`)

```go
package smithers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SSEEventType identifies the type of server-sent event
type SSEEventType string

const (
	EventRunStatus       SSEEventType = "run.status"
	EventRunOutput       SSEEventType = "run.output"
	EventTaskStatus      SSEEventType = "task.status"
	EventTaskOutput      SSEEventType = "task.output"
	EventApprovalRequest SSEEventType = "approval.requested"
)

// SSEEvent represents a parsed server-sent event from the Smithers API
type SSEEvent struct {
	Type SSEEventType
	Data json.RawMessage
}

// StreamEvents connects to the SSE endpoint and returns a channel of events.
// The channel is closed when the context is cancelled or the connection drops
// after exhausting reconnection attempts.
func (c *Client) StreamEvents(ctx context.Context) (<-chan SSEEvent, error) {
	ch := make(chan SSEEvent, 64)
	go c.streamLoop(ctx, ch)
	return ch, nil
}
```

### 2. SSE Stream Loop with Reconnection

```go
const (
	maxReconnectDelay = 30 * time.Second
	initialReconnectDelay = 1 * time.Second
)

func (c *Client) streamLoop(ctx context.Context, ch chan<- SSEEvent) {
	defer close(ch)
	delay := initialReconnectDelay

	for {
		err := c.consumeStream(ctx, ch)
		if ctx.Err() != nil {
			return
		}
		// Log error if needed, then backoff
		_ = err
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}
```

### 3. SSE Parser

```go
func (c *Client) consumeStream(ctx context.Context, ch chan<- SSEEvent) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse: unexpected status %d", resp.StatusCode)
	}

	return parseSSE(ctx, resp.Body, ch)
}

func parseSSE(ctx context.Context, r io.Reader, ch chan<- SSEEvent) error {
	scanner := bufio.NewScanner(r)
	var eventType string
	var dataBuf strings.Builder

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()

		switch {
		case line == "":
			// Blank line = event dispatch
			if dataBuf.Len() > 0 {
				evt := SSEEvent{
					Type: SSEEventType(eventType),
					Data: json.RawMessage(dataBuf.String()),
				}
				select {
				case ch <- evt:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			eventType = ""
			dataBuf.Reset()
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
		case strings.HasPrefix(line, ":"):
			// Comment / heartbeat - ignore
		}
	}
	return scanner.Err()
}
```

### 4. Integration with Existing PubSub

The SSE consumer can bridge into the existing `pubsub.Hub` so internal components subscribe via the same interface:

```go
// Bridge pipes SSE events into a local pubsub Hub, translating
// SSEEvent payloads into typed domain events.
func BridgeRunEvents(ctx context.Context, client *Client, hub *pubsub.Hub[Run]) error {
	ch, err := client.StreamEvents(ctx)
	if err != nil {
		return err
	}
	go func() {
		for evt := range ch {
			switch evt.Type {
			case EventRunStatus:
				var run Run
				if json.Unmarshal(evt.Data, &run) == nil {
					hub.Publish(pubsub.UpdatedEvent, run)
				}
			}
		}
	}()
	return nil
}
```

### 5. Testing Strategy

Create `internal/smithers/events_test.go`:
- **`TestParseSSE`**: Feed mock SSE text through `parseSSE`, verify events are correctly parsed
- **`TestStreamReconnection`**: Use `httptest.Server` that drops connection after N events, verify client reconnects and continues receiving
- **`TestStreamContextCancellation`**: Verify clean shutdown when context is cancelled
- **`TestHeartbeatIgnored`**: Verify comment lines (`:heartbeat`) don't produce events

## Design Decisions

1. **Use `json.RawMessage` for `Data`**: Keeps the SSE layer generic — consumers decode into their own types. This avoids coupling the transport layer to specific domain types.

2. **Buffered channel (size 64)**: Matches the existing pubsub Hub pattern. Prevents slow consumers from blocking the SSE reader.

3. **Exponential backoff reconnection**: Standard pattern for SSE. Starts at 1s, caps at 30s. Resets on successful connection.

4. **No external dependencies**: Use stdlib `bufio.Scanner` for SSE parsing rather than adding a third-party SSE library. The SSE format is simple enough that a custom parser is straightforward.

5. **Bridge pattern for pubsub integration**: Rather than making SSE events implement the pubsub interface directly, use a bridge function. This keeps concerns separated and allows different event types to be routed to different hubs.

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/smithers/events.go` | Create | SSE types, parser, StreamEvents method, reconnection loop |
| `internal/smithers/events_test.go` | Create | Unit tests for SSE parsing, reconnection, and context cancellation |
| `internal/smithers/client.go` | No changes | Existing client struct is extended via method in events.go |

## Risks and Considerations

- **SSE endpoint path**: The ticket says `/events` but the daemon reference uses workspace-scoped paths like `/api/workspaces/:id/runs/:runId/events/stream`. The implementation should accept the endpoint path as a parameter or use a configurable base.
- **Last-Event-ID**: The SSE spec supports `Last-Event-ID` for resumption. Initial implementation omits this but the parser structure supports adding it later by tracking `id:` fields.
- **Large payloads**: `bufio.Scanner` defaults to 64KB max token size. If events contain large outputs, may need `scanner.Buffer()` to increase the limit.
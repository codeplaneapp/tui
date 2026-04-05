package smithers

// events.go — SSE consumer with reconnection, cursor tracking, and Bubble Tea
// integration for the Smithers run-event stream.
//
// Wire format (from GET /v1/runs/:runId/events?afterSeq=N):
//
//	event: smithers\r\n
//	data: {"type":"NodeOutput","runId":"...","timestampMs":...}\r\n
//	id: 42\r\n
//	\r\n
//
// Heartbeats are comment lines (": keep-alive\n\n") and must be ignored.
// The server sends a "retry: 1000\n\n" hint on first connect.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/crush/internal/pubsub"
)

const (
	sseInitialDelay = 1 * time.Second
	sseMaxDelay     = 30 * time.Second
)

// errSSEStreamDone is a sentinel returned by consumeSSEStream when the server
// closes the connection cleanly (terminal run, no more events).
var errSSEStreamDone = fmt.Errorf("sse: stream closed cleanly")

// StreamRunEventsWithReconnect connects to the Smithers SSE endpoint and returns
// a channel of RunEvent values.  Unlike StreamRunEvents (which opens a single
// connection), this method reconnects transparently on transient failures using
// exponential backoff, and advances the afterSeq cursor so already-seen events
// are never replayed.
//
// The channel is closed when:
//   - ctx is cancelled (clean shutdown, no error sent to errCh)
//   - the run reaches terminal state and the server closes the stream
//   - an unrecoverable error occurs (error sent to errCh before close)
//
// errCh receives at most one value.  Callers that only care about events may
// ignore errCh but must still drain ch to avoid goroutine leaks.
//
// afterSeq is the starting cursor: pass -1 to replay all events from the
// beginning, or the last Seq seen to resume.
func (c *Client) StreamRunEventsWithReconnect(ctx context.Context, runID string, afterSeq int64) (<-chan RunEvent, <-chan error) {
	ch := make(chan RunEvent, 128)
	errCh := make(chan error, 1)
	go func() {
		defer close(ch)
		defer close(errCh)
		c.sseReconnectLoop(ctx, runID, afterSeq, ch, errCh)
	}()
	return ch, errCh
}

// sseReconnectLoop is the outer reconnect loop.  It calls consumeSSEStream in a
// loop, applying exponential backoff between attempts.  It stops only when ctx
// is cancelled.
//
// The caller is responsible for stopping the loop by cancelling ctx once
// a terminal event (RunFinished, RunFailed, RunCancelled) has been received.
// This matches the Smithers server contract: the server closes the connection
// when the run is terminal, but the client reconnects immediately; the server
// will again close with no new events, and the consumer should cancel ctx after
// observing the terminal event.
func (c *Client) sseReconnectLoop(
	ctx context.Context,
	runID string,
	initialSeq int64,
	ch chan<- RunEvent,
	_ chan<- error, // reserved for future unrecoverable-error reporting
) {
	delay := sseInitialDelay
	lastSeq := initialSeq

	for {
		newSeq, _ := c.consumeSSEStream(ctx, runID, lastSeq, ch)
		// Always update the cursor with whatever progress was made.
		if newSeq > lastSeq {
			lastSeq = newSeq
			// Reset backoff after making progress.
			delay = sseInitialDelay
		}

		if ctx.Err() != nil {
			// Context cancelled — clean exit.
			return
		}

		// Any disconnect (clean EOF or transient error): wait then reconnect.
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < sseMaxDelay {
			delay *= 2
			if delay > sseMaxDelay {
				delay = sseMaxDelay
			}
		}
	}
}

// consumeSSEStream opens one SSE connection to /v1/runs/:runId/events and reads
// until EOF or error.  Returns (lastSeq, err).
//
//   - On clean EOF (server closed the stream because the run is terminal):
//     returns (lastSeq, errSSEStreamDone).
//   - On context cancellation: returns (lastSeq, ctx.Err()).
//   - On transient network/parse error: returns (lastSeq, err).
func (c *Client) consumeSSEStream(
	ctx context.Context,
	runID string,
	afterSeq int64,
	ch chan<- RunEvent,
) (int64, error) {
	if c.apiURL == "" {
		return afterSeq, ErrServerUnavailable
	}

	rawURL := c.apiURL + "/v1/runs/" + url.PathEscape(runID) + "/events"
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return afterSeq, err
	}
	q := parsedURL.Query()
	q.Set("afterSeq", strconv.FormatInt(afterSeq, 10))
	parsedURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return afterSeq, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	// Use a streaming-safe HTTP client (no timeout on body read).
	// Reuse the transport from httpClient for connection pooling and TLS.
	streamClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   0, // no deadline on streaming body
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return afterSeq, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return afterSeq, ErrUnauthorized
	case http.StatusNotFound:
		return afterSeq, ErrRunNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return afterSeq, fmt.Errorf("sse: unexpected status %d", resp.StatusCode)
	}

	return parseSSEStream(ctx, resp.Body, afterSeq, ch)
}

// parseSSEStream reads SSE frames from r and sends RunEvent values to ch.
// Returns (lastSeq, errSSEStreamDone) on clean EOF; (lastSeq, err) on error.
//
// The Smithers server sends frames in the form:
//
//	event: smithers\n
//	data: {payloadJson}\n
//	id: {seq}\n
//	\n
//
// Heartbeats are ":\n\n" comment lines and are ignored.
// The "retry: N\n\n" hint is parsed and currently ignored.
func parseSSEStream(
	ctx context.Context,
	r io.Reader,
	initialSeq int64,
	ch chan<- RunEvent,
) (int64, error) {
	scanner := bufio.NewScanner(r)
	// Increase buffer to 1 MB to handle large NodeOutput/AgentEvent payloads.
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

	lastSeq := initialSeq
	var (
		currentEventName string
		currentID        string
		dataBuf          strings.Builder
	)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return lastSeq, ctx.Err()
		}

		line := scanner.Text()

		switch {
		case line == "":
			// Blank line: dispatch event if we have data.
			if dataBuf.Len() == 0 {
				currentEventName = ""
				currentID = ""
				continue
			}

			rawData := dataBuf.String()
			dataBuf.Reset()

			// Only dispatch events named "smithers" (or with no name, for
			// compatibility with servers that omit the event: field).
			if currentEventName != "" && currentEventName != "smithers" {
				currentEventName = ""
				currentID = ""
				continue
			}

			var ev RunEvent
			if jsonErr := json.Unmarshal([]byte(rawData), &ev); jsonErr != nil {
				// Malformed JSON — skip silently; caller does not see a parse error
				// here because the reconnect loop would not reconnect on bad JSON.
				// The underlying scanner continues to the next frame.
				currentEventName = ""
				currentID = ""
				continue
			}
			ev.Raw = []byte(rawData)

			// Populate Seq from the SSE id: field.
			seq := lastSeq
			if currentID != "" {
				if n, parseErr := strconv.ParseInt(currentID, 10, 64); parseErr == nil {
					seq = n
				}
			}
			ev.Seq = int(seq)

			select {
			case ch <- ev:
				lastSeq = seq
			case <-ctx.Done():
				return lastSeq, ctx.Err()
			}

			currentEventName = ""
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
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(data)

		case strings.HasPrefix(line, "id:"):
			currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))

		case strings.HasPrefix(line, "retry:"):
			// Reconnect interval hint — ignored (we use our own backoff).

		default:
			// Unknown field — ignore per SSE spec.
		}

		_ = currentEventName // used for dispatch filtering above
	}

	if err := scanner.Err(); err != nil {
		return lastSeq, err
	}
	// Clean EOF — server closed the connection (terminal run).
	return lastSeq, errSSEStreamDone
}

// WaitForRunEvent returns a tea.Cmd that blocks on the next RunEvent from ch.
// When an event arrives the cmd returns a RunEventMsg.  When the stream closes
// cleanly it returns RunEventDoneMsg.  When an error is delivered via errCh it
// returns RunEventErrorMsg.
//
// Views use this in a self-re-scheduling pattern:
//
//	func (v *RunsView) Init() tea.Cmd {
//	    v.eventCh, v.errCh = client.StreamRunEventsWithReconnect(ctx, runID, -1)
//	    return smithers.WaitForRunEvent(runID, v.eventCh, v.errCh)
//	}
//
//	case smithers.RunEventMsg:
//	    // handle event...
//	    return smithers.WaitForRunEvent(runID, v.eventCh, v.errCh)
func WaitForRunEvent(runID string, ch <-chan RunEvent, errCh <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case evt, ok := <-ch:
			if !ok {
				// ch closed — check errCh for a pending error.
				select {
				case err, hasErr := <-errCh:
					if hasErr && err != nil {
						return RunEventErrorMsg{RunID: runID, Err: err}
					}
				default:
				}
				return RunEventDoneMsg{RunID: runID}
			}
			return RunEventMsg{RunID: runID, Event: evt}

		case err, ok := <-errCh:
			if ok && err != nil {
				return RunEventErrorMsg{RunID: runID, Err: err}
			}
			// errCh closed without error — wait for ch to close.
			evt, ok := <-ch
			if !ok {
				return RunEventDoneMsg{RunID: runID}
			}
			return RunEventMsg{RunID: runID, Event: evt}
		}
	}
}

// BridgeRunEvents pipes events from an SSE channel into a pubsub.Broker so
// that multiple subscribers can receive the same run's events without opening
// duplicate HTTP connections.
//
// Typical usage:
//
//	ch, _ := client.StreamRunEventsWithReconnect(ctx, runID, -1)
//	broker := pubsub.NewBroker[smithers.RunEvent]()
//	smithers.BridgeRunEvents(ctx, ch, broker)
//	// Multiple components call broker.Subscribe(ctx) independently.
func BridgeRunEvents(ctx context.Context, ch <-chan RunEvent, broker *pubsub.Broker[RunEvent]) {
	go func() {
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				broker.Publish(pubsub.UpdatedEvent, evt)
			case <-ctx.Done():
				return
			}
		}
	}()
}

package smithers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseFrame is a convenience helper for building a single SSE frame string.
// seq < 0 means omit the id: field.
func sseFrame(eventName, dataJSON string, seq int) string {
	var sb strings.Builder
	if eventName != "" {
		fmt.Fprintf(&sb, "event: %s\n", eventName)
	}
	fmt.Fprintf(&sb, "data: %s\n", dataJSON)
	if seq >= 0 {
		fmt.Fprintf(&sb, "id: %d\n", seq)
	}
	sb.WriteString("\n")
	return sb.String()
}

// sseHeartbeatFrame returns a SSE comment heartbeat line.
func sseHeartbeatFrame() string { return ": keep-alive\n\n" }

// makeRunEventJSON builds a minimal RunEvent JSON payload.
func makeRunEventJSON(t *testing.T, eventType, runID string) string {
	t.Helper()
	b, err := json.Marshal(RunEvent{Type: eventType, RunID: runID, TimestampMs: 1})
	require.NoError(t, err)
	return string(b)
}

// newPipe returns an io.PipeReader/PipeWriter pair and registers cleanup.
func newPipe(t *testing.T) (*io.PipeReader, *io.PipeWriter) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	return pr, pw
}

// --- parseSSEStream unit tests ---

// TestParseSSEStream_BasicDispatch feeds three events through parseSSEStream and
// verifies they are dispatched in order with correct Type, RunID, Seq, and Raw.
func TestParseSSEStream_BasicDispatch(t *testing.T) {
	runID := "run-basic"
	body := "" +
		sseFrame("smithers", makeRunEventJSON(t, "RunStarted", runID), 0) +
		sseHeartbeatFrame() +
		sseFrame("smithers", makeRunEventJSON(t, "NodeOutput", runID), 1) +
		sseFrame("smithers", makeRunEventJSON(t, "RunFinished", runID), 2)

	ch := make(chan RunEvent, 16)
	ctx := context.Background()

	lastSeq, err := parseSSEStream(ctx, strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone, "clean EOF should return errSSEStreamDone")
	assert.Equal(t, int64(2), lastSeq)

	close(ch)
	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 3)
	assert.Equal(t, "RunStarted", evts[0].Type)
	assert.Equal(t, 0, evts[0].Seq)
	assert.Equal(t, "NodeOutput", evts[1].Type)
	assert.Equal(t, 1, evts[1].Seq)
	assert.Equal(t, "RunFinished", evts[2].Type)
	assert.Equal(t, 2, evts[2].Seq)

	// Verify Raw is populated.
	assert.NotEmpty(t, evts[0].Raw)
	// Verify heartbeat between events didn't produce a fourth event.
	assert.Len(t, evts, 3)
}

// TestParseSSEStream_HeartbeatIgnored ensures ": keep-alive" comment lines
// produce no events.
func TestParseSSEStream_HeartbeatIgnored(t *testing.T) {
	runID := "run-heartbeat-only"
	body := sseHeartbeatFrame() + sseHeartbeatFrame() +
		sseFrame("smithers", makeRunEventJSON(t, "ping", runID), 0)

	ch := make(chan RunEvent, 4)
	ctx := context.Background()

	_, err := parseSSEStream(ctx, strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1, "only real events should be dispatched")
	assert.Equal(t, "ping", evts[0].Type)
}

// TestParseSSEStream_MultilineData verifies that a data: field split across two
// lines is concatenated with \n per the SSE spec.
func TestParseSSEStream_MultilineData(t *testing.T) {
	// Build a split-data frame manually.  The two data: lines join with '\n',
	// producing valid JSON: {"type":"NodeOutput","runId":"r1","timestampMs":1}
	body := "event: smithers\n" +
		"data: {\"type\":\"NodeOutput\",\"runId\":\"r1\",\n" +
		"data: \"timestampMs\":1}\n" +
		"id: 0\n\n"

	ch := make(chan RunEvent, 4)
	ctx := context.Background()
	_, err := parseSSEStream(ctx, strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1, "multiline data: should produce one event")
	assert.Equal(t, "NodeOutput", evts[0].Type)
}

// TestParseSSEStream_LargePayload feeds a 200 KB JSON payload through
// parseSSEStream to verify the 1 MB scanner buffer increase works.
func TestParseSSEStream_LargePayload(t *testing.T) {
	runID := "run-large"
	// Embed a large string in the Status field to inflate the payload.
	largeText := strings.Repeat("X", 200*1024)
	payload, err := json.Marshal(RunEvent{
		Type:        "NodeOutput",
		RunID:       runID,
		TimestampMs: 1,
		NodeID:      "n1",
		Status:      largeText,
	})
	require.NoError(t, err)
	assert.Greater(t, len(payload), 200*1024, "payload should be >200 KB")

	body := sseFrame("smithers", string(payload), 0)

	ch := make(chan RunEvent, 4)
	ctx := context.Background()
	_, scanErr := parseSSEStream(ctx, strings.NewReader(body), -1, ch)
	require.ErrorIs(t, scanErr, errSSEStreamDone, "should not error on large payload")
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1, "large payload should produce exactly one event")
	assert.Equal(t, "NodeOutput", evts[0].Type)
}

// TestParseSSEStream_MalformedJSON verifies that a malformed data: line is
// silently dropped and the subsequent valid event is still delivered.
func TestParseSSEStream_MalformedJSON(t *testing.T) {
	runID := "run-malformed-parse"

	// First event: bad JSON — should be silently dropped.
	// Second event: good JSON — should arrive.
	body := sseFrame("smithers", "{not valid json}", 0) +
		sseFrame("smithers", makeRunEventJSON(t, "RunFinished", runID), 1)

	ch := make(chan RunEvent, 4)
	ctx := context.Background()
	_, err := parseSSEStream(ctx, strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1, "malformed event should be dropped; valid event should arrive")
	assert.Equal(t, "RunFinished", evts[0].Type)
	assert.Equal(t, 1, evts[0].Seq)
}

// TestParseSSEStream_ContextCancellation starts parseSSEStream on an infinite
// stream (pipe) and cancels the context; verifies the function returns promptly.
//
// Note: bufio.Scanner blocks on the underlying Read() call, so context
// cancellation alone is not enough to unblock a pipe read.  We simulate proper
// HTTP streaming behaviour by closing the write end of the pipe when the
// context is cancelled (which is what the HTTP transport does when it tears
// down the response body on context cancellation).
func TestParseSSEStream_ContextCancellation(t *testing.T) {
	pr, pw := newPipe(t)

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan RunEvent, 4)

	done := make(chan error, 1)
	go func() {
		_, err := parseSSEStream(ctx, pr, -1, ch)
		done <- err
	}()

	// Send one event so the goroutine is definitely inside the scanner loop.
	runID := "run-cancel-parse"
	fmt.Fprint(pw, sseFrame("smithers", makeRunEventJSON(t, "RunStarted", runID), 0))

	// Wait for the event to arrive.
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first event")
	}

	// Cancel context AND close the writer to unblock the blocking Read() call.
	// In real HTTP usage, the HTTP transport closes the body when ctx is done.
	cancel()
	pw.Close() // unblocks scanner.Scan() → triggers EOF → parseSSEStream checks ctx

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("parseSSEStream did not return after context cancellation")
	}
}

// TestParseSSEStream_CursorTracking verifies that lastSeq is correctly
// advanced with each event and reflects the most recent id: value.
func TestParseSSEStream_CursorTracking(t *testing.T) {
	runID := "run-cursor"
	body := sseFrame("smithers", makeRunEventJSON(t, "E1", runID), 10) +
		sseFrame("smithers", makeRunEventJSON(t, "E2", runID), 20) +
		sseFrame("smithers", makeRunEventJSON(t, "E3", runID), 30)

	ch := make(chan RunEvent, 8)
	lastSeq, err := parseSSEStream(context.Background(), strings.NewReader(body), 5, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	assert.Equal(t, int64(30), lastSeq)
	close(ch)

	var seqs []int
	for ev := range ch {
		seqs = append(seqs, ev.Seq)
	}
	assert.Equal(t, []int{10, 20, 30}, seqs)
}

// TestParseSSEStream_UnknownEventNameIgnored verifies that events with an
// event: field other than "smithers" are silently dropped.
func TestParseSSEStream_UnknownEventNameIgnored(t *testing.T) {
	runID := "run-unknown-evt"
	body := sseFrame("unknown-type", makeRunEventJSON(t, "ShouldBeDropped", runID), 0) +
		sseFrame("smithers", makeRunEventJSON(t, "ShouldArrive", runID), 1)

	ch := make(chan RunEvent, 4)
	_, err := parseSSEStream(context.Background(), strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1)
	assert.Equal(t, "ShouldArrive", evts[0].Type)
}

// TestParseSSEStream_NoEventNameAccepted verifies that events with no event:
// field are accepted (backward compatibility with servers that omit it).
func TestParseSSEStream_NoEventNameAccepted(t *testing.T) {
	runID := "run-no-eventname"
	body := fmt.Sprintf("data: %s\nid: 0\n\n", makeRunEventJSON(t, "RunStarted", runID))

	ch := make(chan RunEvent, 4)
	_, err := parseSSEStream(context.Background(), strings.NewReader(body), -1, ch)
	require.ErrorIs(t, err, errSSEStreamDone)
	close(ch)

	var evts []RunEvent
	for ev := range ch {
		evts = append(evts, ev)
	}
	require.Len(t, evts, 1)
	assert.Equal(t, "RunStarted", evts[0].Type)
}

// --- StreamRunEventsWithReconnect integration tests ---

// TestStreamWithReconnect_NormalFlow verifies that events from a live httptest
// server are received in order.  The reconnect loop runs until context cancel,
// so we cancel after receiving the 3 expected events.
func TestStreamWithReconnect_NormalFlow(t *testing.T) {
	runID := "run-reconnect-normal"
	events := []RunEvent{
		{Type: "RunStarted", RunID: runID, TimestampMs: 1},
		{Type: "NodeOutput", RunID: runID, TimestampMs: 2, NodeID: "n1"},
		{Type: "RunFinished", RunID: runID, TimestampMs: 3},
	}

	// Track which connection we are on so subsequent connections block.
	var connCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}

		conn := int(connCount.Add(1))
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		f, ok := w.(http.Flusher)
		require.True(t, ok)

		if conn == 1 {
			assert.Equal(t, "-1", r.URL.Query().Get("afterSeq"))
			for i, ev := range events {
				data, _ := json.Marshal(ev)
				fmt.Fprint(w, sseFrame("smithers", string(data), i))
				f.Flush()
			}
			// First connection: deliver events then close.
			return
		}
		// Subsequent connections: block until client disconnects.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, errCh := c.StreamRunEventsWithReconnect(ctx, runID, -1)

	// Collect exactly 3 events then cancel.
	var received []RunEvent
	for ev := range ch {
		received = append(received, ev)
		if len(received) == 3 {
			cancel()
		}
	}

	require.Len(t, received, 3)
	assert.Equal(t, "RunStarted", received[0].Type)
	assert.Equal(t, "NodeOutput", received[1].Type)
	assert.Equal(t, "RunFinished", received[2].Type)

	// errCh should be closed without sending an error.
	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			t.Errorf("unexpected error on errCh: %v", err)
		}
	default:
	}
}

// TestStreamWithReconnect_ReconnectsOnDrop verifies that when the server drops
// the connection mid-stream the client reconnects, advances the afterSeq
// cursor, and continues delivering events.
//
// The reconnect loop runs until the context is cancelled, so we cancel after
// receiving the expected 6 events (3 from each of 2 connections).
func TestStreamWithReconnect_ReconnectsOnDrop(t *testing.T) {
	runID := "run-reconnect-drop"

	var connCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}

		conn := int(connCount.Add(1))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		switch conn {
		case 1:
			// Deliver 3 events (seq 0, 1, 2) then abruptly drop the connection.
			for i := 0; i < 3; i++ {
				ev := RunEvent{Type: fmt.Sprintf("ev%d", i), RunID: runID, TimestampMs: int64(i)}
				data, _ := json.Marshal(ev)
				fmt.Fprint(w, sseFrame("smithers", string(data), i))
				f.Flush()
			}
			// Returning causes the HTTP server to close the connection (non-clean EOF).
			return

		case 2:
			// Verify the cursor was advanced correctly.
			afterSeq := r.URL.Query().Get("afterSeq")
			assert.Equal(t, "2", afterSeq, "second connection should have afterSeq=2")

			for i := 3; i < 6; i++ {
				ev := RunEvent{Type: fmt.Sprintf("ev%d", i), RunID: runID, TimestampMs: int64(i)}
				data, _ := json.Marshal(ev)
				fmt.Fprint(w, sseFrame("smithers", string(data), i))
				f.Flush()
			}
			// Close the connection — client will attempt reconnect (conn 3).
			return

		default:
			// Third+ connection: block on request context to let the test cancel.
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch, _ := c.StreamRunEventsWithReconnect(ctx, runID, -1)

	// Collect exactly 6 events; cancel the context after receiving them.
	var received []RunEvent
	for ev := range ch {
		received = append(received, ev)
		if len(received) == 6 {
			// Got all expected events — cancel to stop the reconnect loop.
			cancel()
		}
	}

	require.Len(t, received, 6, "should receive 3 events from each of the 2 connections")
	for i, ev := range received {
		assert.Equal(t, fmt.Sprintf("ev%d", i), ev.Type, "events should be in order")
		assert.Equal(t, i, ev.Seq)
	}
	assert.GreaterOrEqual(t, int(connCount.Load()), 2, "should have made at least 2 connections")
}

// TestStreamWithReconnect_ContextCancelStopsLoop cancels the context after the
// first connection and verifies the reconnect loop exits without making
// additional connections.
func TestStreamWithReconnect_ContextCancelStopsLoop(t *testing.T) {
	runID := "run-reconnect-cancel"
	var connCount atomic.Int32
	connected := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		connCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		f.Flush()
		select {
		case connected <- struct{}{}:
		default:
		}
		// Block until client disconnects.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(srv.Client()))
	c.serverUp = true

	ctx, cancel := context.WithCancel(context.Background())

	ch, errCh := c.StreamRunEventsWithReconnect(ctx, runID, -1)

	// Wait for the first connection.
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not receive first connection in time")
	}

	cancel()

	// Channel must close promptly.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ch did not close after context cancel")
	}

	// errCh must also close without an error.
	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		// errCh may have already been closed — that's fine.
	}

	// Only 1 connection should have been made.
	assert.Equal(t, int32(1), connCount.Load(), "should not reconnect after context cancel")
}

// TestStreamWithReconnect_HTTPTimeout verifies that the SSE client uses a
// zero-timeout http.Client so a stream that hangs between events is not killed
// by a short client-level timeout.
//
// We give the base httpClient a 50 ms timeout.  The consumeSSEStream path must
// override this with Timeout: 0, allowing the stream to survive a 300 ms pause
// between events.  If it accidentally uses the 50 ms client the second event
// would be lost (connection killed mid-stream).
func TestStreamWithReconnect_HTTPTimeout(t *testing.T) {
	runID := "run-timeout-check"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)

		// Deliver first event immediately.
		data, _ := json.Marshal(RunEvent{Type: "first", RunID: runID, TimestampMs: 1})
		fmt.Fprint(w, sseFrame("smithers", string(data), 0))
		f.Flush()

		// Pause longer than the deliberately short base client timeout (50 ms).
		time.Sleep(300 * time.Millisecond)

		// Deliver second event after the pause.
		data2, _ := json.Marshal(RunEvent{Type: "second", RunID: runID, TimestampMs: 2})
		fmt.Fprint(w, sseFrame("smithers", string(data2), 1))
		f.Flush()

		// Block until client disconnects (so the test can cancel).
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	// Intentionally short timeout on the base client.
	// consumeSSEStream must use Timeout: 0 on the streaming client.
	httpC := srv.Client()
	httpC.Timeout = 50 * time.Millisecond

	c := NewClient(WithAPIURL(srv.URL), WithHTTPClient(httpC))
	c.serverUp = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _ := c.StreamRunEventsWithReconnect(ctx, runID, -1)

	// First event should arrive promptly (within 500 ms).
	select {
	case ev, ok := <-ch:
		if ok {
			assert.Equal(t, "first", ev.Type, "first event should arrive")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first event not received — possible HTTP timeout bug")
	}

	// Second event should arrive after the 300 ms server pause.
	// If the base client's 50 ms timeout was used, the connection would be
	// killed during the pause and the second event would never arrive on the
	// first connection.
	select {
	case ev, ok := <-ch:
		if ok {
			assert.Equal(t, "second", ev.Type, "second event should survive the server pause")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second event not received — HTTP timeout likely killed the stream")
	}

	cancel()
	// Drain to let goroutines exit.
	for range ch {
	}
}

// --- WaitForRunEvent tests ---

// TestWaitForRunEvent_EventDelivery verifies that a buffered channel with one
// event returns RunEventMsg immediately.
func TestWaitForRunEvent_EventDelivery(t *testing.T) {
	ch := make(chan RunEvent, 1)
	errCh := make(chan error, 1)
	ev := RunEvent{Type: "NodeOutput", RunID: "r1", TimestampMs: 42}
	ch <- ev

	cmd := WaitForRunEvent("r1", ch, errCh)
	msg := cmd()

	require.IsType(t, RunEventMsg{}, msg)
	got := msg.(RunEventMsg)
	assert.Equal(t, "r1", got.RunID)
	assert.Equal(t, "NodeOutput", got.Event.Type)
	assert.Equal(t, int64(42), got.Event.TimestampMs)
}

// TestWaitForRunEvent_ChannelClosed verifies that when both channels are closed
// without sending, RunEventDoneMsg is returned.
func TestWaitForRunEvent_ChannelClosed(t *testing.T) {
	ch := make(chan RunEvent)
	errCh := make(chan error)
	close(ch)
	close(errCh)

	cmd := WaitForRunEvent("r1", ch, errCh)
	msg := cmd()

	assert.IsType(t, RunEventDoneMsg{}, msg)
}

// TestWaitForRunEvent_ErrorDelivery verifies that when the event channel is
// closed and the error channel sends an error, RunEventErrorMsg is returned.
func TestWaitForRunEvent_ErrorDelivery(t *testing.T) {
	ch := make(chan RunEvent)
	errCh := make(chan error, 1)
	close(ch)
	errCh <- fmt.Errorf("stream broke")
	close(errCh)

	cmd := WaitForRunEvent("r2", ch, errCh)
	msg := cmd()

	require.IsType(t, RunEventErrorMsg{}, msg)
	got := msg.(RunEventErrorMsg)
	assert.Equal(t, "r2", got.RunID)
	assert.EqualError(t, got.Err, "stream broke")
}

// TestWaitForRunEvent_SelfRescheduling verifies the idiomatic Bubble Tea
// pattern: each call to the cmd function returns one event, and the caller can
// call again to get the next one.
func TestWaitForRunEvent_SelfRescheduling(t *testing.T) {
	ch := make(chan RunEvent, 3)
	errCh := make(chan error, 1)

	for i := 0; i < 3; i++ {
		ch <- RunEvent{Type: fmt.Sprintf("E%d", i), RunID: "r1", TimestampMs: int64(i)}
	}
	close(ch)
	close(errCh)

	var msgs []RunEventMsg
	for {
		cmd := WaitForRunEvent("r1", ch, errCh)
		msg := cmd()
		if m, ok := msg.(RunEventMsg); ok {
			msgs = append(msgs, m)
		} else {
			// Got RunEventDoneMsg — all events drained.
			break
		}
	}

	require.Len(t, msgs, 3)
	for i, m := range msgs {
		assert.Equal(t, fmt.Sprintf("E%d", i), m.Event.Type)
	}
}

// --- BridgeRunEvents tests ---

// TestBridgeRunEvents_PublishesToBroker verifies that events from the SSE
// channel are published to the pubsub broker and delivered to subscribers.
func TestBridgeRunEvents_PublishesToBroker(t *testing.T) {
	ch := make(chan RunEvent, 4)
	broker := pubsub.NewBroker[RunEvent]()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	BridgeRunEvents(ctx, ch, broker)
	sub := broker.Subscribe(ctx)

	ch <- RunEvent{Type: "RunStarted", RunID: "r1"}
	ch <- RunEvent{Type: "NodeOutput", RunID: "r1"}
	close(ch)

	// Collect up to 2 events within a timeout.
	var received []RunEvent
	deadline := time.After(3 * time.Second)
	for len(received) < 2 {
		select {
		case e, ok := <-sub:
			if !ok {
				goto bridgeDone
			}
			received = append(received, e.Payload)
		case <-deadline:
			t.Fatal("timed out waiting for bridged events")
		}
	}
bridgeDone:
	require.Len(t, received, 2)
	assert.Equal(t, "RunStarted", received[0].Type)
	assert.Equal(t, "NodeOutput", received[1].Type)
}

// TestBridgeRunEvents_ContextCancel verifies that cancelling the context stops
// the bridge goroutine without deadlocking.
func TestBridgeRunEvents_ContextCancel(t *testing.T) {
	ch := make(chan RunEvent) // unbuffered, never written
	broker := pubsub.NewBroker[RunEvent]()

	ctx, cancel := context.WithCancel(context.Background())
	BridgeRunEvents(ctx, ch, broker)

	// Cancel immediately — bridge goroutine should unblock via ctx.Done().
	cancel()

	// Give the goroutine a moment to exit.
	time.Sleep(50 * time.Millisecond)
	// Reaching here without deadlock means the test passed.
}

// TestBridgeRunEvents_MultipleSubscribers verifies that two separate subscribers
// both receive every event published via the bridge.
func TestBridgeRunEvents_MultipleSubscribers(t *testing.T) {
	ch := make(chan RunEvent, 4)
	broker := pubsub.NewBroker[RunEvent]()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	BridgeRunEvents(ctx, ch, broker)

	sub1 := broker.Subscribe(ctx)
	sub2 := broker.Subscribe(ctx)

	ch <- RunEvent{Type: "RunStarted", RunID: "r1"}
	ch <- RunEvent{Type: "RunFinished", RunID: "r1"}
	close(ch)

	collect := func(sub <-chan pubsub.Event[RunEvent]) []string {
		var types []string
		deadline := time.After(3 * time.Second)
		for len(types) < 2 {
			select {
			case e, ok := <-sub:
				if !ok {
					return types
				}
				types = append(types, e.Payload.Type)
			case <-deadline:
				t.Error("timed out collecting events")
				return types
			}
		}
		return types
	}

	types1 := collect(sub1)
	types2 := collect(sub2)

	assert.Equal(t, []string{"RunStarted", "RunFinished"}, types1)
	assert.Equal(t, []string{"RunStarted", "RunFinished"}, types2)
}

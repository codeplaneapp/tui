package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestClientSubscribeEventsReconnectsAndClosesOnCancel(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})
	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))

	var connects atomic.Int32
	secondConnectionReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/workspaces/ws-123/events", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := http.NewResponseController(w)

		connect := connects.Add(1)
		_, _ = fmt.Fprint(w, "retry: 15\n\n")
		require.NoError(t, flusher.Flush())

		switch connect {
		case 1:
			writeWorkspaceSSEEvent(t, w, pubsub.PayloadTypePermissionNotification, pubsub.Event[proto.PermissionNotification]{
				Type: pubsub.CreatedEvent,
				Payload: proto.PermissionNotification{
					ToolCallID: "tool-1",
				},
			})
			require.NoError(t, flusher.Flush())
		default:
			writeWorkspaceSSEEvent(t, w, pubsub.PayloadTypePermissionNotification, pubsub.Event[proto.PermissionNotification]{
				Type: pubsub.CreatedEvent,
				Payload: proto.PermissionNotification{
					ToolCallID: "tool-2",
					Granted:    true,
				},
			})
			require.NoError(t, flusher.Flush())
			close(secondConnectionReady)
			<-r.Context().Done()
		}
	}))
	defer server.Close()

	c, err := NewClient(t.TempDir(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, err := c.SubscribeEvents(ctx, "ws-123")
	require.NoError(t, err)

	firstEvent := readWorkspaceEvent(t, events)
	require.Equal(t, "tool-1", firstEvent.Payload.ToolCallID)
	require.False(t, firstEvent.Payload.Granted)

	select {
	case <-secondConnectionReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE reconnect")
	}

	secondEvent := readWorkspaceEvent(t, events)
	require.Equal(t, "tool-2", secondEvent.Payload.ToolCallID)
	require.True(t, secondEvent.Payload.Granted)

	cancel()

	select {
	case _, ok := <-events:
		require.False(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE channel to close")
	}

	require.GreaterOrEqual(t, connects.Load(), int32(2))

	spans := observability.RecentSpans(20)
	require.NotEmpty(t, spans)
	hasSubscribeSpan := false
	streamSpans := 0
	for _, span := range spans {
		if span.Name == "client.workspace_events.subscribe" {
			hasSubscribeSpan = true
		}
		if span.Name == "client.workspace_events.stream" {
			streamSpans++
		}
	}
	require.True(t, hasSubscribeSpan)
	require.GreaterOrEqual(t, streamSpans, 2)
}

func TestClientSubscribeEventsReturnsTerminalStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	c, err := NewClient(t.TempDir(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	_, err = c.SubscribeEvents(t.Context(), "ws-missing")
	require.Error(t, err)
	require.Contains(t, err.Error(), "status code 404")
}

func TestClientReadWorkspaceEventsStreamSkipsMalformedFrames(t *testing.T) {
	t.Run("missing data prefix", func(t *testing.T) {
		envelope := encodeWorkspaceEnvelope(t, pubsub.PayloadTypePermissionNotification, pubsub.Event[proto.PermissionNotification]{
			Type: pubsub.CreatedEvent,
			Payload: proto.PermissionNotification{
				ToolCallID: "tool-missing-prefix",
			},
		})

		server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeRawWorkspaceSSE(t, w, envelope+"\n\n")
		})
		defer server.Close()

		c := newWorkspaceEventsTestClient(t, server)
		rsp := openWorkspaceEventsTestResponse(t, c)
		events := make(chan any, 1)

		retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
		require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
		require.ErrorIs(t, err, errWorkspaceEventStreamClosed)
		requireNoWorkspaceEvent(t, events)
	})

	t.Run("truncated data line", func(t *testing.T) {
		server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeRawWorkspaceSSE(t, w, `data: {"type":"permission_notification","payload":`)
		})
		defer server.Close()

		c := newWorkspaceEventsTestClient(t, server)
		rsp := openWorkspaceEventsTestResponse(t, c)
		events := make(chan any, 1)

		retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
		require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
		require.ErrorIs(t, err, errWorkspaceEventStreamClosed)
		requireNoWorkspaceEvent(t, events)
	})
}

func TestClientReadWorkspaceEventsStreamDropsUnknownEventTypes(t *testing.T) {
	envelope, err := json.Marshal(pubsub.Payload{
		Type:    "mystery_event",
		Payload: json.RawMessage(`{"type":"created","payload":{"tool_call_id":"tool-unknown"}}`),
	})
	require.NoError(t, err)

	server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeRawWorkspaceSSE(t, w, "data:"+string(envelope)+"\n\n")
	})
	defer server.Close()

	c := newWorkspaceEventsTestClient(t, server)
	rsp := openWorkspaceEventsTestResponse(t, c)
	events := make(chan any, 1)

	retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
	require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
	require.ErrorIs(t, err, errWorkspaceEventStreamClosed)
	requireNoWorkspaceEvent(t, events)
}

func TestClientReadWorkspaceEventsStreamAggregatesMultilineData(t *testing.T) {
	server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeRawWorkspaceSSE(t, w,
			"data: {\"type\":\"permission_notification\",\n"+
				"data: \"payload\":{\"type\":\"created\",\"payload\":{\"tool_call_id\":\"tool-multiline\",\"granted\":true}}}\n\n",
		)
	})
	defer server.Close()

	c := newWorkspaceEventsTestClient(t, server)
	rsp := openWorkspaceEventsTestResponse(t, c)
	events := make(chan any, 1)

	retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
	require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
	require.ErrorIs(t, err, errWorkspaceEventStreamClosed)

	ev := readWorkspaceEvent(t, events)
	require.Equal(t, pubsub.CreatedEvent, ev.Type)
	require.Equal(t, "tool-multiline", ev.Payload.ToolCallID)
	require.True(t, ev.Payload.Granted)
	requireNoWorkspaceEvent(t, events)
}

func TestClientReadWorkspaceEventsStreamSkipsHeartbeatOnlyFrames(t *testing.T) {
	server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeRawWorkspaceSSE(t, w, ":\n\n:\n\n")
	})
	defer server.Close()

	c := newWorkspaceEventsTestClient(t, server)
	rsp := openWorkspaceEventsTestResponse(t, c)
	events := make(chan any, 1)

	retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
	require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
	require.ErrorIs(t, err, errWorkspaceEventStreamClosed)
	requireNoWorkspaceEvent(t, events)
}

func TestClientReadWorkspaceEventsStreamScannerBoundary(t *testing.T) {
	tests := []struct {
		name      string
		lineBytes int
		wantEvent bool
		wantError string
	}{
		{
			name:      "buffer minus 1",
			lineBytes: workspaceEventsScannerBufferBytes - 1,
			wantEvent: true,
			wantError: errWorkspaceEventStreamClosed.Error(),
		},
		{
			name:      "buffer exact",
			lineBytes: workspaceEventsScannerBufferBytes,
			wantEvent: false,
			wantError: "token too long",
		},
		{
			name:      "buffer plus 1",
			lineBytes: workspaceEventsScannerBufferBytes + 1,
			wantEvent: false,
			wantError: "token too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := makeSizedWorkspaceDataLine(t, tt.lineBytes)

			server := newWorkspaceEventsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeRawWorkspaceSSE(t, w, line+"\n\n")
			})
			defer server.Close()

			c := newWorkspaceEventsTestClient(t, server)
			rsp := openWorkspaceEventsTestResponse(t, c)
			events := make(chan any, 1)

			retryDelay, err := c.readWorkspaceEventsStream(t.Context(), "ws-123", rsp, events)
			require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantError)

			if tt.wantEvent {
				ev := readWorkspaceEvent(t, events)
				require.Equal(t, pubsub.CreatedEvent, ev.Type)
				require.Len(t, ev.Payload.ToolCallID, tt.lineBytes-len("data:")-sizedPermissionNotificationEnvelopeBaseBytes(t))
				requireNoWorkspaceEvent(t, events)
				return
			}

			requireNoWorkspaceEvent(t, events)
		})
	}
}

func writeWorkspaceSSEEvent[T any](t *testing.T, w http.ResponseWriter, payloadType pubsub.PayloadType, payload pubsub.Event[T]) {
	t.Helper()

	envelope := encodeWorkspaceEnvelope(t, payloadType, payload)

	_, err := fmt.Fprintf(w, "data: %s\n\n", envelope)
	require.NoError(t, err)
}

func readWorkspaceEvent(t *testing.T, events <-chan any) pubsub.Event[proto.PermissionNotification] {
	t.Helper()

	select {
	case ev, ok := <-events:
		require.True(t, ok)
		typed, ok := ev.(pubsub.Event[proto.PermissionNotification])
		require.True(t, ok, "unexpected event type %T", ev)
		return typed
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for workspace event")
		return pubsub.Event[proto.PermissionNotification]{}
	}
}

func newWorkspaceEventsTestServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/workspaces/ws-123/events", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		handler(w, r)
	}))
}

func newWorkspaceEventsTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	c, err := NewClient(t.TempDir(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	return c
}

func openWorkspaceEventsTestResponse(t *testing.T, c *Client) *http.Response {
	t.Helper()

	rsp, retryDelay, err := c.openWorkspaceEventsStream(t.Context(), "ws-123")
	require.NoError(t, err)
	require.Equal(t, workspaceEventsDefaultRetryHint, retryDelay)
	return rsp
}

func writeRawWorkspaceSSE(t *testing.T, w http.ResponseWriter, raw string) {
	t.Helper()

	_, err := fmt.Fprint(w, raw)
	require.NoError(t, err)

	require.NoError(t, http.NewResponseController(w).Flush())
}

func encodeWorkspaceEnvelope[T any](t *testing.T, payloadType pubsub.PayloadType, payload pubsub.Event[T]) string {
	t.Helper()

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	envelope, err := json.Marshal(pubsub.Payload{
		Type:    payloadType,
		Payload: raw,
	})
	require.NoError(t, err)

	return string(envelope)
}

func makeSizedWorkspaceDataLine(t *testing.T, lineBytes int) string {
	t.Helper()

	envelope := makeSizedPermissionNotificationEnvelope(t, lineBytes-len("data:"))
	line := "data:" + envelope
	require.Len(t, line, lineBytes)
	return line
}

func makeSizedPermissionNotificationEnvelope(t *testing.T, size int) string {
	t.Helper()

	padding := size - sizedPermissionNotificationEnvelopeBaseBytes(t)
	require.GreaterOrEqual(t, padding, 0)

	return encodeWorkspaceEnvelope(t, pubsub.PayloadTypePermissionNotification, pubsub.Event[proto.PermissionNotification]{
		Type: pubsub.CreatedEvent,
		Payload: proto.PermissionNotification{
			ToolCallID: strings.Repeat("a", padding),
		},
	})
}

func sizedPermissionNotificationEnvelopeBaseBytes(t *testing.T) int {
	t.Helper()

	return len(encodeWorkspaceEnvelope(t, pubsub.PayloadTypePermissionNotification, pubsub.Event[proto.PermissionNotification]{
		Type:    pubsub.CreatedEvent,
		Payload: proto.PermissionNotification{},
	}))
}

func requireNoWorkspaceEvent(t *testing.T, events <-chan any) {
	t.Helper()

	select {
	case ev := <-events:
		t.Fatalf("unexpected workspace event: %#v", ev)
	default:
	}
}

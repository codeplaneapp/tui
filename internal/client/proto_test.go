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

func writeWorkspaceSSEEvent[T any](t *testing.T, w http.ResponseWriter, payloadType pubsub.PayloadType, payload pubsub.Event[T]) {
	t.Helper()

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	envelope, err := json.Marshal(pubsub.Payload{
		Type:    payloadType,
		Payload: raw,
	})
	require.NoError(t, err)

	_, err = fmt.Fprintf(w, "data: %s\n\n", envelope)
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

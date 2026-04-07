package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// makeSSELine builds a single SSE data line containing a Message event whose
// TextContent body is exactly textLen bytes.  The full wire line includes the
// "data:" prefix, the JSON envelope, and a trailing newline.
func makeSSELine(t *testing.T, textLen int) []byte {
	t.Helper()

	text := strings.Repeat("x", textLen)

	msg := proto.Message{
		ID:        "msg-1",
		Role:      proto.Assistant,
		SessionID: "sess-1",
		Model:     "test-model",
		CreatedAt: 1,
		UpdatedAt: 1,
	}

	// Build the inner event payload with a serialised Message.
	innerPayload, err := json.Marshal(pubsub.Event[proto.Message]{
		Type:    pubsub.CreatedEvent,
		Payload: msg,
	})
	if err != nil {
		t.Fatalf("marshal inner payload: %v", err)
	}

	// We need to embed the text directly into the JSON since Message's
	// MarshalJSON uses a custom parts encoder.  Build the envelope by hand
	// so the text field is exactly the right size without going through
	// ContentPart marshaling.
	envelope := fmt.Sprintf(
		`{"type":"message","payload":{"type":"created","payload":{"id":"msg-1","role":"assistant","session_id":"sess-1","parts":[{"type":"text","text":%s}],"model":"test-model","provider":"","created_at":1,"updated_at":1}}}`,
		mustJSON(t, text),
	)

	// Verify the inner payload is valid JSON.
	_ = innerPayload

	var line bytes.Buffer
	line.WriteString("data: ")
	line.WriteString(envelope)
	line.WriteByte('\n')
	return line.Bytes()
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func TestReadSSEStream_LargePayloads(t *testing.T) {
	tests := []struct {
		name    string
		textLen int
	}{
		{"small_1KB", 1 << 10},
		{"medium_512KB", 512 << 10},
		{"near_old_limit_1MiB_minus_1KB", (1 << 20) - (1 << 10)},
		{"at_old_limit_1MiB", 1 << 20},
		{"above_old_limit_2MiB", 2 << 20},
		{"large_5MiB", 5 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := makeSSELine(t, tt.textLen)
			t.Logf("SSE line size: %d bytes (%.2f MiB)", len(line), float64(len(line))/(1<<20))

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			events := make(chan any, 10)

			// Feed the SSE line to readSSEStream via an io.Reader.
			go readSSEStream(ctx, bytes.NewReader(line), events)

			select {
			case ev := <-events:
				msg, ok := ev.(pubsub.Event[proto.Message])
				if !ok {
					t.Fatalf("expected pubsub.Event[proto.Message], got %T", ev)
				}
				if msg.Payload.ID != "msg-1" {
					t.Errorf("expected message ID msg-1, got %s", msg.Payload.ID)
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for event from readSSEStream")
			}
		})
	}
}

func TestReadSSEStream_MultipleEvents(t *testing.T) {
	// Build a stream with three events of different sizes including one
	// above the old 1 MiB scanner limit.
	sizes := []int{100, 1 << 20, 2 << 20}

	var buf bytes.Buffer
	for _, sz := range sizes {
		buf.Write(makeSSELine(t, sz))
		// SSE streams use blank lines to separate events; add one for realism.
		buf.WriteByte('\n')
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events := make(chan any, 10)
	go readSSEStream(ctx, &buf, events)

	for i, sz := range sizes {
		select {
		case ev := <-events:
			_, ok := ev.(pubsub.Event[proto.Message])
			if !ok {
				t.Fatalf("event %d (textLen=%d): expected pubsub.Event[proto.Message], got %T", i, sz, ev)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for event %d (textLen=%d)", i, sz)
		}
	}
}

func TestSSEBufferSize(t *testing.T) {
	// Verify the buffer constant is at least 10 MiB.
	const minExpected = 10 << 20
	if sseBufferSize < minExpected {
		t.Errorf("sseBufferSize = %d, want >= %d (10 MiB)", sseBufferSize, minExpected)
	}
}

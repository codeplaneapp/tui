package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
)

const (
	workspaceEventsInitialRetryDelay = 250 * time.Millisecond
	workspaceEventsDefaultRetryHint  = time.Second
	workspaceEventsMaximumRetryDelay = 8 * time.Second
)

var errWorkspaceEventStreamClosed = errors.New("workspace event stream closed")

type workspaceEventsStatusError struct {
	statusCode int
}

func (e workspaceEventsStatusError) Error() string {
	return fmt.Sprintf("workspace events returned status code %d", e.statusCode)
}

func parseSSERetryHint(line string) (time.Duration, bool) {
	const prefix = "retry:"

	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) {
		return 0, false
	}

	raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if raw == "" {
		return 0, false
	}

	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 0, false
	}

	return normalizeWorkspaceRetryDelay(time.Duration(ms) * time.Millisecond), true
}

func normalizeWorkspaceRetryDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return workspaceEventsDefaultRetryHint
	}
	if delay < workspaceEventsInitialRetryDelay {
		return workspaceEventsInitialRetryDelay
	}
	if delay > workspaceEventsMaximumRetryDelay {
		return workspaceEventsMaximumRetryDelay
	}
	return delay
}

func nextWorkspaceRetryDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return workspaceEventsInitialRetryDelay
	}

	next := delay * 2
	if next < workspaceEventsInitialRetryDelay {
		return workspaceEventsInitialRetryDelay
	}
	if next > workspaceEventsMaximumRetryDelay {
		return workspaceEventsMaximumRetryDelay
	}
	return next
}

func workspaceReconnectReason(err error) string {
	switch {
	case err == nil:
		return "unknown"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	case errors.Is(err, errWorkspaceEventStreamClosed), errors.Is(err, io.EOF):
		return "stream_closed"
	}

	var statusErr workspaceEventsStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.statusCode {
		case http.StatusUnauthorized:
			return "unauthorized"
		case http.StatusForbidden:
			return "forbidden"
		case http.StatusNotFound:
			return "not_found"
		default:
			return "status_error"
		}
	}

	return "transport_error"
}

func decodeWorkspaceEvent(data string) (any, string, error) {
	var payload pubsub.Payload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, "", fmt.Errorf("unmarshal event envelope: %w", err)
	}

	switch payload.Type {
	case pubsub.PayloadTypeLSPEvent:
		return decodeWorkspaceEventPayload[proto.LSPEvent](payload, string(payload.Type))
	case pubsub.PayloadTypeMCPEvent:
		return decodeWorkspaceEventPayload[proto.MCPEvent](payload, string(payload.Type))
	case pubsub.PayloadTypePermissionRequest:
		return decodeWorkspaceEventPayload[proto.PermissionRequest](payload, string(payload.Type))
	case pubsub.PayloadTypePermissionNotification:
		return decodeWorkspaceEventPayload[proto.PermissionNotification](payload, string(payload.Type))
	case pubsub.PayloadTypeMessage:
		return decodeWorkspaceEventPayload[proto.Message](payload, string(payload.Type))
	case pubsub.PayloadTypeSession:
		return decodeWorkspaceEventPayload[proto.Session](payload, string(payload.Type))
	case pubsub.PayloadTypeFile:
		return decodeWorkspaceEventPayload[proto.File](payload, string(payload.Type))
	case pubsub.PayloadTypeAgentEvent:
		return decodeWorkspaceEventPayload[proto.AgentEvent](payload, string(payload.Type))
	default:
		return nil, "unknown", fmt.Errorf("unknown event type %q", payload.Type)
	}
}

func decodeWorkspaceEventPayload[T any](payload pubsub.Payload, eventType string) (any, string, error) {
	var event pubsub.Event[T]
	if err := json.Unmarshal(payload.Payload, &event); err != nil {
		return nil, eventType, fmt.Errorf("unmarshal %s payload: %w", eventType, err)
	}
	return event, eventType, nil
}

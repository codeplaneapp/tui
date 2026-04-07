package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	workspaceEventsClientStream       = "workspace_events_client"
	workspaceEventsInitialRetryDelay  = 250 * time.Millisecond
	workspaceEventsMaximumRetryDelay  = 5 * time.Second
	workspaceEventsDefaultRetryHint   = time.Second
	workspaceEventsScannerBufferBytes = 1 * 1024 * 1024
)

var errWorkspaceEventStreamClosed = errors.New("workspace event stream closed")

type workspaceEventsStatusError struct {
	statusCode int
}

func (e workspaceEventsStatusError) Error() string {
	return fmt.Sprintf("failed to subscribe to events: status code %d", e.statusCode)
}

func (e workspaceEventsStatusError) reason() string {
	switch e.statusCode {
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

// ListWorkspaces retrieves all workspaces from the server.
func (c *Client) ListWorkspaces(ctx context.Context) ([]proto.Workspace, error) {
	rsp, err := c.get(ctx, "/workspaces", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list workspaces: status code %d", rsp.StatusCode)
	}
	var workspaces []proto.Workspace
	if err := json.NewDecoder(rsp.Body).Decode(&workspaces); err != nil {
		return nil, fmt.Errorf("failed to decode workspaces: %w", err)
	}
	return workspaces, nil
}

// CreateWorkspace creates a new workspace on the server.
func (c *Client) CreateWorkspace(ctx context.Context, ws proto.Workspace) (*proto.Workspace, error) {
	rsp, err := c.post(ctx, "/workspaces", nil, jsonBody(ws), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create workspace: status code %d", rsp.StatusCode)
	}
	var created proto.Workspace
	if err := json.NewDecoder(rsp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("failed to decode workspace: %w", err)
	}
	return &created, nil
}

// GetWorkspace retrieves a workspace from the server.
func (c *Client) GetWorkspace(ctx context.Context, id string) (*proto.Workspace, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get workspace: status code %d", rsp.StatusCode)
	}
	var ws proto.Workspace
	if err := json.NewDecoder(rsp.Body).Decode(&ws); err != nil {
		return nil, fmt.Errorf("failed to decode workspace: %w", err)
	}
	return &ws, nil
}

// DeleteWorkspace deletes a workspace on the server.
func (c *Client) DeleteWorkspace(ctx context.Context, id string) error {
	rsp, err := c.delete(ctx, fmt.Sprintf("/workspaces/%s", id), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete workspace: status code %d", rsp.StatusCode)
	}
	return nil
}

// SubscribeEvents subscribes to server-sent events for a workspace.
func (c *Client) SubscribeEvents(ctx context.Context, id string) (<-chan any, error) {
	ctx = observability.WithWorkspaceID(ctx, id)
	ctx = observability.WithComponent(ctx, workspaceEventsClientStream)
	ctx, span := observability.StartSpan(ctx, "client.workspace_events.subscribe",
		attribute.String("crush.workspace_id", id),
	)

	rsp, retryDelay, err := c.openWorkspaceEventsStream(ctx, id)
	if err != nil {
		observability.RecordError(span, err)
		span.End()
		return nil, err
	}

	events := make(chan any, 100)
	go c.consumeWorkspaceEvents(ctx, span, id, events, rsp, retryDelay)
	return events, nil
}

func (c *Client) consumeWorkspaceEvents(ctx context.Context, span trace.Span, id string, events chan any, rsp *http.Response, retryDelay time.Duration) {
	defer close(events)
	defer span.End()

	result := "ok"
	reconnects := 0
	defer func() {
		span.SetAttributes(
			attribute.String("workspace.events.result", result),
			attribute.Int("workspace.events.reconnects", reconnects),
		)
	}()

	delay := normalizeWorkspaceRetryDelay(retryDelay)

	for {
		nextRetry, err := c.readWorkspaceEventsStream(ctx, id, rsp, events)
		if err == nil {
			return
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result = "canceled"
			return
		}

		reason := workspaceReconnectReason(err)
		var statusErr workspaceEventsStatusError
		if errors.As(err, &statusErr) {
			result = reason
			observability.RecordError(span, err)
			return
		}

		result = "reconnecting"
		if nextRetry > 0 {
			delay = normalizeWorkspaceRetryDelay(nextRetry)
		}
		reconnects++
		observability.RecordRetry(workspaceEventsClientStream, "http", reason, delay)
		observability.LogAttrs(ctx, slog.LevelWarn, "Workspace event stream disconnected",
			slog.String("workspace_id", id),
			slog.String("reason", reason),
			slog.Duration("retry_in", delay),
		)

		if !waitForWorkspaceReconnect(ctx, delay) {
			result = "canceled"
			return
		}

		for {
			var openErr error
			rsp, retryDelay, openErr = c.openWorkspaceEventsStream(ctx, id)
			if openErr == nil {
				delay = normalizeWorkspaceRetryDelay(retryDelay)
				break
			}

			reason = workspaceReconnectReason(openErr)
			if errors.As(openErr, &statusErr) {
				result = reason
				observability.RecordError(span, openErr)
				return
			}

			observability.RecordRetry(workspaceEventsClientStream, "http", reason, delay)
			observability.LogAttrs(ctx, slog.LevelWarn, "Workspace event stream reconnect failed",
				slog.String("workspace_id", id),
				slog.String("reason", reason),
				slog.Duration("retry_in", delay),
			)
			if !waitForWorkspaceReconnect(ctx, delay) {
				result = "canceled"
				return
			}
			delay = nextWorkspaceRetryDelay(delay)
		}
	}
}

func (c *Client) openWorkspaceEventsStream(ctx context.Context, id string) (*http.Response, time.Duration, error) {
	//nolint:bodyclose
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/events", id), nil, http.Header{
		"Accept":        []string{"text/event-stream"},
		"Cache-Control": []string{"no-cache"},
		"Connection":    []string{"keep-alive"},
	})
	if err != nil {
		return nil, workspaceEventsDefaultRetryHint, fmt.Errorf("failed to subscribe to events: %w", err)
	}

	if rsp.StatusCode != http.StatusOK {
		_ = rsp.Body.Close()
		return nil, workspaceEventsDefaultRetryHint, workspaceEventsStatusError{statusCode: rsp.StatusCode}
	}

	return rsp, workspaceEventsDefaultRetryHint, nil
}

func (c *Client) readWorkspaceEventsStream(ctx context.Context, id string, rsp *http.Response, events chan<- any) (time.Duration, error) {
	streamCtx := observability.WithComponent(ctx, workspaceEventsClientStream)
	streamCtx, span := observability.StartSpan(streamCtx, "client.workspace_events.stream",
		attribute.String("crush.workspace_id", id),
	)
	started := time.Now()
	result := "ok"
	retryDelay := workspaceEventsDefaultRetryHint
	observability.RecordSSEConnection(workspaceEventsClientStream, 1)
	defer func() {
		_ = rsp.Body.Close()
		observability.RecordSSEConnection(workspaceEventsClientStream, -1)
		observability.RecordSSEStreamDuration(workspaceEventsClientStream, result, time.Since(started))
		span.SetAttributes(attribute.String("workspace.events.stream.result", result))
		span.End()
	}()

	scr := bufio.NewScanner(rsp.Body)
	scr.Buffer(make([]byte, 0, 64*1024), workspaceEventsScannerBufferBytes)

	var data strings.Builder
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}

		ev, eventType, err := decodeWorkspaceEvent(data.String())
		data.Reset()
		if err != nil {
			observability.RecordSSEEvent(workspaceEventsClientStream, "decode_error")
			observability.LogAttrs(streamCtx, slog.LevelWarn, "Failed to decode workspace event",
				slog.Any("error", err),
			)
			return nil
		}
		observability.RecordSSEEvent(workspaceEventsClientStream, eventType)
		return sendEvent(streamCtx, events, ev)
	}

	for scr.Scan() {
		if ctx.Err() != nil {
			result = "canceled"
			return retryDelay, ctx.Err()
		}

		line := scr.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				result = "canceled"
				return retryDelay, err
			}
		case strings.HasPrefix(line, ":"):
			observability.RecordSSEEvent(workspaceEventsClientStream, "heartbeat")
		case strings.HasPrefix(line, "retry:"):
			if parsed, ok := parseSSERetryHint(line); ok {
				retryDelay = parsed
				observability.RecordSSEEvent(workspaceEventsClientStream, "retry_hint")
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if ctx.Err() != nil {
		result = "canceled"
		return retryDelay, ctx.Err()
	}
	if err := dispatch(); err != nil {
		result = "canceled"
		return retryDelay, err
	}
	if err := scr.Err(); err != nil {
		result = "read_error"
		observability.RecordSSEEvent(workspaceEventsClientStream, "read_error")
		observability.RecordError(span, err)
		return retryDelay, err
	}

	result = "server_disconnect"
	observability.RecordSSEEvent(workspaceEventsClientStream, "server_disconnect")
	return retryDelay, errWorkspaceEventStreamClosed
}

func decodeWorkspaceEvent(data string) (any, string, error) {
	var p pubsub.Payload
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &p); err != nil {
		return nil, "", fmt.Errorf("unmarshal event envelope: %w", err)
	}

	switch p.Type {
	case pubsub.PayloadTypeLSPEvent:
		var e pubsub.Event[proto.LSPEvent]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypeMCPEvent:
		var e pubsub.Event[proto.MCPEvent]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypePermissionRequest:
		var e pubsub.Event[proto.PermissionRequest]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypePermissionNotification:
		var e pubsub.Event[proto.PermissionNotification]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypeMessage:
		var e pubsub.Event[proto.Message]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypeSession:
		var e pubsub.Event[proto.Session]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypeFile:
		var e pubsub.Event[proto.File]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	case pubsub.PayloadTypeAgentEvent:
		var e pubsub.Event[proto.AgentEvent]
		if err := json.Unmarshal(p.Payload, &e); err != nil {
			return nil, "", err
		}
		return e, string(p.Type), nil
	default:
		return nil, "unknown", fmt.Errorf("unknown event type %q", p.Type)
	}
}

func parseSSERetryHint(line string) (time.Duration, bool) {
	value := strings.TrimSpace(strings.TrimPrefix(line, "retry:"))
	if value == "" {
		return 0, false
	}

	millis, err := strconv.Atoi(value)
	if err != nil || millis <= 0 {
		return 0, false
	}
	return normalizeWorkspaceRetryDelay(time.Duration(millis) * time.Millisecond), true
}

func normalizeWorkspaceRetryDelay(delay time.Duration) time.Duration {
	switch {
	case delay <= 0:
		return workspaceEventsDefaultRetryHint
	case delay < workspaceEventsInitialRetryDelay:
		return workspaceEventsInitialRetryDelay
	case delay > workspaceEventsMaximumRetryDelay:
		return workspaceEventsMaximumRetryDelay
	default:
		return delay
	}
}

func nextWorkspaceRetryDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return workspaceEventsInitialRetryDelay
	}
	delay *= 2
	if delay > workspaceEventsMaximumRetryDelay {
		return workspaceEventsMaximumRetryDelay
	}
	return delay
}

func waitForWorkspaceReconnect(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func workspaceReconnectReason(err error) string {
	if err == nil {
		return "unknown"
	}

	var statusErr workspaceEventsStatusError
	if errors.As(err, &statusErr) {
		return statusErr.reason()
	}
	if errors.Is(err, errWorkspaceEventStreamClosed) || errors.Is(err, io.EOF) {
		return "stream_closed"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "transport_error"
}

func sendEvent(ctx context.Context, evc chan<- any, ev any) error {
	observability.LogAttrs(ctx, slog.LevelDebug, "Workspace event received",
		slog.String("event_type", fmt.Sprintf("%T", ev)),
	)
	select {
	case evc <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetLSPDiagnostics retrieves LSP diagnostics for a specific LSP client.
func (c *Client) GetLSPDiagnostics(ctx context.Context, id string, lspName string) (map[protocol.DocumentURI][]protocol.Diagnostic, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/lsps/%s/diagnostics", id, lspName), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get LSP diagnostics: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get LSP diagnostics: status code %d", rsp.StatusCode)
	}
	var diagnostics map[protocol.DocumentURI][]protocol.Diagnostic
	if err := json.NewDecoder(rsp.Body).Decode(&diagnostics); err != nil {
		return nil, fmt.Errorf("failed to decode LSP diagnostics: %w", err)
	}
	return diagnostics, nil
}

// GetLSPs retrieves the LSP client states for a workspace.
func (c *Client) GetLSPs(ctx context.Context, id string) (map[string]proto.LSPClientInfo, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/lsps", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get LSPs: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get LSPs: status code %d", rsp.StatusCode)
	}
	var lsps map[string]proto.LSPClientInfo
	if err := json.NewDecoder(rsp.Body).Decode(&lsps); err != nil {
		return nil, fmt.Errorf("failed to decode LSPs: %w", err)
	}
	return lsps, nil
}

// MCPGetStates retrieves the MCP client states for a workspace.
func (c *Client) MCPGetStates(ctx context.Context, id string) (map[string]proto.MCPClientInfo, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/mcp/states", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP states: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get MCP states: status code %d", rsp.StatusCode)
	}
	var states map[string]proto.MCPClientInfo
	if err := json.NewDecoder(rsp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode MCP states: %w", err)
	}
	return states, nil
}

// MCPRefreshPrompts refreshes prompts for a named MCP client.
func (c *Client) MCPRefreshPrompts(ctx context.Context, id, name string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/mcp/refresh-prompts", id), nil,
		jsonBody(struct {
			Name string `json:"name"`
		}{Name: name}),
		http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to refresh MCP prompts: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh MCP prompts: status code %d", rsp.StatusCode)
	}
	return nil
}

// MCPRefreshResources refreshes resources for a named MCP client.
func (c *Client) MCPRefreshResources(ctx context.Context, id, name string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/mcp/refresh-resources", id), nil,
		jsonBody(struct {
			Name string `json:"name"`
		}{Name: name}),
		http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to refresh MCP resources: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh MCP resources: status code %d", rsp.StatusCode)
	}
	return nil
}

// GetAgentSessionQueuedPrompts retrieves the number of queued prompts for a
// session.
func (c *Client) GetAgentSessionQueuedPrompts(ctx context.Context, id string, sessionID string) (int, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s/prompts/queued", id, sessionID), nil, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get session agent queued prompts: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get session agent queued prompts: status code %d", rsp.StatusCode)
	}
	var count int
	if err := json.NewDecoder(rsp.Body).Decode(&count); err != nil {
		return 0, fmt.Errorf("failed to decode session agent queued prompts: %w", err)
	}
	return count, nil
}

// ClearAgentSessionQueuedPrompts clears the queued prompts for a session.
func (c *Client) ClearAgentSessionQueuedPrompts(ctx context.Context, id string, sessionID string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s/prompts/clear", id, sessionID), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to clear session agent queued prompts: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to clear session agent queued prompts: status code %d", rsp.StatusCode)
	}
	return nil
}

// GetAgentInfo retrieves the agent status for a workspace.
func (c *Client) GetAgentInfo(ctx context.Context, id string) (*proto.AgentInfo, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/agent", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent status: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get agent status: status code %d", rsp.StatusCode)
	}
	var info proto.AgentInfo
	if err := json.NewDecoder(rsp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode agent status: %w", err)
	}
	return &info, nil
}

// UpdateAgent triggers an agent model update on the server.
func (c *Client) UpdateAgent(ctx context.Context, id string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent/update", id), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update agent: status code %d", rsp.StatusCode)
	}
	return nil
}

// SendMessage sends a message to the agent for a workspace.
func (c *Client) SendMessage(ctx context.Context, id string, sessionID, prompt string, attachments ...message.Attachment) error {
	protoAttachments := make([]proto.Attachment, len(attachments))
	for i, a := range attachments {
		protoAttachments[i] = proto.Attachment{
			FilePath: a.FilePath,
			FileName: a.FileName,
			MimeType: a.MimeType,
			Content:  a.Content,
		}
	}
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent", id), nil, jsonBody(proto.AgentMessage{
		SessionID:   sessionID,
		Prompt:      prompt,
		Attachments: protoAttachments,
	}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to send message to agent: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send message to agent: status code %d", rsp.StatusCode)
	}
	return nil
}

// GetAgentSessionInfo retrieves the agent session info for a workspace.
func (c *Client) GetAgentSessionInfo(ctx context.Context, id string, sessionID string) (*proto.AgentSession, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get session agent info: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get session agent info: status code %d", rsp.StatusCode)
	}
	var info proto.AgentSession
	if err := json.NewDecoder(rsp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode session agent info: %w", err)
	}
	return &info, nil
}

// AgentSummarizeSession requests a session summarization.
func (c *Client) AgentSummarizeSession(ctx context.Context, id string, sessionID string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s/summarize", id, sessionID), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to summarize session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to summarize session: status code %d", rsp.StatusCode)
	}
	return nil
}

// InitiateAgentProcessing triggers agent initialization on the server.
func (c *Client) InitiateAgentProcessing(ctx context.Context, id string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent/init", id), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to initiate session agent processing: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to initiate session agent processing: status code %d", rsp.StatusCode)
	}
	return nil
}

// ListMessages retrieves all messages for a session as proto types.
func (c *Client) ListMessages(ctx context.Context, id string, sessionID string) ([]proto.Message, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/messages", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get messages: status code %d", rsp.StatusCode)
	}
	var msgs []proto.Message
	if err := json.NewDecoder(rsp.Body).Decode(&msgs); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to decode messages: %w", err)
	}
	return msgs, nil
}

// GetSession retrieves a specific session as a proto type.
func (c *Client) GetSession(ctx context.Context, id string, sessionID string) (*proto.Session, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get session: status code %d", rsp.StatusCode)
	}
	var sess proto.Session
	if err := json.NewDecoder(rsp.Body).Decode(&sess); err != nil {
		return nil, fmt.Errorf("failed to decode session: %w", err)
	}
	return &sess, nil
}

// ListSessionHistoryFiles retrieves history files for a session as proto types.
func (c *Client) ListSessionHistoryFiles(ctx context.Context, id string, sessionID string) ([]proto.File, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/history", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get session history files: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get session history files: status code %d", rsp.StatusCode)
	}
	var files []proto.File
	if err := json.NewDecoder(rsp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode session history files: %w", err)
	}
	return files, nil
}

// CreateSession creates a new session in a workspace as a proto type.
func (c *Client) CreateSession(ctx context.Context, id string, title string) (*proto.Session, error) {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/sessions", id), nil, jsonBody(proto.Session{Title: title}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create session: status code %d", rsp.StatusCode)
	}
	var sess proto.Session
	if err := json.NewDecoder(rsp.Body).Decode(&sess); err != nil {
		return nil, fmt.Errorf("failed to decode session: %w", err)
	}
	return &sess, nil
}

// ListSessions lists all sessions in a workspace as proto types.
func (c *Client) ListSessions(ctx context.Context, id string) ([]proto.Session, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get sessions: status code %d", rsp.StatusCode)
	}
	var sessions []proto.Session
	if err := json.NewDecoder(rsp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("failed to decode sessions: %w", err)
	}
	return sessions, nil
}

// GrantPermission grants a permission on a workspace.
func (c *Client) GrantPermission(ctx context.Context, id string, req proto.PermissionGrant) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/permissions/grant", id), nil, jsonBody(req), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to grant permission: status code %d", rsp.StatusCode)
	}
	return nil
}

// SetPermissionsSkipRequests sets the skip-requests flag for a workspace.
func (c *Client) SetPermissionsSkipRequests(ctx context.Context, id string, skip bool) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/permissions/skip", id), nil, jsonBody(proto.PermissionSkipRequest{Skip: skip}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to set permissions skip requests: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set permissions skip requests: status code %d", rsp.StatusCode)
	}
	return nil
}

// GetPermissionsSkipRequests retrieves the skip-requests flag for a workspace.
func (c *Client) GetPermissionsSkipRequests(ctx context.Context, id string) (bool, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/permissions/skip", id), nil, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get permissions skip requests: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get permissions skip requests: status code %d", rsp.StatusCode)
	}
	var skip proto.PermissionSkipRequest
	if err := json.NewDecoder(rsp.Body).Decode(&skip); err != nil {
		return false, fmt.Errorf("failed to decode permissions skip requests: %w", err)
	}
	return skip.Skip, nil
}

// GetConfig retrieves the workspace-specific configuration.
func (c *Client) GetConfig(ctx context.Context, id string) (*config.Config, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/config", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get config: status code %d", rsp.StatusCode)
	}
	var cfg config.Config
	if err := json.NewDecoder(rsp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}
	return &cfg, nil
}

func jsonBody(v any) *bytes.Buffer {
	b := new(bytes.Buffer)
	m, _ := json.Marshal(v)
	b.Write(m)
	return b
}

// SaveSession updates a session in a workspace, returning a proto type.
func (c *Client) SaveSession(ctx context.Context, id string, sess proto.Session) (*proto.Session, error) {
	rsp, err := c.put(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s", id, sess.ID), nil, jsonBody(sess), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to save session: status code %d", rsp.StatusCode)
	}
	var saved proto.Session
	if err := json.NewDecoder(rsp.Body).Decode(&saved); err != nil {
		return nil, fmt.Errorf("failed to decode session: %w", err)
	}
	return &saved, nil
}

// DeleteSession deletes a session from a workspace.
func (c *Client) DeleteSession(ctx context.Context, id string, sessionID string) error {
	rsp, err := c.delete(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s", id, sessionID), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete session: status code %d", rsp.StatusCode)
	}
	return nil
}

// ListUserMessages retrieves user-role messages for a session as proto types.
func (c *Client) ListUserMessages(ctx context.Context, id string, sessionID string) ([]proto.Message, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/messages/user", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user messages: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user messages: status code %d", rsp.StatusCode)
	}
	var msgs []proto.Message
	if err := json.NewDecoder(rsp.Body).Decode(&msgs); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to decode user messages: %w", err)
	}
	return msgs, nil
}

// ListAllUserMessages retrieves all user-role messages across sessions as proto types.
func (c *Client) ListAllUserMessages(ctx context.Context, id string) ([]proto.Message, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/messages/user", id), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all user messages: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get all user messages: status code %d", rsp.StatusCode)
	}
	var msgs []proto.Message
	if err := json.NewDecoder(rsp.Body).Decode(&msgs); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to decode all user messages: %w", err)
	}
	return msgs, nil
}

// CancelAgentSession cancels an ongoing agent operation for a session.
func (c *Client) CancelAgentSession(ctx context.Context, id string, sessionID string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s/cancel", id, sessionID), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel agent session: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to cancel agent session: status code %d", rsp.StatusCode)
	}
	return nil
}

// GetAgentSessionQueuedPromptsList retrieves the list of queued prompt
// strings for a session.
func (c *Client) GetAgentSessionQueuedPromptsList(ctx context.Context, id string, sessionID string) ([]string, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/agent/sessions/%s/prompts/list", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get queued prompts list: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get queued prompts list: status code %d", rsp.StatusCode)
	}
	var prompts []string
	if err := json.NewDecoder(rsp.Body).Decode(&prompts); err != nil {
		return nil, fmt.Errorf("failed to decode queued prompts list: %w", err)
	}
	return prompts, nil
}

// GetDefaultSmallModel retrieves the default small model for a provider.
func (c *Client) GetDefaultSmallModel(ctx context.Context, id string, providerID string) (*config.SelectedModel, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/agent/default-small-model", id), url.Values{"provider_id": []string{providerID}}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get default small model: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get default small model: status code %d", rsp.StatusCode)
	}
	var model config.SelectedModel
	if err := json.NewDecoder(rsp.Body).Decode(&model); err != nil {
		return nil, fmt.Errorf("failed to decode default small model: %w", err)
	}
	return &model, nil
}

// FileTrackerRecordRead records a file read for a session.
func (c *Client) FileTrackerRecordRead(ctx context.Context, id string, sessionID, path string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/filetracker/read", id), nil, jsonBody(struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
	}{SessionID: sessionID, Path: path}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to record file read: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to record file read: status code %d", rsp.StatusCode)
	}
	return nil
}

// FileTrackerLastReadTime returns the last read time for a file in a
// session.
func (c *Client) FileTrackerLastReadTime(ctx context.Context, id string, sessionID, path string) (time.Time, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/filetracker/lastread", id), url.Values{
		"session_id": []string{sessionID},
		"path":       []string{path},
	}, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last read time: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("failed to get last read time: status code %d", rsp.StatusCode)
	}
	var t time.Time
	if err := json.NewDecoder(rsp.Body).Decode(&t); err != nil {
		return time.Time{}, fmt.Errorf("failed to decode last read time: %w", err)
	}
	return t, nil
}

// FileTrackerListReadFiles returns the list of read files for a session.
func (c *Client) FileTrackerListReadFiles(ctx context.Context, id string, sessionID string) ([]string, error) {
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/filetracker/files", id, sessionID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get read files: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get read files: status code %d", rsp.StatusCode)
	}
	var files []string
	if err := json.NewDecoder(rsp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode read files: %w", err)
	}
	return files, nil
}

// LSPStart starts an LSP server for a path.
func (c *Client) LSPStart(ctx context.Context, id string, path string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/lsps/start", id), nil, jsonBody(struct {
		Path string `json:"path"`
	}{Path: path}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return fmt.Errorf("failed to start LSP: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to start LSP: status code %d", rsp.StatusCode)
	}
	return nil
}

// LSPStopAll stops all LSP servers for a workspace.
func (c *Client) LSPStopAll(ctx context.Context, id string) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/lsps/stop", id), nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to stop LSPs: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop LSPs: status code %d", rsp.StatusCode)
	}
	return nil
}

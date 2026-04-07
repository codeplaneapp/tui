package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReqSetsSchemeHostAndHeaders(t *testing.T) {
	c := &Client{
		path:    "/workspace",
		network: "tcp",
		addr:    "127.0.0.1:9090",
	}

	customHeaders := http.Header{
		"X-Custom": []string{"value1"},
	}

	req, err := c.buildReq(context.Background(), http.MethodPost, "/v1/health", nil, customHeaders)
	require.NoError(t, err)

	assert.Equal(t, "http", req.URL.Scheme)
	assert.Equal(t, "127.0.0.1:9090", req.URL.Host)
	assert.Equal(t, "/v1/health", req.URL.Path)
	assert.Equal(t, "value1", req.Header.Get("X-Custom"))
	// No body means no default Content-Type
	assert.Empty(t, req.Header.Get("Content-Type"))
}

func TestBuildReqSetsDummyHostForUnixNetwork(t *testing.T) {
	for _, network := range []string{"unix", "npipe"} {
		t.Run(network, func(t *testing.T) {
			c := &Client{
				path:    "/workspace",
				network: network,
				addr:    "/tmp/crush.sock",
			}

			req, err := c.buildReq(context.Background(), http.MethodGet, "/v1/version", nil, nil)
			require.NoError(t, err)

			assert.Equal(t, DummyHost, req.Host,
				"unix/npipe connections should set Host to DummyHost")
		})
	}
}

func TestBuildReqSetsDefaultContentTypeForBody(t *testing.T) {
	c := &Client{
		path:    "/workspace",
		network: "tcp",
		addr:    "localhost:8080",
	}

	body := jsonBody(map[string]string{"key": "value"})
	req, err := c.buildReq(context.Background(), http.MethodPost, "/v1/data", body, nil)
	require.NoError(t, err)

	assert.Equal(t, "text/plain", req.Header.Get("Content-Type"),
		"body present with no Content-Type should default to text/plain")
}

func TestBuildReqDoesNotOverrideExplicitContentType(t *testing.T) {
	c := &Client{
		path:    "/workspace",
		network: "tcp",
		addr:    "localhost:8080",
	}

	body := jsonBody(map[string]string{"key": "value"})
	headers := http.Header{"Content-Type": []string{"application/json"}}

	req, err := c.buildReq(context.Background(), http.MethodPost, "/v1/data", body, headers)
	require.NoError(t, err)

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"),
		"explicit Content-Type header should not be overridden")
}

func TestJsonBodySerializesAndIsReadable(t *testing.T) {
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	buf := jsonBody(payload{Name: "test", Count: 42})
	require.NotNil(t, buf)

	assert.JSONEq(t, `{"name":"test","count":42}`, buf.String())
}

func TestParseSSERetryHint(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   time.Duration
		wantOK bool
	}{
		{
			name:   "valid retry hint",
			line:   "retry: 1000",
			want:   time.Second,
			wantOK: true,
		},
		{
			name:   "small value clamped to minimum",
			line:   "retry: 10",
			want:   workspaceEventsInitialRetryDelay,
			wantOK: true,
		},
		{
			name:   "large value clamped to maximum",
			line:   "retry: 999999",
			want:   workspaceEventsMaximumRetryDelay,
			wantOK: true,
		},
		{
			name:   "empty value",
			line:   "retry:",
			want:   0,
			wantOK: false,
		},
		{
			name:   "non-numeric value",
			line:   "retry: abc",
			want:   0,
			wantOK: false,
		},
		{
			name:   "negative value",
			line:   "retry: -5",
			want:   0,
			wantOK: false,
		},
		{
			name:   "zero value",
			line:   "retry: 0",
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSSERetryHint(tt.line)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestNormalizeWorkspaceRetryDelay(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{"zero returns default", 0, workspaceEventsDefaultRetryHint},
		{"negative returns default", -time.Second, workspaceEventsDefaultRetryHint},
		{"below minimum clamped up", 50 * time.Millisecond, workspaceEventsInitialRetryDelay},
		{"above maximum clamped down", 30 * time.Second, workspaceEventsMaximumRetryDelay},
		{"in range passes through", 2 * time.Second, 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeWorkspaceRetryDelay(tt.input))
		})
	}
}

func TestNextWorkspaceRetryDelay(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{"zero starts at initial", 0, workspaceEventsInitialRetryDelay},
		{"negative starts at initial", -time.Second, workspaceEventsInitialRetryDelay},
		{"doubles delay", 500 * time.Millisecond, time.Second},
		{"caps at maximum", 4 * time.Second, workspaceEventsMaximumRetryDelay},
		{"already at max stays at max", workspaceEventsMaximumRetryDelay, workspaceEventsMaximumRetryDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextWorkspaceRetryDelay(tt.input))
		})
	}
}

func TestWorkspaceReconnectReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, "unknown"},
		{"status 401", workspaceEventsStatusError{statusCode: http.StatusUnauthorized}, "unauthorized"},
		{"status 403", workspaceEventsStatusError{statusCode: http.StatusForbidden}, "forbidden"},
		{"status 404", workspaceEventsStatusError{statusCode: http.StatusNotFound}, "not_found"},
		{"status 500", workspaceEventsStatusError{statusCode: http.StatusInternalServerError}, "status_error"},
		{"stream closed", errWorkspaceEventStreamClosed, "stream_closed"},
		{"EOF", io.EOF, "stream_closed"},
		{"context canceled", context.Canceled, "canceled"},
		{"deadline exceeded", context.DeadlineExceeded, "canceled"},
		{"generic error", errors.New("something broke"), "transport_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, workspaceReconnectReason(tt.err))
		})
	}
}

func TestWorkspaceEventsStatusErrorMessage(t *testing.T) {
	err := workspaceEventsStatusError{statusCode: 404}
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "status code")
}

func TestDecodeWorkspaceEvent(t *testing.T) {
	t.Run("malformed envelope", func(t *testing.T) {
		ev, eventType, err := decodeWorkspaceEvent(`{"type":"permission_notification","payload":`)
		require.Error(t, err)
		assert.Nil(t, ev)
		assert.Empty(t, eventType)
		assert.Contains(t, err.Error(), "unmarshal event envelope")
	})

	t.Run("unknown event type", func(t *testing.T) {
		envelope, err := json.Marshal(map[string]any{
			"type":    "mystery_event",
			"payload": map[string]any{"type": "created", "payload": map[string]any{"tool_call_id": "tool-1"}},
		})
		require.NoError(t, err)

		ev, eventType, err := decodeWorkspaceEvent(string(envelope))
		require.Error(t, err)
		assert.Nil(t, ev)
		assert.Equal(t, "unknown", eventType)
		assert.Contains(t, err.Error(), `unknown event type "mystery_event"`)
	})
}

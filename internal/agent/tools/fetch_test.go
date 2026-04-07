package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchMockPermissions is a permission.Service mock for fetch tests that
// supports configurable grant/deny behavior.
type fetchMockPermissions struct {
	*pubsub.Broker[permission.PermissionRequest]
	grantAll bool
}

func newFetchMockPermissions(grant bool) *fetchMockPermissions {
	return &fetchMockPermissions{
		Broker:   pubsub.NewBroker[permission.PermissionRequest](),
		grantAll: grant,
	}
}

func (m *fetchMockPermissions) GrantPersistent(_ permission.PermissionRequest) {}
func (m *fetchMockPermissions) Grant(_ permission.PermissionRequest)           {}
func (m *fetchMockPermissions) Deny(_ permission.PermissionRequest)            {}
func (m *fetchMockPermissions) AutoApproveSession(_ string)                    {}
func (m *fetchMockPermissions) SetSkipRequests(_ bool)                         {}
func (m *fetchMockPermissions) SkipRequests() bool                             { return false }
func (m *fetchMockPermissions) SubscribeNotifications(_ context.Context) <-chan pubsub.Event[permission.PermissionNotification] {
	return nil
}
func (m *fetchMockPermissions) Request(_ context.Context, _ permission.CreatePermissionRequest) (bool, error) {
	return m.grantAll, nil
}

func runFetchTool(t *testing.T, tool fantasy.AgentTool, params FetchParams) (fantasy.ToolResponse, error) {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	return tool.Run(ctx, fantasy.ToolCall{
		ID:    "test-call",
		Name:  FetchToolName,
		Input: string(input),
	})
}

func TestFetchSizeLimitBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		bodySize      int
		wantTruncated bool
	}{
		{
			name:          "body at MaxFetchSize minus one",
			bodySize:      MaxFetchSize - 1,
			wantTruncated: false,
		},
		{
			name:          "body at exactly MaxFetchSize",
			bodySize:      MaxFetchSize,
			wantTruncated: false,
		},
		{
			name:          "body at MaxFetchSize plus one is capped by LimitReader",
			bodySize:      MaxFetchSize + 1,
			wantTruncated: false, // io.LimitReader caps at MaxFetchSize, so content == MaxFetchSize, no truncation
		},
		{
			name:          "body well over MaxFetchSize is capped",
			bodySize:      MaxFetchSize + 1000,
			wantTruncated: false, // Still capped by LimitReader to exactly MaxFetchSize
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := strings.Repeat("A", tt.bodySize)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			perms := newFetchMockPermissions(true)
			tool := NewFetchTool(perms, t.TempDir(), srv.Client())

			resp, err := runFetchTool(t, tool, FetchParams{
				URL:    srv.URL,
				Format: "text",
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError, "expected non-error response")

			if tt.wantTruncated {
				assert.Contains(t, resp.Content, "[Content truncated to")
			} else {
				assert.NotContains(t, resp.Content, "[Content truncated to")
			}

			// Verify the content never exceeds MaxFetchSize (pre-truncation-notice)
			if !tt.wantTruncated {
				assert.LessOrEqual(t, len(resp.Content), MaxFetchSize)
			}
		})
	}
}

func TestFetchSizeLimitBoundary_HTMLExpands(t *testing.T) {
	t.Parallel()

	// When HTML is converted to markdown, the conversion may change the content length.
	// If the converted content exceeds MaxFetchSize, it should be truncated.
	// Create HTML that is under MaxFetchSize raw but whose markdown conversion stays under too.
	smallHTML := "<html><body><p>" + strings.Repeat("word ", 100) + "</p></body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(smallHTML))
	}))
	defer srv.Close()

	perms := newFetchMockPermissions(true)
	tool := NewFetchTool(perms, t.TempDir(), srv.Client())

	resp, err := runFetchTool(t, tool, FetchParams{
		URL:    srv.URL,
		Format: "markdown",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "```")
}

func TestFetchTimeoutValidation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		timeout int
	}{
		{
			name:    "valid timeout at max (120s)",
			timeout: 120,
		},
		{
			name:    "timeout exceeding max gets clamped to 120",
			timeout: 300,
		},
		{
			name:    "zero timeout uses default client timeout",
			timeout: 0,
		},
		{
			name:    "negative timeout uses default client timeout",
			timeout: -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := newFetchMockPermissions(true)
			tool := NewFetchTool(perms, t.TempDir(), srv.Client())

			resp, err := runFetchTool(t, tool, FetchParams{
				URL:     srv.URL,
				Format:  "text",
				Timeout: tt.timeout,
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError)
			assert.Contains(t, resp.Content, "hello")
		})
	}
}

func TestFetchInvalidFormat(t *testing.T) {
	t.Parallel()

	perms := newFetchMockPermissions(true)
	tool := NewFetchTool(perms, t.TempDir(), &http.Client{})

	tests := []struct {
		name   string
		format string
	}{
		{"xml format", "xml"},
		{"json format", "json"},
		{"empty format", ""},
		{"uppercase TEXT is accepted", "TEXT"}, // lowercased in handler
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := runFetchTool(t, tool, FetchParams{
				URL:    "http://example.com",
				Format: tt.format,
			})
			require.NoError(t, err)

			if strings.EqualFold(tt.format, "text") || strings.EqualFold(tt.format, "markdown") || strings.EqualFold(tt.format, "html") {
				// Valid formats should not produce a format error
				assert.False(t, resp.IsError || strings.Contains(resp.Content, "Format must be one of"),
					"expected valid format %q to be accepted", tt.format)
			} else {
				assert.True(t, resp.IsError)
				assert.Contains(t, resp.Content, "Format must be one of: text, markdown, html")
			}
		})
	}
}

func TestFetchInvalidScheme(t *testing.T) {
	t.Parallel()

	perms := newFetchMockPermissions(true)
	tool := NewFetchTool(perms, t.TempDir(), &http.Client{})

	tests := []struct {
		name string
		url  string
	}{
		{"ftp scheme", "ftp://example.com/file"},
		{"file scheme", "file:///etc/passwd"},
		{"no scheme", "example.com"},
		{"data URI", "data:text/html,<h1>Hi</h1>"},
		{"javascript scheme", "javascript:alert(1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := runFetchTool(t, tool, FetchParams{
				URL:    tt.url,
				Format: "text",
			})
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, "URL must start with http:// or https://")
		})
	}
}

func TestFetchHTMLNoBodyTag(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><head><title>Test</title></head></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer srv.Close()

	perms := newFetchMockPermissions(true)
	tool := NewFetchTool(perms, t.TempDir(), srv.Client())

	t.Run("html format with no body returns error", func(t *testing.T) {
		resp, err := runFetchTool(t, tool, FetchParams{
			URL:    srv.URL,
			Format: "html",
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "No body content found in HTML")
	})

	t.Run("text format with no body returns empty text", func(t *testing.T) {
		resp, err := runFetchTool(t, tool, FetchParams{
			URL:    srv.URL,
			Format: "text",
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})
}

func TestFetchEmptyURL(t *testing.T) {
	t.Parallel()

	perms := newFetchMockPermissions(true)
	tool := NewFetchTool(perms, t.TempDir(), &http.Client{})

	resp, err := runFetchTool(t, tool, FetchParams{
		URL:    "",
		Format: "text",
	})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "URL parameter is required")
}

func TestFetchPermissionDenied(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	perms := newFetchMockPermissions(false)
	tool := NewFetchTool(perms, t.TempDir(), srv.Client())

	_, err := runFetchTool(t, tool, FetchParams{
		URL:    srv.URL,
		Format: "text",
	})
	assert.ErrorIs(t, err, permission.ErrorPermissionDenied)
}

func TestFetchHTTPErrorCodes(t *testing.T) {
	t.Parallel()

	codes := []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusForbidden}

	for _, code := range codes {
		t.Run(fmt.Sprintf("status %d", code), func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			perms := newFetchMockPermissions(true)
			tool := NewFetchTool(perms, t.TempDir(), srv.Client())

			resp, err := runFetchTool(t, tool, FetchParams{
				URL:    srv.URL,
				Format: "text",
			})
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, fmt.Sprintf("status code: %d", code))
		})
	}
}

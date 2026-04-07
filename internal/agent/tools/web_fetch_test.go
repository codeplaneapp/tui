package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runWebFetchTool(t *testing.T, tool fantasy.AgentTool, params WebFetchParams) (fantasy.ToolResponse, error) {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	return tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "test-call",
		Name:  WebFetchToolName,
		Input: string(input),
	})
}

func TestWebFetchLargeContentThresholdBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		bodySize      int
		wantFileWrite bool
	}{
		{
			name:          "content at threshold minus one is inlined",
			bodySize:      LargeContentThreshold - 1,
			wantFileWrite: false,
		},
		{
			name:          "content at exactly threshold is inlined",
			bodySize:      LargeContentThreshold,
			wantFileWrite: false,
		},
		{
			name:          "content at threshold plus one is saved to file",
			bodySize:      LargeContentThreshold + 1,
			wantFileWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := strings.Repeat("x", tt.bodySize)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			workDir := t.TempDir()
			tool := NewWebFetchTool(workDir, srv.Client())

			resp, err := runWebFetchTool(t, tool, WebFetchParams{URL: srv.URL})
			require.NoError(t, err)
			assert.False(t, resp.IsError, "expected non-error response")

			if tt.wantFileWrite {
				assert.Contains(t, resp.Content, "Content saved to:")
				assert.Contains(t, resp.Content, "large page")

				// Verify the temp file was actually created and has the right content.
				files, err := filepath.Glob(filepath.Join(workDir, "page-*.md"))
				require.NoError(t, err)
				require.NotEmpty(t, files, "expected temp file to exist")

				data, err := os.ReadFile(files[0])
				require.NoError(t, err)
				assert.Equal(t, tt.bodySize, len(data))
			} else {
				assert.NotContains(t, resp.Content, "Content saved to:")
				assert.Contains(t, resp.Content, "Fetched content from")
			}
		})
	}
}

func TestWebFetchEmptyURL(t *testing.T) {
	t.Parallel()

	tool := NewWebFetchTool(t.TempDir(), &http.Client{})

	resp, err := runWebFetchTool(t, tool, WebFetchParams{URL: ""})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "url is required")
}

func TestWebFetchHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tool := NewWebFetchTool(t.TempDir(), srv.Client())

	resp, err := runWebFetchTool(t, tool, WebFetchParams{URL: srv.URL})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "Failed to fetch URL")
}

func TestWebFetchHTMLConversion(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><h1>Hello World</h1><p>This is a test page.</p></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(t.TempDir(), srv.Client())

	resp, err := runWebFetchTool(t, tool, WebFetchParams{URL: srv.URL})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	// HTML should be converted to markdown
	assert.Contains(t, resp.Content, "Hello World")
	assert.Contains(t, resp.Content, "This is a test page")
}

func TestWebFetchPlainText(t *testing.T) {
	t.Parallel()

	body := "Just some plain text content."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(t.TempDir(), srv.Client())

	resp, err := runWebFetchTool(t, tool, WebFetchParams{URL: srv.URL})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, body)
}

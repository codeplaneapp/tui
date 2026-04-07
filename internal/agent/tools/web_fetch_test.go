package tools

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebFetch_LargeContentThresholdBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int
		expectFile bool
	}{
		{
			name:       "large content threshold minus one",
			size:       LargeContentThreshold - 1,
			expectFile: false,
		},
		{
			name:       "large content threshold",
			size:       LargeContentThreshold,
			expectFile: false,
		},
		{
			name:       "large content threshold plus one",
			size:       LargeContentThreshold + 1,
			expectFile: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			content := strings.Repeat("b", tc.size)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(content))
			}))
			defer srv.Close()

			tool := NewWebFetchTool(tmpDir, srv.Client())
			resp := runJSONTool(t, tool, t.Context(), WebFetchToolName, WebFetchParams{
				URL: srv.URL,
			})
			require.False(t, resp.IsError, resp.Content)

			if !tc.expectFile {
				assert.Contains(t, resp.Content, "Fetched content from "+srv.URL+":")
				assert.Contains(t, resp.Content, content)
				assert.NotContains(t, resp.Content, "Content saved to:")

				matches, err := filepath.Glob(filepath.Join(tmpDir, "page-*.md"))
				require.NoError(t, err)
				assert.Empty(t, matches)
				return
			}

			assert.Contains(t, resp.Content, "Fetched content from "+srv.URL+" (large page)")
			assert.Contains(t, resp.Content, "Content saved to:")
			assert.Contains(t, resp.Content, "Use the view and grep tools to analyze this file.")

			var savedPath string
			for _, line := range strings.Split(resp.Content, "\n") {
				if strings.HasPrefix(line, "Content saved to: ") {
					savedPath = strings.TrimPrefix(line, "Content saved to: ")
					break
				}
			}

			require.NotEmpty(t, savedPath)
			assert.Equal(t, tmpDir, filepath.Dir(savedPath))

			data, err := os.ReadFile(savedPath)
			require.NoError(t, err)
			assert.Equal(t, content, string(data))
		})
	}
}

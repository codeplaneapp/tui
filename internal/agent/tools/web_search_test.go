package tools

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSearch_EmptyResults(t *testing.T) {
	lastSearchMu.Lock()
	lastSearchTime = time.Time{}
	lastSearchMu.Unlock()
	t.Cleanup(func() {
		lastSearchMu.Lock()
		lastSearchTime = time.Time{}
		lastSearchMu.Unlock()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/lite/", r.URL.Path)
		assert.Equal(t, "no-results", r.URL.Query().Get("q"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><p>No matching results</p></body></html>"))
	}))
	defer srv.Close()

	tool := NewWebSearchTool(newDuckDuckGoRewriteClient(t, srv))
	resp := runJSONTool(t, tool, t.Context(), WebSearchToolName, WebSearchParams{
		Query:      "no-results",
		MaxResults: 5,
	})

	require.False(t, resp.IsError, resp.Content)
	assert.Equal(t, "No results found. Try rephrasing your search.", resp.Content)
}

func newDuckDuckGoRewriteClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()

	target, err := url.Parse(srv.URL)
	require.NoError(t, err)

	client := srv.Client()
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		cloned.Host = target.Host
		return baseTransport.RoundTrip(cloned)
	})

	return client
}

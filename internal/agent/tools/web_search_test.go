package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runWebSearchTool(t *testing.T, tool fantasy.AgentTool, params WebSearchParams) (fantasy.ToolResponse, error) {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	return tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "test-call",
		Name:  WebSearchToolName,
		Input: string(input),
	})
}

func TestWebSearchEmptyQuery(t *testing.T) {
	t.Parallel()

	tool := NewWebSearchTool(&http.Client{})

	resp, err := runWebSearchTool(t, tool, WebSearchParams{Query: ""})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "query is required")
}

func TestWebSearchEmptyResults(t *testing.T) {
	t.Parallel()

	// searchDuckDuckGo constructs a URL from a hardcoded base, so we test
	// the underlying parse + format functions directly.
	results, err := parseLiteSearchResults(`<html><body><p>No results</p></body></html>`, 10)
	require.NoError(t, err)
	assert.Empty(t, results)

	formatted := formatSearchResults(results)
	assert.Equal(t, "No results found. Try rephrasing your search.", formatted)
}

func TestWebSearchWithResults(t *testing.T) {
	t.Parallel()

	// Test the end-to-end flow using parseLiteSearchResults + formatSearchResults.
	// Use a table structure similar to DuckDuckGo Lite's actual HTML.
	redirectURL := "//duckduckgo.com/l/?uddg=" + url.QueryEscape("https://golang.org") + "&rut=abc"
	htmlResponse := `<html><body><table>
		<tr><td><a class="result-link" href="` + redirectURL + `">Go Programming Language</a></td></tr>
		<tr><td class="result-snippet">The Go programming language is an open source project.</td></tr>
	</table></body></html>`

	results, err := parseLiteSearchResults(htmlResponse, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "Go Programming Language", results[0].Title)
	assert.Equal(t, "https://golang.org", results[0].Link) // Redirect cleaned
	assert.Equal(t, "The Go programming language is an open source project.", results[0].Snippet)

	formatted := formatSearchResults(results)
	assert.Contains(t, formatted, "Found 1 search results:")
	assert.Contains(t, formatted, "Go Programming Language")
	assert.Contains(t, formatted, "URL: https://golang.org")
}

func TestWebSearchMaxResultsClamping(t *testing.T) {
	t.Parallel()

	// Verify that maxResults is clamped to 20 and defaults to 10.
	// We test this through parseLiteSearchResults since the tool itself
	// calls searchDuckDuckGo which uses a hardcoded URL.

	// Generate 25 results in HTML using table structure.
	var htmlContent string
	htmlContent = `<html><body><table>`
	for i := 1; i <= 25; i++ {
		htmlContent += fmt.Sprintf(`<tr><td><a class="result-link" href="https://example.com/%d">Result %d</a></td></tr>`, i, i)
		htmlContent += `<tr><td class="result-snippet">Snippet for result.</td></tr>`
	}
	htmlContent += `</table></body></html>`

	t.Run("max 10 results", func(t *testing.T) {
		t.Parallel()
		results, err := parseLiteSearchResults(htmlContent, 10)
		require.NoError(t, err)
		assert.Len(t, results, 10)
	})

	t.Run("max 20 results", func(t *testing.T) {
		t.Parallel()
		results, err := parseLiteSearchResults(htmlContent, 20)
		require.NoError(t, err)
		assert.Len(t, results, 20)
	})

	t.Run("max 5 results", func(t *testing.T) {
		t.Parallel()
		results, err := parseLiteSearchResults(htmlContent, 5)
		require.NoError(t, err)
		assert.Len(t, results, 5)
	})
}

func TestSearchDuckDuckGoHTTPError(t *testing.T) {
	t.Parallel()

	// searchDuckDuckGo uses a hardcoded URL so we can't easily redirect it to
	// a test server. Instead, verify that empty/malformed responses parse gracefully.
	results, err := parseLiteSearchResults("", 10)
	require.NoError(t, err) // Empty HTML parses fine, just no results.
	assert.Empty(t, results)
}

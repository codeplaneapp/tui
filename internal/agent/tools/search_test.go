package tools

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanDuckDuckGoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{
			name:     "standard redirect URL",
			rawURL:   "//duckduckgo.com/l/?uddg=" + url.QueryEscape("https://example.com/page"),
			expected: "https://example.com/page",
		},
		{
			name:     "redirect URL with extra params",
			rawURL:   "//duckduckgo.com/l/?uddg=" + url.QueryEscape("https://example.com/page") + "&rut=abc123",
			expected: "https://example.com/page",
		},
		{
			name:     "redirect URL with encoded special chars",
			rawURL:   "//duckduckgo.com/l/?uddg=" + url.QueryEscape("https://example.com/search?q=hello world&lang=en"),
			expected: "https://example.com/search?q=hello world&lang=en",
		},
		{
			name:     "non-redirect URL passes through unchanged",
			rawURL:   "https://example.com/direct",
			expected: "https://example.com/direct",
		},
		{
			name:     "empty string passes through",
			rawURL:   "",
			expected: "",
		},
		{
			name:     "duckduckgo URL without uddg param",
			rawURL:   "//duckduckgo.com/l/?foo=bar",
			expected: "//duckduckgo.com/l/?foo=bar",
		},
		{
			name:     "redirect with empty uddg value",
			rawURL:   "//duckduckgo.com/l/?uddg=",
			expected: "",
		},
		{
			name:     "redirect with uddg followed by ampersand immediately",
			rawURL:   "//duckduckgo.com/l/?uddg=&rut=abc",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cleanDuckDuckGoURL(tt.rawURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSearchResults(t *testing.T) {
	t.Parallel()

	t.Run("empty results", func(t *testing.T) {
		t.Parallel()
		result := formatSearchResults(nil)
		assert.Equal(t, "No results found. Try rephrasing your search.", result)
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		result := formatSearchResults([]SearchResult{})
		assert.Equal(t, "No results found. Try rephrasing your search.", result)
	})

	t.Run("single result", func(t *testing.T) {
		t.Parallel()
		results := []SearchResult{
			{
				Title:    "Example Title",
				Link:     "https://example.com",
				Snippet:  "This is a snippet.",
				Position: 1,
			},
		}
		result := formatSearchResults(results)
		assert.Contains(t, result, "Found 1 search results:")
		assert.Contains(t, result, "1. Example Title")
		assert.Contains(t, result, "URL: https://example.com")
		assert.Contains(t, result, "Summary: This is a snippet.")
	})

	t.Run("multiple results", func(t *testing.T) {
		t.Parallel()
		results := []SearchResult{
			{Title: "First", Link: "https://first.com", Snippet: "First snippet", Position: 1},
			{Title: "Second", Link: "https://second.com", Snippet: "Second snippet", Position: 2},
		}
		result := formatSearchResults(results)
		assert.Contains(t, result, "Found 2 search results:")
		assert.Contains(t, result, "1. First")
		assert.Contains(t, result, "2. Second")
	})
}

func TestParseLiteSearchResults(t *testing.T) {
	t.Parallel()

	t.Run("empty HTML returns no results", func(t *testing.T) {
		t.Parallel()
		results, err := parseLiteSearchResults("<html><body></body></html>", 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("HTML with result links", func(t *testing.T) {
		t.Parallel()
		// Use table structure so td elements are valid in the HTML parse tree.
		htmlContent := `<html><body><table>
			<tr><td><a class="result-link" href="https://example.com">Example</a></td></tr>
			<tr><td class="result-snippet">A snippet about example.</td></tr>
			<tr><td><a class="result-link" href="https://other.com">Other</a></td></tr>
			<tr><td class="result-snippet">Another snippet.</td></tr>
		</table></body></html>`

		results, err := parseLiteSearchResults(htmlContent, 10)
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, "Example", results[0].Title)
		assert.Equal(t, "https://example.com", results[0].Link)
		assert.Equal(t, "A snippet about example.", results[0].Snippet)
		assert.Equal(t, 1, results[0].Position)

		assert.Equal(t, "Other", results[1].Title)
		assert.Equal(t, "https://other.com", results[1].Link)
		assert.Equal(t, "Another snippet.", results[1].Snippet)
		assert.Equal(t, 2, results[1].Position)
	})

	t.Run("maxResults limits output", func(t *testing.T) {
		t.Parallel()
		htmlContent := `<html><body><table>
			<tr><td><a class="result-link" href="https://one.com">One</a></td></tr>
			<tr><td class="result-snippet">First.</td></tr>
			<tr><td><a class="result-link" href="https://two.com">Two</a></td></tr>
			<tr><td class="result-snippet">Second.</td></tr>
			<tr><td><a class="result-link" href="https://three.com">Three</a></td></tr>
			<tr><td class="result-snippet">Third.</td></tr>
		</table></body></html>`

		results, err := parseLiteSearchResults(htmlContent, 2)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "One", results[0].Title)
		assert.Equal(t, "Two", results[1].Title)
	})

	t.Run("DuckDuckGo redirect URLs are cleaned", func(t *testing.T) {
		t.Parallel()
		redirectURL := "//duckduckgo.com/l/?uddg=" + url.QueryEscape("https://real-site.com/page") + "&rut=hash123"
		htmlContent := `<html><body><table>
			<tr><td><a class="result-link" href="` + redirectURL + `">Real Site</a></td></tr>
			<tr><td class="result-snippet">A real site snippet.</td></tr>
		</table></body></html>`

		results, err := parseLiteSearchResults(htmlContent, 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "https://real-site.com/page", results[0].Link)
	})

	t.Run("result link without snippet", func(t *testing.T) {
		t.Parallel()
		htmlContent := `<html><body><table>
			<tr><td><a class="result-link" href="https://no-snippet.com">No Snippet</a></td></tr>
			<tr><td><a class="result-link" href="https://has-snippet.com">Has Snippet</a></td></tr>
			<tr><td class="result-snippet">The snippet.</td></tr>
		</table></body></html>`

		results, err := parseLiteSearchResults(htmlContent, 10)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "No Snippet", results[0].Title)
		assert.Equal(t, "", results[0].Snippet) // No snippet for first result
		assert.Equal(t, "Has Snippet", results[1].Title)
		assert.Equal(t, "The snippet.", results[1].Snippet)
	})
}

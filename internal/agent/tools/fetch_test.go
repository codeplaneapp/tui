package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ConvertHTMLToMarkdown
// ---------------------------------------------------------------------------

func TestConvertHTMLToMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		html     string
		contains []string
	}{
		{
			name:     "h1 tag",
			html:     "<h1>Hello World</h1>",
			contains: []string{"# Hello World"},
		},
		{
			name:     "paragraph tag",
			html:     "<p>Some paragraph text.</p>",
			contains: []string{"Some paragraph text."},
		},
		{
			name:     "anchor tag",
			html:     `<a href="https://example.com">click here</a>`,
			contains: []string{"[click here](https://example.com)"},
		},
		{
			name:     "unordered list",
			html:     "<ul><li>alpha</li><li>beta</li></ul>",
			contains: []string{"alpha", "beta"},
		},
		{
			name: "mixed tags",
			html: `<h1>Title</h1><p>Intro paragraph.</p><ul><li>one</li><li>two</li></ul>`,
			contains: []string{
				"# Title",
				"Intro paragraph.",
				"one",
				"two",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ConvertHTMLToMarkdown(tc.html)
			require.NoError(t, err)
			for _, substr := range tc.contains {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestConvertHTMLToMarkdown_EmptyInput(t *testing.T) {
	t.Parallel()

	result, err := ConvertHTMLToMarkdown("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// removeNoisyElements
// ---------------------------------------------------------------------------

func TestRemoveNoisyElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		html       string
		wantGone   []string
		wantKept   []string
	}{
		{
			name:     "removes script tag",
			html:     `<html><body><script>alert("xss")</script><p>Keep me</p></body></html>`,
			wantGone: []string{"<script>", "alert"},
			wantKept: []string{"Keep me"},
		},
		{
			name:     "removes style tag",
			html:     `<html><body><style>body{color:red}</style><p>Visible</p></body></html>`,
			wantGone: []string{"<style>", "color:red"},
			wantKept: []string{"Visible"},
		},
		{
			name:     "removes nav tag",
			html:     `<html><body><nav><a href="/">Home</a></nav><p>Main content</p></body></html>`,
			wantGone: []string{"<nav>"},
			wantKept: []string{"Main content"},
		},
		{
			name:     "removes footer tag",
			html:     `<html><body><p>Body text</p><footer>Copyright 2025</footer></body></html>`,
			wantGone: []string{"<footer>", "Copyright"},
			wantKept: []string{"Body text"},
		},
		{
			name:     "removes header tag",
			html:     `<html><body><header>Site header</header><p>Article</p></body></html>`,
			wantGone: []string{"<header>", "Site header"},
			wantKept: []string{"Article"},
		},
		{
			name: "removes multiple noisy tags at once",
			html: `<html><body>
				<script>evil()</script>
				<style>.x{}</style>
				<nav>nav</nav>
				<aside>sidebar</aside>
				<noscript>no js</noscript>
				<iframe src="x"></iframe>
				<svg><circle/></svg>
				<p>Real content</p>
			</body></html>`,
			wantGone: []string{"<script>", "<style>", "<nav>", "<aside>", "<noscript>", "<iframe", "<svg>"},
			wantKept: []string{"Real content"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := removeNoisyElements(tc.html)
			for _, gone := range tc.wantGone {
				assert.NotContains(t, result, gone)
			}
			for _, kept := range tc.wantKept {
				assert.Contains(t, result, kept)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cleanupMarkdown
// ---------------------------------------------------------------------------

func TestCleanupMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "collapses 3+ newlines to 2",
			in:   "line1\n\n\n\nline2",
			want: "line1\n\nline2",
		},
		{
			name: "collapses many blank lines",
			in:   "a\n\n\n\n\n\n\nb",
			want: "a\n\nb",
		},
		{
			name: "trims trailing whitespace on lines",
			in:   "hello   \nworld\t\t",
			want: "hello\nworld",
		},
		{
			name: "trims leading and trailing whitespace",
			in:   "  \n\n  content  \n\n  ",
			want: "content",
		},
		{
			name: "already clean content unchanged",
			in:   "# Title\n\nParagraph text.",
			want: "# Title\n\nParagraph text.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := cleanupMarkdown(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// FormatJSON
// ---------------------------------------------------------------------------

func TestFormatJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple object",
			in:   `{"name":"alice","age":30}`,
			want: "{\n  \"age\": 30,\n  \"name\": \"alice\"\n}\n",
		},
		{
			name: "array",
			in:   `[1,2,3]`,
			want: "[\n  1,\n  2,\n  3\n]\n",
		},
		{
			name: "nested object",
			in:   `{"a":{"b":"c"}}`,
			want: "{\n  \"a\": {\n    \"b\": \"c\"\n  }\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := FormatJSON(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFormatJSON_InvalidJSON(t *testing.T) {
	t.Parallel()

	badInputs := []string{
		"not json at all",
		"{missing: quotes}",
		`{"unclosed": `,
		"",
	}

	for _, bad := range badInputs {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			_, err := FormatJSON(bad)
			assert.Error(t, err, "FormatJSON should return error for invalid JSON: %q", bad)
		})
	}
}

// ---------------------------------------------------------------------------
// FetchURLAndConvert — HTTP integration tests using httptest
// ---------------------------------------------------------------------------

func TestFetchURLAndConvert_NonUTF8(t *testing.T) {
	t.Parallel()

	// Create a test server that returns invalid UTF-8 bytes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// 0xfe 0xff are not valid in UTF-8.
		_, _ = w.Write([]byte{0xfe, 0xff, 0x80, 0x81})
	}))
	defer srv.Close()

	_, err := FetchURLAndConvert(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid UTF-8")
}

func TestFetchURLAndConvert_StatusError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchURLAndConvert(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchURLAndConvert_HTMLContent(t *testing.T) {
	t.Parallel()

	htmlBody := `<html><body><script>evil()</script><h1>Hello</h1><p>World</p></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	result, err := FetchURLAndConvert(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)

	// The result should be converted to markdown and cleaned up.
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "World")
	// Script content should have been stripped by removeNoisyElements.
	assert.NotContains(t, result, "evil()")
}

func TestFetchURLAndConvert_JSONContent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"value"}`))
	}))
	defer srv.Close()

	result, err := FetchURLAndConvert(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)

	// Should be pretty-printed.
	assert.Contains(t, result, "\"key\": \"value\"")
}

func TestFetchURLAndConvert_PlainText(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("just some plain text"))
	}))
	defer srv.Close()

	result, err := FetchURLAndConvert(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "just some plain text", result)
}

package chat

import (
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── highlightableMessageItem ────────────────────────────────────────────────

func TestIsHighlighted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startLine int
		endLine   int
		want      bool
	}{
		{"default state is not highlighted", -1, -1, false},
		{"startLine set", 0, -1, true},
		{"endLine set", -1, 5, true},
		{"both set", 2, 10, true},
		{"both zero counts as highlighted", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &highlightableMessageItem{
				startLine: tt.startLine,
				startCol:  -1,
				endLine:   tt.endLine,
				endCol:    -1,
			}
			assert.Equal(t, tt.want, h.isHighlighted())
		})
	}
}

func TestSetHighlightAdjustsColumns(t *testing.T) {
	t.Parallel()

	h := &highlightableMessageItem{
		startLine: -1,
		startCol:  -1,
		endLine:   -1,
		endCol:    -1,
	}

	// SetHighlight should subtract MessageLeftPaddingTotal from columns,
	// clamping to zero.
	h.SetHighlight(1, 5, 3, 10)

	startLine, startCol, endLine, endCol := h.Highlight()
	assert.Equal(t, 1, startLine)
	assert.Equal(t, 5-MessageLeftPaddingTotal, startCol)
	assert.Equal(t, 3, endLine)
	assert.Equal(t, 10-MessageLeftPaddingTotal, endCol)

	// Verify isHighlighted returns true after SetHighlight.
	assert.True(t, h.isHighlighted())
}

func TestSetHighlightClampsColumnsToZero(t *testing.T) {
	t.Parallel()

	h := &highlightableMessageItem{
		startLine: -1,
		startCol:  -1,
		endLine:   -1,
		endCol:    -1,
	}

	// Columns smaller than the offset should clamp to 0.
	h.SetHighlight(0, 0, 2, 1)

	_, startCol, _, endCol := h.Highlight()
	assert.Equal(t, 0, startCol)
	assert.Equal(t, 0, endCol)
}

func TestSetHighlightNegativeEndCol(t *testing.T) {
	t.Parallel()

	h := &highlightableMessageItem{
		startLine: -1,
		startCol:  -1,
		endLine:   -1,
		endCol:    -1,
	}

	// When endCol is negative, it should be passed through as-is.
	h.SetHighlight(0, 5, 3, -1)

	_, _, _, endCol := h.Highlight()
	assert.Equal(t, -1, endCol)
}

// ─── cachedMessageItem ──────────────────────────────────────────────────────

func TestCachedRenderRoundTrip(t *testing.T) {
	t.Parallel()

	c := &cachedMessageItem{}

	// Initially no cache.
	_, _, ok := c.getCachedRender(80)
	assert.False(t, ok, "empty cache should return false")

	// Set cache.
	c.setCachedRender("hello world", 80, 1)

	rendered, height, ok := c.getCachedRender(80)
	require.True(t, ok)
	assert.Equal(t, "hello world", rendered)
	assert.Equal(t, 1, height)

	// Different width should miss cache.
	_, _, ok = c.getCachedRender(100)
	assert.False(t, ok, "different width should miss cache")

	// Clear cache.
	c.clearCache()
	_, _, ok = c.getCachedRender(80)
	assert.False(t, ok, "cleared cache should return false")
}

func TestCachedRenderEmptyStringNotCached(t *testing.T) {
	t.Parallel()

	c := &cachedMessageItem{}
	// setCachedRender with empty string: getCachedRender should NOT return a
	// hit because the check requires rendered != "".
	c.setCachedRender("", 80, 0)
	_, _, ok := c.getCachedRender(80)
	assert.False(t, ok, "empty rendered string is treated as cache miss")
}

// ─── cappedMessageWidth ─────────────────────────────────────────────────────

func TestCappedMessageWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		available int
		want      int
	}{
		{"wide terminal caps to maxTextWidth", 200, maxTextWidth},
		{"narrow terminal uses available minus padding", 50, 50 - MessageLeftPaddingTotal},
		{"exactly at max boundary", maxTextWidth + MessageLeftPaddingTotal, maxTextWidth},
		{"zero width results in negative capped at zero later", MessageLeftPaddingTotal, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cappedMessageWidth(tt.available)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ─── getDigits ──────────────────────────────────────────────────────────────

func TestGetDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero", 0, 1},
		{"single digit", 5, 1},
		{"two digits", 42, 2},
		{"three digits", 999, 3},
		{"four digits", 1000, 4},
		{"negative treated as positive", -123, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, getDigits(tt.input))
		})
	}
}

// ─── formatSize ─────────────────────────────────────────────────────────────

func TestFormatSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int
		want  string
	}{
		{"zero bytes", 0, "0 B"},
		{"small bytes", 512, "512 B"},
		{"one KB", 1024, "1.0 KB"},
		{"kilobytes", 2048, "2.0 KB"},
		{"one MB", 1024 * 1024, "1.0 MB"},
		{"megabytes", 3 * 1024 * 1024, "3.0 MB"},
		{"just under KB", 1023, "1023 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatSize(tt.bytes))
		})
	}
}

// ─── formatTimeout ──────────────────────────────────────────────────────────

func TestFormatTimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", formatTimeout(0))
	assert.Equal(t, "30s", formatTimeout(30))
	assert.Equal(t, "1s", formatTimeout(1))
}

// ─── formatNonZero ──────────────────────────────────────────────────────────

func TestFormatNonZero(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", formatNonZero(0))
	assert.Equal(t, "42", formatNonZero(42))
	assert.Equal(t, "-1", formatNonZero(-1))
}

// ─── looksLikeMarkdown ─────────────────────────────────────────────────────

func TestLooksLikeMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"header", "# Hello", true},
		{"subheader", "## Section", true},
		{"bold", "some **bold** text", true},
		{"code fence", "```go\nfunc main() {}\n```", true},
		{"unordered list", "- item one", true},
		{"ordered list", "1. first", true},
		{"blockquote", "> quoted text", true},
		{"horizontal rule dashes", "---", true},
		{"horizontal rule stars", "***", true},
		{"plain text", "just some plain text", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, looksLikeMarkdown(tt.content))
		})
	}
}

// ─── genericPrettyName ──────────────────────────────────────────────────────

func TestGenericPrettyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"snake_case", "my_tool_name", "My Tool Name"},
		{"kebab-case", "my-tool-name", "My Tool Name"},
		{"mixed", "my_tool-name", "My Tool Name"},
		{"single word", "bash", "Bash"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, genericPrettyName(tt.input))
		})
	}
}

// ─── getFileExtensionForFormat ──────────────────────────────────────────────

func TestGetFileExtensionForFormat(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "fetch.txt", getFileExtensionForFormat("text"))
	assert.Equal(t, "fetch.html", getFileExtensionForFormat("html"))
	assert.Equal(t, "fetch.md", getFileExtensionForFormat("markdown"))
	assert.Equal(t, "fetch.md", getFileExtensionForFormat(""))
	assert.Equal(t, "fetch.md", getFileExtensionForFormat("json"))
}

// ─── AssistantInfoID ────────────────────────────────────────────────────────

func TestAssistantInfoID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "msg-123:assistant-info", AssistantInfoID("msg-123"))
	assert.Equal(t, ":assistant-info", AssistantInfoID(""))
}

// ─── statusOrder ────────────────────────────────────────────────────────────

func TestStatusOrder(t *testing.T) {
	t.Parallel()

	// completed < in_progress < pending
	assert.Less(t, statusOrder(session.TodoStatusCompleted), statusOrder(session.TodoStatusInProgress))
	assert.Less(t, statusOrder(session.TodoStatusInProgress), statusOrder(session.TodoStatusPending))
	// unknown status should equal pending
	assert.Equal(t, statusOrder(session.TodoStatusPending), statusOrder(session.TodoStatus("unknown")))
}

// ─── joinToolParts ──────────────────────────────────────────────────────────

func TestJoinToolParts(t *testing.T) {
	t.Parallel()

	got := joinToolParts("HEADER", "BODY")
	assert.Equal(t, "HEADER\n\nBODY", got)
}

// ─── ToolRenderOpts helpers ─────────────────────────────────────────────────

func TestToolRenderOptsIsPending(t *testing.T) {
	t.Parallel()

	// Not finished, not canceled => pending.
	opts := &ToolRenderOpts{ToolCall: toolCall(false), Status: ToolStatusRunning}
	assert.True(t, opts.IsPending())

	// Finished => not pending.
	opts = &ToolRenderOpts{ToolCall: toolCall(true), Status: ToolStatusSuccess}
	assert.False(t, opts.IsPending())

	// Canceled => not pending.
	opts = &ToolRenderOpts{ToolCall: toolCall(false), Status: ToolStatusCanceled}
	assert.False(t, opts.IsPending())
}

func TestToolRenderOptsHasEmptyResult(t *testing.T) {
	t.Parallel()

	assert.True(t, (&ToolRenderOpts{}).HasEmptyResult(), "nil result => empty")
	assert.True(t, (&ToolRenderOpts{Result: makeToolResult("")}).HasEmptyResult(), "empty content => empty")
	assert.False(t, (&ToolRenderOpts{Result: makeToolResult("data")}).HasEmptyResult(), "non-empty content => not empty")
}

// ─── test helpers ───────────────────────────────────────────────────────────

func toolCall(finished bool) message.ToolCall {
	return message.ToolCall{Finished: finished}
}

func makeToolResult(content string) *message.ToolResult {
	return &message.ToolResult{Content: content}
}

package views

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// padRight
// ---------------------------------------------------------------------------

func TestPadRight(t *testing.T) {
	t.Run("pads shorter string to width", func(t *testing.T) {
		got := padRight("hi", 6)
		assert.Equal(t, "hi    ", got)
		assert.Len(t, got, 6)
	})

	t.Run("returns string unchanged when already at width", func(t *testing.T) {
		got := padRight("abcd", 4)
		assert.Equal(t, "abcd", got)
	})

	t.Run("returns string unchanged when over width", func(t *testing.T) {
		got := padRight("toolong", 3)
		assert.Equal(t, "toolong", got)
	})

	t.Run("empty string padded to width", func(t *testing.T) {
		got := padRight("", 5)
		assert.Equal(t, "     ", got)
	})
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	t.Run("returns string within limit unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", truncate("hello", 10))
	})

	t.Run("returns string at exact limit unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", truncate("hello", 5))
	})

	t.Run("truncates with ellipsis when over limit", func(t *testing.T) {
		got := truncate("hello world!", 8)
		assert.Equal(t, "hello...", got)
		assert.Len(t, []rune(got), 8)
	})

	t.Run("empty string stays empty", func(t *testing.T) {
		assert.Equal(t, "", truncate("", 10))
	})

	t.Run("zero maxLen defaults to 80", func(t *testing.T) {
		short := strings.Repeat("x", 80)
		assert.Equal(t, short, truncate(short, 0))

		long := strings.Repeat("x", 100)
		got := truncate(long, 0)
		assert.Len(t, []rune(got), 80)
		assert.True(t, strings.HasSuffix(got, "..."))
	})
}

// ---------------------------------------------------------------------------
// truncateStr
// ---------------------------------------------------------------------------

func TestTruncateStr(t *testing.T) {
	t.Run("returns string within limit unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", truncateStr("hello", 10))
	})

	t.Run("returns string at exact limit unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", truncateStr("hello", 5))
	})

	t.Run("truncates with single-char ellipsis when over limit", func(t *testing.T) {
		got := truncateStr("hello world!", 6)
		// Should be 5 chars + "…"
		assert.Equal(t, "hello…", got)
		assert.Equal(t, 6, len([]rune(got)))
	})

	t.Run("empty string stays empty", func(t *testing.T) {
		assert.Equal(t, "", truncateStr("", 10))
	})

	t.Run("zero maxLen returns original string", func(t *testing.T) {
		assert.Equal(t, "abc", truncateStr("abc", 0))
	})

	t.Run("negative maxLen returns original string", func(t *testing.T) {
		assert.Equal(t, "abc", truncateStr("abc", -1))
	})
}

// ---------------------------------------------------------------------------
// fmtRelativeAge
// ---------------------------------------------------------------------------

func TestFmtRelativeAge(t *testing.T) {
	t.Run("returns empty for zero", func(t *testing.T) {
		assert.Equal(t, "", fmtRelativeAge(0))
	})

	t.Run("returns empty for negative", func(t *testing.T) {
		assert.Equal(t, "", fmtRelativeAge(-1))
	})

	t.Run("seconds ago", func(t *testing.T) {
		ms := time.Now().Add(-30 * time.Second).UnixMilli()
		got := fmtRelativeAge(ms)
		assert.Contains(t, got, "s ago")
	})

	t.Run("minutes ago", func(t *testing.T) {
		ms := time.Now().Add(-5 * time.Minute).UnixMilli()
		got := fmtRelativeAge(ms)
		assert.Contains(t, got, "m ago")
	})

	t.Run("hours ago", func(t *testing.T) {
		ms := time.Now().Add(-3 * time.Hour).UnixMilli()
		got := fmtRelativeAge(ms)
		assert.Contains(t, got, "h ago")
	})

	t.Run("days ago", func(t *testing.T) {
		ms := time.Now().Add(-48 * time.Hour).UnixMilli()
		got := fmtRelativeAge(ms)
		assert.Contains(t, got, "d ago")
	})
}

// ---------------------------------------------------------------------------
// wrapText
// ---------------------------------------------------------------------------

func TestWrapText(t *testing.T) {
	t.Run("returns original for zero width", func(t *testing.T) {
		assert.Equal(t, "hello", wrapText("hello", 0))
	})

	t.Run("wraps long line with indent", func(t *testing.T) {
		// width=12 means each chunk is width-2 = 10 chars of original, then
		// "  " prefix is added. So for a 20-char string we get 2 wrapped lines.
		input := "abcdefghijklmnopqrst" // 20 chars
		got := wrapText(input, 12)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		assert.Equal(t, "  abcdefghij", lines[0])
		assert.Equal(t, "  klmnopqrst", lines[1])
	})

	t.Run("short line gets indent only", func(t *testing.T) {
		got := wrapText("hi", 80)
		assert.Equal(t, "  hi", got)
	})

	t.Run("preserves newlines", func(t *testing.T) {
		got := wrapText("a\nb", 80)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		assert.Equal(t, "  a", lines[0])
		assert.Equal(t, "  b", lines[1])
	})
}

// ---------------------------------------------------------------------------
// wrapLineToWidth
// ---------------------------------------------------------------------------

func TestWrapLineToWidth(t *testing.T) {
	t.Run("returns original for zero width", func(t *testing.T) {
		got := wrapLineToWidth("hello", 0)
		assert.Equal(t, []string{"hello"}, got)
	})

	t.Run("splits long line into chunks", func(t *testing.T) {
		got := wrapLineToWidth("abcdefghij", 4)
		assert.Equal(t, []string{"abcd", "efgh", "ij"}, got)
	})

	t.Run("short line stays as single element", func(t *testing.T) {
		got := wrapLineToWidth("hi", 10)
		assert.Equal(t, []string{"hi"}, got)
	})

	t.Run("handles embedded newlines", func(t *testing.T) {
		got := wrapLineToWidth("ab\ncde", 2)
		assert.Equal(t, []string{"ab", "cd", "e"}, got)
	})

	t.Run("exact width boundary", func(t *testing.T) {
		got := wrapLineToWidth("abcd", 4)
		assert.Equal(t, []string{"abcd"}, got)
	})
}

// ---------------------------------------------------------------------------
// formatPayload
// ---------------------------------------------------------------------------

func TestFormatPayload(t *testing.T) {
	t.Run("pretty-prints valid JSON", func(t *testing.T) {
		input := `{"key":"value","num":42}`
		got := formatPayload(input, 80)
		assert.Contains(t, got, `"key"`)
		assert.Contains(t, got, `"value"`)
		// The output should start with "  " (the indent prefix) and be
		// multi-line pretty-printed JSON.
		assert.True(t, strings.HasPrefix(got, "  "))
	})

	t.Run("wraps non-JSON text", func(t *testing.T) {
		input := "just some plain text"
		got := formatPayload(input, 80)
		// wrapText prefixes each line with "  "
		assert.Equal(t, "  just some plain text", got)
	})

	t.Run("wraps invalid JSON as text", func(t *testing.T) {
		input := "{not valid json"
		got := formatPayload(input, 80)
		assert.Contains(t, got, "not valid json")
	})
}

// ---------------------------------------------------------------------------
// jjhubJoinNonEmpty
// ---------------------------------------------------------------------------

func TestJjhubJoinNonEmpty(t *testing.T) {
	t.Run("joins non-empty parts", func(t *testing.T) {
		got := jjhubJoinNonEmpty(" | ", "alpha", "beta", "gamma")
		assert.Equal(t, "alpha | beta | gamma", got)
	})

	t.Run("filters empty strings", func(t *testing.T) {
		got := jjhubJoinNonEmpty(", ", "a", "", "b", "  ", "c")
		assert.Equal(t, "a, b, c", got)
	})

	t.Run("returns empty when all parts are empty", func(t *testing.T) {
		got := jjhubJoinNonEmpty(", ", "", "  ", "	")
		assert.Equal(t, "", got)
	})

	t.Run("returns single part when only one is non-empty", func(t *testing.T) {
		got := jjhubJoinNonEmpty(", ", "", "only", "")
		assert.Equal(t, "only", got)
	})

	t.Run("no parts returns empty", func(t *testing.T) {
		got := jjhubJoinNonEmpty(", ")
		assert.Equal(t, "", got)
	})
}

// ---------------------------------------------------------------------------
// jjhubFormatRelativeTime
// ---------------------------------------------------------------------------

func TestJjhubFormatRelativeTime(t *testing.T) {
	t.Run("empty string returns dash", func(t *testing.T) {
		assert.Equal(t, "-", jjhubFormatRelativeTime(""))
	})

	t.Run("invalid format returns raw string", func(t *testing.T) {
		assert.Equal(t, "not-a-date", jjhubFormatRelativeTime("not-a-date"))
	})

	t.Run("just now for recent timestamp", func(t *testing.T) {
		ts := time.Now().Add(-10 * time.Second).Format(time.RFC3339)
		assert.Equal(t, "just now", jjhubFormatRelativeTime(ts))
	})

	t.Run("minutes ago", func(t *testing.T) {
		ts := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "m ago")
	})

	t.Run("hours ago", func(t *testing.T) {
		ts := time.Now().Add(-3 * time.Hour).Format(time.RFC3339)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "h ago")
	})

	t.Run("days ago", func(t *testing.T) {
		ts := time.Now().Add(-3 * 24 * time.Hour).Format(time.RFC3339)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "d ago")
	})

	t.Run("months ago", func(t *testing.T) {
		ts := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "mo ago")
	})

	t.Run("years ago", func(t *testing.T) {
		ts := time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "y ago")
	})

	t.Run("parses RFC3339Nano", func(t *testing.T) {
		ts := time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)
		got := jjhubFormatRelativeTime(ts)
		assert.Contains(t, got, "h ago")
	})
}

// ---------------------------------------------------------------------------
// jjhubFormatTimestamp
// ---------------------------------------------------------------------------

func TestJjhubFormatTimestamp(t *testing.T) {
	t.Run("empty string returns dash", func(t *testing.T) {
		assert.Equal(t, "-", jjhubFormatTimestamp(""))
	})

	t.Run("invalid format returns raw string", func(t *testing.T) {
		assert.Equal(t, "garbage", jjhubFormatTimestamp("garbage"))
	})

	t.Run("formats RFC3339 to local time", func(t *testing.T) {
		ts := "2026-03-15T14:30:00Z"
		got := jjhubFormatTimestamp(ts)
		// The result depends on the local timezone, but the format is fixed.
		parsed, err := time.Parse(time.RFC3339, ts)
		require.NoError(t, err)
		expected := parsed.Local().Format("2006-01-02 15:04")
		assert.Equal(t, expected, got)
	})

	t.Run("formats RFC3339Nano to local time", func(t *testing.T) {
		ts := "2026-03-15T14:30:00.123456789Z"
		got := jjhubFormatTimestamp(ts)
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		require.NoError(t, err)
		expected := parsed.Local().Format("2006-01-02 15:04")
		assert.Equal(t, expected, got)
	})
}

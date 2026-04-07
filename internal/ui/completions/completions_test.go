package completions

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterPrefersExactBasenameStem(t *testing.T) {
	t.Parallel()

	c := New(lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle())
	c.SetItems([]FileCompletionValue{
		{Path: "internal/ui/chat/search.go"},
		{Path: "internal/ui/chat/user.go"},
	}, nil)

	c.Filter("user")

	filtered := c.filtered
	require.NotEmpty(t, filtered)
	first, ok := filtered[0].(*CompletionItem)
	require.True(t, ok)
	require.Equal(t, "internal/ui/chat/user.go", first.Text())
	require.NotEmpty(t, first.match.MatchedIndexes)
}

func TestFilterPrefersBasenamePrefix(t *testing.T) {
	t.Parallel()

	c := New(lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle())
	c.SetItems([]FileCompletionValue{
		{Path: "internal/ui/chat/mcp.go"},
		{Path: "internal/ui/model/chat.go"},
	}, nil)

	c.Filter("chat.g")

	filtered := c.filtered
	require.NotEmpty(t, filtered)
	first, ok := filtered[0].(*CompletionItem)
	require.True(t, ok)
	require.Equal(t, "internal/ui/model/chat.go", first.Text())
	require.NotEmpty(t, first.match.MatchedIndexes)
}

func TestNamePriorityTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		query    string
		wantTier int
	}{
		{
			name:     "exact stem",
			path:     "internal/ui/chat/user.go",
			query:    "user",
			wantTier: tierExactName,
		},
		{
			name:     "basename prefix",
			path:     "internal/ui/model/chat.go",
			query:    "chat.g",
			wantTier: tierPrefixName,
		},
		{
			name:     "path segment exact",
			path:     "internal/ui/chat/mcp.go",
			query:    "chat",
			wantTier: tierPathSegment,
		},
		{
			name:     "fallback",
			path:     "internal/ui/chat/search.go",
			query:    "user",
			wantTier: tierFallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := namePriorityTier(tt.path, tt.query)
			require.Equal(t, tt.wantTier, got)
		})
	}
}

func TestFilterPrefersPathSegmentExact(t *testing.T) {
	t.Parallel()

	c := New(lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle())
	c.SetItems([]FileCompletionValue{
		{Path: "internal/ui/model/xychat.go"},
		{Path: "internal/ui/chat/mcp.go"},
	}, nil)

	c.Filter("chat")

	filtered := c.filtered
	require.NotEmpty(t, filtered)
	first, ok := filtered[0].(*CompletionItem)
	require.True(t, ok)
	require.Equal(t, "internal/ui/chat/mcp.go", first.Text())
}

func TestMatchedRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []int
		want [][2]int
	}{
		{
			name: "empty input",
			in:   []int{},
			want: [][2]int{},
		},
		{
			name: "single index",
			in:   []int{3},
			want: [][2]int{{3, 3}},
		},
		{
			name: "contiguous range",
			in:   []int{1, 2, 3, 4},
			want: [][2]int{{1, 4}},
		},
		{
			name: "two disjoint ranges",
			in:   []int{0, 1, 5, 6, 7},
			want: [][2]int{{0, 1}, {5, 7}},
		},
		{
			name: "all disjoint",
			in:   []int{2, 5, 9},
			want: [][2]int{{2, 2}, {5, 5}, {9, 9}},
		},
		{
			name: "mixed single and contiguous",
			in:   []int{0, 3, 4, 5, 10},
			want: [][2]int{{0, 0}, {3, 5}, {10, 10}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchedRanges(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBytePosToVisibleCharPos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		str       string
		rng       [2]int
		wantStart int
		wantStop  int
	}{
		{
			name:      "ascii single char",
			str:       "hello",
			rng:       [2]int{0, 0},
			wantStart: 0,
			wantStop:  0,
		},
		{
			name:      "ascii range",
			str:       "hello",
			rng:       [2]int{1, 3},
			wantStart: 1,
			wantStop:  3,
		},
		{
			name:      "multibyte utf8 before range",
			str:       "café",
			rng:       [2]int{0, 0},
			wantStart: 0,
			wantStop:  0,
		},
		{
			name:      "ascii at end of string",
			str:       "abcde",
			rng:       [2]int{4, 4},
			wantStart: 4,
			wantStop:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStart, gotStop := bytePosToVisibleCharPos(tt.str, tt.rng)
			assert.Equal(t, tt.wantStart, gotStart, "start")
			assert.Equal(t, tt.wantStop, gotStop, "stop")
		})
	}
}

func TestNewCompletionItem(t *testing.T) {
	t.Parallel()

	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	focused := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	match := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	val := FileCompletionValue{Path: "src/main.go"}
	item := NewCompletionItem("src/main.go", val, normal, focused, match)

	assert.Equal(t, "src/main.go", item.Text())
	assert.Equal(t, "src/main.go", item.Filter())
	assert.Equal(t, val, item.Value())

	// SetFocused toggles internal state and clears cache.
	item.cache = map[int]string{42: "cached"}
	item.SetFocused(true)
	assert.True(t, item.focused)
	assert.Nil(t, item.cache, "cache should be cleared when focused state changes")

	// Setting to same value should not clear cache.
	item.cache = map[int]string{42: "cached"}
	item.SetFocused(true)
	assert.NotNil(t, item.cache, "cache should be preserved when focused state is unchanged")

	// SetMatch clears cache.
	item.SetMatch(fuzzy.Match{Str: "src/main.go", MatchedIndexes: []int{0, 1}})
	assert.Nil(t, item.cache, "cache should be cleared on SetMatch")
	assert.Equal(t, []int{0, 1}, item.match.MatchedIndexes)
}

func TestHasPathSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pathLower string
		query     string
		want      bool
	}{
		{
			name:      "exact segment match",
			pathLower: "internal/ui/chat/mcp.go",
			query:     "ui",
			want:      true,
		},
		{
			name:      "no match substring only",
			pathLower: "internal/ui/chat/mcp.go",
			query:     "cha",
			want:      false,
		},
		{
			name:      "root segment",
			pathLower: "internal/ui/chat/mcp.go",
			query:     "internal",
			want:      true,
		},
		{
			name:      "file is a segment",
			pathLower: "internal/ui/chat/mcp.go",
			query:     "mcp.go",
			want:      true,
		},
		{
			name:      "backslash separator",
			pathLower: "internal\\ui\\chat\\mcp.go",
			query:     "chat",
			want:      true,
		},
		{
			name:      "empty query",
			pathLower: "internal/ui/chat/mcp.go",
			query:     "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasPathSegment(tt.pathLower, tt.query)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderItem_CachesResult(t *testing.T) {
	t.Parallel()

	normal := lipgloss.NewStyle()
	focused := lipgloss.NewStyle()
	matchSt := lipgloss.NewStyle()
	cache := make(map[int]string)
	m := &fuzzy.Match{}

	result1 := renderItem(normal, focused, matchSt, "test.go", false, 40, cache, m)
	require.NotEmpty(t, result1)

	// Second call with same width should return cached value.
	result2 := renderItem(normal, focused, matchSt, "test.go", false, 40, cache, m)
	assert.Equal(t, result1, result2)

	// Verify it was actually cached.
	cached, ok := cache[40]
	require.True(t, ok)
	assert.Equal(t, result1, cached)

	// Different width should produce different (uncached) result.
	result3 := renderItem(normal, focused, matchSt, "test.go", false, 60, cache, m)
	assert.NotEmpty(t, result3)
	_, ok = cache[60]
	assert.True(t, ok, "new width should be cached")
}

func TestKeyMapBindings(t *testing.T) {
	t.Parallel()

	km := DefaultKeyMap()

	bindings := km.KeyBindings()
	assert.Len(t, bindings, 4, "KeyBindings should return Down, Up, Select, Cancel")

	short := km.ShortHelp()
	assert.Len(t, short, 2, "ShortHelp should return Up and Down")

	full := km.FullHelp()
	require.NotEmpty(t, full)
	// 4 bindings fits in one group of 4.
	total := 0
	for _, group := range full {
		total += len(group)
	}
	assert.Equal(t, 4, total)
}

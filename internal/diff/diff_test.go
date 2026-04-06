package diff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDiff_Additions(t *testing.T) {
	before := ""
	after := "line1\nline2\nline3\n"

	diff, additions, removals := GenerateDiff(before, after, "test.txt")

	require.NotEmpty(t, diff)
	assert.Equal(t, 3, additions)
	assert.Equal(t, 0, removals)
}

func TestGenerateDiff_Removals(t *testing.T) {
	before := "line1\nline2\nline3\n"
	after := ""

	diff, additions, removals := GenerateDiff(before, after, "test.txt")

	require.NotEmpty(t, diff)
	assert.Equal(t, 0, additions)
	assert.Equal(t, 3, removals)
}

func TestGenerateDiff_Mixed(t *testing.T) {
	before := "line1\nline2\nline3\n"
	after := "line1\nmodified\nline3\nnewline\n"

	diff, additions, removals := GenerateDiff(before, after, "test.txt")

	require.NotEmpty(t, diff)
	assert.Equal(t, 2, additions) // "modified" and "newline"
	assert.Equal(t, 1, removals)  // "line2"
}

func TestGenerateDiff_NoChanges(t *testing.T) {
	content := "line1\nline2\nline3\n"

	diff, additions, removals := GenerateDiff(content, content, "test.txt")

	assert.Empty(t, diff)
	assert.Equal(t, 0, additions)
	assert.Equal(t, 0, removals)
}

func TestGenerateDiff_FileNameStripsLeadingSlash(t *testing.T) {
	before := "a\n"
	after := "b\n"

	diff, _, _ := GenerateDiff(before, after, "/path/to/file")

	assert.Contains(t, diff, "a/path/to/file")
	assert.Contains(t, diff, "b/path/to/file")
	assert.NotContains(t, diff, "a//path/to/file")
	assert.NotContains(t, diff, "b//path/to/file")
}

func TestGenerateDiff_CountsExcludeHeaderLines(t *testing.T) {
	before := "old\n"
	after := "new\n"

	diff, additions, removals := GenerateDiff(before, after, "test.txt")

	// The diff header contains "--- a/test.txt" and "+++ b/test.txt" lines.
	// These must NOT be counted as additions or removals.
	require.NotEmpty(t, diff)
	assert.True(t, strings.Contains(diff, "---"), "diff should contain --- header line")
	assert.True(t, strings.Contains(diff, "+++"), "diff should contain +++ header line")
	assert.Equal(t, 1, additions) // only "new"
	assert.Equal(t, 1, removals)  // only "old"
}

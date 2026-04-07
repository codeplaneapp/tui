package model

import (
	"testing"

	"github.com/charmbracelet/crush/internal/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLspFilePaths_Deduplicates(t *testing.T) {
	t.Parallel()

	msg := loadSessionMsg{
		files: []SessionFile{
			{LatestVersion: history.File{Path: "/a/foo.go"}},
			{LatestVersion: history.File{Path: "/a/bar.go"}},
		},
		readFiles: []string{"/a/foo.go", "/a/baz.go"},
	}

	paths := msg.lspFilePaths()
	require.Len(t, paths, 3, "duplicate /a/foo.go should be deduped")
	assert.Contains(t, paths, "/a/foo.go")
	assert.Contains(t, paths, "/a/bar.go")
	assert.Contains(t, paths, "/a/baz.go")
}

func TestLspFilePaths_Empty(t *testing.T) {
	t.Parallel()

	msg := loadSessionMsg{}
	paths := msg.lspFilePaths()
	assert.Empty(t, paths)
}

func TestLspFilePaths_OnlyModifiedFiles(t *testing.T) {
	t.Parallel()

	msg := loadSessionMsg{
		files: []SessionFile{
			{LatestVersion: history.File{Path: "/x/main.go"}},
			{LatestVersion: history.File{Path: "/x/util.go"}},
		},
	}

	paths := msg.lspFilePaths()
	require.Len(t, paths, 2)
	assert.Equal(t, "/x/main.go", paths[0])
	assert.Equal(t, "/x/util.go", paths[1])
}

func TestLspFilePaths_OnlyReadFiles(t *testing.T) {
	t.Parallel()

	msg := loadSessionMsg{
		readFiles: []string{"/y/a.ts", "/y/b.ts"},
	}

	paths := msg.lspFilePaths()
	require.Len(t, paths, 2)
	assert.Equal(t, "/y/a.ts", paths[0])
	assert.Equal(t, "/y/b.ts", paths[1])
}

func TestLspFilePaths_PreservesOrder(t *testing.T) {
	t.Parallel()

	// Modified files come first, then read files (minus duplicates).
	msg := loadSessionMsg{
		files: []SessionFile{
			{LatestVersion: history.File{Path: "/z/first.go"}},
		},
		readFiles: []string{"/z/second.go"},
	}

	paths := msg.lspFilePaths()
	require.Len(t, paths, 2)
	assert.Equal(t, "/z/first.go", paths[0], "modified files should come first")
	assert.Equal(t, "/z/second.go", paths[1], "read files should follow")
}

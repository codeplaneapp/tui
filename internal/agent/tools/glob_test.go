package tools

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeFilePaths(t *testing.T) {
	t.Parallel()

	t.Run("unix paths unchanged", func(t *testing.T) {
		t.Parallel()
		paths := []string{"/home/user/file.go", "/tmp/test.txt"}
		normalizeFilePaths(paths)
		assert.Equal(t, "/home/user/file.go", paths[0])
		assert.Equal(t, "/tmp/test.txt", paths[1])
	})

	t.Run("relative paths normalized with forward slashes", func(t *testing.T) {
		t.Parallel()
		// filepath.ToSlash converts OS separators to forward slashes.
		// On Unix this is already a forward slash, so the path stays the same.
		paths := []string{"src/main.go", "internal/pkg/util.go"}
		normalizeFilePaths(paths)
		assert.Equal(t, "src/main.go", paths[0])
		assert.Equal(t, "internal/pkg/util.go", paths[1])
	})

	t.Run("handles empty slice", func(t *testing.T) {
		t.Parallel()
		paths := []string{}
		normalizeFilePaths(paths)
		assert.Empty(t, paths)
	})

	t.Run("modifies slice in place", func(t *testing.T) {
		t.Parallel()
		original := []string{filepath.Join("a", "b", "c")}
		normalizeFilePaths(original)
		// After normalization, should use forward slashes
		assert.Equal(t, "a/b/c", original[0])
	})
}

func TestGlobResponseMetadata(t *testing.T) {
	t.Parallel()

	meta := GlobResponseMetadata{
		NumberOfFiles: 42,
		Truncated:     true,
	}
	assert.Equal(t, 42, meta.NumberOfFiles)
	assert.True(t, meta.Truncated)
}

func TestGlobParams(t *testing.T) {
	t.Parallel()

	t.Run("pattern is required field", func(t *testing.T) {
		t.Parallel()
		params := GlobParams{Pattern: "**/*.go"}
		assert.Equal(t, "**/*.go", params.Pattern)
		assert.Empty(t, params.Path)
	})

	t.Run("path is optional", func(t *testing.T) {
		t.Parallel()
		params := GlobParams{Pattern: "*.ts", Path: "/some/dir"}
		assert.Equal(t, "*.ts", params.Pattern)
		assert.Equal(t, "/some/dir", params.Path)
	})
}

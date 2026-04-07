package lsp

import (
	"os"
	"path/filepath"
	"testing"

	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveServerName(t *testing.T) {
	t.Parallel()
	manager := powernapconfig.NewManager()
	manager.AddServer("gopls", &powernapconfig.ServerConfig{
		Command: "gopls",
	})
	manager.AddServer("typescript-language-server", &powernapconfig.ServerConfig{
		Command: "typescript-language-server",
	})
	manager.AddServer("rust-analyzer", &powernapconfig.ServerConfig{
		Command: "rust-analyzer",
	})

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact name match returns that name",
			input:    "gopls",
			expected: "gopls",
		},
		{
			name:     "command-based lookup resolves to server name",
			input:    "rust-analyzer",
			expected: "rust-analyzer",
		},
		{
			name:     "unknown name returns itself",
			input:    "nonexistent-lsp",
			expected: "nonexistent-lsp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := resolveServerName(manager, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveServerName_CommandAlias(t *testing.T) {
	t.Parallel()
	// Register a server whose name differs from its command binary.
	manager := powernapconfig.NewManager()
	manager.AddServer("pyright", &powernapconfig.ServerConfig{
		Command: "pyright-langserver",
	})

	// Looking up by the command name should resolve to the server name.
	result := resolveServerName(manager, "pyright-langserver")
	assert.Equal(t, "pyright", result)
}

func TestHandlesFiletype(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fileTypes []string
		filePath  string
		expected  bool
	}{
		{
			name:      "empty fileTypes matches everything",
			fileTypes: []string{},
			filePath:  "main.go",
			expected:  true,
		},
		{
			name:      "matching extension with dot prefix",
			fileTypes: []string{".go"},
			filePath:  "main.go",
			expected:  true,
		},
		{
			name:      "matching extension without dot prefix",
			fileTypes: []string{"go"},
			filePath:  "main.go",
			expected:  true,
		},
		{
			name:      "non-matching extension",
			fileTypes: []string{"py"},
			filePath:  "main.go",
			expected:  false,
		},
		{
			name:      "case insensitive extension matching",
			fileTypes: []string{".GO"},
			filePath:  "main.go",
			expected:  true,
		},
		{
			name:      "multiple filetypes one matches",
			fileTypes: []string{"rs", "go", "py"},
			filePath:  "/path/to/file.py",
			expected:  true,
		},
		{
			name:      "multiple filetypes none match",
			fileTypes: []string{"rs", "go", "py"},
			filePath:  "/path/to/file.java",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handlesFiletype("test-server", tt.fileTypes, tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRootMarkers(t *testing.T) {
	t.Parallel()

	t.Run("empty markers matches everything", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasRootMarkers("/some/dir", nil))
		assert.True(t, hasRootMarkers("/some/dir", []string{}))
	})

	t.Run("marker file exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644))

		assert.True(t, hasRootMarkers(dir, []string{"go.mod"}))
	})

	t.Run("marker file does not exist", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		assert.False(t, hasRootMarkers(dir, []string{"go.mod"}))
	})

	t.Run("glob pattern matches", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0o644))

		assert.True(t, hasRootMarkers(dir, []string{"Cargo.*"}))
	})

	t.Run("at least one marker matches", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644))

		assert.True(t, hasRootMarkers(dir, []string{"go.mod", "package.json", "Cargo.toml"}))
	})
}

func TestHandles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644))

	t.Run("both filetype and root marker match", func(t *testing.T) {
		t.Parallel()
		server := &powernapconfig.ServerConfig{
			Command:     "gopls",
			FileTypes:   []string{"go"},
			RootMarkers: []string{"go.mod"},
		}
		assert.True(t, handles(server, filepath.Join(dir, "main.go"), dir))
	})

	t.Run("filetype matches but root marker missing", func(t *testing.T) {
		t.Parallel()
		server := &powernapconfig.ServerConfig{
			Command:     "gopls",
			FileTypes:   []string{"go"},
			RootMarkers: []string{"Cargo.toml"},
		}
		assert.False(t, handles(server, filepath.Join(dir, "main.go"), dir))
	})

	t.Run("root marker matches but filetype does not", func(t *testing.T) {
		t.Parallel()
		server := &powernapconfig.ServerConfig{
			Command:     "gopls",
			FileTypes:   []string{"rs"},
			RootMarkers: []string{"go.mod"},
		}
		assert.False(t, handles(server, filepath.Join(dir, "main.go"), dir))
	})

	t.Run("no constraints matches anything", func(t *testing.T) {
		t.Parallel()
		server := &powernapconfig.ServerConfig{
			Command: "generic-lsp",
		}
		assert.True(t, handles(server, filepath.Join(dir, "whatever.xyz"), dir))
	})
}

func TestSkipAutoStartCommands(t *testing.T) {
	t.Parallel()

	t.Run("known generic commands are in skip list", func(t *testing.T) {
		t.Parallel()
		for _, cmd := range []string{"node", "npx", "python", "python3", "java", "deno"} {
			assert.True(t, skipAutoStartCommands[cmd], "expected %q to be in skipAutoStartCommands", cmd)
		}
	})

	t.Run("specific LSP binaries are not in skip list", func(t *testing.T) {
		t.Parallel()
		for _, cmd := range []string{"gopls", "rust-analyzer", "clangd", "pyright"} {
			assert.False(t, skipAutoStartCommands[cmd], "expected %q to NOT be in skipAutoStartCommands", cmd)
		}
	})
}

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadFromSource_NonExistentDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")

	cmds, err := loadFromSource(commandSource{path: dir, prefix: userCommandPrefix})
	require.NoError(t, err)
	require.Empty(t, cmds)

	// directory must NOT have been created
	_, statErr := os.Stat(dir)
	require.True(t, os.IsNotExist(statErr))
}

func TestLoadFromSource_ExistingDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.md"), []byte("say hello"), 0o644))

	cmds, err := loadFromSource(commandSource{path: dir, prefix: userCommandPrefix})
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.Equal(t, "user:hello", cmds[0].ID)
	require.Equal(t, "say hello", cmds[0].Content)
}

func TestLoadAll_MixedSources(t *testing.T) {
	t.Parallel()

	existing := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(existing, "cmd.md"), []byte("content"), 0o644))

	missing := filepath.Join(t.TempDir(), "nope")

	cmds, err := loadAll([]commandSource{
		{path: existing, prefix: userCommandPrefix},
		{path: missing, prefix: projectCommandPrefix},
	})
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.Equal(t, "user:cmd", cmds[0].ID)
}

// TestBuildCommandSources asserts that all returned source paths reference
// smithers-tui and none reference crush.
func TestBuildCommandSources(t *testing.T) {
	cfg := &config.Config{
		Options: &config.Options{
			DataDirectory: "/tmp/test-project/.smithers-tui",
		},
	}

	sources := buildCommandSources(cfg)
	require.NotEmpty(t, sources)

	for _, src := range sources {
		lower := strings.ToLower(src.path)
		require.True(t,
			strings.Contains(lower, "smithers-tui"),
			"expected source path to contain smithers-tui, got: %s", src.path,
		)
		require.False(t,
			strings.Contains(lower, "/crush/") || strings.HasSuffix(lower, "/.crush"),
			"expected source path to not contain crush dir, got: %s", src.path,
		)
	}
}

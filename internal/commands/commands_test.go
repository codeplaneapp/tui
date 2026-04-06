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

// TestBuildCommandSources asserts that both smithers-tui and legacy crush
// command directories are searched.
func TestBuildCommandSources(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{
		Options: &config.Options{
			DataDirectory: "/tmp/test-project/.smithers-tui",
		},
	}

	sources := buildCommandSources(cfg)
	require.NotEmpty(t, sources)

	var paths []string
	for _, src := range sources {
		paths = append(paths, src.path)
	}

	require.Contains(t, paths, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "smithers-tui", "commands"))
	require.Contains(t, paths, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "crush", "commands"))
	require.True(t, strings.HasSuffix(paths[2], filepath.Join(".smithers-tui", "commands")))
	require.True(t, strings.HasSuffix(paths[3], filepath.Join(".crush", "commands")))
}

func TestLoadCustomCommands_FromDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))

	// Create two markdown command files.
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "greet.md"), []byte("Hello $USER_NAME!"), 0o644))

	subDir := filepath.Join(cmdDir, "admin")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "deploy.md"), []byte("deploy $ENV to production"), 0o644))

	// Also create a non-markdown file that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "notes.txt"), []byte("ignored"), 0o644))

	cmds, err := loadAll([]commandSource{
		{path: cmdDir, prefix: projectCommandPrefix},
	})
	require.NoError(t, err)
	require.Len(t, cmds, 2)

	// Build a map for easy lookup.
	byID := make(map[string]CustomCommand, len(cmds))
	for _, c := range cmds {
		byID[c.ID] = c
	}

	greet, ok := byID["project:greet"]
	require.True(t, ok, "expected project:greet command")
	require.Equal(t, "Hello $USER_NAME!", greet.Content)
	require.Len(t, greet.Arguments, 1)
	require.Equal(t, "USER_NAME", greet.Arguments[0].ID)

	deploy, ok := byID["project:admin:deploy"]
	require.True(t, ok, "expected project:admin:deploy command")
	require.Equal(t, "deploy $ENV to production", deploy.Content)
}

func TestCustomCommand_ArgumentExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "single argument",
			content:  "Run $TASK now",
			expected: []string{"TASK"},
		},
		{
			name:     "multiple distinct arguments",
			content:  "Deploy $SERVICE to $ENVIRONMENT on $REGION",
			expected: []string{"SERVICE", "ENVIRONMENT", "REGION"},
		},
		{
			name:     "duplicate arguments deduplicated",
			content:  "$NAME says hello to $NAME and $OTHER",
			expected: []string{"NAME", "OTHER"},
		},
		{
			name:     "no arguments",
			content:  "plain text without arguments",
			expected: nil,
		},
		{
			name:     "lowercase dollar not matched",
			content:  "This $lowercaseVar should not match",
			expected: nil,
		},
		{
			name:     "underscore and digits in name",
			content:  "Use $DB_HOST_2 for connection",
			expected: []string{"DB_HOST_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := extractArgNames(tt.content)

			if tt.expected == nil {
				require.Nil(t, args)
				return
			}

			require.Len(t, args, len(tt.expected))
			for i, expectedID := range tt.expected {
				require.Equal(t, expectedID, args[i].ID)
				require.Equal(t, expectedID, args[i].Title)
				require.True(t, args[i].Required, "all custom command args should be required")
			}
		})
	}
}

package prompt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptData_WithSmithersMode(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	tmpl := "{{if .SmithersMode}}smithers{{else}}default{{end}}|{{.SmithersWorkflowDir}}|{{.SmithersMCPServer}}"
	p, err := NewPrompt(
		"test-smithers",
		tmpl,
		WithWorkingDir(workingDir),
		WithTimeFunc(func() time.Time { return time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC) }),
		WithSmithersMode(filepath.ToSlash(filepath.Join(".smithers", "workflows")), "smithers"),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "smithers|.smithers/workflows|smithers", rendered)
}

func TestPromptData_WithoutSmithersMode(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	p, err := NewPrompt(
		"test-default",
		"{{if .SmithersMode}}smithers{{else}}default{{end}}|{{.SmithersWorkflowDir}}|{{.SmithersMCPServer}}",
		WithWorkingDir(workingDir),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "default||", rendered)
}

func TestPromptData_WithSmithersActiveRuns(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	activeRuns := []ActiveRunContext{
		{RunID: "run-1", WorkflowName: "ci", Status: "running"},
		{RunID: "run-2", WorkflowName: "deploy", Status: "waiting-approval"},
	}

	tmpl := "runs:{{len .SmithersActiveRuns}}|approvals:{{.SmithersPendingApprovals}}"
	p, err := NewPrompt(
		"test-active-runs",
		tmpl,
		WithWorkingDir(workingDir),
		WithSmithersMode(".smithers/workflows", "smithers"),
		WithSmithersActiveRuns(activeRuns, 1),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "runs:2|approvals:1", rendered)
}

func TestPromptData_WithSmithersActiveRuns_Empty(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	store, err := config.Init(workingDir, "", false)
	require.NoError(t, err)

	tmpl := "runs:{{len .SmithersActiveRuns}}|approvals:{{.SmithersPendingApprovals}}"
	p, err := NewPrompt(
		"test-no-runs",
		tmpl,
		WithWorkingDir(workingDir),
		WithSmithersMode(".smithers/workflows", "smithers"),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)
	require.Equal(t, "runs:0|approvals:0", rendered)
}

func TestIsGitRepo(t *testing.T) {
	t.Parallel()

	t.Run("returns true when .git directory exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		err := os.Mkdir(filepath.Join(dir, ".git"), 0o755)
		require.NoError(t, err)
		assert.True(t, isGitRepo(dir))
	})

	t.Run("returns false when .git is absent", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		assert.False(t, isGitRepo(dir))
	})

	t.Run("returns true when .git is a file (worktree)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// git worktrees use a .git file instead of a directory.
		err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /some/path"), 0o644)
		require.NoError(t, err)
		assert.True(t, isGitRepo(dir))
	})
}

func TestProcessFile(t *testing.T) {
	t.Parallel()

	t.Run("reads existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		filePath := filepath.Join(dir, "context.md")
		content := "# Project Context\nSome instructions."
		err := os.WriteFile(filePath, []byte(content), 0o644)
		require.NoError(t, err)

		result := processFile(filePath)
		require.NotNil(t, result)
		assert.Equal(t, filePath, result.Path)
		assert.Equal(t, content, result.Content)
	})

	t.Run("returns nil for non-existent file", func(t *testing.T) {
		t.Parallel()
		result := processFile("/no/such/file/ever.txt")
		assert.Nil(t, result)
	})

	t.Run("reads empty file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		filePath := filepath.Join(dir, "empty.txt")
		err := os.WriteFile(filePath, []byte{}, 0o644)
		require.NoError(t, err)

		result := processFile(filePath)
		require.NotNil(t, result)
		assert.Equal(t, "", result.Content)
	})
}

func TestProcessContextPath(t *testing.T) {
	t.Parallel()

	t.Run("loads single file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		store, err := config.Init(dir, "", false)
		require.NoError(t, err)

		filePath := filepath.Join(dir, "instructions.md")
		err = os.WriteFile(filePath, []byte("do this"), 0o644)
		require.NoError(t, err)

		contexts := processContextPath(filePath, store)
		require.Len(t, contexts, 1)
		assert.Equal(t, filePath, contexts[0].Path)
		assert.Equal(t, "do this", contexts[0].Content)
	})

	t.Run("loads directory recursively", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		store, err := config.Init(dir, "", false)
		require.NoError(t, err)

		ctxDir := filepath.Join(dir, "ctx")
		subDir := filepath.Join(ctxDir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(ctxDir, "a.txt"), []byte("aaa"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "b.txt"), []byte("bbb"), 0o644))

		contexts := processContextPath(ctxDir, store)
		require.Len(t, contexts, 2)

		// Collect content to check both files were loaded (order may vary due to walk).
		contents := map[string]string{}
		for _, c := range contexts {
			contents[filepath.Base(c.Path)] = c.Content
		}
		assert.Equal(t, "aaa", contents["a.txt"])
		assert.Equal(t, "bbb", contents["b.txt"])
	})

	t.Run("returns empty for non-existent path", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		store, err := config.Init(dir, "", false)
		require.NoError(t, err)

		contexts := processContextPath("/no/such/path", store)
		assert.Empty(t, contexts)
	})

	t.Run("resolves relative path against working dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		store, err := config.Init(dir, "", false)
		require.NoError(t, err)

		// Create a file relative to the working dir.
		relPath := "docs/notes.txt"
		absPath := filepath.Join(dir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
		require.NoError(t, os.WriteFile(absPath, []byte("notes content"), 0o644))

		contexts := processContextPath(relPath, store)
		require.Len(t, contexts, 1)
		assert.Equal(t, "notes content", contexts[0].Content)
	})
}

func TestExpandPath_PlainPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := config.Init(dir, "", false)
	require.NoError(t, err)

	// A plain absolute path should pass through unchanged.
	input := "/some/absolute/path"
	result := expandPath(input, store)
	assert.Equal(t, input, result)

	// A relative path without ~ or $ should also pass through.
	result = expandPath("relative/path", store)
	assert.Equal(t, "relative/path", result)
}

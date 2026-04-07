package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditTool_EmptyFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, newMockFiletrackerService(), tmpDir)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  EditToolName,
		Input: `{"file_path": "", "old_string": "a", "new_string": "b"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "file_path is required")
}

func TestEditTool_ReplaceContentSingleMatch(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world\nfoo bar\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "hello world", "new_string": "HELLO WORLD"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "HELLO WORLD\nfoo bar\n", string(got))
}

func TestEditTool_ReplaceContentNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "nonexistent", "new_string": "replacement"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "old_string not found")
}

func TestEditTool_ReplaceContentMultipleMatches(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("foo\nfoo\nbar\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "foo", "new_string": "baz"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "multiple times")
}

func TestEditTool_ReplaceAllMultipleMatches(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("foo\nfoo\nbar\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-5",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "foo", "new_string": "baz", "replace_all": true}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success with replace_all, got: %s", resp.Content)

	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "baz\nbaz\nbar\n", string(got))
}

func TestEditTool_DeleteContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("line 1\nline 2\nline 3\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	// When new_string is empty and old_string is set, it deletes the content
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-6",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "line 2\n", "new_string": ""}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "line 1\nline 3\n", string(got))
}

func TestEditTool_CreateNewFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	newFile := filepath.Join(tmpDir, "new_file.txt")

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	// When old_string is empty and new_string is set, it creates a new file
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-7",
		Name:  EditToolName,
		Input: `{"file_path": "` + newFile + `", "old_string": "", "new_string": "new content here"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(newFile)
	require.NoError(t, err)
	assert.Equal(t, "new content here", string(got))
}

func TestEditTool_CreateNewFileAlreadyExists(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("original"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-8",
		Name:  EditToolName,
		Input: `{"file_path": "` + existingFile + `", "old_string": "", "new_string": "overwrite attempt"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "file already exists")
}

func TestEditTool_FileNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "does_not_exist.txt")

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", missingFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-9",
		Name:  EditToolName,
		Input: `{"file_path": "` + missingFile + `", "old_string": "something", "new_string": "else"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "file not found")
}

func TestEditTool_MustReadBeforeEditing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))

	ft := newMockFiletrackerService()
	// Deliberately do NOT call ft.RecordRead
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-10",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "content", "new_string": "new content"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "must read the file before editing")
}

func TestEditTool_SameContentNoChange(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("unchanged"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-11",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "unchanged", "new_string": "unchanged"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "same as old content")
}

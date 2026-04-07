package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/permission"
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

func TestEditTool_ReplaceContentMissingHistoryRowDoesNotCreateDuplicateOldVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "hello world\nfoo bar\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr: errors.New("missing history row"),
	}
	tool := NewEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-12",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "hello world", "new_string": "HELLO WORLD"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{originalContent}, historyService.createCalls)
	assert.Equal(t, []string{"HELLO WORLD\nfoo bar\n"}, historyService.createVersionCalls)
	assert.Equal(t,
		[]string{
			"get",
			"create:" + originalContent,
			"createVersion:HELLO WORLD\nfoo bar\n",
		},
		historyService.callOrder,
	)
}

func TestEditTool_DeleteContentMissingHistoryRowDoesNotCreateDuplicateOldVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "line 1\nline 2\nline 3\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr: errors.New("missing history row"),
	}
	tool := NewEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-13",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "line 2\n", "new_string": ""}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{originalContent}, historyService.createCalls)
	assert.Equal(t, []string{"line 1\nline 3\n"}, historyService.createVersionCalls)
	assert.Equal(t,
		[]string{
			"get",
			"create:" + originalContent,
			"createVersion:line 1\nline 3\n",
		},
		historyService.callOrder,
	)
}

func TestEditTool_DeleteContentMissingHistoryRowCreateFailureReturnsError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("line 1\nline 2\nline 3\n"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr:    errors.New("missing history row"),
		createErr: errors.New("create failed"),
	}
	tool := NewEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-14",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "line 2\n", "new_string": ""}`,
	})

	require.ErrorContains(t, err, "error creating file history: create failed")
	assert.Equal(t, []string{"get", "create:line 1\nline 2\nline 3\n"}, historyService.callOrder)
	assert.Empty(t, historyService.createVersionCalls)
}

func TestEditTool_ReplaceContentCreateVersionFailureDoesNotFailEdit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "hello world\nfoo bar\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getFile: history.File{
			Path:    testFile,
			Content: "stale history content",
		},
		createVersionErrs: map[int]error{
			1: errors.New("create version failed"),
		},
	}
	tool := NewEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-15",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "hello world", "new_string": "HELLO WORLD"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{originalContent, "HELLO WORLD\nfoo bar\n"}, historyService.createVersionCalls)

	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, "HELLO WORLD\nfoo bar\n", string(got))
}

func TestEditTool_PermissionDeniedSkipsHistory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "hello world\nfoo bar\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	permissionService := &mockPermissionService{deny: true}
	historyService := &mockHistoryService{
		getErr:    errors.New("history should not be called"),
		createErr: errors.New("history should not be called"),
		createVersionErrs: map[int]error{
			1: errors.New("history should not be called"),
		},
	}
	tool := NewEditTool(nil, permissionService, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-16",
		Name:  EditToolName,
		Input: `{"file_path": "` + testFile + `", "old_string": "hello world", "new_string": "HELLO WORLD"}`,
	})

	require.ErrorIs(t, err, permission.ErrorPermissionDenied)
	assert.Len(t, permissionService.requests, 1)
	assert.Empty(t, historyService.callOrder)

	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, originalContent, string(got))
}

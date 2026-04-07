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

func TestWriteTool_EmptyFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, newMockFiletrackerService(), tmpDir)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  WriteToolName,
		Input: `{"file_path": "", "content": "some content"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "file_path is required")
}

func TestWriteTool_EmptyContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, ".gitkeep")

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": ""}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	info, err := os.Stat(testFile)
	require.NoError(t, err)
	assert.Zero(t, info.Size())
}

func TestWriteTool_TruncateFileToEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "truncate.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("existing content"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2b",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": ""}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestWriteTool_CreateNewFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	newFile := filepath.Join(tmpDir, "new_file.txt")

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  WriteToolName,
		Input: `{"file_path": "` + newFile + `", "content": "hello world"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Contains(t, resp.Content, "File successfully written")

	got, err := os.ReadFile(newFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(got))
}

func TestWriteTool_OverwriteExistingFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("old content"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "new content"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(got))
}

func TestWriteTool_MissingHistoryRowDoesNotCreateDuplicateOldVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("old content"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr: errors.New("missing history row"),
	}
	tool := NewWriteTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4b",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "new content"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{"old content"}, historyService.createCalls)
	assert.Equal(t, []string{"new content"}, historyService.createVersionCalls)
	assert.Equal(t,
		[]string{
			"get",
			"create:old content",
			"createVersion:new content",
		},
		historyService.callOrder,
	)
}

func TestWriteTool_MissingHistoryRowCreateFailureReturnsError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("old content"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr:    errors.New("missing history row"),
		createErr: errors.New("create failed"),
	}
	tool := NewWriteTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4c",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "new content"}`,
	})

	require.ErrorContains(t, err, "error creating file history: create failed")
	assert.Equal(t, []string{"get", "create:old content"}, historyService.callOrder)
	assert.Empty(t, historyService.createVersionCalls)
}

func TestWriteTool_CreateVersionFailureDoesNotFailWrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("old content"), 0o644))

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
	tool := NewWriteTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4d",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "new content"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{"old content", "new content"}, historyService.createVersionCalls)

	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, "new content", string(got))
}

func TestWriteTool_PermissionDeniedSkipsHistory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	originalContent := "old content"
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
	tool := NewWriteTool(nil, permissionService, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-4e",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "new content"}`,
	})

	require.ErrorIs(t, err, permission.ErrorPermissionDenied)
	assert.Len(t, permissionService.requests, 1)
	assert.Empty(t, historyService.callOrder)

	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, originalContent, string(got))
}

func TestWriteTool_SameContentNoChange(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "same.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("unchanged"), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-5",
		Name:  WriteToolName,
		Input: `{"file_path": "` + testFile + `", "content": "unchanged"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "already contains the exact content")
}

func TestWriteTool_CreatesParentDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	deepFile := filepath.Join(tmpDir, "a", "b", "c", "deep.txt")

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-6",
		Name:  WriteToolName,
		Input: `{"file_path": "` + deepFile + `", "content": "deep content"}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	got, err := os.ReadFile(deepFile)
	require.NoError(t, err)
	assert.Equal(t, "deep content", string(got))
}

func TestWriteTool_DirectoryPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "somedir")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-7",
		Name:  WriteToolName,
		Input: `{"file_path": "` + dirPath + `", "content": "content"}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "directory, not a file")
}

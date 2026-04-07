package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func writeToolCtx(sessionID string) context.Context {
	return context.WithValue(context.Background(), SessionIDContextKey, sessionID)
}

func writeToolCall(filePath, content string) fantasy.ToolCall {
	input, _ := json.Marshal(WriteParams{
		FilePath: filePath,
		Content:  content,
	})
	return fantasy.ToolCall{
		ID:    "test-call-1",
		Name:  WriteToolName,
		Input: string(input),
	}
}

// ---------- tests ----------

func TestWriteToolHistoryNoExistingRow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set") // simulate missing row

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	resp, err := tool.Run(ctx, writeToolCall(testFile, newContent))
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	// When there is no history row, Create must be called with oldContent.
	require.Len(t, hist.createCalls, 1, "Create should be called exactly once")
	require.Equal(t, oldContent, hist.createCalls[0].Content)

	// After Create, file.Content stays as zero-value ("") because the code
	// doesn't reassign file.  So "file.Content != oldContent" is true and an
	// intermediate CreateVersion is emitted, followed by the final one.
	require.GreaterOrEqual(t, len(hist.createVersionCalls), 1, "CreateVersion should be called at least once (the final version)")

	// The last CreateVersion must always store newContent.
	last := hist.createVersionCalls[len(hist.createVersionCalls)-1]
	require.Equal(t, newContent, last.Content)
}

func TestWriteToolHistoryExistingRowContentMatches(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	// GetByPathAndSession succeeds with content matching oldContent -> no intermediate version.
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	resp, err := tool.Run(ctx, writeToolCall(testFile, newContent))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// No Create needed because GetByPathAndSession succeeded.
	require.Empty(t, hist.createCalls)

	// Only the final CreateVersion should be called (no intermediate).
	require.Len(t, hist.createVersionCalls, 1)
	require.Equal(t, newContent, hist.createVersionCalls[0].Content)
}

func TestWriteToolHistoryExistingRowContentDiffers(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	// History row exists but has stale content (user changed file externally).
	hist.getByPathResult = history.File{Path: testFile, Content: "stale content\n"}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	resp, err := tool.Run(ctx, writeToolCall(testFile, newContent))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	require.Empty(t, hist.createCalls)

	// Two CreateVersion calls: intermediate (oldContent) + final (newContent).
	require.Len(t, hist.createVersionCalls, 2)
	require.Equal(t, oldContent, hist.createVersionCalls[0].Content)
	require.Equal(t, newContent, hist.createVersionCalls[1].Content)
}

func TestWriteToolCreateVersionFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}
	hist.createVersionErr = fmt.Errorf("disk full")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	resp, err := tool.Run(ctx, writeToolCall(testFile, newContent))

	// CreateVersion errors are logged but do not fail the operation.
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// The file should still be written on disk despite history failure.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, newContent, string(data))
}

func TestWriteToolHistoryCreateFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")
	hist.createErr = fmt.Errorf("db connection lost")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	_, err := tool.Run(ctx, writeToolCall(testFile, newContent))

	// Create failure returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file history")
}

func TestWriteToolPermissionDeniedNoHistorySideEffect(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "old content\n"
	newContent := "new content\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &denyPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	_, err := tool.Run(ctx, writeToolCall(testFile, newContent))

	// Permission denied returns an error.
	require.Error(t, err)

	// No history mutations should happen when permission is denied.
	require.Empty(t, hist.createCalls)
	require.Empty(t, hist.createVersionCalls)

	// File on disk should not be changed.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, oldContent, string(data))
}

func TestWriteToolNewFileHistoryNoExistingRow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "brand_new.txt")
	newContent := "brand new content\n"

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewWriteTool(nil, perms, hist, ft, tmpDir)

	ctx := writeToolCtx("sess-1")
	resp, err := tool.Run(ctx, writeToolCall(testFile, newContent))
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected success, got: %s", resp.Content)

	// For a new file, Create is called with empty oldContent.
	require.Len(t, hist.createCalls, 1)
	require.Equal(t, "", hist.createCalls[0].Content)

	// CreateVersion stores the new content.
	require.GreaterOrEqual(t, len(hist.createVersionCalls), 1)
	last := hist.createVersionCalls[len(hist.createVersionCalls)-1]
	require.Equal(t, newContent, last.Content)
}

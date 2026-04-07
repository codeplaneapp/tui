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

func editToolCtx(sessionID string) context.Context {
	return context.WithValue(context.Background(), SessionIDContextKey, sessionID)
}

func editToolCall(filePath, oldString, newString string) fantasy.ToolCall {
	input, _ := json.Marshal(EditParams{
		FilePath:  filePath,
		OldString: oldString,
		NewString: newString,
	})
	return fantasy.ToolCall{
		ID:    "test-edit-1",
		Name:  EditToolName,
		Input: string(input),
	}
}

// ---------- replace-content history tests ----------

func TestEditToolReplaceHistoryNoExistingRow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error response: %s", resp.Content)

	// Create must be called with oldContent when no history row exists.
	require.Len(t, hist.createCalls, 1)
	require.Equal(t, oldContent, hist.createCalls[0].Content)

	// CreateVersion should be called at least once (final version).
	require.GreaterOrEqual(t, len(hist.createVersionCalls), 1)
	last := hist.createVersionCalls[len(hist.createVersionCalls)-1]
	require.Equal(t, "goodbye world\n", last.Content)
}

func TestEditToolReplaceHistoryExistingRowMatchesContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// No Create needed.
	require.Empty(t, hist.createCalls)

	// Only the final version (no intermediate).
	require.Len(t, hist.createVersionCalls, 1)
	require.Equal(t, "goodbye world\n", hist.createVersionCalls[0].Content)
}

func TestEditToolReplaceHistoryExistingRowStaleContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: "stale\n"}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// No Create (row exists).
	require.Empty(t, hist.createCalls)

	// Two CreateVersion: intermediate (oldContent) + final (newContent).
	require.Len(t, hist.createVersionCalls, 2)
	require.Equal(t, oldContent, hist.createVersionCalls[0].Content)
	require.Equal(t, "goodbye world\n", hist.createVersionCalls[1].Content)
}

func TestEditToolReplaceCreateVersionFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}
	hist.createVersionErr = fmt.Errorf("disk full")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))

	// CreateVersion errors are logged but don't fail the operation.
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// File on disk should still be written.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, "goodbye world\n", string(data))
}

func TestEditToolReplaceCreateFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")
	hist.createErr = fmt.Errorf("db connection lost")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	_, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file history")
}

// ---------- delete-content history tests ----------

func TestEditToolDeleteHistoryNoExistingRow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	// Delete "hello " from the file (OldString set, NewString empty).
	input, _ := json.Marshal(EditParams{
		FilePath:  testFile,
		OldString: "hello ",
		NewString: "",
	})
	call := fantasy.ToolCall{ID: "test-del-1", Name: EditToolName, Input: string(input)}

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, call)
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	// Create must be called.
	require.Len(t, hist.createCalls, 1)
	require.Equal(t, oldContent, hist.createCalls[0].Content)

	// At least the final CreateVersion with the new content.
	require.GreaterOrEqual(t, len(hist.createVersionCalls), 1)
	last := hist.createVersionCalls[len(hist.createVersionCalls)-1]
	require.Equal(t, "world\n", last.Content)
}

// ---------- permission denied + history isolation ----------

func TestEditToolPermissionDeniedNoHistorySideEffect(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "hello world\n"

	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &denyPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	ctx := editToolCtx("sess-1")
	_, err := tool.Run(ctx, editToolCall(testFile, "hello", "goodbye"))

	require.Error(t, err)

	require.Empty(t, hist.createCalls)
	require.Empty(t, hist.createVersionCalls)

	// File on disk should be untouched.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, oldContent, string(data))
}

// ---------- create-new-file history tests ----------

func TestEditToolCreateNewFileHistory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "new_file.txt")
	newContent := "brand new content\n"

	hist := newConfigurableHistoryService()
	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	// OldString == "" triggers createNewFile path.
	input, _ := json.Marshal(EditParams{
		FilePath:  testFile,
		OldString: "",
		NewString: newContent,
	})
	call := fantasy.ToolCall{ID: "test-create-1", Name: EditToolName, Input: string(input)}

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, call)
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	// Create is called with empty content (new file).
	require.Len(t, hist.createCalls, 1)
	require.Equal(t, "", hist.createCalls[0].Content)

	// CreateVersion stores the new content.
	require.Len(t, hist.createVersionCalls, 1)
	require.Equal(t, newContent, hist.createVersionCalls[0].Content)
}

func TestEditToolCreateNewFileCreateVersionFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "new_file.txt")
	newContent := "brand new content\n"

	hist := newConfigurableHistoryService()
	hist.createVersionErr = fmt.Errorf("disk full")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewEditTool(nil, perms, hist, ft, tmpDir)

	input, _ := json.Marshal(EditParams{
		FilePath:  testFile,
		OldString: "",
		NewString: newContent,
	})
	call := fantasy.ToolCall{ID: "test-create-2", Name: EditToolName, Input: string(input)}

	ctx := editToolCtx("sess-1")
	resp, err := tool.Run(ctx, call)

	// CreateVersion failure in createNewFile is logged, not fatal.
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// File should still exist on disk.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, newContent, string(data))
}

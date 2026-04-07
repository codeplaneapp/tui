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

func TestApplyEditToContentPartialSuccess(t *testing.T) {
	t.Parallel()

	content := "line 1\nline 2\nline 3\n"

	// Test successful edit.
	newContent, err := applyEditToContent(content, MultiEditOperation{
		OldString: "line 1",
		NewString: "LINE 1",
	})
	require.NoError(t, err)
	require.Contains(t, newContent, "LINE 1")
	require.Contains(t, newContent, "line 2")

	// Test failed edit (string not found).
	_, err = applyEditToContent(content, MultiEditOperation{
		OldString: "line 99",
		NewString: "LINE 99",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestMultiEditSequentialApplication(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create test file.
	content := "line 1\nline 2\nline 3\nline 4\n"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	require.NoError(t, err)

	// Manually test the sequential application logic.
	currentContent := content

	// Apply edits sequentially, tracking failures.
	edits := []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},   // Should succeed
		{OldString: "line 99", NewString: "LINE 99"}, // Should fail - doesn't exist
		{OldString: "line 3", NewString: "LINE 3"},   // Should succeed
		{OldString: "line 2", NewString: "LINE 2"},   // Should succeed - still exists
	}

	var failedEdits []FailedEdit
	successCount := 0

	for i, edit := range edits {
		newContent, err := applyEditToContent(currentContent, edit)
		if err != nil {
			failedEdits = append(failedEdits, FailedEdit{
				Index: i + 1,
				Error: err.Error(),
				Edit:  edit,
			})
			continue
		}
		currentContent = newContent
		successCount++
	}

	// Verify results.
	require.Equal(t, 3, successCount, "Expected 3 successful edits")
	require.Len(t, failedEdits, 1, "Expected 1 failed edit")

	// Check failed edit details.
	require.Equal(t, 2, failedEdits[0].Index)
	require.Contains(t, failedEdits[0].Error, "not found")

	// Verify content changes.
	require.Contains(t, currentContent, "LINE 1")
	require.Contains(t, currentContent, "LINE 2")
	require.Contains(t, currentContent, "LINE 3")
	require.Contains(t, currentContent, "line 4") // Original unchanged
	require.NotContains(t, currentContent, "LINE 99")
}

func TestMultiEditAllEditsSucceed(t *testing.T) {
	t.Parallel()

	content := "line 1\nline 2\nline 3\n"

	edits := []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
		{OldString: "line 2", NewString: "LINE 2"},
		{OldString: "line 3", NewString: "LINE 3"},
	}

	currentContent := content
	successCount := 0

	for _, edit := range edits {
		newContent, err := applyEditToContent(currentContent, edit)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		currentContent = newContent
		successCount++
	}

	require.Equal(t, 3, successCount)
	require.Contains(t, currentContent, "LINE 1")
	require.Contains(t, currentContent, "LINE 2")
	require.Contains(t, currentContent, "LINE 3")
}

func TestMultiEditAllEditsFail(t *testing.T) {
	t.Parallel()

	content := "line 1\nline 2\n"

	edits := []MultiEditOperation{
		{OldString: "line 99", NewString: "LINE 99"},
		{OldString: "line 100", NewString: "LINE 100"},
	}

	currentContent := content
	var failedEdits []FailedEdit

	for i, edit := range edits {
		newContent, err := applyEditToContent(currentContent, edit)
		if err != nil {
			failedEdits = append(failedEdits, FailedEdit{
				Index: i + 1,
				Error: err.Error(),
				Edit:  edit,
			})
			continue
		}
		currentContent = newContent
	}

	require.Len(t, failedEdits, 2)
	require.Equal(t, content, currentContent, "Content should be unchanged")
}

// --- multiedit history integration tests (exercising processMultiEditExistingFile) ---

func multiEditToolCtx(sessionID string) context.Context {
	return context.WithValue(context.Background(), SessionIDContextKey, sessionID)
}

func multiEditToolCall(filePath string, edits []MultiEditOperation) fantasy.ToolCall {
	input, _ := json.Marshal(MultiEditParams{
		FilePath: filePath,
		Edits:    edits,
	})
	return fantasy.ToolCall{
		ID:    "test-multiedit-1",
		Name:  MultiEditToolName,
		Input: string(input),
	}
}

func TestMultiEditToolHistoryNoExistingRow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	resp, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	// Create must be called with oldContent because GetByPathAndSession failed.
	require.Len(t, hist.createCalls, 1)
	require.Equal(t, oldContent, hist.createCalls[0].Content)

	// At least one CreateVersion (the final) must record the new content.
	require.GreaterOrEqual(t, len(hist.createVersionCalls), 1)
	last := hist.createVersionCalls[len(hist.createVersionCalls)-1]
	require.Equal(t, "LINE 1\nline 2\n", last.Content)
}

func TestMultiEditToolHistoryExistingRowMatchesContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	resp, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	require.Empty(t, hist.createCalls)

	// Only the final CreateVersion (no intermediate).
	require.Len(t, hist.createVersionCalls, 1)
	require.Equal(t, "LINE 1\nline 2\n", hist.createVersionCalls[0].Content)
}

func TestMultiEditToolHistoryExistingRowStaleContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: "stale\n"}

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	resp, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))
	require.NoError(t, err)
	require.False(t, resp.IsError)

	require.Empty(t, hist.createCalls)

	// Two CreateVersion calls: intermediate (oldContent) + final (newContent).
	require.Len(t, hist.createVersionCalls, 2)
	require.Equal(t, oldContent, hist.createVersionCalls[0].Content)
	require.Equal(t, "LINE 1\nline 2\n", hist.createVersionCalls[1].Content)
}

func TestMultiEditToolCreateVersionFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathResult = history.File{Path: testFile, Content: oldContent}
	hist.createVersionErr = fmt.Errorf("disk full")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	resp, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))

	// CreateVersion errors are logged but do not fail the operation.
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// File on disk should still reflect the edit.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, "LINE 1\nline 2\n", string(data))
}

func TestMultiEditToolHistoryCreateFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	hist.getByPathErr = fmt.Errorf("sql: no rows in result set")
	hist.createErr = fmt.Errorf("db connection lost")

	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &mockPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	_, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file history")
}

func TestMultiEditToolPermissionDeniedNoHistorySideEffect(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	oldContent := "line 1\nline 2\n"
	require.NoError(t, os.WriteFile(testFile, []byte(oldContent), 0o644))

	hist := newConfigurableHistoryService()
	ft := &mockFileTrackerService{lastReadTime: time.Now().Add(time.Hour)}
	perms := &denyPermissionService{Broker: pubsub.NewBroker[permission.PermissionRequest]()}

	tool := NewMultiEditTool(nil, perms, hist, ft, tmpDir)

	ctx := multiEditToolCtx("sess-1")
	_, err := tool.Run(ctx, multiEditToolCall(testFile, []MultiEditOperation{
		{OldString: "line 1", NewString: "LINE 1"},
	}))

	require.Error(t, err)

	require.Empty(t, hist.createCalls)
	require.Empty(t, hist.createVersionCalls)

	// File on disk should be untouched.
	data, _ := os.ReadFile(testFile)
	require.Equal(t, oldContent, string(data))
}

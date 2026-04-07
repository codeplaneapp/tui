package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/filetracker"
	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPermissionService struct {
	*pubsub.Broker[permission.PermissionRequest]
	deny       bool
	requestErr error
	requests   []permission.CreatePermissionRequest
}

func (m *mockPermissionService) Request(ctx context.Context, req permission.CreatePermissionRequest) (bool, error) {
	m.requests = append(m.requests, req)
	if m.requestErr != nil {
		return false, m.requestErr
	}
	return !m.deny, nil
}

func (m *mockPermissionService) Grant(req permission.PermissionRequest) {}

func (m *mockPermissionService) Deny(req permission.PermissionRequest) {}

func (m *mockPermissionService) GrantPersistent(req permission.PermissionRequest) {}

func (m *mockPermissionService) AutoApproveSession(sessionID string) {}

func (m *mockPermissionService) SetSkipRequests(skip bool) {}

func (m *mockPermissionService) SkipRequests() bool {
	return false
}

func (m *mockPermissionService) SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[permission.PermissionNotification] {
	return make(<-chan pubsub.Event[permission.PermissionNotification])
}

type mockHistoryService struct {
	*pubsub.Broker[history.File]
	getFile            history.File
	getErr             error
	createFile         history.File
	createErr          error
	createCalls        []string
	createVersionCalls []string
	createVersionErrs  map[int]error
	callOrder          []string
}

func (m *mockHistoryService) Create(ctx context.Context, sessionID, path, content string) (history.File, error) {
	m.callOrder = append(m.callOrder, "create:"+content)
	m.createCalls = append(m.createCalls, content)
	if m.createErr != nil {
		return history.File{}, m.createErr
	}

	file := m.createFile
	if file.Path == "" {
		file.Path = path
	}
	if file.Content == "" {
		file.Content = content
	}

	return file, nil
}

func (m *mockHistoryService) CreateVersion(ctx context.Context, sessionID, path, content string) (history.File, error) {
	m.callOrder = append(m.callOrder, "createVersion:"+content)
	m.createVersionCalls = append(m.createVersionCalls, content)

	if err, ok := m.createVersionErrs[len(m.createVersionCalls)]; ok {
		return history.File{}, err
	}

	return history.File{Path: path, Content: content}, nil
}

func (m *mockHistoryService) GetByPathAndSession(ctx context.Context, path, sessionID string) (history.File, error) {
	m.callOrder = append(m.callOrder, "get")
	if m.getErr != nil {
		return history.File{}, m.getErr
	}

	file := m.getFile
	if file.Path == "" {
		file.Path = path
	}

	return file, nil
}

func (m *mockHistoryService) Get(ctx context.Context, id string) (history.File, error) {
	return history.File{}, nil
}

func (m *mockHistoryService) ListBySession(ctx context.Context, sessionID string) ([]history.File, error) {
	return nil, nil
}

func (m *mockHistoryService) ListLatestSessionFiles(ctx context.Context, sessionID string) ([]history.File, error) {
	return nil, nil
}

func (m *mockHistoryService) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockHistoryService) DeleteSessionFiles(ctx context.Context, sessionID string) error {
	return nil
}

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

// mockFiletrackerService is a test double for filetracker.Service.
type mockFiletrackerService struct {
	reads map[string]time.Time
}

var _ filetracker.Service = (*mockFiletrackerService)(nil)

func newMockFiletrackerService() *mockFiletrackerService {
	return &mockFiletrackerService{reads: make(map[string]time.Time)}
}

func (m *mockFiletrackerService) RecordRead(_ context.Context, sessionID, path string) {
	m.reads[sessionID+":"+path] = time.Now()
}

func (m *mockFiletrackerService) LastReadTime(_ context.Context, sessionID, path string) time.Time {
	return m.reads[sessionID+":"+path]
}

func (m *mockFiletrackerService) ListReadFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func TestMultiEditTool_EmptyEdits(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tool := NewMultiEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, newMockFiletrackerService(), tmpDir)

	// Invoke the tool with an empty edits array via the Run method.
	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  MultiEditToolName,
		Input: `{"file_path": "/some/file.txt", "edits": []}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError, "expected an error response when edits are empty")
	assert.Contains(t, resp.Content, "at least one edit operation is required")
}

func TestMultiEditTool_AllEditsSucceed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "target.txt")

	originalContent := "alpha\nbeta\ngamma\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")

	// Record a read so the filetracker check passes.
	ft.RecordRead(ctx, "test-session", testFile)

	tool := NewMultiEditTool(nil, &mockPermissionService{}, &mockHistoryService{}, ft, tmpDir)

	// Apply three edits that should all succeed.
	input := `{
		"file_path": "` + testFile + `",
		"edits": [
			{"old_string": "alpha", "new_string": "ALPHA"},
			{"old_string": "beta",  "new_string": "BETA"},
			{"old_string": "gamma", "new_string": "GAMMA"}
		]
	}`

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  MultiEditToolName,
		Input: input,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected a success response, got: %s", resp.Content)
	assert.Contains(t, resp.Content, "Applied 3 edits")

	// Verify the file was actually written with all edits applied.
	got, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "ALPHA\nBETA\nGAMMA\n", string(got))
}

func TestMultiEditTool_MissingHistoryRowDoesNotCreateDuplicateOldVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "target.txt")
	originalContent := "alpha\nbeta\ngamma\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr: errors.New("missing history row"),
	}
	tool := NewMultiEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-3",
		Name: MultiEditToolName,
		Input: `{
			"file_path": "` + testFile + `",
			"edits": [{"old_string": "alpha", "new_string": "ALPHA"}]
		}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{originalContent}, historyService.createCalls)
	assert.Equal(t, []string{"ALPHA\nbeta\ngamma\n"}, historyService.createVersionCalls)
	assert.Equal(t,
		[]string{
			"get",
			"create:" + originalContent,
			"createVersion:ALPHA\nbeta\ngamma\n",
		},
		historyService.callOrder,
	)
}

func TestMultiEditTool_MissingHistoryRowCreateFailureReturnsError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "target.txt")
	originalContent := "alpha\nbeta\ngamma\n"
	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0o644))

	ft := newMockFiletrackerService()
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	ft.RecordRead(ctx, "test-session", testFile)

	historyService := &mockHistoryService{
		getErr:    errors.New("missing history row"),
		createErr: errors.New("create failed"),
	}
	tool := NewMultiEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-4",
		Name: MultiEditToolName,
		Input: `{
			"file_path": "` + testFile + `",
			"edits": [{"old_string": "alpha", "new_string": "ALPHA"}]
		}`,
	})

	require.ErrorContains(t, err, "error creating file history: create failed")
	assert.Equal(t, []string{"get", "create:" + originalContent}, historyService.callOrder)
	assert.Empty(t, historyService.createVersionCalls)
}

func TestMultiEditTool_CreateVersionFailureDoesNotFailEdit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "target.txt")
	originalContent := "alpha\nbeta\ngamma\n"
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
	tool := NewMultiEditTool(nil, &mockPermissionService{}, historyService, ft, tmpDir)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-5",
		Name: MultiEditToolName,
		Input: `{
			"file_path": "` + testFile + `",
			"edits": [{"old_string": "alpha", "new_string": "ALPHA"}]
		}`,
	})

	require.NoError(t, err)
	assert.False(t, resp.IsError, "expected success, got: %s", resp.Content)
	assert.Equal(t, []string{originalContent, "ALPHA\nbeta\ngamma\n"}, historyService.createVersionCalls)
	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, "ALPHA\nbeta\ngamma\n", string(got))
}

func TestMultiEditTool_PermissionDeniedSkipsHistory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "target.txt")
	originalContent := "alpha\nbeta\ngamma\n"
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
	tool := NewMultiEditTool(nil, permissionService, historyService, ft, tmpDir)

	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-6",
		Name: MultiEditToolName,
		Input: `{
			"file_path": "` + testFile + `",
			"edits": [{"old_string": "alpha", "new_string": "ALPHA"}]
		}`,
	})

	require.ErrorIs(t, err, permission.ErrorPermissionDenied)
	assert.Len(t, permissionService.requests, 1)
	assert.Empty(t, historyService.callOrder)

	got, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr)
	assert.Equal(t, originalContent, string(got))
}

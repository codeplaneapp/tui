package backend

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackend_GetWorkspace_NotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	ws, err := b.GetWorkspace("nonexistent-id")
	assert.Nil(t, ws)
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

func TestBackend_DeleteWorkspace_LastTriggersShutdown(t *testing.T) {
	var shutdownCalled atomic.Bool

	tmpDir := t.TempDir()
	b := New(t.Context(), nil, func() {
		shutdownCalled.Store(true)
	})

	// Use CreateWorkspace to get a fully-initialized workspace, then delete it.
	ws, _, err := b.CreateWorkspace(proto.Workspace{
		Path: tmpDir,
	})
	require.NoError(t, err)
	require.NotNil(t, ws)

	assert.False(t, shutdownCalled.Load(), "shutdown should not be called before delete")

	b.DeleteWorkspace(ws.ID)

	assert.True(t, shutdownCalled.Load(), "shutdown should be called when last workspace is deleted")
}

func TestBackend_CreateWorkspace_EmptyPath(t *testing.T) {
	b := New(t.Context(), nil, nil)

	ws, result, err := b.CreateWorkspace(proto.Workspace{Path: ""})
	assert.Nil(t, ws)
	assert.Equal(t, proto.Workspace{}, result)
	assert.ErrorIs(t, err, ErrPathRequired)
}

func TestBackend_ListWorkspaces_Empty(t *testing.T) {
	b := New(t.Context(), nil, nil)

	workspaces := b.ListWorkspaces()
	require.NotNil(t, workspaces, "ListWorkspaces should return empty slice, not nil")
	assert.Empty(t, workspaces)
}

func TestBackend_VersionInfo(t *testing.T) {
	b := New(t.Context(), nil, nil)

	info := b.VersionInfo()
	assert.NotEmpty(t, info.GoVersion, "GoVersion should not be empty")
	assert.NotEmpty(t, info.Platform, "Platform should not be empty")
	// Version and Commit may be "devel"/"unknown" in tests, but should not be blank.
	assert.NotEmpty(t, info.Version, "Version should not be empty")
	assert.NotEmpty(t, info.Commit, "Commit should not be empty")
}

func TestBackend_DeleteWorkspace_NotFound(t *testing.T) {
	var shutdownCalled atomic.Bool
	b := New(context.Background(), nil, func() {
		shutdownCalled.Store(true)
	})

	// Deleting a non-existent workspace should be a no-op (no panic).
	assert.NotPanics(t, func() {
		b.DeleteWorkspace("does-not-exist")
	})

	// The shutdown callback should NOT be called because no real workspace was removed.
	// The workspaces map is still empty (was never populated), but the code checks
	// workspaces.Len() == 0 AFTER delete. However, since the workspace wasn't found,
	// the function returns early before the shutdown check.
	assert.False(t, shutdownCalled.Load(), "shutdown should not be called for non-existent workspace delete")
}

// --- SendMessage ---

func TestBackend_SendMessage_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.SendMessage(t.Context(), "nonexistent", proto.AgentMessage{})
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

func TestBackend_SendMessage_AgentNotInitialized(t *testing.T) {
	b := New(t.Context(), nil, nil)

	// Manually insert a workspace with a nil AgentCoordinator to avoid
	// the full CreateWorkspace flow which may auto-initialize the agent.
	ws := &Workspace{
		App:  &app.App{}, // AgentCoordinator is nil by default.
		ID:   "ws-no-agent",
		Path: t.TempDir(),
	}
	b.workspaces.Set(ws.ID, ws)

	err := b.SendMessage(t.Context(), ws.ID, proto.AgentMessage{
		SessionID: "some-session",
		Prompt:    "hello",
	})
	assert.ErrorIs(t, err, ErrAgentNotInitialized)
}

// --- GetAgentInfo ---

func TestBackend_GetAgentInfo_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	info, err := b.GetAgentInfo("nonexistent")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	assert.Equal(t, proto.AgentInfo{}, info)
}

// --- InitAgent ---

func TestBackend_InitAgent_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.InitAgent(t.Context(), "nonexistent")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- CreateSession ---

func TestBackend_CreateSession_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	_, err := b.CreateSession(t.Context(), "nonexistent", "title")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- ListSessions ---

func TestBackend_ListSessions_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	sessions, err := b.ListSessions(t.Context(), "nonexistent")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	assert.Nil(t, sessions)
}

// --- DeleteSession ---

func TestBackend_DeleteSession_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.DeleteSession(t.Context(), "nonexistent", "some-session")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- GrantPermission ---

func TestBackend_GrantPermission_InvalidAction(t *testing.T) {
	b := New(t.Context(), nil, nil)

	ws := &Workspace{
		App: &app.App{
			Permissions: permission.NewPermissionService(t.TempDir(), false, nil),
		},
		ID:   "ws-perm-test",
		Path: t.TempDir(),
	}
	b.workspaces.Set(ws.ID, ws)

	err := b.GrantPermission(ws.ID, proto.PermissionGrant{
		Permission: proto.PermissionRequest{
			ID:       "perm-1",
			ToolName: "bash",
		},
		Action: proto.PermissionAction("invalid_action"),
	})
	assert.ErrorIs(t, err, ErrInvalidPermissionAction)
}

// --- SetPermissionsSkip ---

func TestBackend_SetPermissionsSkip_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.SetPermissionsSkip("nonexistent", true)
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- GetPermissionsSkip ---

func TestBackend_GetPermissionsSkip_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	skip, err := b.GetPermissionsSkip("nonexistent")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	assert.False(t, skip)
}

// --- SetConfigField ---

func TestBackend_SetConfigField_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.SetConfigField("nonexistent", config.ScopeGlobal, "some.key", "value")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- GetWorkingDir ---

func TestBackend_GetWorkingDir_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	dir, err := b.GetWorkingDir("nonexistent")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	assert.Empty(t, dir)
}

// --- FileTrackerRecordRead ---

func TestBackend_FileTrackerRecordRead_WorkspaceNotFound(t *testing.T) {
	b := New(t.Context(), nil, nil)

	err := b.FileTrackerRecordRead(t.Context(), "nonexistent", "session-1", "/some/path")
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}

// --- createDotCrushDir ---

func TestBackend_CreateDotCrushDir(t *testing.T) {
	tmpDir := t.TempDir()
	crushDir := filepath.Join(tmpDir, ".crush")

	err := createDotCrushDir(crushDir)
	require.NoError(t, err)

	// Verify the directory was created.
	info, err := os.Stat(crushDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "expected .crush to be a directory")

	// Verify the .gitignore file was created with correct contents.
	gitignorePath := filepath.Join(crushDir, ".gitignore")
	contents, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(contents))

	// Calling it again should be idempotent (no error, .gitignore unchanged).
	err = createDotCrushDir(crushDir)
	require.NoError(t, err)
	contents2, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(contents2))
}

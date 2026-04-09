package views

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mock
// ---------------------------------------------------------------------------

type mockWorkspaceManager struct {
	repo       *jjhub.Repo
	workspaces []jjhub.Workspace
	snapshots  []jjhub.WorkspaceSnapshot

	createCalls []struct {
		name       string
		snapshotID string
	}
	deleteCalls  []string
	suspendCalls []string
	resumeCalls  []string
	forkCalls    []struct {
		workspaceID string
		name        string
	}
	snapshotCreateCalls []struct {
		workspaceID string
		name        string
	}
	snapshotDeleteCalls []string

	createErr     error
	deleteErr     error
	suspendErr    error
	resumeErr     error
	forkErr       error
	snapCreateErr error
	snapDeleteErr error
}

func (m *mockWorkspaceManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	if m.repo != nil {
		return m.repo, nil
	}
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockWorkspaceManager) ListWorkspaces(_ context.Context, limit int) ([]jjhub.Workspace, error) {
	out := append([]jjhub.Workspace(nil), m.workspaces[:min(limit, len(m.workspaces))]...)
	return out, nil
}

func (m *mockWorkspaceManager) CreateWorkspace(_ context.Context, name, snapshotID string) (*jjhub.Workspace, error) {
	m.createCalls = append(m.createCalls, struct {
		name       string
		snapshotID string
	}{name: name, snapshotID: snapshotID})
	if m.createErr != nil {
		return nil, m.createErr
	}
	ws := jjhub.Workspace{ID: "ws-new", Name: name, Status: "running"}
	return &ws, nil
}

func (m *mockWorkspaceManager) DeleteWorkspace(_ context.Context, workspaceID string) error {
	m.deleteCalls = append(m.deleteCalls, workspaceID)
	return m.deleteErr
}

func (m *mockWorkspaceManager) SuspendWorkspace(_ context.Context, workspaceID string) (*jjhub.Workspace, error) {
	m.suspendCalls = append(m.suspendCalls, workspaceID)
	if m.suspendErr != nil {
		return nil, m.suspendErr
	}
	return &jjhub.Workspace{ID: workspaceID, Status: "suspended"}, nil
}

func (m *mockWorkspaceManager) ResumeWorkspace(_ context.Context, workspaceID string) (*jjhub.Workspace, error) {
	m.resumeCalls = append(m.resumeCalls, workspaceID)
	if m.resumeErr != nil {
		return nil, m.resumeErr
	}
	return &jjhub.Workspace{ID: workspaceID, Status: "running"}, nil
}

func (m *mockWorkspaceManager) ForkWorkspace(_ context.Context, workspaceID, name string) (*jjhub.Workspace, error) {
	m.forkCalls = append(m.forkCalls, struct {
		workspaceID string
		name        string
	}{workspaceID: workspaceID, name: name})
	if m.forkErr != nil {
		return nil, m.forkErr
	}
	return &jjhub.Workspace{ID: "ws-forked", Name: name, Status: "running"}, nil
}

func (m *mockWorkspaceManager) ListWorkspaceSnapshots(_ context.Context, limit int) ([]jjhub.WorkspaceSnapshot, error) {
	out := append([]jjhub.WorkspaceSnapshot(nil), m.snapshots[:min(limit, len(m.snapshots))]...)
	return out, nil
}

func (m *mockWorkspaceManager) CreateWorkspaceSnapshot(_ context.Context, workspaceID, name string) (*jjhub.WorkspaceSnapshot, error) {
	m.snapshotCreateCalls = append(m.snapshotCreateCalls, struct {
		workspaceID string
		name        string
	}{workspaceID: workspaceID, name: name})
	if m.snapCreateErr != nil {
		return nil, m.snapCreateErr
	}
	return &jjhub.WorkspaceSnapshot{ID: "snap-new", Name: name, SnapshotID: "snap-001"}, nil
}

func (m *mockWorkspaceManager) DeleteWorkspaceSnapshot(_ context.Context, snapshotID string) error {
	m.snapshotDeleteCalls = append(m.snapshotDeleteCalls, snapshotID)
	return m.snapDeleteErr
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func wsStrPtr(s string) *string { return &s }

func makeWS(id, name, status string) jjhub.Workspace {
	return jjhub.Workspace{ID: id, Name: name, Status: status}
}

func makeWSWithSSH(id, name, status, sshHost string) jjhub.Workspace {
	ws := makeWS(id, name, status)
	ws.SSHHost = wsStrPtr(sshHost)
	return ws
}

func makeWSSnapshot(id, name, snapshotID string, workspaceID *string) jjhub.WorkspaceSnapshot {
	return jjhub.WorkspaceSnapshot{ID: id, Name: name, SnapshotID: snapshotID, WorkspaceID: workspaceID}
}

func configureWorkspacesObservability(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))
}

func requireWorkspaceViewSpanAttrs(t *testing.T, operation, result string) map[string]any {
	t.Helper()

	for _, span := range observability.RecentSpans(30) {
		if span.Name != "workspace.lifecycle" {
			continue
		}
		if span.Attributes["codeplane.workspace.operation"] == operation &&
			span.Attributes["codeplane.workspace.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing workspace.lifecycle span operation=%q result=%q", operation, result)
	return nil
}

// ---------------------------------------------------------------------------
// Init (line 186)
// ---------------------------------------------------------------------------

func TestWorkspacesView_Init_NilClient(t *testing.T) {
	t.Parallel()
	v := newWorkspacesViewWithClient(nil)

	cmd := v.Init()
	assert.Nil(t, cmd, "Init should return nil when client is nil")
	assert.NotNil(t, v.err, "error should be set when client is nil")
	assert.Contains(t, v.err.Error(), "jjhub CLI not found")
}

func TestWorkspacesView_Init_WithClient(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)

	cmd := v.Init()
	require.NotNil(t, cmd, "Init should return a batch command when client is present")
	assert.True(t, v.loading, "loading should be true")
}

// ---------------------------------------------------------------------------
// CLI unavailable error rendering
// ---------------------------------------------------------------------------

func TestWorkspacesView_CLIUnavailable_RendersError(t *testing.T) {
	t.Parallel()
	v := newWorkspacesViewWithClient(nil)
	v.width = 80
	v.height = 24

	output := v.View()
	assert.Contains(t, output, "jjhub CLI not found")
}

// ---------------------------------------------------------------------------
// Empty workspace list rendering
// ---------------------------------------------------------------------------

func TestWorkspacesView_EmptyList_Rendering(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.width = 80
	v.height = 24

	// Simulate workspaces loaded with empty list
	updated, _ := v.Update(workspacesLoadedMsg{workspaces: nil})
	wv := updated.(*WorkspacesView)

	assert.False(t, wv.loading)
	assert.Nil(t, wv.err)

	output := wv.View()
	assert.Contains(t, output, "No workspaces found")
}

func TestWorkspacesView_EmptySnapshotList_Rendering(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.width = 80
	v.height = 24
	v.mode = snapshotMode
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false

	output := v.View()
	assert.Contains(t, output, "No snapshots found")
}

// ---------------------------------------------------------------------------
// Load error states (line 549: workspacesErrorMsg, workspaceSnapshotsErrorMsg)
// ---------------------------------------------------------------------------

func TestWorkspacesView_LoadError_Workspaces(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.width = 80
	v.height = 24

	updated, _ := v.Update(workspacesErrorMsg{err: errors.New("network down")})
	wv := updated.(*WorkspacesView)

	assert.False(t, wv.loading)
	assert.EqualError(t, wv.err, "network down")

	output := wv.View()
	assert.Contains(t, output, "network down")
}

func TestWorkspacesView_LoadError_Snapshots(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.width = 80
	v.height = 24
	v.mode = snapshotMode
	v.loading = false

	updated, _ := v.Update(workspaceSnapshotsErrorMsg{err: errors.New("snapshot API error")})
	wv := updated.(*WorkspacesView)

	assert.False(t, wv.snapshotsLoading)
	assert.EqualError(t, wv.snapshotsErr, "snapshot API error")

	output := wv.View()
	assert.Contains(t, output, "snapshot API error")
}

// ---------------------------------------------------------------------------
// RepoLoadedMsg
// ---------------------------------------------------------------------------

func TestWorkspacesView_RepoLoaded(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)

	repo := &jjhub.Repo{FullName: "org/myrepo"}
	updated, _ := v.Update(workspacesRepoLoadedMsg{repo: repo})
	wv := updated.(*WorkspacesView)

	assert.Equal(t, "org/myrepo", wv.repo.FullName)
}

// ---------------------------------------------------------------------------
// Cursor/page clamping: 0, 1, and N items
// ---------------------------------------------------------------------------

func TestWorkspacesView_ClampCursor_ZeroItems(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.workspaces = nil
	v.cursor = 5
	v.scrollOffset = 3

	v.clampWorkspaceCursor()

	assert.Equal(t, 0, v.cursor)
	assert.Equal(t, 0, v.scrollOffset)
}

func TestWorkspacesView_ClampCursor_OneItem(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.workspaces = []jjhub.Workspace{makeWS("ws-1", "alpha", "running")}
	v.cursor = 5
	v.height = 30

	v.clampWorkspaceCursor()

	assert.Equal(t, 0, v.cursor, "cursor should clamp to 0 with one item")
}

func TestWorkspacesView_ClampCursor_NItems(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.height = 30
	v.workspaces = make([]jjhub.Workspace, 10)
	for i := range v.workspaces {
		v.workspaces[i] = makeWS(fmt.Sprintf("ws-%d", i), fmt.Sprintf("ws%d", i), "running")
	}

	v.cursor = 20
	v.clampWorkspaceCursor()
	assert.Equal(t, 9, v.cursor, "cursor should clamp to last item index")

	v.cursor = -3
	v.clampWorkspaceCursor()
	assert.Equal(t, 0, v.cursor, "negative cursor should clamp to 0")
}

func TestWorkspacesView_ClampSnapshotCursor_ZeroItems(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.snapshots = nil
	v.snapshotCursor = 3
	v.snapshotOffset = 2

	v.clampSnapshotCursor()

	assert.Equal(t, 0, v.snapshotCursor)
	assert.Equal(t, 0, v.snapshotOffset)
}

func TestWorkspacesView_ClampSnapshotCursor_NItems(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.height = 30
	v.snapshots = make([]jjhub.WorkspaceSnapshot, 5)
	for i := range v.snapshots {
		v.snapshots[i] = makeWSSnapshot(fmt.Sprintf("snap-%d", i), fmt.Sprintf("s%d", i), fmt.Sprintf("sid-%d", i), nil)
	}

	v.snapshotCursor = 100
	v.clampSnapshotCursor()
	assert.Equal(t, 4, v.snapshotCursor, "snapshot cursor should clamp to last item")

	v.snapshotCursor = -1
	v.clampSnapshotCursor()
	assert.Equal(t, 0, v.snapshotCursor, "negative snapshot cursor should clamp to 0")
}

// ---------------------------------------------------------------------------
// Cursor navigation via key presses
// ---------------------------------------------------------------------------

func TestWorkspacesView_CursorUpDown_Workspaces(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.height = 60
	v.workspaces = []jjhub.Workspace{
		makeWS("ws-1", "first", "running"),
		makeWS("ws-2", "second", "running"),
		makeWS("ws-3", "third", "running"),
	}

	assert.Equal(t, 0, v.cursor)

	// Move down
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	wv := updated.(*WorkspacesView)
	assert.Equal(t, 1, wv.cursor)

	// Move down again
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'j'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 2, wv.cursor)

	// Move down at end - should stay
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'j'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 2, wv.cursor, "cursor should not exceed list length")

	// Move up
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 1, wv.cursor)

	// Move up to top
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 0, wv.cursor)

	// Move up at top - should stay
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 0, wv.cursor, "cursor should not go below 0")
}

func TestWorkspacesView_CursorUpDown_Snapshots(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.mode = snapshotMode
	v.height = 60
	v.snapshots = []jjhub.WorkspaceSnapshot{
		makeWSSnapshot("s-1", "snap1", "sid-1", nil),
		makeWSSnapshot("s-2", "snap2", "sid-2", nil),
	}

	assert.Equal(t, 0, v.snapshotCursor)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'j'})
	wv := updated.(*WorkspacesView)
	assert.Equal(t, 1, wv.snapshotCursor)

	// At end
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'j'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 1, wv.snapshotCursor, "snapshot cursor should not exceed list length")

	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 0, wv.snapshotCursor)

	// At top
	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'k'})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, 0, wv.snapshotCursor, "snapshot cursor should not go below 0")
}

// ---------------------------------------------------------------------------
// Tab toggles mode
// ---------------------------------------------------------------------------

func TestWorkspacesView_TabTogglesMode(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	assert.Equal(t, workspaceMode, v.mode)

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	wv := updated.(*WorkspacesView)
	assert.Equal(t, snapshotMode, wv.mode)

	updated, _ = wv.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	wv = updated.(*WorkspacesView)
	assert.Equal(t, workspaceMode, wv.mode)
}

// ---------------------------------------------------------------------------
// WindowSizeMsg sets dimensions and clamps
// ---------------------------------------------------------------------------

func TestWorkspacesView_WindowSizeMsg(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.workspaces = []jjhub.Workspace{makeWS("ws-1", "only", "running")}
	v.cursor = 5

	updated, _ := v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	wv := updated.(*WorkspacesView)
	assert.Equal(t, 120, wv.width)
	assert.Equal(t, 40, wv.height)
	assert.Equal(t, 0, wv.cursor, "cursor should be clamped after resize")
}

// ---------------------------------------------------------------------------
// Prompt cancel flows (Esc in prompt)
// ---------------------------------------------------------------------------

func TestWorkspacesView_PromptCancel_Create(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{makeWS("ws-1", "dev", "running")},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces

	// Press 'c' to open create prompt
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptCreate, wv.prompt.kind)

	// Press Esc to cancel
	updated, _ = wv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv = updated.(*WorkspacesView)
	assert.False(t, wv.prompt.active, "prompt should be closed after Esc")
}

func TestWorkspacesView_PromptCancel_Delete(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{makeWS("ws-1", "dev", "running")},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces

	// Press 'd' to open delete prompt
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptDeleteWorkspace, wv.prompt.kind)

	// Press Esc to cancel
	updated, _ = wv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv = updated.(*WorkspacesView)
	assert.False(t, wv.prompt.active)
	assert.Empty(t, manager.deleteCalls, "delete should not be called after cancel")
}

func TestWorkspacesView_PromptCancel_Fork(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{makeWS("ws-1", "dev", "running")},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces

	// Press 'f' to open fork prompt
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'f'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptFork, wv.prompt.kind)

	// Press Esc to cancel
	updated, _ = wv.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wv = updated.(*WorkspacesView)
	assert.False(t, wv.prompt.active)
	assert.Empty(t, manager.forkCalls, "fork should not be called after cancel")
}

// ---------------------------------------------------------------------------
// Delete workspace flow (line 347: createWorkspaceCmd context, 440: deleteWorkspaceCmd)
// ---------------------------------------------------------------------------

func TestWorkspacesView_DeleteWorkspace_ConfirmFlow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "my-workspace", "running"),
			makeWS("ws-2", "other", "suspended"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	// Press 'd' to open delete prompt
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptDeleteWorkspace, wv.prompt.kind)

	// Press Enter to confirm delete
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd, "submit should produce a command")

	// Execute the command
	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok, "expected workspaceActionDoneMsg, got %T", msg)
	assert.Contains(t, doneMsg.message, "Deleted")
	assert.Contains(t, doneMsg.message, "my-workspace")
	assert.Equal(t, 1, len(manager.deleteCalls))
	assert.Equal(t, "ws-1", manager.deleteCalls[0])

	// Feed the done message back
	updated, refreshCmd := wv.Update(doneMsg)
	wv = updated.(*WorkspacesView)
	assert.False(t, wv.prompt.active)
	assert.Contains(t, wv.actionMsg, "Deleted")
	require.NotNil(t, refreshCmd, "should trigger refresh after delete")
}

func TestWorkspacesView_DeleteWorkspace_NoSelection(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = nil

	// Press 'd' with no workspaces - should do nothing
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkspacesView)
	assert.False(t, wv.prompt.active, "prompt should not open with no workspace selected")
}

// ---------------------------------------------------------------------------
// Delete snapshot flow
// ---------------------------------------------------------------------------

func TestWorkspacesView_DeleteSnapshot_ConfirmFlow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		snapshots: []jjhub.WorkspaceSnapshot{
			makeWSSnapshot("snap-1", "my-snap", "sid-1", nil),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = manager.snapshots
	v.height = 30

	// Press 'd' to open delete prompt
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'd'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptDeleteSnapshot, wv.prompt.kind)

	// Press Enter to confirm
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Deleted snapshot")
	assert.Equal(t, 1, len(manager.snapshotDeleteCalls))
	assert.Equal(t, "snap-1", manager.snapshotDeleteCalls[0])
}

// ---------------------------------------------------------------------------
// Create workspace flow
// ---------------------------------------------------------------------------

func TestWorkspacesView_CreateWorkspace_Flow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.height = 30

	// Press 'c' to create
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptCreate, wv.prompt.kind)

	// Type a name and press enter
	wv.prompt.input.SetValue("new-ws")
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Created")
	assert.Equal(t, 1, len(manager.createCalls))
	assert.Equal(t, "new-ws", manager.createCalls[0].name)
	assert.Equal(t, "", manager.createCalls[0].snapshotID, "snapshotID should be empty for plain create")
}

// ---------------------------------------------------------------------------
// Snapshot-without-workspace selection (Enter in snapshot mode with no WorkspaceID)
// ---------------------------------------------------------------------------

func TestWorkspacesView_SnapshotEnter_NoWorkspaceID(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		snapshots: []jjhub.WorkspaceSnapshot{
			makeWSSnapshot("snap-1", "orphan-snap", "sid-1", nil),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = manager.snapshots
	v.height = 30

	// Press Enter on a snapshot without a workspace ID
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd, "should return nil cmd when snapshot has no workspace ID")
	assert.Equal(t, snapshotMode, wv.mode, "should stay in snapshot mode")
}

func TestWorkspacesView_SnapshotEnter_WithWorkspaceID(t *testing.T) {
	t.Parallel()
	wsID := "ws-linked"
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-linked", "linked-ws", "running"),
		},
		snapshots: []jjhub.WorkspaceSnapshot{
			makeWSSnapshot("snap-1", "linked-snap", "sid-1", wsStrPtr(wsID)),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = manager.snapshots
	v.workspaces = manager.workspaces
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd, "selecting linked workspace should not exec a command")
	assert.Equal(t, workspaceMode, wv.mode, "should switch to workspace mode")
	assert.Contains(t, wv.actionMsg, "ws-linked")
}

func TestWorkspacesView_SnapshotEnter_EmptyList(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = nil
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd)
	assert.Equal(t, snapshotMode, wv.mode, "should remain in snapshot mode when no snapshots")
}

// ---------------------------------------------------------------------------
// SSH unavailable handling (Enter in workspace mode)
// ---------------------------------------------------------------------------

func TestWorkspacesView_SSH_Unavailable(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-nossh", "no-ssh", "running"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	// Press Enter on a workspace without SSH
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd, "should not return an SSH command")
	assert.Contains(t, wv.actionMsg, "SSH is not available")
}

func TestWorkspacesView_SSH_EmptyHost(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWSWithSSH("ws-empty", "empty-ssh", "running", "  "),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd, "blank SSH host should be treated as unavailable")
	assert.Contains(t, wv.actionMsg, "SSH is not available")
}

func TestWorkspacesView_SSH_Available(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWSWithSSH("ws-ssh", "has-ssh", "running", "ws-ssh.example.com"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	// When SSH is available, sshCmd returns a tea.ExecProcess command
	require.NotNil(t, cmd, "should return an SSH exec command")
	assert.Equal(t, "ws-ssh", wv.connectingID, "connectingID should be set")
}

func TestWorkspacesView_SSH_NoWorkspaceSelected(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = nil
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv := updated.(*WorkspacesView)

	assert.Nil(t, cmd, "should return nil when no workspace selected")
	assert.Empty(t, wv.connectingID)
}

// ---------------------------------------------------------------------------
// SSH return messages
// ---------------------------------------------------------------------------

func TestWorkspacesView_SSHReturn_Error(t *testing.T) {
	configureWorkspacesObservability(t)
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.connectingID = "ws-1"

	updated, cmd := v.Update(workspaceConnectReturnMsg{
		workspaceID: "ws-1",
		mode:        workspaceConnectSSH,
		err:         errors.New("connection refused"),
	})
	wv := updated.(*WorkspacesView)

	assert.Empty(t, wv.connectingID)
	assert.Contains(t, wv.actionMsg, "SSH error")
	assert.Contains(t, wv.actionMsg, "connection refused")
	assert.Nil(t, cmd, "should not refresh on SSH error")

	attrs := requireWorkspaceViewSpanAttrs(t, "ssh", "error")
	assert.Equal(t, "ws-1", attrs["codeplane.workspace.id"])
}

func TestWorkspacesView_SSHReturn_Success(t *testing.T) {
	configureWorkspacesObservability(t)
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.connectingID = "ws-1"

	updated, cmd := v.Update(workspaceConnectReturnMsg{
		workspaceID: "ws-1",
		mode:        workspaceConnectSSH,
		err:         nil,
	})
	wv := updated.(*WorkspacesView)

	assert.Empty(t, wv.connectingID)
	assert.Contains(t, wv.actionMsg, "Disconnected")
	require.NotNil(t, cmd, "should refresh after successful SSH disconnect")

	attrs := requireWorkspaceViewSpanAttrs(t, "ssh", "ok")
	assert.Equal(t, "ws-1", attrs["codeplane.workspace.id"])
}

func TestWorkspacesView_AttachReturn_SuccessRecordsObservability(t *testing.T) {
	configureWorkspacesObservability(t)

	v := newWorkspacesViewWithClient(&mockWorkspaceManager{})
	v.connectingID = "ws-attach"

	updated, cmd := v.Update(workspaceConnectReturnMsg{
		workspaceID: "ws-attach",
		mode:        workspaceConnectAttach,
		err:         nil,
	})
	wv := updated.(*WorkspacesView)

	assert.Empty(t, wv.connectingID)
	assert.Contains(t, wv.actionMsg, "Detached from ws-attach")
	require.NotNil(t, cmd)

	attrs := requireWorkspaceViewSpanAttrs(t, "attach", "ok")
	assert.Equal(t, "ws-attach", attrs["codeplane.workspace.id"])
}

// ---------------------------------------------------------------------------
// Action error while prompt is open vs closed
// ---------------------------------------------------------------------------

func TestWorkspacesView_ActionError_PromptOpen(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.prompt.active = true
	v.prompt.kind = workspacePromptCreate

	updated, cmd := v.Update(workspaceActionErrorMsg{err: errors.New("quota exceeded")})
	wv := updated.(*WorkspacesView)

	assert.True(t, wv.prompt.active, "prompt should remain active")
	assert.EqualError(t, wv.prompt.err, "quota exceeded")
	assert.Nil(t, cmd)
}

func TestWorkspacesView_ActionError_PromptClosed(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.prompt.active = false

	updated, cmd := v.Update(workspaceActionErrorMsg{err: errors.New("server error")})
	wv := updated.(*WorkspacesView)

	assert.Equal(t, "server error", wv.actionMsg)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Suspend/resume toggle (line 411)
// ---------------------------------------------------------------------------

func TestWorkspacesView_SuspendResumeCmd_Running(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "active-ws", "running"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	// Press 's' to suspend
	updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	wv := updated.(*WorkspacesView)
	_ = wv
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Suspended")
	assert.Equal(t, 1, len(manager.suspendCalls))
	assert.Equal(t, "ws-1", manager.suspendCalls[0])
}

func TestWorkspacesView_SuspendResumeCmd_Suspended(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "paused-ws", "suspended"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	wv := updated.(*WorkspacesView)
	_ = wv
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Resumed")
	assert.Equal(t, 1, len(manager.resumeCalls))
	assert.Equal(t, "ws-1", manager.resumeCalls[0])
}

func TestWorkspacesView_SuspendResume_NoSelection(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = nil

	_, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	assert.Nil(t, cmd, "suspend should be no-op when no workspace selected")
}

// ---------------------------------------------------------------------------
// Fork workspace flow
// ---------------------------------------------------------------------------

func TestWorkspacesView_ForkWorkspace_Flow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "source-ws", "running"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	// Press 'f' to fork
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'f'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptFork, wv.prompt.kind)

	wv.prompt.input.SetValue("my-fork")
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Forked")
	assert.Equal(t, 1, len(manager.forkCalls))
	assert.Equal(t, "ws-1", manager.forkCalls[0].workspaceID)
	assert.Equal(t, "my-fork", manager.forkCalls[0].name)
}

func TestWorkspacesView_ForkWorkspace_SnapshotModeNoOp(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.mode = snapshotMode

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'f'})
	assert.Nil(t, cmd, "fork should be no-op in snapshot mode")
}

// ---------------------------------------------------------------------------
// Create snapshot flow
// ---------------------------------------------------------------------------

func TestWorkspacesView_CreateSnapshot_Flow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "snap-target", "running"),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = manager.workspaces
	v.height = 30

	// Press 'n' to create snapshot
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'n'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptSnapshot, wv.prompt.kind)

	wv.prompt.input.SetValue("my-snapshot")
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Created snapshot")
	assert.Equal(t, 1, len(manager.snapshotCreateCalls))
	assert.Equal(t, "ws-1", manager.snapshotCreateCalls[0].workspaceID)
	assert.Equal(t, "my-snapshot", manager.snapshotCreateCalls[0].name)
}

func TestWorkspacesView_CreateSnapshot_NoSelection(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = nil

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'n'})
	assert.Nil(t, cmd, "snapshot should be no-op when no workspace selected")
}

// ---------------------------------------------------------------------------
// Create workspace from snapshot
// ---------------------------------------------------------------------------

func TestWorkspacesView_CreateFromSnapshot_Flow(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{
		snapshots: []jjhub.WorkspaceSnapshot{
			makeWSSnapshot("snap-1", "base-snap", "sid-1", nil),
		},
	}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = manager.snapshots
	v.height = 30

	// Press 'c' in snapshot mode to create from snapshot
	updated, _ := v.Update(tea.KeyPressMsg{Code: 'c'})
	wv := updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	assert.Equal(t, workspacePromptCreateFromSnapshot, wv.prompt.kind)

	wv.prompt.input.SetValue("from-snap")
	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(workspaceActionDoneMsg)
	require.True(t, ok)
	assert.Contains(t, doneMsg.message, "Created workspace from")
	assert.Equal(t, 1, len(manager.createCalls))
	assert.Equal(t, "from-snap", manager.createCalls[0].name)
	assert.Equal(t, "snap-1", manager.createCalls[0].snapshotID, "should pass snapshot ID")
}

func TestWorkspacesView_CreateFromSnapshot_NoSnapshotSelected(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.snapshotsLoading = false
	v.mode = snapshotMode
	v.snapshots = nil
	v.height = 30

	// Press 'c' in snapshot mode with no snapshots
	_, cmd := v.Update(tea.KeyPressMsg{Code: 'c'})
	assert.Nil(t, cmd, "should be no-op when no snapshot selected")
}

// ---------------------------------------------------------------------------
// Esc pops the view
// ---------------------------------------------------------------------------

func TestWorkspacesView_EscPopsView(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should produce a PopViewMsg")
}

// ---------------------------------------------------------------------------
// Refresh key
// ---------------------------------------------------------------------------

func TestWorkspacesView_RefreshKey(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil

	_, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd, "r should trigger a refresh command")
	assert.True(t, v.loading, "loading should be set to true on refresh")
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestWorkspacesView_SetSize(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.workspaces = []jjhub.Workspace{makeWS("ws-1", "only", "running")}
	v.cursor = 5

	v.SetSize(100, 50)

	assert.Equal(t, 100, v.width)
	assert.Equal(t, 50, v.height)
	assert.Equal(t, 0, v.cursor, "cursor should be clamped after SetSize")
}

// ---------------------------------------------------------------------------
// Name
// ---------------------------------------------------------------------------

func TestWorkspacesView_Name(t *testing.T) {
	t.Parallel()
	v := newWorkspacesViewWithClient(nil)
	assert.Equal(t, "workspaces", v.Name())
}

// ---------------------------------------------------------------------------
// Workspace loaded message clamps cursor
// ---------------------------------------------------------------------------

func TestWorkspacesView_WorkspacesLoaded_ClampsCursor(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.cursor = 10
	v.height = 30

	updated, _ := v.Update(workspacesLoadedMsg{
		workspaces: []jjhub.Workspace{
			makeWS("ws-1", "only", "running"),
		},
	})
	wv := updated.(*WorkspacesView)

	assert.Equal(t, 0, wv.cursor, "cursor should be clamped to max index after load")
	assert.False(t, wv.loading)
	assert.Nil(t, wv.err)
}

// ---------------------------------------------------------------------------
// Snapshots loaded message clamps cursor
// ---------------------------------------------------------------------------

func TestWorkspacesView_SnapshotsLoaded_ClampsCursor(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.snapshotCursor = 10
	v.height = 30

	updated, _ := v.Update(workspaceSnapshotsLoadedMsg{
		snapshots: []jjhub.WorkspaceSnapshot{
			makeWSSnapshot("s-1", "snap1", "sid-1", nil),
			makeWSSnapshot("s-2", "snap2", "sid-2", nil),
		},
	})
	wv := updated.(*WorkspacesView)

	assert.Equal(t, 1, wv.snapshotCursor, "snapshot cursor should be clamped to last index")
	assert.False(t, wv.snapshotsLoading)
	assert.Nil(t, wv.snapshotsErr)
}

// ---------------------------------------------------------------------------
// Status filter narrows visible workspaces
// ---------------------------------------------------------------------------

func TestWorkspacesView_StatusFilter(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.workspaces = []jjhub.Workspace{
		makeWS("ws-1", "a", "running"),
		makeWS("ws-2", "b", "suspended"),
		makeWS("ws-3", "c", "running"),
	}

	v.statusFilter = "running"
	visible := v.visibleWorkspaces()
	assert.Len(t, visible, 2)

	v.statusFilter = "suspended"
	visible = v.visibleWorkspaces()
	assert.Len(t, visible, 1)
	assert.Equal(t, "ws-2", visible[0].ID)

	v.statusFilter = ""
	visible = v.visibleWorkspaces()
	assert.Len(t, visible, 3)
}

// ---------------------------------------------------------------------------
// ShortHelp returns different bindings by state
// ---------------------------------------------------------------------------

func TestWorkspacesView_ShortHelp_States(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)

	// Default workspace mode
	bindings := v.ShortHelp()
	assert.NotEmpty(t, bindings)
	assert.Contains(t, helpKeys(bindings), "b")

	// Snapshot mode
	v.mode = snapshotMode
	bindings = v.ShortHelp()
	assert.NotEmpty(t, bindings)
	assert.NotContains(t, helpKeys(bindings), "b")

	// Prompt active
	v.prompt.active = true
	bindings = v.ShortHelp()
	require.Len(t, bindings, 2, "prompt mode should have enter and esc bindings")
}

// ---------------------------------------------------------------------------
// Wide vs narrow rendering
// ---------------------------------------------------------------------------

func TestWorkspacesView_WideRendering(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = []jjhub.Workspace{
		makeWS("ws-1", "my-workspace", "running"),
	}
	v.height = 30

	// Wide layout (>= 110)
	v.width = 120
	output := v.View()
	assert.Contains(t, output, "my-workspace")

	// Narrow layout (< 110)
	v.width = 80
	output = v.View()
	assert.Contains(t, output, "my-workspace")
}

// ---------------------------------------------------------------------------
// Connecting indicator in View
// ---------------------------------------------------------------------------

func TestWorkspacesView_ConnectingIndicator(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = []jjhub.Workspace{
		makeWS("ws-1", "busy-ws", "running"),
	}
	v.connectingID = "ws-1"
	v.width = 80
	v.height = 24

	output := v.View()
	assert.Contains(t, output, "Attaching to workspace ws-1")
}

func TestWorkspacesView_SandboxModeCycleAndRendering(t *testing.T) {
	t.Parallel()
	manager := &mockWorkspaceManager{}
	v := newWorkspacesViewWithClient(manager)
	v.loading = false
	v.err = nil
	v.workspaces = []jjhub.Workspace{
		makeWS("ws-1", "busy-ws", "running"),
	}
	v.width = 120
	v.height = 24

	output := v.View()
	assert.Contains(t, output, "[sandbox: auto]")
	assert.Contains(t, output, "auto-detect bubblewrap")

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'b'})
	wv := updated.(*WorkspacesView)
	assert.Nil(t, cmd)
	assert.Equal(t, workspaceSandboxModeBubblewrap, wv.sandboxMode)
	assert.Contains(t, wv.actionMsg, "Workspace sandbox mode set to bwrap")

	output = wv.View()
	assert.Contains(t, output, "[sandbox: bwrap]")
	assert.Contains(t, output, "require bubblewrap sandbox")
}

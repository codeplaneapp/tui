package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssuesView_LoadedMsgPopulatesIssues(t *testing.T) {
	t.Parallel()

	v := &IssuesView{
		client:        jjhub.NewClient(""),
		stateFilter:   "open",
		loading:       true,
		detailCache:   make(map[int]*jjhub.Issue),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
	}

	updated, cmd := v.Update(issuesLoadedMsg{
		issues: []jjhub.Issue{{
			Number:       7,
			Title:        "Fix login redirect",
			State:        "open",
			Author:       jjhub.User{Login: "will"},
			CommentCount: 2,
			CreatedAt:    "2025-04-06T20:52:39Z",
			UpdatedAt:    "2025-04-06T20:52:47Z",
		}},
	})
	iv := updated.(*IssuesView)

	require.NotNil(t, cmd)
	assert.False(t, iv.loading)
	assert.Len(t, iv.issues, 1)
	assert.Equal(t, "issues", iv.Name())
}

func TestLandingsView_DKeyEnablesDiff(t *testing.T) {
	t.Parallel()

	v := &LandingsView{
		client:        jjhub.NewClient(""),
		stateFilter:   "open",
		landings:      []jjhub.Landing{{Number: 12, Title: "Ship feature", State: "open", Author: jjhub.User{Login: "will"}, UpdatedAt: "2025-04-06T20:52:47Z"}},
		detailCache:   make(map[int]*jjhub.LandingDetail),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
		diffCache:     make(map[int]string),
		diffErr:       make(map[int]error),
		diffLoading:   make(map[int]bool),
	}

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'd'})
	lv := updated.(*LandingsView)

	require.NotNil(t, cmd)
	assert.True(t, lv.showDiff)
	assert.Equal(t, "landings", lv.Name())
}

func TestWorkspacesView_FilterCycleAndMissingSSH(t *testing.T) {
	t.Parallel()

	v := &WorkspacesView{
		workspaces: []jjhub.Workspace{
			{ID: "ws-1", Name: "One", Status: "running", UpdatedAt: "2025-04-06T20:52:47Z"},
			{ID: "ws-2", Name: "Two", Status: "suspended", UpdatedAt: "2025-04-06T20:52:47Z"},
		},
	}

	updated, _ := v.Update(tea.KeyPressMsg{Code: 't'})
	wv := updated.(*WorkspacesView)
	assert.Equal(t, "running", wv.statusFilter)

	updated, cmd := wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.Nil(t, cmd)
	assert.Contains(t, wv.actionMsg, "SSH is not available")
	assert.Equal(t, "workspaces", wv.Name())
}

type mockWorkspaceManager struct {
	workspaces []jjhub.Workspace
	snapshots  []jjhub.WorkspaceSnapshot

	createWorkspaceName     string
	createWorkspaceSnapshot string
	forkWorkspaceID         string
	forkWorkspaceName       string
	deleteWorkspaceID       string
	suspendWorkspaceID      string
	resumeWorkspaceID       string
	createSnapshotID        string
	createSnapshotName      string
	deleteSnapshotID        string
}

func (m *mockWorkspaceManager) GetCurrentRepo() (*jjhub.Repo, error) {
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockWorkspaceManager) ListWorkspaces(limit int) ([]jjhub.Workspace, error) {
	return append([]jjhub.Workspace(nil), m.workspaces[:min(limit, len(m.workspaces))]...), nil
}

func (m *mockWorkspaceManager) CreateWorkspace(name, snapshotID string) (*jjhub.Workspace, error) {
	m.createWorkspaceName = name
	m.createWorkspaceSnapshot = snapshotID
	workspace := jjhub.Workspace{
		ID:     "ws-created",
		Name:   name,
		Status: "running",
	}
	m.workspaces = append([]jjhub.Workspace{workspace}, m.workspaces...)
	return &workspace, nil
}

func (m *mockWorkspaceManager) DeleteWorkspace(workspaceID string) error {
	m.deleteWorkspaceID = workspaceID
	return nil
}

func (m *mockWorkspaceManager) SuspendWorkspace(workspaceID string) (*jjhub.Workspace, error) {
	m.suspendWorkspaceID = workspaceID
	return &jjhub.Workspace{ID: workspaceID, Name: workspaceID, Status: "suspended"}, nil
}

func (m *mockWorkspaceManager) ResumeWorkspace(workspaceID string) (*jjhub.Workspace, error) {
	m.resumeWorkspaceID = workspaceID
	return &jjhub.Workspace{ID: workspaceID, Name: workspaceID, Status: "running"}, nil
}

func (m *mockWorkspaceManager) ForkWorkspace(workspaceID, name string) (*jjhub.Workspace, error) {
	m.forkWorkspaceID = workspaceID
	m.forkWorkspaceName = name
	return &jjhub.Workspace{ID: "ws-fork", Name: name, Status: "running"}, nil
}

func (m *mockWorkspaceManager) ListWorkspaceSnapshots(limit int) ([]jjhub.WorkspaceSnapshot, error) {
	return append([]jjhub.WorkspaceSnapshot(nil), m.snapshots[:min(limit, len(m.snapshots))]...), nil
}

func (m *mockWorkspaceManager) CreateWorkspaceSnapshot(workspaceID, name string) (*jjhub.WorkspaceSnapshot, error) {
	m.createSnapshotID = workspaceID
	m.createSnapshotName = name
	snapshot := jjhub.WorkspaceSnapshot{
		ID:         "snap-created",
		Name:       name,
		SnapshotID: "snap-created",
	}
	m.snapshots = append([]jjhub.WorkspaceSnapshot{snapshot}, m.snapshots...)
	return &snapshot, nil
}

func (m *mockWorkspaceManager) DeleteWorkspaceSnapshot(snapshotID string) error {
	m.deleteSnapshotID = snapshotID
	return nil
}

func TestWorkspacesView_CreateFromSnapshotAndDeleteSnapshot(t *testing.T) {
	t.Parallel()

	workspaceID := "ws-1"
	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			{ID: workspaceID, Name: "One", Status: "running", UpdatedAt: "2025-04-06T20:52:47Z"},
		},
		snapshots: []jjhub.WorkspaceSnapshot{
			{ID: "snap-1", Name: "Base Snapshot", SnapshotID: "snap-1", WorkspaceID: &workspaceID},
		},
	}

	v := newWorkspacesViewWithClient(manager)
	v.workspaces = manager.workspaces
	v.snapshots = manager.snapshots

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	wv := updated.(*WorkspacesView)
	require.Equal(t, snapshotMode, wv.mode)

	updated, cmd := wv.Update(tea.KeyPressMsg{Code: 'c'})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)
	wv.prompt.input.SetValue("Forked Snapshot Workspace")

	updated, cmd = wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)
	msg := cmd()

	updated, refreshCmd := wv.Update(msg)
	wv = updated.(*WorkspacesView)
	require.NotNil(t, refreshCmd)
	assert.Equal(t, "Forked Snapshot Workspace", manager.createWorkspaceName)
	assert.Equal(t, "snap-1", manager.createWorkspaceSnapshot)
	assert.False(t, wv.prompt.active)
	assert.Contains(t, wv.actionMsg, "Created workspace from")

	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'd'})
	wv = updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)

	updated, cmd = wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)

	updated, refreshCmd = wv.Update(cmd())
	wv = updated.(*WorkspacesView)
	require.NotNil(t, refreshCmd)
	assert.Equal(t, "snap-1", manager.deleteSnapshotID)
	assert.Contains(t, wv.actionMsg, "Deleted snapshot")
}

func TestWorkspacesView_SuspendResumeAndFork(t *testing.T) {
	t.Parallel()

	manager := &mockWorkspaceManager{
		workspaces: []jjhub.Workspace{
			{ID: "ws-1", Name: "One", Status: "running", UpdatedAt: "2025-04-06T20:52:47Z"},
			{ID: "ws-2", Name: "Two", Status: "suspended", UpdatedAt: "2025-04-06T20:52:47Z"},
		},
	}

	v := newWorkspacesViewWithClient(manager)
	v.workspaces = manager.workspaces

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	wv := updated.(*WorkspacesView)
	require.NotNil(t, cmd)
	updated, refreshCmd := wv.Update(cmd())
	wv = updated.(*WorkspacesView)
	require.NotNil(t, refreshCmd)
	assert.Equal(t, "ws-1", manager.suspendWorkspaceID)

	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'j'})
	wv = updated.(*WorkspacesView)
	updated, cmd = wv.Update(tea.KeyPressMsg{Code: 's'})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)
	updated, refreshCmd = wv.Update(cmd())
	wv = updated.(*WorkspacesView)
	require.NotNil(t, refreshCmd)
	assert.Equal(t, "ws-2", manager.resumeWorkspaceID)

	updated, _ = wv.Update(tea.KeyPressMsg{Code: 'f'})
	wv = updated.(*WorkspacesView)
	require.True(t, wv.prompt.active)
	wv.prompt.input.SetValue("fork-name")

	updated, cmd = wv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wv = updated.(*WorkspacesView)
	require.NotNil(t, cmd)
	updated, refreshCmd = wv.Update(cmd())
	wv = updated.(*WorkspacesView)
	require.NotNil(t, refreshCmd)
	assert.Equal(t, "ws-2", manager.forkWorkspaceID)
	assert.Equal(t, "fork-name", manager.forkWorkspaceName)
}

package workspace_test

import (
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/workspace"
)

func TestNew_DefaultState(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)

	state := m.State()
	if state.ActiveRunCount != 0 {
		t.Errorf("expected ActiveRunCount=0, got %d", state.ActiveRunCount)
	}
	if state.PendingApprovalCount != 0 {
		t.Errorf("expected PendingApprovalCount=0, got %d", state.PendingApprovalCount)
	}
	if state.ConnectionState != workspace.ConnectionUnknown {
		t.Errorf("expected ConnectionUnknown, got %d", state.ConnectionState)
	}
}

func TestInit_ReturnsNonNilCmd(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected Init to return a non-nil cmd")
	}
}

func TestUpdate_UpdatesState(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)

	newState := workspace.WorkspaceState{
		ActiveRunCount:       3,
		PendingApprovalCount: 2,
		ConnectionState:      workspace.ConnectionConnected,
	}
	updated, cmd := m.Update(workspace.WorkspaceUpdateMsg{State: newState})
	if cmd == nil {
		t.Error("expected Update to return a non-nil poll cmd")
	}
	got := updated.State()
	if got.ActiveRunCount != 3 {
		t.Errorf("expected ActiveRunCount=3, got %d", got.ActiveRunCount)
	}
	if got.PendingApprovalCount != 2 {
		t.Errorf("expected PendingApprovalCount=2, got %d", got.PendingApprovalCount)
	}
	if got.ConnectionState != workspace.ConnectionConnected {
		t.Errorf("expected ConnectionConnected, got %d", got.ConnectionState)
	}
}

func TestUpdate_DisconnectedState(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)

	newState := workspace.WorkspaceState{
		ConnectionState: workspace.ConnectionDisconnected,
	}
	updated, _ := m.Update(workspace.WorkspaceUpdateMsg{State: newState})
	if updated.State().ConnectionState != workspace.ConnectionDisconnected {
		t.Errorf("expected ConnectionDisconnected")
	}
}

func TestConnectionState_Constants(t *testing.T) {
	// Verify the zero value is ConnectionUnknown (important for zero-value Model).
	if workspace.ConnectionUnknown != 0 {
		t.Errorf("ConnectionUnknown should be zero value, got %d", workspace.ConnectionUnknown)
	}
}

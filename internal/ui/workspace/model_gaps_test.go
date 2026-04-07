package workspace_test

import (
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/workspace"
	"github.com/stretchr/testify/assert"
)

func TestUpdate_OverwritesPreviousState(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)

	// First update: connected with some counts.
	first := workspace.WorkspaceState{
		ActiveRunCount:       5,
		PendingApprovalCount: 3,
		ConnectionState:      workspace.ConnectionConnected,
	}
	m, _ = m.Update(workspace.WorkspaceUpdateMsg{State: first})
	assert.Equal(t, 5, m.State().ActiveRunCount)

	// Second update: completely different values.
	second := workspace.WorkspaceState{
		ActiveRunCount:       0,
		PendingApprovalCount: 0,
		ConnectionState:      workspace.ConnectionDisconnected,
	}
	m, _ = m.Update(workspace.WorkspaceUpdateMsg{State: second})

	got := m.State()
	assert.Equal(t, 0, got.ActiveRunCount, "ActiveRunCount should reflect latest update")
	assert.Equal(t, 0, got.PendingApprovalCount, "PendingApprovalCount should reflect latest update")
	assert.Equal(t, workspace.ConnectionDisconnected, got.ConnectionState, "ConnectionState should reflect latest update")
}

func TestState_ZeroValueBeforeAnyUpdate(t *testing.T) {
	client := smithers.NewClient()
	m := workspace.New(client)

	// Without any Update calls, State should return the zero-value struct,
	// which means ConnectionUnknown and all counts at zero.
	got := m.State()
	assert.Equal(t, workspace.ConnectionUnknown, got.ConnectionState)
	assert.Equal(t, 0, got.ActiveRunCount)
	assert.Equal(t, 0, got.PendingApprovalCount)

	// Verify that WorkspaceState{} is identical to what State() returns,
	// confirming it behaves as a proper zero value.
	assert.Equal(t, workspace.WorkspaceState{}, got)
}

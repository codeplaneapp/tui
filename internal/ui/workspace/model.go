// Package workspace provides a lightweight polling model that supplies live
// Smithers runtime metrics (active run count, pending approval count,
// connection state) to the TUI header and status bar.
package workspace

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// ConnectionState describes the Smithers server connection status.
type ConnectionState int

const (
	// ConnectionUnknown is the initial state before the first poll completes.
	ConnectionUnknown ConnectionState = iota
	// ConnectionConnected means the server responded successfully.
	ConnectionConnected
	// ConnectionDisconnected means the last poll failed or the server was unreachable.
	ConnectionDisconnected
)

// WorkspaceState holds live Smithers runtime metrics.
type WorkspaceState struct {
	ActiveRunCount       int
	PendingApprovalCount int
	ConnectionState      ConnectionState
}

// WorkspaceUpdateMsg is emitted by the polling loop when the workspace state
// has been refreshed.  The root model should handle this message and schedule
// the next poll by returning the command from Model.Update.
type WorkspaceUpdateMsg struct {
	State WorkspaceState
}

// Model owns the polling loop and the most-recently-fetched WorkspaceState.
type Model struct {
	client   *smithers.Client
	state    WorkspaceState
	interval time.Duration
}

// New creates a new workspace Model with a 10-second polling interval.
func New(client *smithers.Client) *Model {
	return &Model{
		client:   client,
		interval: 10 * time.Second,
	}
}

// Init starts the polling loop.  Call this once from the root model's Init.
func (m *Model) Init() tea.Cmd {
	return m.poll()
}

// poll returns a tea.Cmd that waits for the polling interval, fetches data from
// the Smithers client, and emits a WorkspaceUpdateMsg.
func (m *Model) poll() tea.Cmd {
	interval := m.interval
	client := m.client
	return tea.Tick(interval, func(_ time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		approvals, approvalErr := client.ListPendingApprovals(ctx)

		state := WorkspaceState{
			PendingApprovalCount: len(approvals),
		}

		if approvalErr == nil {
			state.ConnectionState = ConnectionConnected
		} else {
			state.ConnectionState = ConnectionDisconnected
		}

		return WorkspaceUpdateMsg{State: state}
	})
}

// Update handles a WorkspaceUpdateMsg, updates the cached state, and
// schedules the next poll.  Call this from the root model's Update.
func (m *Model) Update(msg WorkspaceUpdateMsg) (*Model, tea.Cmd) {
	m.state = msg.State
	return m, m.poll()
}

// State returns the most-recently-fetched WorkspaceState.
// Before the first poll completes, all counts are zero and ConnectionState is
// ConnectionUnknown.
func (m *Model) State() WorkspaceState {
	return m.state
}

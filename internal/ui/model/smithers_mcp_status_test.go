package model

import (
	"testing"

	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/stretchr/testify/require"
)

func TestSmithersMCPStatusFromStates_EmptyMap(t *testing.T) {
	t.Parallel()

	connected, name, tools := smithersMCPStatusFromStates(nil)
	require.False(t, connected)
	require.Empty(t, name)
	require.Zero(t, tools)

	connected, name, tools = smithersMCPStatusFromStates(map[string]mcp.ClientInfo{})
	require.False(t, connected)
	require.Empty(t, name)
	require.Zero(t, tools)
}

func TestSmithersMCPStatusFromStates_ExactCanonicalName_Connected(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"smithers": {
			Name:   "codeplane",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 12},
		},
	}

	connected, name, tools := smithersMCPStatusFromStates(states)
	require.True(t, connected)
	require.Equal(t, "codeplane", name)
	require.Equal(t, 12, tools)
}

func TestSmithersMCPStatusFromStates_ExactCanonicalName_Disconnected(t *testing.T) {
	t.Parallel()

	for _, state := range []mcp.State{mcp.StateStarting, mcp.StateError, mcp.StateDisabled} {
		state := state
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			states := map[string]mcp.ClientInfo{
				"smithers": {Name: "codeplane", State: state},
			}
			connected, name, tools := smithersMCPStatusFromStates(states)
			require.False(t, connected, "state=%s should be disconnected", state)
			require.Equal(t, "codeplane", name)
			require.Zero(t, tools)
		})
	}
}

func TestSmithersMCPStatusFromStates_AlternateName(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"smithers-orchestrator": {
			Name:   "smithers-orchestrator",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 7},
		},
	}

	connected, name, tools := smithersMCPStatusFromStates(states)
	require.True(t, connected)
	require.Equal(t, "smithers-orchestrator", name)
	require.Equal(t, 7, tools)
}

func TestSmithersMCPStatusFromStates_FuzzyMatchContainsSmithers(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"my-smithers-server": {
			Name:   "my-smithers-server",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 3},
		},
	}

	connected, name, tools := smithersMCPStatusFromStates(states)
	require.True(t, connected)
	require.Equal(t, "my-smithers-server", name)
	require.Equal(t, 3, tools)
}

func TestSmithersMCPStatusFromStates_CanonicalTakesPriorityOverAlternate(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"smithers": {
			Name:   "codeplane",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 10},
		},
		"smithers-orchestrator": {
			Name:   "smithers-orchestrator",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 5},
		},
	}

	connected, name, tools := smithersMCPStatusFromStates(states)
	require.True(t, connected)
	require.Equal(t, "codeplane", name)
	require.Equal(t, 10, tools)
}

func TestSmithersMCPStatusFromStates_NoSmithersKey(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"github": {Name: "github", State: mcp.StateConnected, Counts: mcp.Counts{Tools: 5}},
		"docker": {Name: "docker", State: mcp.StateConnected, Counts: mcp.Counts{Tools: 3}},
	}

	connected, name, tools := smithersMCPStatusFromStates(states)
	require.False(t, connected)
	require.Empty(t, name)
	require.Zero(t, tools)
}

func TestUpdateSmithersStatusMCP_NilExisting(t *testing.T) {
	t.Parallel()

	states := map[string]mcp.ClientInfo{
		"smithers": {
			Name:   "codeplane",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 8},
		},
	}

	result := updateSmithersStatusMCP(nil, states)
	require.NotNil(t, result)
	require.True(t, result.MCPConnected)
	require.Equal(t, "codeplane", result.MCPServerName)
	require.Equal(t, 8, result.MCPToolCount)
	require.Zero(t, result.ActiveRuns)
	require.Zero(t, result.PendingApprovals)
}

func TestUpdateSmithersStatusMCP_PreservesRunFields(t *testing.T) {
	t.Parallel()

	existing := &SmithersStatus{
		ActiveRuns:       3,
		PendingApprovals: 1,
		MCPConnected:     false,
		MCPServerName:    "",
		MCPToolCount:     0,
	}

	states := map[string]mcp.ClientInfo{
		"smithers": {
			Name:   "codeplane",
			State:  mcp.StateConnected,
			Counts: mcp.Counts{Tools: 15},
		},
	}

	result := updateSmithersStatusMCP(existing, states)
	require.Equal(t, existing, result, "should return same pointer")
	require.True(t, result.MCPConnected)
	require.Equal(t, "codeplane", result.MCPServerName)
	require.Equal(t, 15, result.MCPToolCount)
	// Run fields must be unchanged.
	require.Equal(t, 3, result.ActiveRuns)
	require.Equal(t, 1, result.PendingApprovals)
}

func TestUpdateSmithersStatusMCP_DisconnectedWhenNoSmithersKey(t *testing.T) {
	t.Parallel()

	existing := &SmithersStatus{
		MCPConnected:  true,
		MCPServerName: "codeplane",
		MCPToolCount:  5,
	}

	result := updateSmithersStatusMCP(existing, map[string]mcp.ClientInfo{})
	require.False(t, result.MCPConnected)
	require.Empty(t, result.MCPServerName)
	require.Zero(t, result.MCPToolCount)
}

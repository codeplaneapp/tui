package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetLSPStates replaces the package-level lspStates with a fresh map so
// tests don't interfere with each other.
func resetLSPStates(t *testing.T) {
	t.Helper()
	old := lspStates
	lspStates = csync.NewMap[string, LSPClientInfo]()
	t.Cleanup(func() { lspStates = old })
}

func TestUpdateLSPState_StoresAndPublishes(t *testing.T) {
	resetLSPStates(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := lspBroker.Subscribe(ctx)

	updateLSPState("gopls", lsp.StateReady, nil, nil, 5)

	info, ok := GetLSPState("gopls")
	require.True(t, ok)
	assert.Equal(t, "gopls", info.Name)
	assert.Equal(t, lsp.StateReady, info.State)
	assert.Nil(t, info.Error)
	assert.Equal(t, 5, info.DiagnosticCount)
	assert.False(t, info.ConnectedAt.IsZero(), "ConnectedAt should be set when state is Ready")

	// Verify the event was published.
	ev := <-ch
	assert.Equal(t, pubsub.UpdatedEvent, ev.Type)
	assert.Equal(t, LSPEventStateChanged, ev.Payload.Type)
	assert.Equal(t, "gopls", ev.Payload.Name)
	assert.Equal(t, lsp.StateReady, ev.Payload.State)
}

func TestUpdateLSPState_PreservesConnectedAtOnNonReadyTransition(t *testing.T) {
	resetLSPStates(t)

	// Set initial state as Ready (which records ConnectedAt).
	updateLSPState("gopls", lsp.StateReady, nil, nil, 0)
	info, _ := GetLSPState("gopls")
	originalConnectedAt := info.ConnectedAt
	require.False(t, originalConnectedAt.IsZero())

	// Transition to a non-ready state; ConnectedAt should be preserved.
	updateLSPState("gopls", lsp.StateStarting, nil, nil, 0)
	info, _ = GetLSPState("gopls")
	assert.Equal(t, lsp.StateStarting, info.State)
	assert.Equal(t, originalConnectedAt, info.ConnectedAt, "ConnectedAt should be preserved across non-ready transitions")
}

func TestUpdateLSPDiagnostics_UpdatesExistingState(t *testing.T) {
	resetLSPStates(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := lspBroker.Subscribe(ctx)

	// Seed the state first.
	updateLSPState("gopls", lsp.StateReady, nil, nil, 0)
	<-ch // drain the state_changed event

	updateLSPDiagnostics("gopls", 12)

	info, ok := GetLSPState("gopls")
	require.True(t, ok)
	assert.Equal(t, 12, info.DiagnosticCount)

	ev := <-ch
	assert.Equal(t, LSPEventDiagnosticsChanged, ev.Payload.Type)
	assert.Equal(t, 12, ev.Payload.DiagnosticCount)
}

func TestUpdateLSPDiagnostics_NoOpForUnknownClient(t *testing.T) {
	resetLSPStates(t)

	// Calling diagnostics update on an unknown name should be a no-op.
	updateLSPDiagnostics("unknown", 10)

	_, ok := GetLSPState("unknown")
	assert.False(t, ok, "No state should be stored for unknown client")
}

func TestGetLSPStates_ReturnsCopyOfAll(t *testing.T) {
	resetLSPStates(t)

	updateLSPState("gopls", lsp.StateReady, nil, nil, 3)
	updateLSPState("rust-analyzer", lsp.StateStarting, nil, nil, 0)

	states := GetLSPStates()
	assert.Len(t, states, 2)
	assert.Contains(t, states, "gopls")
	assert.Contains(t, states, "rust-analyzer")
	assert.Equal(t, 3, states["gopls"].DiagnosticCount)
}

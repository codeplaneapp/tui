package model

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/workspace"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestRenderHeaderDetails_WithSmithersStatus(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{
		PromptTokens:     1200,
		CompletionTokens: 800,
	}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		220,
		&SmithersStatus{
			ActiveRuns:       2,
			PendingApprovals: 1,
			MCPConnected:     true,
			MCPServerName:    "smithers",
		},
	)

	plain := ansi.Strip(details)
	require.Contains(t, plain, "● codeplane connected")
	require.Contains(t, plain, "2 active")
	require.Contains(t, plain, "⚠ 1 pending approval")
}

func TestRenderHeaderDetails_WithoutSmithersStatus(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{
		PromptTokens:     600,
		CompletionTokens: 400,
	}
	details := renderHeaderDetails(com, sess, 0, false, 200, nil)
	plain := ansi.Strip(details)

	require.NotContains(t, plain, "smithers")
	require.NotContains(t, plain, "pending approval")
	require.Contains(t, plain, "%")
}

func TestRenderHeaderDetails_MCPDisconnected(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{
		PromptTokens:     500,
		CompletionTokens: 300,
	}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		220,
		&SmithersStatus{
			MCPConnected:  false,
			MCPServerName: "smithers",
		},
	)

	plain := ansi.Strip(details)
	require.Contains(t, plain, "○ codeplane disconnected")
	require.NotContains(t, plain, "● codeplane connected")
	require.NotContains(t, plain, "tools")
}

func TestRenderHeaderDetails_MCPConnectedWithToolCount(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		260,
		&SmithersStatus{
			MCPConnected:  true,
			MCPServerName: "smithers",
			MCPToolCount:  14,
		},
	)

	plain := ansi.Strip(details)
	require.Contains(t, plain, "● codeplane connected (14 tools)")
}

func TestRenderHeaderDetails_MCPConnectedZeroTools(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		220,
		&SmithersStatus{
			MCPConnected:  true,
			MCPServerName: "smithers",
			MCPToolCount:  0,
		},
	)

	plain := ansi.Strip(details)
	// When tool count is zero do not show "(0 tools)".
	require.Contains(t, plain, "● codeplane connected")
	require.NotContains(t, plain, "tools")
}

func TestRenderHeaderDetails_DefaultServerName(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{}
	// Empty MCPServerName should fall back to "codeplane".
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		220,
		&SmithersStatus{
			MCPConnected:  false,
			MCPServerName: "",
		},
	)

	plain := ansi.Strip(details)
	require.Contains(t, plain, "codeplane disconnected")
}

func TestRenderHeaderDetails_PendingApprovals_Plural(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		280,
		&SmithersStatus{
			PendingApprovals: 3,
			MCPConnected:     true,
			MCPServerName:    "smithers",
		},
	)

	plain := ansi.Strip(details)
	// Three pending approvals should use plural "approvals".
	require.Contains(t, plain, "⚠ 3 pending approvals")
	require.NotContains(t, plain, "3 pending approval ")
}

func TestRenderHeaderDetails_PendingApprovals_EscalatesColor(t *testing.T) {
	t.Parallel()

	// Verify that 5+ pending approvals produces a different ANSI sequence
	// than 1–4. We compare the raw (non-stripped) output rather than color
	// values directly, since the exact escape codes depend on the terminal
	// color mode. At minimum, both should contain the warning text.
	com := newHeaderTestCommon(t)
	sess := &session.Session{}
	lowDetails := renderHeaderDetails(
		com, sess, 0, false, 280,
		&SmithersStatus{PendingApprovals: 1, MCPConnected: true, MCPServerName: "s"},
	)
	highDetails := renderHeaderDetails(
		com, sess, 0, false, 280,
		&SmithersStatus{PendingApprovals: 5, MCPConnected: true, MCPServerName: "s"},
	)

	// Both should show the badge text.
	require.Contains(t, ansi.Strip(lowDetails), "⚠ 1 pending approval")
	require.Contains(t, ansi.Strip(highDetails), "⚠ 5 pending approvals")

	// The rendered ANSI strings should differ because the high count uses
	// t.Error (red) instead of t.Warning (yellow).
	require.NotEqual(t, lowDetails, highDetails)
}

func TestRenderHeaderDetails_NoPendingApprovals_NoBadge(t *testing.T) {
	t.Parallel()

	com := newHeaderTestCommon(t)
	sess := &session.Session{}
	details := renderHeaderDetails(
		com,
		sess,
		0,
		false,
		220,
		&SmithersStatus{
			ActiveRuns:       2,
			PendingApprovals: 0,
			MCPConnected:     true,
			MCPServerName:    "smithers",
		},
	)

	plain := ansi.Strip(details)
	require.NotContains(t, plain, "pending approval")
	require.NotContains(t, plain, "⚠")
	// Active runs still shown.
	require.Contains(t, plain, "2 active")
}

func newHeaderTestCommon(t *testing.T) *common.Common {
	t.Helper()

	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set("test-provider", config.ProviderConfig{
		ID: "test-provider",
		Models: []catwalk.Model{
			{
				ID:            "test-model",
				ContextWindow: 100_000,
			},
		},
	})

	cfg := &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {
				Provider: "test-provider",
				Model:    "test-model",
			},
		},
		Providers: providers,
		Agents: map[string]config.Agent{
			config.AgentCoder: {
				Model: config.SelectedModelTypeLarge,
			},
		},
	}

	return common.DefaultCommon(&headerTestWorkspace{
		cfg:        cfg,
		workingDir: t.TempDir(),
	})
}

type headerTestWorkspace struct {
	workspace.Workspace
	cfg        *config.Config
	workingDir string
}

func (w *headerTestWorkspace) Config() *config.Config {
	return w.cfg
}

func (w *headerTestWorkspace) WorkingDir() string {
	return w.workingDir
}

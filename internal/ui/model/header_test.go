package model

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
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
	lspClients := csync.NewMap[string, *lsp.Client]()

	details := renderHeaderDetails(
		com,
		sess,
		lspClients,
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
	require.Contains(t, plain, "● smithers connected")
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
	lspClients := csync.NewMap[string, *lsp.Client]()

	details := renderHeaderDetails(com, sess, lspClients, false, 200, nil)
	plain := ansi.Strip(details)

	require.NotContains(t, plain, "smithers")
	require.NotContains(t, plain, "pending approval")
	require.Contains(t, plain, "%")
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

	store := &config.ConfigStore{}
	setUnexportedField(t, store, "config", cfg)
	setUnexportedField(t, store, "workingDir", t.TempDir())

	appInstance := &app.App{}
	setUnexportedField(t, appInstance, "config", store)

	return common.DefaultCommon(appInstance)
}

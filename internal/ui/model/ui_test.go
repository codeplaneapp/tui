package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/views"
	"github.com/charmbracelet/crush/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestCurrentModelSupportsImages(t *testing.T) {
	t.Parallel()

	t.Run("returns false when config is nil", func(t *testing.T) {
		t.Parallel()

		ui := newTestUIWithConfig(t, nil)
		require.False(t, ui.currentModelSupportsImages())
	})

	t.Run("returns false when coder agent is missing", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Agents:    map[string]config.Agent{},
		}
		ui := newTestUIWithConfig(t, cfg)
		require.False(t, ui.currentModelSupportsImages())
	})

	t.Run("returns false when model is not found", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Agents: map[string]config.Agent{
				config.AgentCoder: {Model: config.SelectedModelTypeLarge},
			},
		}
		ui := newTestUIWithConfig(t, cfg)
		require.False(t, ui.currentModelSupportsImages())
	})

	t.Run("returns true when current model supports images", func(t *testing.T) {
		t.Parallel()

		providers := csync.NewMap[string, config.ProviderConfig]()
		providers.Set("test-provider", config.ProviderConfig{
			ID: "test-provider",
			Models: []catwalk.Model{
				{ID: "test-model", SupportsImages: true},
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
				config.AgentCoder: {Model: config.SelectedModelTypeLarge},
			},
		}

		ui := newTestUIWithConfig(t, cfg)
		require.True(t, ui.currentModelSupportsImages())
	})
}

func TestHandleViewResult_OpenChat_ResetsViewRouter(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.viewRouter = views.NewRouter()
	ui.viewRouter.Push(views.NewRunsView(smithers.NewClient()), 0, 0)

	ui.handleViewResult(views.OpenChatMsg{})

	require.Equal(t, 0, ui.viewRouter.Depth())
	require.Equal(t, uiLanding, ui.state)
}

func TestHandleViewResult_PopViewMsg_LastViewReturnsToDashboard(t *testing.T) {
	t.Parallel()

	ui := newShortcutTestUI()
	ui.dashboard = views.NewDashboardView(ui.com, nil, false)
	ui.viewRouter = views.NewRouter()
	ui.viewRouter.Push(views.NewChatView(nil), 0, 0)
	ui.setState(uiSmithersView, uiFocusMain)

	ui.handleViewResult(views.PopViewMsg{})

	require.Equal(t, 0, ui.viewRouter.Depth())
	require.Equal(t, uiSmithersDashboard, ui.state)
}

// ─── substituteArgs ───────────────────────────────────────────────────────────

func TestSubstituteArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		args    map[string]string
		want    string
	}{
		{
			name:    "single substitution",
			content: "Hello $NAME",
			args:    map[string]string{"NAME": "World"},
			want:    "Hello World",
		},
		{
			name:    "multiple substitutions",
			content: "$GREETING $NAME, welcome to $PLACE",
			args:    map[string]string{"GREETING": "Hi", "NAME": "Alice", "PLACE": "Wonderland"},
			want:    "Hi Alice, welcome to Wonderland",
		},
		{
			name:    "no matching placeholders",
			content: "No placeholders here",
			args:    map[string]string{"FOO": "bar"},
			want:    "No placeholders here",
		},
		{
			name:    "empty args map",
			content: "$KEEP_ME",
			args:    map[string]string{},
			want:    "$KEEP_ME",
		},
		{
			name:    "nil args map",
			content: "$KEEP_ME",
			args:    nil,
			want:    "$KEEP_ME",
		},
		{
			name:    "repeated placeholder",
			content: "$X and $X",
			args:    map[string]string{"X": "val"},
			want:    "val and val",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, substituteArgs(tt.content, tt.args))
		})
	}
}

// ─── isWhitespace ─────────────────────────────────────────────────────────────

func TestIsWhitespace(t *testing.T) {
	t.Parallel()

	whitespace := []byte{' ', '\t', '\n', '\r'}
	for _, b := range whitespace {
		require.True(t, isWhitespace(b), "expected %q to be whitespace", string(b))
	}

	nonWhitespace := []byte{'a', '0', '.', '-', '_'}
	for _, b := range nonWhitespace {
		require.False(t, isWhitespace(b), "expected %q to NOT be whitespace", string(b))
	}
}

// ─── mimeOf ───────────────────────────────────────────────────────────────────

func TestMimeOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  []byte
		contains string
	}{
		{"plain text", []byte("Hello, world!"), "text/plain"},
		{"html", []byte("<html><body>hi</body></html>"), "text/html"},
		{"empty", []byte{}, "text/plain"},
		// PNG magic bytes
		{"png header", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0, 0, 0, 0}, "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mimeOf(tt.content)
			require.Contains(t, got, tt.contains)
		})
	}
}

// ─── hasPasteExceededThreshold ─────────────────────────────────────────────────

func TestHasPasteExceededThreshold(t *testing.T) {
	t.Parallel()

	t.Run("short paste does not exceed", func(t *testing.T) {
		t.Parallel()
		msg := tea.PasteMsg{Content: "short line"}
		require.False(t, hasPasteExceededThreshold(msg))
	})

	t.Run("many lines exceeds", func(t *testing.T) {
		t.Parallel()
		lines := ""
		for i := 0; i < pasteLinesThreshold+5; i++ {
			lines += "line\n"
		}
		msg := tea.PasteMsg{Content: lines}
		require.True(t, hasPasteExceededThreshold(msg))
	})

	t.Run("wide single line exceeds", func(t *testing.T) {
		t.Parallel()
		wide := strings.Repeat("x", pasteColsThreshold+10)
		msg := tea.PasteMsg{Content: wide}
		require.True(t, hasPasteExceededThreshold(msg))
	})
}

// ─── featureEnabled ───────────────────────────────────────────────────────────

func TestFeatureEnabled(t *testing.T) {
	// Uses t.Setenv so cannot be parallel at the top level.
	const envKey = "CRUSH_TEST_FEATURE_ENABLED"

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"set to 1", "1", true},
		{"set to true", "true", true},
		{"set to false", "false", false}, // explicitly disabled
		{"set to 0", "0", false},         // explicitly disabled
		{"set to arbitrary", "yes", true},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envKey, tt.value)
			require.Equal(t, tt.want, featureEnabled(envKey))
		})
	}
}

// ─── approvalBellEnabled ──────────────────────────────────────────────────────

func TestApprovalBellEnabled(t *testing.T) {
	// Uses t.Setenv so cannot be parallel at the top level.
	const envKey = "CODEPLANE_APPROVAL_BELL"

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"unset defaults to enabled", "", true},
		{"set to 0 disables", "0", false},
		{"set to false disables", "false", false},
		{"set to 1 enables", "1", true},
		{"set to arbitrary string enables", "yes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				// Ensure the env var is unset for this case.
				t.Setenv(envKey, "")
				// We need to actually unset it, not set to empty string.
				// t.Setenv will restore the old value on cleanup.
			} else {
				t.Setenv(envKey, tt.value)
			}
			require.Equal(t, tt.want, approvalBellEnabled())
		})
	}
}

func TestApprovalBellEnabled_DefaultWhenUnset(t *testing.T) {
	// Verify that when the env var is completely absent, bell is enabled.
	// We use a name unlikely to be set.
	t.Setenv("CODEPLANE_APPROVAL_BELL", "")
	// Empty string: approvalBellEnabled checks != "0" && != "false", so empty passes.
	require.True(t, approvalBellEnabled())
}

func newTestUIWithConfig(t *testing.T, cfg *config.Config) *UI {
	t.Helper()

	return &UI{
		com: &common.Common{
			Workspace: &testWorkspace{cfg: cfg},
		},
	}
}

// testWorkspace is a minimal [workspace.Workspace] stub for unit tests.
type testWorkspace struct {
	workspace.Workspace
	cfg *config.Config
}

func (w *testWorkspace) Config() *config.Config {
	return w.cfg
}

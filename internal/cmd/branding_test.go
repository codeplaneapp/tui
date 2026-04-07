package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandBrandingUsesCodeplane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "root", cmd: rootCmd},
		{name: "tui", cmd: tuiCmd},
		{name: "dirs", cmd: dirsCmd},
		{name: "models", cmd: modelsCmd},
		{name: "run", cmd: runCmd},
		{name: "login", cmd: loginCmd},
		{name: "update-providers", cmd: updateProvidersCmd},
		{name: "logs", cmd: logsCmd},
		{name: "schema", cmd: schemaCmd},
		{name: "projects", cmd: projectsCmd},
		{name: "server", cmd: serverCmd},
		{name: "session", cmd: sessionCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			text := strings.ToLower(strings.Join([]string{
				tt.cmd.Use,
				tt.cmd.Short,
				tt.cmd.Long,
				tt.cmd.Example,
			}, "\n"))

			require.NotContains(t, text, "smithers")
			require.NotContains(t, text, "smithers-tui")
			require.NotContains(t, text, "'crush'")
			require.NotContains(t, text, "`crush`")
			require.NotContains(t, text, " crush ")
		})
	}
}

func TestShouldEnableMetricsHonorsCodeplaneEnv(t *testing.T) {
	t.Setenv("CODEPLANE_DISABLE_METRICS", "true")
	t.Setenv("SMITHERS_TUI_DISABLE_METRICS", "")
	t.Setenv("CRUSH_DISABLE_METRICS", "")

	require.False(t, shouldEnableMetrics(&config.Config{
		Options: &config.Options{},
	}))
}

func TestGenerateConfigSchemaUsesCodeplaneBranding(t *testing.T) {
	t.Parallel()

	bts, err := generateConfigSchema()
	require.NoError(t, err)

	text := strings.ToLower(string(bts))
	require.Contains(t, text, "https://charm.land/codeplane.json")
	require.NotContains(t, text, "https://charm.land/crush.json")
	require.NotContains(t, text, "crush-config")
	require.NotContains(t, text, "smithers-tui")
}

func TestShouldLogCommandSkipsSchema(t *testing.T) {
	t.Parallel()

	require.False(t, shouldLogCommand(schemaCmd))
	require.True(t, shouldLogCommand(runCmd))
}

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmithersMCPDefaultNotInjectedOutsideSmithersMode(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")

	_, exists := cfg.MCP[SmithersMCPName]
	assert.False(t, exists, "smithers MCP should not be injected outside smithers mode")
	assert.Nil(t, cfg.Smithers)
}

func TestSmithersMCPDefaultInjectedInSmithersMode(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Smithers: &SmithersConfig{},
	}
	cfg.setDefaults(t.TempDir(), "")

	mcpCfg, exists := cfg.MCP[SmithersMCPName]
	require.True(t, exists, "smithers MCP should be injected in smithers mode")
	assert.Equal(t, MCPStdio, mcpCfg.Type)
	assert.Equal(t, "smithers", mcpCfg.Command)
	assert.Equal(t, []string{"--mcp"}, mcpCfg.Args)
	assert.False(t, mcpCfg.Disabled)
}

func TestSmithersMCPUserOverrideRespected(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Smithers: &SmithersConfig{},
		MCP: map[string]MCPConfig{
			SmithersMCPName: {
				Type:    MCPStdio,
				Command: "/custom/path/smithers",
				Args:    []string{"--mcp", "--verbose"},
			},
		},
	}
	cfg.setDefaults(t.TempDir(), "")

	mcpCfg := cfg.MCP[SmithersMCPName]
	assert.Equal(t, "/custom/path/smithers", mcpCfg.Command,
		"user-provided config should not be overwritten")
	assert.Equal(t, []string{"--mcp", "--verbose"}, mcpCfg.Args)
}

func TestSmithersMCPUserDisabledRespected(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Smithers: &SmithersConfig{},
		MCP: map[string]MCPConfig{
			SmithersMCPName: {
				Type:     MCPStdio,
				Command:  "smithers",
				Args:     []string{"--mcp"},
				Disabled: true,
			},
		},
	}
	cfg.setDefaults(t.TempDir(), "")

	mcpCfg := cfg.MCP[SmithersMCPName]
	assert.True(t, mcpCfg.Disabled,
		"user should be able to disable Smithers MCP")
}

func TestDefaultDisabledToolsNotAppliedOutsideSmithersMode(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")

	assert.Nil(t, cfg.Options.DisabledTools)
}

func TestDefaultDisabledToolsAppliedInSmithersMode(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Smithers: &SmithersConfig{},
	}
	cfg.setDefaults(t.TempDir(), "")

	assert.Contains(t, cfg.Options.DisabledTools, "sourcegraph")
}

func TestDefaultDisabledToolsUserOverrideRespected(t *testing.T) {
	t.Parallel()

	customDisabled := []string{"bash", "edit"}
	cfg := &Config{
		Smithers: &SmithersConfig{},
		Options: &Options{
			DisabledTools: customDisabled,
		},
	}
	cfg.setDefaults(t.TempDir(), "")

	assert.Equal(t, customDisabled, cfg.Options.DisabledTools,
		"user-provided disabled tools should not be overwritten")
}

func TestDefaultSmithersMCPConfig(t *testing.T) {
	t.Parallel()

	mcpCfg := DefaultSmithersMCPConfig()
	assert.Equal(t, MCPStdio, mcpCfg.Type)
	assert.Equal(t, "smithers", mcpCfg.Command)
	assert.Equal(t, []string{"--mcp"}, mcpCfg.Args)
}

func TestDefaultDisabledTools(t *testing.T) {
	t.Parallel()

	tools := DefaultDisabledTools()
	assert.Contains(t, tools, "sourcegraph")
}

func TestSmithersDefaultsUseWorkspacePath(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	smithersDir := filepath.Join(projectDir, ".smithers")
	nestedDir := filepath.Join(projectDir, "nested", "workspace")

	require.NoError(t, os.MkdirAll(filepath.Join(smithersDir, "workflows"), 0o755))
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))

	cfg := &Config{}
	cfg.setDefaults(nestedDir, "")

	require.NotNil(t, cfg.Smithers)
	assert.Equal(t, filepath.Join(smithersDir, "smithers.db"), cfg.Smithers.DBPath)
	assert.Equal(t, filepath.Join(smithersDir, "workflows"), cfg.Smithers.WorkflowDir)

	mcpCfg, exists := cfg.MCP[SmithersMCPName]
	require.True(t, exists)
	assert.Equal(t, "smithers", mcpCfg.Command)
	assert.Contains(t, cfg.Options.DisabledTools, "sourcegraph")
}

func TestSmithersNotTriggeredByBareDataDirectory(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	// Create .codeplane without workflows/ — this is a normal project, not Smithers
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, defaultDataDirectory), 0o755))

	cfg := &Config{}
	cfg.setDefaults(projectDir, "")

	assert.Nil(t, cfg.Smithers, "bare .codeplane should not enable smithers mode")
	_, exists := cfg.MCP[SmithersMCPName]
	assert.False(t, exists, "smithers MCP should not be injected for bare data dir")
}

func TestSmithersTriggeredByDataDirWithWorkflows(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, defaultDataDirectory, "workflows"), 0o755))

	cfg := &Config{}
	cfg.setDefaults(projectDir, "")

	require.NotNil(t, cfg.Smithers, ".codeplane/workflows should enable smithers mode")
	_, exists := cfg.MCP[SmithersMCPName]
	assert.True(t, exists)
}

func TestSmithersDetectionUsesWorkingDirNotProcessCWD(t *testing.T) {
	processDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(processDir, ".smithers"), 0o755))
	t.Chdir(processDir)

	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")

	_, exists := cfg.MCP[SmithersMCPName]
	assert.False(t, exists, "process cwd should not enable smithers mode")
	assert.Nil(t, cfg.Smithers)
	assert.Nil(t, cfg.Options.DisabledTools)
}

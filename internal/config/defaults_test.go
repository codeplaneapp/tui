package config

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmithersMCPDefaultInjected(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")

	mcpCfg, exists := cfg.MCP[SmithersMCPName]
	require.True(t, exists, "smithers MCP should be injected by default")
	assert.Equal(t, MCPStdio, mcpCfg.Type)
	assert.Equal(t, "smithers", mcpCfg.Command)
	assert.Equal(t, []string{"--mcp"}, mcpCfg.Args)
	assert.False(t, mcpCfg.Disabled)
}

func TestSmithersMCPUserOverrideRespected(t *testing.T) {
	t.Parallel()
	cfg := &Config{
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

func TestDefaultDisabledToolsApplied(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.setDefaults(t.TempDir(), "")
	assert.Contains(t, cfg.Options.DisabledTools, "sourcegraph")
}

func TestDefaultDisabledToolsUserOverrideRespected(t *testing.T) {
	t.Parallel()
	customDisabled := []string{"bash", "edit"}
	cfg := &Config{
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

func TestSmithersMCPFullWorkflowWithUserConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "smithers-tui.json")

	cfg := &Config{
		MCP: make(map[string]MCPConfig),
		Options: &Options{
			DisabledTools: nil,
		},
	}
	store := &ConfigStore{
		config:         cfg,
		globalDataPath: configPath,
		resolver:       NewShellVariableResolver(env.New()),
	}

	// Apply defaults
	cfg.setDefaults(tmpDir, "")

	// Verify Smithers MCP was injected
	mcpCfg, exists := cfg.MCP[SmithersMCPName]
	require.True(t, exists)
	assert.Equal(t, MCPStdio, mcpCfg.Type)
	assert.Equal(t, "smithers", mcpCfg.Command)

	// Verify disabled tools were applied
	assert.Contains(t, cfg.Options.DisabledTools, "sourcegraph")

	// Verify we can access store methods
	require.NotNil(t, store)
}

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinatorSmithersAgentDispatch verifies that resolveAgent returns the
// Smithers agent when a Smithers config block is present, and that the resulting
// agent has the expected tool restrictions and MCP allowlist.
func TestCoordinatorSmithersAgentDispatch(t *testing.T) {
	t.Parallel()

	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)

	cfg.Config().Smithers = &config.SmithersConfig{
		WorkflowDir: ".smithers/workflows",
	}
	cfg.SetupAgents()

	coord := &coordinator{}
	agentName, agentCfg, err := coord.resolveAgent(cfg)
	require.NoError(t, err)

	assert.Equal(t, config.AgentSmithers, agentName)
	assert.Equal(t, config.AgentSmithers, agentCfg.ID)

	// Tool filtering: sourcegraph and multiedit must be excluded.
	assert.NotContains(t, agentCfg.AllowedTools, "sourcegraph",
		"sourcegraph should be excluded from the Smithers agent")
	assert.NotContains(t, agentCfg.AllowedTools, "multiedit",
		"multiedit should be excluded from the Smithers agent")

	// Core tools that Smithers still needs should be present.
	assert.Contains(t, agentCfg.AllowedTools, "bash")
	assert.Contains(t, agentCfg.AllowedTools, "view")

	// MCP allowlist: the "smithers" MCP server key must be present.
	require.NotNil(t, agentCfg.AllowedMCP,
		"Smithers agent AllowedMCP must not be nil")
	_, hasMCPEntry := agentCfg.AllowedMCP[config.SmithersMCPName]
	assert.True(t, hasMCPEntry,
		"AllowedMCP must contain the key %q", config.SmithersMCPName)
}

// TestCoordinatorCoderFallbackWhenNoSmithersConfig verifies that resolveAgent
// returns the coder agent when no Smithers config block is present.
func TestCoordinatorCoderFallbackWhenNoSmithersConfig(t *testing.T) {
	t.Parallel()

	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)

	cfg.Config().Smithers = nil
	cfg.SetupAgents()

	coord := &coordinator{}
	agentName, agentCfg, err := coord.resolveAgent(cfg)
	require.NoError(t, err)

	assert.Equal(t, config.AgentCoder, agentName)
	assert.Equal(t, config.AgentCoder, agentCfg.ID)

	// The coder agent has no restricted MCP allowlist.
	assert.Nil(t, agentCfg.AllowedMCP,
		"coder agent should not have a restricted AllowedMCP map")
}

// TestSmithersSystemPromptContainsDomainInstructions verifies that when
// the Smithers agent is resolved, the resulting system prompt contains
// Smithers-specific instructions and references the correct MCP server name.
func TestSmithersSystemPromptContainsDomainInstructions(t *testing.T) {
	t.Parallel()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	store.Config().Options.ContextPaths = nil
	store.Config().Options.SkillsPaths = nil

	p, err := smithersPrompt(
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("darwin"),
		prompt.WithWorkingDir("/tmp/smithers-test"),
		prompt.WithSmithersMode(".smithers/workflows", config.SmithersMCPName),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)

	// Must identify as the Smithers assistant.
	assert.Contains(t, rendered, "Smithers TUI assistant")

	// MCP tool names must use the configured server name constant.
	expectedToolPrefix := "mcp_" + config.SmithersMCPName + "_"
	assert.Contains(t, rendered, expectedToolPrefix+"runs_list")
	assert.Contains(t, rendered, expectedToolPrefix+"workflow_list")
	assert.Contains(t, rendered, expectedToolPrefix+"approve")

	// Must include workflow directory context.
	assert.Contains(t, rendered, "Workflow directory: .smithers/workflows")

	// Must not include coder-specific sections.
	assert.NotContains(t, rendered, "<editing_files>")
}

// TestCoderSystemPromptAbsent verifies that when the coder agent is used
// (no Smithers config), the system prompt does not include Smithers instructions.
func TestCoderSystemPromptAbsent(t *testing.T) {
	t.Parallel()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	store.Config().Options.ContextPaths = nil
	store.Config().Options.SkillsPaths = nil

	p, err := coderPrompt(
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("darwin"),
		prompt.WithWorkingDir("/tmp/coder-test"),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)

	// Must not include Smithers-specific role text.
	assert.NotContains(t, rendered, "Smithers TUI assistant",
		"coder prompt must not contain Smithers domain instructions")
}

// TestSmithersAgentAllowedMCPUsesConstant verifies the AllowedMCP key in the
// Smithers agent config uses config.SmithersMCPName rather than any hardcoded
// string literal.
func TestSmithersAgentAllowedMCPUsesConstant(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Options: &config.Options{
			DisabledTools: []string{},
		},
		Smithers: &config.SmithersConfig{
			WorkflowDir: ".smithers/workflows",
		},
	}
	cfg.SetupAgents()

	smithersAgent, ok := cfg.Agents[config.AgentSmithers]
	require.True(t, ok)

	_, hasMCPEntry := smithersAgent.AllowedMCP[config.SmithersMCPName]
	assert.True(t, hasMCPEntry,
		"AllowedMCP key must equal config.SmithersMCPName (%q)", config.SmithersMCPName)

	// Sanity-check: the constant value is "smithers".
	assert.Equal(t, "smithers", config.SmithersMCPName)
}

// TestSmithersPromptContextFilesLoaded verifies that when context files
// exist in the working directory, they are rendered under the <memory> section.
func TestSmithersPromptContextFilesLoaded(t *testing.T) {
	t.Parallel()

	// Create a temp working directory with a context file.
	workDir := t.TempDir()
	contextContent := "Smithers workspace: production cluster."
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "smithers-tui.md"),
		[]byte(contextContent),
		0o644,
	))

	store, err := config.Init(workDir, "", false)
	require.NoError(t, err)
	// Override context paths so only our known file is loaded.
	store.Config().Options.ContextPaths = []string{"smithers-tui.md"}
	store.Config().Options.SkillsPaths = nil

	p, err := smithersPrompt(
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("darwin"),
		prompt.WithWorkingDir(workDir),
		prompt.WithSmithersMode(".smithers/workflows", config.SmithersMCPName),
	)
	require.NoError(t, err)

	rendered, err := p.Build(context.Background(), "mock", "model", store)
	require.NoError(t, err)

	// Context file must appear under <memory>.
	assert.Contains(t, rendered, "<memory>")
	// The path attribute contains the absolute path, so check for the filename suffix.
	assert.Contains(t, rendered, `smithers-tui.md">`)
	assert.Contains(t, rendered, contextContent)
}

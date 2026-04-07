package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_AgentIDs(t *testing.T) {
	cfg := &Config{
		Options: &Options{
			DisabledTools: []string{},
		},
	}
	cfg.SetupAgents()

	t.Run("Coder agent should have correct ID", func(t *testing.T) {
		coderAgent, ok := cfg.Agents[AgentCoder]
		require.True(t, ok)
		assert.Equal(t, AgentCoder, coderAgent.ID, "Coder agent ID should be '%s'", AgentCoder)
	})

	t.Run("Task agent should have correct ID", func(t *testing.T) {
		taskAgent, ok := cfg.Agents[AgentTask]
		require.True(t, ok)
		assert.Equal(t, AgentTask, taskAgent.ID, "Task agent ID should be '%s'", AgentTask)
	})

	t.Run("Smithers agent should not exist without smithers config", func(t *testing.T) {
		_, ok := cfg.Agents[AgentSmithers]
		assert.False(t, ok)
	})
}

func TestConfig_AgentIDsWithSmithers(t *testing.T) {
	cfg := &Config{
		Options: &Options{
			DisabledTools: []string{},
		},
		Smithers: &SmithersConfig{
			WorkflowDir: ".smithers/workflows",
		},
	}
	cfg.SetupAgents()

	t.Run("Smithers agent should have correct ID", func(t *testing.T) {
		smithersAgent, ok := cfg.Agents[AgentSmithers]
		require.True(t, ok)
		assert.Equal(t, AgentSmithers, smithersAgent.ID, "Smithers agent ID should be '%s'", AgentSmithers)
		assert.Equal(t, "Codeplane", smithersAgent.Name)
		assert.NotContains(t, smithersAgent.AllowedTools, "sourcegraph")
		assert.NotContains(t, smithersAgent.AllowedTools, "multiedit")
		require.NotNil(t, smithersAgent.AllowedMCP)
		_, hasMCP := smithersAgent.AllowedMCP["smithers"]
		assert.True(t, hasMCP, "Smithers agent should allow smithers MCP")
	})
}

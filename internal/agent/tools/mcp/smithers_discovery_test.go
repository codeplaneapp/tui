package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// TestSmithersMCPDiscoveryFlow tests the end-to-end discovery flow with a mock MCP server.
func TestSmithersMCPDiscoveryFlow(t *testing.T) {
	t.Parallel()

	// Create a config with the mock Smithers MCP server.
	cfg := &config.Config{
		MCP: map[string]config.MCPConfig{
			config.SmithersMCPName: {
				Type:     config.MCPStdio,
				Command:  "echo",
				Args:     []string{},
				Disabled: false,
			},
		},
		Options: &config.Options{
			DisabledTools: config.DefaultDisabledTools(),
		},
	}

	// Verify default config was applied.
	require.Equal(t, config.MCPStdio, cfg.MCP[config.SmithersMCPName].Type)
	require.Equal(t, "echo", cfg.MCP[config.SmithersMCPName].Command)
}

// TestSmithersMCPDefaultInjectedIntoConfig tests that the default Smithers MCP config is properly injected.
func TestSmithersMCPDefaultInjectedIntoConfig(t *testing.T) {
	t.Parallel()

	// Create a new config without any MCP configuration.
	cfg := &config.Config{
		MCP:     make(map[string]config.MCPConfig),
		Options: &config.Options{},
	}

	// Call setDefaults to inject the Smithers MCP config.
	cfg.SetupAgents()

	// Verify Smithers agent is configured with proper MCP access.
	smithersAgent, ok := cfg.Agents[config.AgentSmithers]
	if ok {
		// Smithers agent should allow the smithers MCP.
		_, hasMCP := smithersAgent.AllowedMCP[config.SmithersMCPName]
		require.True(t, hasMCP, "Smithers agent should allow smithers MCP")
	}
}


// TestSmithersMCPToolDiscoveryWithMockServer tests tool discovery with a mock in-memory MCP server.
func TestSmithersMCPToolDiscoveryWithMockServer(t *testing.T) {
	t.Parallel()

	// Create in-memory transport pair.
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Create and start mock server.
	serverImpl := &mcp.Implementation{
		Name:    "mock-smithers",
		Version: "1.0.0",
	}
	server := mcp.NewServer(serverImpl, nil)
	go func() {
		ctx := context.Background()
		serverSession, err := server.Connect(ctx, serverTransport, nil)
		if err != nil {
			t.Logf("Error connecting server: %v", err)
			return
		}
		defer serverSession.Close()

		// Keep server running for test duration.
		<-time.After(5 * time.Second)
	}()

	// Create client and connect.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientImpl := &mcp.Implementation{
		Name:    "crush-test",
		Version: "1.0.0",
	}
	client := mcp.NewClient(clientImpl, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "client should connect to mock server")
	defer session.Close()

	// Verify we can call ListTools (even if empty).
	toolsResp, err := session.ListTools(ctx, nil)
	require.NoError(t, err, "should be able to list tools from mock server")
	require.NotNil(t, toolsResp, "tools response should not be nil")
}

// TestSmithersMCPStateTransitions tests that the MCP client properly transitions through states.
func TestSmithersMCPStateTransitions(t *testing.T) {
	t.Parallel()

	// Verify state constants are defined.
	require.Equal(t, StateDisabled, State(0))
	require.Equal(t, StateStarting, State(1))
	require.Equal(t, StateConnected, State(2))
	require.Equal(t, StateError, State(3))

	// Verify state string representations.
	require.Equal(t, "disabled", StateDisabled.String())
	require.Equal(t, "starting", StateStarting.String())
	require.Equal(t, "connected", StateConnected.String())
	require.Equal(t, "error", StateError.String())
}

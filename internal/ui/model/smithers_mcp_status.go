package model

import (
	"strings"

	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/config"
)

// smithersMCPStatusFromStates resolves the workflow MCP connection status from
// the provided MCP state map. It applies a stable name-resolution order:
//  1. exact match on config.SmithersMCPName ("smithers")
//  2. exact match on "smithers-orchestrator"
//  3. first key whose name contains "smithers" (case-insensitive)
//
// Returns (connected, serverName, toolCount). When no workflow MCP entry is
// found all three zero values are returned.
func smithersMCPStatusFromStates(states map[string]mcp.ClientInfo) (connected bool, serverName string, toolCount int) {
	if len(states) == 0 {
		return false, "", 0
	}

	// Priority 1: exact canonical name.
	if info, ok := states[config.SmithersMCPName]; ok {
		return info.State == mcp.StateConnected, info.Name, info.Counts.Tools
	}

	// Priority 2: well-known alternate name.
	const alternateName = "smithers-orchestrator"
	if info, ok := states[alternateName]; ok {
		return info.State == mcp.StateConnected, info.Name, info.Counts.Tools
	}

	// Priority 3: first key containing "smithers" (case-insensitive).
	for key, info := range states {
		if strings.Contains(strings.ToLower(key), "smithers") {
			return info.State == mcp.StateConnected, info.Name, info.Counts.Tools
		}
	}

	return false, "", 0
}

// updateSmithersStatusMCP updates the MCP-related fields of the provided
// SmithersStatus in-place using the given MCP state map.  If status is nil a
// new SmithersStatus is returned; otherwise the existing pointer is mutated and
// returned as-is.
func updateSmithersStatusMCP(existing *SmithersStatus, states map[string]mcp.ClientInfo) *SmithersStatus {
	connected, serverName, toolCount := smithersMCPStatusFromStates(states)
	if existing == nil {
		return &SmithersStatus{
			MCPConnected:  connected,
			MCPServerName: serverName,
			MCPToolCount:  toolCount,
		}
	}
	existing.MCPConnected = connected
	existing.MCPServerName = serverName
	existing.MCPToolCount = toolCount
	return existing
}

package config

import (
	"os/exec"
)

const SmithersMCPName = "smithers"

// DefaultSmithersMCPConfig returns the default MCP configuration for
// the Smithers server. It uses stdio transport to spawn `smithers --mcp`.
func DefaultSmithersMCPConfig() MCPConfig {
	return MCPConfig{
		Type:    MCPStdio,
		Command: "smithers",
		Args:    []string{"--mcp"},
	}
}

// DefaultDisabledTools returns tools that are disabled by default in
// Smithers TUI context (not relevant for workflow operations).
func DefaultDisabledTools() []string {
	return []string{
		"sourcegraph",
	}
}

// IsSmithersCLIAvailable checks if the smithers binary is on PATH.
func IsSmithersCLIAvailable() bool {
	_, err := exec.LookPath("smithers")
	return err == nil
}

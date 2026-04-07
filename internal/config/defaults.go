package config

import (
	"os/exec"
)

const SmithersMCPName = "smithers"

// DefaultSmithersMCPConfig returns the default MCP configuration for
// the workflow server. It uses stdio transport to spawn `smithers --mcp`.
func DefaultSmithersMCPConfig() MCPConfig {
	return MCPConfig{
		Type:    MCPStdio,
		Command: "smithers",
		Args:    []string{"--mcp"},
	}
}

// DefaultDisabledTools returns tools that are disabled by default in the
// workflow context.
func DefaultDisabledTools() []string {
	return []string{
		"sourcegraph",
	}
}

// IsSmithersCLIAvailable checks if the smithers binary is on PATH.
func IsSmithersCLIAvailable() bool {
	if _, err := exec.LookPath("codeplane"); err == nil {
		return true
	}
	_, err := exec.LookPath("smithers")
	return err == nil
}

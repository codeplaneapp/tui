// Package main implements a minimal mock Smithers MCP server for use in E2E
// tests.  It registers a small set of fake workflow tools so that the TUI can
// display a non-zero tool count in the header when connected.
//
// Optional environment variables:
//   - MOCK_MCP_STARTUP_DELAY_MS — milliseconds to sleep before accepting the
//     first connection (defaults to 0).  Use this to exercise the
//     disconnected→connected transition in tests.
package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if delayStr := os.Getenv("MOCK_MCP_STARTUP_DELAY_MS"); delayStr != "" {
		if ms, err := strconv.Atoi(delayStr); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "smithers", Title: "Smithers Mock MCP"}, nil)

	// Register fake workflow tools so the TUI shows a non-zero tool count.
	for _, tool := range []struct{ name, desc string }{
		{"list_workflows", "List all available Smithers workflows"},
		{"run_workflow", "Trigger a Smithers workflow by name"},
		{"get_run_status", "Retrieve the status of a specific Smithers run"},
	} {
		tool := tool
		mcp.AddTool(server, &mcp.Tool{Name: tool.name, Description: tool.desc},
			func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "mock response"}},
				}, nil, nil
			},
		)
	}

	// Run until the parent process (the TUI) closes stdin.
	_ = server.Run(context.Background(), &mcp.StdioTransport{})
}

# Smithers MCP connection status happy-path recording.
# This tape verifies that:
# 1. Smithers TUI starts successfully and displays the SMITHERS branding
# 2. The compact header shows Smithers MCP connection status (connected or disconnected)
# 3. When smithers --mcp is available the header shows "smithers connected"
# 4. The TUI remains usable regardless of MCP state
Output tests/vhs/output/smithers-mcp-connection-status.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch with fixture config that wires smithers --mcp as the MCP server.
Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures/smithers-mcp-connection-status.json SMITHERS_TUI_GLOBAL_DATA=/tmp/smithers-tui-mcp-status-vhs go run ."
Enter
Sleep 4s

# At this point the MCP handshake should have completed.
# The compact header should show either "smithers connected" (smithers on PATH)
# or "smithers disconnected" (smithers not on PATH).
Screenshot tests/vhs/output/smithers-mcp-connection-status.png

# Ask the agent about MCP tools to exercise the connected path.
Type "What Smithers MCP tools are available?"
Enter
Sleep 2s

Screenshot tests/vhs/output/smithers-mcp-connection-status-chat.png

Ctrl+c
Sleep 1s

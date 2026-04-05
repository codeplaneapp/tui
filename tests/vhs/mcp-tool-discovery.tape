# Smithers MCP tool discovery happy-path recording.
# This tape verifies that:
# 1. Smithers TUI starts successfully
# 2. MCP status shows Smithers connected (or error gracefully)
# 3. Agent can see and use MCP tools

Output tests/vhs/output/mcp-tool-discovery.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-mcp go run ."
Enter
Sleep 3s

# The MCP status area should show smithers connection state.
# If smithers binary is available, it should show "connected".
# If unavailable, it should show "error" but TUI remains usable.
Sleep 1s

# Ask the agent about available tools to verify MCP tool discovery.
Type "What MCP tools are available for managing workflows?"
Enter
Sleep 2s

Screenshot tests/vhs/output/mcp-tool-discovery.png

# Exit the TUI cleanly.
Ctrl+c
Sleep 1s

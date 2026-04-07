# Smithers MCP tool discovery happy-path recording.
# This tape verifies that:
# 1. Smithers TUI starts successfully
# 2. The Start Chat flow opens the built-in Smithers chat
# 3. The chat chrome shows Smithers MCP tool discovery state

Output tests/vhs/output/mcp-tool-discovery.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CODEPLANE_GLOBAL_CONFIG=tests/vhs/fixtures CODEPLANE_GLOBAL_DATA=/tmp/codeplane-vhs-mcp ./crush 2>/dev/null"
Enter
Sleep 5s

# Open the Start Chat flow, then launch the built-in Smithers chat.
Type "c"
Sleep 1s
Enter
Sleep 2s

Screenshot tests/vhs/output/mcp-tool-discovery.png
Sleep 1s

Ctrl+c
Sleep 1s
Type "y"
Sleep 1s

# Smithers default console and back-stack happy-path recording.
Output tests/vhs/output/chat-default-console.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CODEPLANE_GLOBAL_CONFIG=tests/vhs/fixtures CODEPLANE_GLOBAL_DATA=/tmp/codeplane-vhs-chat-default-console ./crush 2>/dev/null"
Enter
Sleep 5s

# Open the default Start Chat flow, then return to the overview root.
Enter
Sleep 1s
Escape
Sleep 1s

# Push the Runs view and pop back to the default console.
Ctrl+r
Sleep 1s
Escape
Sleep 2s

Screenshot tests/vhs/output/chat-default-console.png
Sleep 1s

Ctrl+c
Sleep 1s
Type "y"
Sleep 1s

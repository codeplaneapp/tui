# Smithers domain system prompt happy-path smoke recording.
Output tests/vhs/output/smithers-domain-system-prompt.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

Type "What tools can you use to manage workflows?"
Enter
Sleep 2s

Screenshot tests/vhs/output/smithers-domain-system-prompt.png

Ctrl+c
Sleep 1s

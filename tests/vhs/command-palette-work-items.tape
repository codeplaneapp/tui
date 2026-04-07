# Smithers command palette to work-items happy-path recording.
Output tests/vhs/output/command-palette-work-items.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CODEPLANE_GLOBAL_CONFIG=tests/vhs/fixtures CODEPLANE_GLOBAL_DATA=/tmp/codeplane-vhs-command-palette-work-items ./crush 2>/dev/null"
Enter
Sleep 5s

Ctrl+p
Sleep 1s
Type "items"
Sleep 1s
Enter
Sleep 2s

Screenshot tests/vhs/output/command-palette-work-items.png

Escape
Sleep 1s
Ctrl+c
Sleep 1s
Type "y"
Sleep 1s

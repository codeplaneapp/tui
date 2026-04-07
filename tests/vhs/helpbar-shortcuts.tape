# Smithers helpbar shortcuts happy-path smoke recording.
Output tests/vhs/output/helpbar-shortcuts.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CODEPLANE_GLOBAL_CONFIG=tests/vhs/fixtures CODEPLANE_GLOBAL_DATA=/tmp/codeplane-vhs-helpbar-shortcuts ./crush 2>/dev/null"
Enter
Sleep 5s

Ctrl+r
Sleep 1s
Escape
Sleep 1s
Ctrl+a
Sleep 1s
Escape
Sleep 1s
Ctrl+g
Sleep 1s

Screenshot tests/vhs/output/helpbar-shortcuts.png

Ctrl+c
Sleep 1s
Type "y"
Sleep 1s

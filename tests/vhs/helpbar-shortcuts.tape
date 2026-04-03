# Smithers helpbar shortcuts happy-path smoke recording.
Output tests/vhs/output/helpbar-shortcuts.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-helpbar-shortcuts go run ."
Enter
Sleep 3s

Ctrl+r
Sleep 1s
Ctrl+a
Sleep 1s
Ctrl+g
Sleep 1s

Screenshot tests/vhs/output/helpbar-shortcuts.png

Ctrl+c
Sleep 1s

# Smithers branding/status happy-path smoke recording.
Output tests/vhs/output/branding-status.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-branding-status go run ."
Enter
Sleep 3s

Ctrl+g
Sleep 1s

Screenshot tests/vhs/output/branding-status.png

Ctrl+c
Sleep 1s

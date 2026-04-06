Output tests/vhs/output/branding-status.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "SMITHERS_TUI_GLOBAL_CONFIG=tests/vhs/fixtures SMITHERS_TUI_GLOBAL_DATA=/tmp/crush-vhs-branding-status go run ."
Enter
Sleep 3s

Screenshot tests/vhs/output/branding-status.png

Ctrl+c
Sleep 1s

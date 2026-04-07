# Smithers branding/status happy-path smoke recording.
Output tests/vhs/output/branding-status.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CODEPLANE_GLOBAL_CONFIG=tests/vhs/fixtures CODEPLANE_GLOBAL_DATA=/tmp/codeplane-vhs-branding-status ./crush 2>/dev/null"
Enter
Sleep 5s

Screenshot tests/vhs/output/branding-status.png

Ctrl+c
Sleep 1s
Type "y"
Sleep 1s

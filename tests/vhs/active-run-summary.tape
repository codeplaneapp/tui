# active-run-summary.tape — records the Smithers header in zero-state
# (no active runs, blank run-summary segment).
# Set SMITHERS_TUI_E2E=1 and point apiUrl at a live server to record with real data.
Output tests/vhs/output/active-run-summary.gif

Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 5s

# Capture the SMITHERS header with zero-state (no active runs).
Screenshot tests/vhs/output/active-run-summary-startup.png

Ctrl+c
Sleep 500ms

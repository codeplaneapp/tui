# Memory Browser — happy-path smoke recording.
Output tests/vhs/output/memory-browser.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with VHS fixtures and test memory DB
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-memory SMITHERS_DB=tests/fixtures/memory-test.db go run ."
Enter
Sleep 3s

# Open command palette and navigate to memory browser
Ctrl+p
Sleep 500ms
Type "memory"
Sleep 500ms
Enter
Sleep 2s

# Memory browser should be visible with fact list
Screenshot tests/vhs/output/memory-browser-list.png

# Navigate down through facts
Down
Sleep 300ms
Down
Sleep 300ms

Screenshot tests/vhs/output/memory-browser-navigated.png

# Refresh
Type "r"
Sleep 1s

Screenshot tests/vhs/output/memory-browser-refreshed.png

# Return to previous view
Escape
Sleep 1s

Screenshot tests/vhs/output/memory-browser-back.png

Ctrl+c
Sleep 500ms

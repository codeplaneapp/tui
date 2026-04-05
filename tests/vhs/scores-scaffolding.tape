# scores-scaffolding.tape — Happy-path smoke recording for the Scores dashboard.
Output tests/vhs/output/scores-scaffolding.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with fixture DB and clean config/data dirs.
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-scores-scaffolding SMITHERS_DB_PATH=tests/vhs/fixtures/scores-test.db go run ."
Enter
Sleep 3s

# Open command palette.
Type "/"
Sleep 1s

# Filter to scores entry.
Type "scores"
Sleep 500ms

# Select Scores.
Enter
Sleep 2s

# Screenshot scores dashboard.
Screenshot tests/vhs/output/scores-scaffolding.png

# Refresh the dashboard.
Type "r"
Sleep 2s

# Return to chat.
Escape
Sleep 1s

Ctrl+c

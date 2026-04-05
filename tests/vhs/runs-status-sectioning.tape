# runs-status-sectioning.tape — records the runs dashboard with grouped sections
Output tests/vhs/output/runs-status-sectioning.gif
Set FontSize 14
Set Width 130
Set Height 40
Set Shell "bash"
Set Env CRUSH_GLOBAL_CONFIG tests/vhs/fixtures

Type "go run . --config tests/vhs/fixtures/crush.json"
Enter
Sleep 3s

# Open runs dashboard
Ctrl+R
Sleep 2s

# Navigate down (cursor skips headers, only lands on run rows)
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Up
Sleep 400ms

# Refresh
Type "r"
Sleep 2s

# Back to chat
Escape
Sleep 1s

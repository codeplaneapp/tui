Output tests/vhs/output/runs-realtime.gif
Set FontSize 14
Set Width 120
Set Height 40
Set Shell "bash"

# Start TUI with a running Smithers server.
Type "SMITHERS_API_URL=http://localhost:7331 go run ."
Enter
Sleep 3s

# Open the runs dashboard via Ctrl+R.
Ctrl+R
Sleep 2s

# "● Live" indicator should be visible in the header when the server is running.
# Status changes stream in automatically — no user input needed.
Sleep 8s

# Return to main view.
Escape
Sleep 1s

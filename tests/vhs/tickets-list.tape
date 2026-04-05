# Tickets list view happy-path smoke recording.
Output tests/vhs/output/tickets-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with VHS fixtures (includes seeded .smithers/tickets/ files)
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-tickets go run ."
Enter
Sleep 3s

# Open command palette and navigate to tickets view
Ctrl+p
Sleep 500ms
Type "tickets"
Sleep 500ms
Enter
Sleep 2s

# Ticket list should be visible with count header
Screenshot tests/vhs/output/tickets-list-loaded.png

# Navigate down through a few tickets
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms

Screenshot tests/vhs/output/tickets-list-navigated.png

# Jump to end (G key)
Type "G"
Sleep 500ms

Screenshot tests/vhs/output/tickets-list-end.png

# Jump back to top (g key)
Type "g"
Sleep 500ms

Screenshot tests/vhs/output/tickets-list-top.png

# Return to chat view
Escape
Sleep 1s

Screenshot tests/vhs/output/tickets-list-back.png

Ctrl+c
Sleep 1s

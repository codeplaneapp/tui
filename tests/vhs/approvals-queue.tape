# Approvals queue — happy path with pending approvals.
# Records the full lifecycle: open view, see populated queue, navigate, refresh, return.
Output tests/vhs/output/approvals-queue.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI (configure a real or mock Smithers API via SMITHERS_API_URL if available)
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-approvals go run ."
Enter
Sleep 3s

# Open the approvals view via Ctrl+A
Ctrl+a
Sleep 2s

# Approvals view should be visible
Screenshot tests/vhs/output/approvals-queue-loaded.png

# Navigate down through the list
Down
Sleep 300ms
Down
Sleep 300ms

Screenshot tests/vhs/output/approvals-queue-cursor.png

# Navigate back up
Up
Sleep 300ms

# Manual refresh with r key
Type "r"
Sleep 2s

Screenshot tests/vhs/output/approvals-queue-refreshed.png

# Return to chat view with Escape
Escape
Sleep 1s

Screenshot tests/vhs/output/approvals-queue-back.png

Ctrl+c
Sleep 1s

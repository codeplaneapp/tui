# Handoff: Debug ChangesView escape and d key

## Problem
In the Crush TUI (bubbletea v2), the ChangesView has two bugs:
1. **Pressing `d` on a change is a visual noop** — the code runs, `InstallPromptMsg` is received by root model, toast cmd is returned, but nothing renders on screen
2. **Pressing `esc` is a noop** — the key reaches `handleKeyPressMsg` with state=4 (uiSmithersView) and key="esc", but PopViewMsg never appears in the debug log, meaning the ChangesView's escape handler at line 500 is not being reached

## What we know from debug logging (via tmux)

Debug log output (`/tmp/crush-keys.log` with `CRUSH_DEBUG_KEYS=1`):
```
KEY state=5 key="6" focus=2        # dashboard, press 6 for Changes tab
KEY state=5 key="enter" focus=2     # dashboard, press Enter
MSG DashboardNavigateMsg view=changes  # navigates to ChangesView (works!)
KEY state=4 key="d" focus=2         # in ChangesView, press d
MSG InstallPromptMsg cmd=jjhub change diff --no-color 'xmnvlrl...'  # msg IS received
KEY state=4 key="esc" focus=2       # press esc — NO PopViewMsg follows
```

State 4 = `uiSmithersView`, State 5 = `uiSmithersDashboard`

## Bug 1: `d` key — InstallPromptMsg received but toast doesn't show

The `InstallPromptMsg` handler in `internal/ui/model/ui.go` (around line 1144) creates a `ShowToastMsg` cmd. But the toast never appears. Possible causes:
- `m.toasts` might be nil
- The toast ShowToastMsg might be swallowed by message forwarding before reaching the toast manager
- The toast manager's Update is called at the top of root Update (line ~628) but the ShowToastMsg is returned as a cmd that produces the msg on the NEXT Update cycle — need to verify the msg reaches the toast manager on that next cycle

## Bug 2: `esc` key — never reaches ChangesView escape handler

The key goes through `handleKeyPressMsg` → state switch → `uiSmithersView` case → `viewRouter.Update(msg)`. The router calls `ChangesView.Update()`. But the escape handler at `changes.go:500` is not reached.

**Likely cause**: Look at `changes.go:529-538`. After the key switch (498-528), if no case matches, the code falls through to `v.splitPane.Update(msg)` at line 531-538. The SplitPane's Update handles `tab`/`shift+tab` but forwards ALL other keys to the focused pane. The focused pane (changeListPane or changePreviewPane) might be consuming the escape key.

**BUT** — the switch at line 499 SHOULD match `"esc"` before we ever reach line 529. Unless `key.Matches` is failing for some reason, or the key arrives as something other than what we expect.

Debug logging was added at line 473: `debugLogView("ChangesView key=%q ...")` — but the `debugLogView` function doesn't exist yet (was about to add it when the session was interrupted). Add it and rebuild to see if the ChangesView even receives the key.

## Key files

- `internal/ui/model/ui.go` — root model, handleKeyPressMsg (line ~2160), main Update switch (line ~644), message forwarding (line ~1230)
- `internal/ui/views/changes.go` — ChangesView.Update (line 441), escape handler (line 499-500), d handler (line 526)
- `internal/ui/views/router.go` — Router.Update forwards to current view
- `internal/ui/components/splitpane.go` — SplitPane.Update handles tab, forwards other keys
- `internal/ui/diffnav/launch.go` — LaunchDiffnavWithCommand returns InstallPromptMsg when diffnav not installed
- `internal/ui/components/toast.go` — ShowToastMsg type and toast manager

## Debug helper already in place

`debugLog()` function in `internal/ui/model/ui.go` (around line 2148) writes to `/tmp/crush-keys.log` when `CRUSH_DEBUG_KEYS=1`.

Need to add equivalent `debugLogView()` in `internal/ui/views/changes.go` (partially added, function not defined yet).

## How to test (IMPORTANT)

You CANNOT run the TUI binary directly — bubbletea needs a real PTY. Use tmux:

```bash
# Build
go build -o tests/smithers-tui .

# Run in tmux with debug logging
rm -f /tmp/crush-keys.log
SESSION="test-$$"
tmux new-session -d -s "$SESSION" -x 120 -y 40 "export CRUSH_DEBUG_KEYS=1; ./tests/smithers-tui"
sleep 3

# Send keys
tmux send-keys -t "$SESSION" "6"     # Changes tab
sleep 1
tmux send-keys -t "$SESSION" Enter   # Open ChangesView
sleep 2
tmux send-keys -t "$SESSION" "d"     # Try diff
sleep 1
tmux send-keys -t "$SESSION" Escape  # Try escape
sleep 1

# Capture screen
tmux capture-pane -t "$SESSION" -p

# Read debug log
cat /tmp/crush-keys.log

# Cleanup
tmux kill-session -t "$SESSION"
```

## What to do

1. Add `debugLogView()` function to `changes.go` (same pattern as `debugLog` in ui.go)
2. Add debug logging at key points in ChangesView.Update to trace exactly where the key goes
3. Rebuild, run via tmux, read the log
4. Fix whatever is swallowing the keys
5. Verify fix via tmux
6. Write the fix as an e2e test using tmux (see CLAUDE.md for pattern)
7. Remove all debug logging when done

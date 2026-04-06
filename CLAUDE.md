# CLAUDE.md

## TUI E2E Testing

The TUI binary requires a real PTY to start (bubbletea v2 opens `/dev/tty`). Claude Code runs without a controlling terminal, so the binary crashes if launched directly or via pipes.

### How to run TUI E2E tests from Claude Code

Use `tmux` to allocate a real PTY:

```bash
# Build the binary first
go build -o tests/smithers-tui .

# Launch in a detached tmux session with a real PTY
SESSION="crush-e2e-$$"
tmux new-session -d -s "$SESSION" -x 120 -y 40 "./tests/smithers-tui"
sleep 3

# Capture what's on screen
tmux capture-pane -t "$SESSION" -p

# Send keystrokes
tmux send-keys -t "$SESSION" "6"        # press 6
tmux send-keys -t "$SESSION" Enter       # press Enter
tmux send-keys -t "$SESSION" Escape      # press Escape
tmux send-keys -t "$SESSION" "d"         # press d
tmux send-keys -t "$SESSION" C-c         # ctrl+c

# Capture screen after each action
sleep 1
tmux capture-pane -t "$SESSION" -p

# Clean up
tmux kill-session -t "$SESSION" 2>/dev/null
```

### Why tmux works

- `tmux new-session -d` allocates a fresh PTY via `openpty()` even when the parent process has no controlling terminal
- The child process (smithers-tui) gets a real `/dev/tty` so bubbletea starts normally
- `capture-pane -p` dumps the virtual screen buffer as plain text for assertions
- `send-keys` sends real terminal input through the PTY

### Why other approaches fail

- **Direct `exec.Command` with pipes**: bubbletea ignores stdin/stdout pipes and opens `/dev/tty` directly, which fails without a controlling terminal
- **`expect`**: Allocates a PTY but only captures sequential output, not cursor-addressed screen rendering (bubbletea v2 uses `Draw()` which writes to specific screen positions)
- **`script` command**: Fails with "Operation not supported on socket" when called from a subprocess
- **`node-pty`**: Fails with "posix_spawnp failed" in sandboxed environments
- **`pyte` (Python terminal emulator)**: Crashes on modern escape sequences bubbletea v2 emits

### Existing test frameworks

- `@microsoft/tui-test` (in `tests/`) — uses node-pty + xterm headless. Works in real terminals and CI but NOT from Claude Code's sandbox
- Go e2e tests (in `internal/e2e/`) — use pipe-based helpers that only work when the Go test process itself has a terminal

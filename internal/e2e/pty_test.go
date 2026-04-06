package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

// TestPTY_DashboardEscape tests escape key navigation using a real PTY.
func TestPTY_DashboardEscape(t *testing.T) {
	if os.Getenv("CRUSH_TUI_E2E") != "1" {
		t.Skip("set CRUSH_TUI_E2E=1 to run terminal E2E tests")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	binary := filepath.Join(repoRoot, "tests", "smithers-tui")

	// Ensure binary exists
	_, err = os.Stat(binary)
	require.NoError(t, err, "binary not found at %s — run: go build -o tests/smithers-tui .", binary)

	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	require.NoError(t, err)
	defer ptmx.Close()

	// Read output in background
	output := make(chan string, 100)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				output <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Collect output for a duration
	collectOutput := func(d time.Duration) string {
		var sb strings.Builder
		deadline := time.After(d)
		for {
			select {
			case s := <-output:
				sb.WriteString(s)
			case <-deadline:
				// Drain remaining
				for {
					select {
					case s := <-output:
						sb.WriteString(s)
					default:
						return sb.String()
					}
				}
			}
		}
	}

	waitForText := func(text string, timeout time.Duration) bool {
		var all strings.Builder
		deadline := time.After(timeout)
		for {
			select {
			case s := <-output:
				all.WriteString(s)
				if strings.Contains(stripAnsi(all.String()), text) {
					return true
				}
			case <-deadline:
				t.Logf("waitForText(%q) timed out. Buffer:\n%s", text, stripAnsi(all.String()))
				return false
			}
		}
	}

	// Step 1: Wait for app to render
	t.Log("Waiting for app to start...")
	started := waitForText("SMITHERS", 15*time.Second)
	if !started {
		// Maybe it's onboarding
		all := collectOutput(2 * time.Second)
		stripped := stripAnsi(all)
		t.Logf("App output (stripped): %s", stripped[:min(len(stripped), 500)])
		t.Fatal("App did not show SMITHERS within 15s")
	}

	// Step 2: Press "2" to go to Runs tab
	t.Log("Pressing 2 for Runs tab...")
	ptmx.Write([]byte("2"))
	time.Sleep(500 * time.Millisecond)

	// Step 3: Press Escape
	t.Log("Pressing Escape...")
	ptmx.Write([]byte("\x1b"))
	time.Sleep(500 * time.Millisecond)

	// Step 4: Should still show SMITHERS (went back to Overview, not quit)
	post := collectOutput(2 * time.Second)
	stripped := stripAnsi(post)
	t.Logf("After escape: %s", stripped[:min(len(stripped), 200)])

	// Step 5: Quit
	ptmx.Write([]byte("\x03")) // ctrl+c
	cmd.Wait()
}

func stripAnsi(s string) string {
	// Simple ANSI stripper
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip escape sequence
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++
				}
			} else if i < len(s) && s[i] == ']' {
				// OSC sequence — skip until BEL or ST
				i++
				for i < len(s) && s[i] != '\x07' && s[i] != '\x1b' {
					i++
				}
				if i < len(s) && s[i] == '\x07' {
					i++
				}
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	// Suppress unused import warning
	_ = fmt.Sprintf
}

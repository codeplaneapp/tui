package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	defaultWaitTimeout = 10 * time.Second
	pollInterval       = 100 * time.Millisecond
)

var ansiPattern = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type TUITestInstance struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	buffer *syncBuffer
}

func launchTUI(t *testing.T, args ...string) *TUITestInstance {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start tui: %v", err)
	}

	buf := &syncBuffer{}
	go func() { _, _ = io.Copy(buf, stdout) }()
	go func() { _, _ = io.Copy(buf, stderr) }()

	return &TUITestInstance{t: t, cmd: cmd, stdin: stdin, buffer: buf}
}

func (t *TUITestInstance) bufferText() string {
	out := ansiPattern.ReplaceAllString(t.buffer.String(), "")
	return strings.ReplaceAll(out, "\r", "")
}

func normalizeTerminalText(value string) string {
	builder := strings.Builder{}
	for _, r := range value {
		if r >= 0x2500 && r <= 0x257F {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(r)
	}
	return strings.Join(strings.Fields(builder.String()), "")
}

func (t *TUITestInstance) matchesText(expected string) bool {
	buf := t.bufferText()
	if strings.Contains(buf, expected) {
		return true
	}
	return strings.Contains(normalizeTerminalText(buf), normalizeTerminalText(expected))
}

func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if t.matchesText(text) {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("waitForText: %q not found within %s\nBuffer:\n%s", text, timeout, t.bufferText())
}

func (t *TUITestInstance) WaitForNoText(text string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !t.matchesText(text) {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("waitForNoText: %q still present after %s\nBuffer:\n%s", text, timeout, t.bufferText())
}

func (t *TUITestInstance) SendKeys(keys string) {
	if t.stdin == nil {
		return
	}
	_, _ = io.WriteString(t.stdin, keys)
}

func (t *TUITestInstance) Snapshot() string {
	return t.bufferText()
}

func (t *TUITestInstance) Terminate() {
	t.t.Helper()
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}

	_ = t.cmd.Process.Signal(os.Interrupt)
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- t.cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil && !errors.Is(err, exec.ErrNotFound) {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.t.Fatalf("wait process: %v", err)
			}
		}
	case <-time.After(2 * time.Second):
		_ = t.cmd.Process.Kill()
		_ = <-waitCh
	}
}

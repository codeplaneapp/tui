package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	defaultWaitTimeout = 10 * time.Second
	pollInterval       = 100 * time.Millisecond
)

var (
	ansiPattern = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)

	builtTUIBinaryOnce sync.Once
	builtTUIBinaryPath string
	builtTUIBinaryErr  error
)

type TUITestInstance struct {
	t       *testing.T
	session string
}

type tuiLaunchOptions struct {
	args         []string
	env          map[string]string
	pathPrefixes []string
	workingDir   string
}

type tmuxKeyToken struct {
	hex   bool
	value string
}

func launchTUI(t *testing.T, args ...string) *TUITestInstance {
	t.Helper()
	return launchTUIWithOptions(t, tuiLaunchOptions{args: args})
}

func launchTUIWithOptions(t *testing.T, opts tuiLaunchOptions) *TUITestInstance {
	t.Helper()

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for this e2e test")
	}

	binary := buildSharedTUIBinary(t)
	workingDir := opts.workingDir
	if workingDir == "" {
		workingDir = e2eRepoRoot(t)
	}

	env := mergeEnv(os.Environ(), opts.env)
	env = mergeEnv(env, map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
		"LANG":      "en_US.UTF-8",
	})
	if len(opts.pathPrefixes) > 0 {
		env = prependEnvPath(env, opts.pathPrefixes...)
	}

	scriptPath := filepath.Join(t.TempDir(), "launch.sh")
	script := buildLaunchScript(env, workingDir, append([]string{binary}, opts.args...))
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write launch script: %v", err)
	}

	session := fmt.Sprintf("crush-e2e-%d", time.Now().UnixNano())
	cmd := exec.Command("tmux", "new-session", "-d", "-s", session, "-x", "120", "-y", "40", scriptPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch tmux session: %v\n%s", err, output)
	}

	return &TUITestInstance{t: t, session: session}
}

func buildLaunchScript(env []string, workingDir string, argv []string) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		builder.WriteString("export ")
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(shellQuote(value))
		builder.WriteString("\n")
	}
	builder.WriteString("cd ")
	builder.WriteString(shellQuote(workingDir))
	builder.WriteString("\n")
	builder.WriteString(shellJoin(argv))
	builder.WriteString("\nstatus=$?\nprintf '\\n[crush exited: %s]\\n' \"$status\"\nsleep 3600\n")
	return builder.String()
}

func buildSharedTUIBinary(t *testing.T) string {
	t.Helper()

	builtTUIBinaryOnce.Do(func() {
		buildDir, err := os.MkdirTemp("", "crush-tui-e2e-*")
		if err != nil {
			builtTUIBinaryErr = err
			return
		}

		builtTUIBinaryPath = filepath.Join(buildDir, "crush-tui")
		cmd := exec.Command("go", "build", "-o", builtTUIBinaryPath, "./main.go")
		cmd.Dir = e2eRepoRoot(t)
		output, err := cmd.CombinedOutput()
		if err != nil {
			builtTUIBinaryErr = fmt.Errorf("build tui binary: %w\n%s", err, output)
		}
	})

	if builtTUIBinaryErr != nil {
		t.Fatalf("%v", builtTUIBinaryErr)
	}
	return builtTUIBinaryPath
}

func e2eRepoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func (t *TUITestInstance) bufferText() string {
	out := ansiPattern.ReplaceAllString(t.Snapshot(), "")
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
	t.t.Helper()

	for _, token := range parseKeyTokens(keys) {
		var cmd *exec.Cmd
		if token.hex {
			cmd = exec.Command("tmux", "send-keys", "-t", t.session, "-H", token.value)
		} else {
			cmd = exec.Command("tmux", "send-keys", "-t", t.session, token.value)
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.t.Fatalf("send keys %q: %v\n%s", token.value, err, output)
		}
	}
}

func (t *TUITestInstance) Snapshot() string {
	t.t.Helper()

	out, err := exec.Command("tmux", "capture-pane", "-t", t.session, "-p").CombinedOutput()
	if err != nil {
		t.t.Fatalf("capture pane: %v\n%s", err, out)
	}
	return strings.ReplaceAll(string(out), "\r", "")
}

func (t *TUITestInstance) Terminate() {
	t.t.Helper()
	_ = exec.Command("tmux", "kill-session", "-t", t.session).Run()
}

func openCommandsPalette(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	tui.SendKeys("\x10") // ctrl+p
	require.NoError(t, tui.WaitForText("Commands", 5*time.Second),
		"commands dialog must open; buffer:\n%s", tui.Snapshot())
}

func openStartChatFromDashboard(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	tui.SendKeys("\r")
	require.NoError(t, tui.WaitForText("MCPs", 10*time.Second),
		"start chat should open the landing view; buffer:\n%s", tui.Snapshot())
}

func parseKeyTokens(keys string) []tmuxKeyToken {
	data := []byte(keys)
	tokens := make([]tmuxKeyToken, 0, len(data))

	for i := 0; i < len(data); i++ {
		switch data[i] {
		case 0x1b:
			if i+2 < len(data) && data[i+1] == '[' {
				if keyName, ok := tmuxArrowKey(data[i+2]); ok {
					tokens = append(tokens, tmuxKeyToken{value: keyName})
					i += 2
					continue
				}
			}
			tokens = append(tokens, tmuxKeyToken{hex: true, value: "1b"})
		case '\r', '\n':
			tokens = append(tokens, tmuxKeyToken{hex: true, value: "0d"})
		case '\t':
			tokens = append(tokens, tmuxKeyToken{hex: true, value: "09"})
		case ' ':
			tokens = append(tokens, tmuxKeyToken{value: "Space"})
		default:
			if keyHex, ok := tmuxControlKey(data[i]); ok {
				tokens = append(tokens, tmuxKeyToken{hex: true, value: keyHex})
				continue
			}
			tokens = append(tokens, tmuxKeyToken{value: string([]byte{data[i]})})
		}
	}
	return tokens
}

func tmuxArrowKey(code byte) (string, bool) {
	switch code {
	case 'A':
		return "Up", true
	case 'B':
		return "Down", true
	case 'C':
		return "Right", true
	case 'D':
		return "Left", true
	default:
		return "", false
	}
}

func tmuxControlKey(code byte) (string, bool) {
	switch {
	case code == 0x00:
		return "", false
	case code == 0x7f || code < 0x20:
		return fmt.Sprintf("%02x", code), true
	default:
		return "", false
	}
}

func mergeEnv(base []string, overrides map[string]string) []string {
	order := make([]string, 0, len(base)+len(overrides))
	values := make(map[string]string, len(base)+len(overrides))

	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}

	for key, value := range overrides {
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}

	merged := make([]string, 0, len(order))
	for _, key := range order {
		merged = append(merged, key+"="+values[key])
	}
	return merged
}

func prependEnvPath(env []string, prefixes ...string) []string {
	currentPath := envValue(env, "PATH")
	parts := make([]string, 0, len(prefixes)+1)
	for _, prefix := range prefixes {
		if prefix != "" {
			parts = append(parts, prefix)
		}
	}
	if currentPath != "" {
		parts = append(parts, currentPath)
	}
	return mergeEnv(env, map[string]string{
		"PATH": strings.Join(parts, string(os.PathListSeparator)),
	})
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func shellJoin(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

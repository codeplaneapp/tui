package e2e_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
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
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

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

	if binary := os.Getenv("CRUSH_E2E_BINARY"); binary != "" {
		return binary
	}

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

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve repo root: runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
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
	for _, variant := range textMatchVariants(expected) {
		if strings.Contains(buf, variant) {
			return true
		}
		if strings.Contains(normalizeTerminalText(buf), normalizeTerminalText(variant)) {
			return true
		}
	}
	return false
}

func textMatchVariants(expected string) []string {
	variants := []string{expected}

	replacements := []struct {
		old  string
		news []string
	}{
		{old: "CRUSH", news: []string{"CODEPLANE", "SMITHERS"}},
		{old: "WORK ITEMS", news: []string{"Tickets"}},
		{old: "No local tickets found.", news: []string{"No tickets found."}},
		{old: "No local tickets found", news: []string{"No tickets found"}},
		{old: "No local tickets", news: []string{"No tickets found"}},
	}

	for _, replacement := range replacements {
		current := append([]string(nil), variants...)
		for _, variant := range current {
			if !strings.Contains(variant, replacement.old) {
				continue
			}
			for _, next := range replacement.news {
				candidate := strings.ReplaceAll(variant, replacement.old, next)
				if !containsString(variants, candidate) {
					variants = append(variants, candidate)
				}
			}
		}
	}

	return variants
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func (t *TUITestInstance) WaitForAnyText(texts []string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, text := range texts {
			if t.matchesText(text) {
				return nil
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("waitForAnyText: none of %q found within %s\nBuffer:\n%s", texts, timeout, t.bufferText())
}

func (t *TUITestInstance) SendKeys(keys string) {
	t.t.Helper()

	for _, b := range []byte(keys) {
		var cmd *exec.Cmd
		switch b {
		case '\x1b':
			cmd = exec.Command("tmux", "send-keys", "-t", t.session, "Escape")
		case '\r', '\n':
			cmd = exec.Command("tmux", "send-keys", "-t", t.session, "Enter")
		default:
			cmd = exec.Command("tmux", "send-keys", "-t", t.session, "-H", fmt.Sprintf("%02x", b))
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.t.Fatalf("send key %q: %v\n%s", b, err, output)
		}
	}
}

func (t *TUITestInstance) SendText(text string) {
	t.t.Helper()

	cmd := exec.Command("tmux", "send-keys", "-t", t.session, "-l", text)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.t.Fatalf("send text %q: %v\n%s", text, err, output)
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
	require.NoError(t, tui.WaitForText("Switch Model", 5*time.Second),
		"commands dialog must finish rendering before filtering; buffer:\n%s", tui.Snapshot())
}

func openStartChatFromDashboard(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	if err := tui.WaitForAnyText([]string{
		"MCPs",
		"Ready for instructions",
		"Ready...",
	}, 300*time.Millisecond); err == nil {
		return
	}

	if err := tui.WaitForText("Choose how you want to chat in this workspace.", 300*time.Millisecond); err != nil {
		tui.SendKeys("c")
	}
	if err := tui.WaitForText("Choose how you want to chat in this workspace.", 5*time.Second); err == nil {
		tui.SendKeys("\r")
	}
	require.NoError(t, tui.WaitForAnyText([]string{
		"MCPs",
		"Ready for instructions",
		"Ready...",
	}, 10*time.Second),
		"start chat should open the landing view; buffer:\n%s", tui.Snapshot())

	ensureEditorFocus(t, tui)
}

func ensureEditorFocus(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	if strings.Contains(tui.bufferText(), "focus editor") {
		tui.SendKeys("\t")
		time.Sleep(150 * time.Millisecond)
	}
}

func returnToDashboard(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	for range 3 {
		if strings.Contains(tui.bufferText(), "At a Glance") {
			if strings.Contains(tui.bufferText(), "New Chat") ||
				strings.Contains(tui.bufferText(), "Run Workflow") ||
				strings.Contains(tui.bufferText(), "Initialize Smithers") {
				return
			}
		}
		tui.SendKeys("\x1b")
		time.Sleep(200 * time.Millisecond)
	}

	require.NoError(t, tui.WaitForText("At a Glance", 10*time.Second))
	require.NoError(t, tui.WaitForAnyText([]string{
		"New Chat",
		"Run Workflow",
		"Initialize Smithers",
	}, 5*time.Second))
}

func mergeEnv(base []string, overrides map[string]string) []string {
	envMap := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		envMap[key] = value
	}
	for key, value := range overrides {
		envMap[key] = value
	}

	merged := make([]string, 0, len(envMap))
	for key, value := range envMap {
		merged = append(merged, key+"="+value)
	}
	return merged
}

func prependEnvPath(env []string, prefixes ...string) []string {
	envMap := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		envMap[key] = value
	}

	pathValue := envMap["PATH"]
	parts := make([]string, 0, len(prefixes)+1)
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		parts = append(parts, prefix)
	}
	if pathValue != "" {
		parts = append(parts, pathValue)
	}
	envMap["PATH"] = strings.Join(parts, string(os.PathListSeparator))

	merged := make([]string, 0, len(envMap))
	for key, value := range envMap {
		merged = append(merged, key+"="+value)
	}
	return merged
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

const configuredFixtureConfig = `{
  "options": {
    "disable_default_providers": true,
    "disable_notifications": true,
    "disable_provider_auto_update": true
  },
  "providers": {
    "fixture": {
      "name": "Fixture AI",
      "type": "openai",
      "base_url": "https://example.invalid/v1",
      "api_key": "fixture-key",
      "models": [
        {
          "id": "vision-alpha",
          "name": "Vision Alpha",
          "cost_per_1m_in": 0,
          "cost_per_1m_out": 0,
          "cost_per_1m_in_cached": 0,
          "cost_per_1m_out_cached": 0,
          "context_window": 128000,
          "default_max_tokens": 4096,
          "can_reason": true,
          "reasoning_levels": ["low", "medium", "high"],
          "default_reasoning_effort": "medium",
          "supports_attachments": true,
          "options": {}
        },
        {
          "id": "reason-mini",
          "name": "Reason Mini",
          "cost_per_1m_in": 0,
          "cost_per_1m_out": 0,
          "cost_per_1m_in_cached": 0,
          "cost_per_1m_out_cached": 0,
          "context_window": 64000,
          "default_max_tokens": 2048,
          "can_reason": true,
          "reasoning_levels": ["low", "medium", "high"],
          "default_reasoning_effort": "medium",
          "supports_attachments": true,
          "options": {}
        }
      ]
    }
  },
  "models": {
    "large": {
      "provider": "fixture",
      "model": "vision-alpha",
      "reasoning_effort": "medium"
    },
    "small": {
      "provider": "fixture",
      "model": "reason-mini",
      "reasoning_effort": "medium"
    }
  }
}`

const onboardingFixtureConfig = `{
  "options": {
    "disable_notifications": true,
    "disable_provider_auto_update": true
  },
  "providers": {
    "openai": {
      "disable": true
    },
    "synthetic": {
      "disable": true
    },
    "gemini": {
      "disable": true
    },
    "azure": {
      "disable": true
    },
    "bedrock": {
      "disable": true
    },
    "vertexai": {
      "disable": true
    },
    "xai": {
      "disable": true
    },
    "zai": {
      "disable": true
    },
    "zhipu": {
      "disable": true
    },
    "zhipu-coding": {
      "disable": true
    },
    "groq": {
      "disable": true
    },
    "openrouter": {
      "disable": true
    },
    "cerebras": {
      "disable": true
    },
    "venice": {
      "disable": true
    },
    "chutes": {
      "disable": true
    },
    "huggingface": {
      "disable": true
    },
    "aihubmix": {
      "disable": true
    },
    "kimi-coding": {
      "disable": true
    },
    "copilot": {
      "disable": true
    },
    "vercel": {
      "disable": true
    },
    "minimax": {
      "disable": true
    },
    "minimax-china": {
      "disable": true
    },
    "qiniucloud": {
      "disable": true
    },
    "avian": {
      "disable": true
    }
  }
}`

const (
	fixtureLargeModelName = "Vision Alpha"
	fixtureSmallModelName = "Reason Mini"
)

type tuiFixture struct {
	configDir  string
	dataDir    string
	workingDir string
	envVars    map[string]string
}

type seededSession struct {
	title    string
	messages []string
}

func newConfiguredFixture(t *testing.T) tuiFixture {
	t.Helper()

	fixture := tuiFixture{
		configDir:  t.TempDir(),
		dataDir:    t.TempDir(),
		workingDir: t.TempDir(),
		envVars:    offlineProviderEnv(),
	}
	writeGlobalConfig(t, fixture.configDir, configuredFixtureConfig)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.workingDir, "AGENTS.md"), []byte("fixture context\n"), 0o644))
	return fixture
}

func newOnboardingFixture(t *testing.T) tuiFixture {
	t.Helper()

	fixture := tuiFixture{
		configDir:  t.TempDir(),
		dataDir:    t.TempDir(),
		workingDir: t.TempDir(),
		envVars:    offlineProviderEnv(),
	}
	writeGlobalConfig(t, fixture.configDir, onboardingFixtureConfig)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.workingDir, "AGENTS.md"), []byte("fixture context\n"), 0o644))
	return fixture
}

func (f tuiFixture) env() map[string]string {
	envVars := map[string]string{
		"SMITHERS_TUI_GLOBAL_CONFIG": f.configDir,
		"SMITHERS_TUI_GLOBAL_DATA":   f.dataDir,
		"CRUSH_GLOBAL_CONFIG":        f.configDir,
		"CRUSH_GLOBAL_DATA":          f.dataDir,
	}
	for key, value := range f.envVars {
		envVars[key] = value
	}
	return envVars
}

func (f tuiFixture) workspaceDataDir() string {
	return filepath.Join(f.workingDir, ".smithers-tui")
}

func offlineProviderEnv() map[string]string {
	return map[string]string{
		"AIHUBMIX_API_KEY":      "",
		"ANTHROPIC_API_KEY":     "",
		"AVIAN_API_KEY":         "",
		"AWS_ACCESS_KEY_ID":     "",
		"AWS_DEFAULT_REGION":    "",
		"AWS_REGION":            "",
		"AWS_SECRET_ACCESS_KEY": "",
		"AWS_SESSION_TOKEN":     "",
		"AZURE_OPENAI_API_KEY":  "",
		"CEREBRAS_API_KEY":      "",
		"CHUTES_API_KEY":        "",
		"COPILOT_TOKEN":         "",
		"GEMINI_API_KEY":        "",
		"GOOGLE_API_KEY":        "",
		"GROQ_API_KEY":          "",
		"HF_TOKEN":              "",
		"IONET_API_KEY":         "",
		"KIMI_CODING_API_KEY":   "",
		"MINIMAX_API_KEY":       "",
		"OPENAI_API_KEY":        "",
		"OPENROUTER_API_KEY":    "",
		"QINIUCLOUD_API_KEY":    "",
		"SYNTHETIC_API_KEY":     "",
		"VERCEL_API_KEY":        "",
		"VENICE_API_KEY":        "",
		"VERTEXAI_LOCATION":     "",
		"VERTEXAI_PROJECT":      "",
		"XAI_API_KEY":           "",
		"ZAI_API_KEY":           "",
		"ZHIPU_API_KEY":         "",
	}
}

func launchFixtureTUI(t *testing.T, fixture tuiFixture, args ...string) *TUITestInstance {
	t.Helper()
	return launchTUIWithOptions(t, tuiLaunchOptions{
		args:       args,
		env:        fixture.env(),
		workingDir: fixture.workingDir,
	})
}

func launchFixtureTUIWithOptions(t *testing.T, fixture tuiFixture, opts tuiLaunchOptions) *TUITestInstance {
	t.Helper()

	opts.env = mergeEnvVars(fixture.env(), opts.env)
	if opts.workingDir == "" {
		opts.workingDir = fixture.workingDir
	}
	return launchTUIWithOptions(t, opts)
}

func waitForConfiguredLanding(t *testing.T, tui *TUITestInstance) {
	t.Helper()
	require.NoError(t, tui.WaitForText("CRUSH", 15*time.Second))
	require.NoError(t, tui.WaitForText(fixtureLargeModelName, 10*time.Second))
}

func waitForDashboard(t *testing.T, tui *TUITestInstance) {
	t.Helper()

	if err := tui.WaitForAnyText([]string{
		"New Chat",
		"Run Workflow",
		"Initialize Smithers",
	}, 15*time.Second); err == nil {
		if err := tui.WaitForText("At a Glance", 3*time.Second); err == nil {
			return
		}
	}

	require.NoError(t, tui.WaitForAnyText([]string{
		"MCPs",
		"Ready for instructions",
		"Ready...",
		"Ready?",
		fixtureLargeModelName,
	}, 15*time.Second))
}

func openModelsDialog(t *testing.T, tui *TUITestInstance) {
	t.Helper()
	tui.SendKeys("\x0c") // ctrl+l
	require.NoError(t, tui.WaitForText("Switch Model", 5*time.Second),
		"models dialog must open; buffer:\n%s", tui.Snapshot())
}

func openSessionsDialog(t *testing.T, tui *TUITestInstance) {
	t.Helper()
	tui.SendKeys("\x13") // ctrl+s
	require.NoError(t, tui.WaitForText("Sessions", 5*time.Second),
		"sessions dialog must open; buffer:\n%s", tui.Snapshot())
}

func openQuitDialog(t *testing.T, tui *TUITestInstance) {
	t.Helper()
	tui.SendKeys("\x03") // ctrl+c
	require.NoError(t, tui.WaitForText("Are you sure you want to quit?", 5*time.Second),
		"quit dialog must open; buffer:\n%s", tui.Snapshot())
}

func writeTextFixture(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func writePNGFixture(t *testing.T, dir, name string) string {
	t.Helper()

	const oneByOnePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jXioAAAAASUVORK5CYII="

	data, err := base64.StdEncoding.DecodeString(oneByOnePNG)
	require.NoError(t, err)

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func waitForFileText(path, text string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}

	deadline := time.Now().Add(timeout)
	var lastContents string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			lastContents = string(data)
			if strings.Contains(lastContents, text) {
				return nil
			}
		case !os.IsNotExist(err):
			return err
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("waitForFileText: %q not found in %s within %s; last contents: %q", text, path, timeout, lastContents)
}

func mergeEnvVars(base, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}

	merged := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func seedSessions(t *testing.T, dataDir string, seeds ...seededSession) []session.Session {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()

	queries := db.New(conn)
	sessionsSvc := session.NewService(queries, conn)
	messagesSvc := message.NewService(queries)

	created := make([]session.Session, 0, len(seeds))
	for _, seed := range seeds {
		sess, err := sessionsSvc.Create(ctx, seed.title)
		require.NoError(t, err)
		created = append(created, sess)

		for _, body := range seed.messages {
			_, err = messagesSvc.Create(ctx, sess.ID, message.CreateMessageParams{
				Role:  message.User,
				Parts: []message.ContentPart{message.TextContent{Text: body}},
			})
			require.NoError(t, err)
		}
	}

	return created
}

func waitForSessionTitleState(t *testing.T, dataDir, title string, wantPresent bool, timeout time.Duration) {
	t.Helper()

	ctx := context.Background()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()

	queries := db.New(conn)
	sessionsSvc := session.NewService(queries, conn)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sessions, listErr := sessionsSvc.List(ctx)
		require.NoError(t, listErr)

		found := false
		for _, sess := range sessions {
			if sess.Title == title {
				found = true
				break
			}
		}
		if found == wantPresent {
			return
		}

		time.Sleep(pollInterval)
	}

	state := "absent"
	if wantPresent {
		state = "present"
	}
	t.Fatalf("session title %q did not become %s within %s", title, state, timeout)
}

func skipUnlessCrushTUIE2E(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for this e2e test")
	}
}

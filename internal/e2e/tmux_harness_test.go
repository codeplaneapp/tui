package e2e_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	tmuxWaitTimeout = 20 * time.Second
	tmuxPoll        = 200 * time.Millisecond
	defaultCols     = 120
	defaultRows     = 40
)

// ansiStripPattern strips ANSI escape sequences from captured pane output.
var ansiStripPattern = regexp.MustCompile(`\x1B(?:\[[0-9;]*[a-zA-Z]|\].*?\x07|\(B)`)

// TmuxSession wraps a real tmux session running the TUI binary with a proper PTY.
type TmuxSession struct {
	t          *testing.T
	sessionID  string
	binaryPath string
	configDir  string
	dataDir    string
	cols       int
	rows       int
}

// TmuxOpt configures a TmuxSession.
type TmuxOpt func(*TmuxSession)

// WithSize sets the tmux pane dimensions.
func WithSize(cols, rows int) TmuxOpt {
	return func(s *TmuxSession) {
		s.cols = cols
		s.rows = rows
	}
}

// WithSmithersConfig writes a smithers config that points at the given API URL.
// If apiURL is empty, a minimal offline config is written.
func WithSmithersConfig(apiURL string) TmuxOpt {
	return func(s *TmuxSession) {
		writeSmithersConfig(s.t, s.configDir, apiURL, ".smithers/smithers.db")
	}
}

// WithSmithersDBPath writes an offline smithers config that points at the
// given SQLite database path.
func WithSmithersDBPath(dbPath string) TmuxOpt {
	return func(s *TmuxSession) {
		writeSmithersConfig(s.t, s.configDir, "", dbPath)
	}
}

// WithObservability enables the local observability debug server for the test.
func WithObservability(addr string) TmuxOpt {
	return func(s *TmuxSession) {
		writeObservabilityConfig(s.t, s.configDir, addr)
	}
}

// buildBinary compiles the TUI binary once per test run and returns its path.
// The binary is placed in a temp directory and cleaned up automatically.
func buildBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "crush-e2e")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build binary: %v\n%s", err, out)
	}
	return binPath
}

// NewTmuxSession creates a tmux session running the TUI binary.
// The session, temp dirs, and process are cleaned up via t.Cleanup.
func NewTmuxSession(t *testing.T, binary string, opts ...TmuxOpt) *TmuxSession {
	t.Helper()

	sess := &TmuxSession{
		t:          t,
		sessionID:  fmt.Sprintf("e2e-%s-%d", t.Name(), time.Now().UnixNano()),
		binaryPath: binary,
		configDir:  t.TempDir(),
		dataDir:    t.TempDir(),
		cols:       defaultCols,
		rows:       defaultRows,
	}

	for _, o := range opts {
		o(sess)
	}

	// Default smithers config if none was set by opts.
	cfgPath := filepath.Join(sess.configDir, "smithers-tui.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		WithSmithersConfig("")(sess)
	}

	// Create the tmux session with the TUI binary.
	env := fmt.Sprintf(
		"CRUSH_GLOBAL_CONFIG=%s CRUSH_GLOBAL_DATA=%s TERM=xterm-256color COLORTERM=truecolor LANG=en_US.UTF-8",
		sess.configDir, sess.dataDir,
	)
	shellCmd := fmt.Sprintf("%s %s", env, sess.binaryPath)

	args := []string{
		"new-session",
		"-d",                 // detached
		"-s", sess.sessionID, // session name
		"-x", fmt.Sprintf("%d", sess.cols),
		"-y", fmt.Sprintf("%d", sess.rows),
		shellCmd,
	}

	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux new-session: %v\n%s", err, out)
	}

	// Set escape-time to 0 so bare Escape keys are forwarded immediately
	// to BubbleTea v2 (which uses Kitty keyboard protocol).
	_ = exec.Command("tmux", "set-option", "-t", sess.sessionID, "escape-time", "0").Run()

	t.Cleanup(func() {
		// Kill the tmux session and all processes in it.
		_ = exec.Command("tmux", "kill-session", "-t", sess.sessionID).Run()
	})

	return sess
}

func writeSmithersConfig(t *testing.T, configDir, apiURL, dbPath string) {
	t.Helper()

	cfg := loadSmithersConfig(t, configDir)
	cfg.Smithers.DBPath = dbPath
	cfg.Smithers.WorkflowDir = ".smithers/workflows"
	cfg.Smithers.APIURL = apiURL
	saveSmithersConfig(t, configDir, cfg)
}

func writeObservabilityConfig(t *testing.T, configDir, addr string) {
	t.Helper()

	cfg := loadSmithersConfig(t, configDir)
	if strings.TrimSpace(addr) == "" {
		if cfg.Options != nil {
			cfg.Options.Observability = nil
			if cfg.Options.isEmpty() {
				cfg.Options = nil
			}
		}
		saveSmithersConfig(t, configDir, cfg)
		return
	}

	if cfg.Options == nil {
		cfg.Options = &tmuxOptionsConfig{}
	}
	sampleRatio := 1.0
	cfg.Options.Observability = &tmuxObservabilityConfig{
		Address:          addr,
		TraceBufferSize:  128,
		TraceSampleRatio: &sampleRatio,
	}
	saveSmithersConfig(t, configDir, cfg)
}

type tmuxConfigFile struct {
	Smithers tmuxSmithersConfig `json:"smithers"`
	Options  *tmuxOptionsConfig `json:"options,omitempty"`
}

type tmuxSmithersConfig struct {
	DBPath      string `json:"dbPath"`
	WorkflowDir string `json:"workflowDir"`
	APIURL      string `json:"apiUrl,omitempty"`
}

type tmuxOptionsConfig struct {
	Observability *tmuxObservabilityConfig `json:"observability,omitempty"`
}

func (o *tmuxOptionsConfig) isEmpty() bool {
	return o == nil || o.Observability == nil
}

type tmuxObservabilityConfig struct {
	Address          string   `json:"address,omitempty"`
	TraceBufferSize  int      `json:"trace_buffer_size,omitempty"`
	TraceSampleRatio *float64 `json:"trace_sample_ratio,omitempty"`
}

func loadSmithersConfig(t *testing.T, configDir string) tmuxConfigFile {
	t.Helper()

	cfg := tmuxConfigFile{
		Smithers: tmuxSmithersConfig{
			DBPath:      ".smithers/smithers.db",
			WorkflowDir: ".smithers/workflows",
		},
	}

	path := filepath.Join(configDir, "smithers-tui.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg
		}
		t.Fatalf("read smithers config: %v", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode smithers config: %v", err)
	}

	if cfg.Smithers.DBPath == "" {
		cfg.Smithers.DBPath = ".smithers/smithers.db"
	}
	if cfg.Smithers.WorkflowDir == "" {
		cfg.Smithers.WorkflowDir = ".smithers/workflows"
	}
	return cfg
}

func saveSmithersConfig(t *testing.T, configDir string, cfg tmuxConfigFile) {
	t.Helper()

	path := filepath.Join(configDir, "smithers-tui.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal smithers config: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write smithers config: %v", err)
	}
}

// SendKeys sends key sequences to the tmux pane.
// Uses tmux send-keys which properly handles control characters.
// For Escape keys, a small delay is added to ensure BubbleTea v2's
// Kitty keyboard input parser recognizes the bare escape byte.
func (s *TmuxSession) SendKeys(keys ...string) {
	s.t.Helper()
	args := append([]string{"send-keys", "-t", s.sessionID}, keys...)
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		s.t.Fatalf("tmux send-keys %v: %v\n%s", keys, err, out)
	}
	// BubbleTea v2 uses Kitty keyboard protocol. When it receives a bare
	// \x1b, it waits briefly to see if more bytes follow (escape sequence).
	// Adding a short sleep after Escape gives the parser time to timeout
	// and emit the key event.
	for _, k := range keys {
		if k == "Escape" {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// SendText types literal text into the tmux pane (no special key interpretation).
func (s *TmuxSession) SendText(text string) {
	s.t.Helper()
	args := []string{"send-keys", "-t", s.sessionID, "-l", text}
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		s.t.Fatalf("tmux send-keys -l: %v\n%s", err, out)
	}
}

// CapturePane returns the current visible content of the tmux pane
// with ANSI escapes stripped.
func (s *TmuxSession) CapturePane() string {
	s.t.Helper()
	out, err := exec.Command(
		"tmux", "capture-pane",
		"-t", s.sessionID,
		"-p", // print to stdout
		"-e", // include escape sequences (we strip them)
	).Output()
	if err != nil {
		s.t.Fatalf("tmux capture-pane: %v", err)
	}
	return stripANSI(string(out))
}

// WaitForText polls the tmux pane until the given text appears or timeout.
func (s *TmuxSession) WaitForText(text string, timeout ...time.Duration) {
	s.t.Helper()
	to := tmuxWaitTimeout
	if len(timeout) > 0 {
		to = timeout[0]
	}
	deadline := time.Now().Add(to)
	var lastCapture string
	for time.Now().Before(deadline) {
		lastCapture = s.CapturePane()
		if containsNormalized(lastCapture, text) {
			return
		}
		time.Sleep(tmuxPoll)
	}
	s.t.Fatalf("WaitForText: %q not found within %s\nPane content:\n%s", text, to, lastCapture)
}

// WaitForNoText polls until the given text is NOT present or timeout.
func (s *TmuxSession) WaitForNoText(text string, timeout ...time.Duration) {
	s.t.Helper()
	to := tmuxWaitTimeout
	if len(timeout) > 0 {
		to = timeout[0]
	}
	deadline := time.Now().Add(to)
	var lastCapture string
	for time.Now().Before(deadline) {
		lastCapture = s.CapturePane()
		if !containsNormalized(lastCapture, text) {
			return
		}
		time.Sleep(tmuxPoll)
	}
	s.t.Fatalf("WaitForNoText: %q still present after %s\nPane content:\n%s", text, to, lastCapture)
}

// WaitForAnyText polls until any of the given texts appear or timeout.
func (s *TmuxSession) WaitForAnyText(texts []string, timeout ...time.Duration) string {
	s.t.Helper()
	to := tmuxWaitTimeout
	if len(timeout) > 0 {
		to = timeout[0]
	}
	deadline := time.Now().Add(to)
	var lastCapture string
	for time.Now().Before(deadline) {
		lastCapture = s.CapturePane()
		for _, text := range texts {
			if containsNormalized(lastCapture, text) {
				return text
			}
		}
		time.Sleep(tmuxPoll)
	}
	s.t.Fatalf("WaitForAnyText: none of %v found within %s\nPane content:\n%s", texts, to, lastCapture)
	return ""
}

// AssertVisible asserts that the given text is currently visible in the pane.
func (s *TmuxSession) AssertVisible(text string) {
	s.t.Helper()
	capture := s.CapturePane()
	if !containsNormalized(capture, text) {
		s.t.Fatalf("AssertVisible: %q not found\nPane content:\n%s", text, capture)
	}
}

// AssertNotVisible asserts that the given text is NOT currently visible in the pane.
func (s *TmuxSession) AssertNotVisible(text string) {
	s.t.Helper()
	capture := s.CapturePane()
	if containsNormalized(capture, text) {
		s.t.Fatalf("AssertNotVisible: %q was found\nPane content:\n%s", text, capture)
	}
}

// Snapshot returns a snapshot of the current pane for debugging.
func (s *TmuxSession) Snapshot() string {
	return s.CapturePane()
}

func openChatTargetPickerViaDashboard(t *testing.T, s *TmuxSession) {
	t.Helper()
	s.SendKeys("Enter")
	s.WaitForAnyText([]string{
		"Choose how you want to chat in this workspace.",
		"Start Chat",
	}, 10*time.Second)
}

func openSmithersChatViaDashboard(t *testing.T, s *TmuxSession) {
	t.Helper()
	openChatTargetPickerViaDashboard(t, s)
	s.SendKeys("Enter")
	s.WaitForAnyText([]string{
		"MCPs",
		"Ready for instructions",
		"Ready...",
		"New Session",
	}, 15*time.Second)
}

// stripANSI removes ANSI escape sequences from text.
func stripANSI(s string) string {
	return ansiStripPattern.ReplaceAllString(s, "")
}

// containsNormalized checks if haystack contains needle after normalizing
// whitespace and box-drawing characters.
func containsNormalized(haystack, needle string) bool {
	if strings.Contains(haystack, needle) {
		return true
	}
	// Also try with normalized box-drawing chars and collapsed whitespace.
	normH := normalizeForMatch(haystack)
	normN := normalizeForMatch(needle)
	return strings.Contains(normH, normN)
}

// normalizeForMatch replaces box-drawing characters with spaces and
// collapses whitespace for fuzzy text matching.
func normalizeForMatch(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x2500 && r <= 0x257F {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// skipUnlessTmuxE2E skips the test unless SMITHERS_E2E=1 is set.
func skipUnlessTmuxE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("SMITHERS_E2E") != "1" {
		t.Skip("set SMITHERS_E2E=1 to run tmux E2E tests")
	}
	// Verify tmux is available.
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}
}

type debugTraceSpan struct {
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes"`
}

func reserveObservabilityAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve observability addr: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close reserved observability listener: %v", err)
	}
	return addr
}

func waitForObservabilityReady(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	waitForHTTP(t, "http://"+addr+"/debug/observability", timeout, func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	})
}

func waitForTraceSpan(t *testing.T, addr string, timeout time.Duration, predicate func(debugTraceSpan) bool) debugTraceSpan {
	t.Helper()

	var matched debugTraceSpan
	waitForHTTP(t, "http://"+addr+"/debug/traces?limit=200", timeout, func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}

		var spans []debugTraceSpan
		if err := json.NewDecoder(resp.Body).Decode(&spans); err != nil {
			return err
		}

		for _, span := range spans {
			if predicate(span) {
				matched = span
				return nil
			}
		}
		return fmt.Errorf("matching span not found yet")
	})
	return matched
}

func waitForMetricAtLeast(t *testing.T, addr, metricName string, labels map[string]string, minValue float64, timeout time.Duration) {
	t.Helper()

	waitForHTTP(t, "http://"+addr+"/metrics", timeout, func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}

		parser := expfmt.TextParser{}
		families, err := parser.TextToMetricFamilies(resp.Body)
		if err != nil {
			return err
		}

		family := families[metricName]
		if family == nil {
			return fmt.Errorf("metric %s not found", metricName)
		}

		for _, metric := range family.GetMetric() {
			if metricLabelsMatch(metric, labels) && metricNumericValue(metric) >= minValue {
				return nil
			}
		}

		return fmt.Errorf("metric %s with labels %v below %.2f", metricName, labels, minValue)
	})
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration, predicate func(*http.Response) error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}

		err = predicate(resp)
		_ = resp.Body.Close()
		if err == nil {
			return
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("wait for %s: %v", url, lastErr)
}

func metricLabelsMatch(metric *dto.Metric, labels map[string]string) bool {
	for key, want := range labels {
		found := false
		for _, label := range metric.GetLabel() {
			if label.GetName() == key && label.GetValue() == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func metricNumericValue(metric *dto.Metric) float64 {
	switch {
	case metric.Counter != nil:
		return metric.GetCounter().GetValue()
	case metric.Gauge != nil:
		return metric.GetGauge().GetValue()
	case metric.Histogram != nil:
		return float64(metric.GetHistogram().GetSampleCount())
	default:
		return 0
	}
}

func spanHasAttrs(span debugTraceSpan, attrs map[string]string) bool {
	if span.Attributes == nil {
		return false
	}
	for key, want := range attrs {
		got, ok := span.Attributes[key]
		if !ok || strings.TrimSpace(fmt.Sprint(got)) != want {
			return false
		}
	}
	return true
}

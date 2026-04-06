// Package diffnav provides helpers for launching the diffnav TUI diff viewer
// as a subprocess, and prompting the user to install it if not found.
package diffnav

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/handoff"
)

// Available returns true if diffnav is on PATH.
func Available() bool {
	_, err := exec.LookPath("diffnav")
	return err == nil
}

// LaunchDiffnavWithCommand runs the diff command, writes its output to a
// temporary patch file, and pipes that file into diffnav. If diffnav is not
// installed, returns an InstallPromptMsg instead.
func LaunchDiffnavWithCommand(command string, cwd string, tag any) tea.Cmd {
	if !Available() {
		return func() tea.Msg {
			return InstallPromptMsg{
				PendingCommand: command,
				PendingCwd:     cwd,
				PendingTag:     tag,
			}
		}
	}
	return func() tea.Msg {
		tmpPath, err := writeCommandDiffToTempFile(command, cwd)
		if err != nil {
			return handoff.HandoffMsg{
				Tag:    tag,
				Result: handoffResultFromError(err),
			}
		}
		return launchDiffnavFromFile(tmpPath, cwd, tag)()
	}
}

// LaunchDiffnav writes diff content to a temp file and launches diffnav.
func LaunchDiffnav(diffContent string, tag any) tea.Cmd {
	if !Available() {
		return func() tea.Msg {
			return InstallPromptMsg{PendingTag: tag}
		}
	}
	return func() tea.Msg {
		tmpPath, err := writeDiffToTempFile(diffContent)
		if err != nil {
			return handoff.HandoffMsg{
				Tag:    tag,
				Result: handoffResultFromError(err),
			}
		}
		return launchDiffnavFromFile(tmpPath, "", tag)()
	}
}

func launchDiffnavFromFile(tmpPath string, cwd string, tag any) tea.Cmd {
	stderrPath := tmpPath + ".stderr"
	binary, args := diffnavInputCommand(tmpPath, stderrPath)
	return handoff.HandoffWithCallback(handoff.Options{
		Binary: binary,
		Args:   args,
		Cwd:    cwd,
		Tag:    tag,
	}, func(err error) tea.Msg {
		return finishDiffnavLaunch(err, stderrPath, tmpPath, cwd, tag)
	})
}

func writeDiffToTempFile(diffContent string) (string, error) {
	tmp, err := os.CreateTemp("", "crush-diff-*.diff")
	if err != nil {
		return "", err
	}

	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(diffContent); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return tmpPath, nil
}

func writeCommandDiffToTempFile(command string, cwd string) (string, error) {
	tmp, err := os.CreateTemp("", "crush-diff-*.diff")
	if err != nil {
		return "", err
	}

	tmpPath := tmp.Name()
	if err := runCommandToWriter(command, cwd, tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func runCommandToWriter(command string, cwd string, output *os.File) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("diff command must not be empty")
	}

	binary, args := shellCommand(command)
	cmd := exec.Command(binary, args...) //nolint:gosec // command string comes from the caller.
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = output

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("run diff command: %s", msg)
	}

	return nil
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func diffnavInputCommand(inputPath string, stderrPath string) (string, []string) {
	return shellCommand("diffnav < " + shellQuote(inputPath) + " 2> " + shellQuote(stderrPath))
}

func pagerCommand(path string, pagerEnv string) (string, []string) {
	pagerEnv = strings.TrimSpace(pagerEnv)
	if pagerEnv != "" {
		return shellCommand(pagerEnv + " " + shellQuote(path))
	}

	if runtime.GOOS == "windows" {
		return shellCommand("more < " + shellQuote(path))
	}
	if _, err := exec.LookPath("less"); err == nil {
		return "less", []string{"-R", path}
	}
	if _, err := exec.LookPath("more"); err == nil {
		return "more", []string{path}
	}

	return shellCommand("cat " + shellQuote(path))
}

func shellQuote(value string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func handoffResultFromError(err error) handoff.HandoffResult {
	result := handoff.HandoffResult{Err: err}
	if err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	return result
}

func finishDiffnavLaunch(err error, stderrPath string, tmpPath string, cwd string, tag any) tea.Msg {
	reason := readCommandOutput(stderrPath)
	_ = os.Remove(stderrPath)

	if err == nil {
		_ = os.Remove(tmpPath)
		return handoff.HandoffMsg{
			Tag:    tag,
			Result: handoffResultFromError(nil),
		}
	}

	if strings.TrimSpace(reason) == "" {
		reason = err.Error()
	}
	return PagerFallbackMsg{
		Path:   tmpPath,
		Cwd:    cwd,
		Tag:    tag,
		Reason: reason,
	}
}

func readCommandOutput(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// PagerFallbackMsg asks the UI to open a raw pager when diffnav exits with an
// error after the patch file has already been prepared.
type PagerFallbackMsg struct {
	Path   string
	Cwd    string
	Tag    any
	Reason string
}

// PagerErrorMsg reports a pager launch failure back to the UI.
type PagerErrorMsg struct {
	Tag any
	Err error
}

// LaunchPager opens the prepared patch file in a plain pager and removes the
// temp file when the pager exits.
func LaunchPager(path string, cwd string, tag any) tea.Cmd {
	binary, args := pagerCommand(path, os.Getenv("PAGER"))
	return handoff.HandoffWithCallback(handoff.Options{
		Binary: binary,
		Args:   args,
		Cwd:    cwd,
		Tag:    tag,
	}, func(err error) tea.Msg {
		return finishPagerLaunch(err, path, tag)
	})
}

func finishPagerLaunch(err error, path string, tag any) tea.Msg {
	_ = os.Remove(path)
	if err != nil {
		return PagerErrorMsg{Tag: tag, Err: err}
	}
	return nil
}

// --- Install prompt ---

// InstallPromptMsg is emitted when diffnav is not found. The UI should
// show a prompt asking the user to install it.
type InstallPromptMsg struct {
	PendingCommand string
	PendingCwd     string
	PendingTag     any
}

// InstallResultMsg is emitted after an install attempt completes.
type InstallResultMsg struct {
	Success bool
	Method  string // "brew", "binary", "go"
	Err     error
}

// InstallDiffnav attempts to install diffnav using the best available method.
func InstallDiffnav() tea.Cmd {
	return func() tea.Msg {
		// 1. Try brew (macOS + Linux)
		if _, err := exec.LookPath("brew"); err == nil {
			cmd := exec.Command("brew", "install", "dlvhdr/formulae/diffnav")
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return InstallResultMsg{Success: true, Method: "brew"}
			}
		}

		// 2. Try direct binary download from GitHub releases
		if err := installFromRelease(); err == nil {
			return InstallResultMsg{Success: true, Method: "binary"}
		}

		// 3. Try go install (if Go is available)
		if _, err := exec.LookPath("go"); err == nil {
			cmd := exec.Command("go", "install", "github.com/dlvhdr/diffnav@latest")
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err == nil {
				return InstallResultMsg{Success: true, Method: "go"}
			} else {
				return InstallResultMsg{Err: fmt.Errorf("go install: %s", strings.TrimSpace(string(out)))}
			}
		}

		return InstallResultMsg{
			Err: errors.New("could not install diffnav: no package manager found (tried brew, direct download, go install)"),
		}
	}
}

// installFromRelease downloads the latest diffnav binary from GitHub releases.
func installFromRelease() error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map to release naming convention
	osName := map[string]string{
		"darwin": "Darwin", "linux": "Linux", "windows": "Windows",
	}[goos]
	archName := map[string]string{
		"amd64": "x86_64", "arm64": "arm64", "386": "i386",
	}[goarch]
	if osName == "" || archName == "" {
		return fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}

	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	// Get latest version
	version := "0.11.0" // pinned known-good version
	url := fmt.Sprintf(
		"https://github.com/dlvhdr/diffnav/releases/download/v%s/diffnav_%s_%s.%s",
		version, osName, archName, ext,
	)

	// Download and extract to a bin directory
	binDir := os.ExpandEnv("$HOME/.local/bin")
	os.MkdirAll(binDir, 0o755)

	if ext == "tar.gz" {
		cmd := exec.Command("sh", "-c",
			fmt.Sprintf("curl -sL '%s' | tar xz -C '%s' diffnav", url, binDir))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("download: %w", err)
		}
	} else {
		return fmt.Errorf("windows zip install not yet supported")
	}

	// Verify it's there
	path := binDir + "/diffnav"
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("binary not found after download: %w", err)
	}
	os.Chmod(path, 0o755)

	// Add to PATH for this session if needed
	currentPath := os.Getenv("PATH")
	if !strings.Contains(currentPath, binDir) {
		os.Setenv("PATH", binDir+":"+currentPath)
	}

	return nil
}

// InstallMethods returns a human-readable list of install options for display.
func InstallMethods() string {
	var methods []string
	if _, err := exec.LookPath("brew"); err == nil {
		methods = append(methods, "brew install dlvhdr/formulae/diffnav")
	}
	methods = append(methods, "Download from github.com/dlvhdr/diffnav/releases")
	if _, err := exec.LookPath("go"); err == nil {
		methods = append(methods, "go install github.com/dlvhdr/diffnav@latest (requires source build)")
	}
	return strings.Join(methods, "\n")
}

package jjhub

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultWorkerSessionName   = "codeplane-worker"
	workspaceSandboxEnv        = "CODEPLANE_WORKSPACE_SANDBOX"
	workspaceSandboxAutoValue  = "auto"
	workspaceSandboxBwrapValue = "bwrap"
	workspaceSandboxOffValue   = "off"
)

var (
	ErrWorkspaceSSHUnavailable = errors.New("workspace SSH is not available")
	ErrSSHBinaryUnavailable    = errors.New("ssh not found on PATH")
)

// AttachWorkspaceCommand builds the SSH command that attaches to a persistent
// Codeplane worker running inside a JJHub workspace. The remote worker is
// hosted in a tmux session so users can detach and reattach without losing the
// process.
func AttachWorkspaceCommand(workspace Workspace) (*exec.Cmd, error) {
	return attachWorkspaceCommand(workspace, exec.LookPath)
}

func attachWorkspaceCommand(workspace Workspace, lookPathFn func(string) (string, error)) (*exec.Cmd, error) {
	start := time.Now()
	attrs := []attribute.KeyValue{
		attribute.String("codeplane.workspace.source", "jjhub"),
		attribute.String("codeplane.workspace.id", workspace.ID),
	}
	host := workspaceSSHHost(workspace)
	if host == "" {
		err := ErrWorkspaceSSHUnavailable
		recordAttachWorkspacePrepareResult(time.Since(start), err, attrs...)
		return nil, err
	}
	attrs = append(attrs, attribute.String("codeplane.workspace.ssh_host", host))
	if _, err := lookPathFn("ssh"); err != nil {
		err = ErrSSHBinaryUnavailable
		recordAttachWorkspacePrepareResult(time.Since(start), err, attrs...)
		return nil, err
	}

	cmd := exec.Command( //nolint:gosec
		"ssh",
		"-tt",
		host,
		"bash",
		"-lc",
		workerAttachScript(defaultWorkerSessionName),
	)
	recordAttachWorkspacePrepareResult(time.Since(start), nil, attrs...)
	return cmd, nil
}

func workerAttachScript(sessionName string) string {
	return workerAttachScriptWithSandbox(sessionName, os.Getenv(workspaceSandboxEnv))
}

func workerAttachScriptWithSandbox(sessionName, sandboxMode string) string {
	quotedSession := shellQuote(sessionName)
	mode := normalizeWorkspaceSandboxMode(sandboxMode)
	lines := []string{
		"set -euo pipefail",
		"if ! command -v tmux >/dev/null 2>&1; then",
		"  echo 'tmux is required in the workspace to attach a persistent Codeplane worker' >&2",
		"  exit 127",
		"fi",
		"worker_bin=''",
		"if command -v codeplane >/dev/null 2>&1; then",
		"  worker_bin='codeplane'",
		"elif command -v crush >/dev/null 2>&1; then",
		"  worker_bin='crush'",
		"else",
		"  echo 'Neither codeplane nor crush is installed in the workspace' >&2",
		"  exit 127",
		"fi",
		fmt.Sprintf("session=%s", quotedSession),
		fmt.Sprintf("sandbox_mode=%s", shellQuote(mode)),
		"workspace_dir=$(git rev-parse --show-toplevel 2>/dev/null || pwd)",
		`if ! tmux has-session -t "$session" 2>/dev/null; then`,
		`  escaped_dir=$(printf '%q' "$workspace_dir")`,
		`  escaped_bin=$(printf '%q' "$worker_bin")`,
		`  launch_cmd="cd ${escaped_dir} && exec ${escaped_bin}"`,
	}
	lines = append(lines, workerSandboxScriptLines(mode)...)
	lines = append(lines,
		`  tmux new-session -d -s "$session" "$launch_cmd"`,
		"fi",
		`exec tmux attach-session -t "$session"`,
	)
	return strings.Join(lines, "\n")
}

func workerSandboxScriptLines(mode string) []string {
	if mode == workspaceSandboxOffValue {
		return nil
	}
	lines := []string{
		`  sandbox_bin=''`,
		`  if command -v bwrap >/dev/null 2>&1; then`,
		`    sandbox_bin='bwrap'`,
		`  elif command -v bubblewrap >/dev/null 2>&1; then`,
		`    sandbox_bin='bubblewrap'`,
		`  fi`,
	}
	if mode == workspaceSandboxBwrapValue {
		lines = append(lines,
			`  if [ -z "$sandbox_bin" ]; then`,
			`    echo 'bubblewrap requested but neither bwrap nor bubblewrap is installed in the workspace' >&2`,
			`    exit 127`,
			`  fi`,
		)
	}
	lines = append(lines,
		`  if [ -n "$sandbox_bin" ]; then`,
		`    escaped_sandbox=$(printf '%q' "$sandbox_bin")`,
		`    launch_cmd="cd ${escaped_dir} && exec ${escaped_sandbox} --die-with-parent --new-session --proc /proc --dev-bind / / --chdir ${escaped_dir} ${escaped_bin}"`,
		`  fi`,
	)
	return lines
}

func normalizeWorkspaceSandboxMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", workspaceSandboxAutoValue, "true":
		return workspaceSandboxAutoValue
	case workspaceSandboxBwrapValue, "always", "required":
		return workspaceSandboxBwrapValue
	case workspaceSandboxOffValue, "false", "disabled", "none":
		return workspaceSandboxOffValue
	default:
		return workspaceSandboxAutoValue
	}
}

func workspaceSSHHost(workspace Workspace) string {
	if workspace.SSHHost == nil {
		return ""
	}
	return strings.TrimSpace(*workspace.SSHHost)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func recordAttachWorkspacePrepareResult(duration time.Duration, err error, attrs ...attribute.KeyValue) {
	result := "ok"
	if err != nil {
		result = "error"
		attrs = append(attrs, attribute.String("codeplane.error", err.Error()))
	}
	observability.RecordWorkspaceLifecycle("attach_prepare", result, duration, attrs...)
}

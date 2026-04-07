package jjhub

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const defaultWorkerSessionName = "crush-worker"

var (
	ErrWorkspaceSSHUnavailable = errors.New("workspace SSH is not available")
	ErrSSHBinaryUnavailable    = errors.New("ssh not found on PATH")
)

// AttachWorkspaceCommand builds the SSH command that attaches to a persistent
// Crush worker running inside a JJHub workspace. The remote worker is hosted in
// a tmux session so users can detach and reattach without losing the process.
func AttachWorkspaceCommand(workspace Workspace) (*exec.Cmd, error) {
	return attachWorkspaceCommand(workspace, exec.LookPath)
}

func attachWorkspaceCommand(workspace Workspace, lookPathFn func(string) (string, error)) (*exec.Cmd, error) {
	host := workspaceSSHHost(workspace)
	if host == "" {
		return nil, ErrWorkspaceSSHUnavailable
	}
	if _, err := lookPathFn("ssh"); err != nil {
		return nil, ErrSSHBinaryUnavailable
	}

	return exec.Command( //nolint:gosec
		"ssh",
		"-tt",
		host,
		"bash",
		"-lc",
		workerAttachScript(defaultWorkerSessionName),
	), nil
}

func workerAttachScript(sessionName string) string {
	quotedSession := shellQuote(sessionName)
	lines := []string{
		"set -euo pipefail",
		"if ! command -v tmux >/dev/null 2>&1; then",
		"  echo 'tmux is required in the workspace to attach a persistent Crush worker' >&2",
		"  exit 127",
		"fi",
		"worker_bin=''",
		"if command -v crush >/dev/null 2>&1; then",
		"  worker_bin='crush'",
		"elif command -v codeplane >/dev/null 2>&1; then",
		"  worker_bin='codeplane'",
		"else",
		"  echo 'Neither crush nor codeplane is installed in the workspace' >&2",
		"  exit 127",
		"fi",
		fmt.Sprintf("session=%s", quotedSession),
		"workspace_dir=$(git rev-parse --show-toplevel 2>/dev/null || pwd)",
		`if ! tmux has-session -t "$session" 2>/dev/null; then`,
		`  escaped_dir=$(printf '%q' "$workspace_dir")`,
		`  escaped_bin=$(printf '%q' "$worker_bin")`,
		`  tmux new-session -d -s "$session" "cd ${escaped_dir} && exec ${escaped_bin}"`,
		"fi",
		`exec tmux attach-session -t "$session"`,
	}
	return strings.Join(lines, "\n")
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

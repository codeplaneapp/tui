package jjhub

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachWorkspaceCommand_MissingSSHHost(t *testing.T) {
	configureWorkerObservability(t)

	workspace := Workspace{ID: "ws-1"}
	cmd, err := attachWorkspaceCommand(workspace, func(string) (string, error) {
		t.Fatal("lookPath should not be called when SSH is unavailable")
		return "", nil
	})

	assert.Nil(t, cmd)
	require.ErrorIs(t, err, ErrWorkspaceSSHUnavailable)

	attrs := requireWorkerSpanAttrs(t, "attach_prepare", "error")
	assert.Equal(t, "ws-1", attrs["codeplane.workspace.id"])
}

func TestAttachWorkspaceCommand_MissingSSHBinary(t *testing.T) {
	configureWorkerObservability(t)

	host := "alpha.example.com"
	workspace := Workspace{ID: "ws-1", SSHHost: &host}
	cmd, err := attachWorkspaceCommand(workspace, func(name string) (string, error) {
		assert.Equal(t, "ssh", name)
		return "", errors.New("missing")
	})

	assert.Nil(t, cmd)
	require.ErrorIs(t, err, ErrSSHBinaryUnavailable)

	attrs := requireWorkerSpanAttrs(t, "attach_prepare", "error")
	assert.Equal(t, host, attrs["codeplane.workspace.ssh_host"])
}

func TestAttachWorkspaceCommand_BuildsSSHCommand(t *testing.T) {
	configureWorkerObservability(t)

	host := "alpha.example.com"
	workspace := Workspace{ID: "ws-1", SSHHost: &host}
	cmd, err := attachWorkspaceCommand(workspace, func(name string) (string, error) {
		assert.Equal(t, "ssh", name)
		return "/usr/bin/ssh", nil
	})
	require.NoError(t, err)
	require.NotNil(t, cmd)

	require.Len(t, cmd.Args, 6)
	assert.Equal(t, "ssh", cmd.Args[0])
	assert.Equal(t, "-tt", cmd.Args[1])
	assert.Equal(t, host, cmd.Args[2])
	assert.Equal(t, "bash", cmd.Args[3])
	assert.Equal(t, "-lc", cmd.Args[4])

	script := cmd.Args[5]
	assert.Contains(t, script, "command -v tmux")
	assert.Contains(t, script, "command -v crush")
	assert.Contains(t, script, "command -v codeplane")
	assert.Less(t, strings.Index(script, "command -v codeplane"), strings.Index(script, "command -v crush"))
	assert.Contains(t, script, "git rev-parse --show-toplevel")
	assert.Contains(t, script, "tmux new-session -d -s")
	assert.Contains(t, script, "tmux attach-session -t")
	assert.Contains(t, script, "session='codeplane-worker'")
	assert.Contains(t, script, "persistent Codeplane worker")
	assert.Contains(t, script, "Neither codeplane nor crush is installed")

	attrs := requireWorkerSpanAttrs(t, "attach_prepare", "ok")
	assert.Equal(t, "ws-1", attrs["codeplane.workspace.id"])
	assert.Equal(t, host, attrs["codeplane.workspace.ssh_host"])
}

func TestWorkerAttachScriptWithSandbox_AutoModePrefersBubblewrapWhenAvailable(t *testing.T) {
	script := workerAttachScriptWithSandbox(defaultWorkerSessionName, workspaceSandboxAutoValue)
	assert.Contains(t, script, "command -v bwrap")
	assert.Contains(t, script, "command -v bubblewrap")
	assert.Contains(t, script, "launch_cmd=\"cd ${escaped_dir} && exec ${escaped_sandbox} --die-with-parent --new-session --proc /proc --dev-bind / / --chdir ${escaped_dir} ${escaped_bin}\"")
}

func TestWorkerAttachScriptWithSandbox_RequiredModeFailsWithoutBubblewrap(t *testing.T) {
	script := workerAttachScriptWithSandbox(defaultWorkerSessionName, workspaceSandboxBwrapValue)
	assert.Contains(t, script, "bubblewrap requested but neither bwrap nor bubblewrap is installed in the workspace")
}

func TestWorkerAttachScriptWithSandbox_OffModeSkipsSandboxSetup(t *testing.T) {
	script := workerAttachScriptWithSandbox(defaultWorkerSessionName, workspaceSandboxOffValue)
	assert.NotContains(t, script, "command -v bwrap")
	assert.NotContains(t, script, "bubblewrap")
}

func TestNormalizeWorkspaceSandboxMode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":         workspaceSandboxAutoValue,
		"auto":     workspaceSandboxAutoValue,
		"TRUE":     workspaceSandboxAutoValue,
		"bwrap":    workspaceSandboxBwrapValue,
		"required": workspaceSandboxBwrapValue,
		"off":      workspaceSandboxOffValue,
		"disabled": workspaceSandboxOffValue,
		"junk":     workspaceSandboxAutoValue,
	}
	for input, want := range tests {
		assert.Equal(t, want, normalizeWorkspaceSandboxMode(input), input)
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "''", shellQuote(""))
	assert.Equal(t, "'plain'", shellQuote("plain"))
	assert.Equal(t, `'has"quote'`, shellQuote(`has"quote`))
	assert.Equal(t, `'a'"'"'b'`, shellQuote("a'b"))
}

func configureWorkerObservability(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))
}

func requireWorkerSpanAttrs(t *testing.T, operation, result string) map[string]any {
	t.Helper()

	for _, span := range observability.RecentSpans(20) {
		if span.Name != "workspace.lifecycle" {
			continue
		}
		if span.Attributes["codeplane.workspace.operation"] == operation &&
			span.Attributes["codeplane.workspace.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing workspace lifecycle span operation=%q result=%q", operation, result)
	return nil
}

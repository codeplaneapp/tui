package jjhub

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachWorkspaceCommand_MissingSSHHost(t *testing.T) {
	t.Parallel()

	workspace := Workspace{ID: "ws-1"}
	cmd, err := attachWorkspaceCommand(workspace, func(string) (string, error) {
		t.Fatal("lookPath should not be called when SSH is unavailable")
		return "", nil
	})

	assert.Nil(t, cmd)
	require.ErrorIs(t, err, ErrWorkspaceSSHUnavailable)
}

func TestAttachWorkspaceCommand_MissingSSHBinary(t *testing.T) {
	t.Parallel()

	host := "alpha.example.com"
	workspace := Workspace{ID: "ws-1", SSHHost: &host}
	cmd, err := attachWorkspaceCommand(workspace, func(name string) (string, error) {
		assert.Equal(t, "ssh", name)
		return "", errors.New("missing")
	})

	assert.Nil(t, cmd)
	require.ErrorIs(t, err, ErrSSHBinaryUnavailable)
}

func TestAttachWorkspaceCommand_BuildsSSHCommand(t *testing.T) {
	t.Parallel()

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
	assert.Contains(t, script, "git rev-parse --show-toplevel")
	assert.Contains(t, script, "tmux new-session -d -s")
	assert.Contains(t, script, "tmux attach-session -t")
	assert.Contains(t, script, "session='crush-worker'")
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "''", shellQuote(""))
	assert.Equal(t, "'plain'", shellQuote("plain"))
	assert.Equal(t, `'has"quote'`, shellQuote(`has"quote`))
	assert.Equal(t, `'a'"'"'b'`, shellQuote("a'b"))
}

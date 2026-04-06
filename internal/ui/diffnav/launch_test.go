package diffnav

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/require"
)

func TestWriteCommandDiffToTempFile_WritesCommandOutput(t *testing.T) {
	t.Parallel()

	command := "printf 'hello\\n'"
	if runtime.GOOS == "windows" {
		command = "echo hello"
	}

	tmpPath, err := writeCommandDiffToTempFile(command, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(tmpPath) })

	content, err := os.ReadFile(tmpPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "hello")
}

func TestWriteCommandDiffToTempFile_UsesCwd(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	command := "pwd"
	if runtime.GOOS == "windows" {
		command = "cd"
	}

	tmpPath, err := writeCommandDiffToTempFile(command, cwd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(tmpPath) })

	content, err := os.ReadFile(tmpPath)
	require.NoError(t, err)
	require.Contains(t, strings.TrimSpace(string(content)), filepath.Clean(cwd))
}

func TestWriteCommandDiffToTempFile_ReturnsCommandStderr(t *testing.T) {
	t.Parallel()

	command := "echo boom >&2; exit 7"
	if runtime.GOOS == "windows" {
		command = "echo boom 1>&2 & exit /b 7"
	}

	tmpPath, err := writeCommandDiffToTempFile(command, "")
	require.Error(t, err)
	require.ErrorContains(t, err, "boom")
	if tmpPath != "" {
		_, statErr := os.Stat(tmpPath)
		require.True(t, errors.Is(statErr, os.ErrNotExist))
	}
}

func TestDiffnavInputCommand_UsesStdinRedirect(t *testing.T) {
	t.Parallel()

	inputPath := filepath.Join("tmp", "sample diff.patch")
	stderrPath := filepath.Join("tmp", "sample.stderr")
	binary, args := diffnavInputCommand(inputPath, stderrPath)
	require.NotEmpty(t, binary)
	require.NotEmpty(t, args)

	command := args[len(args)-1]
	require.Contains(t, command, "diffnav < ")
	require.Contains(t, command, shellQuote(inputPath))
	require.Contains(t, command, "2> ")
	require.Contains(t, command, shellQuote(stderrPath))
	require.NotContains(t, command, "--command")
}

func TestPagerCommand_UsesPagerEnv(t *testing.T) {
	t.Parallel()

	path := filepath.Join("tmp", "sample diff.patch")
	binary, args := pagerCommand(path, "pager --plain")
	require.NotEmpty(t, binary)
	require.NotEmpty(t, args)

	command := args[len(args)-1]
	require.Contains(t, command, "pager --plain")
	require.Contains(t, command, shellQuote(path))
}

func TestFinishDiffnavLaunch_FallsBackToPagerAndPreservesPatch(t *testing.T) {
	t.Parallel()

	tmpPath := filepath.Join(t.TempDir(), "sample.diff")
	stderrPath := tmpPath + ".stderr"
	require.NoError(t, os.WriteFile(tmpPath, []byte("diff --git"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte("Caught panic: divide by zero"), 0o644))

	msg := finishDiffnavLaunch(errors.New("exit status 1"), stderrPath, tmpPath, "/tmp/repo", "tag")

	fallback, ok := msg.(PagerFallbackMsg)
	require.True(t, ok, "expected PagerFallbackMsg, got %T", msg)
	require.Equal(t, tmpPath, fallback.Path)
	require.Equal(t, "/tmp/repo", fallback.Cwd)
	require.Equal(t, "tag", fallback.Tag)
	require.Contains(t, fallback.Reason, "divide by zero")

	_, err := os.Stat(tmpPath)
	require.NoError(t, err)
	_, err = os.Stat(stderrPath)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestFinishDiffnavLaunch_SuccessCleansTempFiles(t *testing.T) {
	t.Parallel()

	tmpPath := filepath.Join(t.TempDir(), "sample.diff")
	stderrPath := tmpPath + ".stderr"
	require.NoError(t, os.WriteFile(tmpPath, []byte("diff --git"), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte(""), 0o644))

	msg := finishDiffnavLaunch(nil, stderrPath, tmpPath, "/tmp/repo", "tag")

	handoffMsg, ok := msg.(handoff.HandoffMsg)
	require.True(t, ok, "expected handoff.HandoffMsg, got %T", msg)
	require.Equal(t, "tag", handoffMsg.Tag)
	require.Zero(t, handoffMsg.Result.ExitCode)
	require.NoError(t, handoffMsg.Result.Err)

	_, err := os.Stat(tmpPath)
	require.True(t, errors.Is(err, os.ErrNotExist))
	_, err = os.Stat(stderrPath)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestFinishPagerLaunch_RemovesPatchAndReturnsError(t *testing.T) {
	t.Parallel()

	tmpPath := filepath.Join(t.TempDir(), "sample.diff")
	require.NoError(t, os.WriteFile(tmpPath, []byte("diff --git"), 0o644))

	msg := finishPagerLaunch(errors.New("pager failed"), tmpPath, "tag")

	pagerErr, ok := msg.(PagerErrorMsg)
	require.True(t, ok, "expected PagerErrorMsg, got %T", msg)
	require.Equal(t, "tag", pagerErr.Tag)
	require.ErrorContains(t, pagerErr.Err, "pager failed")

	_, err := os.Stat(tmpPath)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

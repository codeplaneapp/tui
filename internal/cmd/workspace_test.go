package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunWorkspaceAttach_Usage(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "workspace"}
	cmd.SetContext(context.Background())

	err := runWorkspaceAttach(cmd, nil)
	require.EqualError(t, err, "usage: workspace attach <workspace-id>")
}

func TestRunWorkspaceAttach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace attach test uses POSIX shell scripts")
	}

	configureWorkspaceAttachObservability(t)

	dir := t.TempDir()
	writeFakeWorkspaceAttachBinaries(t, dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := &cobra.Command{Use: "workspace"}
	cmd.SetContext(context.Background())

	require.NoError(t, runWorkspaceAttach(cmd, []string{"ws-1"}))

	attrs := requireWorkspaceAttachSpanAttrs(t, "attach", "ok")
	require.Equal(t, "cli", attrs["codeplane.workspace.source"])
	require.Equal(t, "ws-1", attrs["codeplane.workspace.id"])
}

func TestRunWorkspaceAttach_MissingSSHHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace attach test uses POSIX shell scripts")
	}

	configureWorkspaceAttachObservability(t)

	dir := t.TempDir()
	writeFakeWorkspaceAttachBinaries(t, dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := &cobra.Command{Use: "workspace"}
	cmd.SetContext(context.Background())

	err := runWorkspaceAttach(cmd, []string{"ws-nohost"})
	require.ErrorIs(t, err, jjhub.ErrWorkspaceSSHUnavailable)

	attrs := requireWorkspaceAttachSpanAttrs(t, "attach", "error")
	require.Equal(t, "ws-nohost", attrs["codeplane.workspace.id"])
	require.Equal(t, jjhub.ErrWorkspaceSSHUnavailable.Error(), attrs["codeplane.error"])
}

func configureWorkspaceAttachObservability(t *testing.T) {
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

func requireWorkspaceAttachSpanAttrs(t *testing.T, operation, result string) map[string]any {
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

func writeFakeWorkspaceAttachBinaries(t *testing.T, dir string) {
	t.Helper()

	jjhubPath := filepath.Join(dir, "jjhub")
	sshPath := filepath.Join(dir, "ssh")

	const fakeJJHub = `#!/bin/sh
if [ "$1" = "workspace" ] && [ "$2" = "view" ] && [ "$3" = "ws-1" ]; then
  cat <<'EOF'
{"id":"ws-1","ssh_host":"alpha.example.com"}
EOF
  exit 0
fi
if [ "$1" = "workspace" ] && [ "$2" = "view" ] && [ "$3" = "ws-nohost" ]; then
  cat <<'EOF'
{"id":"ws-nohost","ssh_host":null}
EOF
  exit 0
fi
echo "unsupported fake jjhub invocation" >&2
exit 1
`

	const fakeSSH = `#!/bin/sh
exit 0
`

	require.NoError(t, os.WriteFile(jjhubPath, []byte(fakeJJHub), 0o755))
	require.NoError(t, os.WriteFile(sshPath, []byte(fakeSSH), 0o755))
}

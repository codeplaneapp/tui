package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/require"
)

func configureRootObservability(t *testing.T) {
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

func requireStartupSpanAttrs(t *testing.T, flow, source, result string) map[string]any {
	t.Helper()

	for _, span := range observability.RecentSpans(20) {
		if span.Name != "codeplane.startup."+flow {
			continue
		}
		if span.Attributes["codeplane.startup.flow"] == flow &&
			span.Attributes["codeplane.startup.source"] == source &&
			span.Attributes["codeplane.startup.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing startup span flow=%q source=%q result=%q", flow, source, result)
	return nil
}

func TestRecordLocalWorkspaceStartup_RecordsObservability(t *testing.T) {
	configureRootObservability(t)

	recordLocalWorkspaceStartup("/tmp/project", "/tmp/codeplane")

	attrs := requireStartupSpanAttrs(t, "workspace_mode", "local", "ok")
	require.Equal(t, "/tmp/project", attrs["codeplane.cwd"])
	require.Equal(t, "/tmp/codeplane", attrs["codeplane.data_dir"])
}

func TestRecordClientServerWorkspaceStartup_RecordsObservability(t *testing.T) {
	configureRootObservability(t)

	recordClientServerWorkspaceStartup("ws-123")

	attrs := requireStartupSpanAttrs(t, "workspace_mode", "client_server", "ok")
	require.Equal(t, "ws-123", attrs["codeplane.workspace_id"])
}

func TestRecordServerAutostart_RecordsSuccess(t *testing.T) {
	configureRootObservability(t)

	recordServerAutostart("unix", "/tmp/codeplane.sock", 250*time.Millisecond, nil)

	attrs := requireStartupSpanAttrs(t, "server_autostart", "unix", "ok")
	require.Equal(t, "/tmp/codeplane.sock", attrs["codeplane.server.host"])
	require.EqualValues(t, 250, attrs["codeplane.duration_ms"])
}

func TestRecordServerAutostart_RecordsError(t *testing.T) {
	configureRootObservability(t)

	recordServerAutostart("unix", "/tmp/codeplane.sock", 100*time.Millisecond, errors.New("spawn failed"))

	attrs := requireStartupSpanAttrs(t, "server_autostart", "unix", "error")
	require.Equal(t, "/tmp/codeplane.sock", attrs["codeplane.server.host"])
	require.Equal(t, "spawn failed", attrs["codeplane.error"])
}

func TestRecordServerRestart_RecordsObservability(t *testing.T) {
	configureRootObservability(t)

	recordServerRestart("unix", "/tmp/codeplane.sock", "1.0.0", "1.1.0", 500*time.Millisecond)

	attrs := requireStartupSpanAttrs(t, "server_restart", "unix", "ok")
	require.Equal(t, "/tmp/codeplane.sock", attrs["codeplane.server.host"])
	require.Equal(t, "1.0.0", attrs["codeplane.server.version"])
	require.Equal(t, "1.1.0", attrs["codeplane.client.version"])
	require.EqualValues(t, 500, attrs["codeplane.duration_ms"])
}

func TestRecordConfigSelection_RecordsLegacyPath(t *testing.T) {
	configureRootObservability(t)

	recordConfigSelection("global_config", "/tmp/crush/crush.json")

	attrs := requireStartupSpanAttrs(t, "config_source", "global_config", "legacy")
	require.Equal(t, "/tmp/crush/crush.json", attrs["codeplane.config.path"])
}

package model

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/views"
	"github.com/stretchr/testify/require"
)

func TestHandleViewResult_OpenSnapshots_RecordsObservability(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	ui := newShortcutTestUI()
	ui.viewRouter = views.NewRouter()
	ui.smithersClient = smithers.NewClient()
	ui.status = NewStatus(ui.com, ui)
	ui.chat = NewChat(ui.com)

	cmds := ui.handleViewResult(views.OpenSnapshotsMsg{
		RunID:  "run-123",
		Source: views.SnapshotsOpenSourceRuns,
	})
	require.NotEmpty(t, cmds)
	require.Equal(t, uiSmithersView, ui.state)

	spans := observability.RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.navigation", spans[0].Name)
	require.Equal(t, "runs", spans[0].Attributes["crush.ui.entrypoint"])
	require.Equal(t, "snapshots", spans[0].Attributes["crush.ui.target"])
	require.Equal(t, "ok", spans[0].Attributes["crush.ui.result"])
	require.Equal(t, "run-123", spans[0].Attributes["crush.run_id"])
}

func TestHandleNavigateToView_Timeline_RecordsMissingRunContext(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  16,
		TraceSampleRatio: 1,
	}))

	ui := newShortcutTestUI()

	cmd := ui.handleNavigateToView(NavigateToViewMsg{View: "timeline"})
	require.NotNil(t, cmd)

	spans := observability.RecentSpans(10)
	require.Len(t, spans, 1)
	require.Equal(t, "ui.navigation", spans[0].Name)
	require.Equal(t, "global", spans[0].Attributes["crush.ui.entrypoint"])
	require.Equal(t, "timeline", spans[0].Attributes["crush.ui.target"])
	require.Equal(t, "missing_run_context", spans[0].Attributes["crush.ui.result"])
}

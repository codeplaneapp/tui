package views

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/require"
)

func TestChatView_ImplementsView(t *testing.T) {
	var _ View = (*ChatView)(nil)
}

func TestNewChatView_DefaultsToSmithers(t *testing.T) {
	t.Parallel()

	v := NewChatView(nil)
	require.Len(t, v.targets, 1)
	require.Equal(t, "smithers", v.targets[0].id)
	require.True(t, v.targets[0].recommended)
	require.Contains(t, v.View(), "Smithers")
}

func TestChatView_Init_NoClient_RecordsPickerOpen(t *testing.T) {
	configureChatViewObservability(t)

	v := NewChatView(nil)
	cmd := v.Init()
	require.Nil(t, cmd)

	attrs := requireChatSpanAttrs(t, "dashboard", "chat_picker", "ok")
	require.Equal(t, false, attrs["crush.chat.has_client"])
	require.EqualValues(t, 1, attrs["crush.chat.total_targets"])
}

func TestChatView_LoadedMsg_PopulatesUsableAgentsOnlyAndRecordsDiscovery(t *testing.T) {
	configureChatViewObservability(t)

	v := NewChatView(nil)
	updated, cmd := v.Update(chatTargetsLoadedMsg{agents: []smithers.Agent{
		{ID: "opencode", Name: "OpenCode", Usable: true, BinaryPath: "/tmp/opencode", Status: "binary-only", Roles: []string{"coding"}},
		{ID: "codex", Name: "Codex", Usable: false, BinaryPath: "/tmp/codex"},
	}})
	require.Nil(t, cmd)

	cv := updated.(*ChatView)
	require.False(t, cv.loading)
	require.Len(t, cv.targets, 2)
	require.Equal(t, "smithers", cv.targets[0].id)
	require.Equal(t, "opencode", cv.targets[1].id)
	require.NotContains(t, cv.View(), "Codex")

	attrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_discovery", "ok")
	require.EqualValues(t, 2, attrs["crush.chat.total_targets"])
	require.EqualValues(t, 1, attrs["crush.chat.external_targets"])
}

func TestChatView_TargetLoadError_RecordsDiscoveryFailure(t *testing.T) {
	configureChatViewObservability(t)

	v := NewChatView(nil)
	updated, cmd := v.Update(chatTargetsErrorMsg{err: errors.New("agent discovery failed")})
	require.Nil(t, cmd)

	cv := updated.(*ChatView)
	require.Error(t, cv.err)

	attrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_discovery", "error")
	require.Equal(t, "*errors.errorString", attrs["crush.chat.error_type"])
}

func TestChatView_EnterOnSmithers_ReturnsOpenChatMsg(t *testing.T) {
	configureChatViewObservability(t)

	v := NewChatView(nil)
	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, OpenChatMsg{}, msg)
	require.Same(t, v, updated)

	attrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_select", "ok")
	require.Equal(t, "smithers", attrs["crush.chat.target"])
	require.Equal(t, "smithers", attrs["crush.chat.kind"])
}

func TestChatView_EnterOnExternalAgent_HandlesSuccessfulHandoff(t *testing.T) {
	configureChatViewObservability(t)

	scriptPath := writeFakeChatViewAgent(t)
	v := NewChatView(nil)
	updated, _ := v.Update(chatTargetsLoadedMsg{agents: []smithers.Agent{
		{
			ID:         "opencode",
			Name:       "OpenCode",
			Usable:     true,
			BinaryPath: scriptPath,
			Status:     "binary-only",
			Roles:      []string{"coding", "chat"},
		},
	}})

	cv := updated.(*ChatView)
	cv.cursor = 1

	updated, cmd := cv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	inflight := updated.(*ChatView)
	require.True(t, inflight.launching)
	require.Equal(t, "OpenCode", inflight.launchingName)
	handoffMsg := handoff.HandoffMsg{
		Tag:    "opencode",
		Result: handoff.HandoffResult{ExitCode: 0},
	}

	updated, followCmd := inflight.Update(handoffMsg)
	require.Nil(t, followCmd)

	done := updated.(*ChatView)
	require.False(t, done.launching)
	require.NoError(t, done.err)

	selectAttrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_select", "ok")
	require.Equal(t, "opencode", selectAttrs["crush.chat.target"])

	handoffAttrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_handoff", "ok")
	require.Equal(t, "opencode", handoffAttrs["crush.chat.target"])
}

func TestChatView_ExternalHandoffErrorIsRendered(t *testing.T) {
	configureChatViewObservability(t)

	v := NewChatView(nil)
	updated, _ := v.Update(chatTargetsLoadedMsg{agents: []smithers.Agent{
		{
			ID:         "codex",
			Name:       "Codex",
			Usable:     true,
			BinaryPath: filepath.Join(t.TempDir(), "missing-codex"),
			Status:     "binary-only",
			Roles:      []string{"coding"},
		},
	}})

	cv := updated.(*ChatView)
	cv.cursor = 1

	updated, cmd := cv.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	handoffMsg := handoff.HandoffMsg{
		Tag: "codex",
		Result: handoff.HandoffResult{
			ExitCode: 1,
			Err:      errors.New("launch failed"),
		},
	}
	require.Error(t, handoffMsg.Result.Err)

	updated, _ = updated.(*ChatView).Update(handoffMsg)
	failed := updated.(*ChatView)
	require.Error(t, failed.err)
	require.Contains(t, failed.View(), "Error:")

	handoffAttrs := requireChatSpanAttrs(t, "chat_picker", "chat_target_handoff", "error")
	require.Equal(t, "codex", handoffAttrs["crush.chat.target"])
}

func configureChatViewObservability(t *testing.T) {
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

func requireChatSpanAttrs(t *testing.T, entrypoint, target, result string) map[string]any {
	t.Helper()

	for _, span := range observability.RecentSpans(20) {
		if span.Name != "ui.navigation" {
			continue
		}
		if span.Attributes["codeplane.ui.entrypoint"] == entrypoint &&
			span.Attributes["codeplane.ui.target"] == target &&
			span.Attributes["codeplane.ui.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing ui.navigation span entrypoint=%q target=%q result=%q", entrypoint, target, result)
	return nil
}

func writeFakeChatViewAgent(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "opencode")
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

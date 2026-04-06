package views

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatView_ImplementsView(t *testing.T) {
	t.Parallel()

	var _ View = (*ChatView)(nil)
}

func TestNewChatView_StartsWithSmithersTarget(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	require.Len(t, v.targets, 1)
	assert.Equal(t, "smithers", v.targets[0].id)
	assert.True(t, v.targets[0].recommended)
}

func TestChatView_LoadedMsg_AddsUsableAgents(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	updated, _ := v.Update(chatTargetsLoadedMsg{
		agents: []smithers.Agent{
			testAgent("claude-code", "Claude Code", "likely-subscription", true),
			testAgent("codex", "Codex", "unavailable", false),
		},
	})

	cv := updated.(*ChatView)
	require.Len(t, cv.targets, 2)
	assert.Equal(t, "smithers", cv.targets[0].id)
	assert.Equal(t, "claude-code", cv.targets[1].id)
}

func TestChatView_EnterOnSmithers_ReturnsOpenChatMsg(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(OpenChatMsg)
	assert.True(t, ok)
}

func TestChatView_EnterOnExternalAgent_StartsLaunch(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	v.loading = false
	v.targets = []chatTarget{
		buildChatTargets(nil)[0],
		{
			kind:   chatTargetAgent,
			id:     "claude-code",
			name:   "Claude Code",
			status: "likely-subscription",
			binary: "/tmp/claude",
			usable: true,
		},
	}
	v.cursor = 1

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	cv := updated.(*ChatView)
	assert.True(t, cv.launching)
	assert.Equal(t, "Claude Code", cv.launchingName)
}

func TestChatView_HandoffError_SetsErrAndReloads(t *testing.T) {
	t.Parallel()

	v := NewChatView(nil)
	v.launching = true
	v.launchingName = "Claude Code"

	updated, cmd := v.Update(handoff.HandoffMsg{
		Tag:    "claude-code",
		Result: handoff.HandoffResult{ExitCode: 1, Err: errors.New("missing binary")},
	})
	assert.Nil(t, cmd)

	cv := updated.(*ChatView)
	assert.False(t, cv.launching)
	assert.Contains(t, cv.err.Error(), "missing binary")
}

func TestChatView_View_ShowsSmithersAndAgent(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	v.loading = false
	v.width = 100
	v.targets = buildChatTargets([]smithers.Agent{
		testAgent("opencode", "OpenCode", "binary-only", true),
	})

	out := v.View()
	assert.Contains(t, out, "SMITHERS › Start Chat")
	assert.Contains(t, out, "Smithers")
	assert.Contains(t, out, "OpenCode")
	assert.Contains(t, out, "Recommended")
}

func TestChatView_Refresh_ReloadsTargets(t *testing.T) {
	t.Parallel()

	v := NewChatView(smithers.NewClient())
	v.loading = false

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd)

	cv := updated.(*ChatView)
	assert.True(t, cv.loading)
}

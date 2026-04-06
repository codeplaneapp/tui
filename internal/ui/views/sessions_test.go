package views

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSessionsView() *SessionsView {
	return NewSessionsView(smithers.NewClient())
}

func testSessionRun(id, workflow string, status smithers.RunStatus) smithers.RunSummary {
	startedAtMs := time.Now().Add(-5 * time.Minute).UnixMilli()
	return smithers.RunSummary{
		RunID:        id,
		WorkflowName: workflow,
		Status:       status,
		StartedAtMs:  &startedAtMs,
		Summary: map[string]int{
			"finished": 1,
			"running":  1,
		},
	}
}

func TestSessionsView_InitReturnsFetchCmd(t *testing.T) {
	v := newTestSessionsView()
	cmd := v.Init()
	assert.NotNil(t, cmd)
}

func TestSessionsView_JKNavigation(t *testing.T) {
	v := newTestSessionsView()
	updated, _ := v.Update(sessionsLoadedMsg{
		runs: []smithers.RunSummary{
			testSessionRun("run-1", "Interactive", smithers.RunStatusRunning),
			testSessionRun("run-2", "Review", smithers.RunStatusFinished),
			testSessionRun("run-3", "Plan", smithers.RunStatusFailed),
		},
	})
	v = updated.(*SessionsView)
	assert.Equal(t, 0, v.cursor)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'j'})
	v = updated.(*SessionsView)
	assert.Equal(t, 1, v.cursor)

	updated, _ = v.Update(tea.KeyPressMsg{Code: 'k'})
	v = updated.(*SessionsView)
	assert.Equal(t, 0, v.cursor)
}

func TestSessionsView_EscEmitsPopViewMsg(t *testing.T) {
	v := newTestSessionsView()
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(PopViewMsg)
	assert.True(t, ok, "Esc should emit PopViewMsg")
}

func TestSessionsView_ViewRendersLoadingState(t *testing.T) {
	v := newTestSessionsView()
	v.SetSize(100, 30)

	out := v.View()
	assert.Contains(t, out, "Loading sessions")
}

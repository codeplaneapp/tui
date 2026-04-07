package model

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/util"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestStatusDraw_WithSmithersSummary(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})
	status.SetSmithersStatus(&SmithersStatus{
		ActiveRuns:       3,
		PendingApprovals: 1,
	})

	out := renderStatus(t, status, 90)
	require.Contains(t, out, "3 runs · 1 approval")
}

func TestStatusDraw_WithoutSmithersSummary(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})

	out := renderStatus(t, status, 90)
	require.NotContains(t, out, "approval")
	require.NotContains(t, out, "runs")
}

func TestStatusDraw_InfoMessageTakesPriority(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})
	status.SetSmithersStatus(&SmithersStatus{
		ActiveRuns:       3,
		PendingApprovals: 1,
	})
	status.SetInfoMsg(util.NewInfoMsg("status message"))

	out := renderStatus(t, status, 90)
	require.Contains(t, out, "status message")
	require.NotContains(t, out, "3 runs")
	require.NotContains(t, out, "1 approval")
}

func TestStatusDraw_PluralApprovals(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})
	status.SetSmithersStatus(&SmithersStatus{
		ActiveRuns:       4,
		PendingApprovals: 3,
	})

	out := renderStatus(t, status, 120)
	require.Contains(t, out, "4 runs · 3 approvals")
}

func TestStatusDraw_OnlyPendingApprovals(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})
	// ActiveRuns includes WaitingApproval, but here we simulate a case where
	// PendingApprovals is set without explicit active run count.
	status.SetSmithersStatus(&SmithersStatus{
		ActiveRuns:       0,
		PendingApprovals: 2,
	})

	out := renderStatus(t, status, 90)
	// Status bar only renders PendingApprovals when > 0, even with no active run count.
	require.Contains(t, out, "2 approvals")
	require.NotContains(t, out, "runs")
}

func TestStatusDraw_SingleRun_NoApprovals(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	status := NewStatus(com, statusTestKeyMap{})
	status.SetSmithersStatus(&SmithersStatus{
		ActiveRuns:       1,
		PendingApprovals: 0,
	})

	out := renderStatus(t, status, 90)
	require.Contains(t, out, "1 run")
	require.NotContains(t, out, "approval")
}

// --- formatSmithersSummary unit tests (pure string formatting) ---

func TestFormatSmithersSummary_RunsAndApprovals(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 3, PendingApprovals: 1}}
	require.Equal(t, "3 runs · 1 approval", s.formatSmithersSummary())
}

func TestFormatSmithersSummary_OnlyRuns(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 1, PendingApprovals: 0}}
	require.Equal(t, "1 run", s.formatSmithersSummary())
}

func TestFormatSmithersSummary_OnlyApprovals(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 0, PendingApprovals: 5}}
	require.Equal(t, "5 approvals", s.formatSmithersSummary())
}

func TestFormatSmithersSummary_NothingActive(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 0, PendingApprovals: 0}}
	require.Equal(t, "", s.formatSmithersSummary())
}

func TestFormatSmithersSummary_PluralRuns(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 10, PendingApprovals: 0}}
	require.Equal(t, "10 runs", s.formatSmithersSummary())
}

func TestFormatSmithersSummary_SingularApproval(t *testing.T) {
	t.Parallel()

	s := &Status{smithersStatus: &SmithersStatus{ActiveRuns: 0, PendingApprovals: 1}}
	require.Equal(t, "1 approval", s.formatSmithersSummary())
}

func renderStatus(t *testing.T, s *Status, width int) string {
	t.Helper()

	s.SetWidth(width)
	canvas := uv.NewScreenBuffer(width, 1)
	s.Draw(canvas, canvas.Bounds())
	rendered := strings.ReplaceAll(canvas.Render(), "\r", "")
	return ansi.Strip(rendered)
}

type statusTestKeyMap struct{}

func (statusTestKeyMap) ShortHelp() []key.Binding  { return nil }
func (statusTestKeyMap) FullHelp() [][]key.Binding { return nil }

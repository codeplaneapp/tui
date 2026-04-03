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

package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
)

// runsInfo renders the Smithers active runs status section.
func (m *UI) runsInfo(width, maxItems int, isSection bool) string {
	t := m.com.Styles
	if m.smithersStatus == nil {
		return ""
	}

	title := t.ResourceGroupTitle.Render("Runs")
	if isSection {
		title = common.Section(t, title, width)
	}

	var parts []string
	if m.smithersStatus.ActiveRuns > 0 {
		activeStyle := t.ResourceOnlineIcon
		parts = append(parts, activeStyle.Render(fmt.Sprintf("%d active", m.smithersStatus.ActiveRuns)))
	}
	if m.smithersStatus.PendingApprovals > 0 {
		pendingStyle := t.ResourceBusyIcon
		parts = append(parts, pendingStyle.Render(fmt.Sprintf("%d pending approval", m.smithersStatus.PendingApprovals)))
	}

	if len(parts) == 0 {
		return title + "\n" + t.ResourceAdditionalText.Render("None")
	}

	var b strings.Builder
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, parts...))

	return b.String()
}

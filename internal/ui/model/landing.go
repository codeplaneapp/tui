package model

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/logo"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/workspace"
	"github.com/charmbracelet/ultraviolet/layout"
)

// selectedLargeModel returns the currently selected large language model from
// the agent coordinator, if one exists.
func (m *UI) selectedLargeModel() *workspace.AgentModel {
	if m.com.Workspace.AgentIsReady() {
		model := m.com.Workspace.AgentModel()
		return &model
	}
	return nil
}

// landingView renders the landing page view showing the current working
// directory, model information, and LSP/MCP status in a two-column layout.
func (m *UI) landingView() string {
	t := m.com.Styles
	width := m.layout.main.Dx()
	cwd := common.PrettyPath(t, m.com.Workspace.WorkingDir(), width)

	logoView := logo.LargeRender(t, width)
	var systemStatus string
	if m.com.Workspace.AgentIsReady() {
		systemStatus = styles.ApplyBoldForegroundGrad(t, "SMITHERS", t.Primary, t.Secondary)
	} else {
		systemStatus = m.systemAnim.Render()
	}

	// Smithers Mode indicator.
	smithersMode := ""
	if m.com.Config().Smithers != nil {
		smithersMode = t.ResourceOnlineIcon.Render("Smithers Agent Mode")
	}

	modelBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderColor).
		Padding(0, 1).
		MarginBottom(1).
		Render(m.modelInfo(width - 4))

	parts := []string{
		logoView,
		"",
		cwd,
		systemStatus,
	}
	if smithersMode != "" {
		parts = append(parts, smithersMode)
	}
	parts = append(parts, "", modelBox)

	infoSection := lipgloss.JoinVertical(lipgloss.Left, parts...)

	_, remainingHeightArea := layout.SplitVertical(m.layout.main, layout.Fixed(lipgloss.Height(infoSection)+1))

	columnWidth := min(30, (width-1)/2)
	maxItems := max(1, remainingHeightArea.Dy())

	var leftSection, rightSection string
	if m.com.Config().Smithers != nil {
		leftSection = m.mcpInfo(columnWidth, maxItems, false)
		rightSection = m.runsInfo(columnWidth, maxItems, false)
	} else {
		leftSection = m.lspInfo(columnWidth, maxItems, false)
		rightSection = m.mcpInfo(columnWidth, maxItems, false)
	}

	content := lipgloss.JoinHorizontal(lipgloss.Left, leftSection, " ", rightSection)

	return lipgloss.NewStyle().
		Width(width).
		Height(m.layout.main.Dy() - 1).
		PaddingTop(1).
		Render(
			lipgloss.JoinVertical(lipgloss.Left, infoSection, "", content),
		)
}

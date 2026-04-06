package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/fsext"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	headerDiag           = " "
	minHeaderDiags       = 1
	leftPadding          = 1
	rightPadding         = 1
	diagToDetailsSpacing = 1 // space between diagonal pattern and details section
)

// SmithersStatus holds Smithers runtime metrics displayed in header and
// status-bar surfaces.
type SmithersStatus struct {
	ActiveRuns       int
	PendingApprovals int
	MCPConnected     bool
	MCPServerName    string
	MCPToolCount     int
}

type header struct {
	// cached logo and compact logo
	logo        string
	compactLogo string

	smithersStatus *SmithersStatus

	com     *common.Common
	width   int
	compact bool
}

// newHeader creates a new header model.
func newHeader(com *common.Common) *header {
	h := &header{
		com: com,
	}
	t := com.Styles
	h.compactLogo = styles.ApplyBoldForegroundGrad(t, "CRUSH", t.Secondary, t.Primary) + " "
	return h
}

func (h *header) SetSmithersStatus(status *SmithersStatus) {
	h.smithersStatus = status
}

// drawHeader draws the header for the given session.
func (h *header) drawHeader(
	scr uv.Screen,
	area uv.Rectangle,
	session *session.Session,
	compact bool,
	detailsOpen bool,
	width int,
) {
	t := h.com.Styles
	if width != h.width || compact != h.compact {
		h.logo = renderLogo(h.com.Styles, compact, width)
	}

	h.width = width
	h.compact = compact

	if !compact || session == nil {
		uv.NewStyledString(h.logo).Draw(scr, area)
		return
	}

	if session.ID == "" {
		return
	}

	var b strings.Builder
	b.WriteString(h.compactLogo)

	availDetailWidth := width - leftPadding - rightPadding - lipgloss.Width(b.String()) - minHeaderDiags - diagToDetailsSpacing
	lspErrorCount := 0
	for _, info := range h.com.Workspace.LSPGetStates() {
		lspErrorCount += info.DiagnosticCount
	}
	details := renderHeaderDetails(
		h.com,
		session,
		lspErrorCount,
		detailsOpen,
		availDetailWidth,
		h.smithersStatus,
	)

	remainingWidth := width -
		lipgloss.Width(b.String()) -
		lipgloss.Width(details) -
		leftPadding -
		rightPadding -
		diagToDetailsSpacing

	if remainingWidth > 0 {
		b.WriteString(t.Header.Diagonals.Render(
			strings.Repeat(headerDiag, max(minHeaderDiags, remainingWidth)),
		))
		b.WriteString(" ")
	}

	b.WriteString(details)

	view := uv.NewStyledString(
		t.Base.Padding(0, rightPadding, 0, leftPadding).Render(b.String()))
	view.Draw(scr, area)
}

// renderHeaderDetails renders the details section of the header.
func renderHeaderDetails(
	com *common.Common,
	session *session.Session,
	lspErrorCount int,
	detailsOpen bool,
	availWidth int,
	smithersStatus *SmithersStatus,
) string {
	t := com.Styles

	var parts []string

	if lspErrorCount > 0 {
		parts = append(parts, t.LSP.ErrorDiagnostic.Render(fmt.Sprintf("%s%d", styles.LSPErrorIcon, lspErrorCount)))
	}

	agentCfg := com.Config().Agents[config.AgentCoder]
	model := com.Config().GetModelByType(agentCfg.Model)
	if model != nil && model.ContextWindow > 0 {
		percentage := (float64(session.CompletionTokens+session.PromptTokens) / float64(model.ContextWindow)) * 100
		formattedPercentage := t.Header.Percentage.Render(fmt.Sprintf("%d%%", int(percentage)))
		parts = append(parts, formattedPercentage)
	}

	const keystroke = "ctrl+d"
	if detailsOpen {
		parts = append(parts, t.Header.Keystroke.Render(keystroke)+t.Header.KeystrokeTip.Render(" close"))
	} else {
		parts = append(parts, t.Header.Keystroke.Render(keystroke)+t.Header.KeystrokeTip.Render(" open "))
	}

	if smithersStatus != nil {
		serverName := strings.TrimSpace(smithersStatus.MCPServerName)
		if serverName == "" {
			serverName = "crush"
		}

		indicator := "○"
		connection := "disconnected"
		connectionStyle := t.Muted
		if smithersStatus.MCPConnected {
			indicator = "●"
			connection = "connected"
			connectionStyle = t.Base.Foreground(t.Primary)
		}

		mcpStatus := fmt.Sprintf("%s %s %s", indicator, serverName, connection)
		if smithersStatus.MCPConnected && smithersStatus.MCPToolCount > 0 {
			mcpStatus = fmt.Sprintf("%s (%d tools)", mcpStatus, smithersStatus.MCPToolCount)
		}
		parts = append(parts, connectionStyle.Render(mcpStatus))

		if smithersStatus.ActiveRuns > 0 {
			parts = append(parts, t.Muted.Render(fmt.Sprintf("%d active", smithersStatus.ActiveRuns)))
		}

		if smithersStatus.PendingApprovals > 0 {
			approvalNoun := "approval"
			if smithersStatus.PendingApprovals != 1 {
				approvalNoun = "approvals"
			}
			pendingText := fmt.Sprintf("⚠ %d pending %s", smithersStatus.PendingApprovals, approvalNoun)
			// Escalate to red (Error color) when 5+ approvals are pending; yellow otherwise.
			badgeColor := t.Warning
			if smithersStatus.PendingApprovals >= 5 {
				badgeColor = t.Error
			}
			parts = append(parts, t.Base.Foreground(badgeColor).Bold(true).Render(pendingText))
		}
	}

	dot := t.Header.Separator.Render(" • ")
	metadata := strings.Join(parts, dot)
	metadata = dot + metadata

	const dirTrimLimit = 4
	cwd := fsext.DirTrim(fsext.PrettyPath(com.Workspace.WorkingDir()), dirTrimLimit)
	cwd = t.Header.WorkingDir.Render(cwd)

	result := cwd + metadata
	return ansi.Truncate(result, max(0, availWidth), "…")
}

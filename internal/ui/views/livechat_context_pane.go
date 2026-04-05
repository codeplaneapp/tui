package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// LiveChatContextPane is a side-pane that displays run context alongside the
// live chat transcript.  It implements components.Pane so it can be embedded
// in a SplitPane.
type LiveChatContextPane struct {
	runID  string
	run    *smithers.RunSummary
	width  int
	height int
}

// Compile-time check: LiveChatContextPane must satisfy components.Pane.
var _ components.Pane = (*LiveChatContextPane)(nil)

// newLiveChatContextPane creates an uninitialised context pane for the given run.
func newLiveChatContextPane(runID string) *LiveChatContextPane {
	return &LiveChatContextPane{runID: runID}
}

// Init satisfies components.Pane; returns nil (no startup commands needed).
func (p *LiveChatContextPane) Init() tea.Cmd { return nil }

// Update satisfies components.Pane; syncs run metadata from liveChatRunLoadedMsg.
func (p *LiveChatContextPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	if m, ok := msg.(liveChatRunLoadedMsg); ok {
		p.run = m.run
	}
	return p, nil
}

// SetSize satisfies components.Pane.
func (p *LiveChatContextPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// View renders the context pane.
func (p *LiveChatContextPane) View() string {
	w := p.width
	if w < 10 {
		w = 10
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	labelStyle := lipgloss.NewStyle().Faint(true)
	valueStyle := lipgloss.NewStyle()
	divStyle := lipgloss.NewStyle().Faint(true)

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Context"))
	sb.WriteString("\n")
	sb.WriteString(divStyle.Render(strings.Repeat("─", w)))
	sb.WriteString("\n")

	if p.run == nil {
		sb.WriteString(labelStyle.Render("Loading..."))
		sb.WriteString("\n")
		return sb.String()
	}

	run := p.run

	// Run ID (truncated to 8 chars)
	runIDShort := run.RunID
	if len(runIDShort) > 8 {
		runIDShort = runIDShort[:8]
	}
	sb.WriteString(labelStyle.Render("Run:    "))
	sb.WriteString(valueStyle.Render(runIDShort))
	sb.WriteString("\n")

	// Workflow name
	if run.WorkflowName != "" {
		sb.WriteString(labelStyle.Render("Flow:   "))
		sb.WriteString(valueStyle.Render(truncateStr(run.WorkflowName, w-8)))
		sb.WriteString("\n")
	}

	// Status with colour coding
	statusStr := string(run.Status)
	statusStyle := valueStyle
	switch run.Status {
	case smithers.RunStatusRunning:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case smithers.RunStatusFailed:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	case smithers.RunStatusFinished:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Faint(true)
	case smithers.RunStatusWaitingApproval, smithers.RunStatusWaitingEvent:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	}
	sb.WriteString(labelStyle.Render("Status: "))
	sb.WriteString(statusStyle.Render(statusStr))
	sb.WriteString("\n")

	// Elapsed time
	if run.StartedAtMs != nil {
		var elapsed time.Duration
		if run.FinishedAtMs != nil {
			elapsed = time.Duration(*run.FinishedAtMs-*run.StartedAtMs) * time.Millisecond
		} else {
			elapsed = time.Since(time.UnixMilli(*run.StartedAtMs)).Round(time.Second)
		}
		sb.WriteString(labelStyle.Render("Elapsed:"))
		sb.WriteString(valueStyle.Render(" " + fmtDuration(elapsed)))
		sb.WriteString("\n")
	}

	// Started-at relative age
	if run.StartedAtMs != nil {
		age := fmtRelativeAge(*run.StartedAtMs)
		if age != "" {
			sb.WriteString(labelStyle.Render("Started:"))
			sb.WriteString(valueStyle.Render(" " + age))
			sb.WriteString("\n")
		}
	}

	// Node summary counts
	if len(run.Summary) > 0 {
		sb.WriteString("\n")
		sb.WriteString(divStyle.Render(strings.Repeat("─", w)))
		sb.WriteString("\n")
		sb.WriteString(labelStyle.Render("Nodes"))
		sb.WriteString("\n")
		for state, count := range run.Summary {
			sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-12s", state)))
			sb.WriteString(valueStyle.Render(fmt.Sprintf("%d", count)))
			sb.WriteString("\n")
		}
	}

	// Error reason
	if errReason := run.ErrorReason(); errReason != "" {
		sb.WriteString("\n")
		sb.WriteString(divStyle.Render(strings.Repeat("─", w)))
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error:"))
		sb.WriteString("\n")
		for _, line := range wrapLineToWidth(errReason, w-2) {
			sb.WriteString(lipgloss.NewStyle().Faint(true).Render("  "+line))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

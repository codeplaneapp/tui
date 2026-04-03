package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*ApprovalsView)(nil)

type approvalsLoadedMsg struct {
	approvals []smithers.Approval
}

type approvalsErrorMsg struct {
	err error
}

// ApprovalsView displays a split-pane approvals queue with context details.
// Left pane: navigable list of approvals. Right pane: context for the selected item.
type ApprovalsView struct {
	client    *smithers.Client
	approvals []smithers.Approval
	cursor    int
	width     int
	height    int
	loading   bool
	err       error
}

// NewApprovalsView creates a new approvals view.
func NewApprovalsView(client *smithers.Client) *ApprovalsView {
	return &ApprovalsView{
		client:  client,
		loading: true,
	}
}

// Init loads approvals from the client.
func (v *ApprovalsView) Init() tea.Cmd {
	return func() tea.Msg {
		approvals, err := v.client.ListPendingApprovals(context.Background())
		if err != nil {
			return approvalsErrorMsg{err: err}
		}
		return approvalsLoadedMsg{approvals: approvals}
	}
}

// Update handles messages for the approvals view.
func (v *ApprovalsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case approvalsLoadedMsg:
		v.approvals = msg.approvals
		v.loading = false
		return v, nil

	case approvalsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.approvals)-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			return v, v.Init()
		}
	}
	return v, nil
}

// View renders the split-pane approvals view.
func (v *ApprovalsView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Approvals")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading approvals...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.approvals) == 0 {
		b.WriteString("  No pending approvals.\n")
		return b.String()
	}

	// Split-pane: list on left, details on right.
	// Use ~35% for list, rest for details, with a divider column.
	listWidth := 30
	dividerWidth := 3
	detailWidth := v.width - listWidth - dividerWidth
	if v.width < 80 || detailWidth < 20 {
		// Compact: show list + inline detail below
		b.WriteString(v.renderListCompact())
		return b.String()
	}

	listContent := v.renderList(listWidth)
	detailContent := v.renderDetail(detailWidth)

	divider := lipgloss.NewStyle().Faint(true).Render(" │ ")

	// Join list and detail side by side, line by line.
	listLines := strings.Split(listContent, "\n")
	detailLines := strings.Split(detailContent, "\n")

	maxLines := len(listLines)
	if len(detailLines) > maxLines {
		maxLines = len(detailLines)
	}

	// Cap to available height (leave 3 lines for header + padding).
	availHeight := v.height - 3
	if availHeight > 0 && maxLines > availHeight {
		maxLines = availHeight
	}

	for i := 0; i < maxLines; i++ {
		left := ""
		if i < len(listLines) {
			left = listLines[i]
		}
		right := ""
		if i < len(detailLines) {
			right = detailLines[i]
		}
		// Pad left column to fixed width.
		left = padRight(left, listWidth)
		b.WriteString(left + divider + right + "\n")
	}

	return b.String()
}

// renderList renders the approval list pane constrained to the given width.
func (v *ApprovalsView) renderList(width int) string {
	var b strings.Builder

	sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true)

	// Split into pending and resolved.
	var pending, resolved []int
	for i, a := range v.approvals {
		if a.Status == "pending" {
			pending = append(pending, i)
		} else {
			resolved = append(resolved, i)
		}
	}

	if len(pending) > 0 {
		b.WriteString(sectionHeader.Render("Pending") + "\n")
		for _, idx := range pending {
			b.WriteString(v.renderListItem(idx, width))
		}
	}

	if len(resolved) > 0 {
		if len(pending) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(sectionHeader.Render("Recent") + "\n")
		for _, idx := range resolved {
			b.WriteString(v.renderListItem(idx, width))
		}
	}

	return b.String()
}

// renderListItem renders a single approval in the list.
func (v *ApprovalsView) renderListItem(idx, width int) string {
	a := v.approvals[idx]
	cursor := "  "
	nameStyle := lipgloss.NewStyle()
	if idx == v.cursor {
		cursor = "▸ "
		nameStyle = nameStyle.Bold(true)
	}

	label := a.Gate
	if label == "" {
		label = a.NodeID
	}
	if len(label) > width-4 {
		label = label[:width-7] + "..."
	}

	statusIcon := "○"
	switch a.Status {
	case "approved":
		statusIcon = "✓"
	case "denied":
		statusIcon = "✗"
	}

	return cursor + statusIcon + " " + nameStyle.Render(label) + "\n"
}

// renderListCompact renders the list with inline detail for narrow terminals.
func (v *ApprovalsView) renderListCompact() string {
	var b strings.Builder

	for i, a := range v.approvals {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			nameStyle = nameStyle.Bold(true)
		}

		label := a.Gate
		if label == "" {
			label = a.NodeID
		}

		statusIcon := "○"
		switch a.Status {
		case "approved":
			statusIcon = "✓"
		case "denied":
			statusIcon = "✗"
		}

		b.WriteString(cursor + statusIcon + " " + nameStyle.Render(label) + "\n")

		// Show inline context for selected item.
		if i == v.cursor {
			faint := lipgloss.NewStyle().Faint(true)
			b.WriteString(faint.Render("    Workflow: "+a.WorkflowPath) + "\n")
			b.WriteString(faint.Render("    Run: "+a.RunID) + "\n")
			if a.Payload != "" {
				b.WriteString(faint.Render("    "+truncate(a.Payload, 60)) + "\n")
			}
		}

		if i < len(v.approvals)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderDetail renders the context detail pane for the currently selected approval.
func (v *ApprovalsView) renderDetail(width int) string {
	if v.cursor < 0 || v.cursor >= len(v.approvals) {
		return ""
	}

	a := v.approvals[v.cursor]
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)

	// Gate / question
	gate := a.Gate
	if gate == "" {
		gate = a.NodeID
	}
	b.WriteString(titleStyle.Render(gate) + "\n\n")

	// Metadata
	b.WriteString(labelStyle.Render("Status:   ") + formatStatus(a.Status) + "\n")
	b.WriteString(labelStyle.Render("Workflow: ") + a.WorkflowPath + "\n")
	b.WriteString(labelStyle.Render("Run:      ") + a.RunID + "\n")
	b.WriteString(labelStyle.Render("Node:     ") + a.NodeID + "\n")

	// Payload (task inputs / context)
	if a.Payload != "" {
		b.WriteString("\n" + labelStyle.Render("Payload:") + "\n")
		b.WriteString(formatPayload(a.Payload, width) + "\n")
	}

	return b.String()
}

// Name returns the view name.
func (v *ApprovalsView) Name() string {
	return "approvals"
}

// ShortHelp returns keybinding hints for the help bar.
func (v *ApprovalsView) ShortHelp() []string {
	return []string{"[↑↓] Navigate", "[r] Refresh", "[Esc] Back"}
}

// --- Helpers ---

// padRight pads a string to the given width with spaces.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate shortens a string to maxLen, adding ellipsis if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatStatus returns a styled status string.
func formatStatus(status string) string {
	switch status {
	case "pending":
		return lipgloss.NewStyle().Bold(true).Render("● pending")
	case "approved":
		return lipgloss.NewStyle().Render("✓ approved")
	case "denied":
		return lipgloss.NewStyle().Render("✗ denied")
	default:
		return status
	}
}

// formatPayload attempts to pretty-print JSON payload, falling back to raw text.
func formatPayload(payload string, width int) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		// Not JSON; wrap text.
		return wrapText(payload, width)
	}

	pretty, err := json.MarshalIndent(parsed, "  ", "  ")
	if err != nil {
		return wrapText(payload, width)
	}
	return "  " + string(pretty)
}

// wrapText wraps text to fit within the given width.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			lines = append(lines, "  "+line[:width-2])
			line = line[width-2:]
		}
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}

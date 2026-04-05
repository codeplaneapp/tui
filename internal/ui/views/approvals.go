package views

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*ApprovalsView)(nil)

type approvalsLoadedMsg struct {
	approvals []smithers.Approval
}

type approvalsErrorMsg struct {
	err error
}

type decisionsLoadedMsg struct {
	decisions []smithers.ApprovalDecision
}

type decisionsErrorMsg struct {
	err error
}

type approveSuccessMsg struct{ approvalID string }
type approveErrorMsg struct {
	approvalID string
	err        error
}

type denySuccessMsg struct{ approvalID string }
type denyErrorMsg struct {
	approvalID string
	err        error
}

type runSummaryLoadedMsg struct {
	runID   string
	summary *smithers.RunSummary
}

type runSummaryErrorMsg struct {
	runID string
	err   error
}

// ApprovalsView displays a split-pane approvals queue with context details.
// Tab switches between the Pending Queue tab and the Recent Decisions tab.
// In the pending queue tab, the layout uses a SplitPane: list on the left,
// detail on the right.
type ApprovalsView struct {
	client    *smithers.Client
	approvals []smithers.Approval
	cursor    int
	width     int
	height    int
	loading   bool
	err       error

	// Recent decisions tab state
	showRecent       bool
	recentDecisions  []smithers.ApprovalDecision
	decisionsLoading bool
	decisionsErr     error
	decisionsCursor  int

	// Split pane for list+detail layout (pending queue tab)
	splitPane  *components.SplitPane
	listPane   *approvalListPane
	detailPane *approvalDetailPane

	// Inflight decision state
	inflightIdx int          // index of approval being acted on; -1 when idle
	actionErr   error        // last approve/deny error; nil when idle or cleared
	spinner     spinner.Model

	// Enriched run context for the selected approval (async-fetched on cursor change).
	selectedRun    *smithers.RunSummary // nil until fetched or if fetch failed
	contextLoading bool                // true while fetching RunContext
	contextErr     error               // non-nil if fetch failed
	lastFetchRun   string              // RunID of last triggered fetch (for dedup / stale-result guard)
}

// NewApprovalsView creates a new approvals view.
func NewApprovalsView(client *smithers.Client) *ApprovalsView {
	listPane := &approvalListPane{inflightIdx: -1}
	detailPane := &approvalDetailPane{}
	sp := components.NewSplitPane(listPane, detailPane, components.SplitPaneOpts{
		LeftWidth:         30,
		CompactBreakpoint: 80,
	})
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	return &ApprovalsView{
		client:      client,
		loading:     true,
		splitPane:   sp,
		listPane:    listPane,
		detailPane:  detailPane,
		inflightIdx: -1,
		spinner:     s,
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

// doApprove fires a background approve for the given approval and returns success/error messages.
func (v *ApprovalsView) doApprove(a smithers.Approval) tea.Cmd {
	return func() tea.Msg {
		err := v.client.Approve(context.Background(), a.RunID, a.NodeID, 0, "")
		if err != nil {
			return approveErrorMsg{approvalID: a.ID, err: err}
		}
		return approveSuccessMsg{approvalID: a.ID}
	}
}

// doDeny fires a background deny for the given approval and returns success/error messages.
func (v *ApprovalsView) doDeny(a smithers.Approval) tea.Cmd {
	return func() tea.Msg {
		err := v.client.Deny(context.Background(), a.RunID, a.NodeID, 0, "")
		if err != nil {
			return denyErrorMsg{approvalID: a.ID, err: err}
		}
		return denySuccessMsg{approvalID: a.ID}
	}
}

// loadDecisions returns a command that fetches recent decisions.
func (v *ApprovalsView) loadDecisions() tea.Cmd {
	return func() tea.Msg {
		decisions, err := v.client.ListRecentDecisions(context.Background(), 50)
		if err != nil {
			return decisionsErrorMsg{err: err}
		}
		return decisionsLoadedMsg{decisions: decisions}
	}
}

// fetchRunContext returns a Cmd that fetches RunContext for the currently selected approval.
// Returns nil if the list is empty, if no approval is at the cursor, or if the
// RunID matches the last triggered fetch (dedup guard).
func (v *ApprovalsView) fetchRunContext() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.approvals) {
		return nil
	}
	a := v.approvals[v.cursor]
	if a.RunID == "" {
		return nil
	}
	if a.RunID == v.lastFetchRun && v.selectedRun != nil {
		// Already fetched for this run; skip unless we have no result.
		return nil
	}
	v.contextLoading = true
	v.contextErr = nil
	v.lastFetchRun = a.RunID
	v.detailPane.contextLoading = true
	v.detailPane.contextErr = nil
	v.detailPane.selectedRun = nil
	runID := a.RunID
	return func() tea.Msg {
		summary, err := v.client.GetRunSummary(context.Background(), runID)
		if err != nil {
			return runSummaryErrorMsg{runID: runID, err: err}
		}
		return runSummaryLoadedMsg{runID: runID, summary: summary}
	}
}

// Update handles messages for the approvals view.
func (v *ApprovalsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case approvalsLoadedMsg:
		v.approvals = msg.approvals
		v.loading = false
		v.listPane.approvals = msg.approvals
		v.detailPane.approvals = msg.approvals
		v.splitPane.SetSize(v.width, max(0, v.height-2))
		// Kick off context fetch for the first item if any approvals loaded.
		if len(msg.approvals) > 0 {
			return v, v.fetchRunContext()
		}
		return v, nil

	case approvalsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case decisionsLoadedMsg:
		v.recentDecisions = msg.decisions
		v.decisionsLoading = false
		v.decisionsErr = nil
		return v, nil

	case decisionsErrorMsg:
		v.decisionsErr = msg.err
		v.decisionsLoading = false
		return v, nil

	case runSummaryLoadedMsg:
		if msg.runID == v.lastFetchRun {
			v.selectedRun = msg.summary
			v.contextLoading = false
			v.contextErr = nil
			v.detailPane.selectedRun = msg.summary
			v.detailPane.contextLoading = false
			v.detailPane.contextErr = nil
		}
		return v, nil

	case runSummaryErrorMsg:
		if msg.runID == v.lastFetchRun {
			v.contextErr = msg.err
			v.contextLoading = false
			v.detailPane.contextErr = msg.err
			v.detailPane.contextLoading = false
		}
		return v, nil

	case spinner.TickMsg:
		if v.inflightIdx != -1 {
			var cmd tea.Cmd
			v.spinner, cmd = v.spinner.Update(msg)
			v.listPane.spinnerView = v.spinner.View()
			return v, cmd
		}
		return v, nil

	case approveSuccessMsg:
		for i, a := range v.approvals {
			if a.ID == msg.approvalID {
				v.approvals = append(v.approvals[:i], v.approvals[i+1:]...)
				if v.cursor >= len(v.approvals) && v.cursor > 0 {
					v.cursor = len(v.approvals) - 1
				}
				v.listPane.approvals = v.approvals
				v.listPane.cursor = v.cursor
				v.listPane.inflightIdx = -1
				v.detailPane.approvals = v.approvals
				v.detailPane.cursor = v.cursor
				v.detailPane.actionErr = nil
				break
			}
		}
		v.inflightIdx = -1
		v.actionErr = nil
		return v, nil

	case approveErrorMsg:
		v.inflightIdx = -1
		v.listPane.inflightIdx = -1
		v.actionErr = msg.err
		v.detailPane.actionErr = msg.err
		return v, nil

	case denySuccessMsg:
		for i, a := range v.approvals {
			if a.ID == msg.approvalID {
				v.approvals = append(v.approvals[:i], v.approvals[i+1:]...)
				if v.cursor >= len(v.approvals) && v.cursor > 0 {
					v.cursor = len(v.approvals) - 1
				}
				v.listPane.approvals = v.approvals
				v.listPane.cursor = v.cursor
				v.listPane.inflightIdx = -1
				v.detailPane.approvals = v.approvals
				v.detailPane.cursor = v.cursor
				v.detailPane.actionErr = nil
				break
			}
		}
		v.inflightIdx = -1
		v.actionErr = nil
		return v, nil

	case denyErrorMsg:
		v.inflightIdx = -1
		v.listPane.inflightIdx = -1
		v.actionErr = msg.err
		v.detailPane.actionErr = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.splitPane.SetSize(msg.Width, max(0, msg.Height-2))
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			v.showRecent = !v.showRecent
			if v.showRecent && !v.decisionsLoading && v.recentDecisions == nil {
				v.decisionsLoading = true
				return v, v.loadDecisions()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if v.showRecent {
				v.decisionsLoading = true
				return v, v.loadDecisions()
			}
			v.loading = true
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			if !v.showRecent && v.inflightIdx == -1 && v.cursor < len(v.approvals) {
				if v.approvals[v.cursor].Status == "pending" {
					v.inflightIdx = v.cursor
					v.actionErr = nil
					v.detailPane.actionErr = nil
					v.listPane.inflightIdx = v.cursor
					v.listPane.spinnerView = v.spinner.View()
					return v, tea.Batch(v.spinner.Tick, v.doApprove(v.approvals[v.cursor]))
				}
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if !v.showRecent && v.inflightIdx == -1 && v.cursor < len(v.approvals) {
				if v.approvals[v.cursor].Status == "pending" {
					v.inflightIdx = v.cursor
					v.actionErr = nil
					v.detailPane.actionErr = nil
					v.listPane.inflightIdx = v.cursor
					v.listPane.spinnerView = v.spinner.View()
					return v, tea.Batch(v.spinner.Tick, v.doDeny(v.approvals[v.cursor]))
				}
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.showRecent {
				if v.decisionsCursor > 0 {
					v.decisionsCursor--
				}
			} else {
				if v.cursor > 0 {
					v.cursor--
				}
				v.listPane.cursor = v.cursor
				v.detailPane.cursor = v.cursor
				return v, v.fetchRunContext()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.showRecent {
				if v.decisionsCursor < len(v.recentDecisions)-1 {
					v.decisionsCursor++
				}
			} else {
				if v.cursor < len(v.approvals)-1 {
					v.cursor++
				}
				v.listPane.cursor = v.cursor
				v.detailPane.cursor = v.cursor
				return v, v.fetchRunContext()
			}
			return v, nil
		}
	}
	return v, nil
}

// View renders the approvals view.
func (v *ApprovalsView) View() string {
	var b strings.Builder

	if v.showRecent {
		// --- Recent Decisions tab ---
		header := lipgloss.NewStyle().Bold(true).Render("SMITHERS \u203a RECENT DECISIONS")
		helpHint := lipgloss.NewStyle().Faint(true).Render("[tab] Pending  [Esc] Back")
		headerLine := header
		if v.width > 0 {
			gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
			if gap > 0 {
				headerLine = header + strings.Repeat(" ", gap) + helpHint
			}
		}
		b.WriteString(headerLine)
		b.WriteString("\n\n")

		if v.decisionsLoading {
			b.WriteString("  Loading recent decisions...\n")
			return b.String()
		}
		if v.decisionsErr != nil {
			b.WriteString(fmt.Sprintf("  Error: %v\n", v.decisionsErr))
			return b.String()
		}
		if len(v.recentDecisions) == 0 {
			b.WriteString("  No recent decisions.\n")
			return b.String()
		}
		b.WriteString(v.renderRecentDecisions())
		return b.String()
	}

	// --- Pending Queue tab ---
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS \u203a Approvals")
	var hintParts []string
	if v.cursor < len(v.approvals) && v.approvals[v.cursor].Status == "pending" {
		if v.inflightIdx != -1 {
			hintParts = append(hintParts, "Acting...")
		} else {
			hintParts = append(hintParts, "[a] Approve  [d] Deny")
		}
	}
	hintParts = append(hintParts, "[tab] History  [Esc] Back")
	helpHint := lipgloss.NewStyle().Faint(true).Render(strings.Join(hintParts, "  "))
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

	// SplitPane handles wide vs. compact layout automatically.
	b.WriteString(v.splitPane.View())

	return b.String()
}

// renderRecentDecisions renders the recent decisions list.
func (v *ApprovalsView) renderRecentDecisions() string {
	var b strings.Builder

	for i, d := range v.recentDecisions {
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if i == v.decisionsCursor {
			cursor = "\u25b8 "
			nameStyle = nameStyle.Bold(true)
		}

		icon := "\u2713"
		if d.Decision == "denied" {
			icon = "\u2717"
		}

		label := d.Gate
		if label == "" {
			label = d.NodeID
		}
		label = truncate(label, 40)

		ts := relativeTime(d.DecidedAt)
		byStr := ""
		if d.DecidedBy != nil && *d.DecidedBy != "" {
			byStr = " by " + *d.DecidedBy
		}

		line := cursor + icon + " " + nameStyle.Render(label) +
			lipgloss.NewStyle().Faint(true).Render("  "+ts+byStr)

		b.WriteString(line + "\n")
	}

	return b.String()
}

// renderDetail renders the context detail pane for the currently selected approval.
// This is used by tests and by the approvalDetailPane.View() method.
func (v *ApprovalsView) renderDetail(width int) string {
	if v.cursor < 0 || v.cursor >= len(v.approvals) {
		return ""
	}
	return renderApprovalDetail(v.approvals[v.cursor], v.selectedRun, v.contextLoading, v.contextErr, v.actionErr, width, v.height)
}

// renderApprovalDetail produces the full detail pane text for a single approval.
// It is the canonical implementation used by both renderDetail (test/view) and approvalDetailPane.View().
func renderApprovalDetail(a smithers.Approval, run *smithers.RunSummary, contextLoading bool, contextErr error, actionErr error, width, height int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	// 1. Gate header (prominent, at the top).
	gate := a.Gate
	if gate == "" {
		gate = a.NodeID
	}
	b.WriteString(titleStyle.Render(gate) + "\n")

	// Wait time + status on the same line as the gate, or just below it.
	wait := time.Since(time.UnixMilli(a.RequestedAt))
	waitStr := formatWait(wait)
	statusStr := formatStatus(a.Status)
	if a.Status == "pending" {
		waitColored := slaStyle(wait).Render("⏱ " + waitStr)
		b.WriteString(statusStr + "  " + waitColored + "\n")
	} else {
		b.WriteString(statusStr + "\n")
	}
	b.WriteString("\n")

	// 2. Workflow name (extracted from WorkflowPath).
	workflowName := ""
	if run != nil && run.WorkflowName != "" {
		workflowName = run.WorkflowName
	} else if a.WorkflowPath != "" {
		workflowName = workflowNameDisplay(a.WorkflowPath)
	}
	if workflowName != "" {
		b.WriteString(labelStyle.Render("Workflow: ") + workflowName + "\n")
	}

	// 3. Run ID + node context.
	b.WriteString(labelStyle.Render("Run:      ") + a.RunID + "\n")
	b.WriteString(labelStyle.Render("Node:     ") + a.NodeID + "\n")

	// 4. Enriched run context (async-fetched).
	if contextLoading {
		b.WriteString("\n" + faintStyle.Render("Loading run details...") + "\n")
	} else if contextErr != nil {
		errStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("1"))
		b.WriteString("\n" + errStyle.Render("Could not load run details: "+contextErr.Error()) + "\n")
	} else if run != nil {
		// Step progress derived from Summary map (node-state → count).
		nodeTotal, nodesDone := runNodeProgress(run)
		if nodeTotal > 0 {
			b.WriteString(labelStyle.Render("Progress: ") +
				fmt.Sprintf("Step %d of %d · %s", nodesDone, nodeTotal, string(run.Status)) + "\n")
		} else if run.Status != "" {
			b.WriteString(labelStyle.Render("Status:   ") + string(run.Status) + "\n")
		}
		if run.StartedAtMs != nil && *run.StartedAtMs > 0 {
			b.WriteString(labelStyle.Render("Started:  ") + relativeTime(*run.StartedAtMs) + "\n")
		}
	}

	// 5. RequestedAt timestamp with relative time.
	if a.RequestedAt > 0 {
		ts := time.UnixMilli(a.RequestedAt).Format("2006-01-02 15:04:05")
		b.WriteString(labelStyle.Render("Requested:") + " " + ts + " (" + relativeTime(a.RequestedAt) + ")" + "\n")
	}

	// 6. Resolution info (non-pending approvals only).
	if a.ResolvedAt != nil {
		resolvedBy := "(unknown)"
		if a.ResolvedBy != nil && *a.ResolvedBy != "" {
			resolvedBy = *a.ResolvedBy
		}
		b.WriteString(labelStyle.Render("Resolved by: ") + resolvedBy + "\n")
		b.WriteString(labelStyle.Render("Resolved at: ") + relativeTime(*a.ResolvedAt) + "\n")
	}

	// 7. Full JSON payload with height-capped display.
	if a.Payload != "" {
		b.WriteString("\n" + labelStyle.Render("Payload:") + "\n")
		payloadText := formatPayload(a.Payload, width)
		payloadText = capPayloadLines(payloadText, height, &b)
		b.WriteString(payloadText + "\n")
	}

	// 8. Action error banner.
	if actionErr != nil && a.Status == "pending" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString("\n" + errStyle.Render("Action failed: "+actionErr.Error()) + "\n")
		b.WriteString(faintStyle.Render("  Press [a] to approve or [d] to deny") + "\n")
	}

	return b.String()
}

// workflowNameDisplay extracts a short display name from a workflow path.
// E.g., ".smithers/workflows/deploy.ts" → "deploy".
// Unlike the client-side helper, this operates on the raw path string without
// importing path.Base from the smithers package.
func workflowNameDisplay(p string) string {
	base := path.Base(p)
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".yaml", ".yml"} {
		if strings.HasSuffix(base, ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

// capPayloadLines limits the payload text to a reasonable number of lines
// based on available terminal height. Returns the (possibly truncated) text.
// The builder b is passed to count lines already written above the payload.
func capPayloadLines(payloadText string, height int, b *strings.Builder) string {
	if height <= 0 {
		return payloadText
	}
	// Estimate lines already used by the header/metadata above the payload.
	linesUsed := strings.Count(b.String(), "\n") + 3 // +3 for payload header + padding
	maxPayloadLines := height - linesUsed
	if maxPayloadLines < 4 {
		maxPayloadLines = 4
	}
	lines := strings.Split(payloadText, "\n")
	if len(lines) <= maxPayloadLines {
		return payloadText
	}
	truncated := strings.Join(lines[:maxPayloadLines], "\n")
	remaining := len(lines) - maxPayloadLines
	return truncated + fmt.Sprintf("\n  ... (%d more lines)", remaining)
}

// formatWait formats a duration as a short human-readable wait time string.
// E.g., "<1m", "8m", "1h 23m".
func formatWait(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// slaStyle returns a lipgloss.Style with SLA-appropriate foreground color.
// Green  < 5 minutes (was recently requested — low urgency)
// Yellow < 15 minutes (needs attention)
// Red    ≥ 15 minutes (overdue / blocking)
// Note: the ticket brief says <5m green, <15m yellow, >15m red.
func slaStyle(d time.Duration) lipgloss.Style {
	switch {
	case d < 5*time.Minute:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case d < 15*time.Minute:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	}
}

// runNodeProgress derives total and done node counts from a RunSummary's Summary map.
// The Summary map contains node-state → count pairs (e.g., {"running": 1, "finished": 2}).
// "Done" counts finished + failed + cancelled + skipped states.
// Returns (0, 0) when the Summary map is nil or empty.
func runNodeProgress(run *smithers.RunSummary) (nodeTotal, nodesDone int) {
	if run == nil || len(run.Summary) == 0 {
		return 0, 0
	}
	for _, count := range run.Summary {
		nodeTotal += count
	}
	doneStates := []string{"finished", "failed", "cancelled", "skipped"}
	for _, s := range doneStates {
		nodesDone += run.Summary[s]
	}
	return nodeTotal, nodesDone
}

// Name returns the view name.
func (v *ApprovalsView) Name() string {
	return "approvals"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *ApprovalsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.splitPane.SetSize(width, max(0, height-2))
}

// ShortHelp returns keybinding hints for the help bar.
func (v *ApprovalsView) ShortHelp() []key.Binding {
	if v.showRecent {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191\u2193", "navigate")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pending queue")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191\u2193", "navigate")),
	}
	if v.cursor < len(v.approvals) && v.approvals[v.cursor].Status == "pending" {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "deny")),
		)
	}
	bindings = append(bindings,
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "history")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	)
	return bindings
}

// relativeTime returns a human-readable relative time string for a Unix-ms timestamp.
func relativeTime(ms int64) string {
	d := time.Since(time.UnixMilli(ms))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// --- Private pane types for the split layout ---

// approvalListPane is the left pane: navigable list of approvals.
type approvalListPane struct {
	approvals   []smithers.Approval
	cursor      int
	width       int
	height      int
	inflightIdx int    // index of approval with active inflight action; -1 when idle
	spinnerView string // current spinner frame rendered string
}

func (p *approvalListPane) Init() tea.Cmd { return nil }

func (p *approvalListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil
}

func (p *approvalListPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *approvalListPane) View() string {
	if len(p.approvals) == 0 {
		return ""
	}
	var b strings.Builder
	sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true)

	var pending, resolved []int
	for i, a := range p.approvals {
		if a.Status == "pending" {
			pending = append(pending, i)
		} else {
			resolved = append(resolved, i)
		}
	}

	if len(pending) > 0 {
		b.WriteString(sectionHeader.Render("Pending") + "\n")
		for _, idx := range pending {
			b.WriteString(p.renderItem(idx))
		}
	}

	if len(resolved) > 0 {
		if len(pending) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(sectionHeader.Render("Recent") + "\n")
		for _, idx := range resolved {
			b.WriteString(p.renderItem(idx))
		}
	}

	return b.String()
}

func (p *approvalListPane) renderItem(idx int) string {
	a := p.approvals[idx]
	cursor := "  "
	nameStyle := lipgloss.NewStyle()
	if idx == p.cursor {
		cursor = "\u25b8 "
		nameStyle = nameStyle.Bold(true)
	}

	label := a.Gate
	if label == "" {
		label = a.NodeID
	}

	statusIcon := "\u25cb" // ○
	switch {
	case idx == p.inflightIdx && p.spinnerView != "":
		statusIcon = p.spinnerView
	case a.Status == "approved":
		statusIcon = "\u2713" // ✓
	case a.Status == "denied":
		statusIcon = "\u2717" // ✗
	}

	// Build wait-time badge for pending items.
	waitBadge := ""
	if a.Status == "pending" && a.RequestedAt > 0 {
		wait := time.Since(time.UnixMilli(a.RequestedAt))
		waitBadge = slaStyle(wait).Render(formatWait(wait))
	}

	// Compute label width accounting for cursor (2), icon (1), space (1), and wait badge.
	badgeWidth := lipgloss.Width(waitBadge)
	reserved := 4 // cursor(2) + icon(1) + space(1)
	if badgeWidth > 0 {
		reserved += badgeWidth + 1 // +1 for the space before badge
	}
	maxLabelLen := p.width - reserved
	if maxLabelLen < 1 {
		maxLabelLen = 1
	}
	label = truncate(label, maxLabelLen)

	line := cursor + statusIcon + " " + nameStyle.Render(label)
	if waitBadge != "" {
		// Right-align the badge within the pane width.
		currentWidth := lipgloss.Width(line)
		gap := p.width - currentWidth - badgeWidth
		if gap > 0 {
			line += strings.Repeat(" ", gap)
		} else {
			line += " "
		}
		line += waitBadge
	}
	return line + "\n"
}

// approvalDetailPane is the right pane: detail display for the selected approval.
type approvalDetailPane struct {
	approvals []smithers.Approval
	cursor    int
	width     int
	height    int
	actionErr error // last approve/deny error to display inline

	// Enriched run context (populated by ApprovalsView on cursor change).
	selectedRun    *smithers.RunSummary
	contextLoading bool
	contextErr     error
}

func (p *approvalDetailPane) Init() tea.Cmd { return nil }

func (p *approvalDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil // read-only in v1
}

func (p *approvalDetailPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *approvalDetailPane) View() string {
	if p.cursor < 0 || p.cursor >= len(p.approvals) {
		return ""
	}
	return renderApprovalDetail(p.approvals[p.cursor], p.selectedRun, p.contextLoading, p.contextErr, p.actionErr, p.width, p.height)
}

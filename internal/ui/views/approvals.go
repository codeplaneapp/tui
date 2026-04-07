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
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*ApprovalsView)(nil)

type approvalsLoadedMsg struct {
	approvals []smithers.Approval
}

type decisionsLoadedMsg struct {
	decisions []smithers.ApprovalDecision
}

type approvalsErrorMsg struct {
	err error
}

type approvalActionDoneMsg struct {
	idx int
}

type approvalActionErrorMsg struct {
	idx int
	err error
}

type approvalRunContextMsg struct {
	runID string
	run   *smithers.RunSummary
	err   error
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

// ApprovalsView displays a navigable list of human-in-the-loop approval gates
// with a detail pane showing run context and payload.
type ApprovalsView struct {
	com            *common.Common
	client         *smithers.Client
	approvals      []smithers.Approval
	cursor         int
	width, height  int
	loading        bool
	err            error
	showRecent     bool // toggle between pending queue and resolution history
	inflightIdx    int  // index of approval with active action; -1 when idle
	actionErr      error
	contextLoading bool
	contextErr     error
	selectedRun    *smithers.RunSummary
	lastFetchRun   string

	splitPane  *components.SplitPane
	listPane   *approvalListPane
	detailPane *approvalDetailPane
	spinner    spinner.Model
}

// NewApprovalsView creates a new approvals view.
func NewApprovalsView(args ...any) *ApprovalsView {
	com, client, _ := parseCommonAndClient(args)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(com.Styles.Primary)

	lp := &approvalListPane{com: com}
	dp := &approvalDetailPane{com: com}

	sp := components.NewSplitPane(lp, dp, components.SplitPaneOpts{
		LeftWidth:          32,
		FocusedBorderColor: "63",
	})

	return &ApprovalsView{
		com:         com,
		client:      client,
		loading:     true,
		inflightIdx: -1,
		splitPane:   sp,
		listPane:    lp,
		detailPane:  dp,
		spinner:     s,
	}
}

// Init loads pending approvals and starts the spinner.
func (v *ApprovalsView) Init() tea.Cmd {
	return tea.Batch(
		v.loadApprovalsCmd(),
		v.spinner.Tick,
	)
}

func (v *ApprovalsView) loadApprovalsCmd() tea.Cmd {
	client := v.client
	history := v.showRecent
	return func() tea.Msg {
		var approvals []smithers.Approval
		var err error
		if history {
			decisions, dErr := client.ListRecentDecisions(context.Background(), 50)
			if dErr != nil {
				return approvalsErrorMsg{err: dErr}
			}
			// Map decisions back to approvals for consistent UI
			for _, d := range decisions {
				approvals = append(approvals, smithers.Approval{
					ID:           d.ID,
					RunID:        d.RunID,
					NodeID:       d.NodeID,
					WorkflowPath: d.WorkflowPath,
					Gate:         d.Gate,
					Status:       d.Decision,
					ResolvedAt:   &d.DecidedAt,
					ResolvedBy:   d.DecidedBy,
					RequestedAt:  d.RequestedAt,
				})
			}
		} else {
			approvals, err = client.ListPendingApprovals(context.Background())
		}

		if err != nil {
			return approvalsErrorMsg{err: err}
		}
		return approvalsLoadedMsg{approvals: approvals}
	}
}

func (v *ApprovalsView) loadRunContextCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		run, err := client.GetRun(context.Background(), runID)
		return approvalRunContextMsg{runID: runID, run: run, err: err}
	}
}

func decisionsAsApprovals(decisions []smithers.ApprovalDecision) []smithers.Approval {
	approvals := make([]smithers.Approval, 0, len(decisions))
	for _, d := range decisions {
		approvals = append(approvals, smithers.Approval{
			ID:           d.ID,
			RunID:        d.RunID,
			NodeID:       d.NodeID,
			WorkflowPath: d.WorkflowPath,
			Gate:         d.Gate,
			Status:       d.Decision,
			ResolvedAt:   &d.DecidedAt,
			ResolvedBy:   d.DecidedBy,
			RequestedAt:  d.RequestedAt,
		})
	}
	return approvals
}

func (v *ApprovalsView) shouldFetchRunContext(runID string) bool {
	if strings.TrimSpace(runID) == "" {
		return false
	}
	return v.lastFetchRun != runID || v.selectedRun == nil || v.selectedRun.RunID != runID || v.contextErr != nil
}

func (v *ApprovalsView) beginRunContextLoad(runID string) tea.Cmd {
	if !v.shouldFetchRunContext(runID) {
		v.contextLoading = false
		v.contextErr = nil
		v.syncPanes()
		return nil
	}
	v.lastFetchRun = runID
	v.contextLoading = true
	v.contextErr = nil
	v.syncPanes()
	return v.loadRunContextCmd(runID)
}

func (v *ApprovalsView) removeApprovalByID(approvalID string) {
	for i, approval := range v.approvals {
		if approval.ID != approvalID {
			continue
		}
		v.approvals = append(v.approvals[:i], v.approvals[i+1:]...)
		v.clampCursor()
		v.syncPanes()
		return
	}
}

func (v *ApprovalsView) approveCmd(idx int) tea.Cmd {
	if idx < 0 || idx >= len(v.approvals) {
		return nil
	}
	a := v.approvals[idx]
	client := v.client
	return func() tea.Msg {
		err := client.Approve(context.Background(), a.RunID, a.NodeID, 0, "")
		if err != nil {
			return approvalActionErrorMsg{idx: idx, err: err}
		}
		return approvalActionDoneMsg{idx: idx}
	}
}

func (v *ApprovalsView) denyCmd(idx int) tea.Cmd {
	if idx < 0 || idx >= len(v.approvals) {
		return nil
	}
	a := v.approvals[idx]
	client := v.client
	return func() tea.Msg {
		err := client.Deny(context.Background(), a.RunID, a.NodeID, 0, "")
		if err != nil {
			return approvalActionErrorMsg{idx: idx, err: err}
		}
		return approvalActionDoneMsg{idx: idx}
	}
}

// Update handles messages for the approvals view.
func (v *ApprovalsView) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case approvalsLoadedMsg:
		v.approvals = msg.approvals
		v.loading = false
		v.err = nil
		v.clampCursor()
		v.syncPanes()
		if len(v.approvals) > 0 {
			cmds = append(cmds, v.beginRunContextLoad(v.approvals[v.cursor].RunID))
		}
		return v, tea.Batch(cmds...)

	case decisionsLoadedMsg:
		v.approvals = decisionsAsApprovals(msg.decisions)
		v.loading = false
		v.err = nil
		v.clampCursor()
		v.syncPanes()
		if len(v.approvals) > 0 {
			cmds = append(cmds, v.beginRunContextLoad(v.approvals[v.cursor].RunID))
		}
		return v, tea.Batch(cmds...)

	case approvalsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case approvalRunContextMsg:
		if msg.runID != "" && v.lastFetchRun != "" && msg.runID != v.lastFetchRun {
			return v, nil
		}
		v.contextLoading = false
		v.selectedRun = msg.run
		v.contextErr = msg.err
		v.syncPanes()
		return v, nil

	case runSummaryLoadedMsg:
		if msg.runID != "" && v.lastFetchRun != "" && msg.runID != v.lastFetchRun {
			return v, nil
		}
		v.contextLoading = false
		v.selectedRun = msg.summary
		v.contextErr = nil
		if msg.runID != "" {
			v.lastFetchRun = msg.runID
		}
		v.syncPanes()
		return v, nil

	case runSummaryErrorMsg:
		if msg.runID != "" && v.lastFetchRun != "" && msg.runID != v.lastFetchRun {
			return v, nil
		}
		v.contextLoading = false
		v.selectedRun = nil
		v.contextErr = msg.err
		v.syncPanes()
		return v, nil

	case approvalActionDoneMsg:
		v.inflightIdx = -1
		v.actionErr = nil
		// Refresh list after successful action.
		v.loading = true
		cmds = append(cmds, v.loadApprovalsCmd())

	case approvalActionErrorMsg:
		v.inflightIdx = -1
		v.actionErr = msg.err
		v.syncPanes()
		return v, nil

	case approveSuccessMsg:
		v.inflightIdx = -1
		v.actionErr = nil
		v.removeApprovalByID(msg.approvalID)
		return v, nil

	case denySuccessMsg:
		v.inflightIdx = -1
		v.actionErr = nil
		v.removeApprovalByID(msg.approvalID)
		return v, nil

	case approveErrorMsg:
		v.inflightIdx = -1
		v.actionErr = msg.err
		v.syncPanes()
		return v, nil

	case denyErrorMsg:
		v.inflightIdx = -1
		v.actionErr = msg.err
		v.syncPanes()
		return v, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		v.listPane.spinnerView = v.spinner.View()
		return v, cmd

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.syncPanes()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
				v.actionErr = nil
				cmds = append(cmds, v.beginRunContextLoad(v.approvals[v.cursor].RunID))
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.approvals)-1 {
				v.cursor++
				v.actionErr = nil
				cmds = append(cmds, v.beginRunContextLoad(v.approvals[v.cursor].RunID))
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			v.showRecent = !v.showRecent
			v.loading = true
			v.cursor = 0
			v.actionErr = nil
			return v, v.loadApprovalsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.actionErr = nil
			return v, v.loadApprovalsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			if v.canAct() {
				v.inflightIdx = v.cursor
				v.syncPanes()
				return v, v.approveCmd(v.cursor)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if v.canAct() {
				v.inflightIdx = v.cursor
				v.syncPanes()
				return v, v.denyCmd(v.cursor)
			}
		}
	}

	// Forward other messages to SplitPane (handles focus toggle via Tab).
	sp, cmd := v.splitPane.Update(msg)
	v.splitPane = sp
	cmds = append(cmds, cmd)

	return v, tea.Batch(cmds...)
}

func (v *ApprovalsView) clampCursor() {
	if len(v.approvals) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor >= len(v.approvals) {
		v.cursor = len(v.approvals) - 1
	}
}

func (v *ApprovalsView) canAct() bool {
	return v.inflightIdx == -1 &&
		v.cursor < len(v.approvals) &&
		v.approvals[v.cursor].Status == "pending"
}

func (v *ApprovalsView) renderDetail(width int) string {
	if v.cursor < 0 || v.cursor >= len(v.approvals) {
		return ""
	}
	return renderApprovalDetail(v.com, v.approvals[v.cursor], v.selectedRun, v.contextLoading, v.contextErr, v.actionErr, width, v.height)
}

// syncPanes updates the sub-pane state from the main view.
func (v *ApprovalsView) syncPanes() {
	v.listPane.approvals = v.approvals
	v.listPane.cursor = v.cursor
	v.listPane.inflightIdx = v.inflightIdx

	v.detailPane.approvals = v.approvals
	v.detailPane.cursor = v.cursor
	v.detailPane.actionErr = v.actionErr
	v.detailPane.selectedRun = v.selectedRun
	v.detailPane.contextLoading = v.contextLoading
	v.detailPane.contextErr = v.contextErr

	v.splitPane.SetSize(v.width, max(0, v.height-2))
}

// View renders the approvals layout.
func (v *ApprovalsView) View() string {
	var b strings.Builder
	t := v.com.Styles

	// Header
	viewName := "Pending Approvals"
	if v.showRecent {
		viewName = "Resolution History"
	}
	b.WriteString(ViewHeader(v.com.Styles, "CODEPLANE", viewName, v.width, "[Tab] switch mode  [Esc] Back"))
	b.WriteString("\n\n")

	if v.loading && len(v.approvals) == 0 {
		b.WriteString("  Loading approvals...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.approvals) == 0 {
		msg := "No pending approvals."
		if v.showRecent {
			msg = "No resolution history found."
		}
		b.WriteString("  " + t.Subtle.Render(msg) + "\n")
		return b.String()
	}

	b.WriteString(v.splitPane.View())

	return b.String()
}

// renderApprovalDetail renders the metadata and payload for a single approval.
func renderApprovalDetail(com *common.Common, a smithers.Approval, run *smithers.RunSummary, contextLoading bool, contextErr error, actionErr error, width, height int) string {
	var b strings.Builder
	t := com.Styles
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary)
	faintStyle := t.Subtle

	// 1. Title/Gate
	title := a.Gate
	if title == "" {
		title = a.NodeID
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(title) + "\n\n")

	// 2. Workflow Name (enriched from Run or parsed from path)
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
		errStyle := lipgloss.NewStyle().Faint(true).Foreground(t.Error)
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
		errStyle := lipgloss.NewStyle().Foreground(t.Error)
		b.WriteString("\n" + errStyle.Render("Action failed: "+actionErr.Error()) + "\n")
		b.WriteString(faintStyle.Render("  Press [a] to approve or [d] to deny") + "\n")
	}

	return b.String()
}

// workflowNameDisplay extracts a short display name from a workflow path.
func workflowNameDisplay(p string) string {
	base := path.Base(p)
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".yaml", ".yml"} {
		if strings.HasSuffix(base, ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

// capPayloadLines limits the payload text to a reasonable number of lines.
func capPayloadLines(payloadText string, height int, b *strings.Builder) string {
	if height <= 0 {
		return payloadText
	}
	linesUsed := strings.Count(b.String(), "\n") + 3
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
func slaStyle(args ...any) lipgloss.Style {
	com := packageCom
	var d time.Duration
	switch len(args) {
	case 1:
		d, _ = args[0].(time.Duration)
	case 2:
		if provided, ok := args[0].(*common.Common); ok && provided != nil {
			com = provided
		}
		d, _ = args[1].(time.Duration)
	default:
		return lipgloss.NewStyle()
	}
	t := com.Styles
	switch {
	case d < 5*time.Minute:
		return lipgloss.NewStyle().Foreground(t.Green) // green
	case d < 15*time.Minute:
		return lipgloss.NewStyle().Foreground(t.Yellow) // yellow
	default:
		return lipgloss.NewStyle().Foreground(t.Red) // red
	}
}

// runNodeProgress derives total and done node counts from a RunSummary's Summary map.
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
	com         *common.Common
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
		waitBadge = slaStyle(p.com, wait).Render(formatWait(wait))
	}

	badgeWidth := lipgloss.Width(waitBadge)
	reserved := 4
	if badgeWidth > 0 {
		reserved += badgeWidth + 1
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
	com       *common.Common
	approvals []smithers.Approval
	cursor    int
	width     int
	height    int
	actionErr error

	selectedRun    *smithers.RunSummary
	contextLoading bool
	contextErr     error
}

func (p *approvalDetailPane) Init() tea.Cmd { return nil }

func (p *approvalDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil
}

func (p *approvalDetailPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *approvalDetailPane) View() string {
	if p.cursor < 0 || p.cursor >= len(p.approvals) {
		return ""
	}
	return renderApprovalDetail(p.com, p.approvals[p.cursor], p.selectedRun, p.contextLoading, p.contextErr, p.actionErr, p.width, p.height)
}

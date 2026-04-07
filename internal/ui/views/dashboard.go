package views

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/anim"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

// DashboardTab identifies a tab on the Smithers homepage.
type DashboardTab int

const (
	DashTabOverview DashboardTab = iota
	DashTabRuns
	DashTabWorkflows
	DashTabLandings
	DashTabIssues
	DashTabWorkspaces
	DashTabSessions
)

func (t DashboardTab) String() string {
	switch t {
	case DashTabOverview:
		return "Overview"
	case DashTabRuns:
		return "Runs"
	case DashTabWorkflows:
		return "Workflows"
	case DashTabLandings:
		return "Landings"
	case DashTabIssues:
		return "Issues"
	case DashTabWorkspaces:
		return "Workspaces"
	case DashTabSessions:
		return "Sessions"
	default:
		return "?"
	}
}

// DashboardView is the Smithers homepage — a tabbed overview shown on startup.
type DashboardView struct {
	com          *common.Common
	client       *smithers.Client
	jjhubClient  *jjhub.Client
	jjhubEnabled bool // true when jjhub CLI is available
	width        int
	height       int
	activeTab    int
	tabs         []DashboardTab // instance-level tab list (not the global)

	brandingAnim *anim.Anim

	// Smithers data
	runs             []smithers.RunSummary
	workflows        []smithers.Workflow
	runsLoading      bool
	wfLoading        bool
	approvalsLoading bool
	runsErr          error
	wfErr            error
	approvalsErr     error
	approvals        []smithers.Approval

	// JJHub data
	landings          []jjhub.Landing
	issues            []jjhub.Issue
	workspaces        []jjhub.Workspace
	landingsLoading   bool
	issuesLoading     bool
	workspacesLoading bool
	landingsErr       error
	issuesErr         error
	workspacesErr     error

	// repo name shown in header when jjhub is available
	repoName string

	// Menu items for overview
	menuCursor int
	menuItems  []menuItem
}

type menuItem struct {
	icon  string
	label string
	desc  string
	// action returns a tea.Msg to emit when selected
	action func() tea.Msg
}

func (d *DashboardView) isLoading() bool {
	return d.runsLoading || d.wfLoading || d.approvalsLoading ||
		d.landingsLoading || d.issuesLoading || d.workspacesLoading
}

// --- Message types ---

// dashRunsFetchedMsg delivers run data to the dashboard.
type dashRunsFetchedMsg struct {
	runs []smithers.RunSummary
	err  error
}

// dashWorkflowsFetchedMsg delivers workflow data to the dashboard.
type dashWorkflowsFetchedMsg struct {
	workflows []smithers.Workflow
	err       error
}

// dashApprovalsFetchedMsg delivers approval data to the dashboard.
type dashApprovalsFetchedMsg struct {
	approvals []smithers.Approval
	err       error
}

// dashLandingsFetchedMsg delivers landing data from jjhub.
type dashLandingsFetchedMsg struct {
	landings []jjhub.Landing
	err      error
}

// dashIssuesFetchedMsg delivers issue data from jjhub.
type dashIssuesFetchedMsg struct {
	issues []jjhub.Issue
	err    error
}

// dashWorkspacesFetchedMsg delivers workspace data from jjhub.
type dashWorkspacesFetchedMsg struct {
	workspaces []jjhub.Workspace
	err        error
}

// InitSmithersMsg is returned when user selects "Init Smithers" from the dashboard.
type InitSmithersMsg struct{}

func NewDashboardView(args ...any) *DashboardView {
	com, client, offset := parseCommonAndClient(args)
	hasSmithers := client != nil
	if len(args) > offset {
		if provided, ok := args[offset].(bool); ok {
			hasSmithers = provided
		}
	}
	return NewDashboardViewWithJJHub(com, client, hasSmithers, nil)
}

func NewDashboardViewWithJJHub(com *common.Common, client *smithers.Client, hasSmithers bool, jc *jjhub.Client) *DashboardView {
	com = viewCommon(com)
	hasJJHub := jc != nil && jjhubAvailable()

	t := com.Styles
	brandingAnim := anim.New(anim.Settings{
		Size:        10,
		Label:       "SMITHERS",
		GradColorA:  t.Primary,
		GradColorB:  t.Secondary,
		LabelColor:  t.Primary,
		CycleColors: true,
	})

	d := &DashboardView{
		com:               com,
		client:            client,
		jjhubClient:       jc,
		jjhubEnabled:      hasJJHub,
		runsLoading:       hasSmithers,
		wfLoading:         hasSmithers,
		approvalsLoading:  hasSmithers,
		landingsLoading:   hasJJHub,
		issuesLoading:     hasJJHub,
		workspacesLoading: hasJJHub,
		brandingAnim:      brandingAnim,
	}

	// Build the instance-level tab list.
	baseTabs := []DashboardTab{DashTabOverview, DashTabRuns, DashTabWorkflows, DashTabSessions}
	if hasJJHub {
		baseTabs = append(baseTabs, DashTabLandings, DashTabIssues, DashTabWorkspaces)
	}
	d.tabs = baseTabs

	// Setup menu items
	d.menuItems = []menuItem{
		{icon: "⚡", label: "Run Workflow", desc: "Choose a workflow to execute", action: func() tea.Msg { d.activeTab = 2; return nil }},
		{icon: "💬", label: "New Chat", desc: "Start a new AI session", action: func() tea.Msg {
			return DashboardNavigateMsg{View: "chat"}
		}},
		{icon: "📁", label: "Browse Sessions", desc: "Open recent chat history", action: func() tea.Msg { d.activeTab = 3; return nil }},
	}
	if !hasSmithers {
		d.menuItems = append([]menuItem{{icon: "✨", label: "Initialize Smithers", desc: "Set up smithers in this repo", action: func() tea.Msg { return InitSmithersMsg{} }}}, d.menuItems...)
	}

	return d
}

func (d *DashboardView) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, d.brandingAnim.Start())
	if d.client != nil {
		cmds = append(cmds, d.fetchRuns(), d.fetchWorkflows(), d.fetchApprovals())
	}
	if d.jjhubEnabled {
		cmds = append(cmds, d.fetchLandings(), d.fetchIssues(), d.fetchWorkspaces(), d.fetchRepoName())
	}
	return tea.Batch(cmds...)
}

func (d *DashboardView) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case anim.StepMsg:
		if d.isLoading() {
			return d, d.brandingAnim.Animate(msg)
		}
		return d, nil

	case dashRunsFetchedMsg:
		d.runsLoading = false
		d.runsErr = msg.err
		if msg.err == nil {
			d.runs = msg.runs
		}

	case dashWorkflowsFetchedMsg:
		d.wfLoading = false
		d.wfErr = msg.err
		if msg.err == nil {
			d.workflows = msg.workflows
		}

	case dashApprovalsFetchedMsg:
		d.approvalsLoading = false
		d.approvalsErr = msg.err
		if msg.err == nil {
			d.approvals = msg.approvals
		}

	case dashLandingsFetchedMsg:
		d.landingsLoading = false
		d.landingsErr = msg.err
		if msg.err == nil {
			d.landings = msg.landings
		}

	case dashIssuesFetchedMsg:
		d.issuesLoading = false
		d.issuesErr = msg.err
		if msg.err == nil {
			d.issues = msg.issues
		}

	case dashWorkspacesFetchedMsg:
		d.workspacesLoading = false
		d.workspacesErr = msg.err
		if msg.err == nil {
			d.workspaces = msg.workspaces
		}

	case dashRepoNameFetchedMsg:
		d.repoName = msg.name

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7"))):
			idx := int(msg.String()[0] - '1')
			if idx < len(d.tabs) {
				d.activeTab = idx
				d.clampCursor()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if d.tabs[d.activeTab] == DashTabOverview {
				if d.menuCursor > 0 {
					d.menuCursor--
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if d.tabs[d.activeTab] == DashTabOverview {
				if d.menuCursor < len(d.menuItems)-1 {
					d.menuCursor++
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if d.tabs[d.activeTab] == DashTabOverview {
				item := d.menuItems[d.menuCursor]
				if item.action != nil {
					if res := item.action(); res != nil {
						return d, func() tea.Msg { return res }
					}
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return d, d.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			return d, func() tea.Msg { return DashboardNavigateMsg{View: "chat"} }
		}
	}

	return d, tea.Batch(cmds...)
}

// --- Rendering ---

func (d *DashboardView) renderHeader() string {
	t := d.com.Styles
	var logoText string
	if d.isLoading() {
		logoText = d.brandingAnim.Render()
	} else {
		logoText = styles.ApplyBoldForegroundGrad(d.com.Styles, "SMITHERS", d.com.Styles.Primary, d.com.Styles.Secondary)
	}
	logo := "◆ " + logoText

	// Show jjhub repo name in header if available.
	if d.repoName != "" {
		logo += lipgloss.NewStyle().Foreground(t.FgSubtle).Render("  " + d.repoName)
	}

	status := ""

	activeCount := 0
	for _, r := range d.runs {
		if r.Status == smithers.RunStatusRunning || r.Status == smithers.RunStatusWaitingApproval {
			activeCount++
		}
	}
	if activeCount > 0 {
		status += lipgloss.NewStyle().Foreground(t.Green).Render(fmt.Sprintf("● %d active", activeCount))
	}
	if len(d.approvals) > 0 {
		if status != "" {
			status += "  "
		}
		style := lipgloss.NewStyle().Foreground(t.Warning).Bold(true)
		if len(d.approvals) >= 5 {
			style = style.Foreground(t.Error)
		}
		status += style.Render(fmt.Sprintf("⚠ %d pending approval%s", len(d.approvals), pluralS(len(d.approvals))))
	}

	// JJHub open landings / issues counts.
	if d.jjhubEnabled && !d.landingsLoading {
		openLandings := 0
		for _, l := range d.landings {
			if l.State == "open" {
				openLandings++
			}
		}
		if openLandings > 0 {
			if status != "" {
				status += "  "
			}
			status += lipgloss.NewStyle().Foreground(t.Primary).Render(fmt.Sprintf("⬆ %d landing%s", openLandings, pluralS(openLandings)))
		}
	}

	gap := d.width - lipgloss.Width(logo) - lipgloss.Width(status) - 4
	if gap < 1 {
		gap = 1
	}
	line := "  " + logo + strings.Repeat(" ", gap) + status
	return lipgloss.NewStyle().
		Background(t.BgBaseLighter).
		Width(d.width).
		Render(line)
}

func (d *DashboardView) renderTabBar() string {
	t := d.com.Styles
	var tabs []string
	for i, tabType := range d.tabs {
		num := fmt.Sprintf("%d", i+1)
		label := tabType.String()
		if i == d.activeTab {
			tab := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(num) +
				" " + lipgloss.NewStyle().Bold(true).Underline(true).Render(label)
			tabs = append(tabs, tab)
		} else {
			tab := lipgloss.NewStyle().Foreground(t.FgSubtle).Render(num) +
				" " + lipgloss.NewStyle().Foreground(t.FgSubtle).Render(label)
			tabs = append(tabs, tab)
		}
	}
	bar := " " + strings.Join(tabs, "   ")
	return lipgloss.NewStyle().
		Background(t.BgBase).
		Width(d.width).
		Render(bar)
}

func (d *DashboardView) renderContent(height int) string {
	switch d.tabs[d.activeTab] {
	case DashTabOverview:
		return d.renderOverview(height)
	case DashTabRuns:
		return d.renderRunsSummary(height)
	case DashTabWorkflows:
		return d.renderWorkflowsSummary(height)
	case DashTabLandings:
		return d.renderLandingsSummary(height)
	case DashTabIssues:
		return d.renderIssuesSummary(height)
	case DashTabWorkspaces:
		return d.renderWorkspacesSummary(height)
	case DashTabSessions:
		return d.renderSessionsSummary(height)
	default:
		return ""
	}
}

func (d *DashboardView) renderOverview(height int) string {
	t := d.com.Styles
	var b strings.Builder

	// Quick actions menu
	b.WriteString("\n")
	for i, item := range d.menuItems {
		cursor := "  "
		style := t.Base
		if i == d.menuCursor {
			cursor = "▸ "
			style = style.Bold(true).Foreground(t.Primary)
		}
		b.WriteString(cursor + item.icon + " " + style.Render(item.label))
		b.WriteString("  " + t.Subtle.Render(item.desc))
		b.WriteString("\n")
	}

	// At-a-glance stats
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("  At a Glance") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.runsLoading {
		b.WriteString("  ⟳ Loading runs...\n")
	} else if d.runsErr != nil {
		b.WriteString("  " + t.Subtle.Render("No runs data") + "\n")
	} else {
		running, waiting, completed, failed := 0, 0, 0, 0
		for _, r := range d.runs {
			switch r.Status {
			case smithers.RunStatusRunning:
				running++
			case smithers.RunStatusWaitingApproval:
				waiting++
			case smithers.RunStatusFinished:
				completed++
			case smithers.RunStatusFailed:
				failed++
			}
		}
		b.WriteString(fmt.Sprintf("  Runs: %d total", len(d.runs)))
		if running > 0 {
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(t.Green).Render(fmt.Sprintf("● %d running", running))))
		}
		if waiting > 0 {
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(t.Warning).Render(fmt.Sprintf("⚠ %d waiting", waiting))))
		}
		if failed > 0 {
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(t.Error).Render(fmt.Sprintf("✗ %d failed", failed))))
		}
		b.WriteString("\n")
	}

	if d.wfLoading {
		b.WriteString("  ⟳ Loading workflows...\n")
	} else if d.wfErr != nil {
		b.WriteString("  " + t.Subtle.Render("No workflow data") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  Workflows: %d available\n", len(d.workflows)))
	}

	if d.approvalsLoading {
		b.WriteString("  ⟳ Loading approvals...\n")
	} else if d.approvalsErr != nil {
		b.WriteString("  " + t.Subtle.Render("No approval data") + "\n")
	} else if len(d.approvals) > 0 {
		style := lipgloss.NewStyle().Foreground(t.Warning).Bold(true)
		if len(d.approvals) >= 5 {
			style = style.Foreground(t.Error)
		}
		b.WriteString(fmt.Sprintf("  %s\n", style.Render(fmt.Sprintf("⚠ Approvals: %d pending", len(d.approvals)))))
		for i, a := range d.approvals {
			if i >= 3 {
				b.WriteString(fmt.Sprintf("    ... and %d more\n", len(d.approvals)-3))
				break
			}
			gate := a.Gate
			if gate == "" {
				gate = "approval gate"
			}
			id := a.RunID
			if len(id) > 8 {
				id = id[:8]
			}
			b.WriteString(fmt.Sprintf("    %s %s\n", t.Subtle.Render(id), gate))
		}
	} else {
		b.WriteString("  Approvals: " + lipgloss.NewStyle().Foreground(t.Green).Render("none pending ✓") + "\n")
	}

	// JJHub at-a-glance
	if d.jjhubEnabled {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("  Codeplane") + "\n")
		b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

		if d.landingsLoading {
			b.WriteString("  ⟳ Loading landings...\n")
		} else if d.landingsErr != nil {
			b.WriteString("  " + t.Subtle.Render("No landings data") + "\n")
		} else {
			open, merged, draft := 0, 0, 0
			for _, l := range d.landings {
				switch l.State {
				case "open":
					open++
				case "merged":
					merged++
				case "draft":
					draft++
				}
			}
			b.WriteString(fmt.Sprintf("  Landings: %d total", len(d.landings)))
			if open > 0 {
				b.WriteString("  " + jjLandingStateStyle(t, "open").Render(fmt.Sprintf("⬆ %d open", open)))
			}
			if draft > 0 {
				b.WriteString("  " + jjLandingStateStyle(t, "draft").Render(fmt.Sprintf("◌ %d draft", draft)))
			}
			if merged > 0 {
				b.WriteString("  " + jjLandingStateStyle(t, "merged").Render(fmt.Sprintf("✓ %d merged", merged)))
			}
			b.WriteString("\n")
		}

		if d.issuesLoading {
			b.WriteString("  ⟳ Loading issues...\n")
		} else if d.issuesErr != nil {
			b.WriteString("  " + t.Subtle.Render("No issues data") + "\n")
		} else {
			openIssues := 0
			for _, iss := range d.issues {
				if iss.State == "open" {
					openIssues++
				}
			}
			b.WriteString(fmt.Sprintf("  Issues: %d total", len(d.issues)))
			if openIssues > 0 {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(t.Green).Render(fmt.Sprintf("◉ %d open", openIssues)))
			}
			b.WriteString("\n")
		}

		if d.workspacesLoading {
			b.WriteString("  ⟳ Loading workspaces...\n")
		} else if d.workspacesErr != nil {
			b.WriteString("  " + t.Subtle.Render("No workspaces data") + "\n")
		} else {
			running := 0
			for _, w := range d.workspaces {
				if w.Status == "running" {
					running++
				}
			}
			b.WriteString(fmt.Sprintf("  Workspaces: %d total", len(d.workspaces)))
			if running > 0 {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(t.Green).Render(fmt.Sprintf("● %d running", running)))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (d *DashboardView) renderRunsSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Recent Runs") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.runsLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if len(d.runs) == 0 {
		b.WriteString("  " + t.Subtle.Render("No runs yet. Start a workflow to see runs here.") + "\n")
		b.WriteString("\n  Press " + lipgloss.NewStyle().Bold(true).Render("Enter") + " to open the full Run Dashboard\n")
		return b.String()
	}

	limit := height - 5
	if limit > len(d.runs) {
		limit = len(d.runs)
	}
	if limit > 10 {
		limit = 10
	}

	for i := 0; i < limit; i++ {
		r := d.runs[i]
		status := statusGlyph(t, r.Status)
		id := r.RunID
		if len(id) > 8 {
			id = id[:8]
		}
		wf := r.WorkflowName
		if wf == "" {
			wf = r.WorkflowPath
		}
		b.WriteString(fmt.Sprintf("  %s %s  %s\n", status, id, wf))
	}

	if len(d.runs) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more. Press Enter for full dashboard.\n", len(d.runs)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderWorkflowsSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Available Workflows") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.wfLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if len(d.workflows) == 0 {
		b.WriteString("  " + t.Subtle.Render("No workflows found in .smithers/workflows/") + "\n")
		return b.String()
	}

	limit := height - 5
	if limit > len(d.workflows) {
		limit = len(d.workflows)
	}
	if limit > 15 {
		limit = 15
	}

	for i := 0; i < limit; i++ {
		wf := d.workflows[i]
		name := wf.Name
		if name == "" {
			name = wf.ID
		}
		b.WriteString(fmt.Sprintf("  ⚡ %-25s %s\n", name, t.Subtle.Render(wf.RelativePath)))
	}

	if len(d.workflows) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more. Press Enter for full list.\n", len(d.workflows)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderLandingsSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Landing Requests") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.landingsLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.landingsErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(t.Error).Render("✗ "+d.landingsErr.Error()) + "\n")
		return b.String()
	}
	if len(d.landings) == 0 {
		b.WriteString("  " + t.Subtle.Render("No landing requests found.") + "\n")
		return b.String()
	}

	limit := height - 5
	if limit > len(d.landings) {
		limit = len(d.landings)
	}
	if limit > 15 {
		limit = 15
	}

	// Header row
	b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-40s  %-14s  %-7s  %s\n",
		t.Subtle.Render(""),
		t.Subtle.Render("#"),
		t.Subtle.Render("Title"),
		t.Subtle.Render("Author"),
		t.Subtle.Render("Changes"),
		t.Subtle.Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		l := d.landings[i]
		icon := jjLandingStateIcon(t, l.State)
		num := fmt.Sprintf("#%d", l.Number)
		title := truncateStr(l.Title, 40)
		author := truncateStr(l.Author.Login, 14)
		changes := fmt.Sprintf("%d", len(l.ChangeIDs))
		updated := jjRelativeTime(l.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-40s  %-14s  %-7s  %s\n",
			icon, num, title, author, changes, t.Subtle.Render(updated)))
	}

	if len(d.landings) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.landings)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderIssuesSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Issues") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.issuesLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.issuesErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(t.Error).Render("✗ "+d.issuesErr.Error()) + "\n")
		return b.String()
	}
	if len(d.issues) == 0 {
		b.WriteString("  " + t.Subtle.Render("No issues found.") + "\n")
		return b.String()
	}

	limit := height - 5
	if limit > len(d.issues) {
		limit = len(d.issues)
	}
	if limit > 15 {
		limit = 15
	}

	// Header row
	b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-42s  %-14s  %-9s  %s\n",
		t.Subtle.Render(""),
		t.Subtle.Render("#"),
		t.Subtle.Render("Title"),
		t.Subtle.Render("Author"),
		t.Subtle.Render("Comments"),
		t.Subtle.Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		iss := d.issues[i]
		icon := jjIssueStateIcon(t, iss.State)
		num := fmt.Sprintf("#%d", iss.Number)
		title := truncateStr(iss.Title, 42)
		author := truncateStr(iss.Author.Login, 14)
		comments := fmt.Sprintf("%d", iss.CommentCount)
		updated := jjRelativeTime(iss.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-42s  %-14s  %-9s  %s\n",
			icon, num, title, author, comments, t.Subtle.Render(updated)))
	}

	if len(d.issues) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.issues)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderWorkspacesSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Workspaces") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")

	if d.workspacesLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.workspacesErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(t.Error).Render("✗ "+d.workspacesErr.Error()) + "\n")
		return b.String()
	}
	if len(d.workspaces) == 0 {
		b.WriteString("  " + t.Subtle.Render("No workspaces found.") + "\n")
		return b.String()
	}

	limit := height - 5
	if limit > len(d.workspaces) {
		limit = len(d.workspaces)
	}
	if limit > 15 {
		limit = 15
	}

	// Header row
	b.WriteString(fmt.Sprintf("  %-3s  %-20s  %-12s  %-14s  %-30s  %s\n",
		t.Subtle.Render(""),
		t.Subtle.Render("Name"),
		t.Subtle.Render("Status"),
		t.Subtle.Render("Persistence"),
		t.Subtle.Render("SSH"),
		t.Subtle.Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		w := d.workspaces[i]
		icon := jjWorkspaceStatusIcon(t, w.Status)
		name := w.Name
		if name == "" {
			name = t.Subtle.Render("(unnamed)")
		}
		name = truncateStr(name, 20)
		ssh := "-"
		if w.SSHHost != nil && *w.SSHHost != "" {
			ssh = truncateStr(*w.SSHHost, 30)
		}
		updated := jjRelativeTime(w.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-20s  %-12s  %-14s  %-30s  %s\n",
			icon, name, w.Status, w.Persistence, ssh, t.Subtle.Render(updated)))
	}

	if len(d.workspaces) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.workspaces)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderSessionsSummary(height int) string {
	t := d.com.Styles
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("Chat Sessions") + "\n")
	b.WriteString(t.Subtle.Render("  ─────────────") + "\n")
	b.WriteString("  " + t.Subtle.Render("Press 'c' to start a new chat session") + "\n")
	b.WriteString("  " + t.Subtle.Render("Or Ctrl+S from chat to browse sessions") + "\n")
	return b.String()
}

func (d *DashboardView) renderFooter() string {
	t := d.com.Styles
	sep := t.Subtle.Render(" │ ")
	numTabs := len(d.tabs)
	tabNums := "1-4"
	if numTabs > 4 {
		tabNums = fmt.Sprintf("1-%d", numTabs)
	}
	parts := []string{
		helpKV(t, "j/k", "nav"),
		helpKV(t, tabNums, "tabs"),
		helpKV(t, "enter", "select"),
		helpKV(t, "c", "chat"),
		helpKV(t, "r", "refresh"),
		helpKV(t, "q", "quit"),
	}
	line := " " + strings.Join(parts, sep)
	return lipgloss.NewStyle().
		Background(t.BgBaseLighter).
		Width(d.width).
		Render(line)
}

func (d *DashboardView) View() string {
	header := d.renderHeader()
	tabs := d.renderTabBar()
	content := d.renderContent(d.height - 4)
	footer := d.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		tabs,
		content,
		lipgloss.NewStyle().Height(d.height-lipgloss.Height(header)-lipgloss.Height(tabs)-lipgloss.Height(content)-1).Render(""),
		footer,
	)
}

// Name returns the view name.
func (d *DashboardView) Name() string {
	return "dashboard"
}

// SetSize stores the terminal dimensions for use during rendering.
func (d *DashboardView) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (d *DashboardView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("1-7"), key.WithHelp("1-7", "tabs")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "new chat")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func helpKV(t *styles.Styles, k, v string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(k) + " " + t.Subtle.Render(v)
}

// --- Helpers ---

func (d *DashboardView) clampCursor() {
	max := len(d.menuItems) - 1
	if len(d.tabs) > 0 && d.tabs[d.activeTab] != DashTabOverview {
		max = 0
	}
	if d.menuCursor < 0 {
		d.menuCursor = 0
	}
	if d.menuCursor > max {
		d.menuCursor = max
	}
}

func (d *DashboardView) fetchRuns() tea.Cmd {
	client := d.client
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		runs, err := client.ListRuns(context.Background(), smithers.RunFilter{Limit: 20})
		return dashRunsFetchedMsg{runs: runs, err: err}
	}
}

func (d *DashboardView) fetchWorkflows() tea.Cmd {
	client := d.client
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		wfs, err := client.ListWorkflows(context.Background())
		return dashWorkflowsFetchedMsg{workflows: wfs, err: err}
	}
}

func (d *DashboardView) fetchApprovals() tea.Cmd {
	client := d.client
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		approvals, err := client.ListPendingApprovals(context.Background())
		return dashApprovalsFetchedMsg{approvals: approvals, err: err}
	}
}

func (d *DashboardView) fetchLandings() tea.Cmd {
	jc := d.jjhubClient
	if jc == nil {
		return nil
	}
	return func() tea.Msg {
		landings, err := jc.ListLandings(context.Background(), "open", 30)
		return dashLandingsFetchedMsg{landings: landings, err: err}
	}
}

func (d *DashboardView) fetchIssues() tea.Cmd {
	jc := d.jjhubClient
	if jc == nil {
		return nil
	}
	return func() tea.Msg {
		issues, err := jc.ListIssues(context.Background(), "open", 30)
		return dashIssuesFetchedMsg{issues: issues, err: err}
	}
}

func (d *DashboardView) fetchWorkspaces() tea.Cmd {
	jc := d.jjhubClient
	if jc == nil {
		return nil
	}
	return func() tea.Msg {
		workspaces, err := jc.ListWorkspaces(context.Background(), 20)
		return dashWorkspacesFetchedMsg{workspaces: workspaces, err: err}
	}
}

// dashRepoNameFetchedMsg delivers the detected repo name.
type dashRepoNameFetchedMsg struct {
	name string
	err  error
}

func (d *DashboardView) fetchRepoName() tea.Cmd {
	jc := d.jjhubClient
	if jc == nil {
		return nil
	}
	return func() tea.Msg {
		repo, err := jc.GetCurrentRepo(context.Background())
		if err != nil {
			return dashRepoNameFetchedMsg{err: err}
		}
		return dashRepoNameFetchedMsg{name: repo.FullName}
	}
}

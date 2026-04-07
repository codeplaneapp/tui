package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
)

// DashboardTab identifies a tab on the Smithers homepage.
type DashboardTab int

const (
	DashTabOverview DashboardTab = iota
	DashTabRuns
	DashTabWorkflows
	DashTabSessions
	DashTabLandings
	DashTabIssues
	DashTabWorkspaces
)

func (t DashboardTab) String() string {
	switch t {
	case DashTabOverview:
		return "Overview"
	case DashTabRuns:
		return "Runs"
	case DashTabWorkflows:
		return "Workflows"
	case DashTabSessions:
		return "Sessions"
	case DashTabLandings:
		return "Landings"
	case DashTabIssues:
		return "Issues"
	case DashTabWorkspaces:
		return "Workspaces"
	default:
		return "?"
	}
}

// dashTabs is the ordered list of tabs to show; populated in NewDashboardView
// based on whether jjhub is available.
var dashTabs []DashboardTab

// OpenChatMsg signals the root model to switch to chat mode.
type OpenChatMsg struct{}

// DashboardNavigateMsg signals the root model to navigate to a named view.
type DashboardNavigateMsg struct {
	View string
}

// DashboardView is the Smithers homepage — a tabbed overview shown on startup.
type DashboardView struct {
	client       *smithers.Client
	jjhubClient  *jjhub.Client
	jjhubEnabled bool // true when jjhub CLI is available
	width        int
	height       int
	activeTab    int
	tabs         []DashboardTab // instance-level tab list (not the global)

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

func NewDashboardView(client *smithers.Client, hasSmithers bool) *DashboardView {
	return NewDashboardViewWithJJHub(client, hasSmithers, nil)
}

func NewDashboardViewWithJJHub(client *smithers.Client, hasSmithers bool, jc *jjhub.Client) *DashboardView {
	hasJJHub := jc != nil && jjhubAvailable()

	d := &DashboardView{
		client:            client,
		jjhubClient:       jc,
		jjhubEnabled:      hasJJHub,
		runsLoading:       hasSmithers,
		wfLoading:         hasSmithers,
		approvalsLoading:  hasSmithers,
		landingsLoading:   hasJJHub,
		issuesLoading:     hasJJHub,
		workspacesLoading: hasJJHub,
	}

	// Build the instance-level tab list.
	baseTabs := []DashboardTab{DashTabOverview, DashTabRuns, DashTabWorkflows, DashTabSessions}
	if hasJJHub {
		baseTabs = append(baseTabs, DashTabLandings, DashTabIssues, DashTabWorkspaces)
	}
	d.tabs = baseTabs

	// Keep the package-level dashTabs in sync (used by renderTabBar / renderContent).
	dashTabs = d.tabs

	if hasSmithers {
		d.menuItems = []menuItem{
			{icon: "💬", label: "Start Chat", desc: "Choose how you want to chat", action: func() tea.Msg { return DashboardNavigateMsg{View: "chat"} }},
			{icon: "📊", label: "Run Dashboard", desc: "View all workflow runs", action: func() tea.Msg { return DashboardNavigateMsg{View: "runs"} }},
			{icon: "⚡", label: "Workflows", desc: "Browse and run workflows", action: func() tea.Msg { return DashboardNavigateMsg{View: "workflows"} }},
			{icon: "✅", label: "Approvals", desc: "Manage pending approval gates", action: func() tea.Msg { return DashboardNavigateMsg{View: "approvals"} }},
			{icon: "🎫", label: "Tickets", desc: "Browse project tickets", action: func() tea.Msg { return DashboardNavigateMsg{View: "tickets"} }},
			{icon: "🔍", label: "SQL Browser", desc: "Query the Smithers database", action: func() tea.Msg { return DashboardNavigateMsg{View: "sql"} }},
		}
	} else {
		d.menuItems = []menuItem{
			{icon: "💬", label: "Start Chat", desc: "Choose how you want to chat", action: func() tea.Msg { return DashboardNavigateMsg{View: "chat"} }},
			{icon: "🚀", label: "Init Smithers", desc: "Set up .smithers/ workflows in this project", action: func() tea.Msg { return InitSmithersMsg{} }},
		}
	}

	// Append JJHub quick-action items when available.
	if hasJJHub {
		d.menuItems = append(d.menuItems,
			menuItem{icon: "Δ", label: "Changes", desc: "Inspect recent JJ changes and diffs", action: func() tea.Msg { return DashboardNavigateMsg{View: "changes"} }},
			menuItem{icon: "≋", label: "Status", desc: "Inspect working copy status and diff", action: func() tea.Msg { return DashboardNavigateMsg{View: "status"} }},
			menuItem{icon: "⬆", label: "Landings", desc: "Browse landing requests", action: func() tea.Msg { return DashboardNavigateMsg{View: "landings"} }},
			menuItem{icon: "◉", label: "Issues", desc: "Browse issues", action: func() tea.Msg { return DashboardNavigateMsg{View: "issues"} }},
			menuItem{icon: "▣", label: "Workspaces", desc: "Manage cloud workspaces", action: func() tea.Msg { return DashboardNavigateMsg{View: "workspaces"} }},
		)
	}

	return d
}

func (d *DashboardView) Init() tea.Cmd {
	var cmds []tea.Cmd
	if d.client != nil {
		cmds = append(cmds, d.fetchRuns(), d.fetchWorkflows(), d.fetchApprovals())
	}
	if d.jjhubEnabled {
		cmds = append(cmds, d.fetchLandings(), d.fetchIssues(), d.fetchWorkspaces(), d.fetchRepoName())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (d *DashboardView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case dashRunsFetchedMsg:
		d.runsLoading = false
		d.runsErr = msg.err
		if msg.err == nil {
			d.runs = msg.runs
		}
		return d, nil

	case dashWorkflowsFetchedMsg:
		d.wfLoading = false
		d.wfErr = msg.err
		if msg.err == nil {
			d.workflows = msg.workflows
		}
		return d, nil

	case dashApprovalsFetchedMsg:
		d.approvalsLoading = false
		d.approvalsErr = msg.err
		if msg.err == nil {
			d.approvals = msg.approvals
		}
		return d, nil

	case dashLandingsFetchedMsg:
		d.landingsLoading = false
		d.landingsErr = msg.err
		if msg.err == nil {
			d.landings = msg.landings
		}
		return d, nil

	case dashIssuesFetchedMsg:
		d.issuesLoading = false
		d.issuesErr = msg.err
		if msg.err == nil {
			d.issues = msg.issues
		}
		return d, nil

	case dashWorkspacesFetchedMsg:
		d.workspacesLoading = false
		d.workspacesErr = msg.err
		if msg.err == nil {
			d.workspaces = msg.workspaces
		}
		return d, nil

	case dashRepoNameFetchedMsg:
		if msg.err == nil {
			d.repoName = msg.name
		}
		return d, nil

	case tea.KeyPressMsg:
		switch {
		// Tab switching
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "l", "right"))):
			d.activeTab = (d.activeTab + 1) % len(d.tabs)
			d.menuCursor = 0
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "h", "left"))):
			d.activeTab--
			if d.activeTab < 0 {
				d.activeTab = len(d.tabs) - 1
			}
			d.menuCursor = 0
			return d, nil

		// Number keys for tabs
		case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
			d.activeTab = 0
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
			if len(d.tabs) > 1 {
				d.activeTab = 1
			}
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("3"))):
			if len(d.tabs) > 2 {
				d.activeTab = 2
			}
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("4"))):
			if len(d.tabs) > 3 {
				d.activeTab = 3
			}
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("5"))):
			if len(d.tabs) > 4 {
				d.activeTab = 4
			}
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("6"))):
			if len(d.tabs) > 5 {
				d.activeTab = 5
			}
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("7"))):
			if len(d.tabs) > 6 {
				d.activeTab = 6
			}
			return d, nil

		// Navigation within tab
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			d.menuCursor++
			d.clampCursor()
			return d, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			d.menuCursor--
			d.clampCursor()
			return d, nil

		// Enter to select
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(d.tabs) > 0 && d.tabs[d.activeTab] == DashTabOverview && d.menuCursor < len(d.menuItems) {
				return d, func() tea.Msg { return d.menuItems[d.menuCursor].action() }
			}
			if len(d.tabs) > 0 && d.tabs[d.activeTab] == DashTabRuns {
				return d, func() tea.Msg { return DashboardNavigateMsg{View: "runs"} }
			}
			if len(d.tabs) > 0 && d.tabs[d.activeTab] == DashTabWorkflows {
				return d, func() tea.Msg { return DashboardNavigateMsg{View: "workflows"} }
			}
			return d, nil

		// c for quick chat
		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			return d, func() tea.Msg { return DashboardNavigateMsg{View: "chat"} }

		// r to refresh
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			d.runsLoading = d.client != nil
			d.wfLoading = d.client != nil
			d.landingsLoading = d.jjhubEnabled
			d.issuesLoading = d.jjhubEnabled
			d.workspacesLoading = d.jjhubEnabled
			var cmds []tea.Cmd
			if d.client != nil {
				cmds = append(cmds, d.fetchRuns(), d.fetchWorkflows())
			}
			if d.jjhubEnabled {
				cmds = append(cmds, d.fetchLandings(), d.fetchIssues(), d.fetchWorkspaces())
			}
			return d, tea.Batch(cmds...)

		// q to quit (dashboard is the root, so quit the app)
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return d, tea.Quit
		}
	}
	return d, nil
}

func (d *DashboardView) View() string {
	if d.width == 0 {
		return "  Loading..."
	}

	var parts []string

	// Header
	parts = append(parts, d.renderHeader())

	// Tab bar
	parts = append(parts, d.renderTabBar())

	// Content
	contentHeight := d.height - 5 // header + tab + footer + borders
	if contentHeight < 3 {
		contentHeight = 3
	}
	parts = append(parts, d.renderContent(contentHeight))

	// Footer
	parts = append(parts, d.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (d *DashboardView) Name() string { return "Dashboard" }

func (d *DashboardView) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *DashboardView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// --- Rendering ---

func (d *DashboardView) renderHeader() string {
	logo := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Render("◆ SMITHERS")

	// Show jjhub repo name in header if available.
	if d.repoName != "" {
		logo += lipgloss.NewStyle().Faint(true).Render("  " + d.repoName)
	}

	status := ""

	activeCount := 0
	for _, r := range d.runs {
		if r.Status == smithers.RunStatusRunning || r.Status == smithers.RunStatusWaitingApproval {
			activeCount++
		}
	}
	if activeCount > 0 {
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("● %d active", activeCount))
	}
	if len(d.approvals) > 0 {
		if status != "" {
			status += "  "
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
		if len(d.approvals) >= 5 {
			style = style.Foreground(lipgloss.Color("1"))
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
			status += lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render(fmt.Sprintf("⬆ %d landing%s", openLandings, pluralS(openLandings)))
		}
	}

	gap := d.width - lipgloss.Width(logo) - lipgloss.Width(status) - 4
	if gap < 1 {
		gap = 1
	}
	line := "  " + logo + strings.Repeat(" ", gap) + status
	return lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Width(d.width).
		Render(line)
}

func (d *DashboardView) renderTabBar() string {
	var tabs []string
	for i, t := range d.tabs {
		num := fmt.Sprintf("%d", i+1)
		label := t.String()
		if i == d.activeTab {
			tab := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Render(num) +
				" " + lipgloss.NewStyle().Bold(true).Underline(true).Render(label)
			tabs = append(tabs, tab)
		} else {
			tab := lipgloss.NewStyle().Faint(true).Render(num) +
				" " + lipgloss.NewStyle().Faint(true).Render(label)
			tabs = append(tabs, tab)
		}
	}
	bar := " " + strings.Join(tabs, "   ")
	return lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Width(d.width).
		Render(bar)
}

func (d *DashboardView) renderContent(height int) string {
	if len(d.tabs) == 0 {
		return ""
	}
	switch d.tabs[d.activeTab] {
	case DashTabOverview:
		return d.renderOverview(height)
	case DashTabRuns:
		return d.renderRunsSummary(height)
	case DashTabWorkflows:
		return d.renderWorkflowsSummary(height)
	case DashTabSessions:
		return d.renderSessionsSummary(height)
	case DashTabLandings:
		return d.renderLandingsSummary(height)
	case DashTabIssues:
		return d.renderIssuesSummary(height)
	case DashTabWorkspaces:
		return d.renderWorkspacesSummary(height)
	}
	return ""
}

func (d *DashboardView) renderOverview(height int) string {
	var b strings.Builder

	// Quick actions menu
	b.WriteString("\n")
	for i, item := range d.menuItems {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == d.menuCursor {
			cursor = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("63"))
		}
		b.WriteString(cursor + item.icon + " " + style.Render(item.label))
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(item.desc))
		b.WriteString("\n")
	}

	// At-a-glance stats
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  At a Glance") + "\n")
	b.WriteString("  ─────────────\n")

	if d.runsLoading {
		b.WriteString("  ⟳ Loading runs...\n")
	} else if d.runsErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No runs data") + "\n")
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
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("● %d running", running))))
		}
		if waiting > 0 {
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf("⚠ %d waiting", waiting))))
		}
		if failed > 0 {
			b.WriteString(fmt.Sprintf("  %s", lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("✗ %d failed", failed))))
		}
		b.WriteString("\n")
	}

	if d.wfLoading {
		b.WriteString("  ⟳ Loading workflows...\n")
	} else if d.wfErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No workflow data") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  Workflows: %d available\n", len(d.workflows)))
	}

	if d.approvalsLoading {
		b.WriteString("  ⟳ Loading approvals...\n")
	} else if d.approvalsErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No approval data") + "\n")
	} else if len(d.approvals) > 0 {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
		if len(d.approvals) >= 5 {
			style = style.Foreground(lipgloss.Color("1"))
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
			b.WriteString(fmt.Sprintf("    %s %s\n", lipgloss.NewStyle().Faint(true).Render(id), gate))
		}
	} else {
		b.WriteString("  Approvals: " + lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("none pending ✓") + "\n")
	}

	// JJHub at-a-glance
	if d.jjhubEnabled {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Codeplane") + "\n")
		b.WriteString("  ─────────────\n")

		if d.landingsLoading {
			b.WriteString("  ⟳ Loading landings...\n")
		} else if d.landingsErr != nil {
			b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No landings data") + "\n")
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
				b.WriteString("  " + jjLandingStateStyle("open").Render(fmt.Sprintf("⬆ %d open", open)))
			}
			if draft > 0 {
				b.WriteString("  " + jjLandingStateStyle("draft").Render(fmt.Sprintf("◌ %d draft", draft)))
			}
			if merged > 0 {
				b.WriteString("  " + jjLandingStateStyle("merged").Render(fmt.Sprintf("✓ %d merged", merged)))
			}
			b.WriteString("\n")
		}

		if d.issuesLoading {
			b.WriteString("  ⟳ Loading issues...\n")
		} else if d.issuesErr != nil {
			b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No issues data") + "\n")
		} else {
			openIssues := 0
			for _, iss := range d.issues {
				if iss.State == "open" {
					openIssues++
				}
			}
			b.WriteString(fmt.Sprintf("  Issues: %d total", len(d.issues)))
			if openIssues > 0 {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("◉ %d open", openIssues)))
			}
			b.WriteString("\n")
		}

		if d.workspacesLoading {
			b.WriteString("  ⟳ Loading workspaces...\n")
		} else if d.workspacesErr != nil {
			b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No workspaces data") + "\n")
		} else {
			running := 0
			for _, w := range d.workspaces {
				if w.Status == "running" {
					running++
				}
			}
			b.WriteString(fmt.Sprintf("  Workspaces: %d total", len(d.workspaces)))
			if running > 0 {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("● %d running", running)))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (d *DashboardView) renderRunsSummary(height int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Recent Runs") + "\n")
	b.WriteString("  ─────────────\n")

	if d.runsLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if len(d.runs) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No runs yet. Start a workflow to see runs here.") + "\n")
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
		status := statusGlyph(r.Status)
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
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Available Workflows") + "\n")
	b.WriteString("  ─────────────\n")

	if d.wfLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if len(d.workflows) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No workflows found in .smithers/workflows/") + "\n")
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
		b.WriteString(fmt.Sprintf("  ⚡ %-25s %s\n", name, lipgloss.NewStyle().Faint(true).Render(wf.RelativePath)))
	}

	if len(d.workflows) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more. Press Enter for full list.\n", len(d.workflows)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderSessionsSummary(height int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Chat Sessions") + "\n")
	b.WriteString("  ─────────────\n")
	b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("Press 'c' to start a new chat session") + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("Or Ctrl+S from chat to browse sessions") + "\n")
	return b.String()
}

func (d *DashboardView) renderLandingsSummary(height int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Landing Requests") + "\n")
	b.WriteString("  ─────────────\n")

	if d.landingsLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.landingsErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ "+d.landingsErr.Error()) + "\n")
		return b.String()
	}
	if len(d.landings) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No landing requests found.") + "\n")
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
		lipgloss.NewStyle().Faint(true).Render(""),
		lipgloss.NewStyle().Faint(true).Render("#"),
		lipgloss.NewStyle().Faint(true).Render("Title"),
		lipgloss.NewStyle().Faint(true).Render("Author"),
		lipgloss.NewStyle().Faint(true).Render("Changes"),
		lipgloss.NewStyle().Faint(true).Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		l := d.landings[i]
		icon := jjLandingStateIcon(l.State)
		num := fmt.Sprintf("#%d", l.Number)
		title := truncateStr(l.Title, 40)
		author := truncateStr(l.Author.Login, 14)
		changes := fmt.Sprintf("%d", len(l.ChangeIDs))
		updated := jjRelativeTime(l.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-40s  %-14s  %-7s  %s\n",
			icon, num, title, author, changes, lipgloss.NewStyle().Faint(true).Render(updated)))
	}

	if len(d.landings) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.landings)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderIssuesSummary(height int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Issues") + "\n")
	b.WriteString("  ─────────────\n")

	if d.issuesLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.issuesErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ "+d.issuesErr.Error()) + "\n")
		return b.String()
	}
	if len(d.issues) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No issues found.") + "\n")
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
		lipgloss.NewStyle().Faint(true).Render(""),
		lipgloss.NewStyle().Faint(true).Render("#"),
		lipgloss.NewStyle().Faint(true).Render("Title"),
		lipgloss.NewStyle().Faint(true).Render("Author"),
		lipgloss.NewStyle().Faint(true).Render("Comments"),
		lipgloss.NewStyle().Faint(true).Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		iss := d.issues[i]
		icon := jjIssueStateIcon(iss.State)
		num := fmt.Sprintf("#%d", iss.Number)
		title := truncateStr(iss.Title, 42)
		author := truncateStr(iss.Author.Login, 14)
		comments := fmt.Sprintf("%d", iss.CommentCount)
		updated := jjRelativeTime(iss.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-5s  %-42s  %-14s  %-9s  %s\n",
			icon, num, title, author, comments, lipgloss.NewStyle().Faint(true).Render(updated)))
	}

	if len(d.issues) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.issues)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderWorkspacesSummary(height int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Workspaces") + "\n")
	b.WriteString("  ─────────────\n")

	if d.workspacesLoading {
		b.WriteString("  ⟳ Loading...\n")
		return b.String()
	}
	if d.workspacesErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ "+d.workspacesErr.Error()) + "\n")
		return b.String()
	}
	if len(d.workspaces) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("No workspaces found.") + "\n")
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
		lipgloss.NewStyle().Faint(true).Render(""),
		lipgloss.NewStyle().Faint(true).Render("Name"),
		lipgloss.NewStyle().Faint(true).Render("Status"),
		lipgloss.NewStyle().Faint(true).Render("Persistence"),
		lipgloss.NewStyle().Faint(true).Render("SSH"),
		lipgloss.NewStyle().Faint(true).Render("Updated"),
	))

	for i := 0; i < limit; i++ {
		w := d.workspaces[i]
		icon := jjWorkspaceStatusIcon(w.Status)
		name := w.Name
		if name == "" {
			name = lipgloss.NewStyle().Faint(true).Render("(unnamed)")
		}
		name = truncateStr(name, 20)
		ssh := "-"
		if w.SSHHost != nil && *w.SSHHost != "" {
			ssh = truncateStr(*w.SSHHost, 30)
		}
		updated := jjRelativeTime(w.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-3s  %-20s  %-12s  %-14s  %-30s  %s\n",
			icon, name, w.Status, w.Persistence, ssh, lipgloss.NewStyle().Faint(true).Render(updated)))
	}

	if len(d.workspaces) > limit {
		b.WriteString(fmt.Sprintf("\n  ... and %d more.\n", len(d.workspaces)-limit))
	}
	return b.String()
}

func (d *DashboardView) renderFooter() string {
	sep := lipgloss.NewStyle().Faint(true).Render(" │ ")
	numTabs := len(d.tabs)
	tabNums := "1-4"
	if numTabs > 4 {
		tabNums = fmt.Sprintf("1-%d", numTabs)
	}
	parts := []string{
		helpKV("j/k", "nav"),
		helpKV(tabNums, "tabs"),
		helpKV("enter", "select"),
		helpKV("c", "chat"),
		helpKV("r", "refresh"),
		helpKV("q", "quit"),
	}
	line := " " + strings.Join(parts, sep)
	return lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Width(d.width).
		Render(line)
}

func helpKV(k, v string) string {
	return lipgloss.NewStyle().Bold(true).Render(k) + " " + lipgloss.NewStyle().Faint(true).Render(v)
}

func statusGlyph(s smithers.RunStatus) string {
	switch s {
	case smithers.RunStatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("⚠")
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Faint(true).Render("✓")
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Faint(true).Render("–")
	default:
		return lipgloss.NewStyle().Faint(true).Render("○")
	}
}

func fmtDurationMs(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// --- JJHub icon / style helpers ---

func jjLandingStateIcon(state string) string {
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("⬆")
	case "merged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("✓")
	case "closed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	case "draft":
		return lipgloss.NewStyle().Faint(true).Render("◌")
	default:
		return lipgloss.NewStyle().Faint(true).Render("?")
	}
}

func jjLandingStateStyle(state string) lipgloss.Style {
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case "merged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	case "closed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	case "draft":
		return lipgloss.NewStyle().Faint(true)
	default:
		return lipgloss.NewStyle().Faint(true)
	}
}

func jjIssueStateIcon(state string) string {
	switch state {
	case "open":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("◉")
	case "closed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("◎")
	default:
		return lipgloss.NewStyle().Faint(true).Render("?")
	}
}

func jjWorkspaceStatusIcon(status string) string {
	switch status {
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	case "pending":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("◌")
	case "stopped":
		return lipgloss.NewStyle().Faint(true).Render("○")
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	default:
		return lipgloss.NewStyle().Faint(true).Render("?")
	}
}

// jjRelativeTime converts an RFC3339 timestamp string to a relative time string.
func jjRelativeTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return ts
		}
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
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

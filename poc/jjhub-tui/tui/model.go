package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/poc/jjhub-tui/jjhub"
)

// ---- Messages ----

type landingsFetchedMsg struct {
	landings []jjhub.Landing
	err      error
}

type issuesFetchedMsg struct {
	issues []jjhub.Issue
	err    error
}

type reposFetchedMsg struct {
	repos []jjhub.Repo
	err   error
}

type notificationsFetchedMsg struct {
	notifications []jjhub.Notification
	err           error
}

type workspacesFetchedMsg struct {
	workspaces []jjhub.Workspace
	err        error
}

type workflowsFetchedMsg struct {
	workflows []jjhub.Workflow
	err       error
}

type repoInfoMsg struct {
	repo *jjhub.Repo
	err  error
}

// ---- Model ----

type Model struct {
	client    *jjhub.Client
	tabs      []TabKind
	activeTab int
	sections  map[TabKind]*Section

	// Layout
	width       int
	height      int
	previewOpen bool
	showHelp    bool

	// Search
	searching   bool
	searchInput string

	// Repo info (shown in header).
	repoName string
}

func NewModel(repo string) *Model {
	client := jjhub.NewClient(repo)
	sections := make(map[TabKind]*Section)
	sections[TabLandings] = NewLandingsSection()
	sections[TabIssues] = NewIssuesSection()
	sections[TabWorkspaces] = NewWorkspacesSection()
	sections[TabWorkflows] = NewWorkflowsSection()
	sections[TabRepos] = NewReposSection()
	sections[TabNotifications] = NewNotificationsSection()

	return &Model{
		client:      client,
		tabs:        allTabs,
		activeTab:   0,
		sections:    sections,
		previewOpen: true,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchLandings("open"),
		m.fetchIssues("open"),
		m.fetchWorkspaces(),
		m.fetchWorkflows(),
		m.fetchRepos(),
		m.fetchNotifications(),
		m.fetchRepoInfo(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	// ---- Data fetch results ----
	case repoInfoMsg:
		if msg.err == nil && msg.repo != nil {
			m.repoName = msg.repo.FullName
			if m.repoName == "" {
				m.repoName = msg.repo.Name
			}
		}
		return m, nil

	case landingsFetchedMsg:
		s := m.sections[TabLandings]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildLandingRows(msg.landings)
		}
		return m, nil

	case issuesFetchedMsg:
		s := m.sections[TabIssues]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildIssueRows(msg.issues)
		}
		return m, nil

	case reposFetchedMsg:
		s := m.sections[TabRepos]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildRepoRows(msg.repos)
		}
		return m, nil

	case notificationsFetchedMsg:
		s := m.sections[TabNotifications]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildNotificationRows(msg.notifications)
		}
		return m, nil

	case workspacesFetchedMsg:
		s := m.sections[TabWorkspaces]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildWorkspaceRows(msg.workspaces)
		}
		return m, nil

	case workflowsFetchedMsg:
		s := m.sections[TabWorkflows]
		if msg.err != nil {
			s.SetError(msg.err)
		} else {
			s.BuildWorkflowRows(msg.workflows)
		}
		return m, nil

	// ---- Keyboard ----
	case tea.KeyMsg:
		// Search mode intercepts all keys.
		if m.searching {
			return m.updateSearch(msg)
		}

		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		sect := m.currentSection()
		pageSize := m.contentHeight() / 2
		if pageSize < 1 {
			pageSize = 1
		}

		switch {
		case key.Matches(msg, defaultKeys.Quit):
			return m, tea.Quit

		case key.Matches(msg, defaultKeys.Help):
			m.showHelp = true
			return m, nil

		// Tab switching by number.
		case key.Matches(msg, defaultKeys.Num1):
			return m, m.switchTab(0)
		case key.Matches(msg, defaultKeys.Num2):
			return m, m.switchTab(1)
		case key.Matches(msg, defaultKeys.Num3):
			return m, m.switchTab(2)
		case key.Matches(msg, defaultKeys.Num4):
			return m, m.switchTab(3)
		case key.Matches(msg, defaultKeys.Num5):
			return m, m.switchTab(4)
		case key.Matches(msg, defaultKeys.Num6):
			return m, m.switchTab(5)

		case key.Matches(msg, defaultKeys.Tab, defaultKeys.Right):
			m.nextTab()
			return m, nil
		case key.Matches(msg, defaultKeys.ShiftTab, defaultKeys.Left):
			m.prevTab()
			return m, nil

		case key.Matches(msg, defaultKeys.Down):
			sect.CursorDown()
			return m, nil
		case key.Matches(msg, defaultKeys.Up):
			sect.CursorUp()
			return m, nil
		case key.Matches(msg, defaultKeys.GotoTop):
			sect.GotoTop()
			return m, nil
		case key.Matches(msg, defaultKeys.GotoBottom):
			sect.GotoBottom()
			return m, nil
		case key.Matches(msg, defaultKeys.PageDown):
			sect.PageDown(pageSize)
			return m, nil
		case key.Matches(msg, defaultKeys.PageUp):
			sect.PageUp(pageSize)
			return m, nil

		case key.Matches(msg, defaultKeys.Preview):
			m.previewOpen = !m.previewOpen
			return m, nil

		case key.Matches(msg, defaultKeys.Refresh):
			return m, m.refreshAll()

		case key.Matches(msg, defaultKeys.Filter):
			return m, m.cycleFilter()

		case key.Matches(msg, defaultKeys.Search):
			m.searching = true
			m.searchInput = ""
			return m, nil

		case key.Matches(msg, defaultKeys.Escape):
			// Clear search if active.
			if sect.Search != "" {
				sect.Search = ""
				return m, m.rebuildCurrentSection()
			}
			return m, nil
		}
	}

	return m, nil
}

// ---- Search mode ----

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape"))):
		m.searching = false
		m.searchInput = ""
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		m.searching = false
		sect := m.currentSection()
		sect.Search = m.searchInput
		return m, m.rebuildCurrentSection()

	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}
		return m, nil

	default:
		// Append printable characters.
		r := msg.String()
		if len(r) == 1 && r[0] >= 32 {
			m.searchInput += r
		}
		return m, nil
	}
}

func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.width == 0 || m.height == 0 {
		v.Content = spinnerStyle.Render("  ⟳ Loading...")
		return v
	}

	if m.showHelp {
		v.Content = m.viewHelp()
		return v
	}

	var parts []string

	// Header bar.
	parts = append(parts, m.viewHeader())

	// Tab bar.
	parts = append(parts, m.viewTabBar())

	// Search bar (if searching).
	if m.searching {
		parts = append(parts, m.viewSearchBar())
	}

	// Main content area.
	ch := m.contentHeight()
	if m.searching {
		ch-- // search bar takes one line
	}
	parts = append(parts, m.viewContent(ch))

	// Footer.
	parts = append(parts, m.viewFooter())

	v.Content = lipgloss.JoinVertical(lipgloss.Left, parts...)
	return v
}

// contentHeight returns the available height for the table.
func (m *Model) contentHeight() int {
	h := m.height - 5 // header + tab bar + footer + borders
	if h < 3 {
		h = 3
	}
	return h
}

// ---- Header ----

func (m *Model) viewHeader() string {
	logo := logoStyle.Render("◆ Codeplane")
	right := ""
	if m.repoName != "" {
		right = repoNameStyle.Render(m.repoName)
	}
	gap := m.width - lipgloss.Width(logo) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	line := logo + strings.Repeat(" ", gap) + right
	return headerBarStyle.Width(m.width).Render(line)
}

// ---- Tab bar ----

func (m *Model) viewTabBar() string {
	var tabs []string
	for i, t := range m.tabs {
		num := fmt.Sprintf("%d", i+1)
		label := t.String()
		sect := m.sections[t]
		count := ""
		if !sect.Loading && sect.Error == "" {
			count = tabCountStyle.Render(fmt.Sprintf(" %d", len(sect.Rows)))
		}

		if i == m.activeTab {
			tab := activeTabNumStyle.Render(num) + " " + activeTabStyle.Render(label) + count
			tabs = append(tabs, tab)
		} else {
			tab := inactiveTabNumStyle.Render(num) + " " + inactiveTabStyle.Render(label) + count
			tabs = append(tabs, tab)
		}
	}
	bar := strings.Join(tabs, "  ")
	return tabBarStyle.Width(m.width).Render(" " + bar)
}

// ---- Search bar ----

func (m *Model) viewSearchBar() string {
	prompt := searchPromptStyle.Render(" / ")
	input := searchInputStyle.Render(m.searchInput)
	cursor := "█"
	return searchBarStyle.Width(m.width).Render(prompt + input + cursor)
}

// ---- Main content ----

func (m *Model) viewContent(height int) string {
	sect := m.currentSection()

	if !m.previewOpen || m.width < 60 {
		return sect.ViewTable(m.width, height)
	}

	// Split: table on left, preview on right.
	previewWidth := m.width * 38 / 100
	if previewWidth > 60 {
		previewWidth = 60
	}
	if previewWidth < 25 {
		previewWidth = 25
	}
	tableWidth := m.width - previewWidth - 1

	table := sect.ViewTable(tableWidth, height)

	previewContent := sect.PreviewContent(previewWidth)
	preview := sidebarStyle.
		Width(previewWidth - 4).
		Height(height).
		Render(previewContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, table, preview)
}

// ---- Footer ----

func (m *Model) viewFooter() string {
	sep := footerSepStyle.Render("│")
	var parts []string

	// Context-aware actions based on current tab.
	switch m.tabs[m.activeTab] {
	case TabLandings:
		sect := m.sections[TabLandings]
		parts = append(parts, helpPair("s", "filter:"+sect.FilterLabel))
	case TabIssues:
		sect := m.sections[TabIssues]
		parts = append(parts, helpPair("s", "filter:"+sect.FilterLabel))
	}

	// Search indicator.
	sect := m.currentSection()
	if sect.Search != "" {
		parts = append(parts, footerFilterStyle.Render("/"+sect.Search))
	}

	parts = append(parts,
		sep,
		helpPair("j/k", "nav"),
		helpPair("1-6", "tabs"),
		helpPair("w", "preview"),
		helpPair("/", "search"),
		helpPair("R", "refresh"),
		helpPair("?", "help"),
	)

	line := strings.Join(parts, "  ")
	return footerStyle.Width(m.width).Render(line)
}

func helpPair(k, desc string) string {
	return footerKeyStyle.Render(k) + " " + footerDescStyle.Render(desc)
}

// ---- Help overlay ----

func (m *Model) viewHelp() string {
	title := titleStyle.Render("◆ Codeplane TUI — Keyboard Shortcuts")

	sections := []struct {
		name string
		keys []struct{ key, desc string }
	}{
		{"Navigation", []struct{ key, desc string }{
			{"j / ↓", "Move cursor down"},
			{"k / ↑", "Move cursor up"},
			{"g", "Go to top"},
			{"G", "Go to bottom"},
			{"Ctrl+d", "Page down"},
			{"Ctrl+u", "Page up"},
		}},
		{"Tabs", []struct{ key, desc string }{
			{"1-6", "Jump to tab by number"},
			{"l / → / Tab", "Next tab"},
			{"h / ← / S-Tab", "Previous tab"},
		}},
		{"Views", []struct{ key, desc string }{
			{"w", "Toggle preview sidebar"},
			{"s", "Cycle state filter (landings/issues)"},
			{"/", "Search within current tab"},
			{"Esc", "Clear search / go back"},
		}},
		{"Actions", []struct{ key, desc string }{
			{"R", "Refresh all tabs"},
			{"?", "Toggle this help"},
			{"q / Ctrl+C", "Quit"},
		}},
	}

	var lines []string
	for _, s := range sections {
		lines = append(lines, helpSectionStyle.Render(s.name))
		for _, h := range s.keys {
			lines = append(lines, "  "+helpKeyStyle.Render(h.key)+helpDescStyle.Render(h.desc))
		}
	}

	body := strings.Join(lines, "\n")
	content := title + "\n\n" + body + "\n\n" + dimStyle.Render("  Press any key to close")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// ---- Tab navigation ----

func (m *Model) nextTab() {
	m.activeTab = (m.activeTab + 1) % len(m.tabs)
}

func (m *Model) prevTab() {
	m.activeTab--
	if m.activeTab < 0 {
		m.activeTab = len(m.tabs) - 1
	}
}

func (m *Model) switchTab(index int) tea.Cmd {
	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
	}
	return nil
}

func (m *Model) currentSection() *Section {
	return m.sections[m.tabs[m.activeTab]]
}

// ---- Filter cycling ----

func (m *Model) cycleFilter() tea.Cmd {
	tab := m.tabs[m.activeTab]
	sect := m.sections[tab]

	switch tab {
	case TabLandings:
		sect.FilterIndex = (sect.FilterIndex + 1) % len(landingFilters)
		f := landingFilters[sect.FilterIndex]
		sect.FilterLabel = f.label
		sect.Loading = true
		return m.fetchLandings(f.value)

	case TabIssues:
		sect.FilterIndex = (sect.FilterIndex + 1) % len(issueFilters)
		f := issueFilters[sect.FilterIndex]
		sect.FilterLabel = f.label
		sect.Loading = true
		return m.fetchIssues(f.value)
	}
	return nil
}

// ---- Rebuild section (after search change) ----

func (m *Model) rebuildCurrentSection() tea.Cmd {
	sect := m.currentSection()
	switch sect.Kind {
	case TabLandings:
		sect.BuildLandingRows(sect.Landings)
	case TabIssues:
		sect.BuildIssueRows(sect.Issues)
	case TabWorkspaces:
		sect.BuildWorkspaceRows(sect.Workspaces)
	case TabWorkflows:
		sect.BuildWorkflowRows(sect.Workflows)
	case TabRepos:
		sect.BuildRepoRows(sect.Repos)
	case TabNotifications:
		sect.BuildNotificationRows(sect.Notifications)
	}
	return nil
}

// ---- Data fetching (tea.Cmd) ----

func (m *Model) fetchRepoInfo() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		return repoInfoMsg{repo: repo, err: err}
	}
}

func (m *Model) fetchLandings(state string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		landings, err := client.ListLandings(state, 50)
		return landingsFetchedMsg{landings: landings, err: err}
	}
}

func (m *Model) fetchIssues(state string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		issues, err := client.ListIssues(state, 50)
		return issuesFetchedMsg{issues: issues, err: err}
	}
}

func (m *Model) fetchRepos() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		repos, err := client.ListRepos(50)
		return reposFetchedMsg{repos: repos, err: err}
	}
}

func (m *Model) fetchNotifications() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		notifs, err := client.ListNotifications(50)
		return notificationsFetchedMsg{notifications: notifs, err: err}
	}
}

func (m *Model) fetchWorkspaces() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ws, err := client.ListWorkspaces(50)
		return workspacesFetchedMsg{workspaces: ws, err: err}
	}
}

func (m *Model) fetchWorkflows() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		wf, err := client.ListWorkflows(50)
		return workflowsFetchedMsg{workflows: wf, err: err}
	}
}

func (m *Model) refreshAll() tea.Cmd {
	for _, s := range m.sections {
		s.Loading = true
	}

	landingFilter := "open"
	if s := m.sections[TabLandings]; s.FilterIndex < len(landingFilters) {
		landingFilter = landingFilters[s.FilterIndex].value
	}
	issueFilter := "open"
	if s := m.sections[TabIssues]; s.FilterIndex < len(issueFilters) {
		issueFilter = issueFilters[s.FilterIndex].value
	}

	return tea.Batch(
		m.fetchLandings(landingFilter),
		m.fetchIssues(issueFilter),
		m.fetchWorkspaces(),
		m.fetchWorkflows(),
		m.fetchRepos(),
		m.fetchNotifications(),
	)
}

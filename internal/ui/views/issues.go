package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

var _ View = (*IssuesView)(nil)

type issuesLoadedMsg struct {
	issues []jjhub.Issue
}

type issuesErrorMsg struct {
	err error
}

type issueRepoLoadedMsg struct {
	repo *jjhub.Repo
}

type issueDetailLoadedMsg struct {
	number int
	issue  *jjhub.Issue
}

type issueDetailErrorMsg struct {
	number int
	err    error
}

// IssuesView renders a JJHub issues dashboard.
type IssuesView struct {
	smithersClient *smithers.Client
	jjhubClient    *jjhub.Client
	sty            styles.Styles

	width  int
	height int

	loading bool
	err     error

	repo *jjhub.Repo

	previewOpen bool
	search      jjSearchState
	searchQuery string
	filterIndex int

	allIssues []jjhub.Issue
	issues    []jjhub.Issue

	detailCache   map[int]*jjhub.Issue
	detailLoading map[int]bool
	detailErrors  map[int]error

	tablePane   *jjTablePane
	previewPane *jjPreviewPane
	splitPane   *components.SplitPane
}

// IssueDetailView renders a full-screen issue detail drill-down.
type IssueDetailView struct {
	parent View

	jjhubClient *jjhub.Client
	repo        *jjhub.Repo
	sty         styles.Styles

	width  int
	height int

	issue   jjhub.Issue
	detail  *jjhub.Issue
	loading bool
	err     error

	previewPane *jjPreviewPane
}

type issueDetailViewLoadedMsg struct {
	issue *jjhub.Issue
}

type issueDetailViewErrorMsg struct {
	err error
}

var issueFilters = []jjFilterTab{
	{Value: "open", Label: "Open", Icon: jjhubIssueStateIcon("open")},
	{Value: "closed", Label: "Closed", Icon: jjhubIssueStateIcon("closed")},
	{Value: "all", Label: "All", Icon: "•"},
}

var issueTableColumns = []components.Column{
	{Title: "", Width: 2},
	{Title: "#", Width: 6, Align: components.AlignRight},
	{Title: "Title", Grow: true},
	{Title: "Author", Width: 14, MinWidth: 90},
	{Title: "Assignees", Width: 16, MinWidth: 108},
	{Title: "Comments", Width: 8, MinWidth: 100, Align: components.AlignRight},
	{Title: "Labels", Width: 18, MinWidth: 118},
	{Title: "Updated", Width: 10, MinWidth: 82},
}

// NewIssuesView creates a JJHub issues view.
func NewIssuesView(client *smithers.Client) *IssuesView {
	tablePane := newJJTablePane(issueTableColumns)
	previewPane := newJJPreviewPane("Select an issue")
	splitPane := components.NewSplitPane(tablePane, previewPane, components.SplitPaneOpts{
		LeftWidth:         70,
		CompactBreakpoint: 100,
	})

	return &IssuesView{
		smithersClient: client,
		jjhubClient:    jjhub.NewClient(""),
		sty:            styles.DefaultStyles(),
		loading:        true,
		previewOpen:    true,
		search:         newJJSearchInput("filter issues by title"),
		detailCache:    make(map[int]*jjhub.Issue),
		detailLoading:  make(map[int]bool),
		detailErrors:   make(map[int]error),
		tablePane:      tablePane,
		previewPane:    previewPane,
		splitPane:      splitPane,
	}
}

func (v *IssuesView) Init() tea.Cmd {
	return tea.Batch(v.loadIssuesCmd(), v.loadRepoCmd())
}

func (v *IssuesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		v.loading = false
		v.err = nil
		v.allIssues = msg.issues
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case issuesErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case issueRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case issueDetailLoadedMsg:
		delete(v.detailLoading, msg.number)
		delete(v.detailErrors, msg.number)
		v.detailCache[msg.number] = msg.issue
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case issueDetailErrorMsg:
		delete(v.detailLoading, msg.number)
		v.detailErrors[msg.number] = msg.err
		return v, v.syncPreview(false)

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		if v.search.active {
			return v.updateSearch(msg)
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			v.search.active = true
			v.search.input.SetValue(v.searchQuery)
			return v, v.search.input.Focus()
		case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
			v.previewOpen = !v.previewOpen
			if v.previewOpen {
				return v, v.syncPreview(true)
			}
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			v.filterIndex = (v.filterIndex + 1) % len(issueFilters)
			selectionChanged := v.rebuildRows()
			return v, v.syncPreview(selectionChanged)
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			v.err = nil
			v.detailCache = make(map[int]*jjhub.Issue)
			v.detailLoading = make(map[int]bool)
			v.detailErrors = make(map[int]error)
			selectionChanged := v.rebuildRows()
			return v, tea.Batch(v.Init(), v.syncPreview(selectionChanged))
		case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
			if issue := v.selectedIssue(); issue != nil {
				return v, jjOpenURLCmd(jjIssueURL(v.repo, issue.Number))
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if issue := v.selectedIssue(); issue != nil {
				detailView := NewIssueDetailView(v, v.jjhubClient, v.repo, v.sty, *issue, v.detailCache[issue.Number])
				detailView.SetSize(v.width, v.height)
				return detailView, detailView.Init()
			}
		}
	}

	previous := v.selectedIssueNumber()
	var cmd tea.Cmd
	if v.previewOpen {
		v.tablePane.SetFocused(v.splitPane.Focus() == components.FocusLeft)
		newSplitPane, splitCmd := v.splitPane.Update(msg)
		v.splitPane = newSplitPane
		cmd = splitCmd
	} else {
		v.tablePane.SetFocused(true)
		_, cmd = v.tablePane.Update(msg)
	}

	selectionChanged := previous != v.selectedIssueNumber()
	return v, tea.Batch(cmd, v.syncPreview(selectionChanged))
}

func (v *IssuesView) View() string {
	header := jjRenderHeader(
		fmt.Sprintf("JJHUB › Issues (%d)", len(v.issues)),
		v.width,
		jjMutedStyle.Render("[/] Search  [w] Preview  [Esc] Back"),
	)
	tabs := jjRenderFilterTabs(issueFilters, v.currentFilter(), v.stateCounts())

	var parts []string
	parts = append(parts, header)
	if v.search.active {
		parts = append(parts, tabs+"  "+jjSearchStyle.Render("Search:")+" "+v.search.input.View())
	} else if v.searchQuery != "" {
		parts = append(parts, tabs+"  "+jjMutedStyle.Render("filter: "+v.searchQuery))
	} else {
		parts = append(parts, tabs)
	}

	if v.loading && len(v.allIssues) == 0 {
		parts = append(parts, jjMutedStyle.Render("Loading issues…"))
		return strings.Join(parts, "\n")
	}
	if v.err != nil && len(v.allIssues) == 0 {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
		return strings.Join(parts, "\n")
	}
	if v.err != nil {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
	}

	contentHeight := max(1, v.height-len(parts)-1)
	if v.previewOpen {
		v.tablePane.SetFocused(v.splitPane.Focus() == components.FocusLeft)
		v.splitPane.SetSize(v.width, contentHeight)
		parts = append(parts, v.splitPane.View())
	} else {
		v.tablePane.SetFocused(true)
		v.tablePane.SetSize(v.width, contentHeight)
		parts = append(parts, v.tablePane.View())
	}
	return strings.Join(parts, "\n")
}

func (v *IssuesView) Name() string { return "issues" }

func (v *IssuesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	contentHeight := max(1, height-3)
	v.tablePane.SetSize(width, contentHeight)
	v.previewPane.SetSize(max(1, width/2), contentHeight)
	v.splitPane.SetSize(width, contentHeight)
}

func (v *IssuesView) ShortHelp() []key.Binding {
	if v.search.active {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}

	help := []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "filter")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "preview")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "browser")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
	if v.previewOpen {
		help = append(help, key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")))
	}
	return help
}

func (v *IssuesView) currentFilter() string {
	return issueFilters[v.filterIndex].Value
}

func (v *IssuesView) stateCounts() map[string]int {
	counts := map[string]int{
		"open":   0,
		"closed": 0,
		"all":    len(v.allIssues),
	}
	for _, issue := range v.allIssues {
		counts[issue.State]++
	}
	return counts
}

func (v *IssuesView) selectedIssue() *jjhub.Issue {
	index := v.tablePane.Cursor()
	if index < 0 || index >= len(v.issues) {
		return nil
	}
	issue := v.issues[index]
	return &issue
}

func (v *IssuesView) selectedIssueNumber() int {
	if issue := v.selectedIssue(); issue != nil {
		return issue.Number
	}
	return 0
}

func (v *IssuesView) rebuildRows() bool {
	previous := v.selectedIssueNumber()
	filter := v.currentFilter()

	filtered := make([]jjhub.Issue, 0, len(v.allIssues))
	rows := make([]components.Row, 0, len(v.allIssues))
	for _, issue := range v.allIssues {
		if filter != "all" && issue.State != filter {
			continue
		}
		if v.searchQuery != "" && !jjMatchesSearch(issue.Title, v.searchQuery) {
			continue
		}

		labels := "-"
		if cached := v.detailCache[issue.Number]; cached != nil && len(cached.Labels) > 0 {
			labelNames := make([]string, 0, len(cached.Labels))
			for _, label := range cached.Labels {
				labelNames = append(labelNames, label.Name)
			}
			labels = strings.Join(labelNames, ", ")
		} else if len(issue.Labels) > 0 {
			labelNames := make([]string, 0, len(issue.Labels))
			for _, label := range issue.Labels {
				labelNames = append(labelNames, label.Name)
			}
			labels = strings.Join(labelNames, ", ")
		}

		filtered = append(filtered, issue)
		rows = append(rows, components.Row{
			Cells: []string{
				jjhubIssueStateIcon(issue.State),
				fmt.Sprintf("#%d", issue.Number),
				issue.Title,
				issue.Author.Login,
				jjJoinAssignees(issue.Assignees),
				fmt.Sprintf("%d", issue.CommentCount),
				labels,
				jjhubRelativeTime(issue.UpdatedAt),
			},
		})
	}

	v.issues = filtered
	v.tablePane.SetRows(rows)

	targetIndex := 0
	for i, issue := range filtered {
		if issue.Number == previous {
			targetIndex = i
			break
		}
	}
	if len(filtered) > 0 {
		v.tablePane.SetCursor(targetIndex)
	}
	return previous != v.selectedIssueNumber()
}

func (v *IssuesView) syncPreview(reset bool) tea.Cmd {
	issue := v.selectedIssue()
	if issue == nil {
		v.previewPane.SetContent("", true)
		return nil
	}
	v.previewPane.SetContent(v.renderPreview(*issue), reset)
	return v.ensureIssueDetail(*issue)
}

func (v *IssuesView) renderPreview(issue jjhub.Issue) string {
	width := max(24, v.previewPane.width-4)
	detail := v.detailCache[issue.Number]
	current := issue
	if detail != nil {
		current = *detail
	}

	var body strings.Builder
	body.WriteString(jjTitleStyle.Render(current.Title))
	body.WriteString("\n")
	body.WriteString(jjBadgeStyleForState(current.State).Render(jjhubIssueStateIcon(current.State) + " " + current.State))
	body.WriteString("\n\n")
	body.WriteString(jjMetaRow("Author", "@"+current.Author.Login) + "\n")
	body.WriteString(jjMetaRow("Number", fmt.Sprintf("#%d", current.Number)) + "\n")
	body.WriteString(jjMetaRow("Assignees", jjJoinAssignees(current.Assignees)) + "\n")
	body.WriteString(jjMetaRow("Comments", fmt.Sprintf("%d", current.CommentCount)) + "\n")
	body.WriteString(jjMetaRow("Updated", jjFormatTime(current.UpdatedAt)) + "\n")

	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Labels"))
	body.WriteString("\n")
	if len(current.Labels) == 0 {
		body.WriteString(jjMutedStyle.Render("No labels."))
		body.WriteString("\n")
	} else {
		parts := make([]string, 0, len(current.Labels))
		for _, label := range current.Labels {
			parts = append(parts, jjRenderLabel(label))
		}
		body.WriteString(strings.Join(parts, " "))
		body.WriteString("\n")
	}

	if v.detailErrors[issue.Number] != nil {
		body.WriteString("\n")
		body.WriteString(jjErrorStyle.Render(v.detailErrors[issue.Number].Error()))
		body.WriteString("\n")
	}

	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Description"))
	body.WriteString("\n")
	body.WriteString(jjMarkdown(current.Body, width, &v.sty))
	return strings.TrimSpace(body.String())
}

func (v *IssuesView) ensureIssueDetail(issue jjhub.Issue) tea.Cmd {
	if v.detailCache[issue.Number] != nil || v.detailLoading[issue.Number] {
		return nil
	}
	v.detailLoading[issue.Number] = true
	return v.loadIssueDetailCmd(issue.Number)
}

func (v *IssuesView) updateSearch(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		v.search.active = false
		v.search.input.Blur()
		return v, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		v.search.active = false
		v.searchQuery = strings.TrimSpace(v.search.input.Value())
		v.search.input.Blur()
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)
	default:
		var cmd tea.Cmd
		v.search.input, cmd = v.search.input.Update(msg)
		return v, cmd
	}
}

func (v *IssuesView) loadIssuesCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		issues, err := client.ListIssues("all", jjDefaultListLimit)
		if err != nil {
			return issuesErrorMsg{err: err}
		}
		return issuesLoadedMsg{issues: issues}
	}
}

func (v *IssuesView) loadRepoCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		if err != nil {
			return nil
		}
		return issueRepoLoadedMsg{repo: repo}
	}
}

func (v *IssuesView) loadIssueDetailCmd(number int) tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		issue, err := client.ViewIssue(number)
		if err != nil {
			return issueDetailErrorMsg{number: number, err: err}
		}
		return issueDetailLoadedMsg{number: number, issue: issue}
	}
}

// NewIssueDetailView creates a full-screen issue detail drill-down view.
func NewIssueDetailView(
	parent View,
	client *jjhub.Client,
	repo *jjhub.Repo,
	sty styles.Styles,
	issue jjhub.Issue,
	detail *jjhub.Issue,
) *IssueDetailView {
	previewPane := newJJPreviewPane("Loading issue detail…")
	return &IssueDetailView{
		parent:      parent,
		jjhubClient: client,
		repo:        repo,
		sty:         sty,
		issue:       issue,
		detail:      detail,
		loading:     detail == nil,
		previewPane: previewPane,
	}
}

func (v *IssueDetailView) Init() tea.Cmd {
	v.syncContent(true)
	if v.detail != nil {
		return nil
	}
	client := v.jjhubClient
	number := v.issue.Number
	return func() tea.Msg {
		issue, err := client.ViewIssue(number)
		if err != nil {
			return issueDetailViewErrorMsg{err: err}
		}
		return issueDetailViewLoadedMsg{issue: issue}
	}
}

func (v *IssueDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issueDetailViewLoadedMsg:
		v.detail = msg.issue
		v.loading = false
		v.err = nil
		if parent, ok := v.parent.(*IssuesView); ok && msg.issue != nil {
			parent.detailCache[v.issue.Number] = msg.issue
			parent.rebuildRows()
			parent.syncPreview(false)
		}
		v.syncContent(true)
		return v, nil

	case issueDetailViewErrorMsg:
		v.loading = false
		v.err = msg.err
		v.syncContent(false)
		return v, nil

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			v.parent.SetSize(v.width, v.height)
			return v.parent, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
			return v, jjOpenURLCmd(jjIssueURL(v.repo, v.issue.Number))
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			v.err = nil
			return v, v.Init()
		}
	}

	_, cmd := v.previewPane.Update(msg)
	return v, cmd
}

func (v *IssueDetailView) View() string {
	header := jjRenderHeader(
		fmt.Sprintf("JJHUB › Issues › #%d", v.issue.Number),
		v.width,
		jjMutedStyle.Render("[o] Browser  [Esc] Back"),
	)
	parts := []string{header}
	if v.err != nil {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
	}
	if v.loading && v.detail == nil {
		parts = append(parts, jjMutedStyle.Render("Loading issue detail…"))
	}
	parts = append(parts, v.previewPane.View())
	return strings.Join(parts, "\n")
}

func (v *IssueDetailView) Name() string { return "issue-detail" }

func (v *IssueDetailView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.previewPane.SetSize(width, max(1, height-2))
	v.syncContent(false)
}

func (v *IssueDetailView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "scroll")),
		key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "browser")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (v *IssueDetailView) syncContent(reset bool) {
	width := max(24, v.previewPane.width-4)
	current := v.issue
	if v.detail != nil {
		current = *v.detail
	}

	var body strings.Builder
	body.WriteString(jjTitleStyle.Render(current.Title))
	body.WriteString("\n")
	body.WriteString(jjBadgeStyleForState(current.State).Render(jjhubIssueStateIcon(current.State) + " " + current.State))
	body.WriteString("\n\n")
	body.WriteString(jjMetaRow("Author", "@"+current.Author.Login) + "\n")
	body.WriteString(jjMetaRow("Number", fmt.Sprintf("#%d", current.Number)) + "\n")
	body.WriteString(jjMetaRow("Assignees", jjJoinAssignees(current.Assignees)) + "\n")
	body.WriteString(jjMetaRow("Comments", fmt.Sprintf("%d", current.CommentCount)) + "\n")
	body.WriteString(jjMetaRow("Created", jjFormatTime(current.CreatedAt)) + "\n")
	body.WriteString(jjMetaRow("Updated", jjFormatTime(current.UpdatedAt)) + "\n")
	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Labels"))
	body.WriteString("\n")
	if len(current.Labels) == 0 {
		body.WriteString(jjMutedStyle.Render("No labels."))
	} else {
		labels := make([]string, 0, len(current.Labels))
		for _, label := range current.Labels {
			labels = append(labels, jjRenderLabel(label))
		}
		body.WriteString(strings.Join(labels, " "))
	}
	body.WriteString("\n\n")
	body.WriteString(jjSectionStyle.Render("Description"))
	body.WriteString("\n")
	body.WriteString(jjMarkdown(current.Body, width, &v.sty))
	if v.err != nil {
		body.WriteString("\n\n")
		body.WriteString(jjErrorStyle.Render(v.err.Error()))
	}
	v.previewPane.SetContent(strings.TrimSpace(body.String()), reset)
}

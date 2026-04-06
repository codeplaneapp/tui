package views

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*IssuesView)(nil)

type issuesLoadedMsg struct {
	issues []jjhub.Issue
}

type issuesErrorMsg struct {
	err error
}

type issuesRepoLoadedMsg struct {
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

var issueStateCycle = []string{"open", "closed", "all"}

// IssuesView displays JJHub issues with a list/detail layout.
type IssuesView struct {
	client *jjhub.Client
	repo   *jjhub.Repo

	issues []jjhub.Issue

	stateFilter  string
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error

	detailCache   map[int]*jjhub.Issue
	detailErr     map[int]error
	detailLoading map[int]bool
}

// NewIssuesView creates a JJHub issues view.
func NewIssuesView(_ *smithers.Client) *IssuesView {
	var client *jjhub.Client
	if jjhubAvailable() {
		client = jjhub.NewClient("")
	}
	v := &IssuesView{
		client:        client,
		stateFilter:   "open",
		loading:       client != nil,
		detailCache:   make(map[int]*jjhub.Issue),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

// Init loads issues and repository metadata.
func (v *IssuesView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadIssuesCmd(), v.loadRepoCmd())
}

func (v *IssuesView) loadIssuesCmd() tea.Cmd {
	client := v.client
	state := v.stateFilter
	return func() tea.Msg {
		issues, err := client.ListIssues(state, 100)
		if err != nil {
			return issuesErrorMsg{err: err}
		}
		return issuesLoadedMsg{issues: issues}
	}
}

func (v *IssuesView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		if err != nil {
			return nil
		}
		return issuesRepoLoadedMsg{repo: repo}
	}
}

func (v *IssuesView) loadSelectedDetailCmd() tea.Cmd {
	issue := v.selectedIssue()
	if issue == nil || v.client == nil {
		return nil
	}
	if v.detailCache[issue.Number] != nil || v.detailLoading[issue.Number] {
		return nil
	}
	delete(v.detailErr, issue.Number)
	v.detailLoading[issue.Number] = true

	number := issue.Number
	client := v.client
	return func() tea.Msg {
		loaded, err := client.ViewIssue(number)
		if err != nil {
			return issueDetailErrorMsg{number: number, err: err}
		}
		return issueDetailLoadedMsg{number: number, issue: loaded}
	}
}

func (v *IssuesView) selectedIssue() *jjhub.Issue {
	if len(v.issues) == 0 || v.cursor < 0 || v.cursor >= len(v.issues) {
		return nil
	}
	issue := v.issues[v.cursor]
	return &issue
}

func (v *IssuesView) pageSize() int {
	const (
		headerLines   = 6
		linesPerIssue = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / linesPerIssue
	if size < 1 {
		return 1
	}
	return size
}

func (v *IssuesView) clampCursor() {
	if len(v.issues) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(v.issues) {
		v.cursor = len(v.issues) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}

	pageSize := v.pageSize()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	if v.cursor >= v.scrollOffset+pageSize {
		v.scrollOffset = v.cursor - pageSize + 1
	}
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
}

func (v *IssuesView) cycleStateFilter() {
	for i, state := range issueStateCycle {
		if state == v.stateFilter {
			v.stateFilter = issueStateCycle[(i+1)%len(issueStateCycle)]
			v.cursor = 0
			v.scrollOffset = 0
			return
		}
	}
	v.stateFilter = issueStateCycle[0]
	v.cursor = 0
	v.scrollOffset = 0
}

// Update handles messages for the issues view.
func (v *IssuesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		v.issues = msg.issues
		v.loading = false
		v.err = nil
		v.clampCursor()
		return v, v.loadSelectedDetailCmd()

	case issuesErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case issuesRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case issueDetailLoadedMsg:
		v.detailLoading[msg.number] = false
		v.detailCache[msg.number] = msg.issue
		delete(v.detailErr, msg.number)
		return v, nil

	case issueDetailErrorMsg:
		v.detailLoading[msg.number] = false
		v.detailErr[msg.number] = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.clampCursor()
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
				v.clampCursor()
				return v, v.loadSelectedDetailCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.issues)-1 {
				v.cursor++
				v.clampCursor()
				return v, v.loadSelectedDetailCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if v.client == nil {
				return v, nil
			}
			v.loading = true
			v.err = nil
			return v, tea.Batch(v.loadIssuesCmd(), v.loadRepoCmd())

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			if v.client == nil {
				return v, nil
			}
			v.cycleStateFilter()
			v.loading = true
			v.err = nil
			return v, v.loadIssuesCmd()
		}
	}

	return v, nil
}

// View renders the issues view.
func (v *IssuesView) View() string {
	var b strings.Builder

	b.WriteString(jjhubHeader("JJHUB › Issues", v.width, jjhubJoinNonEmpty("  ",
		lipgloss.NewStyle().Faint(true).Render("["+v.stateFilter+"]"),
		jjhubRepoLabel(v.repo),
		lipgloss.NewStyle().Faint(true).Render("[Esc] Back"),
	)))
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading issues...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.issues) == 0 {
		b.WriteString("  No issues found.\n")
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderIssueList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderIssueDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderIssueList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderIssueDetail(v.width))
	return b.String()
}

func (v *IssuesView) renderIssueList(width int) string {
	var b strings.Builder
	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(v.issues)-1))
	end := min(len(v.issues), start+pageSize)

	for i := start; i < end; i++ {
		issue := v.issues[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		b.WriteString(cursor + titleStyle.Render(truncateStr(fmt.Sprintf("#%d %s", issue.Number, issue.Title), max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			styleIssueState(issue.State),
			jjhubAtUser(issue.Author.Login),
			fmt.Sprintf("%d comment%s", issue.CommentCount, pluralS(issue.CommentCount)),
			jjhubFormatRelativeTime(issue.UpdatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(v.issues) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(
			fmt.Sprintf("… %d more issue%s", len(v.issues)-end, pluralS(len(v.issues)-end)),
		))
	}

	return b.String()
}

func (v *IssuesView) renderIssueDetail(width int) string {
	issue := v.selectedIssue()
	if issue == nil {
		return "No issue selected."
	}

	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d %s", issue.Number, issue.Title)))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("State", issue.State))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Author", jjhubAtUser(issue.Author.Login)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(issue.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(issue.UpdatedAt)))

	if len(issue.Assignees) > 0 {
		names := make([]string, 0, len(issue.Assignees))
		for _, assignee := range issue.Assignees {
			names = append(names, jjhubAtUser(assignee.Login))
		}
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Assignees", strings.Join(names, ", ")))
	}

	if len(issue.Labels) > 0 {
		labels := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			labels = append(labels, label.Name)
		}
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Labels", strings.Join(labels, ", ")))
	}

	body := issue.Body
	if detail := v.detailCache[issue.Number]; detail != nil {
		body = detail.Body
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Body"))
	b.WriteString("\n")
	if strings.TrimSpace(body) == "" {
		b.WriteString(jjhubMutedStyle.Render("No description provided."))
	} else {
		clipped, truncated := jjhubClipLines(wrapText(body, max(20, width)), max(8, v.height-10))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	if v.detailLoading[issue.Number] {
		b.WriteString("\n\n")
		b.WriteString(jjhubMutedStyle.Render("Loading issue details..."))
	}
	if v.detailErr[issue.Number] != nil {
		b.WriteString("\n\n")
		b.WriteString(jjhubErrorStyle.Render("Detail error: " + v.detailErr[issue.Number].Error()))
	}

	return b.String()
}

// Name returns the view name.
func (v *IssuesView) Name() string { return "issues" }

// SetSize stores the terminal size.
func (v *IssuesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

// ShortHelp returns the contextual help bindings.
func (v *IssuesView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "state")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func styleIssueState(state string) string {
	style := lipgloss.NewStyle()
	switch strings.ToLower(state) {
	case "open":
		style = style.Foreground(lipgloss.Color("111")).Bold(true)
	case "closed":
		style = style.Foreground(lipgloss.Color("245"))
	default:
		style = style.Faint(true)
	}
	return style.Render(state)
}

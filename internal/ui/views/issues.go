package views

import (
	"context"
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

type issueManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	ListIssues(ctx context.Context, state string, limit int) ([]jjhub.Issue, error)
	ViewIssue(ctx context.Context, number int) (*jjhub.Issue, error)
	CreateIssue(ctx context.Context, title, body string) (*jjhub.Issue, error)
	CloseIssue(ctx context.Context, number int, comment string) (*jjhub.Issue, error)
}

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

type issueActionDoneMsg struct {
	message           string
	issue             *jjhub.Issue
	targetStateFilter string
}

type issueActionErrorMsg struct {
	err error
}

type issuePromptKind uint8

const (
	issuePromptCreateTitle issuePromptKind = iota
	issuePromptCreateBody
	issuePromptCloseComment
)

type issuePromptState struct {
	active bool
	kind   issuePromptKind
	input  textinput.Model
	title  string
	err    error
}

var issueStateCycle = []string{"open", "closed", "all"}

// IssuesView displays JJHub issues with a list/detail layout.
type IssuesView struct {
	client issueManager
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

	actionMsg          string
	pendingSelectIssue int
	prompt             issuePromptState
}

// NewIssuesView creates a JJHub issues view.
func NewIssuesView(_ *smithers.Client) *IssuesView {
	var client issueManager
	if jjhubAvailable() {
		client = jjhubIssueManager{client: jjhub.NewClient("")}
	}
	return newIssuesViewWithClient(client)
}

func newIssuesViewWithClient(client issueManager) *IssuesView {
	input := textinput.New()
	input.Placeholder = "Issue title"
	input.SetVirtualCursor(true)

	v := &IssuesView{
		client:        client,
		stateFilter:   "open",
		loading:       client != nil,
		detailCache:   make(map[int]*jjhub.Issue),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
		prompt: issuePromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

type jjhubIssueManager struct {
	client *jjhub.Client
}

func (m jjhubIssueManager) GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error) {
	return m.client.GetCurrentRepo(ctx)
}

func (m jjhubIssueManager) ListIssues(ctx context.Context, state string, limit int) ([]jjhub.Issue, error) {
	return m.client.ListIssues(ctx, state, limit)
}

func (m jjhubIssueManager) ViewIssue(ctx context.Context, number int) (*jjhub.Issue, error) {
	return m.client.ViewIssue(ctx, number)
}

func (m jjhubIssueManager) CreateIssue(ctx context.Context, title, body string) (*jjhub.Issue, error) {
	return m.client.CreateIssue(ctx, title, body)
}

func (m jjhubIssueManager) CloseIssue(ctx context.Context, number int, comment string) (*jjhub.Issue, error) {
	return m.client.CloseIssue(ctx, number, comment)
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
		issues, err := client.ListIssues(context.Background(), state, 100)
		if err != nil {
			return issuesErrorMsg{err: err}
		}
		return issuesLoadedMsg{issues: issues}
	}
}

func (v *IssuesView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
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
		loaded, err := client.ViewIssue(context.Background(), number)
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

func (v *IssuesView) refreshCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = true
	v.err = nil
	return tea.Batch(v.loadIssuesCmd(), v.loadRepoCmd())
}

func (v *IssuesView) selectIssueByNumber(number int) {
	if number <= 0 {
		return
	}
	for i, issue := range v.issues {
		if issue.Number == number {
			v.cursor = i
			v.clampCursor()
			return
		}
	}
}

func (v *IssuesView) createIssueCmd(title, body string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		issue, err := client.CreateIssue(context.Background(), title, body)
		if err != nil {
			return issueActionErrorMsg{err: err}
		}
		return issueActionDoneMsg{
			message:           fmt.Sprintf("Created issue #%d", issue.Number),
			issue:             issue,
			targetStateFilter: "open",
		}
	}
}

func (v *IssuesView) closeIssueCmd(comment string) tea.Cmd {
	client := v.client
	issue := v.selectedIssue()
	if issue == nil {
		return nil
	}

	number := issue.Number
	return func() tea.Msg {
		closed, err := client.CloseIssue(context.Background(), number, comment)
		if err != nil {
			return issueActionErrorMsg{err: err}
		}
		return issueActionDoneMsg{
			message: fmt.Sprintf("Closed issue #%d", number),
			issue:   closed,
		}
	}
}

func (v *IssuesView) openPrompt(kind issuePromptKind, placeholder string) tea.Cmd {
	v.prompt.active = true
	v.prompt.kind = kind
	v.prompt.err = nil
	v.prompt.title = ""
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = placeholder
	return v.prompt.input.Focus()
}

func (v *IssuesView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.title = ""
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *IssuesView) advanceCreatePrompt() tea.Cmd {
	title := strings.TrimSpace(v.prompt.input.Value())
	if title == "" {
		v.prompt.err = errors.New("Title must not be empty")
		return nil
	}

	v.prompt.kind = issuePromptCreateBody
	v.prompt.err = nil
	v.prompt.title = title
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = "Optional issue body"
	return v.prompt.input.Focus()
}

func (v *IssuesView) submitPrompt() tea.Cmd {
	switch v.prompt.kind {
	case issuePromptCreateTitle:
		return v.advanceCreatePrompt()
	case issuePromptCreateBody:
		return v.createIssueCmd(v.prompt.title, strings.TrimSpace(v.prompt.input.Value()))
	case issuePromptCloseComment:
		return v.closeIssueCmd(strings.TrimSpace(v.prompt.input.Value()))
	default:
		return nil
	}
}

func (v *IssuesView) renderPrompt(width int) string {
	boxWidth := max(32, min(max(32, width-4), 80))

	title := "Create issue"
	description := "Enter a title for the new JJHub issue."
	if v.prompt.kind == issuePromptCreateBody {
		description = "Add an optional body, then press Enter again to submit."
	} else if v.prompt.kind == issuePromptCloseComment {
		title = "Close issue"
		description = "Optional closing comment."
	}

	var body strings.Builder
	body.WriteString(jjhubSectionStyle.Render(title))
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render(description))

	if v.prompt.kind == issuePromptCreateBody && strings.TrimSpace(v.prompt.title) != "" {
		body.WriteString("\n\n")
		body.WriteString(jjhubMetaRow("Title", truncateStr(v.prompt.title, max(20, boxWidth-16))))
	}

	body.WriteString("\n\n")
	body.WriteString(v.prompt.input.View())
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render("[Enter] submit  [Esc] cancel"))
	if v.prompt.err != nil {
		body.WriteString("\n")
		body.WriteString(jjhubErrorStyle.Render(v.prompt.err.Error()))
	}

	return lipgloss.NewStyle().
		Width(boxWidth).
		Padding(0, 1).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(body.String())
}

// Update handles messages for the issues view.
func (v *IssuesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		v.issues = msg.issues
		v.loading = false
		v.err = nil
		if v.pendingSelectIssue > 0 {
			v.selectIssueByNumber(v.pendingSelectIssue)
			v.pendingSelectIssue = 0
		}
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

	case issueActionDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		if msg.targetStateFilter != "" {
			v.stateFilter = msg.targetStateFilter
		}
		if msg.issue != nil {
			v.detailCache[msg.issue.Number] = msg.issue
			delete(v.detailErr, msg.issue.Number)
			v.pendingSelectIssue = msg.issue.Number
		}
		return v, v.refreshCmd()

	case issueActionErrorMsg:
		if v.prompt.active {
			v.prompt.err = msg.err
			return v, nil
		}
		v.actionMsg = msg.err.Error()
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.clampCursor()
		return v, nil

	case tea.KeyPressMsg:
		if v.prompt.active {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				v.closePrompt()
				return v, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				return v, v.submitPrompt()
			default:
				var cmd tea.Cmd
				v.prompt.input, cmd = v.prompt.input.Update(msg)
				return v, cmd
			}
		}

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
			v.actionMsg = ""
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			if v.client == nil {
				return v, nil
			}
			v.actionMsg = ""
			v.cycleStateFilter()
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			if v.client == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(issuePromptCreateTitle, "Issue title")

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			if v.client == nil {
				return v, nil
			}
			issue := v.selectedIssue()
			if issue == nil || strings.EqualFold(issue.State, "closed") {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(issuePromptCloseComment, "Optional closing comment")
		}
	}

	return v, nil
}

// View renders the issues view.
func (v *IssuesView) View() string {
	var b strings.Builder

	rightSide := jjhubJoinNonEmpty("  ",
		"["+v.stateFilter+"]",
		jjhubRepoLabel(v.repo),
		"[Esc] Back",
	)
	b.WriteString(ViewHeader(packageCom.Styles, "SMITHERS", "Issues", v.width, rightSide))
	b.WriteString("\n\n")

	if v.prompt.active {
		b.WriteString(v.renderPrompt(v.width))
		b.WriteString("\n\n")
	} else if strings.TrimSpace(v.actionMsg) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  "+v.actionMsg) + "\n\n")
	}

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
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render("  Press c to create a new issue."))
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
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close")),
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

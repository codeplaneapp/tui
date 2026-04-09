package views

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	ghrepo "github.com/charmbracelet/crush/internal/github"
	"github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*PullRequestsView)(nil)

type pullRequestManager interface {
	GetCurrentRepo(ctx context.Context) (*ghrepo.Repo, error)
	ListPullRequests(ctx context.Context, state string, limit int) ([]ghrepo.PullRequest, error)
	CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (*ghrepo.PullRequest, error)
	CommentPullRequest(ctx context.Context, number int, body string) error
	OriginRepository(ctx context.Context) (string, error)
}

type pullRequestPromptKind uint8

const (
	pullRequestPromptCreateTitle pullRequestPromptKind = iota
	pullRequestPromptCreateBody
	pullRequestPromptCreateBase
	pullRequestPromptComment
)

type pullRequestPromptState struct {
	active bool
	kind   pullRequestPromptKind
	input  textinput.Model
	title  string
	body   string
	base   string
	err    error
}

type (
	pullRequestsLoadedMsg struct {
		pulls []ghrepo.PullRequest
	}
	pullRequestsErrorMsg struct {
		err error
	}
	pullRequestsRepoLoadedMsg struct {
		repo *ghrepo.Repo
	}
	pullRequestActionDoneMsg struct {
		message           string
		pull              *ghrepo.PullRequest
		targetStateFilter string
	}
	pullRequestActionErrorMsg struct {
		err error
	}
)

var pullRequestStateCycle = []string{"open", "closed", "merged", "all"}

type PullRequestsView struct {
	client pullRequestManager
	repo   *ghrepo.Repo

	pulls []ghrepo.PullRequest

	stateFilter  string
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error
	actionMsg    string
	prompt       pullRequestPromptState
}

func NewPullRequestsView(_ *smithers.Client) *PullRequestsView {
	var client pullRequestManager
	if ghAvailable() {
		client = githubPullRequestManager{client: ghrepo.NewClient("")}
	}
	return newPullRequestsViewWithClient(client)
}

type githubPullRequestManager struct {
	client *ghrepo.Client
}

func (m githubPullRequestManager) GetCurrentRepo(ctx context.Context) (*ghrepo.Repo, error) {
	return m.client.GetCurrentRepo(ctx)
}

func (m githubPullRequestManager) ListPullRequests(ctx context.Context, state string, limit int) ([]ghrepo.PullRequest, error) {
	return m.client.ListPullRequests(ctx, state, limit)
}

func (m githubPullRequestManager) CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (*ghrepo.PullRequest, error) {
	return m.client.CreatePullRequest(ctx, title, body, head, base, draft)
}

func (m githubPullRequestManager) CommentPullRequest(ctx context.Context, number int, body string) error {
	return m.client.CommentPullRequest(ctx, number, body)
}

func (m githubPullRequestManager) OriginRepository(ctx context.Context) (string, error) {
	return ghrepo.OriginRepository(ctx)
}

func newPullRequestsViewWithClient(client pullRequestManager) *PullRequestsView {
	input := textinput.New()
	input.Placeholder = "Pull request title"
	input.SetVirtualCursor(true)

	v := &PullRequestsView{
		client:      client,
		stateFilter: "open",
		loading:     client != nil,
		prompt: pullRequestPromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("gh CLI not found on PATH")
	}
	return v
}

func (v *PullRequestsView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadPullRequestsCmd(), v.loadRepoCmd())
}

func (v *PullRequestsView) loadPullRequestsCmd() tea.Cmd {
	client := v.client
	state := v.stateFilter
	return func() tea.Msg {
		pulls, err := client.ListPullRequests(context.Background(), state, 100)
		if err != nil {
			return pullRequestsErrorMsg{err: err}
		}
		return pullRequestsLoadedMsg{pulls: pulls}
	}
}

func (v *PullRequestsView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
		if err != nil {
			return nil
		}
		return pullRequestsRepoLoadedMsg{repo: repo}
	}
}

func (v *PullRequestsView) refreshCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = true
	v.err = nil
	return tea.Batch(v.loadPullRequestsCmd(), v.loadRepoCmd())
}

func (v *PullRequestsView) selectedPullRequest() *ghrepo.PullRequest {
	if len(v.pulls) == 0 || v.cursor < 0 || v.cursor >= len(v.pulls) {
		return nil
	}
	pr := v.pulls[v.cursor]
	return &pr
}

func (v *PullRequestsView) pageSize() int {
	const (
		headerLines = 6
		itemLines   = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / itemLines
	if size < 1 {
		return 1
	}
	return size
}

func (v *PullRequestsView) clampCursor() {
	if len(v.pulls) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(v.pulls) {
		v.cursor = len(v.pulls) - 1
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

func (v *PullRequestsView) cycleStateFilter() {
	for i, state := range pullRequestStateCycle {
		if state == v.stateFilter {
			v.stateFilter = pullRequestStateCycle[(i+1)%len(pullRequestStateCycle)]
			v.cursor = 0
			v.scrollOffset = 0
			return
		}
	}
	v.stateFilter = pullRequestStateCycle[0]
	v.cursor = 0
	v.scrollOffset = 0
}

func (v *PullRequestsView) openPrompt(kind pullRequestPromptKind, placeholder string) tea.Cmd {
	v.prompt.active = true
	v.prompt.kind = kind
	v.prompt.err = nil
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = placeholder
	if kind == pullRequestPromptCreateTitle {
		v.prompt.title = ""
		v.prompt.body = ""
		v.prompt.base = ""
	}
	return v.prompt.input.Focus()
}

func (v *PullRequestsView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.title = ""
	v.prompt.body = ""
	v.prompt.base = ""
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *PullRequestsView) createPullRequestCmd(title, body, base string) tea.Cmd {
	return func() tea.Msg {
		head, err := ghrepo.CurrentBranch(context.Background())
		if err != nil {
			return pullRequestActionErrorMsg{err: err}
		}
		if strings.TrimSpace(base) == "" {
			base, err = ghrepo.DefaultBaseBranch(context.Background())
			if err != nil {
				return pullRequestActionErrorMsg{err: err}
			}
		}
		if err := ghrepo.PushBranch(context.Background(), "origin", head); err != nil {
			return pullRequestActionErrorMsg{err: err}
		}
		repo, err := v.client.OriginRepository(context.Background())
		if err != nil {
			return pullRequestActionErrorMsg{err: err}
		}
		pr, err := ghrepo.NewClient(repo).CreatePullRequest(context.Background(), title, body, head, base, true)
		if err != nil {
			return pullRequestActionErrorMsg{err: err}
		}
		return pullRequestActionDoneMsg{
			message:           fmt.Sprintf("Created draft PR #%d", pr.Number),
			pull:              pr,
			targetStateFilter: "open",
		}
	}
}

func (v *PullRequestsView) commentPullRequestCmd(body string) tea.Cmd {
	client := v.client
	pr := v.selectedPullRequest()
	if pr == nil {
		return nil
	}
	number := pr.Number
	return func() tea.Msg {
		if err := client.CommentPullRequest(context.Background(), number, body); err != nil {
			return pullRequestActionErrorMsg{err: err}
		}
		return pullRequestActionDoneMsg{message: fmt.Sprintf("Commented on PR #%d", number)}
	}
}

func (v *PullRequestsView) submitPrompt() tea.Cmd {
	switch v.prompt.kind {
	case pullRequestPromptCreateTitle:
		title := strings.TrimSpace(v.prompt.input.Value())
		if title == "" {
			v.prompt.err = errors.New("Title must not be empty")
			return nil
		}
		v.prompt.title = title
		v.prompt.kind = pullRequestPromptCreateBody
		v.prompt.err = nil
		v.prompt.input.Reset()
		v.prompt.input.Placeholder = "Optional PR summary"
		return v.prompt.input.Focus()
	case pullRequestPromptCreateBody:
		v.prompt.body = strings.TrimSpace(v.prompt.input.Value())
		v.prompt.kind = pullRequestPromptCreateBase
		v.prompt.err = nil
		v.prompt.input.Reset()
		v.prompt.input.Placeholder = "Base branch (default: origin HEAD)"
		return v.prompt.input.Focus()
	case pullRequestPromptCreateBase:
		return v.createPullRequestCmd(v.prompt.title, v.prompt.body, strings.TrimSpace(v.prompt.input.Value()))
	case pullRequestPromptComment:
		return v.commentPullRequestCmd(strings.TrimSpace(v.prompt.input.Value()))
	default:
		return nil
	}
}

func (v *PullRequestsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case pullRequestsLoadedMsg:
		v.pulls = msg.pulls
		v.loading = false
		v.err = nil
		v.clampCursor()
		return v, nil
	case pullRequestsErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil
	case pullRequestsRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil
	case pullRequestActionDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		if msg.targetStateFilter != "" {
			v.stateFilter = msg.targetStateFilter
		}
		return v, v.refreshCmd()
	case pullRequestActionErrorMsg:
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
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.pulls)-1 {
				v.cursor++
				v.clampCursor()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.actionMsg = ""
			return v, v.refreshCmd()
		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			v.actionMsg = ""
			v.cycleStateFilter()
			return v, v.refreshCmd()
		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			v.actionMsg = ""
			return v, v.openPrompt(pullRequestPromptCreateTitle, "Pull request title")
		case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
			if v.selectedPullRequest() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(pullRequestPromptComment, "Comment body")
		}
	}
	return v, nil
}

func (v *PullRequestsView) View() string {
	var b strings.Builder
	right := jjhubJoinNonEmpty("  ", "["+v.stateFilter+"]", githubRepoLabel(v.repo), "[Esc] Back")
	b.WriteString(ViewHeader(packageCom.Styles, "CODEPLANE", "Pull Requests", v.width, right))
	b.WriteString("\n\n")
	if v.prompt.active {
		b.WriteString(v.renderPrompt(v.width))
		b.WriteString("\n\n")
	} else if strings.TrimSpace(v.actionMsg) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  "+v.actionMsg) + "\n\n")
	}
	if v.loading {
		b.WriteString("  Loading pull requests...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.pulls) == 0 {
		b.WriteString("  No pull requests found.\n\n")
		b.WriteString(jjhubMutedStyle.Render("  Press c to create a draft pull request."))
		return b.String()
	}
	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderPullRequestList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderPullRequestDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}
	b.WriteString(v.renderPullRequestList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderPullRequestDetail(v.width))
	return b.String()
}

func (v *PullRequestsView) renderPrompt(width int) string {
	boxWidth := max(36, min(max(36, width-4), 84))
	title := "Create draft pull request"
	description := "Enter a title for the pull request."
	switch v.prompt.kind {
	case pullRequestPromptCreateBody:
		description = "Add an optional summary for the pull request."
	case pullRequestPromptCreateBase:
		description = "Pick a base branch. Leave blank to use origin HEAD."
	case pullRequestPromptComment:
		title = "Comment on pull request"
		description = "Leave a comment on the selected pull request."
	}
	var body strings.Builder
	body.WriteString(jjhubSectionStyle.Render(title))
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render(description))
	if strings.TrimSpace(v.prompt.title) != "" && v.prompt.kind != pullRequestPromptCreateTitle {
		body.WriteString("\n\n")
		body.WriteString(jjhubMetaRow("Title", truncateStr(v.prompt.title, max(20, boxWidth-16))))
	}
	if strings.TrimSpace(v.prompt.body) != "" && v.prompt.kind == pullRequestPromptCreateBase {
		body.WriteString("\n")
		body.WriteString(jjhubMetaRow("Body", truncateStr(v.prompt.body, max(20, boxWidth-16))))
	}
	body.WriteString("\n\n")
	body.WriteString(v.prompt.input.View())
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render("[Enter] submit  [Esc] cancel"))
	if v.prompt.err != nil {
		body.WriteString("\n")
		body.WriteString(jjhubErrorStyle.Render(v.prompt.err.Error()))
	}
	return lipgloss.NewStyle().Width(boxWidth).Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Render(body.String())
}

func (v *PullRequestsView) renderPullRequestList(width int) string {
	var b strings.Builder
	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(v.pulls)-1))
	end := min(len(v.pulls), start+pageSize)
	for i := start; i < end; i++ {
		pr := v.pulls[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}
		b.WriteString(cursor + titleStyle.Render(truncateStr(fmt.Sprintf("#%d %s", pr.Number, pr.Title), max(8, width-2))))
		b.WriteString("\n")
		meta := jjhubJoinNonEmpty(" · ", stylePullRequestState(pr.State, pr.IsDraft), "@"+pr.Author.Login, pr.HeadRefName+" → "+pr.BaseRefName, jjRelativeTime(pr.UpdatedAt))
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	if end < len(v.pulls) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(fmt.Sprintf("… %d more pull request%s", len(v.pulls)-end, pluralS(len(v.pulls)-end))))
	}
	return b.String()
}

func (v *PullRequestsView) renderPullRequestDetail(width int) string {
	pr := v.selectedPullRequest()
	if pr == nil {
		return "No pull request selected."
	}
	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d %s", pr.Number, pr.Title)))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("State", stylePullRequestState(pr.State, pr.IsDraft)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Author", "@"+pr.Author.Login))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Branch", pr.HeadRefName+" → "+pr.BaseRefName))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Changed files", fmt.Sprintf("%d", pr.ChangedFiles)))
	b.WriteString("\n")
	if strings.TrimSpace(pr.MergeStateStatus) != "" {
		b.WriteString(jjhubMetaRow("Merge state", pr.MergeStateStatus))
		b.WriteString("\n")
	}
	if strings.TrimSpace(pr.ReviewDecision) != "" {
		b.WriteString(jjhubMetaRow("Review", pr.ReviewDecision))
		b.WriteString("\n")
	}
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(pr.UpdatedAt)))
	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Body"))
	b.WriteString("\n")
	if strings.TrimSpace(pr.Body) == "" {
		b.WriteString(jjhubMutedStyle.Render("No description provided."))
	} else {
		b.WriteString(wrapText(pr.Body, max(20, width)))
	}
	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Actions"))
	b.WriteString("\n")
	b.WriteString(wrapText("[c] create draft PR  [m] comment  [s] state  [r] refresh", max(20, width)))
	return b.String()
}

func (v *PullRequestsView) Name() string { return "pulls" }

func (v *PullRequestsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

func (v *PullRequestsView) ShortHelp() []key.Binding {
	if v.prompt.active {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create PR")),
		key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "comment")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "state")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func githubRepoLabel(repo *ghrepo.Repo) string {
	if repo == nil || strings.TrimSpace(repo.NameWithOwner) == "" {
		return "GitHub"
	}
	return repo.NameWithOwner
}

func stylePullRequestState(state string, draft bool) string {
	style := lipgloss.NewStyle()
	label := state
	if draft && strings.EqualFold(state, "open") {
		label = "draft"
		style = style.Foreground(lipgloss.Color("214")).Bold(true)
		return style.Render(label)
	}
	switch strings.ToLower(state) {
	case "open":
		style = style.Foreground(lipgloss.Color("111")).Bold(true)
	case "merged":
		style = style.Foreground(lipgloss.Color("42")).Bold(true)
	case "closed":
		style = style.Foreground(lipgloss.Color("245"))
	default:
		style = style.Faint(true)
	}
	return style.Render(strings.ToLower(label))
}

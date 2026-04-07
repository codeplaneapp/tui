package views

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"go.opentelemetry.io/otel/attribute"
)

var _ View = (*SearchView)(nil)

type searchManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	SearchRepositories(ctx context.Context, query string, limit int) (*jjhub.RepositorySearchPage, error)
	SearchIssues(ctx context.Context, query, state string, limit int) (*jjhub.IssueSearchPage, error)
	SearchCode(ctx context.Context, query string, limit int) (*jjhub.CodeSearchPage, error)
}

type searchTab uint8

const (
	searchTabRepos searchTab = iota
	searchTabIssues
	searchTabCode
)

type (
	searchRepoResultsMsg struct {
		results *jjhub.RepositorySearchPage
	}
	searchIssueResultsMsg struct {
		results *jjhub.IssueSearchPage
	}
	searchCodeResultsMsg struct {
		results *jjhub.CodeSearchPage
	}
	searchErrorMsg struct {
		err error
	}
	searchRepoLoadedMsg struct {
		repo *jjhub.Repo
	}
)

type SearchView struct {
	client searchManager
	repo   *jjhub.Repo

	tab          searchTab
	cursor       int
	width        int
	height       int
	loading      bool
	err          error
	hasSearched  bool
	inputFocused bool

	input textinput.Model

	repos  *jjhub.RepositorySearchPage
	issues *jjhub.IssueSearchPage
	code   *jjhub.CodeSearchPage
}

func NewSearchView(_ *smithers.Client) *SearchView {
	var client searchManager
	if jjhubAvailable() {
		client = jjhubSearchAdapter{client: jjhub.NewClient("")}
	}
	return newSearchViewWithClient(client)
}

type jjhubSearchAdapter struct {
	client *jjhub.Client
}

func (m jjhubSearchAdapter) GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error) {
	return m.client.GetCurrentRepo(ctx)
}

func (m jjhubSearchAdapter) SearchRepositories(ctx context.Context, query string, limit int) (*jjhub.RepositorySearchPage, error) {
	return m.client.SearchRepositories(ctx, query, limit)
}

func (m jjhubSearchAdapter) SearchIssues(ctx context.Context, query, state string, limit int) (*jjhub.IssueSearchPage, error) {
	return m.client.SearchIssues(ctx, query, state, limit)
}

func (m jjhubSearchAdapter) SearchCode(ctx context.Context, query string, limit int) (*jjhub.CodeSearchPage, error) {
	return m.client.SearchCode(ctx, query, limit)
}

func newSearchViewWithClient(client searchManager) *SearchView {
	input := textinput.New()
	input.Placeholder = "Search repositories, issues, or code"
	input.SetVirtualCursor(true)
	_ = input.Focus()

	v := &SearchView{
		client:       client,
		input:        input,
		inputFocused: true,
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

func (t searchTab) String() string {
	switch t {
	case searchTabIssues:
		return "issues"
	case searchTabCode:
		return "code"
	default:
		return "repos"
	}
}

func (v *SearchView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return v.loadRepoCmd()
}

func (v *SearchView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
		if err != nil {
			return nil
		}
		return searchRepoLoadedMsg{repo: repo}
	}
}

func (v *SearchView) searchCmd() tea.Cmd {
	client := v.client
	query := strings.TrimSpace(v.input.Value())
	tab := v.tab
	queryLength := len([]rune(query))
	if query == "" {
		err := errors.New("Search query must not be empty")
		observability.RecordUIAction("search", "query", 0, err,
			attribute.String("codeplane.search.tab", tab.String()),
			attribute.Int("codeplane.search.query_length", queryLength),
		)
		return func() tea.Msg {
			return searchErrorMsg{err: err}
		}
	}

	start := time.Now()
	return func() tea.Msg {
		switch tab {
		case searchTabIssues:
			results, err := client.SearchIssues(context.Background(), query, "all", 50)
			observability.RecordUIAction("search", "query", time.Since(start), err,
				attribute.String("codeplane.search.tab", tab.String()),
				attribute.Int("codeplane.search.query_length", queryLength),
				attribute.Int("codeplane.search.result_count", searchIssueCount(results)),
			)
			if err != nil {
				return searchErrorMsg{err: err}
			}
			return searchIssueResultsMsg{results: results}
		case searchTabCode:
			results, err := client.SearchCode(context.Background(), query, 50)
			observability.RecordUIAction("search", "query", time.Since(start), err,
				attribute.String("codeplane.search.tab", tab.String()),
				attribute.Int("codeplane.search.query_length", queryLength),
				attribute.Int("codeplane.search.result_count", searchCodeCount(results)),
			)
			if err != nil {
				return searchErrorMsg{err: err}
			}
			return searchCodeResultsMsg{results: results}
		default:
			results, err := client.SearchRepositories(context.Background(), query, 50)
			observability.RecordUIAction("search", "query", time.Since(start), err,
				attribute.String("codeplane.search.tab", tab.String()),
				attribute.Int("codeplane.search.query_length", queryLength),
				attribute.Int("codeplane.search.result_count", searchRepoCount(results)),
			)
			if err != nil {
				return searchErrorMsg{err: err}
			}
			return searchRepoResultsMsg{results: results}
		}
	}
}

func (v *SearchView) currentCount() int {
	switch v.tab {
	case searchTabIssues:
		if v.issues == nil {
			return 0
		}
		return len(v.issues.Items)
	case searchTabCode:
		if v.code == nil {
			return 0
		}
		return len(v.code.Items)
	default:
		if v.repos == nil {
			return 0
		}
		return len(v.repos.Items)
	}
}

func (v *SearchView) clampCursor() {
	count := v.currentCount()
	if count == 0 {
		v.cursor = 0
		return
	}
	if v.cursor >= count {
		v.cursor = count - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v *SearchView) cycleTab() tea.Cmd {
	v.tab = (v.tab + 1) % 3
	v.cursor = 0
	v.err = nil
	observability.RecordUIAction("search", "cycle_tab", 0, nil,
		attribute.String("codeplane.search.tab", v.tab.String()),
		attribute.Bool("codeplane.search.has_query", strings.TrimSpace(v.input.Value()) != ""),
		attribute.Bool("codeplane.search.has_searched", v.hasSearched),
	)
	if strings.TrimSpace(v.input.Value()) == "" || !v.hasSearched {
		return nil
	}
	v.loading = true
	return v.searchCmd()
}

func (v *SearchView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case searchRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case searchRepoResultsMsg:
		v.loading = false
		v.err = nil
		v.hasSearched = true
		v.repos = msg.results
		v.cursor = 0
		return v, nil

	case searchIssueResultsMsg:
		v.loading = false
		v.err = nil
		v.hasSearched = true
		v.issues = msg.results
		v.cursor = 0
		return v, nil

	case searchCodeResultsMsg:
		v.loading = false
		v.err = nil
		v.hasSearched = true
		v.code = msg.results
		v.cursor = 0
		return v, nil

	case searchErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.clampCursor()
		return v, nil

	case tea.KeyPressMsg:
		if v.inputFocused {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				v.inputFocused = false
				v.input.Blur()
				return v, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
				return v, v.cycleTab()
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				v.loading = true
				v.err = nil
				return v, v.searchCmd()
			default:
				var cmd tea.Cmd
				v.input, cmd = v.input.Update(msg)
				return v, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			v.inputFocused = true
			return v, v.input.Focus()
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			return v, v.cycleTab()
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if strings.TrimSpace(v.input.Value()) == "" {
				return v, nil
			}
			v.loading = true
			v.err = nil
			return v, v.searchCmd()
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < v.currentCount()-1 {
				v.cursor++
			}
			return v, nil
		}
	}

	return v, nil
}

func (v *SearchView) View() string {
	var b strings.Builder
	rightSide := jjhubJoinNonEmpty("  ",
		jjhubRepoLabel(v.repo),
		"[Esc] Back",
	)
	b.WriteString(ViewHeader(packageCom.Styles, "SMITHERS", "Search", v.width, rightSide))
	b.WriteString("\n\n")
	b.WriteString(v.renderTabBar())
	b.WriteString("\n")
	b.WriteString(v.input.View())
	b.WriteString("\n")
	b.WriteString(jjhubMutedStyle.Render("[Enter] search  [Tab] next tab  [/] edit query"))
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Searching...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if !v.hasSearched {
		b.WriteString(jjhubMutedStyle.Render("  Enter a query to search JJHub repositories, issues, or code."))
		return b.String()
	}
	if v.currentCount() == 0 {
		b.WriteString(jjhubMutedStyle.Render("  No results found."))
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderResultList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderResultDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderResultList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderResultDetail(v.width))
	return b.String()
}

func (v *SearchView) renderTabBar() string {
	tabs := []string{
		v.renderTab("Repos", v.tab == searchTabRepos),
		v.renderTab("Issues", v.tab == searchTabIssues),
		v.renderTab("Code", v.tab == searchTabCode),
	}
	return strings.Join(tabs, "  ")
}

func (v *SearchView) renderTab(label string, active bool) string {
	style := lipgloss.NewStyle().Faint(true)
	if active {
		style = style.Bold(true).Foreground(lipgloss.Color("111"))
	}
	return style.Render(label)
}

func (v *SearchView) renderResultList(width int) string {
	var b strings.Builder
	switch v.tab {
	case searchTabIssues:
		for i, item := range v.issues.Items {
			cursor := "  "
			titleStyle := lipgloss.NewStyle()
			if i == v.cursor {
				cursor = "▸ "
				titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
			}
			title := fmt.Sprintf("#%d %s", item.Number, item.Title)
			b.WriteString(cursor + titleStyle.Render(truncateStr(title, max(8, width-2))))
			b.WriteString("\n")
			meta := jjhubJoinNonEmpty(" · ", item.State, item.RepositoryName)
			b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
			b.WriteString("\n")
			if i < len(v.issues.Items)-1 {
				b.WriteString("\n")
			}
		}
	case searchTabCode:
		for i, item := range v.code.Items {
			cursor := "  "
			titleStyle := lipgloss.NewStyle()
			if i == v.cursor {
				cursor = "▸ "
				titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
			}
			title := item.Repository
			if strings.TrimSpace(item.FilePath) != "" {
				title = title + ":" + item.FilePath
			}
			b.WriteString(cursor + titleStyle.Render(truncateStr(title, max(8, width-2))))
			b.WriteString("\n")
			b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(searchCodePreview(item), max(8, width-2))))
			b.WriteString("\n")
			if i < len(v.code.Items)-1 {
				b.WriteString("\n")
			}
		}
	default:
		for i, item := range v.repos.Items {
			cursor := "  "
			titleStyle := lipgloss.NewStyle()
			if i == v.cursor {
				cursor = "▸ "
				titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
			}
			b.WriteString(cursor + titleStyle.Render(truncateStr(item.FullName, max(8, width-2))))
			b.WriteString("\n")
			b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(item.Description, max(8, width-2))))
			b.WriteString("\n")
			if i < len(v.repos.Items)-1 {
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func (v *SearchView) renderResultDetail(width int) string {
	switch v.tab {
	case searchTabIssues:
		if v.issues == nil || v.cursor >= len(v.issues.Items) {
			return "No issue selected."
		}
		item := v.issues.Items[v.cursor]
		var b strings.Builder
		b.WriteString(jjhubTitleStyle.Render(item.Title))
		b.WriteString("\n\n")
		b.WriteString(jjhubMetaRow("Issue", fmt.Sprintf("#%d", item.Number)))
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("State", item.State))
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Repository", item.RepositoryName))
		return b.String()
	case searchTabCode:
		if v.code == nil || v.cursor >= len(v.code.Items) {
			return "No code result selected."
		}
		item := v.code.Items[v.cursor]
		var b strings.Builder
		title := item.Repository
		if strings.TrimSpace(item.FilePath) != "" {
			title = title + ":" + item.FilePath
		}
		b.WriteString(jjhubTitleStyle.Render(title))
		b.WriteString("\n\n")
		if len(item.TextMatches) == 0 {
			b.WriteString(jjhubMutedStyle.Render("No text matches available."))
			return b.String()
		}
		for _, match := range item.TextMatches {
			line := match.Content
			if match.LineNumber > 0 {
				line = fmt.Sprintf("%d: %s", match.LineNumber, line)
			}
			b.WriteString(truncateStr(line, max(20, width)))
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	default:
		if v.repos == nil || v.cursor >= len(v.repos.Items) {
			return "No repository selected."
		}
		item := v.repos.Items[v.cursor]
		var b strings.Builder
		b.WriteString(jjhubTitleStyle.Render(item.FullName))
		b.WriteString("\n\n")
		b.WriteString(jjhubMetaRow("Owner", item.Owner))
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Visibility", boolLabel(item.IsPublic)))
		b.WriteString("\n")
		if len(item.Topics) > 0 {
			b.WriteString(jjhubMetaRow("Topics", strings.Join(item.Topics, ", ")))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if strings.TrimSpace(item.Description) == "" {
			b.WriteString(jjhubMutedStyle.Render("No description provided."))
		} else {
			b.WriteString(wrapText(item.Description, max(20, width)))
		}
		return strings.TrimRight(b.String(), "\n")
	}
}

func (v *SearchView) Name() string { return "search" }

func (v *SearchView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *SearchView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "edit query")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func searchCodePreview(item jjhub.CodeSearchItem) string {
	if len(item.TextMatches) == 0 {
		return item.Repository
	}
	return strings.TrimSpace(item.TextMatches[0].Content)
}

func searchRepoCount(results *jjhub.RepositorySearchPage) int {
	if results == nil {
		return 0
	}
	return len(results.Items)
}

func searchIssueCount(results *jjhub.IssueSearchPage) int {
	if results == nil {
		return 0
	}
	return len(results.Items)
}

func searchCodeCount(results *jjhub.CodeSearchPage) int {
	if results == nil {
		return 0
	}
	return len(results.Items)
}

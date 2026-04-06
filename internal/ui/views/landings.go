package views

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

var _ View = (*LandingsView)(nil)

type landingsLoadedMsg struct {
	landings []jjhub.Landing
}

type landingsErrorMsg struct {
	err error
}

type landingRepoLoadedMsg struct {
	repo *jjhub.Repo
}

type landingChangesLoadedMsg struct {
	changes []jjhub.Change
}

type landingDetailLoadedMsg struct {
	number int
	detail *jjhub.LandingDetail
}

type landingDetailErrorMsg struct {
	number int
	err    error
}

// LandingsView renders a JJHub landing request dashboard.
type LandingsView struct {
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

	allLandings []jjhub.Landing
	landings    []jjhub.Landing
	changeMap   map[string]jjhub.Change

	detailCache   map[int]*jjhub.LandingDetail
	detailLoading map[int]bool
	detailErrors  map[int]error

	tablePane   *jjTablePane
	previewPane *jjPreviewPane
	splitPane   *components.SplitPane
}

// LandingDetailView renders a full-screen tabbed landing detail view.
type LandingDetailView struct {
	parent View

	jjhubClient *jjhub.Client
	repo        *jjhub.Repo
	sty         styles.Styles

	width  int
	height int

	landing   jjhub.Landing
	detail    *jjhub.LandingDetail
	changeMap map[string]jjhub.Change

	loading bool
	err     error
	tab     int

	previewPane *jjPreviewPane
}

type landingDetailViewLoadedMsg struct {
	detail *jjhub.LandingDetail
}

type landingDetailViewErrorMsg struct {
	err error
}

var landingFilters = []jjFilterTab{
	{Value: "open", Label: "Open", Icon: jjhubLandingStateIcon("open")},
	{Value: "merged", Label: "Merged", Icon: jjhubLandingStateIcon("merged")},
	{Value: "closed", Label: "Closed", Icon: jjhubLandingStateIcon("closed")},
	{Value: "draft", Label: "Draft", Icon: jjhubLandingStateIcon("draft")},
	{Value: "all", Label: "All", Icon: "•"},
}

var landingTableColumns = []components.Column{
	{Title: "", Width: 2},
	{Title: "#", Width: 6, Align: components.AlignRight},
	{Title: "Title", Grow: true},
	{Title: "Author", Width: 14, MinWidth: 90},
	{Title: "Stack", Width: 7, MinWidth: 105, Align: components.AlignRight},
	{Title: "Reviews", Width: 8, MinWidth: 118, Align: components.AlignRight},
	{Title: "Conflicts", Width: 10, MinWidth: 100},
	{Title: "Updated", Width: 10, MinWidth: 82},
}

var landingDetailTabs = []string{"Overview", "Changes", "Reviews", "Conflicts"}

// NewLandingsView creates a JJHub landing request view.
func NewLandingsView(client *smithers.Client) *LandingsView {
	tablePane := newJJTablePane(landingTableColumns)
	previewPane := newJJPreviewPane("Select a landing request")
	splitPane := components.NewSplitPane(tablePane, previewPane, components.SplitPaneOpts{
		LeftWidth:         70,
		CompactBreakpoint: 100,
	})

	return &LandingsView{
		smithersClient: client,
		jjhubClient:    jjhub.NewClient(""),
		sty:            styles.DefaultStyles(),
		loading:        true,
		previewOpen:    true,
		search:         newJJSearchInput("filter landings by title"),
		changeMap:      make(map[string]jjhub.Change),
		detailCache:    make(map[int]*jjhub.LandingDetail),
		detailLoading:  make(map[int]bool),
		detailErrors:   make(map[int]error),
		tablePane:      tablePane,
		previewPane:    previewPane,
		splitPane:      splitPane,
	}
}

func (v *LandingsView) Init() tea.Cmd {
	return tea.Batch(
		v.loadLandingsCmd(),
		v.loadRepoCmd(),
		v.loadChangesCmd(),
	)
}

func (v *LandingsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case landingsLoadedMsg:
		v.loading = false
		v.err = nil
		v.allLandings = msg.landings
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case landingsErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case landingRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case landingChangesLoadedMsg:
		v.changeMap = make(map[string]jjhub.Change, len(msg.changes))
		for _, change := range msg.changes {
			v.changeMap[change.ChangeID] = change
		}
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case landingDetailLoadedMsg:
		delete(v.detailLoading, msg.number)
		delete(v.detailErrors, msg.number)
		v.detailCache[msg.number] = msg.detail
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case landingDetailErrorMsg:
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
			v.filterIndex = (v.filterIndex + 1) % len(landingFilters)
			selectionChanged := v.rebuildRows()
			return v, v.syncPreview(selectionChanged)
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			v.err = nil
			v.detailCache = make(map[int]*jjhub.LandingDetail)
			v.detailLoading = make(map[int]bool)
			v.detailErrors = make(map[int]error)
			selectionChanged := v.rebuildRows()
			return v, tea.Batch(v.Init(), v.syncPreview(selectionChanged))
		case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
			if landing := v.selectedLanding(); landing != nil {
				return v, jjOpenURLCmd(jjLandingURL(v.repo, landing.Number))
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if landing := v.selectedLanding(); landing != nil {
				return v, v.diffCmd(landing)
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if landing := v.selectedLanding(); landing != nil {
				detailView := NewLandingDetailView(v, v.jjhubClient, v.repo, v.sty, *landing, v.detailCache[landing.Number], v.changeMap)
				detailView.SetSize(v.width, v.height)
				return detailView, detailView.Init()
			}
		}
	}

	previous := v.selectedLandingNumber()
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

	selectionChanged := previous != v.selectedLandingNumber()
	return v, tea.Batch(cmd, v.syncPreview(selectionChanged))
}

func (v *LandingsView) View() string {
	headerRight := jjMutedStyle.Render("[/] Search  [w] Preview  [Esc] Back")
	header := jjRenderHeader(
		fmt.Sprintf("JJHUB › Landings (%d)", len(v.landings)),
		v.width,
		headerRight,
	)
	tabs := jjRenderFilterTabs(landingFilters, v.currentFilter(), v.stateCounts())

	var parts []string
	parts = append(parts, header)

	searchLine := tabs
	switch {
	case v.search.active:
		searchLine = tabs + "  " + jjSearchStyle.Render("Search:") + " " + v.search.input.View()
	case v.searchQuery != "":
		searchLine = tabs + "  " + jjMutedStyle.Render("filter: "+v.searchQuery)
	}
	parts = append(parts, searchLine)

	if v.loading && len(v.allLandings) == 0 {
		parts = append(parts, jjMutedStyle.Render("Loading landing requests…"))
		return strings.Join(parts, "\n")
	}
	if v.err != nil && len(v.allLandings) == 0 {
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

func (v *LandingsView) Name() string { return "landings" }

func (v *LandingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	contentHeight := max(1, height-3)
	v.tablePane.SetSize(width, contentHeight)
	v.previewPane.SetSize(max(1, width/2), contentHeight)
	v.splitPane.SetSize(width, contentHeight)
}

func (v *LandingsView) ShortHelp() []key.Binding {
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
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
	if v.previewOpen {
		help = append(help, key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")))
	}
	return help
}

func (v *LandingsView) currentFilter() string {
	return landingFilters[v.filterIndex].Value
}

func (v *LandingsView) stateCounts() map[string]int {
	counts := map[string]int{
		"open":   0,
		"merged": 0,
		"closed": 0,
		"draft":  0,
		"all":    len(v.allLandings),
	}
	for _, landing := range v.allLandings {
		counts[landing.State]++
	}
	return counts
}

func (v *LandingsView) selectedLanding() *jjhub.Landing {
	index := v.tablePane.Cursor()
	if index < 0 || index >= len(v.landings) {
		return nil
	}
	landing := v.landings[index]
	return &landing
}

func (v *LandingsView) selectedLandingNumber() int {
	if landing := v.selectedLanding(); landing != nil {
		return landing.Number
	}
	return 0
}

func (v *LandingsView) rebuildRows() bool {
	previous := v.selectedLandingNumber()
	filter := v.currentFilter()

	filtered := make([]jjhub.Landing, 0, len(v.allLandings))
	rows := make([]components.Row, 0, len(v.allLandings))
	for _, landing := range v.allLandings {
		if filter != "all" && landing.State != filter {
			continue
		}
		if v.searchQuery != "" && !jjMatchesSearch(landing.Title, v.searchQuery) {
			continue
		}
		filtered = append(filtered, landing)
		rows = append(rows, components.Row{
			Cells: []string{
				jjhubLandingStateIcon(landing.State),
				fmt.Sprintf("#%d", landing.Number),
				landing.Title,
				landing.Author.Login,
				fmt.Sprintf("%d", max(landing.StackSize, len(landing.ChangeIDs))),
				jjLandingReviewCell(v.detailCache[landing.Number]),
				jjLandingConflictCell(landing, v.detailCache[landing.Number]),
				jjhubRelativeTime(landing.UpdatedAt),
			},
		})
	}

	v.landings = filtered
	v.tablePane.SetRows(rows)

	targetIndex := 0
	for i, landing := range filtered {
		if landing.Number == previous {
			targetIndex = i
			break
		}
	}
	if len(filtered) > 0 {
		v.tablePane.SetCursor(targetIndex)
	}
	return previous != v.selectedLandingNumber()
}

func (v *LandingsView) renderPreview(landing jjhub.Landing) string {
	width := max(24, v.previewPane.width-4)
	detail := v.detailCache[landing.Number]
	var body strings.Builder

	body.WriteString(jjTitleStyle.Render(landing.Title))
	body.WriteString("\n")
	body.WriteString(jjBadgeStyleForState(landing.State).Render(jjhubLandingStateIcon(landing.State) + " " + landing.State))
	body.WriteString("\n\n")
	body.WriteString(jjMetaRow("Author", "@"+landing.Author.Login) + "\n")
	body.WriteString(jjMetaRow("Number", fmt.Sprintf("#%d", landing.Number)) + "\n")
	body.WriteString(jjMetaRow("Target", landing.TargetBookmark) + "\n")
	body.WriteString(jjMetaRow("Created", jjFormatTime(landing.CreatedAt)) + "\n")
	body.WriteString(jjMetaRow("Updated", jjFormatTime(landing.UpdatedAt)) + "\n")
	body.WriteString(jjMetaRow("Conflicts", lipgloss.NewStyle().UnsetWidth().Render(jjLandingConflictCell(landing, detail))) + "\n")

	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Stack"))
	body.WriteString("\n")
	for _, line := range v.renderLandingStack(detail, landing) {
		body.WriteString(line)
		body.WriteString("\n")
	}

	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Reviews"))
	body.WriteString("\n")
	if detail == nil {
		if v.detailErrors[landing.Number] != nil {
			body.WriteString(jjErrorStyle.Render(v.detailErrors[landing.Number].Error()))
		} else {
			body.WriteString(jjMutedStyle.Render("Loading review data…"))
		}
		body.WriteString("\n")
	} else if len(detail.Reviews) == 0 {
		body.WriteString(jjMutedStyle.Render("No reviews yet."))
		body.WriteString("\n")
	} else {
		for _, review := range detail.Reviews {
			line := fmt.Sprintf("%s reviewer #%d", jjReviewStateLabel(review.State), review.ReviewerID)
			body.WriteString(line)
			if review.Body != "" {
				body.WriteString("\n")
				body.WriteString(jjMutedStyle.Render(jjWrapText(strings.TrimSpace(review.Body), width)))
			}
			body.WriteString("\n")
		}
	}

	if detail != nil && detail.Conflicts.HasConflicts {
		body.WriteString("\n")
		body.WriteString(jjSectionStyle.Render("Conflict details"))
		body.WriteString("\n")
		keys := make([]string, 0, len(detail.Conflicts.ConflictsByChange))
		for changeID := range detail.Conflicts.ConflictsByChange {
			keys = append(keys, changeID)
		}
		sort.Strings(keys)
		for _, changeID := range keys {
			body.WriteString(fmt.Sprintf("%s %s\n", lipgloss.NewStyle().Bold(true).Render(changeID), detail.Conflicts.ConflictsByChange[changeID]))
		}
	}

	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Description"))
	body.WriteString("\n")
	if detail != nil && detail.Landing.Body != "" {
		body.WriteString(jjMarkdown(detail.Landing.Body, width, &v.sty))
	} else {
		body.WriteString(jjMarkdown(landing.Body, width, &v.sty))
	}

	return jjSidebarBoxStyle.Render(strings.TrimSpace(body.String()))
}

func (v *LandingsView) renderLandingStack(detail *jjhub.LandingDetail, landing jjhub.Landing) []string {
	if detail == nil || len(detail.Changes) == 0 {
		lines := make([]string, 0, len(landing.ChangeIDs))
		for i, changeID := range landing.ChangeIDs {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, changeID))
		}
		if len(lines) == 0 {
			lines = append(lines, jjMutedStyle.Render("No changes in stack."))
		}
		return lines
	}

	lines := make([]string, 0, len(detail.Changes))
	for _, change := range detail.Changes {
		line := fmt.Sprintf("%d. %s", change.PositionInStack, change.ChangeID)
		if summary, ok := v.changeMap[change.ChangeID]; ok && summary.Description != "" {
			line += "  " + jjMutedStyle.Render(truncateStr(summary.Description, 48))
		}
		lines = append(lines, line)
	}
	return lines
}

func (v *LandingsView) syncPreview(reset bool) tea.Cmd {
	landing := v.selectedLanding()
	if landing == nil {
		v.previewPane.SetContent("", true)
		return nil
	}
	v.previewPane.SetContent(v.renderPreview(*landing), reset)
	return v.ensureLandingDetail(*landing)
}

func (v *LandingsView) ensureLandingDetail(landing jjhub.Landing) tea.Cmd {
	if v.detailCache[landing.Number] != nil || v.detailLoading[landing.Number] {
		return nil
	}
	v.detailLoading[landing.Number] = true
	return v.loadLandingDetailCmd(landing.Number)
}

func (v *LandingsView) updateSearch(msg tea.KeyPressMsg) (View, tea.Cmd) {
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

func (v *LandingsView) loadLandingsCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		landings, err := client.ListLandings("all", jjDefaultListLimit)
		if err != nil {
			return landingsErrorMsg{err: err}
		}
		return landingsLoadedMsg{landings: landings}
	}
}

func (v *LandingsView) loadRepoCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		if err != nil {
			return nil
		}
		return landingRepoLoadedMsg{repo: repo}
	}
}

func (v *LandingsView) loadChangesCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		changes, err := client.ListChanges(jjDefaultListLimit)
		if err != nil {
			return nil
		}
		return landingChangesLoadedMsg{changes: changes}
	}
}

func (v *LandingsView) loadLandingDetailCmd(number int) tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		detail, err := client.ViewLanding(number)
		if err != nil {
			return landingDetailErrorMsg{number: number, err: err}
		}
		return landingDetailLoadedMsg{number: number, detail: detail}
	}
}

func (v *LandingsView) diffCmd(landing *jjhub.Landing) tea.Cmd {
	changeID := ""
	if len(landing.ChangeIDs) > 0 {
		changeID = landing.ChangeIDs[0]
	}
	if detail := v.detailCache[landing.Number]; detail != nil && len(detail.Changes) > 0 {
		changeID = detail.Changes[0].ChangeID
	}
	if changeID == "" {
		return func() tea.Msg {
			return components.ShowToastMsg{
				Title: "No diff available",
				Body:  "The selected landing does not include any changes.",
				Level: components.ToastLevelWarning,
			}
		}
	}

	if _, err := exec.LookPath("diffnav"); err == nil {
		return handoff.Handoff(handoff.Options{
			Binary: "zsh",
			Args: []string{
				"-lc",
				buildChangeDiffCommand(jjhub.Change{ChangeID: changeID}) + " | diffnav",
			},
			Tag: "landing-diff",
		})
	}

	return handoff.Handoff(handoff.Options{
		Binary: "jj",
		Args:   []string{"diff", "--git", "-r", changeID},
		Tag:    "landing-diff",
	})
}

// NewLandingDetailView creates a full-screen landing detail drill-down view.
func NewLandingDetailView(
	parent View,
	client *jjhub.Client,
	repo *jjhub.Repo,
	sty styles.Styles,
	landing jjhub.Landing,
	detail *jjhub.LandingDetail,
	changeMap map[string]jjhub.Change,
) *LandingDetailView {
	previewPane := newJJPreviewPane("Loading landing detail…")
	return &LandingDetailView{
		parent:      parent,
		jjhubClient: client,
		repo:        repo,
		sty:         sty,
		landing:     landing,
		detail:      detail,
		changeMap:   changeMap,
		loading:     detail == nil,
		previewPane: previewPane,
	}
}

func (v *LandingDetailView) Init() tea.Cmd {
	v.syncContent(true)
	if v.detail != nil {
		return nil
	}
	client := v.jjhubClient
	number := v.landing.Number
	return func() tea.Msg {
		detail, err := client.ViewLanding(number)
		if err != nil {
			return landingDetailViewErrorMsg{err: err}
		}
		return landingDetailViewLoadedMsg{detail: detail}
	}
}

func (v *LandingDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case landingDetailViewLoadedMsg:
		v.detail = msg.detail
		v.loading = false
		v.err = nil
		if parent, ok := v.parent.(*LandingsView); ok && msg.detail != nil {
			parent.detailCache[v.landing.Number] = msg.detail
			parent.rebuildRows()
			parent.syncPreview(false)
		}
		v.syncContent(true)
		return v, nil

	case landingDetailViewErrorMsg:
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("left"))):
			if v.tab > 0 {
				v.tab--
				v.syncContent(true)
			}
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("right"))):
			if v.tab < len(landingDetailTabs)-1 {
				v.tab++
				v.syncContent(true)
			}
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
			v.tab = 0
			v.syncContent(true)
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
			v.tab = 1
			v.syncContent(true)
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("3"))):
			v.tab = 2
			v.syncContent(true)
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("4"))):
			v.tab = 3
			v.syncContent(true)
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
			return v, jjOpenURLCmd(jjLandingURL(v.repo, v.landing.Number))
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			v.err = nil
			return v, v.Init()
		}
	}

	_, cmd := v.previewPane.Update(msg)
	return v, cmd
}

func (v *LandingDetailView) View() string {
	header := jjRenderHeader(
		fmt.Sprintf("JJHUB › Landings › #%d", v.landing.Number),
		v.width,
		jjMutedStyle.Render("[1-4] Tabs  [o] Browser  [Esc] Back"),
	)
	tabs := make([]string, 0, len(landingDetailTabs))
	for i, tab := range landingDetailTabs {
		style := jjBadgeBaseStyle.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
		if i == v.tab {
			style = style.Foreground(lipgloss.Color("111")).BorderForeground(lipgloss.Color("111")).Bold(true)
		} else {
			style = style.Faint(true)
		}
		tabs = append(tabs, style.Render(fmt.Sprintf("%d %s", i+1, tab)))
	}

	parts := []string{header, strings.Join(tabs, " ")}
	if v.err != nil {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
	}
	if v.loading && v.detail == nil {
		parts = append(parts, jjMutedStyle.Render("Loading landing detail…"))
	}
	parts = append(parts, v.previewPane.View())
	return strings.Join(parts, "\n")
}

func (v *LandingDetailView) Name() string { return "landing-detail" }

func (v *LandingDetailView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.previewPane.SetSize(width, max(1, height-3))
	v.syncContent(false)
}

func (v *LandingDetailView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "tabs")),
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "tabs")),
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "scroll")),
		key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "browser")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (v *LandingDetailView) syncContent(reset bool) {
	width := max(24, v.previewPane.width-4)
	var body strings.Builder

	body.WriteString(jjTitleStyle.Render(v.landing.Title))
	body.WriteString("\n")
	body.WriteString(jjBadgeStyleForState(v.landing.State).Render(jjhubLandingStateIcon(v.landing.State) + " " + v.landing.State))
	body.WriteString("\n\n")

	switch v.tab {
	case 0:
		body.WriteString(jjMetaRow("Author", "@"+v.landing.Author.Login) + "\n")
		body.WriteString(jjMetaRow("Number", fmt.Sprintf("#%d", v.landing.Number)) + "\n")
		body.WriteString(jjMetaRow("Target", v.landing.TargetBookmark) + "\n")
		body.WriteString(jjMetaRow("Updated", jjFormatTime(v.landing.UpdatedAt)) + "\n")
		body.WriteString(jjMetaRow("Conflicts", jjLandingConflictCell(v.landing, v.detail)) + "\n")
		body.WriteString("\n")
		body.WriteString(jjSectionStyle.Render("Description"))
		body.WriteString("\n")
		content := v.landing.Body
		if v.detail != nil && v.detail.Landing.Body != "" {
			content = v.detail.Landing.Body
		}
		body.WriteString(jjMarkdown(content, width, &v.sty))

	case 1:
		body.WriteString(jjSectionStyle.Render("Stack"))
		body.WriteString("\n")
		for _, line := range v.renderChangesTab() {
			body.WriteString(line)
			body.WriteString("\n")
		}

	case 2:
		body.WriteString(jjSectionStyle.Render("Reviews"))
		body.WriteString("\n")
		if v.detail == nil || len(v.detail.Reviews) == 0 {
			body.WriteString(jjMutedStyle.Render("No reviews yet."))
		} else {
			for _, review := range v.detail.Reviews {
				body.WriteString(fmt.Sprintf("%s reviewer #%d\n", jjReviewStateLabel(review.State), review.ReviewerID))
				if review.Body != "" {
					body.WriteString(jjWrapText(strings.TrimSpace(review.Body), width))
					body.WriteString("\n")
				}
				body.WriteString("\n")
			}
		}

	case 3:
		body.WriteString(jjSectionStyle.Render("Conflicts"))
		body.WriteString("\n")
		if v.detail == nil || !v.detail.Conflicts.HasConflicts {
			body.WriteString(jjSuccessStyle.Render("Stack is clean."))
		} else {
			keys := make([]string, 0, len(v.detail.Conflicts.ConflictsByChange))
			for changeID := range v.detail.Conflicts.ConflictsByChange {
				keys = append(keys, changeID)
			}
			sort.Strings(keys)
			for _, changeID := range keys {
				body.WriteString(lipgloss.NewStyle().Bold(true).Render(changeID))
				body.WriteString("\n")
				body.WriteString(jjWrapText(v.detail.Conflicts.ConflictsByChange[changeID], width))
				body.WriteString("\n\n")
			}
		}
	}

	if v.err != nil {
		body.WriteString("\n\n")
		body.WriteString(jjErrorStyle.Render(v.err.Error()))
	}

	v.previewPane.SetContent(strings.TrimSpace(body.String()), reset)
}

func (v *LandingDetailView) renderChangesTab() []string {
	if v.detail == nil || len(v.detail.Changes) == 0 {
		return []string{jjMutedStyle.Render("No stack data available.")}
	}
	lines := make([]string, 0, len(v.detail.Changes))
	for _, change := range v.detail.Changes {
		line := fmt.Sprintf("%d. %s", change.PositionInStack, change.ChangeID)
		if summary, ok := v.changeMap[change.ChangeID]; ok && summary.Description != "" {
			line += "\n" + jjMutedStyle.Render(summary.Description)
		}
		lines = append(lines, line)
	}
	return lines
}

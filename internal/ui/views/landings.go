package views

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*LandingsView)(nil)

type landingsLoadedMsg struct {
	landings []jjhub.Landing
}

type landingsErrorMsg struct {
	err error
}

type landingsRepoLoadedMsg struct {
	repo *jjhub.Repo
}

type landingDetailLoadedMsg struct {
	number int
	detail *jjhub.LandingDetail
}

type landingDetailErrorMsg struct {
	number int
	err    error
}

type landingDiffLoadedMsg struct {
	number int
	diff   string
}

type landingDiffErrorMsg struct {
	number int
	err    error
}

var landingStateCycle = []string{"open", "draft", "merged", "closed", "all"}

// LandingsView displays JJHub landing requests with a list/detail layout.
type LandingsView struct {
	client *jjhub.Client
	repo   *jjhub.Repo

	landings []jjhub.Landing

	stateFilter  string
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error
	showDiff     bool

	detailCache   map[int]*jjhub.LandingDetail
	detailErr     map[int]error
	detailLoading map[int]bool

	diffCache   map[int]string
	diffErr     map[int]error
	diffLoading map[int]bool
}

// NewLandingsView creates a JJHub landings view.
func NewLandingsView(_ *smithers.Client) *LandingsView {
	var client *jjhub.Client
	if jjhubAvailable() {
		client = jjhub.NewClient("")
	}
	v := &LandingsView{
		client:        client,
		stateFilter:   "open",
		loading:       client != nil,
		detailCache:   make(map[int]*jjhub.LandingDetail),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
		diffCache:     make(map[int]string),
		diffErr:       make(map[int]error),
		diffLoading:   make(map[int]bool),
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

// Init loads landing requests and repository metadata.
func (v *LandingsView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadLandingsCmd(), v.loadRepoCmd())
}

func (v *LandingsView) loadLandingsCmd() tea.Cmd {
	client := v.client
	state := v.stateFilter
	return func() tea.Msg {
		landings, err := client.ListLandings(state, 100)
		if err != nil {
			return landingsErrorMsg{err: err}
		}
		return landingsLoadedMsg{landings: landings}
	}
}

func (v *LandingsView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		if err != nil {
			return nil
		}
		return landingsRepoLoadedMsg{repo: repo}
	}
}

func (v *LandingsView) loadSelectedDetailCmd() tea.Cmd {
	landing := v.selectedLanding()
	if landing == nil || v.client == nil {
		return nil
	}
	if v.detailCache[landing.Number] != nil || v.detailLoading[landing.Number] {
		return nil
	}
	delete(v.detailErr, landing.Number)
	v.detailLoading[landing.Number] = true
	number := landing.Number
	client := v.client
	return func() tea.Msg {
		detail, err := client.ViewLanding(number)
		if err != nil {
			return landingDetailErrorMsg{number: number, err: err}
		}
		return landingDetailLoadedMsg{number: number, detail: detail}
	}
}

func (v *LandingsView) loadSelectedDiffCmd() tea.Cmd {
	landing := v.selectedLanding()
	if landing == nil || v.client == nil {
		return nil
	}
	if _, ok := v.diffCache[landing.Number]; ok || v.diffLoading[landing.Number] {
		return nil
	}
	delete(v.diffErr, landing.Number)
	v.diffLoading[landing.Number] = true
	number := landing.Number
	client := v.client
	return func() tea.Msg {
		diff, err := client.LandingDiff(number)
		if err != nil {
			return landingDiffErrorMsg{number: number, err: err}
		}
		return landingDiffLoadedMsg{number: number, diff: diff}
	}
}

func (v *LandingsView) selectedLanding() *jjhub.Landing {
	if len(v.landings) == 0 || v.cursor < 0 || v.cursor >= len(v.landings) {
		return nil
	}
	landing := v.landings[v.cursor]
	return &landing
}

func (v *LandingsView) pageSize() int {
	const (
		headerLines     = 6
		linesPerLanding = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / linesPerLanding
	if size < 1 {
		return 1
	}
	return size
}

func (v *LandingsView) clampCursor() {
	if len(v.landings) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(v.landings) {
		v.cursor = len(v.landings) - 1
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

func (v *LandingsView) cycleStateFilter() {
	for i, state := range landingStateCycle {
		if state == v.stateFilter {
			v.stateFilter = landingStateCycle[(i+1)%len(landingStateCycle)]
			v.cursor = 0
			v.scrollOffset = 0
			v.showDiff = false
			return
		}
	}
	v.stateFilter = landingStateCycle[0]
	v.cursor = 0
	v.scrollOffset = 0
	v.showDiff = false
}

// Update handles messages for the landings view.
func (v *LandingsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case landingsLoadedMsg:
		v.landings = msg.landings
		v.loading = false
		v.err = nil
		v.clampCursor()
		return v, v.loadSelectedDetailCmd()

	case landingsErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case landingsRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case landingDetailLoadedMsg:
		v.detailLoading[msg.number] = false
		v.detailCache[msg.number] = msg.detail
		delete(v.detailErr, msg.number)
		return v, nil

	case landingDetailErrorMsg:
		v.detailLoading[msg.number] = false
		v.detailErr[msg.number] = msg.err
		return v, nil

	case landingDiffLoadedMsg:
		v.diffLoading[msg.number] = false
		v.diffCache[msg.number] = msg.diff
		delete(v.diffErr, msg.number)
		return v, nil

	case landingDiffErrorMsg:
		v.diffLoading[msg.number] = false
		v.diffErr[msg.number] = msg.err
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
				v.showDiff = false
				v.clampCursor()
				return v, v.loadSelectedDetailCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.landings)-1 {
				v.cursor++
				v.showDiff = false
				v.clampCursor()
				return v, v.loadSelectedDetailCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if v.client == nil {
				return v, nil
			}
			v.loading = true
			v.err = nil
			v.showDiff = false
			return v, tea.Batch(v.loadLandingsCmd(), v.loadRepoCmd())

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			if v.client == nil {
				return v, nil
			}
			v.cycleStateFilter()
			v.loading = true
			v.err = nil
			return v, v.loadLandingsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if v.selectedLanding() == nil {
				return v, nil
			}
			v.showDiff = !v.showDiff
			if v.showDiff {
				return v, v.loadSelectedDiffCmd()
			}
		}
	}

	return v, nil
}

// View renders the landings view.
func (v *LandingsView) View() string {
	var b strings.Builder

	b.WriteString(jjhubHeader("JJHUB › Landings", v.width, jjhubJoinNonEmpty("  ",
		lipgloss.NewStyle().Faint(true).Render("["+v.stateFilter+"]"),
		jjhubRepoLabel(v.repo),
		lipgloss.NewStyle().Faint(true).Render("[Esc] Back"),
	)))
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString("  Loading landing requests...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.landings) == 0 {
		b.WriteString("  No landing requests found.\n")
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(50, max(36, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderLandingList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderLandingDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderLandingList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderLandingDetail(v.width))
	return b.String()
}

func (v *LandingsView) renderLandingList(width int) string {
	var b strings.Builder
	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(v.landings)-1))
	end := min(len(v.landings), start+pageSize)

	for i := start; i < end; i++ {
		landing := v.landings[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		b.WriteString(cursor + titleStyle.Render(truncateStr(fmt.Sprintf("#%d %s", landing.Number, landing.Title), max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			styleLandingState(landing.State),
			jjhubAtUser(landing.Author.Login),
			landingTargetLabel(landing.TargetBookmark),
			jjhubFormatRelativeTime(landing.UpdatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(v.landings) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(
			fmt.Sprintf("… %d more landing%s", len(v.landings)-end, pluralS(len(v.landings)-end)),
		))
	}

	return b.String()
}

func (v *LandingsView) renderLandingDetail(width int) string {
	landing := v.selectedLanding()
	if landing == nil {
		return "No landing selected."
	}
	if v.showDiff {
		return v.renderLandingDiff(*landing)
	}

	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d %s", landing.Number, landing.Title)))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("State", landing.State))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Author", jjhubAtUser(landing.Author.Login)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Target", landingTargetLabel(landing.TargetBookmark)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Changes", fmt.Sprintf("%d", landing.StackSize)))
	b.WriteString("\n")
	if landing.ConflictStatus != "" {
		b.WriteString(jjhubMetaRow("Conflicts", landing.ConflictStatus))
		b.WriteString("\n")
	}
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(landing.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(landing.UpdatedAt)))

	if strings.TrimSpace(landing.Body) != "" {
		b.WriteString("\n\n")
		b.WriteString(jjhubSectionStyle.Render("Body"))
		b.WriteString("\n")
		clipped, truncated := jjhubClipLines(wrapText(landing.Body, max(20, width)), max(6, v.height/5))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	if v.detailLoading[landing.Number] {
		b.WriteString("\n\n")
		b.WriteString(jjhubMutedStyle.Render("Loading landing details..."))
	}
	if v.detailErr[landing.Number] != nil {
		b.WriteString("\n\n")
		b.WriteString(jjhubErrorStyle.Render("Detail error: " + v.detailErr[landing.Number].Error()))
	}
	if detail := v.detailCache[landing.Number]; detail != nil {
		if len(detail.Changes) > 0 {
			b.WriteString("\n\n")
			b.WriteString(jjhubSectionStyle.Render("Stack"))
			for _, change := range detail.Changes {
				b.WriteString("\n")
				b.WriteString(wrapText(fmt.Sprintf("%d. %s", change.PositionInStack, change.ChangeID), max(20, width)))
			}
		}
		if len(detail.Reviews) > 0 {
			b.WriteString("\n\n")
			b.WriteString(jjhubSectionStyle.Render("Reviews"))
			for _, review := range detail.Reviews {
				b.WriteString("\n")
				line := fmt.Sprintf("%s · %s", review.Type, review.State)
				if review.Body != "" {
					line += " · " + review.Body
				}
				b.WriteString(wrapText(line, max(20, width)))
			}
		}
	}

	return b.String()
}

func (v *LandingsView) renderLandingDiff(landing jjhub.Landing) string {
	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d diff", landing.Number)))
	b.WriteString("\n\n")

	switch {
	case v.diffLoading[landing.Number]:
		b.WriteString(jjhubMutedStyle.Render("Loading diff..."))
	case v.diffErr[landing.Number] != nil:
		b.WriteString(jjhubErrorStyle.Render("Diff error: " + v.diffErr[landing.Number].Error()))
	default:
		diff := v.diffCache[landing.Number]
		if diff == "" {
			b.WriteString(jjhubMutedStyle.Render("No diff available."))
			break
		}
		clipped, truncated := jjhubClipLines(diff, max(10, v.height-8))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubMutedStyle.Render("[d] back to detail"))
	return b.String()
}

// Name returns the view name.
func (v *LandingsView) Name() string { return "landings" }

// SetSize stores the terminal size.
func (v *LandingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

// ShortHelp returns the contextual help bindings.
func (v *LandingsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle diff")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "state")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func landingTargetLabel(target string) string {
	if strings.TrimSpace(target) == "" {
		return "default"
	}
	return target
}

func styleLandingState(state string) string {
	style := lipgloss.NewStyle()
	switch strings.ToLower(state) {
	case "open":
		style = style.Foreground(lipgloss.Color("111")).Bold(true)
	case "merged":
		style = style.Foreground(lipgloss.Color("42")).Bold(true)
	case "draft":
		style = style.Foreground(lipgloss.Color("214")).Bold(true)
	case "closed":
		style = style.Foreground(lipgloss.Color("245"))
	default:
		style = style.Faint(true)
	}
	return style.Render(state)
}

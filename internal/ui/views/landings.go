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
	"github.com/charmbracelet/crush/internal/ui/common"
	"go.opentelemetry.io/otel/attribute"
)

var _ View = (*LandingsView)(nil)

type landingManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	ListLandings(ctx context.Context, state string, limit int) ([]jjhub.Landing, error)
	ViewLanding(ctx context.Context, number int) (*jjhub.LandingDetail, error)
	CreateLanding(ctx context.Context, title, body, target string, stack bool) (*jjhub.Landing, error)
	ReviewLanding(ctx context.Context, number int, action, body string) error
	LandLanding(ctx context.Context, number int) error
	LandingDiff(ctx context.Context, number int) (string, error)
	LandingChecks(ctx context.Context, number int) (string, error)
}

type landingPanelMode uint8

const (
	landingPanelDetail landingPanelMode = iota
	landingPanelDiff
	landingPanelChecks
)

type landingPromptKind uint8

const (
	landingPromptCreateTitle landingPromptKind = iota
	landingPromptCreateBody
	landingPromptCreateTarget
	landingPromptApprove
	landingPromptRequestChanges
	landingPromptComment
	landingPromptLandConfirm
)

type landingPromptState struct {
	active bool
	kind   landingPromptKind
	input  textinput.Model
	title  string
	body   string
	err    error
}

type (
	landingsLoadedMsg struct {
		landings []jjhub.Landing
	}
	landingsErrorMsg struct {
		err error
	}
	landingsRepoLoadedMsg struct {
		repo *jjhub.Repo
	}
	landingDetailLoadedMsg struct {
		number int
		detail *jjhub.LandingDetail
	}
	landingDetailErrorMsg struct {
		number int
		err    error
	}
	landingDiffLoadedMsg struct {
		number int
		diff   string
	}
	landingDiffErrorMsg struct {
		number int
		err    error
	}
	landingChecksLoadedMsg struct {
		number int
		checks string
	}
	landingChecksErrorMsg struct {
		number int
		err    error
	}
	landingActionDoneMsg struct {
		message           string
		landing           *jjhub.Landing
		targetStateFilter string
	}
	landingActionErrorMsg struct {
		err error
	}
)

var landingStateCycle = []string{"open", "draft", "merged", "closed", "all"}

type LandingsView struct {
	com    *common.Common
	client landingManager
	repo   *jjhub.Repo

	landings []jjhub.Landing

	stateFilter  string
	panel        landingPanelMode
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error

	detailCache   map[int]*jjhub.LandingDetail
	detailErr     map[int]error
	detailLoading map[int]bool

	diffCache   map[int]string
	diffErr     map[int]error
	diffLoading map[int]bool

	checksCache   map[int]string
	checksErr     map[int]error
	checksLoading map[int]bool

	actionMsg            string
	pendingSelectLanding int
	prompt               landingPromptState
}

func NewLandingsView(client *smithers.Client) *LandingsView {
	var lm landingManager
	if jjhubAvailable() {
		lm = jjhubLandingManager{client: jjhub.NewClient("")}
	}
	return newLandingsViewWithClient(viewCommon(nil), lm)
}

type jjhubLandingManager struct {
	client *jjhub.Client
}

func (m jjhubLandingManager) GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error) {
	return m.client.GetCurrentRepo(ctx)
}

func (m jjhubLandingManager) ListLandings(ctx context.Context, state string, limit int) ([]jjhub.Landing, error) {
	return m.client.ListLandings(ctx, state, limit)
}

func (m jjhubLandingManager) ViewLanding(ctx context.Context, number int) (*jjhub.LandingDetail, error) {
	return m.client.ViewLanding(ctx, number)
}

func (m jjhubLandingManager) CreateLanding(ctx context.Context, title, body, target string, stack bool) (*jjhub.Landing, error) {
	return m.client.CreateLanding(ctx, title, body, target, stack)
}

func (m jjhubLandingManager) ReviewLanding(ctx context.Context, number int, action, body string) error {
	return m.client.ReviewLanding(ctx, number, action, body)
}

func (m jjhubLandingManager) LandLanding(ctx context.Context, number int) error {
	return m.client.LandLanding(ctx, number)
}

func (m jjhubLandingManager) LandingDiff(ctx context.Context, number int) (string, error) {
	return m.client.LandingDiff(ctx, number)
}

func (m jjhubLandingManager) LandingChecks(ctx context.Context, number int) (string, error) {
	return m.client.LandingChecks(ctx, number)
}

func newLandingsViewWithClient(args ...any) *LandingsView {
	com := viewCommon(nil)
	var client landingManager
	switch len(args) {
	case 1:
		if provided, ok := args[0].(landingManager); ok {
			client = provided
		}
	case 2:
		if provided, ok := args[0].(*common.Common); ok {
			com = viewCommon(provided)
		}
		if provided, ok := args[1].(landingManager); ok {
			client = provided
		}
	}

	input := textinput.New()
	input.Placeholder = "Landing title"
	input.SetVirtualCursor(true)

	v := &LandingsView{
		com:           com,
		client:        client,
		stateFilter:   "open",
		panel:         landingPanelDetail,
		loading:       client != nil,
		detailCache:   make(map[int]*jjhub.LandingDetail),
		detailErr:     make(map[int]error),
		detailLoading: make(map[int]bool),
		diffCache:     make(map[int]string),
		diffErr:       make(map[int]error),
		diffLoading:   make(map[int]bool),
		checksCache:   make(map[int]string),
		checksErr:     make(map[int]error),
		checksLoading: make(map[int]bool),
		prompt: landingPromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

func (v *LandingsView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadLandingsCmd(), v.loadRepoCmd())
}

func (v *LandingsView) loadLandingsCmd() tea.Cmd {
	client := v.client
	state := v.stateFilter
	start := time.Now()
	return func() tea.Msg {
		landings, err := client.ListLandings(context.Background(), state, 100)
		observability.RecordUIAction("landings", "list", time.Since(start), err,
			attribute.String("codeplane.landings.state_filter", state),
			attribute.Int("codeplane.landings.result_count", len(landings)),
		)
		if err != nil {
			return landingsErrorMsg{err: err}
		}
		return landingsLoadedMsg{landings: landings}
	}
}

func (v *LandingsView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
		if err != nil {
			return nil
		}
		return landingsRepoLoadedMsg{repo: repo}
	}
}

func (v *LandingsView) selectedLanding() *jjhub.Landing {
	if len(v.landings) == 0 || v.cursor < 0 || v.cursor >= len(v.landings) {
		return nil
	}
	landing := v.landings[v.cursor]
	return &landing
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
	start := time.Now()
	return func() tea.Msg {
		detail, err := client.ViewLanding(context.Background(), number)
		observability.RecordUIAction("landings", "detail", time.Since(start), err,
			attribute.Int("codeplane.landings.number", number),
		)
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
	start := time.Now()
	return func() tea.Msg {
		diff, err := client.LandingDiff(context.Background(), number)
		observability.RecordUIAction("landings", "diff", time.Since(start), err,
			attribute.Int("codeplane.landings.number", number),
			attribute.Int("codeplane.landings.diff_length", len(diff)),
		)
		if err != nil {
			return landingDiffErrorMsg{number: number, err: err}
		}
		return landingDiffLoadedMsg{number: number, diff: diff}
	}
}

func (v *LandingsView) loadSelectedChecksCmd() tea.Cmd {
	landing := v.selectedLanding()
	if landing == nil || v.client == nil {
		return nil
	}
	if _, ok := v.checksCache[landing.Number]; ok || v.checksLoading[landing.Number] {
		return nil
	}
	delete(v.checksErr, landing.Number)
	v.checksLoading[landing.Number] = true

	number := landing.Number
	client := v.client
	start := time.Now()
	return func() tea.Msg {
		checks, err := client.LandingChecks(context.Background(), number)
		observability.RecordUIAction("landings", "checks", time.Since(start), err,
			attribute.Int("codeplane.landings.number", number),
			attribute.Int("codeplane.landings.checks_length", len(checks)),
		)
		if err != nil {
			return landingChecksErrorMsg{number: number, err: err}
		}
		return landingChecksLoadedMsg{number: number, checks: checks}
	}
}

func (v *LandingsView) selectedPanelCmd() tea.Cmd {
	switch v.panel {
	case landingPanelDiff:
		return v.loadSelectedDiffCmd()
	case landingPanelChecks:
		return v.loadSelectedChecksCmd()
	default:
		return v.loadSelectedDetailCmd()
	}
}

func (v *LandingsView) refreshCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = true
	v.err = nil
	return tea.Batch(v.loadLandingsCmd(), v.loadRepoCmd())
}

func (v *LandingsView) pageSize() int {
	const (
		headerLines     = 6
		linesPerLanding = 4
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
			return
		}
	}
	v.stateFilter = landingStateCycle[0]
	v.cursor = 0
	v.scrollOffset = 0
}

func (v *LandingsView) selectLandingByNumber(number int) {
	if number <= 0 {
		return
	}
	for i, landing := range v.landings {
		if landing.Number == number {
			v.cursor = i
			v.clampCursor()
			return
		}
	}
}

func (v *LandingsView) createLandingCmd(title, body, target string) tea.Cmd {
	client := v.client
	start := time.Now()
	return func() tea.Msg {
		landing, err := client.CreateLanding(context.Background(), title, body, target, true)
		attrs := []attribute.KeyValue{
			attribute.Int("codeplane.landings.title_length", len([]rune(title))),
			attribute.Int("codeplane.landings.body_length", len([]rune(body))),
			attribute.Bool("codeplane.landings.has_target", strings.TrimSpace(target) != ""),
		}
		if landing != nil {
			attrs = append(attrs, attribute.Int("codeplane.landings.number", landing.Number))
		}
		observability.RecordUIAction("landings", "create", time.Since(start), err, attrs...)
		if err != nil {
			return landingActionErrorMsg{err: err}
		}
		return landingActionDoneMsg{
			message:           fmt.Sprintf("Created landing #%d", landing.Number),
			landing:           landing,
			targetStateFilter: "open",
		}
	}
}

func (v *LandingsView) reviewLandingCmd(action, body string) tea.Cmd {
	client := v.client
	landing := v.selectedLanding()
	if landing == nil {
		return nil
	}
	number := landing.Number
	start := time.Now()
	return func() tea.Msg {
		if err := client.ReviewLanding(context.Background(), number, action, body); err != nil {
			observability.RecordUIAction("landings", "review", time.Since(start), err,
				attribute.Int("codeplane.landings.number", number),
				attribute.String("codeplane.landings.review_action", action),
				attribute.Int("codeplane.landings.body_length", len([]rune(body))),
			)
			return landingActionErrorMsg{err: err}
		}
		observability.RecordUIAction("landings", "review", time.Since(start), nil,
			attribute.Int("codeplane.landings.number", number),
			attribute.String("codeplane.landings.review_action", action),
			attribute.Int("codeplane.landings.body_length", len([]rune(body))),
		)
		return landingActionDoneMsg{
			message: fmt.Sprintf("%s landing #%d", landingReviewActionLabel(action), number),
			landing: landing,
		}
	}
}

func (v *LandingsView) landLandingCmd() tea.Cmd {
	client := v.client
	landing := v.selectedLanding()
	if landing == nil {
		return nil
	}
	number := landing.Number
	start := time.Now()
	return func() tea.Msg {
		if err := client.LandLanding(context.Background(), number); err != nil {
			observability.RecordUIAction("landings", "land", time.Since(start), err,
				attribute.Int("codeplane.landings.number", number),
			)
			return landingActionErrorMsg{err: err}
		}
		observability.RecordUIAction("landings", "land", time.Since(start), nil,
			attribute.Int("codeplane.landings.number", number),
		)
		return landingActionDoneMsg{message: fmt.Sprintf("Landed landing #%d", number)}
	}
}

func (v *LandingsView) openPrompt(kind landingPromptKind, placeholder string) tea.Cmd {
	v.prompt.active = true
	v.prompt.kind = kind
	v.prompt.err = nil
	v.prompt.title = ""
	v.prompt.body = ""
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = placeholder
	if landingPromptUsesInput(kind) {
		return v.prompt.input.Focus()
	}
	return nil
}

func (v *LandingsView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.title = ""
	v.prompt.body = ""
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *LandingsView) submitPrompt() tea.Cmd {
	switch v.prompt.kind {
	case landingPromptCreateTitle:
		title := strings.TrimSpace(v.prompt.input.Value())
		if title == "" {
			v.prompt.err = errors.New("Title must not be empty")
			return nil
		}
		v.prompt.kind = landingPromptCreateBody
		v.prompt.err = nil
		v.prompt.title = title
		v.prompt.input.Reset()
		v.prompt.input.Placeholder = "Optional landing description"
		return v.prompt.input.Focus()
	case landingPromptCreateBody:
		v.prompt.kind = landingPromptCreateTarget
		v.prompt.err = nil
		v.prompt.body = strings.TrimSpace(v.prompt.input.Value())
		v.prompt.input.Reset()
		v.prompt.input.Placeholder = "Target bookmark (defaults to main)"
		return v.prompt.input.Focus()
	case landingPromptCreateTarget:
		return v.createLandingCmd(v.prompt.title, v.prompt.body, strings.TrimSpace(v.prompt.input.Value()))
	case landingPromptApprove:
		return v.reviewLandingCmd("approve", strings.TrimSpace(v.prompt.input.Value()))
	case landingPromptRequestChanges:
		return v.reviewLandingCmd("request_changes", strings.TrimSpace(v.prompt.input.Value()))
	case landingPromptComment:
		return v.reviewLandingCmd("comment", strings.TrimSpace(v.prompt.input.Value()))
	case landingPromptLandConfirm:
		return v.landLandingCmd()
	default:
		return nil
	}
}

func (v *LandingsView) renderPrompt(width int) string {
	boxWidth := max(36, min(max(36, width-4), 84))

	title := "Create landing"
	description := "Enter a title for the landing request."
	switch v.prompt.kind {
	case landingPromptCreateBody:
		description = "Add an optional description for the landing request."
	case landingPromptCreateTarget:
		description = "Pick a target bookmark. Leave blank to use the CLI default."
	case landingPromptApprove:
		title = "Approve landing"
		description = "Optional review body."
	case landingPromptRequestChanges:
		title = "Request changes"
		description = "Explain what still needs to change."
	case landingPromptComment:
		title = "Comment on landing"
		description = "Leave an optional comment-only review."
	case landingPromptLandConfirm:
		title = "Land request"
		description = "Press Enter to confirm landing the selected stack."
	}

	var body strings.Builder
	body.WriteString(jjhubSectionStyle.Render(title))
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render(description))

	if strings.TrimSpace(v.prompt.title) != "" && v.prompt.kind != landingPromptCreateTitle {
		body.WriteString("\n\n")
		body.WriteString(jjhubMetaRow("Title", truncateStr(v.prompt.title, max(20, boxWidth-16))))
	}
	if strings.TrimSpace(v.prompt.body) != "" && v.prompt.kind == landingPromptCreateTarget {
		body.WriteString("\n")
		body.WriteString(jjhubMetaRow("Body", truncateStr(v.prompt.body, max(20, boxWidth-16))))
	}

	body.WriteString("\n\n")
	if landingPromptUsesInput(v.prompt.kind) {
		body.WriteString(v.prompt.input.View())
		body.WriteString("\n")
	}
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

func (v *LandingsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case landingsLoadedMsg:
		v.landings = msg.landings
		v.loading = false
		v.err = nil
		if v.pendingSelectLanding > 0 {
			v.selectLandingByNumber(v.pendingSelectLanding)
			v.pendingSelectLanding = 0
		}
		v.clampCursor()
		return v, v.selectedPanelCmd()

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

	case landingChecksLoadedMsg:
		v.checksLoading[msg.number] = false
		v.checksCache[msg.number] = msg.checks
		delete(v.checksErr, msg.number)
		return v, nil

	case landingChecksErrorMsg:
		v.checksLoading[msg.number] = false
		v.checksErr[msg.number] = msg.err
		return v, nil

	case landingActionDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		if msg.targetStateFilter != "" {
			v.stateFilter = msg.targetStateFilter
		}
		if msg.landing != nil {
			v.pendingSelectLanding = msg.landing.Number
		}
		return v, v.refreshCmd()

	case landingActionErrorMsg:
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
				if landingPromptUsesInput(v.prompt.kind) {
					var cmd tea.Cmd
					v.prompt.input, cmd = v.prompt.input.Update(msg)
					return v, cmd
				}
				return v, nil
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
				v.clampCursor()
				return v, v.selectedPanelCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.landings)-1 {
				v.cursor++
				v.clampCursor()
				return v, v.selectedPanelCmd()
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
			observability.RecordUIAction("landings", "set_state_filter", 0, nil,
				attribute.String("codeplane.landings.state_filter", v.stateFilter),
			)
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			if v.client == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(landingPromptCreateTitle, "Landing title")

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			if v.client == nil || v.selectedLanding() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(landingPromptApprove, "Optional review body")

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			if v.client == nil || v.selectedLanding() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(landingPromptRequestChanges, "Required review body")

		case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
			if v.client == nil || v.selectedLanding() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(landingPromptComment, "Optional comment")

		case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
			if v.client == nil || v.selectedLanding() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(landingPromptLandConfirm, "")

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if v.selectedLanding() == nil {
				return v, nil
			}
			if v.panel == landingPanelDiff {
				v.panel = landingPanelDetail
			} else {
				v.panel = landingPanelDiff
			}
			observability.RecordUIAction("landings", "toggle_panel", 0, nil,
				attribute.String("codeplane.landings.panel", v.panel.String()),
			)
			return v, v.selectedPanelCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			if v.selectedLanding() == nil {
				return v, nil
			}
			if v.panel == landingPanelChecks {
				v.panel = landingPanelDetail
			} else {
				v.panel = landingPanelChecks
			}
			observability.RecordUIAction("landings", "toggle_panel", 0, nil,
				attribute.String("codeplane.landings.panel", v.panel.String()),
			)
			return v, v.selectedPanelCmd()
		}
	}

	return v, nil
}

func (v *LandingsView) View() string {
	var b strings.Builder
	panelLabel := "[detail]"
	switch v.panel {
	case landingPanelDiff:
		panelLabel = "[diff]"
	case landingPanelChecks:
		panelLabel = "[checks]"
	}

	rightSide := jjhubJoinNonEmpty("  ",
		"["+v.stateFilter+"]",
		panelLabel,
		jjhubRepoLabel(v.repo),
		"[Esc] Back",
	)
	b.WriteString(ViewHeader(v.com.Styles, "CODEPLANE", "Landings", v.width, rightSide))
	b.WriteString("\n\n")

	if v.prompt.active {
		b.WriteString(v.renderPrompt(v.width))
		b.WriteString("\n\n")
	} else if strings.TrimSpace(v.actionMsg) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  "+v.actionMsg) + "\n\n")
	}

	if v.loading {
		b.WriteString("  Loading landings...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.landings) == 0 {
		b.WriteString("  No landings found.\n\n")
		b.WriteString(jjhubMutedStyle.Render("  Press c to create a landing request from the current stack."))
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderLandingList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderLandingPanel(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderLandingList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderLandingPanel(v.width))
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
			fmt.Sprintf("%d change%s", landing.StackSize, pluralS(landing.StackSize)),
			jjhubFormatRelativeTime(landing.UpdatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		secondary := jjhubJoinNonEmpty(" · ",
			landing.TargetBookmark,
			landing.ConflictStatus,
		)
		if strings.TrimSpace(secondary) == "" {
			secondary = "No conflicts reported"
		}
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(secondary, max(8, width-2))))
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

func (v *LandingsView) renderLandingPanel(width int) string {
	landing := v.selectedLanding()
	if landing == nil {
		return "No landing selected."
	}

	switch v.panel {
	case landingPanelDiff:
		return v.renderLandingDiff(*landing)
	case landingPanelChecks:
		return v.renderLandingChecks(*landing)
	default:
		return v.renderLandingDetail(width, *landing)
	}
}

func (v *LandingsView) renderLandingDetail(width int, landing jjhub.Landing) string {
	detail := v.detailCache[landing.Number]
	current := landing
	if detail != nil {
		current = detail.Landing
	}

	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d %s", current.Number, current.Title)))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("State", current.State))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Author", jjhubAtUser(current.Author.Login)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Target", current.TargetBookmark))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(current.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(current.UpdatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Conflicts", boolLabel(strings.EqualFold(current.ConflictStatus, "conflict"))))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Description"))
	b.WriteString("\n")
	if strings.TrimSpace(current.Body) == "" {
		b.WriteString(jjhubMutedStyle.Render("No description provided."))
	} else {
		clipped, truncated := jjhubClipLines(wrapText(current.Body, max(20, width)), max(6, v.height/4))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	if detail != nil {
		b.WriteString("\n\n")
		b.WriteString(jjhubSectionStyle.Render("Changes"))
		b.WriteString("\n")
		if len(detail.Changes) == 0 {
			b.WriteString(jjhubMutedStyle.Render("No change list returned."))
		} else {
			for _, change := range detail.Changes {
				b.WriteString("  ")
				b.WriteString(change.ChangeID)
				if change.PositionInStack > 0 {
					b.WriteString(jjhubMutedStyle.Render(fmt.Sprintf("  · stack %d", change.PositionInStack)))
				}
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
		b.WriteString(jjhubSectionStyle.Render("Reviews"))
		b.WriteString("\n")
		if len(detail.Reviews) == 0 {
			b.WriteString(jjhubMutedStyle.Render("No reviews yet."))
		} else {
			for _, review := range detail.Reviews {
				line := jjhubJoinNonEmpty(" · ", review.Type, review.State, jjhubFormatRelativeTime(review.UpdatedAt))
				b.WriteString("  " + truncateStr(line, max(16, width-2)))
				if strings.TrimSpace(review.Body) != "" {
					b.WriteString("\n")
					b.WriteString(truncateStr(wrapText(review.Body, max(20, width-2)), max(20, width)))
				}
				b.WriteString("\n")
			}
		}

		if detail.Conflicts.HasConflicts {
			b.WriteString("\n")
			b.WriteString(jjhubSectionStyle.Render("Conflicts"))
			b.WriteString("\n")
			for changeID, conflict := range detail.Conflicts.ConflictsByChange {
				b.WriteString("  " + truncateStr(changeID+": "+conflict, max(16, width-2)) + "\n")
			}
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

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Actions"))
	b.WriteString("\n")
	b.WriteString(wrapText("[d] diff  [t] checks  [a] approve  [x] request changes  [m] comment  [l] land", max(20, width)))
	return b.String()
}

func (v *LandingsView) renderLandingDiff(landing jjhub.Landing) string {
	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d diff", landing.Number)))
	b.WriteString("\n\n")

	switch {
	case v.diffLoading[landing.Number]:
		b.WriteString(jjhubMutedStyle.Render("Loading landing diff..."))
	case v.diffErr[landing.Number] != nil:
		b.WriteString(jjhubErrorStyle.Render("Diff error: " + v.diffErr[landing.Number].Error()))
	default:
		diff := v.diffCache[landing.Number]
		if strings.TrimSpace(diff) == "" {
			b.WriteString(jjhubMutedStyle.Render("No diff available."))
		} else {
			clipped, truncated := jjhubClipLines(diff, max(10, v.height-8))
			b.WriteString(clipped)
			if truncated {
				b.WriteString("\n")
				b.WriteString(jjhubMutedStyle.Render("…"))
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubMutedStyle.Render("[d] back to detail"))
	return b.String()
}

func (v *LandingsView) renderLandingChecks(landing jjhub.Landing) string {
	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("#%d checks", landing.Number)))
	b.WriteString("\n\n")

	switch {
	case v.checksLoading[landing.Number]:
		b.WriteString(jjhubMutedStyle.Render("Loading landing checks..."))
	case v.checksErr[landing.Number] != nil:
		b.WriteString(jjhubErrorStyle.Render("Checks error: " + v.checksErr[landing.Number].Error()))
	default:
		checks := strings.TrimSpace(v.checksCache[landing.Number])
		if checks == "" {
			b.WriteString(jjhubMutedStyle.Render("No checks available."))
		} else {
			clipped, truncated := jjhubClipLines(checks, max(10, v.height-8))
			b.WriteString(clipped)
			if truncated {
				b.WriteString("\n")
				b.WriteString(jjhubMutedStyle.Render("…"))
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubMutedStyle.Render("[t] back to detail"))
	return b.String()
}

func (v *LandingsView) Name() string { return "landings" }

func (v *LandingsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

func (v *LandingsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "request changes")),
		key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "comment")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "land")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "checks")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "state")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (m landingPanelMode) String() string {
	switch m {
	case landingPanelDiff:
		return "diff"
	case landingPanelChecks:
		return "checks"
	default:
		return "detail"
	}
}

func landingPromptUsesInput(kind landingPromptKind) bool {
	return kind != landingPromptLandConfirm
}

func landingReviewActionLabel(action string) string {
	switch action {
	case "approve":
		return "Approved"
	case "request_changes":
		return "Requested changes on"
	default:
		return "Commented on"
	}
}

func styleLandingState(state string) string {
	style := lipgloss.NewStyle()
	switch strings.ToLower(state) {
	case "open":
		style = style.Foreground(lipgloss.Color("111")).Bold(true)
	case "merged":
		style = style.Foreground(lipgloss.Color("42")).Bold(true)
	case "closed":
		style = style.Foreground(lipgloss.Color("245"))
	case "draft":
		style = style.Foreground(lipgloss.Color("214"))
	default:
		style = style.Faint(true)
	}
	return style.Render(state)
}

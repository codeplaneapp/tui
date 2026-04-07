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
	"github.com/charmbracelet/crush/internal/ui/common"
)

var _ View = (*ChangesView)(nil)

type changesViewMode uint8

const (
	changesModeList changesViewMode = iota
	changesModeStatus
)

type changeManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	ListChanges(ctx context.Context, limit int) ([]jjhub.Change, error)
	ViewChange(ctx context.Context, changeID string) (*jjhub.Change, error)
	ChangeDiff(ctx context.Context, changeID string) (string, error)
	Status(ctx context.Context) (string, error)
	CreateBookmark(ctx context.Context, name, changeID string, remote bool) (*jjhub.Bookmark, error)
	DeleteBookmark(ctx context.Context, name string, remote bool) error
}

type changeRepoLoadedMsg struct {
	repo *jjhub.Repo
}

type changesLoadedMsg struct {
	changes []jjhub.Change
}

type changesErrorMsg struct {
	err error
}

type changeDetailLoadedMsg struct {
	changeID string
	change   *jjhub.Change
}

type changeDetailErrorMsg struct {
	changeID string
	err      error
}

type changeDiffLoadedMsg struct {
	changeID string
	diff     string
}

type changeDiffErrorMsg struct {
	changeID string
	err      error
}

type changeStatusLoadedMsg struct {
	status string
}

type changeStatusErrorMsg struct {
	err error
}

type workingDiffLoadedMsg struct {
	diff string
}

type workingDiffErrorMsg struct {
	err error
}

type workingCopyLoadedMsg struct {
	status    string
	statusErr error
	diff      string
	diffErr   error
}

type changeActionDoneMsg struct {
	message string
}

type changeActionErrorMsg struct {
	err error
}

type changePromptKind uint8

const (
	changePromptCreateBookmark changePromptKind = iota
	changePromptDeleteBookmark
)

const changePromptBookmarkName = changePromptCreateBookmark

type changePromptState struct {
	active bool
	kind   changePromptKind
	input  textinput.Model
	err    error
}

// ChangesView displays JJHub recent changes and working copy status.
type ChangesView struct {
	com       *common.Common
	client    changeManager
	repo      *jjhub.Repo
	routeName string
	mode      changesViewMode

	changes []jjhub.Change

	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error
	showDiff     bool

	detailCache   map[string]*jjhub.Change
	detailErr     map[string]error
	detailLoading map[string]bool

	diffCache   map[string]string
	diffErr     map[string]error
	diffLoading map[string]bool

	statusLoading  bool
	statusText     string
	statusErr      error
	workingDiff    string
	workingDiffErr error

	actionMsg string
	prompt    changePromptState
}

// NewChangesView creates a JJHub changes view.
func NewChangesView(com *common.Common, _ *smithers.Client) *ChangesView {
	return newChangesViewWithClient(com, "changes", changesModeList, newJJHubChangeClient())
}

// NewStatusView creates a JJHub working-copy status view.
func NewStatusView(com *common.Common, _ *smithers.Client) *ChangesView {
	return newChangesViewWithClient(com, "status", changesModeStatus, newJJHubChangeClient())
}

func newJJHubChangeClient() changeManager {
	if !jjhubAvailable() {
		return nil
	}
	return jjhubChangeAdapter{client: jjhub.NewClient("")}
}

type jjhubChangeAdapter struct {
	client *jjhub.Client
}

func (a jjhubChangeAdapter) GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error) {
	return a.client.GetCurrentRepo(ctx)
}

func (a jjhubChangeAdapter) ListChanges(ctx context.Context, limit int) ([]jjhub.Change, error) {
	return a.client.ListChanges(ctx, limit)
}

func (a jjhubChangeAdapter) ViewChange(ctx context.Context, changeID string) (*jjhub.Change, error) {
	return a.client.ViewChange(ctx, changeID)
}

func (a jjhubChangeAdapter) ChangeDiff(ctx context.Context, changeID string) (string, error) {
	return a.client.ChangeDiff(ctx, changeID)
}

func (a jjhubChangeAdapter) Status(ctx context.Context) (string, error) {
	return a.client.Status(ctx)
}

func (a jjhubChangeAdapter) CreateBookmark(ctx context.Context, name, changeID string, remote bool) (*jjhub.Bookmark, error) {
	return a.client.CreateBookmark(ctx, name, changeID, remote)
}

func (a jjhubChangeAdapter) DeleteBookmark(ctx context.Context, name string, remote bool) error {
	return a.client.DeleteBookmark(ctx, name, remote)
}

func newChangesViewWithClient(args ...any) *ChangesView {
	com := packageCom
	var (
		routeName string
		mode      changesViewMode
		client    changeManager
	)
	switch len(args) {
	case 3:
		routeName, _ = args[0].(string)
		mode, _ = args[1].(changesViewMode)
		client, _ = args[2].(changeManager)
	case 4:
		if provided, ok := args[0].(*common.Common); ok && provided != nil {
			com = provided
		}
		routeName, _ = args[1].(string)
		mode, _ = args[2].(changesViewMode)
		client, _ = args[3].(changeManager)
	default:
		return nil
	}
	input := textinput.New()
	input.Placeholder = "Bookmark name"
	input.SetVirtualCursor(true)

	v := &ChangesView{
		com:           com,
		client:        client,
		routeName:     routeName,
		mode:          mode,
		loading:       client != nil && mode == changesModeList,
		statusLoading: client != nil && mode == changesModeStatus,
		detailCache:   make(map[string]*jjhub.Change),
		detailErr:     make(map[string]error),
		detailLoading: make(map[string]bool),
		diffCache:     make(map[string]string),
		diffErr:       make(map[string]error),
		diffLoading:   make(map[string]bool),
		prompt: changePromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

func (v *ChangesView) loadSelectedDetailCmd() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.changes) {
		return nil
	}
	changeID := v.changes[v.cursor].ChangeID
	if _, ok := v.detailCache[changeID]; ok {
		return nil
	}
	return v.loadChangeDetailCmd(changeID)
}

func (v *ChangesView) loadSelectedDiffCmd() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.changes) {
		return nil
	}
	changeID := v.changes[v.cursor].ChangeID
	if _, ok := v.diffCache[changeID]; ok {
		return nil
	}
	return v.loadChangeDiffCmd(changeID)
}

// Init loads repository metadata plus the initial mode's data.
func (v *ChangesView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}

	cmds := []tea.Cmd{v.loadRepoCmd()}
	if v.mode == changesModeStatus {
		cmds = append(cmds, v.loadWorkingCopyCmd())
	} else {
		cmds = append(cmds, v.loadChangesCmd())
	}
	return tea.Batch(cmds...)
}

func (v *ChangesView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
		if err != nil {
			return nil
		}
		return changeRepoLoadedMsg{repo: repo}
	}
}

func (v *ChangesView) loadChangesCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		changes, err := client.ListChanges(context.Background(), 50)
		if err != nil {
			return changesErrorMsg{err: err}
		}
		return changesLoadedMsg{changes: changes}
	}
}

func (v *ChangesView) loadChangeDetailCmd(changeID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		change, err := client.ViewChange(context.Background(), changeID)
		if err != nil {
			return changeDetailErrorMsg{changeID: changeID, err: err}
		}
		return changeDetailLoadedMsg{changeID: changeID, change: change}
	}
}

func (v *ChangesView) loadChangeDiffCmd(changeID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		diff, err := client.ChangeDiff(context.Background(), changeID)
		if err != nil {
			return changeDiffErrorMsg{changeID: changeID, err: err}
		}
		return changeDiffLoadedMsg{changeID: changeID, diff: diff}
	}
}

func (v *ChangesView) loadWorkingCopyCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		status, statusErr := client.Status(context.Background())
		diff, diffErr := client.ChangeDiff(context.Background(), "@")
		if diff == "" && diffErr == nil {
			diff, diffErr = client.ChangeDiff(context.Background(), "")
		}
		return workingCopyLoadedMsg{
			status:    status,
			statusErr: statusErr,
			diff:      diff,
			diffErr:   diffErr,
		}
	}
}

func (v *ChangesView) createBookmarkCmd(name, changeID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		_, err := client.CreateBookmark(context.Background(), name, changeID, true)
		if err != nil {
			return changeActionErrorMsg{err: err}
		}
		return changeActionDoneMsg{message: fmt.Sprintf("Created bookmark '%s'", name)}
	}
}

func (v *ChangesView) deleteBookmarkCmd(name string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		err := client.DeleteBookmark(context.Background(), name, true)
		if err != nil {
			return changeActionErrorMsg{err: err}
		}
		return changeActionDoneMsg{message: fmt.Sprintf("Deleted bookmark '%s'", name)}
	}
}

// Update handles messages for the changes view.
func (v *ChangesView) Update(msg tea.Msg) (View, tea.Cmd) {
	if v.prompt.active {
		return v.updatePrompt(msg)
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case changeRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case changesLoadedMsg:
		v.changes = msg.changes
		v.loading = false
		v.err = nil
		v.clampCursor()
		if len(v.changes) > 0 {
			cmds = append(cmds, v.onCursorMoved())
		}
		return v, nil

	case changesErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case changeDetailLoadedMsg:
		v.detailCache[msg.changeID] = msg.change
		v.detailLoading[msg.changeID] = false
		return v, nil

	case changeDetailErrorMsg:
		v.detailErr[msg.changeID] = msg.err
		v.detailLoading[msg.changeID] = false
		return v, nil

	case changeDiffLoadedMsg:
		v.diffCache[msg.changeID] = msg.diff
		v.diffLoading[msg.changeID] = false
		return v, nil

	case changeDiffErrorMsg:
		v.diffErr[msg.changeID] = msg.err
		v.diffLoading[msg.changeID] = false
		return v, nil

	case changeStatusLoadedMsg:
		v.statusText = msg.status
		v.statusLoading = false
		v.statusErr = nil
		return v, nil

	case changeStatusErrorMsg:
		v.statusErr = msg.err
		v.statusLoading = false
		return v, nil

	case workingDiffLoadedMsg:
		v.workingDiff = msg.diff
		v.workingDiffErr = nil
		return v, nil

	case workingCopyLoadedMsg:
		v.statusText = msg.status
		v.statusErr = msg.statusErr
		v.workingDiff = msg.diff
		v.workingDiffErr = msg.diffErr
		v.statusLoading = false
		return v, nil

	case workingDiffErrorMsg:
		v.workingDiffErr = msg.err
		return v, nil

	case changeActionDoneMsg:
		v.actionMsg = msg.message
		// Refresh data
		if v.mode == changesModeStatus {
			cmds = append(cmds, v.loadWorkingCopyCmd())
		} else {
			cmds = append(cmds, v.loadChangesCmd())
		}
		return v, tea.Batch(cmds...)

	case changeActionErrorMsg:
		v.err = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.mode == changesModeList && v.cursor > 0 {
				v.cursor--
				v.actionMsg = ""
				cmds = append(cmds, v.onCursorMoved())
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.mode == changesModeList && v.cursor < len(v.changes)-1 {
				v.cursor++
				v.actionMsg = ""
				cmds = append(cmds, v.onCursorMoved())
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.actionMsg = ""
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if v.mode == changesModeList {
				v.mode = changesModeStatus
				v.statusLoading = true
				return v, v.loadWorkingCopyCmd()
			}
			v.mode = changesModeList
			v.loading = true
			return v, v.loadChangesCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			v.showDiff = !v.showDiff
			if v.showDiff {
				return v, v.loadSelectedDiffCmd()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("b"))):
			if v.mode == changesModeList && len(v.changes) > 0 {
				v.prompt.active = true
				v.prompt.kind = changePromptCreateBookmark
				v.prompt.input.Reset()
				v.prompt.input.Focus()
				return v, nil
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			if v.mode == changesModeList && len(v.changes) > 0 && len(v.changes[v.cursor].Bookmarks) > 0 {
				v.prompt.active = true
				v.prompt.kind = changePromptDeleteBookmark
				v.prompt.input.Reset()
				v.prompt.input.SetValue(v.changes[v.cursor].Bookmarks[0])
				v.prompt.input.Focus()
				return v, nil
			}
		}
	}

	return v, tea.Batch(cmds...)
}

func (v *ChangesView) updatePrompt(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			v.prompt.active = false
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			name := strings.TrimSpace(v.prompt.input.Value())
			if name == "" {
				return v, nil
			}
			v.prompt.active = false
			if v.prompt.kind == changePromptDeleteBookmark {
				return v, v.deleteBookmarkCmd(name)
			}
			changeID := v.changes[v.cursor].ChangeID
			return v, v.createBookmarkCmd(name, changeID)
		}
	}

	var cmd tea.Cmd
	v.prompt.input, cmd = v.prompt.input.Update(msg)
	return v, cmd
}

func (v *ChangesView) onCursorMoved() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.changes) {
		return nil
	}
	changeID := v.changes[v.cursor].ChangeID
	var cmds []tea.Cmd
	if _, ok := v.detailCache[changeID]; !ok && !v.detailLoading[changeID] {
		v.detailLoading[changeID] = true
		cmds = append(cmds, v.loadChangeDetailCmd(changeID))
	}
	if _, ok := v.diffCache[changeID]; !ok && !v.diffLoading[changeID] {
		v.diffLoading[changeID] = true
		cmds = append(cmds, v.loadChangeDiffCmd(changeID))
	}
	return tea.Batch(cmds...)
}

func (v *ChangesView) clampCursor() {
	if len(v.changes) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor >= len(v.changes) {
		v.cursor = len(v.changes) - 1
	}
}

// View renders the changes/status layout.
func (v *ChangesView) View() string {
	if v.mode == changesModeStatus {
		return v.renderStatus()
	}
	return v.renderList()
}

func (v *ChangesView) renderList() string {
	var b strings.Builder
	t := v.com.Styles

	// Header
	header := jjhubHeader(t, "CODEPLANE › JJHub Changes", v.width, jjhubRepoLabel(t, v.repo))
	b.WriteString(header + "\n\n")

	if v.loading && len(v.changes) == 0 {
		b.WriteString("  Loading changes...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(v.com.Styles.JJHub.Error.Render(fmt.Sprintf("  Error: %v", v.err)) + "\n")
		return b.String()
	}

	if len(v.changes) == 0 {
		b.WriteString("  " + v.com.Styles.JJHub.Muted.Render("No recent changes found.") + "\n")
		return b.String()
	}

	if v.actionMsg != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(t.Green).Render("  "+v.actionMsg) + "\n\n")
	}

	// Two-column layout: left = list, right = detail/diff
	leftWidth := 40
	rightWidth := v.width - leftWidth - 3

	var leftLines []string
	for i, c := range v.changes {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			style = style.Bold(true)
		}
		changeLine := cursor + style.Render(truncate(c.ChangeID, 8)) + " " + style.Render(truncate(c.Description, 20))
		leftLines = append(leftLines, changeLine)
	}

	var rightLines []string
	if v.cursor >= 0 && v.cursor < len(v.changes) {
		c := v.changes[v.cursor]
		if v.showDiff {
			rightLines = v.renderDiff(c.ChangeID, rightWidth)
		} else {
			rightLines = v.renderDetail(c.ChangeID, rightWidth)
		}
	}

	// Render panes
	maxRows := len(leftLines)
	if len(rightLines) > maxRows {
		maxRows = len(rightLines)
	}

	leftPaneStyle := lipgloss.NewStyle().Width(leftWidth).Height(maxRows)
	divider := lipgloss.NewStyle().Foreground(t.BorderColor).Render(" │ ")

	for i := 0; i < maxRows; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		b.WriteString(leftPaneStyle.Render(left) + divider + right + "\n")
	}

	if v.prompt.active {
		b.WriteString("\n" + v.prompt.input.View() + "\n")
	}

	return b.String()
}

func (v *ChangesView) renderDetail(changeID string, width int) []string {
	t := v.com.Styles
	c, ok := v.detailCache[changeID]
	if !ok {
		if v.detailLoading[changeID] {
			return []string{"Loading details..."}
		}
		return []string{"-"}
	}

	var lines []string
	lines = append(lines, t.JJHub.Title.Render(truncate(c.Description, width)))
	lines = append(lines, "")
	lines = append(lines, jjhubMetaRow(t, "ID:", c.ChangeID))
	author := c.Author.Name
	if strings.TrimSpace(author) == "" {
		author = c.Author.Email
	}
	lines = append(lines, jjhubMetaRow(t, "Author:", author))
	lines = append(lines, jjhubMetaRow(t, "Date:", jjRelativeTime(c.Timestamp)))
	lines = append(lines, jjhubMetaRow(t, "Bookmarks:", formatBookmarksLabel(*c)))

	return lines
}

func (v *ChangesView) renderDiff(changeID string, width int) []string {
	diff, ok := v.diffCache[changeID]
	if !ok {
		if v.diffLoading[changeID] {
			return []string{"Loading diff..."}
		}
		return []string{"-"}
	}
	if diff == "" {
		return []string{"(no changes)"}
	}
	return wrapLineToWidth(diff, width)
}

func (v *ChangesView) renderStatus() string {
	var b strings.Builder
	t := v.com.Styles
	header := jjhubHeader(t, "CODEPLANE › JJHub Status", v.width, jjhubRepoLabel(t, v.repo))
	b.WriteString(header + "\n\n")

	if v.statusLoading {
		b.WriteString("  Loading status...\n")
		return b.String()
	}

	if v.statusErr != nil {
		b.WriteString(t.JJHub.Error.Render(fmt.Sprintf("  Error: %v", v.statusErr)) + "\n")
	} else if v.statusText != "" {
		b.WriteString("  " + t.JJHub.Section.Render("Working Copy Status") + "\n")
		b.WriteString(v.statusText + "\n\n")
	}

	if v.workingDiff != "" {
		b.WriteString("  " + t.JJHub.Section.Render("Uncommitted Changes") + "\n")
		b.WriteString(v.workingDiff + "\n")
	} else if v.workingDiffErr != nil {
		b.WriteString(t.JJHub.Error.Render(fmt.Sprintf("  Diff error: %v", v.workingDiffErr)) + "\n")
	} else {
		b.WriteString("  " + v.com.Styles.JJHub.Muted.Render("Clean working copy.") + "\n")
	}

	return b.String()
}

func (v *ChangesView) Name() string {
	return v.routeName
}

func (v *ChangesView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *ChangesView) ShortHelp() []key.Binding {
	kb := []key.Binding{
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "toggle diff")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
	if v.mode == changesModeList {
		kb = append([]key.Binding{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191\u2193", "navigate")),
			key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bookmark")),
		}, kb...)
	}
	return kb
}

func formatBookmarksLabel(change jjhub.Change) string {
	if len(change.Bookmarks) == 0 {
		return "-"
	}
	return strings.Join(change.Bookmarks, ", ")
}

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

var _ View = (*ChangesView)(nil)

type changeManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	ListChanges(ctx context.Context, limit int) ([]jjhub.Change, error)
	ViewChange(ctx context.Context, changeID string) (*jjhub.Change, error)
	ChangeDiff(ctx context.Context, changeID string) (string, error)
	Status(ctx context.Context) (string, error)
	CreateBookmark(ctx context.Context, name, changeID string, remote bool) (*jjhub.Bookmark, error)
	DeleteBookmark(ctx context.Context, name string, remote bool) error
}

type changesViewMode uint8

const (
	changesModeList changesViewMode = iota
	changesModeStatus
)

type changesLoadedMsg struct {
	changes []jjhub.Change
}

type changesErrorMsg struct {
	err error
}

type changeRepoLoadedMsg struct {
	repo *jjhub.Repo
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

type workingCopyLoadedMsg struct {
	status    string
	statusErr error
	diff      string
	diffErr   error
}

type changePromptKind uint8

const (
	changePromptCreateBookmark changePromptKind = iota
	changePromptDeleteBookmark
)

type changePromptState struct {
	active bool
	kind   changePromptKind
	input  textinput.Model
	err    error
}

type changeActionDoneMsg struct {
	message string
}

type changeActionErrorMsg struct {
	err error
}

// ChangesView displays JJHub recent changes and working copy status.
type ChangesView struct {
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
func NewChangesView(_ *smithers.Client) *ChangesView {
	return newChangesViewWithClient("changes", changesModeList, newJJHubChangeClient())
}

// NewStatusView creates a JJHub working-copy status view.
func NewStatusView(_ *smithers.Client) *ChangesView {
	return newChangesViewWithClient("status", changesModeStatus, newJJHubChangeClient())
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

func newChangesViewWithClient(routeName string, mode changesViewMode, client changeManager) *ChangesView {
	input := textinput.New()
	input.Placeholder = "Bookmark name"
	input.SetVirtualCursor(true)

	v := &ChangesView{
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
		changes, err := client.ListChanges(context.Background(), 100)
		if err != nil {
			return changesErrorMsg{err: err}
		}
		return changesLoadedMsg{changes: changes}
	}
}

func (v *ChangesView) loadSelectedDetailCmd() tea.Cmd {
	change := v.selectedChange()
	if change == nil || v.client == nil {
		return nil
	}

	key := changeCacheKey(*change)
	if v.detailCache[key] != nil || v.detailLoading[key] {
		return nil
	}
	delete(v.detailErr, key)
	v.detailLoading[key] = true

	changeID := change.ChangeID
	client := v.client
	return func() tea.Msg {
		loaded, err := client.ViewChange(context.Background(), changeID)
		if err != nil {
			return changeDetailErrorMsg{changeID: key, err: err}
		}
		return changeDetailLoadedMsg{changeID: key, change: loaded}
	}
}

func (v *ChangesView) loadSelectedDiffCmd() tea.Cmd {
	change := v.selectedChange()
	if change == nil || v.client == nil {
		return nil
	}

	key := changeCacheKey(*change)
	if _, ok := v.diffCache[key]; ok || v.diffLoading[key] {
		return nil
	}
	delete(v.diffErr, key)
	v.diffLoading[key] = true

	changeID := change.ChangeID
	client := v.client
	return func() tea.Msg {
		diff, err := client.ChangeDiff(context.Background(), changeID)
		if err != nil {
			return changeDiffErrorMsg{changeID: key, err: err}
		}
		return changeDiffLoadedMsg{changeID: key, diff: diff}
	}
}

func (v *ChangesView) loadWorkingCopyCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		status, statusErr := client.Status(context.Background())
		diff, diffErr := client.ChangeDiff(context.Background(), "")
		return workingCopyLoadedMsg{
			status:    status,
			statusErr: statusErr,
			diff:      diff,
			diffErr:   diffErr,
		}
	}
}

func (v *ChangesView) refreshCurrentModeCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}

	if v.mode == changesModeStatus {
		v.statusLoading = true
		return v.loadWorkingCopyCmd()
	}

	v.loading = true
	v.err = nil
	return tea.Batch(v.loadChangesCmd(), v.loadRepoCmd())
}

func (v *ChangesView) selectedChange() *jjhub.Change {
	if len(v.changes) == 0 || v.cursor < 0 || v.cursor >= len(v.changes) {
		return nil
	}
	change := v.changes[v.cursor]
	return &change
}

func (v *ChangesView) pageSize() int {
	const (
		headerLines  = 6
		linesPerItem = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / linesPerItem
	if size < 1 {
		return 1
	}
	return size
}

func (v *ChangesView) clampCursor() {
	if len(v.changes) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(v.changes) {
		v.cursor = len(v.changes) - 1
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

func (v *ChangesView) switchMode(mode changesViewMode) tea.Cmd {
	v.mode = mode
	if mode == changesModeStatus {
		if v.statusText == "" && v.statusErr == nil && v.workingDiff == "" && v.workingDiffErr == nil {
			v.statusLoading = true
			return v.loadWorkingCopyCmd()
		}
		return nil
	}

	if len(v.changes) == 0 && v.err == nil {
		v.loading = true
		return v.loadChangesCmd()
	}
	return nil
}

func (v *ChangesView) selectedModeCmd() tea.Cmd {
	if v.showDiff {
		return v.loadSelectedDiffCmd()
	}
	return v.loadSelectedDetailCmd()
}

func (v *ChangesView) createBookmarkCmd(name string) tea.Cmd {
	client := v.client
	change := v.selectedChange()
	if change == nil {
		return nil
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return func() tea.Msg {
			return changeActionErrorMsg{err: errors.New("Bookmark name must not be empty")}
		}
	}

	changeID := ""
	remote := !change.IsWorkingCopy && strings.TrimSpace(change.ChangeID) != ""
	if remote {
		changeID = change.ChangeID
	}

	return func() tea.Msg {
		bookmark, err := client.CreateBookmark(context.Background(), name, changeID, remote)
		if err != nil {
			return changeActionErrorMsg{err: err}
		}
		if bookmark != nil && strings.TrimSpace(bookmark.Name) != "" {
			name = bookmark.Name
		}
		return changeActionDoneMsg{message: fmt.Sprintf("Created bookmark %s", name)}
	}
}

func (v *ChangesView) deleteBookmarkCmd(name string) tea.Cmd {
	client := v.client
	change := v.selectedChange()
	if change == nil {
		return nil
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return func() tea.Msg {
			return changeActionErrorMsg{err: errors.New("Bookmark name must not be empty")}
		}
	}

	remote := !change.IsWorkingCopy && strings.TrimSpace(change.ChangeID) != ""
	return func() tea.Msg {
		if err := client.DeleteBookmark(context.Background(), name, remote); err != nil {
			return changeActionErrorMsg{err: err}
		}
		return changeActionDoneMsg{message: fmt.Sprintf("Deleted bookmark %s", name)}
	}
}

func (v *ChangesView) openPrompt(kind changePromptKind, placeholder, value string) tea.Cmd {
	v.prompt.active = true
	v.prompt.kind = kind
	v.prompt.err = nil
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = placeholder
	v.prompt.input.SetValue(value)
	return v.prompt.input.Focus()
}

func (v *ChangesView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *ChangesView) submitPrompt() tea.Cmd {
	value := strings.TrimSpace(v.prompt.input.Value())
	switch v.prompt.kind {
	case changePromptCreateBookmark:
		return v.createBookmarkCmd(value)
	case changePromptDeleteBookmark:
		return v.deleteBookmarkCmd(value)
	default:
		return nil
	}
}

func (v *ChangesView) renderPrompt(width int) string {
	boxWidth := max(32, min(max(32, width-4), 80))
	title := "Create bookmark"
	description := "Create a bookmark on the selected change."
	if v.prompt.kind == changePromptDeleteBookmark {
		title = "Delete bookmark"
		description = "Delete a bookmark from the selected change."
	}

	var body strings.Builder
	body.WriteString(jjhubSectionStyle.Render(title))
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render(description))
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

// Update handles messages for the changes view.
func (v *ChangesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case changesLoadedMsg:
		v.changes = msg.changes
		v.loading = false
		v.err = nil
		v.clampCursor()
		return v, v.loadSelectedDetailCmd()

	case changesErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case changeRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case changeDetailLoadedMsg:
		v.detailLoading[msg.changeID] = false
		v.detailCache[msg.changeID] = msg.change
		delete(v.detailErr, msg.changeID)
		return v, nil

	case changeDetailErrorMsg:
		v.detailLoading[msg.changeID] = false
		v.detailErr[msg.changeID] = msg.err
		return v, nil

	case changeDiffLoadedMsg:
		v.diffLoading[msg.changeID] = false
		v.diffCache[msg.changeID] = msg.diff
		delete(v.diffErr, msg.changeID)
		return v, nil

	case changeDiffErrorMsg:
		v.diffLoading[msg.changeID] = false
		v.diffErr[msg.changeID] = msg.err
		return v, nil

	case workingCopyLoadedMsg:
		v.statusLoading = false
		v.statusText = msg.status
		v.statusErr = msg.statusErr
		v.workingDiff = msg.diff
		v.workingDiffErr = msg.diffErr
		return v, nil

	case changeActionDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		return v, v.refreshCurrentModeCmd()

	case changeActionErrorMsg:
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

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if v.mode == changesModeStatus {
				return v, v.switchMode(changesModeList)
			}
			return v, v.switchMode(changesModeStatus)

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			return v, v.refreshCurrentModeCmd()
		}

		if v.mode == changesModeStatus {
			return v, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
				v.clampCursor()
				return v, v.selectedModeCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.changes)-1 {
				v.cursor++
				v.clampCursor()
				return v, v.selectedModeCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if v.selectedChange() == nil {
				return v, nil
			}
			v.showDiff = !v.showDiff
			if v.showDiff {
				return v, v.loadSelectedDiffCmd()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("b"))):
			if v.selectedChange() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openPrompt(changePromptCreateBookmark, "Bookmark name", "")

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			change := v.selectedChange()
			if change == nil || len(change.Bookmarks) == 0 {
				return v, nil
			}
			v.actionMsg = ""
			value := ""
			if len(change.Bookmarks) == 1 {
				value = change.Bookmarks[0]
			}
			return v, v.openPrompt(changePromptDeleteBookmark, "Bookmark name", value)
		}
	}

	return v, nil
}

// View renders the changes view.
func (v *ChangesView) View() string {
	title := "JJHUB › Changes"
	modeLabel := "[changes]"
	if v.mode == changesModeStatus {
		title = "JJHUB › Status"
		modeLabel = "[status]"
	}

	var b strings.Builder
	b.WriteString(jjhubHeader(title, v.width, jjhubJoinNonEmpty("  ",
		lipgloss.NewStyle().Faint(true).Render(modeLabel),
		jjhubRepoLabel(v.repo),
		lipgloss.NewStyle().Faint(true).Render("[Esc] Back"),
	)))
	b.WriteString("\n\n")

	if v.prompt.active {
		b.WriteString(v.renderPrompt(v.width))
		b.WriteString("\n\n")
	} else if strings.TrimSpace(v.actionMsg) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  "+v.actionMsg) + "\n\n")
	}

	if v.client == nil && v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}

	if v.mode == changesModeStatus {
		b.WriteString(v.renderStatusMode(v.width))
		return b.String()
	}

	if v.loading {
		b.WriteString("  Loading changes...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.changes) == 0 {
		b.WriteString("  No changes found.\n")
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderChangeList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderChangeDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderChangeList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderChangeDetail(v.width))
	return b.String()
}

func (v *ChangesView) renderStatusMode(width int) string {
	var b strings.Builder

	if v.statusLoading {
		b.WriteString("  Loading working copy status...\n")
		return b.String()
	}

	b.WriteString(jjhubSectionStyle.Render("Status"))
	b.WriteString("\n")
	switch {
	case v.statusErr != nil:
		b.WriteString(jjhubErrorStyle.Render(v.statusErr.Error()))
	case strings.TrimSpace(v.statusText) == "":
		b.WriteString(jjhubMutedStyle.Render("No working copy status available."))
	default:
		clipped, truncated := jjhubClipLines(v.statusText, max(6, v.height/3))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Diff"))
	b.WriteString("\n")
	switch {
	case v.workingDiffErr != nil:
		b.WriteString(jjhubErrorStyle.Render(v.workingDiffErr.Error()))
	case strings.TrimSpace(v.workingDiff) == "":
		b.WriteString(jjhubMutedStyle.Render("No working copy diff available."))
	default:
		clipped, truncated := jjhubClipLines(v.workingDiff, max(10, v.height-12))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubMutedStyle.Render("[Tab] changes  [r] refresh"))
	return b.String()
}

func (v *ChangesView) renderChangeList(width int) string {
	var b strings.Builder
	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(v.changes)-1))
	end := min(len(v.changes), start+pageSize)

	for i := start; i < end; i++ {
		change := v.changes[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		b.WriteString(cursor + titleStyle.Render(truncateStr(changeListTitle(change), max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			changeSecondaryLabel(change),
			changeBookmarksLabel(change),
			jjhubFormatRelativeTime(change.Timestamp),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(v.changes) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(
			fmt.Sprintf("… %d more change%s", len(v.changes)-end, pluralS(len(v.changes)-end)),
		))
	}

	return b.String()
}

func (v *ChangesView) renderChangeDetail(width int) string {
	change := v.selectedChange()
	if change == nil {
		return "No change selected."
	}
	if v.showDiff {
		return v.renderChangeDiff(*change)
	}

	key := changeCacheKey(*change)
	detail := v.detailCache[key]
	if detail == nil {
		detail = change
	}

	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(changeListTitle(*detail)))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("Change", detail.ChangeID))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Commit", detail.CommitID))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Author", strings.TrimSpace(detail.Author.Name)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Email", strings.TrimSpace(detail.Author.Email)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("When", jjhubFormatTimestamp(detail.Timestamp)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Bookmarks", changeBookmarksLabel(*detail)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Working Copy", boolLabel(detail.IsWorkingCopy)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Empty", boolLabel(detail.IsEmpty)))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Description"))
	b.WriteString("\n")
	description := strings.TrimSpace(detail.Description)
	if description == "" {
		b.WriteString(jjhubMutedStyle.Render("No description provided."))
	} else {
		clipped, truncated := jjhubClipLines(wrapText(description, max(20, width)), max(8, v.height-12))
		b.WriteString(clipped)
		if truncated {
			b.WriteString("\n")
			b.WriteString(jjhubMutedStyle.Render("…"))
		}
	}

	if v.detailLoading[key] {
		b.WriteString("\n\n")
		b.WriteString(jjhubMutedStyle.Render("Loading change details..."))
	}
	if v.detailErr[key] != nil {
		b.WriteString("\n\n")
		b.WriteString(jjhubErrorStyle.Render("Detail error: " + v.detailErr[key].Error()))
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Actions"))
	b.WriteString("\n")
	b.WriteString(wrapText("[d] diff  [b] create bookmark  [x] delete bookmark  [Tab] status  [r] refresh", max(20, width)))
	return b.String()
}

func (v *ChangesView) renderChangeDiff(change jjhub.Change) string {
	key := changeCacheKey(change)

	var b strings.Builder
	title := change.ChangeID
	if strings.TrimSpace(title) == "" {
		title = key
	}
	b.WriteString(jjhubTitleStyle.Render(fmt.Sprintf("%s diff", title)))
	b.WriteString("\n\n")

	switch {
	case v.diffLoading[key]:
		b.WriteString(jjhubMutedStyle.Render("Loading diff..."))
	case v.diffErr[key] != nil:
		b.WriteString(jjhubErrorStyle.Render("Diff error: " + v.diffErr[key].Error()))
	default:
		diff := v.diffCache[key]
		if diff == "" {
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

// Name returns the view name.
func (v *ChangesView) Name() string { return v.routeName }

// SetSize stores the terminal size.
func (v *ChangesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

// ShortHelp returns the contextual help bindings.
func (v *ChangesView) ShortHelp() []key.Binding {
	if v.mode == changesModeStatus {
		return []key.Binding{
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "changes")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}

	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bookmark")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete bookmark")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "status")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func changeCacheKey(change jjhub.Change) string {
	if strings.TrimSpace(change.ChangeID) != "" {
		return change.ChangeID
	}
	return change.CommitID
}

func changeListTitle(change jjhub.Change) string {
	title := strings.TrimSpace(change.Description)
	if title == "" {
		title = "(no description)"
	}
	if strings.TrimSpace(change.ChangeID) == "" {
		return title
	}
	return fmt.Sprintf("%s %s", change.ChangeID, title)
}

func changeSecondaryLabel(change jjhub.Change) string {
	if change.IsWorkingCopy {
		return "working copy"
	}
	if strings.TrimSpace(change.Author.Name) != "" {
		return change.Author.Name
	}
	return change.CommitID
}

func changeBookmarksLabel(change jjhub.Change) string {
	if len(change.Bookmarks) == 0 {
		return "-"
	}
	return strings.Join(change.Bookmarks, ", ")
}

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

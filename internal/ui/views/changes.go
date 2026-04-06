package views

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/diffnav"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// Compile-time interface check.
var _ View = (*ChangesView)(nil)

const changesDiffTag = "changes-diffnav"

type changesClient interface {
	ListChanges(limit int) ([]jjhub.Change, error)
}

type diffnavLauncher func(command string, cwd string, tag any) tea.Cmd

type changesLoadedMsg struct {
	changes []jjhub.Change
}

type changesErrorMsg struct {
	err error
}

type changeListPane struct {
	changes      []jjhub.Change
	cursor       int
	scrollOffset int
	width        int
	height       int
}

func (p *changeListPane) Init() tea.Cmd { return nil }

func (p *changeListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		if p.cursor > 0 {
			p.cursor--
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		if p.cursor < len(p.changes)-1 {
			p.cursor++
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("home", "g"))):
		p.cursor = 0
		p.scrollOffset = 0
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("end", "G"))):
		if len(p.changes) > 0 {
			p.cursor = len(p.changes) - 1
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
		p.cursor -= p.pageSize()
		if p.cursor < 0 {
			p.cursor = 0
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
		p.cursor += p.pageSize()
		if len(p.changes) > 0 && p.cursor >= len(p.changes) {
			p.cursor = len(p.changes) - 1
		}
	}

	p.clampCursor()
	p.ensureCursorVisible()
	return p, nil
}

func (p *changeListPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.ensureCursorVisible()
}

func (p *changeListPane) setChanges(changes []jjhub.Change) {
	p.changes = changes
	p.clampCursor()
	p.ensureCursorVisible()
}

func (p *changeListPane) setCursor(cursor int) {
	p.cursor = cursor
	p.clampCursor()
	p.ensureCursorVisible()
}

func (p *changeListPane) clampCursor() {
	if len(p.changes) == 0 {
		p.cursor = 0
		p.scrollOffset = 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.changes) {
		p.cursor = len(p.changes) - 1
	}
}

func (p *changeListPane) visibleRows() int {
	if p.height <= 2 {
		return 1
	}
	return p.height - 2
}

func (p *changeListPane) pageSize() int {
	pageSize := p.visibleRows()
	if pageSize < 1 {
		return 1
	}
	return pageSize
}

func (p *changeListPane) ensureCursorVisible() {
	if len(p.changes) == 0 {
		p.scrollOffset = 0
		return
	}

	visibleRows := p.visibleRows()
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	}
	if p.cursor >= p.scrollOffset+visibleRows {
		p.scrollOffset = p.cursor - visibleRows + 1
	}
	maxOffset := max(0, len(p.changes)-visibleRows)
	if p.scrollOffset > maxOffset {
		p.scrollOffset = maxOffset
	}
}

func (p *changeListPane) View() string {
	if len(p.changes) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("No changes found.")
	}

	visibleColumns := changeTableColumns(p.width)
	var b strings.Builder

	headerCells := make([]string, 0, len(visibleColumns))
	for _, col := range visibleColumns {
		headerCells = append(headerCells, lipgloss.NewStyle().Bold(true).Faint(true).Render(padChangeCell(col.Title, col.Width)))
	}
	b.WriteString("  " + strings.Join(headerCells, " ") + "\n")

	visibleRows := p.visibleRows()
	p.ensureCursorVisible()
	end := min(len(p.changes), p.scrollOffset+visibleRows)

	for i := p.scrollOffset; i < end; i++ {
		change := p.changes[i]
		cells := make([]string, 0, len(visibleColumns))
		for _, col := range visibleColumns {
			cells = append(cells, padChangeCell(changeColumnValue(change, col.Title), col.Width))
		}

		indicator := "  "
		if i == p.cursor {
			indicator = "▸ "
		}

		line := indicator + strings.Join(cells, " ")
		rowStyle := lipgloss.NewStyle()
		if change.IsEmpty {
			rowStyle = rowStyle.Faint(true)
		}
		if i == p.cursor {
			rowStyle = rowStyle.Bold(true).Background(lipgloss.Color("238"))
		} else if i%2 == 1 {
			rowStyle = rowStyle.Background(lipgloss.Color("236"))
		}

		b.WriteString(rowStyle.Width(max(0, p.width)).Render(line))
		b.WriteString("\n")
	}

	scroll := fmt.Sprintf("%d/%d", p.cursor+1, len(p.changes))
	b.WriteString(lipgloss.NewStyle().
		Width(max(0, p.width)).
		Align(lipgloss.Right).
		Faint(true).
		Render(scroll))

	return b.String()
}

type changePreviewPane struct {
	changes      []jjhub.Change
	cursor       *int
	scrollOffset int
	width        int
	height       int
}

func (p *changePreviewPane) Init() tea.Cmd { return nil }

func (p *changePreviewPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		if p.scrollOffset > 0 {
			p.scrollOffset--
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		if p.scrollOffset < p.maxScrollOffset() {
			p.scrollOffset++
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("home", "g"))):
		p.scrollOffset = 0
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("end", "G"))):
		p.scrollOffset = p.maxScrollOffset()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
		p.scrollOffset -= p.pageSize()
		if p.scrollOffset < 0 {
			p.scrollOffset = 0
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
		p.scrollOffset += p.pageSize()
		if p.scrollOffset > p.maxScrollOffset() {
			p.scrollOffset = p.maxScrollOffset()
		}
	}

	return p, nil
}

func (p *changePreviewPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.clampScroll()
}

func (p *changePreviewPane) setChanges(changes []jjhub.Change) {
	p.changes = changes
	p.scrollOffset = 0
}

func (p *changePreviewPane) resetScroll() {
	p.scrollOffset = 0
}

func (p *changePreviewPane) visibleRows() int {
	if p.height <= 1 {
		return 1
	}
	return p.height - 1
}

func (p *changePreviewPane) pageSize() int {
	pageSize := p.visibleRows()
	if pageSize < 1 {
		return 1
	}
	return pageSize
}

func (p *changePreviewPane) maxScrollOffset() int {
	return max(0, len(p.previewLines())-p.visibleRows())
}

func (p *changePreviewPane) clampScroll() {
	maxOffset := p.maxScrollOffset()
	if p.scrollOffset > maxOffset {
		p.scrollOffset = maxOffset
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
}

func (p *changePreviewPane) previewLines() []string {
	if p.cursor == nil || *p.cursor < 0 || *p.cursor >= len(p.changes) {
		return []string{lipgloss.NewStyle().Faint(true).Render("Select a change.")}
	}

	change := p.changes[*p.cursor]
	lines := make([]string, 0, 16)

	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Bold(true).Faint(true)

	lines = append(lines, titleStyle.Render("Change"))
	lines = append(lines, renderPreviewField(labelStyle, "Change ID", change.ChangeID, p.width)...)
	lines = append(lines, renderPreviewField(labelStyle, "Commit ID", change.CommitID, p.width)...)
	lines = append(lines, renderPreviewField(labelStyle, "Author", formatChangeAuthorFull(change.Author), p.width)...)
	lines = append(lines, renderPreviewField(labelStyle, "Timestamp", formatChangeTimestampFull(change.Timestamp), p.width)...)
	lines = append(lines, labelStyle.Render("Bookmarks"))
	if len(change.Bookmarks) == 0 {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("none"))
	} else {
		lines = append(lines, renderPreviewTags(change.Bookmarks, p.width)...)
	}
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Description"))

	description := strings.TrimSpace(change.Description)
	if description == "" {
		description = "(empty)"
	}
	lines = append(lines, wrapPreviewText(description, max(1, p.width))...)

	return lines
}

func (p *changePreviewPane) View() string {
	lines := p.previewLines()
	p.clampScroll()

	start := p.scrollOffset
	end := min(len(lines), start+p.visibleRows())
	scroll := fmt.Sprintf("%d/%d", min(end, len(lines)), len(lines))

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end > start {
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.NewStyle().
		Width(max(0, p.width)).
		Align(lipgloss.Right).
		Faint(true).
		Render(scroll))

	return b.String()
}

// ChangesView displays JJHub changes in a navigable table with an optional
// sidebar preview and diffnav handoff.
type ChangesView struct {
	client           changesClient
	cwd              string
	width            int
	height           int
	loading          bool
	err              error
	statusMsg        string
	statusErr        bool
	changes          []jjhub.Change
	filteredChanges  []jjhub.Change
	searchActive     bool
	searchInput      textinput.Model
	previewVisible   bool
	diffnavAvailable func() bool
	launchDiffnav    diffnavLauncher
	splitPane        *components.SplitPane
	listPane         *changeListPane
	previewPane      *changePreviewPane
}

// NewChangesView creates a new JJHub changes browser.
func NewChangesView() *ChangesView {
	cwd, _ := os.Getwd()
	return newChangesView(jjhub.NewClient(""), cwd, func() bool { return true }, diffnav.LaunchDiffnavWithCommand)
}

func newChangesView(
	client changesClient,
	cwd string,
	diffnavAvailable func() bool,
	launchDiffnav diffnavLauncher,
) *ChangesView {
	listPane := &changeListPane{}
	previewPane := &changePreviewPane{cursor: &listPane.cursor}
	splitPane := components.NewSplitPane(listPane, previewPane, components.SplitPaneOpts{
		LeftWidth:         72,
		CompactBreakpoint: 100,
	})

	searchInput := textinput.New()
	searchInput.Placeholder = "search descriptions..."
	searchInput.Prompt = ""
	searchInput.SetVirtualCursor(true)

	return &ChangesView{
		client:           client,
		cwd:              cwd,
		loading:          true,
		searchInput:      searchInput,
		diffnavAvailable: diffnavAvailable,
		launchDiffnav:    launchDiffnav,
		splitPane:        splitPane,
		listPane:         listPane,
		previewPane:      previewPane,
	}
}

// Init fetches the initial change list.
func (v *ChangesView) Init() tea.Cmd {
	return v.fetchChangesCmd()
}

func (v *ChangesView) fetchChangesCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		if client == nil {
			return changesErrorMsg{err: fmt.Errorf("jjhub client is not configured")}
		}

		changes, err := client.ListChanges(100)
		if err != nil {
			return changesErrorMsg{err: err}
		}
		return changesLoadedMsg{changes: changes}
	}
}

// Update handles messages for the changes view.
func (v *ChangesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case changesLoadedMsg:
		v.loading = false
		v.err = nil
		v.changes = msg.changes
		v.applyFilter()
		v.syncLayout()
		return v, nil

	case changesErrorMsg:
		v.loading = false
		v.err = msg.err
		v.syncLayout()
		return v, nil

	case handoff.HandoffMsg:
		if msg.Tag != changesDiffTag {
			return v, nil
		}
		if msg.Result.Err != nil {
			v.statusMsg = fmt.Sprintf("Diffnav error: %v", msg.Result.Err)
			v.statusErr = true
			v.syncLayout()
		}
		return v, nil

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		if v.searchActive {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				if v.searchInput.Value() != "" {
					v.searchInput.Reset()
					v.applyFilter()
					return v, nil
				}

				v.searchActive = false
				v.searchInput.Blur()
				v.syncLayout()
				return v, nil

			default:
				prevQuery := v.searchInput.Value()
				var cmd tea.Cmd
				v.searchInput, cmd = v.searchInput.Update(msg)
				if v.searchInput.Value() != prevQuery {
					v.applyFilter()
				}
				return v, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			v.searchActive = true
			v.syncLayout()
			return v, v.searchInput.Focus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
			v.previewVisible = !v.previewVisible
			v.previewPane.resetScroll()
			if !v.previewVisible {
				v.splitPane.SetFocus(components.FocusLeft)
			}
			v.syncLayout()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			v.err = nil
			v.statusMsg = ""
			v.statusErr = false
			v.syncLayout()
			return v, v.fetchChangesCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "d"))):
			return v, v.launchSelectedDiff()
		}
	}

	oldCursor := v.listPane.cursor
	if v.previewVisible {
		newSplitPane, cmd := v.splitPane.Update(msg)
		v.splitPane = newSplitPane
		if v.listPane.cursor != oldCursor {
			v.previewPane.resetScroll()
		}
		return v, cmd
	}

	newListPane, cmd := v.listPane.Update(msg)
	v.listPane = newListPane.(*changeListPane)
	if v.listPane.cursor != oldCursor {
		v.previewPane.resetScroll()
	}
	return v, cmd
}

func (v *ChangesView) launchSelectedDiff() tea.Cmd {
	change, ok := v.selectedChange()
	if !ok {
		return nil
	}

	v.statusMsg = ""
	v.statusErr = false
	v.syncLayout()
	return v.launchDiffnav(buildChangeDiffCommand(change), v.cwd, changesDiffTag)
}

func (v *ChangesView) selectedChange() (jjhub.Change, bool) {
	if len(v.filteredChanges) == 0 || v.listPane.cursor >= len(v.filteredChanges) {
		return jjhub.Change{}, false
	}
	return v.filteredChanges[v.listPane.cursor], true
}

func (v *ChangesView) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(v.searchInput.Value()))
	selectedID := ""
	if selected, ok := v.selectedChange(); ok {
		selectedID = selected.ChangeID
	}

	filtered := make([]jjhub.Change, 0, len(v.changes))
	for _, change := range v.changes {
		if query != "" && !strings.Contains(strings.ToLower(change.Description), query) {
			continue
		}
		filtered = append(filtered, change)
	}

	v.filteredChanges = filtered
	v.listPane.setChanges(filtered)
	v.previewPane.setChanges(filtered)

	if selectedID == "" {
		v.listPane.setCursor(0)
		return
	}

	for i, change := range filtered {
		if change.ChangeID == selectedID {
			v.listPane.setCursor(i)
			return
		}
	}

	v.listPane.setCursor(min(v.listPane.cursor, max(0, len(filtered)-1)))
}

func (v *ChangesView) filteredCountLabel() string {
	if !v.loading && v.searchInput.Value() != "" {
		return fmt.Sprintf("%d/%d", len(v.filteredChanges), len(v.changes))
	}
	if v.loading {
		return ""
	}
	return fmt.Sprintf("%d", len(v.filteredChanges))
}

func (v *ChangesView) chromeHeight() int {
	height := 2
	if v.searchActive {
		height += 2
	}
	if v.statusMsg != "" {
		height++
	}
	return height
}

func (v *ChangesView) syncLayout() {
	contentHeight := max(0, v.height-v.chromeHeight())
	v.searchInput.SetWidth(max(10, v.width-4))

	if v.previewVisible {
		v.splitPane.SetSize(v.width, contentHeight)
		return
	}

	v.listPane.SetSize(v.width, contentHeight)
}

// View renders the changes browser.
func (v *ChangesView) View() string {
	var b strings.Builder

	title := "JJHub › Changes"
	if count := v.filteredCountLabel(); count != "" {
		title = fmt.Sprintf("%s (%s)", title, count)
	}

	header := lipgloss.NewStyle().Bold(true).Render(title)
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint)
		if gap > 1 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		} else {
			headerLine = header + " " + helpHint
		}
	}

	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.searchActive {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("/") + " " + v.searchInput.View())
		b.WriteString("\n\n")
	}

	if v.statusMsg != "" {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		if v.statusErr {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		}
		b.WriteString(style.Render("  " + v.statusMsg))
		b.WriteString("\n")
	}

	if v.loading {
		b.WriteString("  Loading changes...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	if len(v.filteredChanges) == 0 {
		if query := v.searchInput.Value(); query != "" {
			b.WriteString(fmt.Sprintf("  No changes matching %q.\n", query))
		} else {
			b.WriteString("  No changes found.\n")
		}
		return b.String()
	}

	if v.previewVisible {
		b.WriteString(v.splitPane.View())
		return b.String()
	}

	b.WriteString(v.listPane.View())
	return b.String()
}

// Name returns the route name.
func (v *ChangesView) Name() string { return "changes" }

// SetSize stores the current terminal size.
func (v *ChangesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.syncLayout()
}

// ShortHelp returns contextual key bindings for the help bar.
func (v *ChangesView) ShortHelp() []key.Binding {
	if v.searchActive {
		return []key.Binding{
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear/back")),
		}
	}

	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
		key.NewBinding(key.WithKeys("enter", "d"), key.WithHelp("enter/d", "diff")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "preview")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}

	if v.previewVisible {
		bindings = append(bindings, key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus pane")))
	}

	bindings = append(bindings, key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")))
	return bindings
}

type changeTableColumn struct {
	Title    string
	Width    int
	Grow     bool
	MinWidth int
}

func changeTableColumns(width int) []changeTableColumn {
	columns := []changeTableColumn{
		{Title: "Change ID", Width: 14},
		{Title: "Description", Grow: true},
		{Title: "Author", Width: 16, MinWidth: 68},
		{Title: "Bookmarks", Width: 18, MinWidth: 88},
		{Title: "Timestamp", Width: 10, MinWidth: 56},
	}

	visible := make([]changeTableColumn, 0, len(columns))
	for _, col := range columns {
		if col.MinWidth > 0 && width < col.MinWidth {
			continue
		}
		visible = append(visible, col)
	}

	fixedWidth := 2
	separatorWidth := max(0, len(visible)-1)
	growColumns := 0
	for _, col := range visible {
		if col.Grow {
			growColumns++
			continue
		}
		fixedWidth += col.Width
	}

	remaining := width - fixedWidth - separatorWidth
	if remaining < 12 {
		remaining = 12
	}
	if growColumns == 0 {
		return visible
	}

	perColumn := remaining / growColumns
	for i := range visible {
		if visible[i].Grow {
			visible[i].Width = perColumn
		}
	}
	return visible
}

func changeColumnValue(change jjhub.Change, column string) string {
	switch column {
	case "Change ID":
		marker := "  "
		if change.IsWorkingCopy {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(styles.ToolPending + " ")
		}
		return marker + shortChangeID(change.ChangeID)
	case "Description":
		return formatChangeDescription(change)
	case "Author":
		return formatChangeAuthor(change.Author)
	case "Bookmarks":
		return formatChangeBookmarks(change.Bookmarks)
	case "Timestamp":
		return formatChangeTimestampShort(change.Timestamp)
	default:
		return ""
	}
}

func shortChangeID(changeID string) string {
	runes := []rune(strings.TrimSpace(changeID))
	if len(runes) <= 12 {
		return string(runes)
	}
	return string(runes[:12])
}

func formatChangeDescription(change jjhub.Change) string {
	description := normalizeChangeText(change.Description)
	if change.IsEmpty {
		emptyLabel := lipgloss.NewStyle().Faint(true).Render("(empty)")
		if description == "" {
			return emptyLabel
		}
		return description + " " + emptyLabel
	}
	if description == "" {
		return lipgloss.NewStyle().Faint(true).Render("(no description)")
	}
	return description
}

func normalizeChangeText(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.Join(strings.Fields(text), " ")
}

func formatChangeAuthor(author jjhub.Author) string {
	switch {
	case author.Name != "":
		return author.Name
	case author.Email != "":
		return author.Email
	default:
		return "-"
	}
}

func formatChangeAuthorFull(author jjhub.Author) string {
	switch {
	case author.Name != "" && author.Email != "":
		return fmt.Sprintf("%s <%s>", author.Name, author.Email)
	case author.Name != "":
		return author.Name
	case author.Email != "":
		return author.Email
	default:
		return "-"
	}
}

func formatChangeBookmarks(bookmarks []string) string {
	if len(bookmarks) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("-")
	}
	return strings.Join(bookmarks, ", ")
}

func formatChangeTimestampShort(ts string) string {
	parsed, ok := parseChangeTimestamp(ts)
	if !ok {
		return truncateStr(ts, 10)
	}

	age := time.Since(parsed)
	switch {
	case age < 0:
		return "now"
	case age < time.Minute:
		return "now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	default:
		return parsed.Format("2006-01-02")
	}
}

func formatChangeTimestampFull(ts string) string {
	parsed, ok := parseChangeTimestamp(ts)
	if !ok {
		return ts
	}
	return parsed.Format(time.RFC3339)
}

func parseChangeTimestamp(ts string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, ts)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func renderPreviewField(labelStyle lipgloss.Style, label string, value string, width int) []string {
	lines := []string{labelStyle.Render(label)}
	lines = append(lines, wrapPreviewText(value, max(1, width))...)
	return lines
}

func renderPreviewTags(bookmarks []string, width int) []string {
	if len(bookmarks) == 0 {
		return []string{lipgloss.NewStyle().Faint(true).Render("none")}
	}

	tagStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	line := make([]string, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		line = append(line, tagStyle.Render(bookmark))
	}

	rendered := strings.Join(line, " ")
	if ansi.StringWidth(rendered) <= width {
		return []string{rendered}
	}

	lines := make([]string, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		lines = append(lines, tagStyle.Render(bookmark))
	}
	return lines
}

func wrapPreviewText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	paragraphs := strings.Split(text, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}

		words := strings.Fields(paragraph)
		current := words[0]
		for _, word := range words[1:] {
			candidate := current + " " + word
			if ansi.StringWidth(candidate) <= width {
				current = candidate
				continue
			}
			lines = append(lines, current)
			current = word
		}
		lines = append(lines, current)
	}

	return lines
}

func padChangeCell(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = ansi.Truncate(value, width, "…")
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func buildChangeDiffCommand(change jjhub.Change) string {
	command := "jj diff --git"
	if strings.TrimSpace(change.ChangeID) == "" {
		return command
	}
	return command + " -r " + shellQuote(change.ChangeID)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

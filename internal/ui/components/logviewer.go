package components

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	uistyles "github.com/charmbracelet/crush/internal/ui/styles"
)

// Compile-time interface check.
var _ Pane = (*LogViewer)(nil)

// LogLine is one rendered line in the log viewer.
type LogLine struct {
	Text  string
	Error bool
}

// LogViewer renders searchable, scrollable log output with line numbers.
type LogViewer struct {
	viewport viewport.Model

	width  int
	height int
	title  string

	lines       []LogLine
	errorLines  map[int]bool
	placeholder string
	content     string

	searchInput  textinput.Model
	searchActive bool
	searchValue  string
	searchErr    error
	matchCount   int

	sty uistyles.Styles
}

// NewLogViewer creates a new log viewer.
func NewLogViewer() *LogViewer {
	sty := uistyles.DefaultStyles()

	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "regex search"
	ti.SetVirtualCursor(true)
	ti.SetStyles(sty.TextInput)

	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = true
	vp.LeftGutterFunc = func(info viewport.GutterContext) string {
		digits := 2
		if info.TotalLines > 0 {
			digits = max(2, len(strconv.Itoa(info.TotalLines)))
		}
		if info.Soft {
			return sty.LineNumber.Render(" " + strings.Repeat(" ", digits) + " ")
		}
		return sty.LineNumber.Render(fmt.Sprintf(" %*d ", digits, info.Index+1))
	}
	vp.HighlightStyle = lipgloss.NewStyle().
		Background(sty.BgOverlay).
		Foreground(sty.White)
	vp.SelectedHighlightStyle = lipgloss.NewStyle().
		Background(sty.Blue).
		Foreground(sty.BgBase)

	lv := &LogViewer{
		viewport:     vp,
		title:        "Logs",
		errorLines:   make(map[int]bool),
		placeholder:  "Select a task to inspect its logs.",
		searchInput:  ti,
		searchActive: false,
		sty:          sty,
	}
	lv.viewport.StyleLineFunc = lv.lineStyle
	lv.SetSize(0, 0)
	return lv
}

// Init implements Pane.
func (lv *LogViewer) Init() tea.Cmd {
	return nil
}

// Update implements Pane.
func (lv *LogViewer) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		lv.SetSize(msg.Width, msg.Height)
		return lv, nil

	case tea.KeyPressMsg:
		if lv.searchActive {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				lv.searchActive = false
				lv.searchInput.Blur()
				lv.searchInput.SetValue(lv.searchValue)
				lv.applySearch(lv.searchValue)
				return lv, nil

			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				lv.searchActive = false
				lv.searchInput.Blur()
				lv.searchValue = lv.searchInput.Value()
				lv.applySearch(lv.searchValue)
				return lv, nil

			default:
				var cmd tea.Cmd
				lv.searchInput, cmd = lv.searchInput.Update(msg)
				lv.applySearch(lv.searchInput.Value())
				return lv, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			lv.searchActive = true
			lv.searchInput.SetValue(lv.searchValue)
			return lv, lv.searchInput.Focus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			lv.viewport.HighlightNext()
			return lv, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("N"))):
			lv.viewport.HighlightPrevious()
			return lv, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("g", "home"))):
			lv.viewport.GotoTop()
			return lv, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("G", "end"))):
			lv.viewport.GotoBottom()
			return lv, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("j"))):
			lv.viewport.ScrollDown(1)
			return lv, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("k"))):
			lv.viewport.ScrollUp(1)
			return lv, nil
		}

		var cmd tea.Cmd
		lv.viewport, cmd = lv.viewport.Update(msg)
		return lv, cmd
	}

	var cmd tea.Cmd
	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

// View implements Pane.
func (lv *LogViewer) View() string {
	if lv.width <= 0 || lv.height <= 0 {
		return ""
	}

	headerLines := []string{
		lv.renderHeader(),
	}
	if line, ok := lv.renderSearchLine(); ok {
		headerLines = append(headerLines, line)
	}

	bodyHeight := max(0, lv.height-len(headerLines))
	body := lv.renderBody(bodyHeight)

	sections := make([]string, 0, len(headerLines)+1)
	sections = append(sections, headerLines...)
	sections = append(sections, body)

	return lipgloss.NewStyle().
		Width(lv.width).
		Height(lv.height).
		Render(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

// SetSize implements Pane.
func (lv *LogViewer) SetSize(width, height int) {
	lv.width = max(0, width)
	lv.height = max(0, height)

	viewportWidth := max(0, lv.width-1)
	lv.viewport.SetWidth(viewportWidth)
	lv.viewport.SetHeight(lv.viewportHeight())
}

// SetTitle updates the viewer title.
func (lv *LogViewer) SetTitle(title string) {
	if strings.TrimSpace(title) == "" {
		lv.title = "Logs"
		return
	}
	lv.title = title
}

// SetPlaceholder clears content and shows a placeholder message.
func (lv *LogViewer) SetPlaceholder(placeholder string) {
	lv.lines = nil
	lv.content = ""
	lv.placeholder = placeholder
	lv.errorLines = make(map[int]bool)
	lv.matchCount = 0
	lv.searchErr = nil
	lv.viewport.SetContent("")
	lv.viewport.ClearHighlights()
}

// SetLines replaces the viewer contents.
func (lv *LogViewer) SetLines(lines []LogLine) {
	lv.lines = append([]LogLine(nil), lines...)
	lv.placeholder = ""
	lv.errorLines = make(map[int]bool, len(lines))

	var b strings.Builder
	for i, line := range lv.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line.Text)
		if line.Error {
			lv.errorLines[i] = true
		}
	}

	atBottom := lv.viewport.AtBottom()
	lv.content = b.String()
	lv.viewport.SetContent(lv.content)
	if atBottom {
		lv.viewport.GotoBottom()
	}

	lv.applySearch(lv.searchValue)
}

// SearchActive reports whether the search input is focused.
func (lv *LogViewer) SearchActive() bool {
	return lv.searchActive
}

// SearchValue reports the current applied search pattern.
func (lv *LogViewer) SearchValue() string {
	return lv.searchValue
}

// MatchCount reports the number of current regex matches.
func (lv *LogViewer) MatchCount() int {
	return lv.matchCount
}

func (lv *LogViewer) lineStyle(index int) lipgloss.Style {
	if lv.errorLines[index] {
		return lipgloss.NewStyle().
			Background(lv.sty.RedDark).
			Foreground(lv.sty.White)
	}
	return lipgloss.NewStyle()
}

func (lv *LogViewer) viewportHeight() int {
	height := lv.height - 1
	if _, ok := lv.renderSearchLine(); ok {
		height--
	}
	return max(0, height)
}

func (lv *LogViewer) renderHeader() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lv.sty.BlueLight).
		Render(lv.title)

	metaParts := make([]string, 0, 2)
	if lv.matchCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d matches", lv.matchCount))
	}
	if len(lv.lines) > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d lines", len(lv.lines)))
	}
	meta := lipgloss.NewStyle().
		Foreground(lv.sty.FgMuted).
		Render(strings.Join(metaParts, "  "))

	if meta == "" {
		return lipgloss.NewStyle().Width(lv.width).Render(title)
	}

	gap := max(1, lv.width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.NewStyle().
		Width(lv.width).
		Render(title + strings.Repeat(" ", gap) + meta)
}

func (lv *LogViewer) renderSearchLine() (string, bool) {
	switch {
	case lv.searchActive:
		return lipgloss.NewStyle().
			Width(lv.width).
			Render(lv.searchInput.View()), true

	case lv.searchErr != nil:
		return lipgloss.NewStyle().
			Foreground(lv.sty.Red).
			Width(lv.width).
			Render("Search error: " + lv.searchErr.Error()), true

	case lv.searchValue != "":
		msg := fmt.Sprintf("Search /%s", lv.searchValue)
		if lv.matchCount == 0 {
			msg += "  no matches"
		}
		return lipgloss.NewStyle().
			Foreground(lv.sty.FgMuted).
			Width(lv.width).
			Render(msg), true
	}

	return "", false
}

func (lv *LogViewer) renderBody(height int) string {
	if height <= 0 {
		return ""
	}

	if len(lv.lines) == 0 {
		placeholder := lv.placeholder
		if placeholder == "" {
			placeholder = "No log output."
		}
		return lipgloss.NewStyle().
			Foreground(lv.sty.FgMuted).
			Width(lv.width).
			Height(height).
			Render(placeholder)
	}

	lv.viewport.SetHeight(height)
	view := lv.viewport.View()
	scrollbar := common.Scrollbar(
		&lv.sty,
		height,
		lv.viewport.TotalLineCount(),
		lv.viewport.VisibleLineCount(),
		lv.viewport.YOffset(),
	)
	if scrollbar == "" {
		return lipgloss.NewStyle().
			Width(lv.width).
			Height(height).
			Render(view)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, view, scrollbar)
}

func (lv *LogViewer) applySearch(pattern string) {
	lv.searchValue = pattern
	lv.searchErr = nil
	lv.matchCount = 0
	lv.viewport.ClearHighlights()

	if pattern == "" || lv.content == "" {
		return
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		lv.searchErr = err
		return
	}

	matches := re.FindAllStringIndex(lv.content, -1)
	if len(matches) == 0 {
		return
	}

	lv.matchCount = len(matches)
	lv.viewport.SetHighlights(matches)
}

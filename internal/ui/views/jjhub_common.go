package views

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

const (
	jjDefaultListLimit = 200
	jjhubWebBaseURL    = "https://jjhub.tech"
)

var (
	jjTitleStyle      = lipgloss.NewStyle().Bold(true)
	jjSectionStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	jjMetaLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(12)
	jjMetaValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	jjSearchStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("80"))
	jjMutedStyle      = lipgloss.NewStyle().Faint(true)
	jjErrorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	jjSuccessStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("77"))
	jjOpenStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("77")).Bold(true)
	jjMergedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
	jjClosedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	jjDraftStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	jjPendingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true)
	jjBadgeBaseStyle  = lipgloss.NewStyle().Padding(0, 1)
	jjSidebarBoxStyle = lipgloss.NewStyle().Padding(0, 1)
)

type jjSearchState struct {
	active bool
	input  textinput.Model
}

type jjTablePane struct {
	columns []components.Column
	rows    []components.Row
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
}

type jjPreviewPane struct {
	sty      styles.Styles
	viewport viewport.Model
	width    int
	height   int
	content  string
	empty    string
}

type jjFilterTab struct {
	Value string
	Label string
	Icon  string
}

func newJJSearchInput(placeholder string) jjSearchState {
	input := textinput.New()
	input.Placeholder = placeholder
	input.SetVirtualCursor(true)
	return jjSearchState{input: input}
}

func newJJTablePane(columns []components.Column) *jjTablePane {
	return &jjTablePane{columns: columns}
}

func (p *jjTablePane) Init() tea.Cmd { return nil }

func (p *jjTablePane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
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
		if p.cursor < len(p.rows)-1 {
			p.cursor++
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("g", "home"))):
		p.cursor = 0
		p.offset = 0
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("G", "end"))):
		if len(p.rows) > 0 {
			p.cursor = len(p.rows) - 1
		}
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
		p.cursor = min(len(p.rows)-1, p.cursor+p.pageSize())
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
		p.cursor = max(0, p.cursor-p.pageSize())
	}

	p.clamp()
	return p, nil
}

func (p *jjTablePane) View() string {
	rendered, offset := components.RenderTable(
		p.columns,
		p.rows,
		p.cursor,
		p.offset,
		p.width,
		p.height,
		p.focused,
	)
	p.offset = offset
	return rendered
}

func (p *jjTablePane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.clamp()
}

func (p *jjTablePane) SetFocused(focused bool) {
	p.focused = focused
}

func (p *jjTablePane) SetRows(rows []components.Row) {
	p.rows = rows
	p.clamp()
}

func (p *jjTablePane) Cursor() int {
	return p.cursor
}

func (p *jjTablePane) Offset() int {
	return p.offset
}

func (p *jjTablePane) SetCursor(cursor int) {
	p.cursor = cursor
	p.clamp()
}

func (p *jjTablePane) pageSize() int {
	if p.height <= 3 {
		return 1
	}
	return max(1, p.height-2)
}

func (p *jjTablePane) clamp() {
	if len(p.rows) == 0 {
		p.cursor = 0
		p.offset = 0
		return
	}
	p.cursor = max(0, min(p.cursor, len(p.rows)-1))
	maxOffset := max(0, len(p.rows)-p.pageSize())
	p.offset = max(0, min(p.offset, maxOffset))
}

func newJJPreviewPane(empty string) *jjPreviewPane {
	sty := styles.DefaultStyles()
	vp := viewport.New(
		viewport.WithWidth(0),
		viewport.WithHeight(0),
	)
	vp.SoftWrap = true
	vp.FillHeight = true
	return &jjPreviewPane{
		sty:      sty,
		viewport: vp,
		empty:    empty,
	}
}

func (p *jjPreviewPane) Init() tea.Cmd { return nil }

func (p *jjPreviewPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		p.viewport.ScrollUp(1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		p.viewport.ScrollDown(1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
		p.viewport.HalfPageDown()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
		p.viewport.HalfPageUp()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("g", "home"))):
		p.viewport.GotoTop()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("G", "end"))):
		p.viewport.GotoBottom()
	}

	return p, nil
}

func (p *jjPreviewPane) View() string {
	if p.width <= 0 || p.height <= 0 {
		return ""
	}
	if strings.TrimSpace(p.content) == "" {
		return lipgloss.NewStyle().
			Width(p.width).
			Height(p.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(jjMutedStyle.Render(p.empty))
	}

	bodyHeight := p.bodyHeight()
	view := p.viewport.View()
	scrollbar := common.Scrollbar(
		&p.sty,
		bodyHeight,
		p.viewport.TotalLineCount(),
		bodyHeight,
		p.viewport.YOffset(),
	)
	if scrollbar != "" {
		view = lipgloss.JoinHorizontal(lipgloss.Top, view, scrollbar)
	}

	pager := jjMutedStyle.Render(
		fmt.Sprintf(
			"%d%%  %d/%d",
			int(p.viewport.ScrollPercent()*100),
			min(p.viewport.TotalLineCount(), p.viewport.YOffset()+1),
			max(1, p.viewport.TotalLineCount()),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left, view, pager)
}

func (p *jjPreviewPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.syncViewport(false)
}

func (p *jjPreviewPane) SetContent(content string, reset bool) {
	p.content = content
	p.syncViewport(reset)
}

func (p *jjPreviewPane) bodyHeight() int {
	if p.height <= 1 {
		return max(1, p.height)
	}
	return p.height - 1
}

func (p *jjPreviewPane) syncViewport(reset bool) {
	bodyHeight := p.bodyHeight()
	contentWidth := max(1, p.width)
	if bodyHeight > 0 && strings.Count(p.content, "\n")+1 > bodyHeight && p.width > 1 {
		contentWidth = p.width - 1
	}
	p.viewport.SetWidth(max(1, contentWidth))
	p.viewport.SetHeight(max(1, bodyHeight))
	p.viewport.SetContent(p.content)
	if reset {
		p.viewport.GotoTop()
	}
}

func jjRenderHeader(title string, width int, right string) string {
	left := jjTitleStyle.Render(title)
	if width <= 0 || right == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func jjRenderFilterTabs(tabs []jjFilterTab, selected string, counts map[string]int) string {
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := fmt.Sprintf("%s %s %d", tab.Icon, tab.Label, counts[tab.Value])
		style := jjBadgeStyleForState(tab.Value)
		if tab.Value != selected {
			style = style.Faint(true)
		}
		parts = append(parts, style.Render(label))
	}
	return strings.Join(parts, "  ")
}

func jjBadgeStyleForState(state string) lipgloss.Style {
	style := jjBadgeBaseStyle
	switch state {
	case "open", "running":
		return style.Foreground(lipgloss.Color("120")).BorderForeground(lipgloss.Color("120")).Border(lipgloss.RoundedBorder())
	case "merged":
		return style.Foreground(lipgloss.Color("141")).BorderForeground(lipgloss.Color("141")).Border(lipgloss.RoundedBorder())
	case "closed", "failed":
		return style.Foreground(lipgloss.Color("203")).BorderForeground(lipgloss.Color("203")).Border(lipgloss.RoundedBorder())
	case "draft", "stopped":
		return style.Foreground(lipgloss.Color("245")).BorderForeground(lipgloss.Color("245")).Border(lipgloss.RoundedBorder())
	case "pending":
		return style.Foreground(lipgloss.Color("221")).BorderForeground(lipgloss.Color("221")).Border(lipgloss.RoundedBorder())
	default:
		return style.Foreground(lipgloss.Color("250")).BorderForeground(lipgloss.Color("240")).Border(lipgloss.RoundedBorder())
	}
}

func jjhubLandingStateIcon(state string) string {
	switch state {
	case "open":
		return jjOpenStyle.Render("↑")
	case "merged":
		return jjMergedStyle.Render(styles.CheckIcon)
	case "closed":
		return jjClosedStyle.Render(styles.ToolError)
	case "draft":
		return jjDraftStyle.Render("◌")
	default:
		return jjMutedStyle.Render("?")
	}
}

func jjhubIssueStateIcon(state string) string {
	switch state {
	case "open":
		return jjOpenStyle.Render(styles.RadioOn)
	case "closed":
		return jjClosedStyle.Render(styles.RadioOff)
	default:
		return jjMutedStyle.Render("?")
	}
}

func jjhubWorkspaceStatusIcon(status string) string {
	switch status {
	case "running":
		return jjOpenStyle.Render(styles.ToolPending)
	case "pending":
		return jjPendingStyle.Render("◌")
	case "stopped":
		return jjDraftStyle.Render(styles.RadioOff)
	case "failed":
		return jjClosedStyle.Render(styles.ToolError)
	default:
		return jjMutedStyle.Render("?")
	}
}

func jjLandingConflictCell(landing jjhub.Landing, detail *jjhub.LandingDetail) string {
	conflictStatus := landing.ConflictStatus
	if detail != nil && detail.Conflicts.ConflictStatus != "" {
		conflictStatus = detail.Conflicts.ConflictStatus
	}
	switch {
	case strings.Contains(strings.ToLower(conflictStatus), "conflict"):
		return jjClosedStyle.Render("conflict")
	case conflictStatus == "" || conflictStatus == "unknown":
		return jjMutedStyle.Render("unknown")
	default:
		return jjSuccessStyle.Render(conflictStatus)
	}
}

func jjLandingReviewCell(detail *jjhub.LandingDetail) string {
	if detail == nil {
		return jjMutedStyle.Render("…")
	}
	return fmt.Sprintf("%d", len(detail.Reviews))
}

func jjReviewStateLabel(state string) string {
	switch state {
	case "approve":
		return jjSuccessStyle.Render("approve")
	case "request_changes":
		return jjClosedStyle.Render("changes")
	case "comment":
		return jjPendingStyle.Render("comment")
	default:
		return jjMutedStyle.Render(state)
	}
}

func jjRenderLabel(label jjhub.Label) string {
	base := lipgloss.NewStyle().Padding(0, 1)
	if label.Color != "" {
		return base.Foreground(lipgloss.Color(label.Color)).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(label.Color)).Render(label.Name)
	}
	return base.Foreground(lipgloss.Color("111")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Render(label.Name)
}

func jjJoinAssignees(users []jjhub.User) string {
	if len(users) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(users))
	for _, user := range users {
		if user.Login == "" {
			continue
		}
		parts = append(parts, "@"+user.Login)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func jjMarkdown(md string, width int, sty *styles.Styles) string {
	if strings.TrimSpace(md) == "" {
		return jjMutedStyle.Render("(no description)")
	}
	if width <= 0 {
		width = 40
	}

	if sty == nil {
		return jjWrapText(md, width)
	}

	renderer := common.MarkdownRenderer(sty, width)
	rendered, err := renderer.Render(md)
	if err != nil {
		return jjWrapText(md, width)
	}
	return strings.TrimSpace(rendered)
}

func jjWrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var out []string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimRight(rawLine, " ")
		if line == "" {
			out = append(out, "")
			continue
		}
		for lipgloss.Width(line) > width {
			runes := []rune(line)
			split := min(len(runes), width)
			out = append(out, string(runes[:split]))
			line = string(runes[split:])
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func jjhubRelativeTime(raw string) string {
	if raw == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return raw
		}
	}

	delta := time.Since(parsed)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
	case delta < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(delta.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(delta.Hours()/(24*365)))
	}
}

func jjFormatTime(raw string) string {
	if raw == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return raw
		}
	}
	return parsed.Format("2006-01-02 15:04")
}

func jjMatchesSearch(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

func jjMetaRow(label, value string) string {
	return jjMetaLabelStyle.Render(label) + jjMetaValueStyle.Render(value)
}

func jjOpenURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return components.ShowToastMsg{
				Title: "Browser open failed",
				Body:  "No URL available for this item.",
				Level: components.ToastLevelError,
			}
		}

		var (
			cmd *exec.Cmd
			err error
		)

		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url) //nolint:gosec // user-triggered URL open
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // user-triggered URL open
		default:
			cmd = exec.Command("xdg-open", url) //nolint:gosec // user-triggered URL open
		}

		if err = cmd.Start(); err != nil {
			return components.ShowToastMsg{
				Title: "Browser open failed",
				Body:  err.Error(),
				Level: components.ToastLevelError,
			}
		}

		return components.ShowToastMsg{
			Title: "Opened in browser",
			Body:  url,
			Level: components.ToastLevelSuccess,
		}
	}
}

func jjLandingURL(repo *jjhub.Repo, number int) string {
	if repo == nil || repo.FullName == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/landings/%d", jjhubWebBaseURL, repo.FullName, number)
}

func jjIssueURL(repo *jjhub.Repo, number int) string {
	if repo == nil || repo.FullName == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/issues/%d", jjhubWebBaseURL, repo.FullName, number)
}

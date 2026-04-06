package views

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/handoff"
)

// Compile-time interface check.
var _ View = (*SessionsView)(nil)

type sessionsLoadedMsg struct {
	runs []smithers.RunSummary
}

type sessionsErrorMsg struct {
	err error
}

type sessionPreviewLoadedMsg struct {
	runID string
	cache sessionPreviewCache
}

type sessionPreviewErrorMsg struct {
	runID string
	err   error
}

type sessionsHijackSessionMsg struct {
	runID   string
	session *smithers.HijackSession
	err     error
}

type sessionPreviewCache struct {
	mainNodeID string
	blocks     []smithers.ChatBlock
}

type sessionsHandoffTag struct {
	runID string
}

// SessionsView displays recent session-like runs with an optional transcript
// preview sidebar.
type SessionsView struct {
	client *smithers.Client

	runs    []smithers.RunSummary
	cursor  int
	width   int
	height  int
	loading bool
	err     error

	showSidebar bool
	splitPane   *components.SplitPane
	listPane    *sessionsListPane
	previewPane *sessionsPreviewPane

	searchActive bool
	searchInput  textinput.Model

	spinner spinner.Model

	previewCache map[string]sessionPreviewCache
	previewErrs  map[string]error
	previewLoads map[string]bool

	engineCache map[string]string

	hijacking bool
	hijackErr error
}

// NewSessionsView creates a new sessions browser.
func NewSessionsView(client *smithers.Client) *SessionsView {
	ti := textinput.New()
	ti.Placeholder = "filter sessions..."
	ti.SetVirtualCursor(true)

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	v := &SessionsView{
		client:       client,
		loading:      true,
		showSidebar:  true,
		searchInput:  ti,
		spinner:      s,
		previewCache: make(map[string]sessionPreviewCache),
		previewErrs:  make(map[string]error),
		previewLoads: make(map[string]bool),
		engineCache:  make(map[string]string),
	}

	v.listPane = &sessionsListPane{view: v}
	v.previewPane = &sessionsPreviewPane{view: v}
	v.splitPane = components.NewSplitPane(v.listPane, v.previewPane, components.SplitPaneOpts{
		LeftWidth:         58,
		CompactBreakpoint: 104,
	})

	return v
}

// Init loads recent runs and starts the spinner if needed.
func (v *SessionsView) Init() tea.Cmd {
	return tea.Batch(v.loadRunsCmd(), v.spinner.Tick)
}

func (v *SessionsView) loadRunsCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		if client == nil {
			return sessionsErrorMsg{err: fmt.Errorf("smithers client not configured")}
		}
		runs, err := client.ListRuns(context.Background(), smithers.RunFilter{Limit: 50})
		if err != nil {
			return sessionsErrorMsg{err: err}
		}
		return sessionsLoadedMsg{runs: filterSessionRuns(runs)}
	}
}

func (v *SessionsView) fetchPreviewCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		if client == nil {
			return sessionPreviewErrorMsg{runID: runID, err: fmt.Errorf("smithers client not configured")}
		}
		blocks, err := client.GetChatOutput(context.Background(), runID)
		if err != nil {
			return sessionPreviewErrorMsg{runID: runID, err: err}
		}
		return sessionPreviewLoadedMsg{
			runID: runID,
			cache: buildSessionPreviewCache(blocks),
		}
	}
}

func (v *SessionsView) hijackRunCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		if client == nil {
			return sessionsHijackSessionMsg{runID: runID, err: fmt.Errorf("smithers client not configured")}
		}
		session, err := client.HijackRun(context.Background(), runID)
		return sessionsHijackSessionMsg{runID: runID, session: session, err: err}
	}
}

func (v *SessionsView) visibleRuns() []smithers.RunSummary {
	query := strings.TrimSpace(strings.ToLower(v.searchInput.Value()))
	if query == "" {
		return v.runs
	}

	out := make([]smithers.RunSummary, 0, len(v.runs))
	for _, run := range v.runs {
		name := strings.ToLower(sessionDisplayName(run))
		if strings.Contains(name, query) {
			out = append(out, run)
		}
	}
	return out
}

func (v *SessionsView) selectedRun() (smithers.RunSummary, bool) {
	return components.RunAtCursor(v.visibleRuns(), v.cursor)
}

func (v *SessionsView) selectedRunID() string {
	run, ok := v.selectedRun()
	if !ok {
		return ""
	}
	return run.RunID
}

func (v *SessionsView) clampCursor() {
	visible := v.visibleRuns()
	switch {
	case len(visible) == 0:
		v.cursor = 0
	case v.cursor < 0:
		v.cursor = 0
	case v.cursor >= len(visible):
		v.cursor = len(visible) - 1
	}
}

func (v *SessionsView) ensurePreviewCmd() tea.Cmd {
	run, ok := v.selectedRun()
	if !ok {
		return nil
	}
	runID := run.RunID
	if _, ok := v.previewCache[runID]; ok {
		return nil
	}
	if _, ok := v.previewErrs[runID]; ok {
		return nil
	}
	if v.previewLoads[runID] {
		return nil
	}
	v.previewLoads[runID] = true
	return v.fetchPreviewCmd(runID)
}

func (v *SessionsView) refreshCmd() tea.Cmd {
	v.loading = true
	v.err = nil
	v.hijackErr = nil
	v.previewCache = make(map[string]sessionPreviewCache)
	v.previewErrs = make(map[string]error)
	v.previewLoads = make(map[string]bool)
	return tea.Batch(v.loadRunsCmd(), v.spinner.Tick)
}

func (v *SessionsView) bodyHeight() int {
	h := v.height - 4
	if v.searchActive {
		h--
	}
	if v.hijacking || v.hijackErr != nil {
		h--
	}
	if h < 1 {
		return 1
	}
	return h
}

func (v *SessionsView) resizeBody() {
	bodyHeight := v.bodyHeight()
	if v.showSidebar && v.splitPane != nil {
		v.splitPane.SetSize(v.width, bodyHeight)
		return
	}
	if v.listPane != nil {
		v.listPane.SetSize(v.width, bodyHeight)
	}
	if v.previewPane != nil {
		v.previewPane.SetSize(max(0, v.width/2), bodyHeight)
	}
}

func (v *SessionsView) hasActiveRuns() bool {
	for _, run := range v.runs {
		if isActiveSessionStatus(run.Status) {
			return true
		}
	}
	return false
}

func (v *SessionsView) shouldSpin() bool {
	return v.loading || v.hijacking || v.hasActiveRuns()
}

func (v *SessionsView) maybeSpinnerCmd() tea.Cmd {
	if !v.shouldSpin() {
		return nil
	}
	return v.spinner.Tick
}

// Update handles messages for the sessions browser.
func (v *SessionsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		selectedID := v.selectedRunID()
		v.runs = msg.runs
		v.loading = false
		v.err = nil
		if selectedID != "" {
			for i, run := range v.visibleRuns() {
				if run.RunID == selectedID {
					v.cursor = i
					break
				}
			}
		}
		v.clampCursor()
		v.resizeBody()
		return v, tea.Batch(v.ensurePreviewCmd(), v.maybeSpinnerCmd())

	case sessionsErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case sessionPreviewLoadedMsg:
		delete(v.previewLoads, msg.runID)
		delete(v.previewErrs, msg.runID)
		v.previewCache[msg.runID] = msg.cache
		return v, nil

	case sessionPreviewErrorMsg:
		delete(v.previewLoads, msg.runID)
		v.previewErrs[msg.runID] = msg.err
		return v, nil

	case sessionsHijackSessionMsg:
		v.hijacking = false
		if msg.err != nil {
			v.hijackErr = msg.err
			return v, nil
		}
		if msg.session == nil {
			v.hijackErr = fmt.Errorf("resume session: empty hijack response")
			return v, nil
		}
		if msg.session.AgentEngine != "" {
			v.engineCache[msg.runID] = msg.session.AgentEngine
		}
		args := msg.session.ResumeArgs()
		if !msg.session.SupportsResume || len(args) == 0 {
			engine := msg.session.AgentEngine
			if engine == "" {
				engine = "agent"
			}
			v.hijackErr = fmt.Errorf("%s does not support session resume", engine)
			return v, nil
		}
		binary := msg.session.AgentBinary
		if binary == "" {
			binary = msg.session.AgentEngine
		}
		return v, handoff.Handoff(handoff.Options{
			Binary: binary,
			Args:   args,
			Cwd:    msg.session.CWD,
			Tag:    sessionsHandoffTag{runID: msg.runID},
		})

	case handoff.HandoffMsg:
		tag, ok := msg.Tag.(sessionsHandoffTag)
		if !ok {
			return v, nil
		}
		v.hijacking = false
		if msg.Result.Err != nil {
			v.hijackErr = fmt.Errorf("resume session %s: %w", tag.runID, msg.Result.Err)
		} else {
			v.hijackErr = nil
		}
		return v, v.refreshCmd()

	case spinner.TickMsg:
		if !v.shouldSpin() {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		if v.searchActive {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				if v.searchInput.Value() != "" {
					v.searchInput.Reset()
				} else {
					v.searchActive = false
					v.searchInput.Blur()
				}
				v.cursor = 0
				v.clampCursor()
				v.resizeBody()
				return v, v.ensurePreviewCmd()

			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				v.searchActive = false
				v.searchInput.Blur()
				v.resizeBody()
				return v, v.ensurePreviewCmd()

			default:
				prev := v.searchInput.Value()
				var cmd tea.Cmd
				v.searchInput, cmd = v.searchInput.Update(msg)
				if v.searchInput.Value() != prev {
					v.cursor = 0
					v.clampCursor()
					return v, tea.Batch(cmd, v.ensurePreviewCmd())
				}
				return v, cmd
			}
		}

		oldSelected := v.selectedRunID()

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			v.searchActive = true
			v.searchInput.CursorEnd()
			v.resizeBody()
			return v, v.searchInput.Focus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.visibleRuns())-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			v.cursor = 0

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			if n := len(v.visibleRuns()); n > 0 {
				v.cursor = n - 1
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
			v.showSidebar = !v.showSidebar
			v.resizeBody()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			run, ok := v.selectedRun()
			if !ok {
				return v, nil
			}
			agentName := v.engineLabel(run.RunID)
			if agentName == "—" {
				agentName = ""
			}
			return v, func() tea.Msg {
				return OpenLiveChatMsg{
					RunID:     run.RunID,
					AgentName: agentName,
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			run, ok := v.selectedRun()
			if !ok || run.Status.IsTerminal() || v.hijacking {
				return v, nil
			}
			v.hijacking = true
			v.hijackErr = nil
			return v, tea.Batch(v.hijackRunCmd(run.RunID), v.maybeSpinnerCmd())
		}

		v.clampCursor()
		if v.selectedRunID() != oldSelected {
			return v, v.ensurePreviewCmd()
		}
	}

	return v, nil
}

// View renders the sessions browser.
func (v *SessionsView) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Sessions")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			b.WriteString(header + strings.Repeat(" ", gap) + helpHint)
		} else {
			b.WriteString(header + " " + helpHint)
		}
	} else {
		b.WriteString(header)
	}
	b.WriteString("\n")

	if v.searchActive {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("/") + " " + v.searchInput.View())
	} else {
		info := fmt.Sprintf("%d sessions", len(v.visibleRuns()))
		if q := strings.TrimSpace(v.searchInput.Value()); q != "" {
			info += "  filter: " + q
		}
		sidebar := "preview: off"
		if v.showSidebar {
			sidebar = "preview: on"
		}
		info = info + "  " + sidebar
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(info))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", max(1, v.width)))
	b.WriteString("\n")

	if v.hijacking {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(v.spinner.View() + " Resuming session..."))
		b.WriteString("\n")
	}
	if v.hijackErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
			fmt.Sprintf("  Resume error: %v", v.hijackErr),
		))
		b.WriteString("\n")
	}

	if v.loading {
		b.WriteString("  " + v.spinner.View() + " Loading sessions...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}
	if len(v.visibleRuns()) == 0 {
		if q := strings.TrimSpace(v.searchInput.Value()); q != "" {
			b.WriteString(fmt.Sprintf("  No sessions matching %q.\n", q))
		} else {
			b.WriteString("  No sessions found.\n")
		}
		return b.String()
	}

	v.resizeBody()
	if v.showSidebar && v.splitPane != nil {
		b.WriteString(v.splitPane.View())
		return b.String()
	}

	b.WriteString(v.listPane.View())
	return b.String()
}

// Name returns the router name.
func (v *SessionsView) Name() string {
	return "sessions"
}

// SetSize stores terminal dimensions.
func (v *SessionsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.searchInput.SetWidth(max(12, width-4))
	v.resizeBody()
}

// ShortHelp returns help bindings for the contextual help bar.
func (v *SessionsView) ShortHelp() []key.Binding {
	if v.searchActive {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear/close")),
		}
	}

	previewHelp := "preview"
	if v.showSidebar {
		previewHelp = "hide preview"
	}

	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open chat")),
		key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "resume")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", previewHelp)),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	}
}

type sessionsListPane struct {
	view         *SessionsView
	width        int
	height       int
	scrollOffset int
}

func (p *sessionsListPane) Init() tea.Cmd { return nil }

func (p *sessionsListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil
}

func (p *sessionsListPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *sessionsListPane) View() string {
	v := p.view
	runs := v.visibleRuns()
	if len(runs) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("No sessions found")
	}

	width := p.width
	if width <= 0 {
		width = max(40, v.width)
	}

	statusW := 2
	engineW := 10
	startedW := 8
	durationW := 7
	messagesW := 4
	if width >= 72 {
		engineW = 12
		startedW = 9
		durationW = 8
		messagesW = 5
	}

	gaps := 5
	nameW := width - statusW - engineW - startedW - durationW - messagesW - gaps
	if nameW < 12 {
		nameW = 12
	}

	headerStyle := lipgloss.NewStyle().Faint(true)
	header := strings.Join([]string{
		padRight("", statusW),
		padRight("Session", nameW),
		padRight("Engine", engineW),
		padRight("Started", startedW),
		padRight("Duration", durationW),
		padRight("Msgs", messagesW),
	}, " ")

	rowsHeight := p.height - 1
	if rowsHeight < 1 {
		rowsHeight = 1
	}
	if p.scrollOffset > v.cursor {
		p.scrollOffset = v.cursor
	}
	if v.cursor >= p.scrollOffset+rowsHeight {
		p.scrollOffset = v.cursor - rowsHeight + 1
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}

	end := p.scrollOffset + rowsHeight
	if end > len(runs) {
		end = len(runs)
	}

	var lines []string
	lines = append(lines, headerStyle.Render(header))

	selectedStyle := lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236"))
	for i := p.scrollOffset; i < end; i++ {
		run := runs[i]
		line := strings.Join([]string{
			padRight(v.statusIcon(run.Status), statusW),
			padRight(truncate(sessionDisplayName(run), nameW), nameW),
			padRight(truncate(v.engineLabel(run.RunID), engineW), engineW),
			padRight(truncate(sessionStartedLabel(run), startedW), startedW),
			padRight(truncate(sessionDurationLabel(run), durationW), durationW),
			padRight(v.messageCountLabel(run.RunID), messagesW),
		}, " ")
		if i == v.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

type sessionsPreviewPane struct {
	view   *SessionsView
	width  int
	height int
}

func (p *sessionsPreviewPane) Init() tea.Cmd { return nil }

func (p *sessionsPreviewPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
	return p, nil
}

func (p *sessionsPreviewPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *sessionsPreviewPane) View() string {
	v := p.view
	run, ok := v.selectedRun()
	if !ok {
		return lipgloss.NewStyle().Faint(true).Render("Select a session")
	}

	width := p.width
	if width <= 0 {
		width = max(30, v.width/2)
	}
	contentWidth := max(16, width-2)

	labelStyle := lipgloss.NewStyle().Faint(true)
	titleStyle := lipgloss.NewStyle().Bold(true)

	lines := []string{
		titleStyle.Render("Preview"),
		"",
		labelStyle.Render("Run ID:") + " " + run.RunID,
		labelStyle.Render("Workflow:") + " " + sessionDisplayName(run),
		labelStyle.Render("Engine:") + " " + v.engineLabel(run.RunID),
		labelStyle.Render("Status:") + " " + string(run.Status),
		labelStyle.Render("Started:") + " " + previewTimestamp(run.StartedAtMs),
		labelStyle.Render("Duration:") + " " + sessionDurationLabel(run),
		labelStyle.Render("Messages:") + " " + v.messageCountLabel(run.RunID),
		"",
		titleStyle.Render("Recent Messages"),
	}

	if v.previewLoads[run.RunID] {
		lines = append(lines, v.spinner.View()+" Loading transcript...")
		return clipLines(lines, p.height)
	}
	if err, ok := v.previewErrs[run.RunID]; ok {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(err.Error()))
		return clipLines(lines, p.height)
	}

	cache, ok := v.previewCache[run.RunID]
	if !ok || len(cache.blocks) == 0 {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("No transcript available."))
		return clipLines(lines, p.height)
	}

	previewBlocks := cache.blocks
	if len(previewBlocks) > 4 {
		previewBlocks = previewBlocks[len(previewBlocks)-4:]
	}
	for _, block := range previewBlocks {
		lines = append(lines, renderPreviewBlock(block, contentWidth)...)
	}

	return clipLines(lines, p.height)
}

func (v *SessionsView) statusIcon(status smithers.RunStatus) string {
	switch status {
	case smithers.RunStatusRunning, smithers.RunStatusWaitingApproval, smithers.RunStatusWaitingEvent:
		return v.spinner.View()
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Faint(true).Render("✓")
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗")
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Faint(true).Render("•")
	default:
		return lipgloss.NewStyle().Faint(true).Render("·")
	}
}

func (v *SessionsView) engineLabel(runID string) string {
	if engine := v.engineCache[runID]; engine != "" {
		return engine
	}
	return "—"
}

func (v *SessionsView) messageCountLabel(runID string) string {
	if cache, ok := v.previewCache[runID]; ok {
		return fmt.Sprintf("%d", len(cache.blocks))
	}
	if v.previewLoads[runID] {
		return "..."
	}
	return "—"
}

func filterSessionRuns(runs []smithers.RunSummary) []smithers.RunSummary {
	out := make([]smithers.RunSummary, 0, len(runs))
	for _, run := range runs {
		if len(run.Summary) > 0 || isInteractiveRun(run) {
			out = append(out, run)
		}
	}
	return out
}

func isInteractiveRun(run smithers.RunSummary) bool {
	return strings.TrimSpace(run.WorkflowName) == "" && strings.TrimSpace(run.WorkflowPath) == ""
}

func isActiveSessionStatus(status smithers.RunStatus) bool {
	switch status {
	case smithers.RunStatusRunning, smithers.RunStatusWaitingApproval, smithers.RunStatusWaitingEvent:
		return true
	default:
		return false
	}
}

func sessionDisplayName(run smithers.RunSummary) string {
	if name := strings.TrimSpace(run.WorkflowName); name != "" {
		return name
	}
	if workflowPath := strings.TrimSpace(run.WorkflowPath); workflowPath != "" {
		base := path.Base(workflowPath)
		ext := path.Ext(base)
		return strings.TrimSuffix(base, ext)
	}
	return "Interactive"
}

func sessionStartedLabel(run smithers.RunSummary) string {
	if run.StartedAtMs == nil {
		return "—"
	}
	return relativeTime(*run.StartedAtMs)
}

func sessionDurationLabel(run smithers.RunSummary) string {
	if run.StartedAtMs == nil {
		return "—"
	}

	endMs := time.Now().UnixMilli()
	if run.FinishedAtMs != nil {
		endMs = *run.FinishedAtMs
	}
	d := time.Duration(endMs-*run.StartedAtMs) * time.Millisecond
	if d < 0 {
		d = 0
	}
	return formatWait(d)
}

func previewTimestamp(ms *int64) string {
	if ms == nil || *ms <= 0 {
		return "—"
	}
	return time.UnixMilli(*ms).Format("2006-01-02 15:04")
}

func buildSessionPreviewCache(blocks []smithers.ChatBlock) sessionPreviewCache {
	if len(blocks) == 0 {
		return sessionPreviewCache{}
	}

	type groupStats struct {
		count   int
		firstMs int64
	}

	groups := make(map[string][]smithers.ChatBlock)
	stats := make(map[string]groupStats)

	for _, block := range blocks {
		nodeID := block.NodeID
		groups[nodeID] = append(groups[nodeID], block)

		s := stats[nodeID]
		s.count++
		if s.firstMs == 0 || (block.TimestampMs > 0 && block.TimestampMs < s.firstMs) {
			s.firstMs = block.TimestampMs
		}
		stats[nodeID] = s
	}

	bestNodeID := ""
	bestStats := groupStats{}
	for nodeID, stat := range stats {
		if stat.count > bestStats.count ||
			(stat.count == bestStats.count && (bestStats.firstMs == 0 || stat.firstMs < bestStats.firstMs)) {
			bestNodeID = nodeID
			bestStats = stat
		}
	}

	return sessionPreviewCache{
		mainNodeID: bestNodeID,
		blocks:     groups[bestNodeID],
	}
}

func renderPreviewBlock(block smithers.ChatBlock, width int) []string {
	roleStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	role := previewRoleLabel(block.Role)
	lines := []string{roleStyle.Render(role)}

	bodyWidth := max(8, width-2)
	for _, rawLine := range strings.Split(block.Content, "\n") {
		for _, wrapped := range wrapLineToWidth(rawLine, bodyWidth) {
			lines = append(lines, " "+faintStyle.Render(truncateStr(wrapped, bodyWidth)))
		}
	}
	lines = append(lines, "")
	return lines
}

func previewRoleLabel(role smithers.ChatRole) string {
	switch role {
	case smithers.ChatRoleSystem:
		return "System"
	case smithers.ChatRoleUser:
		return "User"
	case smithers.ChatRoleAssistant:
		return "Assistant"
	case smithers.ChatRoleTool:
		return "Tool"
	default:
		return "Message"
	}
}

func clipLines(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if height == 1 {
		return "…"
	}
	clipped := append([]string{}, lines[:height-1]...)
	clipped = append(clipped, "…")
	return strings.Join(clipped, "\n")
}

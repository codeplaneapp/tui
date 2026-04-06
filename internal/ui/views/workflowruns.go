package views

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/components"
	uistyles "github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// Compile-time interface checks.
var (
	_ View      = (*WorkflowRunView)(nil)
	_ Focusable = (*WorkflowRunView)(nil)
)

type workflowRunsLoadedMsg struct {
	runs []smithers.RunSummary
}

type workflowRunsErrorMsg struct {
	err error
}

type workflowRunInspectionMsg struct {
	runID      string
	inspection *smithers.RunInspection
	err        error
}

type workflowRunLogsLoadedMsg struct {
	key     string
	runID   string
	nodeID  string
	attempt int
	blocks  []smithers.ChatBlock
	err     error
}

type workflowStreamReadyMsg struct {
	ch <-chan interface{}
}

type workflowStreamUnavailableMsg struct{}

type workflowTickMsg struct{}

type workflowRunEnrichedMsg struct {
	run smithers.RunSummary
}

type workflowPane int

const (
	workflowPaneRuns workflowPane = iota
	workflowPaneTasks
	workflowPaneLogs
)

type workflowLayoutMode int

const (
	workflowLayoutNarrow workflowLayoutMode = iota
	workflowLayoutMedium
	workflowLayoutWide
)

type workflowTaskLog struct {
	key     string
	runID   string
	nodeID  string
	attempt int
	blocks  []smithers.ChatBlock
	loading bool
	loaded  bool
	err     error
}

var workflowErrorPattern = regexp.MustCompile(`(?i)\b(error|failed|panic|exception|traceback)\b`)

// WorkflowRunView shows workflow runs, tasks, and task logs side by side.
type WorkflowRunView struct {
	client *smithers.Client
	sty    uistyles.Styles

	width  int
	height int

	ctx    context.Context
	cancel context.CancelFunc

	runs       []smithers.RunSummary
	runCursor  int
	taskCursor int

	loading bool
	err     error

	focus      workflowPane
	zoomedPane *workflowPane

	inspections   map[string]*smithers.RunInspection
	inspectionErr map[string]error
	inspecting    map[string]bool

	logs map[string]workflowTaskLog

	logViewer *components.LogViewer
	spinner   spinner.Model

	allEventsCh <-chan interface{}
	streamMode  string
	pollTicker  *time.Ticker
}

// NewWorkflowRunView creates a workflow run viewer.
func NewWorkflowRunView(client *smithers.Client) *WorkflowRunView {
	sty := uistyles.DefaultStyles()
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	s.Style = lipgloss.NewStyle().Foreground(sty.Green)

	v := &WorkflowRunView{
		client:        client,
		sty:           sty,
		loading:       true,
		focus:         workflowPaneRuns,
		inspections:   make(map[string]*smithers.RunInspection),
		inspectionErr: make(map[string]error),
		inspecting:    make(map[string]bool),
		logs:          make(map[string]workflowTaskLog),
		logViewer:     components.NewLogViewer(),
		spinner:       s,
	}
	v.syncLogViewer()
	return v
}

// Init implements View.
func (v *WorkflowRunView) Init() tea.Cmd {
	v.ctx, v.cancel = context.WithCancel(context.Background())
	return tea.Batch(
		v.loadRunsCmd(),
		v.startStreamCmd(),
		v.spinner.Tick,
	)
}

// OnFocus implements Focusable.
func (v *WorkflowRunView) OnFocus() tea.Cmd {
	return nil
}

// OnBlur implements Focusable.
func (v *WorkflowRunView) OnBlur() tea.Cmd {
	v.stopBackgroundWork()
	return nil
}

func (v *WorkflowRunView) loadRunsCmd() tea.Cmd {
	ctx := v.viewContext()
	client := v.client
	return func() tea.Msg {
		runs, err := client.ListRuns(ctx, smithers.RunFilter{Limit: 50})
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return workflowRunsErrorMsg{err: err}
		}
		return workflowRunsLoadedMsg{runs: runs}
	}
}

func (v *WorkflowRunView) inspectRunCmd(runID string) tea.Cmd {
	ctx := v.viewContext()
	client := v.client
	return func() tea.Msg {
		inspection, err := client.InspectRun(ctx, runID)
		if ctx.Err() != nil {
			return nil
		}
		return workflowRunInspectionMsg{
			runID:      runID,
			inspection: inspection,
			err:        err,
		}
	}
}

func (v *WorkflowRunView) loadTaskLogsCmd(runID string, task smithers.RunTask) tea.Cmd {
	ctx := v.viewContext()
	client := v.client
	key := v.logKey(runID, task)
	nodeID := task.NodeID
	attempt := taskAttempt(task)

	return func() tea.Msg {
		blocks, err := client.GetChatOutput(ctx, runID)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return workflowRunLogsLoadedMsg{
				key:     key,
				runID:   runID,
				nodeID:  nodeID,
				attempt: attempt,
				err:     err,
			}
		}
		return workflowRunLogsLoadedMsg{
			key:     key,
			runID:   runID,
			nodeID:  nodeID,
			attempt: attempt,
			blocks:  filterTaskBlocks(blocks, nodeID, attempt),
		}
	}
}

func (v *WorkflowRunView) enrichRunCmd(runID string) tea.Cmd {
	ctx := v.viewContext()
	client := v.client
	return func() tea.Msg {
		run, err := client.GetRunSummary(ctx, runID)
		if err != nil || run == nil || ctx.Err() != nil {
			return nil
		}
		return workflowRunEnrichedMsg{run: *run}
	}
}

func (v *WorkflowRunView) startStreamCmd() tea.Cmd {
	ctx := v.viewContext()
	client := v.client
	return func() tea.Msg {
		ch, err := client.StreamAllEvents(ctx)
		if err != nil {
			return workflowStreamUnavailableMsg{}
		}
		return workflowStreamReadyMsg{ch: ch}
	}
}

func (v *WorkflowRunView) pollTickCmd() tea.Cmd {
	if v.pollTicker == nil {
		return nil
	}
	ch := v.pollTicker.C
	return func() tea.Msg {
		<-ch
		return workflowTickMsg{}
	}
}

// Update implements View.
func (v *WorkflowRunView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case workflowRunsLoadedMsg:
		selectedRunID := v.currentRunID()
		v.loading = false
		v.err = nil
		v.runs = msg.runs
		v.restoreRunSelection(selectedRunID)
		v.clampCursors()
		v.syncLogViewer()
		return v, v.ensureSelectedInspection(false)

	case workflowRunsErrorMsg:
		v.loading = false
		v.err = msg.err
		v.syncLogViewer()
		return v, nil

	case workflowRunInspectionMsg:
		v.inspecting[msg.runID] = false
		v.inspections[msg.runID] = msg.inspection
		if msg.err != nil {
			v.inspectionErr[msg.runID] = msg.err
		} else {
			delete(v.inspectionErr, msg.runID)
		}
		v.clampCursors()
		v.syncLogViewer()
		return v, nil

	case workflowRunLogsLoadedMsg:
		v.logs[msg.key] = workflowTaskLog{
			key:     msg.key,
			runID:   msg.runID,
			nodeID:  msg.nodeID,
			attempt: msg.attempt,
			blocks:  append([]smithers.ChatBlock(nil), msg.blocks...),
			loaded:  msg.err == nil,
			loading: false,
			err:     msg.err,
		}
		v.syncLogViewer()
		return v, nil

	case workflowRunEnrichedMsg:
		for i := range v.runs {
			if v.runs[i].RunID == msg.run.RunID {
				v.runs[i] = msg.run
				break
			}
		}
		v.syncLogViewer()
		return v, nil

	case workflowStreamReadyMsg:
		v.allEventsCh = msg.ch
		v.streamMode = "live"
		return v, smithers.WaitForAllEvents(v.allEventsCh)

	case workflowStreamUnavailableMsg:
		v.streamMode = "polling"
		if v.pollTicker != nil {
			v.pollTicker.Stop()
		}
		v.pollTicker = time.NewTicker(5 * time.Second)
		return v, v.pollTickCmd()

	case workflowTickMsg:
		if v.ctx != nil && v.ctx.Err() != nil {
			return v, nil
		}
		return v, tea.Batch(v.loadRunsCmd(), v.pollTickCmd())

	case smithers.RunEventMsg:
		newRunID := v.applyRunEvent(msg.Event)
		v.syncLogViewer()
		cmds := []tea.Cmd{smithers.WaitForAllEvents(v.allEventsCh)}
		if newRunID != "" {
			cmds = append(cmds, v.enrichRunCmd(newRunID))
		}
		return v, tea.Batch(cmds...)

	case smithers.RunEventErrorMsg:
		return v, smithers.WaitForAllEvents(v.allEventsCh)

	case smithers.RunEventDoneMsg:
		if v.ctx != nil && v.ctx.Err() == nil {
			return v, v.startStreamCmd()
		}
		return v, nil

	case spinner.TickMsg:
		if !v.shouldAnimate() {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		if v.focus == workflowPaneLogs && v.logViewer.SearchActive() &&
			key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			updated, cmd := v.logViewer.Update(msg)
			v.logViewer = updated.(*components.LogViewer)
			return v, cmd
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
			v.stopBackgroundWork()
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			v.focus = v.nextPane()
			v.syncLogViewer()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			v.focus = v.prevPane()
			v.syncLogViewer()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
			v.focus = v.nextPane()
			v.syncLogViewer()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			v.focus = v.prevPane()
			v.syncLogViewer()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("z"))):
			v.toggleZoom()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			cmds := []tea.Cmd{v.loadRunsCmd()}
			if cmd := v.ensureSelectedInspection(true); cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := v.ensureSelectedLogs(true); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return v, tea.Batch(cmds...)

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			switch v.focus {
			case workflowPaneRuns:
				v.focus = workflowPaneTasks
				return v, v.ensureSelectedInspection(true)
			case workflowPaneTasks:
				v.focus = workflowPaneLogs
				return v, v.ensureSelectedLogs(true)
			default:
				return v, nil
			}
		}

		if v.focus == workflowPaneLogs {
			updated, cmd := v.logViewer.Update(msg)
			v.logViewer = updated.(*components.LogViewer)
			return v, cmd
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			return v.handleListMove(-1)

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			return v.handleListMove(1)

		case key.Matches(msg, key.NewBinding(key.WithKeys("g", "home"))):
			return v.handleListSet(0)

		case key.Matches(msg, key.NewBinding(key.WithKeys("G", "end"))):
			switch v.focus {
			case workflowPaneRuns:
				return v.handleListSet(len(v.runs) - 1)
			case workflowPaneTasks:
				tasks := v.currentTasks()
				return v.handleListSet(len(tasks) - 1)
			}
		}
	}

	return v, nil
}

func (v *WorkflowRunView) handleListMove(delta int) (View, tea.Cmd) {
	switch v.focus {
	case workflowPaneRuns:
		if len(v.runs) == 0 {
			return v, nil
		}
		next := clampIndex(v.runCursor+delta, len(v.runs))
		if next == v.runCursor {
			return v, nil
		}
		v.runCursor = next
		v.taskCursor = 0
		v.syncLogViewer()
		return v, v.ensureSelectedInspection(false)

	case workflowPaneTasks:
		tasks := v.currentTasks()
		if len(tasks) == 0 {
			return v, nil
		}
		next := clampIndex(v.taskCursor+delta, len(tasks))
		if next == v.taskCursor {
			return v, nil
		}
		v.taskCursor = next
		v.syncLogViewer()
		return v, nil
	}

	return v, nil
}

func (v *WorkflowRunView) handleListSet(index int) (View, tea.Cmd) {
	switch v.focus {
	case workflowPaneRuns:
		if len(v.runs) == 0 {
			return v, nil
		}
		v.runCursor = clampIndex(index, len(v.runs))
		v.taskCursor = 0
		v.syncLogViewer()
		return v, v.ensureSelectedInspection(false)

	case workflowPaneTasks:
		tasks := v.currentTasks()
		if len(tasks) == 0 {
			return v, nil
		}
		v.taskCursor = clampIndex(index, len(tasks))
		v.syncLogViewer()
	}

	return v, nil
}

// View implements View.
func (v *WorkflowRunView) View() string {
	if v.width <= 0 {
		return ""
	}

	header := v.renderHeader()
	mainHeight := max(0, v.height-1)
	if mainHeight <= 0 {
		return header
	}

	mode := v.layoutMode()
	if v.zoomedPane != nil {
		content := v.renderPane(*v.zoomedPane, v.width, mainHeight)
		return lipgloss.JoinVertical(lipgloss.Left, header, content)
	}

	switch mode {
	case workflowLayoutWide:
		leftW := max(30, v.width/4)
		midW := max(34, v.width/4)
		if leftW+midW > v.width-24 {
			midW = max(28, (v.width-leftW)/2)
		}
		rightW := max(24, v.width-leftW-midW)
		left := v.renderPane(workflowPaneRuns, leftW, mainHeight)
		mid := v.renderPane(workflowPaneTasks, midW, mainHeight)
		right := v.renderPane(workflowPaneLogs, rightW, mainHeight)
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right),
		)

	case workflowLayoutMedium:
		leftW := max(28, v.width/3)
		rightW := max(24, v.width-leftW)
		detailPane := workflowPaneTasks
		if v.focus == workflowPaneLogs {
			detailPane = workflowPaneLogs
		}
		left := v.renderPane(workflowPaneRuns, leftW, mainHeight)
		right := v.renderPane(detailPane, rightW, mainHeight)
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		)

	default:
		return lipgloss.JoinVertical(lipgloss.Left, header, v.renderPane(v.focus, v.width, mainHeight))
	}
}

// Name implements View.
func (v *WorkflowRunView) Name() string {
	return "workflow-runs"
}

// SetSize implements View.
func (v *WorkflowRunView) SetSize(width, height int) {
	v.width = max(0, width)
	v.height = max(0, height)
	v.syncLogViewer()
}

// ShortHelp implements View.
func (v *WorkflowRunView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k", "↑", "↓"), key.WithHelp("jk/↑↓", "navigate")),
		key.NewBinding(key.WithKeys("h", "l", "tab"), key.WithHelp("hl/tab", "switch pane")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "drill in")),
		key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "zoom pane")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search logs")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	}
}

func (v *WorkflowRunView) viewContext() context.Context {
	if v.ctx != nil {
		return v.ctx
	}
	return context.Background()
}

func (v *WorkflowRunView) stopBackgroundWork() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.pollTicker != nil {
		v.pollTicker.Stop()
		v.pollTicker = nil
	}
}

func (v *WorkflowRunView) layoutMode() workflowLayoutMode {
	switch {
	case v.width > 150:
		return workflowLayoutWide
	case v.width >= 100:
		return workflowLayoutMedium
	default:
		return workflowLayoutNarrow
	}
}

func (v *WorkflowRunView) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Workflow Runs")

	mode := ""
	switch v.streamMode {
	case "live":
		mode = lipgloss.NewStyle().Foreground(v.sty.Green).Render("Live")
	case "polling":
		mode = lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render("Polling")
	}

	focus := lipgloss.NewStyle().
		Foreground(v.sty.FgMuted).
		Render("Focus: " + v.focus.String())

	metaParts := make([]string, 0, 3)
	if mode != "" {
		metaParts = append(metaParts, mode)
	}
	metaParts = append(metaParts, focus)
	if v.zoomedPane != nil {
		metaParts = append(metaParts, lipgloss.NewStyle().Foreground(v.sty.BlueLight).Render("Zoom"))
	}
	meta := strings.Join(metaParts, "  ")
	if meta == "" {
		return lipgloss.NewStyle().Width(v.width).Render(title)
	}

	gap := max(1, v.width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.NewStyle().
		Width(v.width).
		Render(title + strings.Repeat(" ", gap) + meta)
}

func (v *WorkflowRunView) renderPane(p workflowPane, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	focused := v.focus == p
	contentWidth := max(0, width-2)
	contentHeight := max(0, height-2)

	var content string
	switch p {
	case workflowPaneRuns:
		content = v.renderRunsPane(contentWidth, contentHeight, focused)
	case workflowPaneTasks:
		content = v.renderTasksPane(contentWidth, contentHeight, focused)
	default:
		v.logViewer.SetSize(contentWidth, contentHeight)
		content = v.logViewer.View()
	}

	return v.wrapPane(content, width, height, focused)
}

func (v *WorkflowRunView) wrapPane(content string, width, height int, focused bool) string {
	borderColor := v.sty.Border
	if focused {
		borderColor = v.sty.BorderColor
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(max(0, width-2)).
		Height(max(0, height-2))

	return style.Render(content)
}

func (v *WorkflowRunView) renderRunsPane(width, height int, focused bool) string {
	title := v.renderPaneTitle("Runs", fmt.Sprintf("%d", len(v.runs)), width, focused)
	bodyHeight := max(0, height-1)

	switch {
	case v.loading:
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, v.spinner.View()+" Loading runs...", false))
	case v.err != nil:
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, v.err.Error(), true))
	case len(v.runs) == 0:
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, "No runs found.", false))
	}

	start, end := windowForCursor(v.runCursor, len(v.runs), bodyHeight)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, v.renderRunRow(v.runs[i], i == v.runCursor, width, focused))
	}

	body := lipgloss.NewStyle().
		Width(width).
		Height(bodyHeight).
		Render(strings.Join(rows, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

func (v *WorkflowRunView) renderTasksPane(width, height int, focused bool) string {
	run := v.selectedRun()
	meta := ""
	if run != nil {
		meta = truncateText(run.WorkflowName, 18)
	}
	title := v.renderPaneTitle("Tasks", meta, width, focused)
	bodyHeight := max(0, height-1)

	if run == nil {
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, "Select a run.", false))
	}
	if v.inspecting[run.RunID] && v.inspections[run.RunID] == nil {
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, v.spinner.View()+" Loading tasks...", false))
	}
	if err := v.inspectionErr[run.RunID]; err != nil && v.inspections[run.RunID] == nil {
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, err.Error(), true))
	}

	tasks := v.currentTasks()
	if len(tasks) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, v.renderMessageBody(width, bodyHeight, "No tasks for this run.", false))
	}

	start, end := windowForCursor(v.taskCursor, len(tasks), bodyHeight)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, v.renderTaskRow(tasks[i], i == v.taskCursor, width, focused))
	}

	body := lipgloss.NewStyle().
		Width(width).
		Height(bodyHeight).
		Render(strings.Join(rows, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

func (v *WorkflowRunView) renderPaneTitle(label, meta string, width int, focused bool) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(v.sty.BlueLight)
	if focused {
		titleStyle = titleStyle.Foreground(v.sty.White)
	}
	title := titleStyle.Render(label)
	if meta == "" {
		return lipgloss.NewStyle().Width(width).Render(title)
	}

	right := lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render(meta)
	gap := max(1, width-lipgloss.Width(title)-lipgloss.Width(right))
	return lipgloss.NewStyle().Width(width).Render(title + strings.Repeat(" ", gap) + right)
}

func (v *WorkflowRunView) renderMessageBody(width, height int, msg string, isErr bool) string {
	style := lipgloss.NewStyle().Foreground(v.sty.FgMuted)
	if isErr {
		style = lipgloss.NewStyle().Foreground(v.sty.Red)
	}
	return style.Width(width).Height(height).Render(msg)
}

func (v *WorkflowRunView) renderRunRow(run smithers.RunSummary, selected bool, width int, focused bool) string {
	rightParts := make([]string, 0, 2)
	if progress := runProgress(run); progress != "" {
		rightParts = append(rightParts, progress)
	}
	if elapsed := runElapsed(run); elapsed != "" {
		rightParts = append(rightParts, elapsed)
	}
	right := strings.Join(rightParts, "  ")

	left := fmt.Sprintf("%s %s", v.runStatusIcon(run.Status), truncateText(run.WorkflowName, max(4, width-lipgloss.Width(right)-6)))
	row := left
	if right != "" {
		gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
		row += strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render(right)
	}
	return v.renderSelectableRow(row, width, selected, focused)
}

func (v *WorkflowRunView) renderTaskRow(task smithers.RunTask, selected bool, width int, focused bool) string {
	right := ""
	if attempt := taskAttemptLabel(task); attempt != "" {
		right = lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render(attempt)
	}
	label := fmt.Sprintf("%s %s", v.taskStatusIcon(task.State), truncateText(taskLabel(task), max(4, width-lipgloss.Width(right)-6)))
	row := label
	if right != "" {
		gap := max(1, width-lipgloss.Width(label)-lipgloss.Width(right))
		row += strings.Repeat(" ", gap) + right
	}
	return v.renderSelectableRow(row, width, selected, focused)
}

func (v *WorkflowRunView) renderSelectableRow(row string, width int, selected, focused bool) string {
	style := lipgloss.NewStyle().Width(width).Padding(0, 1)
	switch {
	case selected && focused:
		style = style.Background(v.sty.BgOverlay).Foreground(v.sty.White)
	case selected:
		style = style.Background(v.sty.BgSubtle)
	default:
		style = style.Foreground(v.sty.FgBase)
	}
	return style.Render(ansi.Truncate(row, max(0, width-2), "…"))
}

func (v *WorkflowRunView) runStatusIcon(status smithers.RunStatus) string {
	switch status {
	case smithers.RunStatusRunning:
		return v.spinner.View()
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Foreground(v.sty.Green).Render(uistyles.CheckIcon)
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(v.sty.Red).Render(uistyles.ToolError)
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Foreground(v.sty.Yellow).Render("○")
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(v.sty.Yellow).Render("⌛")
	case smithers.RunStatusWaitingEvent:
		return lipgloss.NewStyle().Foreground(v.sty.Yellow).Render("⧖")
	default:
		return lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render(uistyles.ToolPending)
	}
}

func (v *WorkflowRunView) taskStatusIcon(state smithers.TaskState) string {
	switch state {
	case smithers.TaskStateRunning:
		return v.spinner.View()
	case smithers.TaskStateFinished:
		return lipgloss.NewStyle().Foreground(v.sty.Green).Render(uistyles.ToolSuccess)
	case smithers.TaskStateFailed:
		return lipgloss.NewStyle().Foreground(v.sty.Red).Render(uistyles.ToolError)
	case smithers.TaskStateCancelled:
		return lipgloss.NewStyle().Foreground(v.sty.Yellow).Render("○")
	case smithers.TaskStateSkipped:
		return lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render("⊘")
	case smithers.TaskStateBlocked:
		return lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render("⊗")
	default:
		return lipgloss.NewStyle().Foreground(v.sty.FgMuted).Render("○")
	}
}

func (v *WorkflowRunView) ensureSelectedInspection(force bool) tea.Cmd {
	run := v.selectedRun()
	if run == nil {
		return nil
	}
	if !force {
		if v.inspecting[run.RunID] {
			return nil
		}
		if _, ok := v.inspections[run.RunID]; ok {
			return nil
		}
	}
	v.inspecting[run.RunID] = true
	delete(v.inspectionErr, run.RunID)
	return v.inspectRunCmd(run.RunID)
}

func (v *WorkflowRunView) ensureSelectedLogs(force bool) tea.Cmd {
	run := v.selectedRun()
	task := v.selectedTask()
	if run == nil || task == nil {
		return nil
	}

	key := v.logKey(run.RunID, *task)
	cache, ok := v.logs[key]
	if ok && cache.loading {
		return nil
	}
	if ok && cache.loaded && !force {
		return nil
	}

	v.logs[key] = workflowTaskLog{
		key:     key,
		runID:   run.RunID,
		nodeID:  task.NodeID,
		attempt: taskAttempt(*task),
		loading: true,
	}
	v.syncLogViewer()
	return v.loadTaskLogsCmd(run.RunID, *task)
}

func (v *WorkflowRunView) syncLogViewer() {
	run := v.selectedRun()
	if run == nil {
		v.logViewer.SetTitle("Logs")
		v.logViewer.SetPlaceholder("Select a run to inspect logs.")
		return
	}

	if v.inspecting[run.RunID] && v.inspections[run.RunID] == nil {
		v.logViewer.SetTitle("Logs")
		v.logViewer.SetPlaceholder("Loading tasks for " + run.WorkflowName + "...")
		return
	}

	if err := v.inspectionErr[run.RunID]; err != nil && v.inspections[run.RunID] == nil {
		v.logViewer.SetTitle("Logs")
		v.logViewer.SetPlaceholder("Failed to load tasks: " + err.Error())
		return
	}

	task := v.selectedTask()
	if task == nil {
		v.logViewer.SetTitle("Logs")
		v.logViewer.SetPlaceholder("Select a task and press Enter to load logs.")
		return
	}

	v.logViewer.SetTitle(taskLabel(*task))
	cacheKey := v.logKey(run.RunID, *task)
	cache, ok := v.logs[cacheKey]
	if !ok {
		v.logViewer.SetPlaceholder("Press Enter to load logs.")
		return
	}
	if cache.loading {
		v.logViewer.SetPlaceholder("Loading logs...")
		return
	}
	if cache.err != nil {
		v.logViewer.SetPlaceholder("Failed to load logs: " + cache.err.Error())
		return
	}

	lines := v.buildLogLines(*task, cache.blocks)
	if len(lines) == 0 {
		v.logViewer.SetPlaceholder("No logs available for this task.")
		return
	}
	v.logViewer.SetLines(lines)
}

func (v *WorkflowRunView) buildLogLines(task smithers.RunTask, blocks []smithers.ChatBlock) []components.LogLine {
	if len(blocks) == 0 {
		return nil
	}

	width := max(24, v.width/2)
	lines := make([]components.LogLine, 0, len(blocks)*4)

	for i, block := range blocks {
		header := strings.ToUpper(string(block.Role))
		if header == "" {
			header = "EVENT"
		}
		headerLine := header
		if block.Attempt >= 0 {
			headerLine += fmt.Sprintf(" · attempt %d", block.Attempt+1)
		}
		lines = append(lines, components.LogLine{Text: headerLine})

		rendered := renderChatBlock(v.sty, block, width)
		for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
			if line == "" {
				lines = append(lines, components.LogLine{Text: ""})
				continue
			}
			lines = append(lines, components.LogLine{
				Text:  "  " + line,
				Error: task.State == smithers.TaskStateFailed && shouldHighlightError(block, line),
			})
		}

		if i < len(blocks)-1 {
			lines = append(lines, components.LogLine{Text: ""})
		}
	}

	return lines
}

func (v *WorkflowRunView) shouldAnimate() bool {
	if v.loading {
		return true
	}
	for _, r := range v.runs {
		if r.Status == smithers.RunStatusRunning {
			return true
		}
	}
	for _, loading := range v.inspecting {
		if loading {
			return true
		}
	}
	for _, logState := range v.logs {
		if logState.loading {
			return true
		}
	}
	if inspection := v.currentInspection(); inspection != nil {
		for _, task := range inspection.Tasks {
			if task.State == smithers.TaskStateRunning {
				return true
			}
		}
	}
	return false
}

func (v *WorkflowRunView) applyRunEvent(ev smithers.RunEvent) string {
	eventType := normalizeEventType(ev.Type)
	run := v.findRun(ev.RunID)
	if run == nil && ev.RunID != "" {
		selectedRunID := v.currentRunID()
		stub := smithers.RunSummary{
			RunID:        ev.RunID,
			Status:       runStatusFromEvent(eventType, ev.Status),
			StartedAtMs:  timestampPtr(ev.TimestampMs),
			FinishedAtMs: nil,
		}
		if stub.Status == "" {
			stub.Status = smithers.RunStatusRunning
		}
		v.runs = append([]smithers.RunSummary{stub}, v.runs...)
		if selectedRunID != "" {
			v.restoreRunSelection(selectedRunID)
		}
		return ev.RunID
	}
	if run == nil {
		return ""
	}

	switch eventType {
	case "runstarted":
		run.Status = smithers.RunStatusRunning
		if run.StartedAtMs == nil && ev.TimestampMs > 0 {
			run.StartedAtMs = timestampPtr(ev.TimestampMs)
		}

	case "runstatuschanged":
		if status := runStatusFromString(ev.Status); status != "" {
			run.Status = status
			if status.IsTerminal() && ev.TimestampMs > 0 {
				run.FinishedAtMs = timestampPtr(ev.TimestampMs)
			}
		}

	case "runfinished":
		run.Status = smithers.RunStatusFinished
		if ev.TimestampMs > 0 {
			run.FinishedAtMs = timestampPtr(ev.TimestampMs)
		}

	case "runfailed":
		run.Status = smithers.RunStatusFailed
		if ev.TimestampMs > 0 {
			run.FinishedAtMs = timestampPtr(ev.TimestampMs)
		}

	case "runcancelled":
		run.Status = smithers.RunStatusCancelled
		if ev.TimestampMs > 0 {
			run.FinishedAtMs = timestampPtr(ev.TimestampMs)
		}

	case "nodewaitingapproval":
		run.Status = smithers.RunStatusWaitingApproval
		v.applyTaskState(ev, smithers.TaskStateBlocked)

	case "nodestatechanged":
		v.applyTaskState(ev, taskStateFromString(ev.Status))

	case "nodestarted":
		v.applyTaskState(ev, smithers.TaskStateRunning)

	case "nodefinished":
		v.applyTaskState(ev, smithers.TaskStateFinished)

	case "nodefailed":
		v.applyTaskState(ev, smithers.TaskStateFailed)

	case "nodecancelled":
		v.applyTaskState(ev, smithers.TaskStateCancelled)

	case "nodeskipped":
		v.applyTaskState(ev, smithers.TaskStateSkipped)

	case "nodeblocked":
		v.applyTaskState(ev, smithers.TaskStateBlocked)
	}

	v.clampCursors()
	return ""
}

func (v *WorkflowRunView) applyTaskState(ev smithers.RunEvent, state smithers.TaskState) {
	if state == "" {
		return
	}
	insp := v.inspections[ev.RunID]
	if insp == nil {
		return
	}

	for i := range insp.Tasks {
		if insp.Tasks[i].NodeID != ev.NodeID {
			continue
		}
		if ev.Iteration != 0 && insp.Tasks[i].Iteration != ev.Iteration {
			continue
		}
		insp.Tasks[i].State = state
		if ev.TimestampMs > 0 {
			insp.Tasks[i].UpdatedAtMs = timestampPtr(ev.TimestampMs)
		}
		attempt := ev.Attempt
		insp.Tasks[i].LastAttempt = &attempt
		return
	}

	task := smithers.RunTask{
		NodeID:    ev.NodeID,
		Iteration: ev.Iteration,
		State:     state,
	}
	if ev.Attempt >= 0 {
		attempt := ev.Attempt
		task.LastAttempt = &attempt
	}
	if ev.TimestampMs > 0 {
		task.UpdatedAtMs = timestampPtr(ev.TimestampMs)
	}
	insp.Tasks = append(insp.Tasks, task)
}

func (v *WorkflowRunView) selectedRun() *smithers.RunSummary {
	if len(v.runs) == 0 {
		return nil
	}
	v.runCursor = clampIndex(v.runCursor, len(v.runs))
	return &v.runs[v.runCursor]
}

func (v *WorkflowRunView) currentTasks() []smithers.RunTask {
	insp := v.currentInspection()
	if insp == nil {
		return nil
	}
	return insp.Tasks
}

func (v *WorkflowRunView) currentInspection() *smithers.RunInspection {
	run := v.selectedRun()
	if run == nil {
		return nil
	}
	return v.inspections[run.RunID]
}

func (v *WorkflowRunView) selectedTask() *smithers.RunTask {
	insp := v.currentInspection()
	if insp == nil || len(insp.Tasks) == 0 {
		return nil
	}
	v.taskCursor = clampIndex(v.taskCursor, len(insp.Tasks))
	return &insp.Tasks[v.taskCursor]
}

func (v *WorkflowRunView) currentRunID() string {
	run := v.selectedRun()
	if run == nil {
		return ""
	}
	return run.RunID
}

func (v *WorkflowRunView) restoreRunSelection(runID string) {
	if runID == "" {
		v.runCursor = clampIndex(v.runCursor, len(v.runs))
		return
	}
	for i := range v.runs {
		if v.runs[i].RunID == runID {
			v.runCursor = i
			return
		}
	}
	v.runCursor = clampIndex(v.runCursor, len(v.runs))
}

func (v *WorkflowRunView) clampCursors() {
	v.runCursor = clampIndex(v.runCursor, len(v.runs))
	tasks := v.currentTasks()
	v.taskCursor = clampIndex(v.taskCursor, len(tasks))
}

func (v *WorkflowRunView) findRun(runID string) *smithers.RunSummary {
	for i := range v.runs {
		if v.runs[i].RunID == runID {
			return &v.runs[i]
		}
	}
	return nil
}

func (v *WorkflowRunView) logKey(runID string, task smithers.RunTask) string {
	return fmt.Sprintf("%s:%s:%d", runID, task.NodeID, taskAttempt(task))
}

func (v *WorkflowRunView) nextPane() workflowPane {
	switch v.focus {
	case workflowPaneRuns:
		return workflowPaneTasks
	case workflowPaneTasks:
		return workflowPaneLogs
	default:
		return workflowPaneRuns
	}
}

func (v *WorkflowRunView) prevPane() workflowPane {
	switch v.focus {
	case workflowPaneRuns:
		return workflowPaneLogs
	case workflowPaneTasks:
		return workflowPaneRuns
	default:
		return workflowPaneTasks
	}
}

func (v *WorkflowRunView) toggleZoom() {
	if v.zoomedPane != nil && *v.zoomedPane == v.focus {
		v.zoomedPane = nil
		return
	}
	pane := v.focus
	v.zoomedPane = &pane
}

func renderChatBlock(sty uistyles.Styles, block smithers.ChatBlock, width int) string {
	content := ansi.Strip(block.Content)
	if block.Role != smithers.ChatRoleAssistant {
		return content
	}

	rendered, err := common.MarkdownRenderer(&sty, width).Render(block.Content)
	if err != nil {
		return content
	}
	return ansi.Strip(rendered)
}

func shouldHighlightError(block smithers.ChatBlock, line string) bool {
	if block.Role == smithers.ChatRoleUser || block.Role == smithers.ChatRoleSystem {
		return false
	}
	return workflowErrorPattern.MatchString(ansi.Strip(line))
}

func filterTaskBlocks(blocks []smithers.ChatBlock, nodeID string, attempt int) []smithers.ChatBlock {
	if len(blocks) == 0 {
		return nil
	}

	filtered := make([]smithers.ChatBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.NodeID != nodeID {
			continue
		}
		if attempt >= 0 && block.Attempt != attempt {
			continue
		}
		filtered = append(filtered, block)
	}
	if len(filtered) > 0 || attempt < 0 {
		return filtered
	}

	for _, block := range blocks {
		if block.NodeID == nodeID {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func taskLabel(task smithers.RunTask) string {
	if task.Label != nil && *task.Label != "" {
		return *task.Label
	}
	return task.NodeID
}

func taskAttempt(task smithers.RunTask) int {
	if task.LastAttempt == nil {
		return -1
	}
	return *task.LastAttempt
}

func taskAttemptLabel(task smithers.RunTask) string {
	if task.LastAttempt == nil {
		return ""
	}
	return fmt.Sprintf("#%d", *task.LastAttempt+1)
}

func runElapsed(run smithers.RunSummary) string {
	if run.StartedAtMs == nil {
		return ""
	}
	start := time.UnixMilli(*run.StartedAtMs)
	end := time.Now()
	if run.FinishedAtMs != nil {
		end = time.UnixMilli(*run.FinishedAtMs)
	}
	elapsed := end.Sub(start).Round(time.Second)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60

	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func runProgress(run smithers.RunSummary) string {
	total := run.Summary["total"]
	if total <= 0 {
		return ""
	}
	done := run.Summary["finished"] + run.Summary["failed"] + run.Summary["cancelled"]
	if done > total {
		done = total
	}
	return fmt.Sprintf("%d/%d", done, total)
}

func truncateText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func clampIndex(index, total int) int {
	if total <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= total {
		return total - 1
	}
	return index
}

func windowForCursor(cursor, total, height int) (int, int) {
	if total <= 0 || height <= 0 {
		return 0, 0
	}
	if total <= height {
		return 0, total
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start > total-height {
		start = total - height
	}
	return start, min(total, start+height)
}

func normalizeEventType(value string) string {
	token := strings.ToLower(strings.TrimSpace(value))
	token = strings.ReplaceAll(token, "_", "")
	token = strings.ReplaceAll(token, "-", "")
	token = strings.ReplaceAll(token, " ", "")
	return token
}

func runStatusFromEvent(eventType, status string) smithers.RunStatus {
	if normalized := runStatusFromString(status); normalized != "" {
		return normalized
	}
	switch eventType {
	case "runfinished":
		return smithers.RunStatusFinished
	case "runfailed":
		return smithers.RunStatusFailed
	case "runcancelled":
		return smithers.RunStatusCancelled
	case "nodewaitingapproval":
		return smithers.RunStatusWaitingApproval
	default:
		return smithers.RunStatusRunning
	}
}

func runStatusFromString(value string) smithers.RunStatus {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-")) {
	case "running":
		return smithers.RunStatusRunning
	case "waiting-approval":
		return smithers.RunStatusWaitingApproval
	case "waiting-event":
		return smithers.RunStatusWaitingEvent
	case "finished":
		return smithers.RunStatusFinished
	case "failed":
		return smithers.RunStatusFailed
	case "cancelled":
		return smithers.RunStatusCancelled
	default:
		return ""
	}
}

func taskStateFromString(value string) smithers.TaskState {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-")) {
	case "pending":
		return smithers.TaskStatePending
	case "running":
		return smithers.TaskStateRunning
	case "finished":
		return smithers.TaskStateFinished
	case "failed":
		return smithers.TaskStateFailed
	case "cancelled":
		return smithers.TaskStateCancelled
	case "skipped":
		return smithers.TaskStateSkipped
	case "blocked":
		return smithers.TaskStateBlocked
	default:
		return ""
	}
}

func timestampPtr(ts int64) *int64 {
	if ts <= 0 {
		return nil
	}
	value := ts
	return &value
}

func (p workflowPane) String() string {
	switch p {
	case workflowPaneRuns:
		return "runs"
	case workflowPaneTasks:
		return "tasks"
	default:
		return "logs"
	}
}

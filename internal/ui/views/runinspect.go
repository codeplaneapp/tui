package views

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*RunInspectView)(nil)

// --- Message types ---

// OpenRunInspectMsg signals ui.go to push a RunInspectView for the given run.
type OpenRunInspectMsg struct {
	RunID string
}

// OpenLiveChatMsg signals ui.go to push a LiveChatView for the given run/node.
type OpenLiveChatMsg struct {
	RunID     string
	TaskID    string // optional: filter display to a single node's chat
	AgentName string // optional display hint for the sub-header
}

// --- Internal async messages ---

type runInspectLoadedMsg struct {
	inspection *smithers.RunInspection
}

type runInspectErrorMsg struct {
	err error
}

// runInspectHijackSessionMsg is returned when HijackRun completes (success or error).
type runInspectHijackSessionMsg struct {
	runID   string
	session *smithers.HijackSession
	err     error
}

// runInspectHijackReturnMsg is returned after the hijacked CLI process exits.
type runInspectHijackReturnMsg struct {
	runID string
	err   error
}

// --- RunInspectView ---

// dagViewMode tracks which sub-view is active in the run inspector.
type dagViewMode int

const (
	dagViewModeList dagViewMode = iota // default: flat node list
	dagViewModeDAG                     // tree/DAG visualization
)

// RunInspectView shows detailed run metadata and a per-node task list.
type RunInspectView struct {
	client     *smithers.Client
	runID      string
	inspection *smithers.RunInspection

	cursor  int
	width   int
	height  int
	loading bool
	err     error

	// DAG view toggle
	viewMode  dagViewMode
	dagCursor int // selected node index in DAG view

	// Hijack state
	hijacking bool
	hijackErr error
}

// NewRunInspectView constructs a new inspector for the given run ID.
func NewRunInspectView(client *smithers.Client, runID string) *RunInspectView {
	return &RunInspectView{
		client:  client,
		runID:   runID,
		loading: true,
	}
}

// Init dispatches an async Cmd that calls client.InspectRun.
func (v *RunInspectView) Init() tea.Cmd {
	runID := v.runID
	client := v.client
	return func() tea.Msg {
		inspection, err := client.InspectRun(context.Background(), runID)
		if err != nil {
			return runInspectErrorMsg{err: err}
		}
		return runInspectLoadedMsg{inspection: inspection}
	}
}

// hijackRunCmd calls HijackRun for the given runID and returns a
// runInspectHijackSessionMsg with the session or error.
func (v *RunInspectView) hijackRunCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		session, err := client.HijackRun(context.Background(), runID)
		return runInspectHijackSessionMsg{runID: runID, session: session, err: err}
	}
}

// Update handles messages for the run inspect view.
func (v *RunInspectView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case runInspectLoadedMsg:
		v.inspection = msg.inspection
		v.loading = false
		// Clamp cursor to valid range.
		if len(v.inspection.Tasks) > 0 && v.cursor >= len(v.inspection.Tasks) {
			v.cursor = len(v.inspection.Tasks) - 1
		}
		return v, nil

	case runInspectErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	// --- Hijack flow ---

	case runInspectHijackSessionMsg:
		v.hijacking = false
		if msg.err != nil {
			v.hijackErr = msg.err
			return v, nil
		}
		s := msg.session
		// Validate binary exists before suspending the TUI.
		if _, lookErr := exec.LookPath(s.AgentBinary); lookErr != nil {
			v.hijackErr = fmt.Errorf("cannot hijack: %s binary not found (%s). Install it or check PATH", s.AgentEngine, s.AgentBinary)
			return v, nil
		}
		cmd := exec.Command(s.AgentBinary, s.ResumeArgs()...) //nolint:gosec
		if s.CWD != "" {
			cmd.Dir = s.CWD
		}
		runID := msg.runID
		return v, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return runInspectHijackReturnMsg{runID: runID, err: err}
		})

	case runInspectHijackReturnMsg:
		v.hijacking = false
		v.hijackErr = msg.err
		// Refresh run data after returning from the hijacked session.
		v.loading = true
		return v, v.Init()

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			// Switch to DAG view; sync dagCursor from list cursor.
			v.viewMode = dagViewModeDAG
			v.dagCursor = v.cursor

		case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
			// Switch to list view; sync list cursor from dagCursor.
			v.viewMode = dagViewModeList
			v.cursor = v.dagCursor

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.viewMode == dagViewModeDAG {
				if v.dagCursor > 0 {
					v.dagCursor--
				}
			} else {
				if v.cursor > 0 {
					v.cursor--
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.viewMode == dagViewModeDAG {
				if v.inspection != nil && v.dagCursor < len(v.inspection.Tasks)-1 {
					v.dagCursor++
				}
			} else {
				if v.inspection != nil && v.cursor < len(v.inspection.Tasks)-1 {
					v.cursor++
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			// Hijack the run shown by this inspector.
			if !v.hijacking {
				v.hijacking = true
				v.hijackErr = nil
				return v, v.hijackRunCmd(v.runID)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.err = nil
			return v, v.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			// Use the active-mode cursor to select the task.
			activeCursor := v.cursor
			if v.viewMode == dagViewModeDAG {
				activeCursor = v.dagCursor
			}
			if v.inspection != nil && len(v.inspection.Tasks) > 0 {
				task := v.inspection.Tasks[activeCursor]
				taskID := task.NodeID
				runID := v.runID
				return v, func() tea.Msg {
					return OpenLiveChatMsg{
						RunID:     runID,
						TaskID:    taskID,
						AgentName: "",
					}
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			if v.runID == "" {
				break
			}
			runID := v.runID
			return v, func() tea.Msg {
				return OpenSnapshotsMsg{
					RunID:  runID,
					Source: SnapshotsOpenSourceRunInspect,
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Enter on a task row opens the detailed node inspector.
			activeCursor := v.cursor
			if v.viewMode == dagViewModeDAG {
				activeCursor = v.dagCursor
			}
			if v.inspection != nil && len(v.inspection.Tasks) > 0 &&
				activeCursor >= 0 && activeCursor < len(v.inspection.Tasks) {
				task := v.inspection.Tasks[activeCursor]
				runID := v.runID
				return v, func() tea.Msg {
					return OpenNodeInspectMsg{
						RunID:  runID,
						NodeID: task.NodeID,
						Task:   task,
					}
				}
			}
		}
	}
	return v, nil
}

// View renders the run inspect view.
func (v *RunInspectView) View() string {
	var b strings.Builder

	b.WriteString(v.renderHeader())
	b.WriteString("\n")

	// Hijack overlay: show status while waiting or on error.
	if v.hijacking {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Hijacking session..."))
		b.WriteString("\n")
		return b.String()
	}
	if v.hijackErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
			fmt.Sprintf("  Hijack error: %v", v.hijackErr)))
		b.WriteString("\n")
	}

	if v.loading {
		b.WriteString("  Loading run...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	b.WriteString(v.renderSubHeader())
	b.WriteString("\n")
	b.WriteString(v.renderDivider())
	b.WriteString("\n")

	if v.inspection == nil || len(v.inspection.Tasks) == 0 {
		b.WriteString("  No nodes found.\n")
	} else if v.viewMode == dagViewModeDAG {
		b.WriteString(v.renderDAGView())
	} else {
		b.WriteString(v.renderNodeList())
	}

	b.WriteString(v.renderHelpBar())
	return b.String()
}

// Name returns the view name for the router.
func (v *RunInspectView) Name() string {
	return "runinspect"
}

// SetSize stores terminal dimensions for use during rendering.
func (v *RunInspectView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *RunInspectView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "node detail")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "dag view")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "list view")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "snapshots")),
		key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hijack")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	}
}

// --- Rendering helpers ---

func (v *RunInspectView) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)

	runPart := v.runID
	if len(runPart) > 8 {
		runPart = runPart[:8]
	}

	title := "SMITHERS › Runs › " + runPart
	if v.inspection != nil && v.inspection.WorkflowName != "" {
		title += " (" + v.inspection.WorkflowName + ")"
	}

	header := titleStyle.Render(title)
	hint := hintStyle.Render("[Esc] Back")

	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(hint) - 2
		if gap > 0 {
			return header + strings.Repeat(" ", gap) + hint
		}
	}
	return header
}

func (v *RunInspectView) renderSubHeader() string {
	if v.inspection == nil {
		return ""
	}
	faint := lipgloss.NewStyle().Faint(true)
	var parts []string

	// Status (color-coded).
	statusStr := string(v.inspection.Status)
	styledStatus := taskInspectStatusStyle(v.inspection.Status).Render(statusStr)
	parts = append(parts, "Status: "+styledStatus)

	// Started elapsed time.
	if v.inspection.StartedAtMs != nil {
		elapsed := time.Since(time.UnixMilli(*v.inspection.StartedAtMs)).Round(time.Second)
		parts = append(parts, "Started: "+elapsed.String()+" ago")
	}

	// Node progress from Summary map or task count.
	nodes := v.fmtNodeProgress()
	if nodes != "" {
		parts = append(parts, "Nodes: "+nodes)
	}

	// Terminal indicator.
	if v.inspection.Status.IsTerminal() {
		switch v.inspection.Status {
		case smithers.RunStatusFinished:
			parts = append(parts, "✓ DONE")
		case smithers.RunStatusFailed:
			parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ FAILED"))
		default:
			parts = append(parts, "DONE")
		}
	} else {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● LIVE"))
	}

	return faint.Render(strings.Join(parts, " │ "))
}

// fmtNodeProgress computes "completed/total" from Summary or task slice.
func (v *RunInspectView) fmtNodeProgress() string {
	if v.inspection == nil {
		return ""
	}
	// Prefer the Summary map on RunSummary.
	if len(v.inspection.Summary) > 0 {
		completed := v.inspection.Summary["finished"] + v.inspection.Summary["failed"] + v.inspection.Summary["cancelled"]
		total := v.inspection.Summary["total"]
		if total > 0 {
			return fmt.Sprintf("%d/%d", completed, total)
		}
	}
	// Fall back to Tasks slice.
	if len(v.inspection.Tasks) > 0 {
		var done int
		for _, t := range v.inspection.Tasks {
			switch t.State {
			case smithers.TaskStateFinished, smithers.TaskStateFailed, smithers.TaskStateCancelled:
				done++
			}
		}
		return fmt.Sprintf("%d/%d", done, len(v.inspection.Tasks))
	}
	return ""
}

func (v *RunInspectView) renderDivider() string {
	w := v.width
	if w <= 0 {
		w = 40
	}
	return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", w))
}

// visibleHeight returns the number of node-list rows that fit in the terminal.
// Reserves: header(1) + sub-header(1) + divider(1) + help bar(1) + 1 margin = 5.
func (v *RunInspectView) visibleHeight() int {
	h := v.height - 5
	if h < 4 {
		return 4
	}
	return h
}

func (v *RunInspectView) renderNodeList() string {
	tasks := v.inspection.Tasks
	if len(tasks) == 0 {
		return ""
	}

	visible := v.visibleHeight()
	start := 0
	if len(tasks) > visible {
		start = v.cursor - visible/2
		if start < 0 {
			start = 0
		}
		if start+visible > len(tasks) {
			start = len(tasks) - visible
		}
	}
	end := start + visible
	if end > len(tasks) {
		end = len(tasks)
	}

	// Compute flex width for the label column.
	// Layout: cursor(2) + glyph(2) + label(flex) + gap(2) + stateText(12) + gap(2) + attempt(4) + gap(2) + elapsed(8)
	const (
		cursorW  = 2
		glyphW   = 2
		stateW   = 12
		attemptW = 4
		elapsedW = 8
		gapW     = 2
	)
	fixed := cursorW + glyphW + stateW + gapW + attemptW + gapW + elapsedW
	labelW := v.width - fixed - gapW
	if labelW < 8 {
		labelW = 8
	}

	reverseStyle := lipgloss.NewStyle().Reverse(true)
	var b strings.Builder

	for i := start; i < end; i++ {
		task := tasks[i]

		cursor := "  "
		if i == v.cursor {
			cursor = "▸ "
		}

		glyph, glyphStyle := taskGlyphAndStyle(task.State)
		styledGlyph := glyphStyle.Render(glyph + " ")

		label := task.NodeID
		if task.Label != nil && *task.Label != "" {
			label = *task.Label
		}
		if len(label) > labelW {
			if labelW > 3 {
				label = label[:labelW-3] + "..."
			} else {
				label = label[:labelW]
			}
		}

		stateText := string(task.State)
		styledState := glyphStyle.Render(fmt.Sprintf("%-*s", stateW, stateText))

		attemptStr := "    "
		if task.LastAttempt != nil && *task.LastAttempt > 0 {
			attemptStr = fmt.Sprintf("#%-*d", attemptW-1, *task.LastAttempt)
		}

		elapsedStr := "—       "
		if task.UpdatedAtMs != nil {
			elapsed := time.Since(time.UnixMilli(*task.UpdatedAtMs)).Round(time.Second)
			elapsedStr = fmt.Sprintf("%-*s", elapsedW, elapsed.String())
		}

		line := cursor + styledGlyph + fmt.Sprintf("%-*s", labelW, label) +
			"  " + styledState +
			"  " + attemptStr +
			"  " + elapsedStr

		if i == v.cursor {
			line = reverseStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// renderDAGView renders the task dependency graph using lipgloss/v2/tree.
//
// Since the RunTask data model does not carry explicit edge information, the
// graph is presented as a flat tree rooted at the workflow name (or run ID)
// with every task as a direct child.  Each node is labelled with:
//
//	<status-glyph> <label> [<agent-attempt>]
//
// The currently-selected node is highlighted.  Below the tree a detail panel
// shows full metadata for the selected task.
func (v *RunInspectView) renderDAGView() string {
	if v.inspection == nil || len(v.inspection.Tasks) == 0 {
		return ""
	}
	tasks := v.inspection.Tasks

	// Clamp dagCursor.
	if v.dagCursor >= len(tasks) {
		v.dagCursor = len(tasks) - 1
	}
	if v.dagCursor < 0 {
		v.dagCursor = 0
	}

	// Build node labels for the tree.
	selectedStyle := lipgloss.NewStyle().Reverse(true)
	children := make([]any, 0, len(tasks))
	for i, task := range tasks {
		glyph, glyphStyle := taskGlyphAndStyle(task.State)

		label := task.NodeID
		if task.Label != nil && *task.Label != "" {
			label = *task.Label
		}

		attemptSuffix := ""
		if task.LastAttempt != nil && *task.LastAttempt > 0 {
			attemptSuffix = fmt.Sprintf(" #%d", *task.LastAttempt)
		}

		nodeLabel := glyphStyle.Render(glyph) + " " + label + attemptSuffix

		if i == v.dagCursor {
			nodeLabel = selectedStyle.Render(nodeLabel)
		}

		children = append(children, nodeLabel)
	}

	// Determine root label (workflow name or run ID).
	rootLabel := v.runID
	if len(rootLabel) > 8 {
		rootLabel = rootLabel[:8]
	}
	if v.inspection.WorkflowName != "" {
		rootLabel = v.inspection.WorkflowName
	}

	// Build the lipgloss tree.
	t := tree.Root(rootLabel).Child(children...)

	treeStr := t.String()

	// Detail panel for selected task.
	detail := v.renderDAGTaskDetail(tasks[v.dagCursor])

	var b strings.Builder
	b.WriteString(treeStr)
	b.WriteString("\n")
	if detail != "" {
		b.WriteString(v.renderDivider())
		b.WriteString("\n")
		b.WriteString(detail)
		b.WriteString("\n")
	}
	return b.String()
}

// renderDAGTaskDetail renders a compact detail panel for the given task,
// shown below the DAG tree when a node is selected.
func (v *RunInspectView) renderDAGTaskDetail(task smithers.RunTask) string {
	faint := lipgloss.NewStyle().Faint(true)
	_, glyphStyle := taskGlyphAndStyle(task.State)

	label := task.NodeID
	if task.Label != nil && *task.Label != "" {
		label = *task.Label
	}

	stateStr := glyphStyle.Render(string(task.State))

	var parts []string
	parts = append(parts, "Node: "+label)
	parts = append(parts, "ID: "+faint.Render(task.NodeID))
	parts = append(parts, "State: "+stateStr)

	if task.LastAttempt != nil && *task.LastAttempt > 0 {
		parts = append(parts, fmt.Sprintf("Attempt: #%d", *task.LastAttempt))
	}

	if task.UpdatedAtMs != nil {
		elapsed := time.Since(time.UnixMilli(*task.UpdatedAtMs)).Round(time.Second)
		parts = append(parts, "Updated: "+elapsed.String()+" ago")
	}

	return "  " + strings.Join(parts, "  │  ")
}

// renderHelpBar returns a one-line help string built from ShortHelp bindings.
func (v *RunInspectView) renderHelpBar() string {
	var parts []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", h.Key, h.Desc))
		}
	}
	return lipgloss.NewStyle().Faint(true).Render("  "+strings.Join(parts, "  ")) + "\n"
}

// --- Style helpers ---

// taskGlyphAndStyle returns the display glyph and lipgloss style for a TaskState.
func taskGlyphAndStyle(state smithers.TaskState) (glyph string, style lipgloss.Style) {
	switch state {
	case smithers.TaskStateRunning:
		return "●", lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case smithers.TaskStateFinished:
		return "●", lipgloss.NewStyle().Faint(true)
	case smithers.TaskStateFailed:
		return "●", lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	case smithers.TaskStatePending:
		return "○", lipgloss.NewStyle().Faint(true)
	case smithers.TaskStateCancelled:
		return "–", lipgloss.NewStyle().Faint(true).Strikethrough(true)
	case smithers.TaskStateSkipped:
		return "↷", lipgloss.NewStyle().Faint(true)
	case smithers.TaskStateBlocked:
		return "⏸", lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default:
		return "○", lipgloss.NewStyle().Faint(true)
	}
}

// taskInspectStatusStyle returns a lipgloss style for a run status in the sub-header.
func taskInspectStatusStyle(status smithers.RunStatus) lipgloss.Style {
	switch status {
	case smithers.RunStatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	case smithers.RunStatusWaitingEvent:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Faint(true)
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Faint(true).Strikethrough(true)
	default:
		return lipgloss.NewStyle()
	}
}

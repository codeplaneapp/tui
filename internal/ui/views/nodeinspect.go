package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*NodeInspectView)(nil)

// OpenNodeInspectMsg signals the router to push a NodeInspectView for the given task.
type OpenNodeInspectMsg struct {
	RunID  string
	NodeID string
	Task   smithers.RunTask
}

// NodeInspectView shows detailed metadata for a single task/node within a run.
// It is pushed onto the stack when the user presses Enter on a task row in
// RunInspectView.
type NodeInspectView struct {
	client *smithers.Client
	runID  string
	task   smithers.RunTask

	width  int
	height int
}

// NewNodeInspectView constructs a NodeInspectView for the given task.
func NewNodeInspectView(client *smithers.Client, runID string, task smithers.RunTask) *NodeInspectView {
	return &NodeInspectView{
		client: client,
		runID:  runID,
		task:   task,
	}
}

// Init is a no-op: all data is passed in at construction time.
func (v *NodeInspectView) Init() tea.Cmd {
	return nil
}

// Update handles messages for the node inspect view.
func (v *NodeInspectView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			// Open live chat for this task.
			runID := v.runID
			taskID := v.task.NodeID
			return v, func() tea.Msg {
				return OpenLiveChatMsg{
					RunID:  runID,
					TaskID: taskID,
				}
			}
		}
	}
	return v, nil
}

// View renders the node inspect view.
func (v *NodeInspectView) View() string {
	var b strings.Builder

	b.WriteString(v.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(v.renderNodeDetail())
	b.WriteString(v.renderHelpBar())

	return b.String()
}

// Name returns the view name for the router.
func (v *NodeInspectView) Name() string {
	return "nodeinspect"
}

// SetSize stores terminal dimensions for use during rendering.
func (v *NodeInspectView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *NodeInspectView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	}
}

// --- Rendering helpers ---

func (v *NodeInspectView) renderHeader() string {
	runPart := v.runID
	if len(runPart) > 8 {
		runPart = runPart[:8]
	}

	label := v.task.NodeID
	if v.task.Label != nil && *v.task.Label != "" {
		label = *v.task.Label
	}

	viewName := "Runs › " + runPart + " › " + label
	return ViewHeader(packageCom.Styles, "SMITHERS", viewName, v.width, "[Esc] Back")
}

func (v *NodeInspectView) renderNodeDetail() string {
	var b strings.Builder
	labelStyle := lipgloss.NewStyle().Faint(true)
	titleStyle := lipgloss.NewStyle().Bold(true)

	task := v.task
	glyph, glyphStyle := taskGlyphAndStyle(task.State)

	label := task.NodeID
	if task.Label != nil && *task.Label != "" {
		label = *task.Label
	}

	b.WriteString(titleStyle.Render("Node") + "\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", glyphStyle.Render(glyph), label))
	b.WriteString(labelStyle.Render(fmt.Sprintf("  ID: %s\n", task.NodeID)))
	b.WriteString("\n")

	b.WriteString(titleStyle.Render("State") + "\n")
	b.WriteString(fmt.Sprintf("  %s\n", glyphStyle.Render(string(task.State))))
	b.WriteString("\n")

	if task.LastAttempt != nil && *task.LastAttempt > 0 {
		b.WriteString(titleStyle.Render("Attempt") + "\n")
		b.WriteString(fmt.Sprintf("  #%d\n", *task.LastAttempt))
		b.WriteString("\n")
	}

	if task.UpdatedAtMs != nil {
		elapsed := time.Since(time.UnixMilli(*task.UpdatedAtMs)).Round(time.Second)
		b.WriteString(titleStyle.Render("Last Updated") + "\n")
		b.WriteString(fmt.Sprintf("  %s ago\n", elapsed.String()))
		b.WriteString("\n")
	}

	b.WriteString(labelStyle.Render(fmt.Sprintf("  Run: %s\n", v.runID)))

	return b.String()
}

// renderHelpBar returns a one-line help string built from ShortHelp bindings.
func (v *NodeInspectView) renderHelpBar() string {
	var parts []string
	for _, b := range v.ShortHelp() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", h.Key, h.Desc))
		}
	}
	return lipgloss.NewStyle().Faint(true).Render("  "+strings.Join(parts, "  ")) + "\n"
}

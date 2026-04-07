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

var _ View = (*JJHubWorkflowsView)(nil)

type jjhubWorkflowManager interface {
	GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error)
	ListWorkflows(ctx context.Context, limit int) ([]jjhub.Workflow, error)
	TriggerWorkflow(ctx context.Context, workflowID int, ref string) (*jjhub.WorkflowRun, error)
}

type (
	jjhubWorkflowsLoadedMsg struct {
		workflows []jjhub.Workflow
	}
	jjhubWorkflowsErrorMsg struct {
		err error
	}
	jjhubWorkflowsRepoLoadedMsg struct {
		repo *jjhub.Repo
	}
	jjhubWorkflowRunDoneMsg struct {
		message string
	}
	jjhubWorkflowRunErrorMsg struct {
		err error
	}
)

type jjhubWorkflowPromptState struct {
	active   bool
	input    textinput.Model
	err      error
	ref      string
	workflow *jjhub.Workflow
}

type JJHubWorkflowsView struct {
	client jjhubWorkflowManager
	repo   *jjhub.Repo

	workflows []jjhub.Workflow

	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error

	actionMsg string
	prompt    jjhubWorkflowPromptState
}

func NewJJHubWorkflowsView(_ *smithers.Client) *JJHubWorkflowsView {
	var client jjhubWorkflowManager
	if jjhubAvailable() {
		client = jjhubWorkflowAdapter{client: jjhub.NewClient("")}
	}
	return newJJHubWorkflowsViewWithClient(client)
}

type jjhubWorkflowAdapter struct {
	client *jjhub.Client
}

func (m jjhubWorkflowAdapter) GetCurrentRepo(ctx context.Context) (*jjhub.Repo, error) {
	return m.client.GetCurrentRepo(ctx)
}

func (m jjhubWorkflowAdapter) ListWorkflows(ctx context.Context, limit int) ([]jjhub.Workflow, error) {
	return m.client.ListWorkflows(ctx, limit)
}

func (m jjhubWorkflowAdapter) TriggerWorkflow(ctx context.Context, workflowID int, ref string) (*jjhub.WorkflowRun, error) {
	return m.client.TriggerWorkflow(ctx, workflowID, ref)
}

func newJJHubWorkflowsViewWithClient(client jjhubWorkflowManager) *JJHubWorkflowsView {
	input := textinput.New()
	input.Placeholder = "Git ref"
	input.SetVirtualCursor(true)

	v := &JJHubWorkflowsView{
		client:  client,
		loading: client != nil,
		prompt: jjhubWorkflowPromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

func (v *JJHubWorkflowsView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadWorkflowsCmd(), v.loadRepoCmd())
}

func (v *JJHubWorkflowsView) loadWorkflowsCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		workflows, err := client.ListWorkflows(context.Background(), 100)
		if err != nil {
			return jjhubWorkflowsErrorMsg{err: err}
		}
		return jjhubWorkflowsLoadedMsg{workflows: workflows}
	}
}

func (v *JJHubWorkflowsView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo(context.Background())
		if err != nil {
			return nil
		}
		return jjhubWorkflowsRepoLoadedMsg{repo: repo}
	}
}

func (v *JJHubWorkflowsView) refreshCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = true
	v.err = nil
	return tea.Batch(v.loadWorkflowsCmd(), v.loadRepoCmd())
}

func (v *JJHubWorkflowsView) selectedWorkflow() *jjhub.Workflow {
	if len(v.workflows) == 0 || v.cursor < 0 || v.cursor >= len(v.workflows) {
		return nil
	}
	workflow := v.workflows[v.cursor]
	return &workflow
}

func (v *JJHubWorkflowsView) pageSize() int {
	const (
		headerLines      = 6
		linesPerWorkflow = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / linesPerWorkflow
	if size < 1 {
		return 1
	}
	return size
}

func (v *JJHubWorkflowsView) clampCursor() {
	if len(v.workflows) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(v.workflows) {
		v.cursor = len(v.workflows) - 1
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

func (v *JJHubWorkflowsView) openRunPrompt() tea.Cmd {
	workflow := v.selectedWorkflow()
	if workflow == nil {
		return nil
	}
	defaultRef := "main"
	if v.repo != nil && strings.TrimSpace(v.repo.DefaultBookmark) != "" {
		defaultRef = v.repo.DefaultBookmark
	}
	v.prompt.active = true
	v.prompt.err = nil
	v.prompt.workflow = workflow
	v.prompt.ref = defaultRef
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = "Git ref"
	v.prompt.input.SetValue(defaultRef)
	return v.prompt.input.Focus()
}

func (v *JJHubWorkflowsView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.ref = ""
	v.prompt.workflow = nil
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *JJHubWorkflowsView) triggerWorkflowCmd(workflow jjhub.Workflow, ref string) tea.Cmd {
	client := v.client
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "main"
	}
	return func() tea.Msg {
		run, err := client.TriggerWorkflow(context.Background(), workflow.ID, ref)
		if err != nil {
			return jjhubWorkflowRunErrorMsg{err: err}
		}
		message := fmt.Sprintf("Triggered %s on %s", workflow.Name, ref)
		if run != nil && run.ID > 0 {
			message = fmt.Sprintf("%s (run #%d)", message, run.ID)
		}
		return jjhubWorkflowRunDoneMsg{message: message}
	}
}

func (v *JJHubWorkflowsView) renderPrompt(width int) string {
	boxWidth := max(34, min(max(34, width-4), 80))
	workflowName := "selected workflow"
	if v.prompt.workflow != nil && strings.TrimSpace(v.prompt.workflow.Name) != "" {
		workflowName = v.prompt.workflow.Name
	}

	var body strings.Builder
	body.WriteString(jjhubSectionStyle.Render("Run workflow"))
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render("Choose the git ref to run against."))
	body.WriteString("\n\n")
	body.WriteString(jjhubMetaRow("Workflow", workflowName))
	body.WriteString("\n\n")
	body.WriteString(v.prompt.input.View())
	body.WriteString("\n")
	body.WriteString(jjhubMutedStyle.Render("[Enter] run  [Esc] cancel"))
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

func (v *JJHubWorkflowsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case jjhubWorkflowsLoadedMsg:
		v.workflows = msg.workflows
		v.loading = false
		v.err = nil
		v.clampCursor()
		return v, nil

	case jjhubWorkflowsErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case jjhubWorkflowsRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case jjhubWorkflowRunDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		return v, nil

	case jjhubWorkflowRunErrorMsg:
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
				if v.prompt.workflow == nil {
					v.prompt.err = errors.New("No workflow selected")
					return v, nil
				}
				return v, v.triggerWorkflowCmd(*v.prompt.workflow, v.prompt.input.Value())
			default:
				var cmd tea.Cmd
				v.prompt.input, cmd = v.prompt.input.Update(msg)
				return v, cmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
				v.clampCursor()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.workflows)-1 {
				v.cursor++
				v.clampCursor()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.actionMsg = ""
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if v.selectedWorkflow() == nil {
				return v, nil
			}
			v.actionMsg = ""
			return v, v.openRunPrompt()
		}
	}

	return v, nil
}

func (v *JJHubWorkflowsView) View() string {
	var b strings.Builder
	b.WriteString(jjhubHeader("JJHUB › Workflows", v.width, jjhubJoinNonEmpty("  ",
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

	if v.loading {
		b.WriteString("  Loading workflows...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}
	if len(v.workflows) == 0 {
		b.WriteString("  No JJHub workflows found.\n")
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderWorkflowList(leftWidth))
		right := lipgloss.NewStyle().Width(rightWidth).Render(v.renderWorkflowDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right))
		return b.String()
	}

	b.WriteString(v.renderWorkflowList(v.width))
	b.WriteString("\n\n")
	b.WriteString(v.renderWorkflowDetail(v.width))
	return b.String()
}

func (v *JJHubWorkflowsView) renderWorkflowList(width int) string {
	var b strings.Builder
	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(v.workflows)-1))
	end := min(len(v.workflows), start+pageSize)

	for i := start; i < end; i++ {
		workflow := v.workflows[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		b.WriteString(cursor + titleStyle.Render(truncateStr(fmt.Sprintf("#%d %s", workflow.ID, workflow.Name), max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			jjhubWorkflowStatusLabel(workflow.IsActive),
			truncateStr(workflow.Path, max(8, width-8)),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(meta))
		b.WriteString("\n")

		timestamps := jjhubJoinNonEmpty(" · ",
			jjhubFormatRelativeTime(workflow.UpdatedAt),
			jjhubFormatRelativeTime(workflow.CreatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(timestamps, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (v *JJHubWorkflowsView) renderWorkflowDetail(width int) string {
	workflow := v.selectedWorkflow()
	if workflow == nil {
		return "No workflow selected."
	}

	var b strings.Builder
	b.WriteString(jjhubTitleStyle.Render(workflow.Name))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("ID", fmt.Sprintf("%d", workflow.ID)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Status", jjhubWorkflowStatusLabel(workflow.IsActive)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Path", workflow.Path))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(workflow.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(workflow.UpdatedAt)))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Run"))
	b.WriteString("\n")
	b.WriteString(wrapText("Press Enter to run this workflow and optionally override the git ref.", max(20, width)))
	return b.String()
}

func (v *JJHubWorkflowsView) Name() string { return "jjhub-workflows" }

func (v *JJHubWorkflowsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

func (v *JJHubWorkflowsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func jjhubWorkflowStatusLabel(active bool) string {
	if active {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true).Render("active")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("inactive")
}

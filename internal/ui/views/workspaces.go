package views

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/charmbracelet/crush/internal/ui/handoff"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

var _ View = (*WorkspacesView)(nil)

type workspacesLoadedMsg struct {
	workspaces []jjhub.Workspace
}

type workspacesErrorMsg struct {
	err error
}

type workspaceActionDoneMsg struct {
	action string
	name   string
}

type workspaceActionErrorMsg struct {
	action string
	name   string
	err    error
}

// WorkspacesView renders a JJHub workspaces dashboard.
type WorkspacesView struct {
	smithersClient *smithers.Client
	jjhubClient    *jjhub.Client
	sty            styles.Styles

	width  int
	height int

	loading bool
	err     error

	previewOpen bool
	search      jjSearchState
	searchQuery string

	allWorkspaces []jjhub.Workspace
	workspaces    []jjhub.Workspace

	tablePane   *jjTablePane
	previewPane *jjPreviewPane
	splitPane   *components.SplitPane
}

var workspaceTableColumns = []components.Column{
	{Title: "Name", Width: 18},
	{Title: "Status", Width: 12},
	{Title: "SSH Host", Grow: true, MinWidth: 94},
	{Title: "Fork?", Width: 6, MinWidth: 84},
	{Title: "Idle", Width: 8, MinWidth: 100},
	{Title: "Created", Width: 10, MinWidth: 88},
}

// NewWorkspacesView creates a JJHub workspaces view.
func NewWorkspacesView(client *smithers.Client) *WorkspacesView {
	tablePane := newJJTablePane(workspaceTableColumns)
	previewPane := newJJPreviewPane("Select a workspace")
	splitPane := components.NewSplitPane(tablePane, previewPane, components.SplitPaneOpts{
		LeftWidth:         68,
		CompactBreakpoint: 96,
	})

	return &WorkspacesView{
		smithersClient: client,
		jjhubClient:    jjhub.NewClient(""),
		sty:            styles.DefaultStyles(),
		loading:        true,
		previewOpen:    true,
		search:         newJJSearchInput("filter workspaces by name"),
		tablePane:      tablePane,
		previewPane:    previewPane,
		splitPane:      splitPane,
	}
}

func (v *WorkspacesView) Init() tea.Cmd {
	return v.loadWorkspacesCmd()
}

func (v *WorkspacesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case workspacesLoadedMsg:
		v.loading = false
		v.err = nil
		v.allWorkspaces = msg.workspaces
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)

	case workspacesErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case workspaceActionDoneMsg:
		v.loading = true
		actionTitle := workspaceActionTitle(msg.action)
		toast := func() tea.Msg {
			return components.ShowToastMsg{
				Title: actionTitle + " complete",
				Body:  msg.name,
				Level: components.ToastLevelSuccess,
			}
		}
		return v, tea.Batch(v.Init(), toast)

	case workspaceActionErrorMsg:
		actionTitle := workspaceActionTitle(msg.action)
		return v, func() tea.Msg {
			return components.ShowToastMsg{
				Title: actionTitle + " failed",
				Body:  msg.err.Error(),
				Level: components.ToastLevelError,
			}
		}

	case tea.WindowSizeMsg:
		v.SetSize(msg.Width, msg.Height)
		return v, nil

	case tea.KeyPressMsg:
		if v.search.active {
			return v.updateSearch(msg)
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			return v, func() tea.Msg { return PopViewMsg{} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			v.search.active = true
			v.search.input.SetValue(v.searchQuery)
			return v, v.search.input.Focus()
		case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
			v.previewOpen = !v.previewOpen
			if v.previewOpen {
				return v, v.syncPreview(true)
			}
			return v, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			v.loading = true
			return v, v.Init()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if workspace := v.selectedWorkspace(); workspace != nil {
				return v, v.sshCmd(*workspace)
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			if workspace := v.selectedWorkspace(); workspace != nil {
				return v, v.toggleWorkspaceCmd(*workspace)
			}
		}
	}

	previous := v.selectedWorkspaceName()
	var cmd tea.Cmd
	if v.previewOpen {
		v.tablePane.SetFocused(v.splitPane.Focus() == components.FocusLeft)
		newSplitPane, splitCmd := v.splitPane.Update(msg)
		v.splitPane = newSplitPane
		cmd = splitCmd
	} else {
		v.tablePane.SetFocused(true)
		_, cmd = v.tablePane.Update(msg)
	}

	selectionChanged := previous != v.selectedWorkspaceName()
	return v, tea.Batch(cmd, v.syncPreview(selectionChanged))
}

func (v *WorkspacesView) View() string {
	header := jjRenderHeader(
		fmt.Sprintf("JJHUB › Workspaces (%d)", len(v.workspaces)),
		v.width,
		jjMutedStyle.Render("[/] Search  [w] Preview  [Esc] Back"),
	)

	var parts []string
	parts = append(parts, header)
	if v.search.active {
		parts = append(parts, jjSearchStyle.Render("Search:")+" "+v.search.input.View())
	} else if v.searchQuery != "" {
		parts = append(parts, jjMutedStyle.Render("filter: "+v.searchQuery))
	}

	if v.loading && len(v.allWorkspaces) == 0 {
		parts = append(parts, jjMutedStyle.Render("Loading workspaces…"))
		return strings.Join(parts, "\n")
	}
	if v.err != nil && len(v.allWorkspaces) == 0 {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
		return strings.Join(parts, "\n")
	}
	if v.err != nil {
		parts = append(parts, jjErrorStyle.Render("Error: "+v.err.Error()))
	}

	contentHeight := max(1, v.height-len(parts)-1)
	if v.previewOpen {
		v.tablePane.SetFocused(v.splitPane.Focus() == components.FocusLeft)
		v.splitPane.SetSize(v.width, contentHeight)
		parts = append(parts, v.splitPane.View())
	} else {
		v.tablePane.SetFocused(true)
		v.tablePane.SetSize(v.width, contentHeight)
		parts = append(parts, v.tablePane.View())
	}
	return strings.Join(parts, "\n")
}

func (v *WorkspacesView) Name() string { return "workspaces" }

func (v *WorkspacesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	contentHeight := max(1, height-2)
	v.tablePane.SetSize(width, contentHeight)
	v.previewPane.SetSize(max(1, width/2), contentHeight)
	v.splitPane.SetSize(width, contentHeight)
}

func (v *WorkspacesView) ShortHelp() []key.Binding {
	if v.search.active {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}

	help := []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "ssh")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start/stop")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "preview")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
	if v.previewOpen {
		help = append(help, key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")))
	}
	return help
}

func (v *WorkspacesView) selectedWorkspace() *jjhub.Workspace {
	index := v.tablePane.Cursor()
	if index < 0 || index >= len(v.workspaces) {
		return nil
	}
	workspace := v.workspaces[index]
	return &workspace
}

func (v *WorkspacesView) selectedWorkspaceName() string {
	if workspace := v.selectedWorkspace(); workspace != nil {
		return workspace.Name
	}
	return ""
}

func (v *WorkspacesView) rebuildRows() bool {
	previous := v.selectedWorkspaceName()

	filtered := make([]jjhub.Workspace, 0, len(v.allWorkspaces))
	rows := make([]components.Row, 0, len(v.allWorkspaces))
	for _, workspace := range v.allWorkspaces {
		if v.searchQuery != "" && !jjMatchesSearch(workspace.Name, v.searchQuery) {
			continue
		}

		sshHost := "-"
		if workspace.SSHHost != nil && *workspace.SSHHost != "" {
			sshHost = *workspace.SSHHost
		}
		idle := "-"
		if workspace.IdleTimeoutSeconds > 0 {
			idle = fmt.Sprintf("%dm", workspace.IdleTimeoutSeconds/60)
		}
		filtered = append(filtered, workspace)
		rows = append(rows, components.Row{
			Cells: []string{
				workspace.Name,
				jjhubWorkspaceStatusIcon(workspace.Status) + " " + workspace.Status,
				sshHost,
				map[bool]string{true: "yes", false: "no"}[workspace.IsFork],
				idle,
				jjhubRelativeTime(workspace.CreatedAt),
			},
		})
	}

	v.workspaces = filtered
	v.tablePane.SetRows(rows)

	targetIndex := 0
	for i, workspace := range filtered {
		if workspace.Name == previous {
			targetIndex = i
			break
		}
	}
	if len(filtered) > 0 {
		v.tablePane.SetCursor(targetIndex)
	}
	return previous != v.selectedWorkspaceName()
}

func (v *WorkspacesView) syncPreview(reset bool) tea.Cmd {
	workspace := v.selectedWorkspace()
	if workspace == nil {
		v.previewPane.SetContent("", true)
		return nil
	}
	v.previewPane.SetContent(v.renderPreview(*workspace), reset)
	return nil
}

func (v *WorkspacesView) renderPreview(workspace jjhub.Workspace) string {
	sshHost := "-"
	if workspace.SSHHost != nil && *workspace.SSHHost != "" {
		sshHost = *workspace.SSHHost
	}
	idle := "-"
	if workspace.IdleTimeoutSeconds > 0 {
		idle = fmt.Sprintf("%d minutes", workspace.IdleTimeoutSeconds/60)
	}

	var body strings.Builder
	body.WriteString(jjTitleStyle.Render(workspace.Name))
	body.WriteString("\n")
	body.WriteString(jjBadgeStyleForState(workspace.Status).Render(jjhubWorkspaceStatusIcon(workspace.Status) + " " + workspace.Status))
	body.WriteString("\n\n")
	body.WriteString(jjMetaRow("SSH", sshHost) + "\n")
	body.WriteString(jjMetaRow("Fork", map[bool]string{true: "yes", false: "no"}[workspace.IsFork]) + "\n")
	body.WriteString(jjMetaRow("Idle", idle) + "\n")
	body.WriteString(jjMetaRow("Created", jjFormatTime(workspace.CreatedAt)) + "\n")
	body.WriteString(jjMetaRow("Updated", jjFormatTime(workspace.UpdatedAt)) + "\n")
	body.WriteString(jjMetaRow("VM", workspace.FreestyleVMID) + "\n")
	if workspace.SnapshotID != nil {
		body.WriteString(jjMetaRow("Snapshot", *workspace.SnapshotID) + "\n")
	}
	if workspace.ParentWorkspaceID != nil {
		body.WriteString(jjMetaRow("Parent", *workspace.ParentWorkspaceID) + "\n")
	}
	if workspace.SuspendedAt != nil {
		body.WriteString(jjMetaRow("Suspended", jjFormatTime(*workspace.SuspendedAt)) + "\n")
	}
	body.WriteString("\n")
	body.WriteString(jjSectionStyle.Render("Actions"))
	body.WriteString("\n")
	body.WriteString("Enter to connect over SSH.\n")
	body.WriteString("Press s to ")
	if workspace.Status == "running" {
		body.WriteString("suspend the workspace.")
	} else {
		body.WriteString("resume the workspace.")
	}
	return strings.TrimSpace(body.String())
}

func (v *WorkspacesView) updateSearch(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		v.search.active = false
		v.search.input.Blur()
		return v, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		v.search.active = false
		v.searchQuery = strings.TrimSpace(v.search.input.Value())
		v.search.input.Blur()
		selectionChanged := v.rebuildRows()
		return v, v.syncPreview(selectionChanged)
	default:
		var cmd tea.Cmd
		v.search.input, cmd = v.search.input.Update(msg)
		return v, cmd
	}
}

func (v *WorkspacesView) loadWorkspacesCmd() tea.Cmd {
	client := v.jjhubClient
	return func() tea.Msg {
		workspaces, err := client.ListWorkspaces(jjDefaultListLimit)
		if err != nil {
			return workspacesErrorMsg{err: err}
		}
		return workspacesLoadedMsg{workspaces: workspaces}
	}
}

func (v *WorkspacesView) sshCmd(workspace jjhub.Workspace) tea.Cmd {
	return handoff.Handoff(handoff.Options{
		Binary: "jjhub",
		Args:   []string{"workspace", "ssh", workspace.ID},
		Tag:    "workspace-ssh",
	})
}

func (v *WorkspacesView) toggleWorkspaceCmd(workspace jjhub.Workspace) tea.Cmd {
	action := "resume"
	args := []string{"workspace", "resume", workspace.ID}
	if workspace.Status == "running" {
		action = "suspend"
		args = []string{"workspace", "suspend", workspace.ID}
	}
	if workspace.Status == "pending" {
		return func() tea.Msg {
			return components.ShowToastMsg{
				Title: "Workspace pending",
				Body:  "Wait for the workspace to finish starting before toggling it.",
				Level: components.ToastLevelWarning,
			}
		}
	}

	return func() tea.Msg {
		cmd := exec.Command("jjhub", args...) //nolint:gosec // user-triggered CLI action
		if out, err := cmd.CombinedOutput(); err != nil {
			message := strings.TrimSpace(string(out))
			if message == "" {
				message = err.Error()
			}
			return workspaceActionErrorMsg{action: action, name: workspace.Name, err: fmt.Errorf("%s", message)}
		}
		return workspaceActionDoneMsg{action: action, name: workspace.Name}
	}
}

func workspaceActionTitle(action string) string {
	if action == "" {
		return "Action"
	}
	return strings.ToUpper(action[:1]) + action[1:]
}

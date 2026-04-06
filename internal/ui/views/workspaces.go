package views

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
)

var _ View = (*WorkspacesView)(nil)

type workspaceManager interface {
	GetCurrentRepo() (*jjhub.Repo, error)
	ListWorkspaces(limit int) ([]jjhub.Workspace, error)
	CreateWorkspace(name, snapshotID string) (*jjhub.Workspace, error)
	DeleteWorkspace(workspaceID string) error
	SuspendWorkspace(workspaceID string) (*jjhub.Workspace, error)
	ResumeWorkspace(workspaceID string) (*jjhub.Workspace, error)
	ForkWorkspace(workspaceID, name string) (*jjhub.Workspace, error)
	ListWorkspaceSnapshots(limit int) ([]jjhub.WorkspaceSnapshot, error)
	CreateWorkspaceSnapshot(workspaceID, name string) (*jjhub.WorkspaceSnapshot, error)
	DeleteWorkspaceSnapshot(snapshotID string) error
}

type workspaceBrowserMode uint8

const (
	workspaceMode workspaceBrowserMode = iota
	snapshotMode
)

func (m workspaceBrowserMode) String() string {
	switch m {
	case snapshotMode:
		return "snapshots"
	default:
		return "workspaces"
	}
}

type workspacePromptKind uint8

const (
	workspacePromptCreate workspacePromptKind = iota
	workspacePromptCreateFromSnapshot
	workspacePromptFork
	workspacePromptSnapshot
	workspacePromptDeleteWorkspace
	workspacePromptDeleteSnapshot
)

type workspacePromptState struct {
	active bool
	kind   workspacePromptKind
	input  textinput.Model
	err    error
}

type workspacesLoadedMsg struct {
	workspaces []jjhub.Workspace
}

type workspacesErrorMsg struct {
	err error
}

type workspaceSnapshotsLoadedMsg struct {
	snapshots []jjhub.WorkspaceSnapshot
}

type workspaceSnapshotsErrorMsg struct {
	err error
}

type workspacesRepoLoadedMsg struct {
	repo *jjhub.Repo
}

type workspaceSSHReturnMsg struct {
	workspaceID string
	err         error
}

type workspaceActionDoneMsg struct {
	message string
}

type workspaceActionErrorMsg struct {
	err error
}

// WorkspacesView displays JJHub workspaces and snapshots with a list/detail
// layout.
type WorkspacesView struct {
	client workspaceManager
	repo   *jjhub.Repo

	workspaces []jjhub.Workspace
	snapshots  []jjhub.WorkspaceSnapshot

	statusFilter string
	mode         workspaceBrowserMode

	cursor         int
	scrollOffset   int
	snapshotCursor int
	snapshotOffset int

	width  int
	height int

	loading          bool
	snapshotsLoading bool
	err              error
	snapshotsErr     error

	connectingID string
	actionMsg    string

	prompt workspacePromptState
}

// NewWorkspacesView creates a JJHub workspaces view.
func NewWorkspacesView(_ *smithers.Client) *WorkspacesView {
	var client workspaceManager
	if jjhubAvailable() {
		client = jjhub.NewClient("")
	}
	return newWorkspacesViewWithClient(client)
}

func newWorkspacesViewWithClient(client workspaceManager) *WorkspacesView {
	input := textinput.New()
	input.Placeholder = "optional name"
	input.SetVirtualCursor(true)

	v := &WorkspacesView{
		client:           client,
		mode:             workspaceMode,
		loading:          client != nil,
		snapshotsLoading: client != nil,
		prompt: workspacePromptState{
			input: input,
		},
	}
	if client == nil {
		v.err = errors.New("jjhub CLI not found on PATH")
	}
	return v
}

// Init loads workspace data, snapshot data, and repository metadata.
func (v *WorkspacesView) Init() tea.Cmd {
	if v.client == nil {
		return nil
	}
	return tea.Batch(v.loadWorkspacesCmd(), v.loadSnapshotsCmd(), v.loadRepoCmd())
}

func (v *WorkspacesView) loadWorkspacesCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		workspaces, err := client.ListWorkspaces(100)
		if err != nil {
			return workspacesErrorMsg{err: err}
		}
		return workspacesLoadedMsg{workspaces: workspaces}
	}
}

func (v *WorkspacesView) loadSnapshotsCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		snapshots, err := client.ListWorkspaceSnapshots(100)
		if err != nil {
			return workspaceSnapshotsErrorMsg{err: err}
		}
		return workspaceSnapshotsLoadedMsg{snapshots: snapshots}
	}
}

func (v *WorkspacesView) loadRepoCmd() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		repo, err := client.GetCurrentRepo()
		if err != nil {
			return nil
		}
		return workspacesRepoLoadedMsg{repo: repo}
	}
}

func (v *WorkspacesView) refreshCmd() tea.Cmd {
	if v.client == nil {
		return nil
	}
	v.loading = true
	v.snapshotsLoading = true
	return tea.Batch(v.loadWorkspacesCmd(), v.loadSnapshotsCmd(), v.loadRepoCmd())
}

func (v *WorkspacesView) visibleWorkspaces() []jjhub.Workspace {
	if v.statusFilter == "" {
		return v.workspaces
	}
	filtered := make([]jjhub.Workspace, 0, len(v.workspaces))
	for _, workspace := range v.workspaces {
		if strings.EqualFold(workspace.Status, v.statusFilter) {
			filtered = append(filtered, workspace)
		}
	}
	return filtered
}

func (v *WorkspacesView) visibleSnapshots() []jjhub.WorkspaceSnapshot {
	return v.snapshots
}

func (v *WorkspacesView) selectedWorkspace() *jjhub.Workspace {
	workspaces := v.visibleWorkspaces()
	if len(workspaces) == 0 || v.cursor < 0 || v.cursor >= len(workspaces) {
		return nil
	}
	workspace := workspaces[v.cursor]
	return &workspace
}

func (v *WorkspacesView) selectedSnapshot() *jjhub.WorkspaceSnapshot {
	snapshots := v.visibleSnapshots()
	if len(snapshots) == 0 || v.snapshotCursor < 0 || v.snapshotCursor >= len(snapshots) {
		return nil
	}
	snapshot := snapshots[v.snapshotCursor]
	return &snapshot
}

func (v *WorkspacesView) pageSize() int {
	const (
		headerLines = 6
		itemLines   = 3
	)
	if v.height <= headerLines {
		return 1
	}
	size := (v.height - headerLines) / itemLines
	if size < 1 {
		return 1
	}
	return size
}

func (v *WorkspacesView) clampWorkspaceCursor() {
	workspaces := v.visibleWorkspaces()
	if len(workspaces) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	if v.cursor >= len(workspaces) {
		v.cursor = len(workspaces) - 1
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

func (v *WorkspacesView) clampSnapshotCursor() {
	snapshots := v.visibleSnapshots()
	if len(snapshots) == 0 {
		v.snapshotCursor = 0
		v.snapshotOffset = 0
		return
	}
	if v.snapshotCursor >= len(snapshots) {
		v.snapshotCursor = len(snapshots) - 1
	}
	if v.snapshotCursor < 0 {
		v.snapshotCursor = 0
	}
	pageSize := v.pageSize()
	if v.snapshotCursor < v.snapshotOffset {
		v.snapshotOffset = v.snapshotCursor
	}
	if v.snapshotCursor >= v.snapshotOffset+pageSize {
		v.snapshotOffset = v.snapshotCursor - pageSize + 1
	}
	if v.snapshotOffset < 0 {
		v.snapshotOffset = 0
	}
}

func (v *WorkspacesView) clampCursor() {
	v.clampWorkspaceCursor()
	v.clampSnapshotCursor()
}

func (v *WorkspacesView) cycleStatusFilter() {
	seen := map[string]bool{"": true}
	cycle := []string{""}
	for _, workspace := range v.workspaces {
		status := strings.TrimSpace(workspace.Status)
		if status == "" || seen[status] {
			continue
		}
		seen[status] = true
		cycle = append(cycle, status)
	}

	for i, status := range cycle {
		if strings.EqualFold(status, v.statusFilter) {
			v.statusFilter = cycle[(i+1)%len(cycle)]
			v.cursor = 0
			v.scrollOffset = 0
			return
		}
	}

	v.statusFilter = ""
	v.cursor = 0
	v.scrollOffset = 0
}

func (v *WorkspacesView) sshCmd(workspaceID string) tea.Cmd {
	cmd := exec.Command("jjhub", "workspace", "ssh", workspaceID) //nolint:gosec
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return workspaceSSHReturnMsg{workspaceID: workspaceID, err: err}
	})
}

func (v *WorkspacesView) createWorkspaceCmd(name string) tea.Cmd {
	client := v.client
	displayName := strings.TrimSpace(name)
	return func() tea.Msg {
		workspace, err := client.CreateWorkspace(name, "")
		if err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		if workspace != nil {
			displayName = workspaceName(*workspace)
		}
		if displayName == "" {
			displayName = "workspace"
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Created %s", displayName)}
	}
}

func (v *WorkspacesView) createWorkspaceFromSnapshotCmd(name string) tea.Cmd {
	client := v.client
	snapshot := v.selectedSnapshot()
	if snapshot == nil {
		return nil
	}
	displayName := strings.TrimSpace(name)
	snapshotID := snapshot.ID
	return func() tea.Msg {
		workspace, err := client.CreateWorkspace(name, snapshotID)
		if err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		if workspace != nil {
			displayName = workspaceName(*workspace)
		}
		if displayName == "" {
			displayName = snapshotName(*snapshot)
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Created workspace from %s", displayName)}
	}
}

func (v *WorkspacesView) forkWorkspaceCmd(name string) tea.Cmd {
	client := v.client
	workspace := v.selectedWorkspace()
	if workspace == nil {
		return nil
	}
	displayName := strings.TrimSpace(name)
	sourceID := workspace.ID
	return func() tea.Msg {
		forked, err := client.ForkWorkspace(sourceID, name)
		if err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		if forked != nil {
			displayName = workspaceName(*forked)
		}
		if displayName == "" {
			displayName = sourceID
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Forked %s", displayName)}
	}
}

func (v *WorkspacesView) suspendOrResumeCmd() tea.Cmd {
	client := v.client
	workspace := v.selectedWorkspace()
	if workspace == nil {
		return nil
	}
	id := workspace.ID
	action := "Suspended"
	call := func() (*jjhub.Workspace, error) {
		return client.SuspendWorkspace(id)
	}
	if strings.EqualFold(workspace.Status, "suspended") {
		action = "Resumed"
		call = func() (*jjhub.Workspace, error) {
			return client.ResumeWorkspace(id)
		}
	}
	return func() tea.Msg {
		updated, err := call()
		if err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		if updated != nil {
			id = workspaceName(*updated)
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("%s %s", action, id)}
	}
}

func (v *WorkspacesView) deleteWorkspaceCmd() tea.Cmd {
	client := v.client
	workspace := v.selectedWorkspace()
	if workspace == nil {
		return nil
	}
	name := workspaceName(*workspace)
	id := workspace.ID
	return func() tea.Msg {
		if err := client.DeleteWorkspace(id); err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Deleted %s", name)}
	}
}

func (v *WorkspacesView) createSnapshotCmd(name string) tea.Cmd {
	client := v.client
	workspace := v.selectedWorkspace()
	if workspace == nil {
		return nil
	}
	displayName := strings.TrimSpace(name)
	workspaceID := workspace.ID
	return func() tea.Msg {
		snapshot, err := client.CreateWorkspaceSnapshot(workspaceID, name)
		if err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		if snapshot != nil {
			displayName = snapshotName(*snapshot)
		}
		if displayName == "" {
			displayName = workspaceName(*workspace)
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Created snapshot %s", displayName)}
	}
}

func (v *WorkspacesView) deleteSnapshotCmd() tea.Cmd {
	client := v.client
	snapshot := v.selectedSnapshot()
	if snapshot == nil {
		return nil
	}
	name := snapshotName(*snapshot)
	id := snapshot.ID
	return func() tea.Msg {
		if err := client.DeleteWorkspaceSnapshot(id); err != nil {
			return workspaceActionErrorMsg{err: err}
		}
		return workspaceActionDoneMsg{message: fmt.Sprintf("Deleted snapshot %s", name)}
	}
}

func (v *WorkspacesView) openPrompt(kind workspacePromptKind, placeholder string) tea.Cmd {
	v.prompt.active = true
	v.prompt.kind = kind
	v.prompt.err = nil
	v.prompt.input.Reset()
	v.prompt.input.Placeholder = placeholder
	if usesWorkspacePromptInput(kind) {
		return v.prompt.input.Focus()
	}
	return nil
}

func (v *WorkspacesView) closePrompt() {
	v.prompt.active = false
	v.prompt.err = nil
	v.prompt.input.Reset()
	v.prompt.input.Blur()
}

func (v *WorkspacesView) submitPrompt() tea.Cmd {
	value := strings.TrimSpace(v.prompt.input.Value())

	switch v.prompt.kind {
	case workspacePromptCreate:
		return v.createWorkspaceCmd(value)
	case workspacePromptCreateFromSnapshot:
		return v.createWorkspaceFromSnapshotCmd(value)
	case workspacePromptFork:
		return v.forkWorkspaceCmd(value)
	case workspacePromptSnapshot:
		return v.createSnapshotCmd(value)
	case workspacePromptDeleteWorkspace:
		return v.deleteWorkspaceCmd()
	case workspacePromptDeleteSnapshot:
		return v.deleteSnapshotCmd()
	default:
		return nil
	}
}

func (v *WorkspacesView) selectWorkspaceByID(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	v.statusFilter = ""
	workspaces := v.visibleWorkspaces()
	for i, workspace := range workspaces {
		if workspace.ID == id {
			v.cursor = i
			v.scrollOffset = max(0, i-v.pageSize()+1)
			v.clampWorkspaceCursor()
			return
		}
	}
}

// Update handles messages for the workspaces view.
func (v *WorkspacesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case workspacesLoadedMsg:
		v.workspaces = msg.workspaces
		v.loading = false
		v.err = nil
		v.clampWorkspaceCursor()
		return v, nil

	case workspacesErrorMsg:
		v.loading = false
		v.err = msg.err
		return v, nil

	case workspaceSnapshotsLoadedMsg:
		v.snapshots = msg.snapshots
		v.snapshotsLoading = false
		v.snapshotsErr = nil
		v.clampSnapshotCursor()
		return v, nil

	case workspaceSnapshotsErrorMsg:
		v.snapshotsLoading = false
		v.snapshotsErr = msg.err
		return v, nil

	case workspacesRepoLoadedMsg:
		v.repo = msg.repo
		return v, nil

	case workspaceSSHReturnMsg:
		v.connectingID = ""
		if msg.err != nil {
			v.actionMsg = fmt.Sprintf("SSH error: %v", msg.err)
			return v, nil
		}
		v.actionMsg = fmt.Sprintf("Disconnected from %s", msg.workspaceID)
		return v, v.refreshCmd()

	case workspaceActionDoneMsg:
		v.closePrompt()
		v.actionMsg = msg.message
		return v, v.refreshCmd()

	case workspaceActionErrorMsg:
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
				if usesWorkspacePromptInput(v.prompt.kind) {
					var cmd tea.Cmd
					v.prompt.input, cmd = v.prompt.input.Update(msg)
					return v, cmd
				}
				return v, nil
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if v.mode == workspaceMode {
				v.mode = snapshotMode
			} else {
				v.mode = workspaceMode
			}
			v.actionMsg = ""
			v.clampCursor()
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			v.actionMsg = ""
			if v.mode == snapshotMode {
				if v.snapshotCursor > 0 {
					v.snapshotCursor--
					v.clampSnapshotCursor()
				}
				return v, nil
			}
			if v.cursor > 0 {
				v.cursor--
				v.clampWorkspaceCursor()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			v.actionMsg = ""
			if v.mode == snapshotMode {
				if v.snapshotCursor < len(v.visibleSnapshots())-1 {
					v.snapshotCursor++
					v.clampSnapshotCursor()
				}
				return v, nil
			}
			if v.cursor < len(v.visibleWorkspaces())-1 {
				v.cursor++
				v.clampWorkspaceCursor()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.actionMsg = ""
			v.err = nil
			v.snapshotsErr = nil
			return v, v.refreshCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			if v.mode == workspaceMode {
				v.cycleStatusFilter()
				v.actionMsg = ""
				v.clampWorkspaceCursor()
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			if v.mode == snapshotMode {
				if v.selectedSnapshot() == nil {
					return v, nil
				}
				return v, v.openPrompt(workspacePromptCreateFromSnapshot, "optional workspace name")
			}
			return v, v.openPrompt(workspacePromptCreate, "optional workspace name")

		case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
			if v.mode == workspaceMode && v.selectedWorkspace() != nil {
				return v, v.openPrompt(workspacePromptFork, "optional fork name")
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			if v.mode == workspaceMode && v.selectedWorkspace() != nil {
				return v, v.openPrompt(workspacePromptSnapshot, "optional snapshot name")
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			if v.mode != workspaceMode {
				return v, nil
			}
			if v.selectedWorkspace() == nil {
				return v, nil
			}
			return v, v.suspendOrResumeCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if v.mode == snapshotMode {
				if v.selectedSnapshot() == nil {
					return v, nil
				}
				return v, v.openPrompt(workspacePromptDeleteSnapshot, "")
			}
			if v.selectedWorkspace() == nil {
				return v, nil
			}
			return v, v.openPrompt(workspacePromptDeleteWorkspace, "")

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if v.mode == snapshotMode {
				snapshot := v.selectedSnapshot()
				if snapshot == nil || snapshot.WorkspaceID == nil {
					return v, nil
				}
				v.mode = workspaceMode
				v.selectWorkspaceByID(*snapshot.WorkspaceID)
				v.actionMsg = fmt.Sprintf("Selected workspace %s", *snapshot.WorkspaceID)
				return v, nil
			}

			workspace := v.selectedWorkspace()
			if workspace == nil {
				return v, nil
			}
			if workspace.SSHHost == nil || strings.TrimSpace(*workspace.SSHHost) == "" {
				v.actionMsg = "SSH is not available for the selected workspace"
				return v, nil
			}
			v.connectingID = workspace.ID
			v.actionMsg = ""
			return v, v.sshCmd(workspace.ID)
		}
	}

	return v, nil
}

// View renders the workspaces view.
func (v *WorkspacesView) View() string {
	var b strings.Builder

	filter := "all"
	if v.statusFilter != "" {
		filter = v.statusFilter
	}

	right := []string{
		lipgloss.NewStyle().Faint(true).Render("[" + v.mode.String() + "]"),
		jjhubRepoLabel(v.repo),
	}
	if v.mode == workspaceMode {
		right = append(right, lipgloss.NewStyle().Faint(true).Render("[filter:"+filter+"]"))
	}
	right = append(right, lipgloss.NewStyle().Faint(true).Render("[Esc] Back"))

	b.WriteString(jjhubHeader("JJHUB › Workspaces", v.width, jjhubJoinNonEmpty("  ", right...)))
	b.WriteString("\n\n")

	if v.connectingID != "" {
		b.WriteString(jjhubMutedStyle.Render("Connecting to workspace " + v.connectingID + "..."))
		b.WriteString("\n\n")
	}

	if v.loading {
		b.WriteString("  Loading workspaces...\n")
		return b.String()
	}
	if v.err != nil {
		b.WriteString("  Error: " + v.err.Error() + "\n")
		return b.String()
	}

	if v.width >= 110 {
		leftWidth := min(48, max(34, v.width/3))
		rightWidth := max(24, v.width-leftWidth-3)
		left := lipgloss.NewStyle().Width(leftWidth).Render(v.renderList(leftWidth))
		rightPane := lipgloss.NewStyle().Width(rightWidth).Render(v.renderDetail(rightWidth))
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" │ ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, divider, rightPane))
	} else {
		b.WriteString(v.renderList(v.width))
		b.WriteString("\n\n")
		b.WriteString(v.renderDetail(v.width))
	}

	if strings.TrimSpace(v.actionMsg) != "" {
		b.WriteString("\n\n")
		b.WriteString(jjhubMutedStyle.Render(v.actionMsg))
	}

	if v.prompt.active {
		b.WriteString("\n\n")
		b.WriteString(v.renderPrompt(max(48, min(v.width, 84))))
	}

	return b.String()
}

func (v *WorkspacesView) renderList(width int) string {
	if v.mode == snapshotMode {
		return v.renderSnapshotList(width)
	}
	return v.renderWorkspaceList(width)
}

func (v *WorkspacesView) renderDetail(width int) string {
	if v.mode == snapshotMode {
		return v.renderSnapshotDetail(width)
	}
	return v.renderWorkspaceDetail(width)
}

func (v *WorkspacesView) renderWorkspaceList(width int) string {
	var b strings.Builder
	workspaces := v.visibleWorkspaces()
	if len(workspaces) == 0 {
		if v.statusFilter != "" {
			return "No workspaces matched the current filter."
		}
		return "No workspaces found."
	}

	pageSize := v.pageSize()
	start := min(v.scrollOffset, max(0, len(workspaces)-1))
	end := min(len(workspaces), start+pageSize)

	for i := start; i < end; i++ {
		workspace := workspaces[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		title := workspaceName(workspace)
		b.WriteString(cursor + titleStyle.Render(truncateStr(title, max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			styleWorkspaceStatus(workspace.Status),
			workspacePersistenceLabel(workspace.Persistence),
			jjhubFormatRelativeTime(workspace.UpdatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(workspaces) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(
			fmt.Sprintf("… %d more workspace%s", len(workspaces)-end, pluralS(len(workspaces)-end)),
		))
	}

	return b.String()
}

func (v *WorkspacesView) renderSnapshotList(width int) string {
	var b strings.Builder
	if v.snapshotsLoading {
		return "Loading snapshots..."
	}
	if v.snapshotsErr != nil {
		return "Error: " + v.snapshotsErr.Error()
	}

	snapshots := v.visibleSnapshots()
	if len(snapshots) == 0 {
		return "No snapshots found."
	}

	pageSize := v.pageSize()
	start := min(v.snapshotOffset, max(0, len(snapshots)-1))
	end := min(len(snapshots), start+pageSize)

	for i := start; i < end; i++ {
		snapshot := snapshots[i]
		cursor := "  "
		titleStyle := lipgloss.NewStyle()
		if i == v.snapshotCursor {
			cursor = "▸ "
			titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("111"))
		}

		title := snapshotName(snapshot)
		b.WriteString(cursor + titleStyle.Render(truncateStr(title, max(8, width-2))))
		b.WriteString("\n")

		meta := jjhubJoinNonEmpty(" · ",
			snapshot.SnapshotID,
			snapshotWorkspaceRef(snapshot),
			jjhubFormatRelativeTime(snapshot.UpdatedAt),
		)
		b.WriteString("  " + jjhubMutedStyle.Render(truncateStr(meta, max(8, width-2))))
		b.WriteString("\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(snapshots) {
		b.WriteString("\n")
		b.WriteString(jjhubMutedStyle.Render(
			fmt.Sprintf("… %d more snapshot%s", len(snapshots)-end, pluralS(len(snapshots)-end)),
		))
	}

	return b.String()
}

func (v *WorkspacesView) renderWorkspaceDetail(width int) string {
	workspace := v.selectedWorkspace()
	if workspace == nil {
		return "No workspace selected."
	}

	var b strings.Builder
	title := workspaceName(*workspace)
	b.WriteString(jjhubTitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("Status", workspace.Status))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("ID", workspace.ID))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Persistence", workspacePersistenceLabel(workspace.Persistence)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(workspace.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(workspace.UpdatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Idle Timeout", fmt.Sprintf("%ds", workspace.IdleTimeoutSeconds)))

	if workspace.SSHHost != nil && strings.TrimSpace(*workspace.SSHHost) != "" {
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("SSH", *workspace.SSHHost))
	}
	if workspace.SnapshotID != nil && strings.TrimSpace(*workspace.SnapshotID) != "" {
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Snapshot", *workspace.SnapshotID))
	}
	if workspace.ParentWorkspaceID != nil && strings.TrimSpace(*workspace.ParentWorkspaceID) != "" {
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Parent", *workspace.ParentWorkspaceID))
	}
	if workspace.SuspendedAt != nil && strings.TrimSpace(*workspace.SuspendedAt) != "" {
		b.WriteString("\n")
		b.WriteString(jjhubMetaRow("Suspended", jjhubFormatTimestamp(*workspace.SuspendedAt)))
	}

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Workspace"))
	b.WriteString("\n")
	b.WriteString(wrapText(strings.Join([]string{
		fmt.Sprintf("Freestyle VM: %s", workspace.FreestyleVMID),
		fmt.Sprintf("Repository ID: %d", workspace.RepositoryID),
		fmt.Sprintf("Fork: %t", workspace.IsFork),
		fmt.Sprintf("Snapshots: %d", v.snapshotCountForWorkspace(workspace.ID)),
	}, "\n"), max(20, width)))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Actions"))
	b.WriteString("\n")
	actions := []string{
		"[Enter] SSH",
		"[c] create",
		"[f] fork",
		"[s] suspend/resume",
		"[n] snapshot",
		"[d] delete",
		"[t] filter",
		"[Tab] snapshots",
	}
	b.WriteString(wrapText(strings.Join(actions, "  "), max(20, width)))

	return b.String()
}

func (v *WorkspacesView) renderSnapshotDetail(width int) string {
	snapshot := v.selectedSnapshot()
	if snapshot == nil {
		if v.snapshotsLoading {
			return "Loading snapshots..."
		}
		if v.snapshotsErr != nil {
			return "Error: " + v.snapshotsErr.Error()
		}
		return "No snapshot selected."
	}

	var b strings.Builder
	title := snapshotName(*snapshot)
	b.WriteString(jjhubTitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(jjhubMetaRow("Snapshot", snapshot.SnapshotID))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("ID", snapshot.ID))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Workspace", snapshotWorkspaceRef(*snapshot)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Created", jjhubFormatTimestamp(snapshot.CreatedAt)))
	b.WriteString("\n")
	b.WriteString(jjhubMetaRow("Updated", jjhubFormatTimestamp(snapshot.UpdatedAt)))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Snapshot"))
	b.WriteString("\n")
	b.WriteString(wrapText(strings.Join([]string{
		fmt.Sprintf("Repository ID: %d", snapshot.RepositoryID),
		fmt.Sprintf("User ID: %d", snapshot.UserID),
	}, "\n"), max(20, width)))

	b.WriteString("\n\n")
	b.WriteString(jjhubSectionStyle.Render("Actions"))
	b.WriteString("\n")
	actions := []string{
		"[Enter] linked workspace",
		"[c] create workspace",
		"[d] delete",
		"[Tab] workspaces",
	}
	b.WriteString(wrapText(strings.Join(actions, "  "), max(20, width)))

	return b.String()
}

func (v *WorkspacesView) renderPrompt(width int) string {
	title := workspacePromptTitle(v.prompt.kind)
	body := workspacePromptBody(v)

	var content strings.Builder
	content.WriteString(jjhubTitleStyle.Render(title))
	content.WriteString("\n\n")
	content.WriteString(body)

	if usesWorkspacePromptInput(v.prompt.kind) {
		content.WriteString("\n\n")
		content.WriteString(v.prompt.input.View())
	}
	if v.prompt.err != nil {
		content.WriteString("\n\n")
		content.WriteString(jjhubErrorStyle.Render(v.prompt.err.Error()))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(width).
		Render(content.String())
}

func (v *WorkspacesView) snapshotCountForWorkspace(workspaceID string) int {
	count := 0
	for _, snapshot := range v.snapshots {
		if snapshot.WorkspaceID != nil && *snapshot.WorkspaceID == workspaceID {
			count++
		}
	}
	return count
}

// Name returns the view name.
func (v *WorkspacesView) Name() string { return "workspaces" }

// SetSize stores the terminal size.
func (v *WorkspacesView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampCursor()
}

// ShortHelp returns the contextual help bindings.
func (v *WorkspacesView) ShortHelp() []key.Binding {
	if v.prompt.active {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	if v.mode == snapshotMode {
		return []key.Binding{
			key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create ws")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "workspaces")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "ssh")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "suspend")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "snapshots")),
	}
}

func usesWorkspacePromptInput(kind workspacePromptKind) bool {
	switch kind {
	case workspacePromptCreate, workspacePromptCreateFromSnapshot, workspacePromptFork, workspacePromptSnapshot:
		return true
	default:
		return false
	}
}

func workspacePromptTitle(kind workspacePromptKind) string {
	switch kind {
	case workspacePromptCreate:
		return "Create Workspace"
	case workspacePromptCreateFromSnapshot:
		return "Create Workspace From Snapshot"
	case workspacePromptFork:
		return "Fork Workspace"
	case workspacePromptSnapshot:
		return "Create Snapshot"
	case workspacePromptDeleteWorkspace:
		return "Delete Workspace"
	case workspacePromptDeleteSnapshot:
		return "Delete Snapshot"
	default:
		return "Workspace"
	}
}

func workspacePromptBody(v *WorkspacesView) string {
	switch v.prompt.kind {
	case workspacePromptCreate:
		return "Enter an optional workspace name, then press Enter."
	case workspacePromptCreateFromSnapshot:
		if snapshot := v.selectedSnapshot(); snapshot != nil {
			return fmt.Sprintf("Create a workspace from %s. Enter an optional name, then press Enter.", snapshotName(*snapshot))
		}
		return "Enter an optional workspace name, then press Enter."
	case workspacePromptFork:
		if workspace := v.selectedWorkspace(); workspace != nil {
			return fmt.Sprintf("Fork %s. Enter an optional name, then press Enter.", workspaceName(*workspace))
		}
		return "Enter an optional fork name, then press Enter."
	case workspacePromptSnapshot:
		if workspace := v.selectedWorkspace(); workspace != nil {
			return fmt.Sprintf("Create a snapshot from %s. Enter an optional snapshot name, then press Enter.", workspaceName(*workspace))
		}
		return "Enter an optional snapshot name, then press Enter."
	case workspacePromptDeleteWorkspace:
		if workspace := v.selectedWorkspace(); workspace != nil {
			return fmt.Sprintf("Press Enter to delete %s. Press Esc to cancel.", workspaceName(*workspace))
		}
		return "Press Enter to delete the selected workspace. Press Esc to cancel."
	case workspacePromptDeleteSnapshot:
		if snapshot := v.selectedSnapshot(); snapshot != nil {
			return fmt.Sprintf("Press Enter to delete %s. Press Esc to cancel.", snapshotName(*snapshot))
		}
		return "Press Enter to delete the selected snapshot. Press Esc to cancel."
	default:
		return ""
	}
}

func workspaceName(workspace jjhub.Workspace) string {
	if strings.TrimSpace(workspace.Name) != "" {
		return workspace.Name
	}
	return workspace.ID
}

func snapshotName(snapshot jjhub.WorkspaceSnapshot) string {
	if strings.TrimSpace(snapshot.Name) != "" {
		return snapshot.Name
	}
	if strings.TrimSpace(snapshot.SnapshotID) != "" {
		return snapshot.SnapshotID
	}
	return snapshot.ID
}

func snapshotWorkspaceRef(snapshot jjhub.WorkspaceSnapshot) string {
	if snapshot.WorkspaceID == nil || strings.TrimSpace(*snapshot.WorkspaceID) == "" {
		return "workspace:none"
	}
	return "workspace:" + *snapshot.WorkspaceID
}

func styleWorkspaceStatus(status string) string {
	style := lipgloss.NewStyle()
	switch strings.ToLower(status) {
	case "running", "ready", "active":
		style = style.Foreground(lipgloss.Color("42")).Bold(true)
	case "suspended", "sleeping":
		style = style.Foreground(lipgloss.Color("214")).Bold(true)
	case "failed", "deleted", "error":
		style = style.Foreground(lipgloss.Color("203")).Bold(true)
	default:
		style = style.Faint(true)
	}
	return style.Render(status)
}

func workspacePersistenceLabel(persistence string) string {
	if strings.TrimSpace(persistence) == "" {
		return "default"
	}
	return persistence
}

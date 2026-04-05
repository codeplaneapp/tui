package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*TriggersView)(nil)

// --- Internal message types ---

type triggersLoadedMsg struct {
	crons []smithers.CronSchedule
}

type triggersErrorMsg struct {
	err error
}

type triggerToggleSuccessMsg struct {
	cronID  string
	enabled bool
}

type triggerToggleErrorMsg struct {
	cronID string
	err    error
}

type triggerDeleteSuccessMsg struct {
	cronID string
}

type triggerDeleteErrorMsg struct {
	cronID string
	err    error
}

type triggerCreateSuccessMsg struct {
	cron *smithers.CronSchedule
}

type triggerCreateErrorMsg struct {
	err error
}

type triggerEditSuccessMsg struct {
	// We re-fetch the list after an edit since UpdateCron doesn't exist yet;
	// the smithers CLI is called via CreateCron semantics (delete+add) or a
	// future edit path. For now we reload the full list to get the new state.
}

type triggerEditErrorMsg struct {
	cronID string
	err    error
}

// deleteConfirmState is the state machine for the delete confirmation overlay.
type deleteConfirmState int

const (
	deleteConfirmNone    deleteConfirmState = iota // no overlay
	deleteConfirmPending                           // "Delete? [Enter] Yes [Esc] No" shown
	deleteConfirmRunning                           // async DeleteCron in-flight
)

// createFormField identifies which field is focused in the create form.
type createFormField int

const (
	createFieldPattern      createFormField = iota // cron pattern input
	createFieldWorkflowPath                        // workflow path input
	createFieldCount                               // sentinel — total number of fields
)

// createFormState is the state machine for the trigger-creation overlay.
type createFormState int

const (
	createFormNone    createFormState = iota // no overlay
	createFormActive                         // form visible; user filling fields
	createFormRunning                        // async CreateCron in-flight
)

// editFormState is the state machine for the trigger-edit overlay.
type editFormState int

const (
	editFormNone    editFormState = iota // no overlay
	editFormActive                       // form visible; user editing pattern
	editFormRunning                      // async delete+create in-flight
)

// TriggersView displays a list of cron trigger schedules and allows toggling
// enabled/disabled state, creating new triggers, editing existing ones, and
// deleting triggers.
type TriggersView struct {
	client       *smithers.Client
	crons        []smithers.CronSchedule
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error

	// Toggle inflight: cronID of the cron being toggled, "" when idle.
	toggleInflight string
	toggleErr      error

	// Delete confirmation overlay.
	deleteState deleteConfirmState
	deleteErr   error // error from most-recent delete attempt

	// Create form overlay.
	createState  createFormState
	createFields [createFieldCount]textinput.Model
	createFocus  createFormField
	createErr    error

	// Edit form overlay (changes only the cron pattern).
	editState editFormState
	editInput textinput.Model // single textinput for new pattern
	editErr   error
}

// NewTriggersView creates a new cron triggers view.
func NewTriggersView(client *smithers.Client) *TriggersView {
	v := &TriggersView{
		client:  client,
		loading: true,
	}
	v.initCreateForm()
	v.initEditForm()
	return v
}

// initCreateForm initialises the create-form text inputs.
func (v *TriggersView) initCreateForm() {
	pattern := textinput.New()
	pattern.Placeholder = "cron pattern, e.g. 0 8 * * *"
	pattern.SetVirtualCursor(true)
	pattern.Focus()

	workflowPath := textinput.New()
	workflowPath.Placeholder = ".smithers/workflows/my-flow.tsx"
	workflowPath.SetVirtualCursor(true)
	workflowPath.Blur()

	v.createFields[createFieldPattern] = pattern
	v.createFields[createFieldWorkflowPath] = workflowPath
	v.createFocus = createFieldPattern
}

// initEditForm initialises the edit-form text input.
func (v *TriggersView) initEditForm() {
	ti := textinput.New()
	ti.Placeholder = "new cron pattern, e.g. 0 9 * * 1"
	ti.SetVirtualCursor(true)
	ti.Blur()
	v.editInput = ti
}

// Init loads cron triggers from the client.
func (v *TriggersView) Init() tea.Cmd {
	client := v.client
	return func() tea.Msg {
		crons, err := client.ListCrons(context.Background())
		if err != nil {
			return triggersErrorMsg{err: err}
		}
		return triggersLoadedMsg{crons: crons}
	}
}

// selectedCron returns the cron at the current cursor position, or nil if the
// list is empty or cursor is out of range.
func (v *TriggersView) selectedCron() *smithers.CronSchedule {
	if v.cursor >= 0 && v.cursor < len(v.crons) {
		c := v.crons[v.cursor]
		return &c
	}
	return nil
}

// pageSize returns the number of cron rows visible given the current height.
func (v *TriggersView) pageSize() int {
	const linesPerCron = 2
	const headerLines = 4
	if v.height <= headerLines {
		return 1
	}
	n := (v.height - headerLines) / linesPerCron
	if n < 1 {
		return 1
	}
	return n
}

// clampScroll adjusts scrollOffset so the cursor row is always visible.
func (v *TriggersView) clampScroll() {
	ps := v.pageSize()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	if v.cursor >= v.scrollOffset+ps {
		v.scrollOffset = v.cursor - ps + 1
	}
}

// toggleCmd returns a tea.Cmd that calls ToggleCron for the given cron ID.
func (v *TriggersView) toggleCmd(cronID string, enabled bool) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		err := client.ToggleCron(context.Background(), cronID, enabled)
		if err != nil {
			return triggerToggleErrorMsg{cronID: cronID, err: err}
		}
		return triggerToggleSuccessMsg{cronID: cronID, enabled: enabled}
	}
}

// deleteCmd returns a tea.Cmd that calls DeleteCron for the given cron ID.
func (v *TriggersView) deleteCmd(cronID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		err := client.DeleteCron(context.Background(), cronID)
		if err != nil {
			return triggerDeleteErrorMsg{cronID: cronID, err: err}
		}
		return triggerDeleteSuccessMsg{cronID: cronID}
	}
}

// createCmd returns a tea.Cmd that calls CreateCron with the given arguments.
func (v *TriggersView) createCmd(pattern, workflowPath string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		cron, err := client.CreateCron(context.Background(), pattern, workflowPath)
		if err != nil {
			return triggerCreateErrorMsg{err: err}
		}
		return triggerCreateSuccessMsg{cron: cron}
	}
}

// editCmd returns a tea.Cmd that edits a trigger by deleting and re-creating it
// with a new pattern (smithers has no direct edit endpoint).
func (v *TriggersView) editCmd(cronID, newPattern, workflowPath string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		if err := client.DeleteCron(context.Background(), cronID); err != nil {
			return triggerEditErrorMsg{cronID: cronID, err: err}
		}
		if _, err := client.CreateCron(context.Background(), newPattern, workflowPath); err != nil {
			return triggerEditErrorMsg{cronID: cronID, err: err}
		}
		return triggerEditSuccessMsg{}
	}
}

// removeCronByID removes the cron with the given ID from the slice in-place
// and adjusts the cursor so it stays in bounds.
func (v *TriggersView) removeCronByID(cronID string) {
	for i, c := range v.crons {
		if c.CronID == cronID {
			v.crons = append(v.crons[:i], v.crons[i+1:]...)
			break
		}
	}
	if v.cursor >= len(v.crons) && v.cursor > 0 {
		v.cursor = len(v.crons) - 1
	}
	if len(v.crons) == 0 {
		v.cursor = 0
	}
	v.clampScroll()
}

// updateCronEnabled updates the Enabled field of the cron with cronID.
func (v *TriggersView) updateCronEnabled(cronID string, enabled bool) {
	for i := range v.crons {
		if v.crons[i].CronID == cronID {
			v.crons[i].Enabled = enabled
			return
		}
	}
}

// openCreateForm resets and shows the create-form overlay.
func (v *TriggersView) openCreateForm() {
	v.initCreateForm()
	v.createErr = nil
	v.createState = createFormActive
}

// openEditForm pre-fills and shows the edit-form overlay for the selected cron.
func (v *TriggersView) openEditForm(cron *smithers.CronSchedule) {
	v.initEditForm()
	v.editInput.SetValue(cron.Pattern)
	v.editInput.Focus()
	v.editErr = nil
	v.editState = editFormActive
}

// createFormMoveNext advances focus to the next create-form field (wraps).
func (v *TriggersView) createFormMoveNext() {
	v.createFields[v.createFocus].Blur()
	v.createFocus = (v.createFocus + 1) % createFieldCount
	v.createFields[v.createFocus].Focus()
}

// createFormMovePrev moves focus to the previous create-form field (wraps).
func (v *TriggersView) createFormMovePrev() {
	v.createFields[v.createFocus].Blur()
	v.createFocus = (v.createFocus - 1 + createFormField(createFieldCount)) % createFormField(createFieldCount)
	v.createFields[v.createFocus].Focus()
}

// Update handles messages for the triggers view.
func (v *TriggersView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case triggersLoadedMsg:
		v.crons = msg.crons
		v.loading = false
		v.err = nil
		return v, nil

	case triggersErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case triggerToggleSuccessMsg:
		v.toggleInflight = ""
		v.toggleErr = nil
		v.updateCronEnabled(msg.cronID, msg.enabled)
		return v, nil

	case triggerToggleErrorMsg:
		v.toggleInflight = ""
		v.toggleErr = msg.err
		return v, nil

	case triggerDeleteSuccessMsg:
		v.deleteState = deleteConfirmNone
		v.deleteErr = nil
		v.removeCronByID(msg.cronID)
		return v, nil

	case triggerDeleteErrorMsg:
		v.deleteState = deleteConfirmNone
		v.deleteErr = msg.err
		return v, nil

	case triggerCreateSuccessMsg:
		v.createState = createFormNone
		v.createErr = nil
		if msg.cron != nil {
			v.crons = append(v.crons, *msg.cron)
			v.cursor = len(v.crons) - 1
			v.clampScroll()
		}
		return v, nil

	case triggerCreateErrorMsg:
		v.createState = createFormActive // return to form so user can correct
		v.createErr = msg.err
		return v, nil

	case triggerEditSuccessMsg:
		v.editState = editFormNone
		v.editErr = nil
		// Reload the list to reflect the new pattern (delete+create changed the ID).
		v.loading = true
		return v, v.Init()

	case triggerEditErrorMsg:
		v.editState = editFormActive // return to form
		v.editErr = msg.err
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		return v.handleKeyPress(msg)
	}
	return v, nil
}

// handleKeyPress dispatches key events based on the current overlay state.
func (v *TriggersView) handleKeyPress(msg tea.KeyPressMsg) (View, tea.Cmd) {
	// Create form is active.
	if v.createState == createFormActive {
		return v.updateCreateForm(msg)
	}
	// Create form is submitting — block all keys.
	if v.createState == createFormRunning {
		return v, nil
	}

	// Edit form is active.
	if v.editState == editFormActive {
		return v.updateEditForm(msg)
	}
	// Edit form is submitting — block all keys.
	if v.editState == editFormRunning {
		return v, nil
	}

	// If delete overlay is pending, handle [Enter] / [Esc].
	if v.deleteState == deleteConfirmPending {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			cron := v.selectedCron()
			if cron == nil {
				v.deleteState = deleteConfirmNone
				return v, nil
			}
			v.deleteState = deleteConfirmRunning
			v.deleteErr = nil
			return v, v.deleteCmd(cron.CronID)

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			v.deleteState = deleteConfirmNone
			v.deleteErr = nil
		}
		return v, nil
	}

	// If delete is in-flight, ignore key presses.
	if v.deleteState == deleteConfirmRunning {
		return v, nil
	}

	// Normal key handling.
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		return v, func() tea.Msg { return PopViewMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if v.cursor > 0 {
			v.cursor--
			v.clampScroll()
			v.toggleErr = nil
			v.deleteErr = nil
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if v.cursor < len(v.crons)-1 {
			v.cursor++
			v.clampScroll()
			v.toggleErr = nil
			v.deleteErr = nil
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		if !v.loading {
			v.loading = true
			v.err = nil
			return v, v.Init()
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
		// Toggle enabled/disabled for selected cron.
		if !v.loading && v.err == nil && v.toggleInflight == "" {
			cron := v.selectedCron()
			if cron != nil {
				v.toggleInflight = cron.CronID
				v.toggleErr = nil
				return v, v.toggleCmd(cron.CronID, !cron.Enabled)
			}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		// Open delete confirmation overlay.
		if !v.loading && v.err == nil && v.deleteState == deleteConfirmNone {
			if v.selectedCron() != nil {
				v.deleteState = deleteConfirmPending
				v.deleteErr = nil
			}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		// Open create-trigger form.
		if !v.loading && v.err == nil {
			v.openCreateForm()
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
		// Open edit-trigger form for the selected cron.
		if !v.loading && v.err == nil {
			cron := v.selectedCron()
			if cron != nil {
				v.openEditForm(cron)
			}
		}
	}

	return v, nil
}

// updateCreateForm handles key presses while the create form is active.
func (v *TriggersView) updateCreateForm(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		v.createState = createFormNone
		v.createErr = nil
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		v.createFormMoveNext()
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		v.createFormMovePrev()
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		pattern := strings.TrimSpace(v.createFields[createFieldPattern].Value())
		workflowPath := strings.TrimSpace(v.createFields[createFieldWorkflowPath].Value())
		if pattern == "" || workflowPath == "" {
			// Keep the form open; nothing to submit.
			return v, nil
		}
		v.createState = createFormRunning
		v.createErr = nil
		return v, v.createCmd(pattern, workflowPath)

	default:
		// Forward key to the focused field.
		var cmd tea.Cmd
		v.createFields[v.createFocus], cmd = v.createFields[v.createFocus].Update(msg)
		return v, cmd
	}
}

// updateEditForm handles key presses while the edit form is active.
func (v *TriggersView) updateEditForm(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		v.editState = editFormNone
		v.editErr = nil
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		newPattern := strings.TrimSpace(v.editInput.Value())
		if newPattern == "" {
			return v, nil
		}
		cron := v.selectedCron()
		if cron == nil {
			v.editState = editFormNone
			return v, nil
		}
		v.editState = editFormRunning
		v.editErr = nil
		return v, v.editCmd(cron.CronID, newPattern, cron.WorkflowPath)

	default:
		var cmd tea.Cmd
		v.editInput, cmd = v.editInput.Update(msg)
		return v, cmd
	}
}

// View renders the cron triggers list.
func (v *TriggersView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS \u203a Triggers")
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
	headerLine := header
	if v.width > 0 {
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + helpHint
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	if v.loading {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Loading triggers...") + "\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		b.WriteString("  Check that smithers is on PATH.\n")
		return b.String()
	}

	if len(v.crons) == 0 {
		b.WriteString("  No cron triggers found.\n")
		b.WriteString("  Press [c] to create one, or run: smithers cron add <pattern> <workflow-path>\n")
		return b.String()
	}

	v.renderList(&b)
	v.renderOverlays(&b)

	return b.String()
}

// renderList renders the cron list.
func (v *TriggersView) renderList(b *strings.Builder) {
	ps := v.pageSize()
	end := v.scrollOffset + ps
	if end > len(v.crons) {
		end = len(v.crons)
	}

	for i := v.scrollOffset; i < end; i++ {
		cron := v.crons[i]
		isSelected := i == v.cursor
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if isSelected {
			cursor = "\u25b8 "
			nameStyle = nameStyle.Bold(true)
		}

		// Enabled badge.
		var enabledStr string
		if cron.Enabled {
			enabledStr = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("enabled")
		} else {
			enabledStr = lipgloss.NewStyle().Faint(true).Render("disabled")
		}

		// Toggle-in-flight indicator for the selected row.
		if isSelected && v.toggleInflight == cron.CronID {
			enabledStr = lipgloss.NewStyle().Faint(true).Render("toggling...")
		}

		// First line: cursor + cron ID + enabled badge (right-aligned).
		idPart := cursor + nameStyle.Render(cron.CronID)
		if v.width > 0 {
			idWidth := lipgloss.Width(idPart)
			badgeWidth := lipgloss.Width(enabledStr)
			gap := v.width - idWidth - badgeWidth - 2
			if gap > 0 {
				idPart = idPart + strings.Repeat(" ", gap) + enabledStr
			} else {
				idPart = idPart + "  " + enabledStr
			}
		}
		b.WriteString(idPart + "\n")

		// Second line: schedule pattern + workflow path + last run.
		schedPart := "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render(cron.Pattern)
		pathPart := lipgloss.NewStyle().Faint(true).Render("  " + cron.WorkflowPath)
		lastRunPart := ""
		if cron.LastRunAtMs != nil {
			t := time.UnixMilli(*cron.LastRunAtMs)
			lastRunPart = lipgloss.NewStyle().Faint(true).Render("  last: " + t.Format("2006-01-02 15:04"))
		}
		b.WriteString(schedPart + pathPart + lastRunPart + "\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator when list is clipped.
	if len(v.crons) > ps {
		b.WriteString(fmt.Sprintf("\n  (%d/%d)", v.cursor+1, len(v.crons)))
	}
}

// renderOverlays appends any active overlay panels to b.
func (v *TriggersView) renderOverlays(b *strings.Builder) {
	// Toggle error banner.
	if v.toggleErr != nil && v.toggleInflight == "" {
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Toggle failed: %v", v.toggleErr)))
		b.WriteString("\n")
	}

	// Delete error banner.
	if v.deleteErr != nil && v.deleteState == deleteConfirmNone {
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Delete failed: %v", v.deleteErr)))
		b.WriteString("\n")
	}

	// Delete confirmation overlay.
	if v.deleteState == deleteConfirmPending {
		cron := v.selectedCron()
		if cron != nil {
			b.WriteString("\n")
			promptStyle := lipgloss.NewStyle().Bold(true)
			hintStyle := lipgloss.NewStyle().Faint(true)
			b.WriteString(promptStyle.Render(fmt.Sprintf("  Delete trigger \"%s\"?", cron.CronID)))
			b.WriteString("  ")
			b.WriteString(hintStyle.Render("[Enter] Yes  [Esc] Cancel"))
			b.WriteString("\n")
		}
	}

	// Delete in-flight.
	if v.deleteState == deleteConfirmRunning {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Deleting..."))
		b.WriteString("\n")
	}

	// Create form overlay.
	v.renderCreateFormOverlay(b)

	// Edit form overlay.
	v.renderEditFormOverlay(b)

	// Key hint footer.
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [c] create  [e] edit  [t] toggle  [d] delete  [r] refresh  [Esc] back"))
	b.WriteString("\n")
}

// renderCreateFormOverlay renders the create-trigger form when active.
func (v *TriggersView) renderCreateFormOverlay(b *strings.Builder) {
	if v.createState == createFormNone {
		return
	}

	b.WriteString("\n")

	divWidth := v.width - 4
	if divWidth < 20 {
		divWidth = 40
	}
	b.WriteString("  " + strings.Repeat("\u2500", divWidth) + "\n")

	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true)
	focusedLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))

	b.WriteString("  " + titleStyle.Render("Create Trigger") + "\n\n")

	labels := [createFieldCount]string{"Cron Pattern", "Workflow Path"}
	for i := createFormField(0); i < createFieldCount; i++ {
		lStyle := labelStyle
		if i == v.createFocus {
			lStyle = focusedLabelStyle
		}
		b.WriteString("  " + lStyle.Render(labels[i]) + "\n")
		b.WriteString("  " + v.createFields[i].View() + "\n")
		if i < createFieldCount-1 {
			b.WriteString("\n")
		}
	}

	if v.createErr != nil {
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %v", v.createErr)))
		b.WriteString("\n")
	}

	if v.createState == createFormRunning {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Creating..."))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  [Tab/Shift+Tab] Next/Prev  [Enter] Create  [Esc] Cancel"))
		b.WriteString("\n")
	}
}

// renderEditFormOverlay renders the edit-trigger form when active.
func (v *TriggersView) renderEditFormOverlay(b *strings.Builder) {
	if v.editState == editFormNone {
		return
	}

	cron := v.selectedCron()
	cronID := ""
	if cron != nil {
		cronID = cron.CronID
	}

	b.WriteString("\n")

	divWidth := v.width - 4
	if divWidth < 20 {
		divWidth = 40
	}
	b.WriteString("  " + strings.Repeat("\u2500", divWidth) + "\n")

	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))

	b.WriteString("  " + titleStyle.Render(fmt.Sprintf("Edit Trigger \"%s\"", cronID)) + "\n\n")
	b.WriteString("  " + labelStyle.Render("New Cron Pattern") + "\n")
	b.WriteString("  " + v.editInput.View() + "\n")

	if v.editErr != nil {
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %v", v.editErr)))
		b.WriteString("\n")
	}

	if v.editState == editFormRunning {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Saving..."))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  [Enter] Save  [Esc] Cancel"))
		b.WriteString("\n")
	}
}

// Name returns the view name.
func (v *TriggersView) Name() string {
	return "triggers"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *TriggersView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *TriggersView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191/\u2193", "navigate")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

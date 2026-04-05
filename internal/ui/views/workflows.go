package views

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*WorkflowsView)(nil)

// --- Internal message types ---

type workflowsLoadedMsg struct {
	workflows []smithers.Workflow
}

type workflowsErrorMsg struct {
	err error
}

// workflowRunStartedMsg is sent when RunWorkflow completes successfully.
type workflowRunStartedMsg struct {
	run        *smithers.RunSummary
	workflowID string
}

// workflowRunErrorMsg is sent when RunWorkflow returns an error.
type workflowRunErrorMsg struct {
	workflowID string
	err        error
}

// workflowDAGLoadedMsg is sent when GetWorkflowDAG returns successfully.
type workflowDAGLoadedMsg struct {
	workflowID string
	dag        *smithers.DAGDefinition
}

// workflowDAGErrorMsg is sent when GetWorkflowDAG returns an error.
type workflowDAGErrorMsg struct {
	workflowID string
	err        error
}

// workflowFormDAGLoadedMsg is sent when GetWorkflowDAG is fetched specifically
// to populate a run-input form (triggered by Enter, not the 'i' info overlay).
type workflowFormDAGLoadedMsg struct {
	workflowID string
	dag        *smithers.DAGDefinition
}

// workflowFormDAGErrorMsg is sent when the form DAG fetch fails.
type workflowFormDAGErrorMsg struct {
	workflowID string
	err        error
}

// --- Doctor overlay message types ---

// workflowDoctorResultMsg is sent when the doctor diagnostics complete.
type workflowDoctorResultMsg struct {
	workflowID string
	issues     []DoctorIssue
}

// workflowDoctorErrorMsg is sent when the doctor diagnostics fail entirely.
type workflowDoctorErrorMsg struct {
	workflowID string
	err        error
}

// DoctorIssue represents a single diagnostic finding.
type DoctorIssue struct {
	Severity string // "ok", "warning", "error"
	Check    string // short check name, e.g. "smithers-binary"
	Message  string // human-readable description
}

// --- Confirm-run overlay state ---

// runConfirmState is the state machine for the confirm-run overlay.
type runConfirmState int

const (
	runConfirmNone    runConfirmState = iota // no overlay
	runConfirmPending                        // "Run <name>? [Enter] Yes [Esc] No" shown
	runConfirmRunning                        // async RunWorkflow in-flight
)

// --- Dynamic input form state ---

// runFormState is the state machine for the dynamic input form overlay.
type runFormState int

const (
	runFormNone    runFormState = iota // no form
	runFormLoading                    // fetching DAG fields before showing form
	runFormActive                     // form is visible; user filling in fields
	runFormError                      // DAG fetch failed (fall back to confirm dialog)
)

// --- Info/DAG overlay state ---

// dagOverlayState is the state machine for the info/DAG overlay.
type dagOverlayState int

const (
	dagOverlayHidden  dagOverlayState = iota // no overlay
	dagOverlayLoading                        // async GetWorkflowDAG in-flight
	dagOverlayVisible                        // DAG data ready to display
	dagOverlayError                          // GetWorkflowDAG returned an error
)

// --- Doctor overlay state ---

// doctorOverlayState is the state machine for the doctor diagnostics overlay.
type doctorOverlayState int

const (
	doctorOverlayHidden  doctorOverlayState = iota // no overlay
	doctorOverlayRunning                           // diagnostics in-flight
	doctorOverlayVisible                           // results ready to display
	doctorOverlayError                             // diagnostics failed entirely
)

// WorkflowsView displays a selectable list of discovered Smithers workflows.
type WorkflowsView struct {
	client       *smithers.Client
	workflows    []smithers.Workflow
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	err          error

	// run confirmation overlay
	confirmState runConfirmState
	confirmError error // error from most-recent RunWorkflow call

	// last-run status per workflow ID
	lastRunStatus map[string]smithers.RunStatus

	// DAG / info overlay
	dagState      dagOverlayState
	dagWorkflowID string // which workflow the DAG belongs to
	dagDefinition *smithers.DAGDefinition
	dagError      error

	// Dynamic input form (workflows-dynamic-input-forms)
	formState      runFormState
	formWorkflowID string                 // workflow being run via the form
	formFields     []smithers.WorkflowTask // ordered field definitions
	formInputs     []textinput.Model       // one textinput per field
	formFocused    int                     // index of the focused input

	// Doctor diagnostics overlay (workflows-doctor)
	doctorState      doctorOverlayState
	doctorWorkflowID string        // workflow being diagnosed
	doctorIssues     []DoctorIssue // results from most-recent doctor run
	doctorError      error         // error when doctor run failed entirely

	// DAG schema visibility toggle (workflows-agent-and-schema-inspection)
	// When true the info overlay shows field type/schema details expanded.
	dagSchemaVisible bool
}

// NewWorkflowsView creates a new workflows view.
func NewWorkflowsView(client *smithers.Client) *WorkflowsView {
	return &WorkflowsView{
		client:        client,
		loading:       true,
		lastRunStatus: make(map[string]smithers.RunStatus),
	}
}

// Init loads workflows from the client.
func (v *WorkflowsView) Init() tea.Cmd {
	return func() tea.Msg {
		workflows, err := v.client.ListWorkflows(context.Background())
		if err != nil {
			return workflowsErrorMsg{err: err}
		}
		return workflowsLoadedMsg{workflows: workflows}
	}
}

// selectedWorkflow returns the workflow at the current cursor position, or nil
// if the list is empty or the cursor is out of range. Exposed for use by
// downstream views (e.g. workflows-dynamic-input-forms).
func (v *WorkflowsView) selectedWorkflow() *smithers.Workflow {
	if v.cursor >= 0 && v.cursor < len(v.workflows) {
		w := v.workflows[v.cursor]
		return &w
	}
	return nil
}

// pageSize returns the number of workflows visible in the current terminal height.
func (v *WorkflowsView) pageSize() int {
	const linesPerWorkflow = 2
	const headerLines = 4
	if v.height <= headerLines {
		return 1
	}
	n := (v.height - headerLines) / linesPerWorkflow
	if n < 1 {
		return 1
	}
	return n
}

// clampScroll adjusts scrollOffset so the cursor row is always visible.
func (v *WorkflowsView) clampScroll() {
	ps := v.pageSize()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	if v.cursor >= v.scrollOffset+ps {
		v.scrollOffset = v.cursor - ps + 1
	}
}

// runWorkflowCmd returns a tea.Cmd that calls RunWorkflow for the given
// workflow ID with an optional inputs map.
func (v *WorkflowsView) runWorkflowCmd(workflowID string, inputs map[string]any) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		run, err := client.RunWorkflow(context.Background(), workflowID, inputs)
		if err != nil {
			return workflowRunErrorMsg{workflowID: workflowID, err: err}
		}
		return workflowRunStartedMsg{run: run, workflowID: workflowID}
	}
}

// loadFormDAGCmd returns a tea.Cmd that fetches the DAG specifically to build a
// run-input form. Uses separate message types so it does not interfere with the
// info overlay's DAG fetch.
func (v *WorkflowsView) loadFormDAGCmd(workflowID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		dag, err := client.GetWorkflowDAG(context.Background(), workflowID)
		if err != nil {
			return workflowFormDAGErrorMsg{workflowID: workflowID, err: err}
		}
		return workflowFormDAGLoadedMsg{workflowID: workflowID, dag: dag}
	}
}

// buildFormInputs creates a slice of textinput.Model values for the given
// WorkflowTask fields. The first field is focused; the rest are blurred.
func buildFormInputs(fields []smithers.WorkflowTask) []textinput.Model {
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.Label
		ti.SetVirtualCursor(true)
		if i == 0 {
			ti.Focus()
		} else {
			ti.Blur()
		}
		inputs[i] = ti
	}
	return inputs
}

// collectFormInputs gathers textinput values into a map keyed by field Key.
// Empty strings are included so callers can decide how to handle them.
func collectFormInputs(fields []smithers.WorkflowTask, inputs []textinput.Model) map[string]any {
	out := make(map[string]any, len(fields))
	for i, f := range fields {
		if i < len(inputs) {
			out[f.Key] = inputs[i].Value()
		}
	}
	return out
}

// loadDAGCmd returns a tea.Cmd that calls GetWorkflowDAG for the given workflow ID.
func (v *WorkflowsView) loadDAGCmd(workflowID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		dag, err := client.GetWorkflowDAG(context.Background(), workflowID)
		if err != nil {
			return workflowDAGErrorMsg{workflowID: workflowID, err: err}
		}
		return workflowDAGLoadedMsg{workflowID: workflowID, dag: dag}
	}
}

// runDoctorCmd returns a tea.Cmd that runs workflow doctor diagnostics for the
// given workflow ID. It performs several health checks in sequence:
//  1. Smithers CLI binary present on PATH.
//  2. DAG / launch-fields can be fetched without error.
//  3. DAG mode is "inferred" (not the "fallback" fallback).
func (v *WorkflowsView) runDoctorCmd(workflowID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		issues := RunWorkflowDoctor(context.Background(), client, workflowID)
		return workflowDoctorResultMsg{workflowID: workflowID, issues: issues}
	}
}

// RunWorkflowDoctor executes a series of diagnostic checks for the given
// workflow and returns a slice of DoctorIssue values.  It is exported so that
// tests can invoke it directly without going through the TUI state machine.
func RunWorkflowDoctor(ctx context.Context, client *smithers.Client, workflowID string) []DoctorIssue {
	var issues []DoctorIssue

	// --- Check 1: smithers binary on PATH ---
	_, binaryErr := client.SmithersBinaryPath()
	if binaryErr != nil {
		issues = append(issues, DoctorIssue{
			Severity: "error",
			Check:    "smithers-binary",
			Message:  "smithers binary not found on PATH. Install smithers and ensure it is accessible.",
		})
	} else {
		issues = append(issues, DoctorIssue{
			Severity: "ok",
			Check:    "smithers-binary",
			Message:  "smithers binary found on PATH.",
		})
	}

	// --- Check 2: DAG / launch-fields fetchable ---
	dag, dagErr := client.GetWorkflowDAG(ctx, workflowID)
	if dagErr != nil {
		issues = append(issues, DoctorIssue{
			Severity: "error",
			Check:    "launch-fields",
			Message:  fmt.Sprintf("Could not fetch workflow launch fields: %v", dagErr),
		})
		return issues
	}
	issues = append(issues, DoctorIssue{
		Severity: "ok",
		Check:    "launch-fields",
		Message:  fmt.Sprintf("Launch fields fetched (%d field(s) found).", len(dag.Fields)),
	})

	// --- Check 3: DAG analysis mode ---
	if dag.Mode == "fallback" {
		msg := "Workflow analysis fell back to generic mode."
		if dag.Message != nil && *dag.Message != "" {
			msg += " " + *dag.Message
		}
		issues = append(issues, DoctorIssue{
			Severity: "warning",
			Check:    "dag-analysis",
			Message:  msg,
		})
	} else {
		issues = append(issues, DoctorIssue{
			Severity: "ok",
			Check:    "dag-analysis",
			Message:  fmt.Sprintf("Workflow analysed successfully (mode: %s).", dag.Mode),
		})
	}

	// --- Check 4: at least one input field defined ---
	if len(dag.Fields) == 0 {
		issues = append(issues, DoctorIssue{
			Severity: "warning",
			Check:    "input-fields",
			Message:  "No input fields defined. The workflow may not accept any parameters.",
		})
	} else {
		// Validate that every field has a non-empty key.
		badFields := 0
		for _, f := range dag.Fields {
			if f.Key == "" {
				badFields++
			}
		}
		if badFields > 0 {
			issues = append(issues, DoctorIssue{
				Severity: "warning",
				Check:    "input-fields",
				Message:  fmt.Sprintf("%d input field(s) have an empty key. Check the workflow source.", badFields),
			})
		} else {
			issues = append(issues, DoctorIssue{
				Severity: "ok",
				Check:    "input-fields",
				Message:  fmt.Sprintf("All %d input field(s) have valid keys.", len(dag.Fields)),
			})
		}
	}

	return issues
}

// Update handles messages for the workflows view.
func (v *WorkflowsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case workflowsLoadedMsg:
		v.workflows = msg.workflows
		v.loading = false
		v.err = nil
		return v, nil

	case workflowsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	case workflowRunStartedMsg:
		// Record last-run status and dismiss the confirm overlay.
		if msg.run != nil {
			v.lastRunStatus[msg.workflowID] = msg.run.Status
		}
		v.confirmState = runConfirmNone
		v.confirmError = nil
		return v, nil

	case workflowRunErrorMsg:
		// Store the error and return to "none" so the error is visible in the overlay.
		v.confirmError = msg.err
		v.confirmState = runConfirmNone
		return v, nil

	case workflowDAGLoadedMsg:
		if msg.workflowID == v.dagWorkflowID {
			v.dagDefinition = msg.dag
			v.dagState = dagOverlayVisible
		}
		return v, nil

	case workflowDAGErrorMsg:
		if msg.workflowID == v.dagWorkflowID {
			v.dagError = msg.err
			v.dagState = dagOverlayError
		}
		return v, nil

	// --- Doctor overlay messages (workflows-doctor) ---

	case workflowDoctorResultMsg:
		if msg.workflowID == v.doctorWorkflowID {
			v.doctorIssues = msg.issues
			v.doctorState = doctorOverlayVisible
		}
		return v, nil

	case workflowDoctorErrorMsg:
		if msg.workflowID == v.doctorWorkflowID {
			v.doctorError = msg.err
			v.doctorState = doctorOverlayError
		}
		return v, nil

	// --- Form DAG messages (triggered by Enter, not 'i') ---

	case workflowFormDAGLoadedMsg:
		if msg.workflowID != v.formWorkflowID {
			// Stale response; ignore.
			return v, nil
		}
		dag := msg.dag
		if dag != nil && len(dag.Fields) > 0 {
			// Build the form.
			v.formFields = dag.Fields
			v.formInputs = buildFormInputs(dag.Fields)
			v.formFocused = 0
			v.formState = runFormActive
		} else {
			// No fields: skip the form and go straight to the confirm dialog.
			v.formState = runFormNone
			v.formWorkflowID = ""
			v.confirmState = runConfirmPending
			v.confirmError = nil
		}
		return v, nil

	case workflowFormDAGErrorMsg:
		if msg.workflowID != v.formWorkflowID {
			return v, nil
		}
		// Fall back to the simple confirm dialog on DAG fetch error.
		v.formState = runFormNone
		v.formWorkflowID = ""
		v.confirmState = runConfirmPending
		v.confirmError = nil
		return v, nil

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyPressMsg:
		// If the doctor overlay is open, only allow 'd' or 'esc' to close it.
		if v.doctorState != doctorOverlayHidden {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("d"))),
				key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
				v.doctorState = doctorOverlayHidden
				v.doctorWorkflowID = ""
				v.doctorIssues = nil
				v.doctorError = nil
			}
			return v, nil
		}

		// If the DAG overlay is open, only allow 'i', 's' (toggle schema), or 'esc' to interact.
		if v.dagState != dagOverlayHidden {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
				// Toggle schema/agent detail visibility.
				v.dagSchemaVisible = !v.dagSchemaVisible
			case key.Matches(msg, key.NewBinding(key.WithKeys("i"))),
				key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
				v.dagState = dagOverlayHidden
				v.dagWorkflowID = ""
				v.dagDefinition = nil
				v.dagError = nil
				v.dagSchemaVisible = false
			}
			return v, nil
		}

		// If the form is loading, block all key presses.
		if v.formState == runFormLoading {
			return v, nil
		}

		// If the form is active, route keys to the form handler.
		if v.formState == runFormActive {
			return v.updateForm(msg)
		}

		// If confirm overlay is pending, handle [Enter] / [Esc].
		if v.confirmState == runConfirmPending {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				wf := v.selectedWorkflow()
				if wf == nil {
					v.confirmState = runConfirmNone
					return v, nil
				}
				v.confirmState = runConfirmRunning
				v.confirmError = nil
				return v, v.runWorkflowCmd(wf.ID, nil)

			case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
				v.confirmState = runConfirmNone
				v.confirmError = nil
			}
			return v, nil
		}

		// If a run is in-flight, ignore key presses.
		if v.confirmState == runConfirmRunning {
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
				// Clear run error when moving away.
				v.confirmError = nil
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.workflows)-1 {
				v.cursor++
				v.clampScroll()
				// Clear run error when moving away.
				v.confirmError = nil
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if !v.loading {
				v.loading = true
				v.err = nil
				return v, v.Init()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Fetch the DAG to decide whether to show a form or the simple
			// confirm dialog. Only triggered when a workflow is selected.
			if !v.loading && v.err == nil {
				wf := v.selectedWorkflow()
				if wf != nil {
					v.formState = runFormLoading
					v.formWorkflowID = wf.ID
					v.confirmError = nil
					return v, v.loadFormDAGCmd(wf.ID)
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
			// Open DAG/info overlay for the selected workflow.
			if !v.loading && v.err == nil {
				wf := v.selectedWorkflow()
				if wf != nil {
					v.dagState = dagOverlayLoading
					v.dagWorkflowID = wf.ID
					v.dagDefinition = nil
					v.dagError = nil
					v.dagSchemaVisible = false
					return v, v.loadDAGCmd(wf.ID)
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			// Run doctor diagnostics for the selected workflow.
			if !v.loading && v.err == nil {
				wf := v.selectedWorkflow()
				if wf != nil {
					v.doctorState = doctorOverlayRunning
					v.doctorWorkflowID = wf.ID
					v.doctorIssues = nil
					v.doctorError = nil
					return v, v.runDoctorCmd(wf.ID)
				}
			}
		}
	}
	return v, nil
}

// updateForm handles key presses while the dynamic input form is active.
func (v *WorkflowsView) updateForm(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
		// Cancel the form.
		v.formState = runFormNone
		v.formWorkflowID = ""
		v.formFields = nil
		v.formInputs = nil
		v.formFocused = 0
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		v.formMoveFocus(1)
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		v.formMoveFocus(-1)
		return v, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		// Submit the form.
		inputs := collectFormInputs(v.formFields, v.formInputs)
		wfID := v.formWorkflowID
		// Reset form state.
		v.formState = runFormNone
		v.formWorkflowID = ""
		v.formFields = nil
		v.formInputs = nil
		v.formFocused = 0
		// Kick off the run.
		v.confirmState = runConfirmRunning
		v.confirmError = nil
		return v, v.runWorkflowCmd(wfID, inputs)

	default:
		// Forward to the focused input.
		if v.formFocused < len(v.formInputs) {
			var cmd tea.Cmd
			v.formInputs[v.formFocused], cmd = v.formInputs[v.formFocused].Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

// formMoveFocus shifts focus by delta (wraps around).
func (v *WorkflowsView) formMoveFocus(delta int) {
	n := len(v.formInputs)
	if n == 0 {
		return
	}
	v.formInputs[v.formFocused].Blur()
	v.formFocused = ((v.formFocused + delta) % n + n) % n
	v.formInputs[v.formFocused].Focus()
}

// View renders the workflows list.
func (v *WorkflowsView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS \u203a Workflows")
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
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Loading workflows...") + "\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		b.WriteString("  Check that smithers is on PATH.\n")
		return b.String()
	}

	if len(v.workflows) == 0 {
		b.WriteString("  No workflows found in .smithers/workflows/\n")
		b.WriteString("  Run: smithers workflow list\n")
		return b.String()
	}

	// Render the main list (wide or narrow).
	if v.width >= 100 {
		v.renderWide(&b)
	} else {
		v.renderNarrow(&b)
	}

	// Overlays are rendered below the list.
	v.renderOverlays(&b)

	return b.String()
}

// renderOverlays appends any active overlay panels to b.
func (v *WorkflowsView) renderOverlays(b *strings.Builder) {
	// Run error banner (persists until cursor moves or a new run is started).
	if v.confirmError != nil && v.confirmState == runConfirmNone && v.formState == runFormNone {
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Run failed: %v", v.confirmError)))
		b.WriteString("\n")
	}

	// Form loading indicator.
	if v.formState == runFormLoading {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Loading input fields..."))
		b.WriteString("\n")
		return
	}

	// Dynamic input form.
	if v.formState == runFormActive {
		v.renderFormOverlay(b)
		return
	}

	// Confirm-run overlay.
	if v.confirmState == runConfirmPending {
		wf := v.selectedWorkflow()
		if wf != nil {
			b.WriteString("\n")
			promptStyle := lipgloss.NewStyle().Bold(true)
			hintStyle := lipgloss.NewStyle().Faint(true)
			b.WriteString(promptStyle.Render(fmt.Sprintf("  Run workflow \"%s\"?", wf.Name)))
			b.WriteString("  ")
			b.WriteString(hintStyle.Render("[Enter] Yes  [Esc] Cancel"))
			b.WriteString("\n")
		}
	}

	// Running spinner (simple text; no dependency on spinner component).
	if v.confirmState == runConfirmRunning {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Starting run..."))
		b.WriteString("\n")
	}

	// DAG / info overlay.
	switch v.dagState {
	case dagOverlayLoading:
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Loading workflow info..."))
		b.WriteString("\n")

	case dagOverlayVisible:
		v.renderDAGOverlay(b)

	case dagOverlayError:
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Info error: %v", v.dagError)))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [i/Esc] Close"))
		b.WriteString("\n")
	}

	// Doctor diagnostics overlay.
	switch v.doctorState {
	case doctorOverlayRunning:
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Running diagnostics..."))
		b.WriteString("\n")

	case doctorOverlayVisible:
		v.renderDoctorOverlay(b)

	case doctorOverlayError:
		b.WriteString("\n")
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Doctor error: %v", v.doctorError)))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  [d/Esc] Close"))
		b.WriteString("\n")
	}
}

// renderFormOverlay renders the dynamic input form.
func (v *WorkflowsView) renderFormOverlay(b *strings.Builder) {
	b.WriteString("\n")

	// Divider.
	divWidth := v.width - 4
	if divWidth < 10 {
		divWidth = 40
	}
	b.WriteString("  " + strings.Repeat("\u2500", divWidth) + "\n")

	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)

	// Find the workflow name for the title.
	wfName := v.formWorkflowID
	if wf := v.selectedWorkflow(); wf != nil && wf.ID == v.formWorkflowID {
		wfName = wf.Name
	}
	b.WriteString("  " + titleStyle.Render(fmt.Sprintf("Run \"%s\" — Input Fields", wfName)) + "\n\n")

	labelStyle := lipgloss.NewStyle().Bold(true)
	focusedLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))

	for i, f := range v.formFields {
		if i < len(v.formInputs) {
			isFocused := i == v.formFocused
			lStyle := labelStyle
			if isFocused {
				lStyle = focusedLabelStyle
			}
			b.WriteString("  " + lStyle.Render(f.Label) + "\n")
			b.WriteString("  " + v.formInputs[i].View() + "\n")
			if i < len(v.formFields)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  [Tab/Shift+Tab] Next/Prev  [Enter] Submit  [Esc] Cancel"))
	b.WriteString("\n")
}

// renderDAGOverlay renders the DAG info panel with agent assignments and I/O
// schema details (workflows-dag-inspection + workflows-agent-and-schema-inspection).
func (v *WorkflowsView) renderDAGOverlay(b *strings.Builder) {
	dag := v.dagDefinition
	if dag == nil {
		return
	}

	b.WriteString("\n")

	// Divider.
	divWidth := v.width - 4
	if divWidth < 10 {
		divWidth = 40
	}
	b.WriteString("  " + strings.Repeat("\u2500", divWidth) + "\n")

	titleStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))

	b.WriteString("  " + titleStyle.Render("Workflow Info") + "\n")
	b.WriteString(fmt.Sprintf("  ID:   %s\n", dag.WorkflowID))
	b.WriteString(fmt.Sprintf("  Mode: %s\n", dag.Mode))

	// Agent assignment: EntryTaskID is the first task that receives run input.
	// When present this identifies which "agent" node is the entry point.
	if dag.EntryTaskID != nil && *dag.EntryTaskID != "" {
		b.WriteString("  " + titleStyle.Render("Agent Assignment") + "\n")
		b.WriteString(fmt.Sprintf("  Entry task:  %s\n", keyStyle.Render(*dag.EntryTaskID)))
	}

	if dag.Message != nil && *dag.Message != "" {
		b.WriteString("  " + faintStyle.Render(*dag.Message) + "\n")
	}

	// DAG visualization: render the launch-fields pipeline.
	if len(dag.Fields) > 0 {
		b.WriteString("\n")
		b.WriteString("  " + titleStyle.Render("DAG (input pipeline)") + "\n")
		dagWidth := v.width - 6
		if dagWidth < 20 {
			dagWidth = 60
		}
		dagStr := components.RenderDAGFields(dag.Fields, dagWidth)
		b.WriteString(dagStr)

		// I/O schema details (toggle with 's').
		b.WriteString("\n")
		schemaHint := "[s] Show schema details"
		if v.dagSchemaVisible {
			schemaHint = "[s] Hide schema details"
		}
		b.WriteString("  " + faintStyle.Render(schemaHint) + "\n")

		if v.dagSchemaVisible {
			b.WriteString("\n")
			b.WriteString("  " + titleStyle.Render("I/O Schema") + "\n")
			for i, f := range dag.Fields {
				typ := f.Type
				if typ == "" {
					typ = "string"
				}
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f.Label))
				b.WriteString(fmt.Sprintf("     key:  %s\n", keyStyle.Render(f.Key)))
				b.WriteString(fmt.Sprintf("     type: %s\n", faintStyle.Render(typ)))
			}
		}
	} else {
		b.WriteString("  No input fields.\n")
	}

	b.WriteString("\n")
	b.WriteString(faintStyle.Render("  [i/Esc] Close"))
	b.WriteString("\n")
}

// renderDoctorOverlay renders the doctor diagnostics results panel.
func (v *WorkflowsView) renderDoctorOverlay(b *strings.Builder) {
	b.WriteString("\n")

	// Divider.
	divWidth := v.width - 4
	if divWidth < 10 {
		divWidth = 40
	}
	b.WriteString("  " + strings.Repeat("\u2500", divWidth) + "\n")

	titleStyle := lipgloss.NewStyle().Bold(true)
	faintStyle := lipgloss.NewStyle().Faint(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	wfName := v.doctorWorkflowID
	if wf := v.selectedWorkflow(); wf != nil && wf.ID == v.doctorWorkflowID {
		wfName = wf.Name
	}
	b.WriteString("  " + titleStyle.Render(fmt.Sprintf("Workflow Doctor: %s", wfName)) + "\n\n")

	hasIssue := false
	for _, issue := range v.doctorIssues {
		var badge string
		switch issue.Severity {
		case "ok":
			badge = okStyle.Render("✓")
		case "warning":
			badge = warnStyle.Render("⚠")
			hasIssue = true
		case "error":
			badge = errStyle.Render("✗")
			hasIssue = true
		default:
			badge = "•"
		}
		b.WriteString(fmt.Sprintf("  %s  %s\n", badge, issue.Message))
	}

	if len(v.doctorIssues) == 0 {
		b.WriteString("  " + faintStyle.Render("No issues found.") + "\n")
	} else if !hasIssue {
		b.WriteString("\n  " + okStyle.Render("All checks passed.") + "\n")
	} else {
		b.WriteString("\n  " + warnStyle.Render("Issues found. Review warnings and errors above.") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(faintStyle.Render("  [d/Esc] Close"))
	b.WriteString("\n")
}

// workflowStatusStyle returns a lipgloss style for a workflow status value.
func workflowStatusStyle(status smithers.WorkflowStatus) lipgloss.Style {
	switch status {
	case smithers.WorkflowStatusActive, smithers.WorkflowStatusHot:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case smithers.WorkflowStatusDraft:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	case smithers.WorkflowStatusArchived:
		return lipgloss.NewStyle().Faint(true)
	default:
		return lipgloss.NewStyle()
	}
}

// runStatusBadge returns a compact styled badge for a RunStatus.
// Returns an empty string when status is zero-value.
func runStatusBadge(status smithers.RunStatus) string {
	switch status {
	case smithers.RunStatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render("▶ running")
	case smithers.RunStatusFinished:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ finished")
	case smithers.RunStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ failed")
	case smithers.RunStatusCancelled:
		return lipgloss.NewStyle().Faint(true).Render("○ cancelled")
	case smithers.RunStatusWaitingApproval:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("⏸ waiting-approval")
	case smithers.RunStatusWaitingEvent:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("⏸ waiting-event")
	default:
		return ""
	}
}

// renderNarrow renders the single-column workflows list.
func (v *WorkflowsView) renderNarrow(b *strings.Builder) {
	ps := v.pageSize()
	end := v.scrollOffset + ps
	if end > len(v.workflows) {
		end = len(v.workflows)
	}

	for i := v.scrollOffset; i < end; i++ {
		wf := v.workflows[i]
		isSelected := i == v.cursor
		cursor := "  "
		nameStyle := lipgloss.NewStyle()
		if isSelected {
			cursor = "\u25b8 "
			nameStyle = nameStyle.Bold(true)
		}

		// Name + status on the same row, right-aligned status.
		statusStr := workflowStatusStyle(wf.Status).Render(string(wf.Status))
		namePart := cursor + nameStyle.Render(wf.Name)
		if v.width > 0 {
			nameWidth := lipgloss.Width(namePart)
			statusWidth := lipgloss.Width(statusStr)
			gap := v.width - nameWidth - statusWidth - 2
			if gap > 0 {
				namePart = namePart + strings.Repeat(" ", gap) + statusStr
			} else {
				namePart = namePart + "  " + statusStr
			}
		}
		b.WriteString(namePart + "\n")

		// Path + last-run badge on the same sub-line.
		pathStr := "  " + lipgloss.NewStyle().Faint(true).Render(wf.RelativePath)
		if badge := runStatusBadge(v.lastRunStatus[wf.ID]); badge != "" {
			pathStr = pathStr + "  " + badge
		}
		b.WriteString(pathStr + "\n")

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator when list is clipped.
	if len(v.workflows) > ps {
		b.WriteString(fmt.Sprintf("\n  (%d/%d)", v.cursor+1, len(v.workflows)))
	}
}

// renderWide renders a two-column layout for terminals wider than 100 columns.
func (v *WorkflowsView) renderWide(b *strings.Builder) {
	const leftWidth = 38
	rightWidth := v.width - leftWidth - 3
	if rightWidth < 20 {
		v.renderNarrow(b)
		return
	}

	// Build left pane lines.
	var leftLines []string
	for i, wf := range v.workflows {
		isSelected := i == v.cursor
		prefix := "  "
		style := lipgloss.NewStyle()
		if isSelected {
			prefix = "\u25b8 "
			style = style.Bold(true)
		}
		name := wf.Name
		if len(name) > leftWidth-4 {
			name = name[:leftWidth-7] + "..."
		}
		leftLines = append(leftLines, prefix+style.Render(name))
	}

	// Build right pane (detail for selected workflow).
	var rightLines []string
	if wf := v.selectedWorkflow(); wf != nil {
		idStyle := lipgloss.NewStyle().Bold(true)
		rightLines = append(rightLines,
			idStyle.Render(wf.ID),
			strings.Repeat("\u2500", rightWidth),
			"Name:    "+wf.Name,
			"Path:    "+lipgloss.NewStyle().Faint(true).Render(wf.RelativePath),
			"Status:  "+workflowStatusStyle(wf.Status).Render(string(wf.Status)),
		)

		// Last-run status if available.
		if badge := runStatusBadge(v.lastRunStatus[wf.ID]); badge != "" {
			rightLines = append(rightLines, "Last run: "+badge)
		}

		rightLines = append(rightLines,
			"",
			lipgloss.NewStyle().Faint(true).Render("[Enter] Run  [i] Info  [d] Doctor  [r] Refresh"),
		)
	}

	// Merge the two panes side-by-side.
	leftStyle := lipgloss.NewStyle().Width(leftWidth)
	rows := len(leftLines)
	if len(rightLines) > rows {
		rows = len(rightLines)
	}
	for i := 0; i < rows; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		b.WriteString(leftStyle.Render(left) + " \u2502 " + right + "\n")
	}
}

// Name returns the view name.
func (v *WorkflowsView) Name() string {
	return "workflows"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *WorkflowsView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// ShortHelp returns keybinding hints for the help bar.
func (v *WorkflowsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("\u2191/\u2193", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run")),
		key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "doctor")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

package views

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
)

// Compile-time interface check.
var _ View = (*RunsView)(nil)

// --- Internal message types ---

type runsLoadedMsg struct {
	runs []smithers.RunSummary
}

type runsErrorMsg struct {
	err error
}

// runsStreamReadyMsg carries the channel returned by StreamAllEvents.
// It is returned by startStreamCmd so the channel can be stored on the view
// before WaitForAllEvents is first dispatched.
type runsStreamReadyMsg struct {
	ch <-chan interface{}
}

// runsStreamUnavailableMsg is returned when StreamAllEvents fails immediately
// (no server, 404 on endpoint). Triggers the auto-poll fallback.
type runsStreamUnavailableMsg struct{}

// runsEnrichRunMsg replaces a stub RunSummary with a fully-fetched one.
type runsEnrichRunMsg struct {
	run smithers.RunSummary
}

// tickMsg is sent by the poll ticker every 5 seconds when in polling mode.
type tickMsg struct{}

// runsHijackSessionMsg is returned when HijackRun completes (success or error).
type runsHijackSessionMsg struct {
	runID   string
	session *smithers.HijackSession
	err     error
}

// runsHijackReturnMsg is returned after the hijacked CLI process exits.
type runsHijackReturnMsg struct {
	runID string
	err   error
}

// runsApproveResultMsg is returned when ApproveNode completes (success or error).
type runsApproveResultMsg struct {
	runID string
	err   error
}

// runsDenyResultMsg is returned when DenyNode completes (success or error).
type runsDenyResultMsg struct {
	runID string
	err   error
}

// runsCancelResultMsg is returned when CancelRun completes (success or error).
type runsCancelResultMsg struct {
	runID string
	err   error
}

// runInspectionMsg delivers a fetched RunInspection for a specific run.
type runInspectionMsg struct {
	runID      string
	inspection *smithers.RunInspection
}

// filterCycle is the ordered sequence of status filters cycled by the 'f' key.
// The empty string represents "All" (no filtering).
var filterCycle = []smithers.RunStatus{
	"", // All
	smithers.RunStatusRunning,
	smithers.RunStatusWaitingApproval,
	smithers.RunStatusFinished,
	smithers.RunStatusFailed,
}

// dateRangeFilter identifies a time-based filter for runs.
type dateRangeFilter int

const (
	dateRangeAll   dateRangeFilter = iota // no date filter
	dateRangeToday                        // runs started today (last 24h)
	dateRangeWeek                         // runs started within the last 7 days
	dateRangeMonth                        // runs started within the last 30 days
)

// dateRangeCycle is the ordered sequence of date filters cycled by the 'D' key.
var dateRangeCycle = []dateRangeFilter{
	dateRangeAll,
	dateRangeToday,
	dateRangeWeek,
	dateRangeMonth,
}

// RunsView displays a selectable tabular list of Smithers workflow runs.
type RunsView struct {
	client         *smithers.Client
	runs           []smithers.RunSummary
	statusFilter   smithers.RunStatus // "" = all; non-empty = filter by this status
	workflowFilter string             // "" = all; non-empty = filter by WorkflowName (case-insensitive prefix)
	dateFilter     dateRangeFilter    // time-based filter
	cursor         int
	width          int
	height         int
	loading        bool
	err            error

	// Hijack state
	hijacking bool
	hijackErr error

	// Quick-action state (approve / deny / cancel)
	// actionMsg holds the last inline result message (success or error text).
	// cancelConfirm is true when waiting for the user to confirm 'x' cancellation.
	actionMsg     string
	cancelConfirm bool

	// Inline expand/inspect state.
	// expanded maps runID → detail row visible.
	// inspections maps runID → fetched RunInspection (nil sentinel = fetch attempted).
	expanded    map[string]bool
	inspections map[string]*smithers.RunInspection

	// Search state
	searchActive bool
	searchInput  textinput.Model

	// Streaming state
	ctx         context.Context
	cancel      context.CancelFunc
	allEventsCh <-chan interface{}
	streamMode  string // "live" | "polling" | "" (before first connect)
	pollTicker  *time.Ticker
}

// NewRunsView creates a new runs dashboard view.
func NewRunsView(client *smithers.Client) *RunsView {
	ti := textinput.New()
	ti.Placeholder = "search by run ID or workflow…"
	ti.SetVirtualCursor(true)
	return &RunsView{
		client:      client,
		loading:     true,
		expanded:    make(map[string]bool),
		inspections: make(map[string]*smithers.RunInspection),
		searchInput: ti,
	}
}

// Init loads runs from the client and subscribes to the SSE stream.
func (v *RunsView) Init() tea.Cmd {
	v.ctx, v.cancel = context.WithCancel(context.Background())
	return tea.Batch(
		v.loadRunsCmd(),
		v.startStreamCmd(),
	)
}

// loadRunsCmd returns a tea.Cmd that fetches the run list, applying the
// current statusFilter so the server-side query matches the active filter.
func (v *RunsView) loadRunsCmd() tea.Cmd {
	ctx := v.ctx
	client := v.client
	filter := smithers.RunFilter{
		Limit:  50,
		Status: string(v.statusFilter),
	}
	return func() tea.Msg {
		runs, err := client.ListRuns(ctx, filter)
		if ctx.Err() != nil {
			return nil // view was popped while loading; discard silently
		}
		if err != nil {
			return runsErrorMsg{err: err}
		}
		return runsLoadedMsg{runs: runs}
	}
}

// visibleRuns returns the subset of v.runs that match all active filters:
// status filter, workflow name filter, date range filter, and the search query.
// When no filters or query are active, all runs are returned without allocation.
func (v *RunsView) visibleRuns() []smithers.RunSummary {
	query := strings.ToLower(v.searchInput.Value())
	noStatus := v.statusFilter == ""
	noWorkflow := v.workflowFilter == ""
	noDate := v.dateFilter == dateRangeAll
	noQuery := query == ""
	if noStatus && noWorkflow && noDate && noQuery {
		return v.runs
	}

	// Determine the cutoff time for date range filtering.
	var cutoffMs int64
	if !noDate {
		now := time.Now()
		switch v.dateFilter {
		case dateRangeToday:
			cutoffMs = now.Add(-24 * time.Hour).UnixMilli()
		case dateRangeWeek:
			cutoffMs = now.Add(-7 * 24 * time.Hour).UnixMilli()
		case dateRangeMonth:
			cutoffMs = now.Add(-30 * 24 * time.Hour).UnixMilli()
		}
	}

	wfFilter := strings.ToLower(v.workflowFilter)

	var out []smithers.RunSummary
	for _, r := range v.runs {
		if !noStatus && r.Status != v.statusFilter {
			continue
		}
		if !noWorkflow {
			if !strings.Contains(strings.ToLower(r.WorkflowName), wfFilter) {
				continue
			}
		}
		if !noDate && cutoffMs > 0 {
			if r.StartedAtMs == nil || *r.StartedAtMs < cutoffMs {
				continue
			}
		}
		if !noQuery {
			idMatch := strings.Contains(strings.ToLower(r.RunID), query)
			nameMatch := strings.Contains(strings.ToLower(r.WorkflowName), query)
			if !idMatch && !nameMatch {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// cycleFilter advances statusFilter to the next value in filterCycle.
func (v *RunsView) cycleFilter() {
	for i, f := range filterCycle {
		if f == v.statusFilter {
			v.statusFilter = filterCycle[(i+1)%len(filterCycle)]
			v.cursor = 0
			return
		}
	}
	// Fallback: reset to first (All).
	v.statusFilter = filterCycle[0]
	v.cursor = 0
}

// clearFilter resets statusFilter to "" (All).
func (v *RunsView) clearFilter() {
	v.statusFilter = ""
	v.cursor = 0
}

// cycleWorkflowFilter cycles workflowFilter through the unique WorkflowNames
// present in v.runs.  The cycle order is: "" (All) → name1 → name2 → … → "".
// If the current filter matches no entry in runs, it resets to "".
func (v *RunsView) cycleWorkflowFilter() {
	// Collect unique workflow names in order of first appearance.
	seen := make(map[string]bool)
	var names []string
	for _, r := range v.runs {
		n := r.WorkflowName
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		v.workflowFilter = ""
		v.cursor = 0
		return
	}
	// Build the cycle: "" first, then each unique name.
	cycle := append([]string{""}, names...)
	for i, n := range cycle {
		if strings.EqualFold(n, v.workflowFilter) {
			v.workflowFilter = cycle[(i+1)%len(cycle)]
			v.cursor = 0
			return
		}
	}
	// Current filter not found — reset.
	v.workflowFilter = ""
	v.cursor = 0
}

// clearWorkflowFilter resets the workflow filter to "" (All).
func (v *RunsView) clearWorkflowFilter() {
	v.workflowFilter = ""
	v.cursor = 0
}

// cycleDateFilter advances the date range filter through dateRangeCycle.
func (v *RunsView) cycleDateFilter() {
	for i, d := range dateRangeCycle {
		if d == v.dateFilter {
			v.dateFilter = dateRangeCycle[(i+1)%len(dateRangeCycle)]
			v.cursor = 0
			return
		}
	}
	v.dateFilter = dateRangeCycle[0]
	v.cursor = 0
}

// clearDateFilter resets the date range filter to "All".
func (v *RunsView) clearDateFilter() {
	v.dateFilter = dateRangeAll
	v.cursor = 0
}

// dateFilterLabel returns a human-readable label for the current date range filter.
func (v *RunsView) dateFilterLabel() string {
	switch v.dateFilter {
	case dateRangeToday:
		return "[Today]"
	case dateRangeWeek:
		return "[Week]"
	case dateRangeMonth:
		return "[Month]"
	default:
		return ""
	}
}

// startStreamCmd returns a tea.Cmd that opens the global SSE stream.
// On success it returns runsStreamReadyMsg carrying the channel.
// On failure (no server, 404) it returns runsStreamUnavailableMsg.
func (v *RunsView) startStreamCmd() tea.Cmd {
	ctx := v.ctx
	client := v.client
	return func() tea.Msg {
		ch, err := client.StreamAllEvents(ctx)
		if err != nil {
			return runsStreamUnavailableMsg{}
		}
		return runsStreamReadyMsg{ch: ch}
	}
}

// pollTickCmd returns a tea.Cmd that blocks on the next poll-ticker tick.
func (v *RunsView) pollTickCmd() tea.Cmd {
	ch := v.pollTicker.C
	return func() tea.Msg {
		<-ch
		return tickMsg{}
	}
}

// enrichRunCmd fetches the full RunSummary for a newly-inserted stub run.
func (v *RunsView) enrichRunCmd(runID string) tea.Cmd {
	ctx := v.ctx
	client := v.client
	return func() tea.Msg {
		run, err := client.GetRunSummary(ctx, runID)
		if err != nil || run == nil {
			return nil
		}
		return runsEnrichRunMsg{run: *run}
	}
}

// hijackRunCmd calls HijackRun for the given runID and returns a
// runsHijackSessionMsg with the session or error.
func (v *RunsView) hijackRunCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		session, err := client.HijackRun(context.Background(), runID)
		return runsHijackSessionMsg{runID: runID, session: session, err: err}
	}
}

// approveRunCmd approves the waiting-approval gate on the given run.
// It resolves the nodeID from the cached inspection; falls back to ApproveNode
// with the runID as the nodeID (the server accepts this for single-gate runs).
func (v *RunsView) approveRunCmd(runID string) tea.Cmd {
	client := v.client
	nodeID := v.resolveApprovalNodeID(runID)
	return func() tea.Msg {
		err := client.ApproveNode(context.Background(), runID, nodeID)
		return runsApproveResultMsg{runID: runID, err: err}
	}
}

// denyRunCmd denies the waiting-approval gate on the given run.
func (v *RunsView) denyRunCmd(runID string) tea.Cmd {
	client := v.client
	nodeID := v.resolveApprovalNodeID(runID)
	return func() tea.Msg {
		err := client.DenyNode(context.Background(), runID, nodeID)
		return runsDenyResultMsg{runID: runID, err: err}
	}
}

// cancelRunCmd cancels the given active run.
func (v *RunsView) cancelRunCmd(runID string) tea.Cmd {
	client := v.client
	return func() tea.Msg {
		err := client.CancelRun(context.Background(), runID)
		return runsCancelResultMsg{runID: runID, err: err}
	}
}

// resolveApprovalNodeID returns the nodeID of the blocked/approval-pending task
// for the given run, looked up from the cached inspection.
// If not found, it falls back to the runID itself (accepted by the server for
// single-gate workflows).
func (v *RunsView) resolveApprovalNodeID(runID string) string {
	if insp, ok := v.inspections[runID]; ok && insp != nil {
		for _, t := range insp.Tasks {
			if t.State == smithers.TaskStateBlocked {
				return t.NodeID
			}
		}
	}
	return runID
}

// fetchInspection returns a tea.Cmd that calls InspectRun and delivers
// a runInspectionMsg.  On error, inspection is nil (prevents repeated fetches).
func (v *RunsView) fetchInspection(runID string) tea.Cmd {
	ctx := v.ctx
	client := v.client
	return func() tea.Msg {
		insp, err := client.InspectRun(ctx, runID)
		if err != nil {
			return runInspectionMsg{runID: runID, inspection: nil}
		}
		return runInspectionMsg{runID: runID, inspection: insp}
	}
}

// selectedRun returns the RunSummary at the current cursor position within
// the visible run list, and true if a valid run was found.
func (v *RunsView) selectedRun() (smithers.RunSummary, bool) {
	return components.RunAtCursor(v.visibleRuns(), v.cursor)
}

// applyRunEvent patches v.runs in-place based on the incoming event.
// Returns the RunID of a newly-inserted stub run (empty string if no insertion).
//
// Note: v.runs is not re-sorted on status change in this implementation.
// Sorting by status section is deferred to the RUNS_STATUS_SECTIONING ticket.
func (v *RunsView) applyRunEvent(ev smithers.RunEvent) (newRunID string) {
	// Find existing entry.
	idx := -1
	for i, r := range v.runs {
		if r.RunID == ev.RunID {
			idx = i
			break
		}
	}

	switch ev.Type {
	case "RunStatusChanged", "RunFinished", "RunFailed", "RunCancelled", "RunStarted":
		if ev.Status == "" {
			return ""
		}
		if idx >= 0 {
			v.runs[idx].Status = smithers.RunStatus(ev.Status)
		} else {
			// Unknown run — insert a stub at the top and enrich asynchronously.
			// Deduplication: only insert when the RunID is not already present
			// (guard against race between initial ListRuns and the first SSE event).
			stub := smithers.RunSummary{
				RunID:  ev.RunID,
				Status: smithers.RunStatus(ev.Status),
			}
			v.runs = append([]smithers.RunSummary{stub}, v.runs...)
			return ev.RunID
		}

	case "NodeWaitingApproval":
		if idx >= 0 {
			v.runs[idx].Status = smithers.RunStatusWaitingApproval
		}
	}
	return ""
}

// Update handles messages for the runs view.
func (v *RunsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case runsLoadedMsg:
		v.runs = msg.runs
		v.loading = false
		return v, nil

	case runsErrorMsg:
		v.err = msg.err
		v.loading = false
		return v, nil

	// --- Inline inspection ---

	case runInspectionMsg:
		// Store even if nil — nil sentinel prevents repeated fetch attempts.
		v.inspections[msg.runID] = msg.inspection
		return v, nil

	// --- Streaming messages ---

	case runsStreamReadyMsg:
		v.allEventsCh = msg.ch
		v.streamMode = "live"
		return v, smithers.WaitForAllEvents(v.allEventsCh)

	case runsStreamUnavailableMsg:
		v.streamMode = "polling"
		v.pollTicker = time.NewTicker(5 * time.Second)
		return v, v.pollTickCmd()

	case smithers.RunEventMsg:
		newRunID := v.applyRunEvent(msg.Event)
		cmds := []tea.Cmd{smithers.WaitForAllEvents(v.allEventsCh)}
		if newRunID != "" {
			cmds = append(cmds, v.enrichRunCmd(newRunID))
		}
		return v, tea.Batch(cmds...)

	case smithers.RunEventErrorMsg:
		// Non-fatal parse error — keep listening.
		return v, smithers.WaitForAllEvents(v.allEventsCh)

	case smithers.RunEventDoneMsg:
		// Stream closed. Reconnect if our context is still alive.
		if v.ctx != nil && v.ctx.Err() == nil {
			return v, v.startStreamCmd()
		}
		return v, nil

	case runsEnrichRunMsg:
		for i, r := range v.runs {
			if r.RunID == msg.run.RunID {
				v.runs[i] = msg.run
				break
			}
		}
		return v, nil

	case tickMsg:
		if v.ctx == nil || v.ctx.Err() != nil {
			return v, nil // view popped; stop ticking
		}
		return v, tea.Batch(v.loadRunsCmd(), v.pollTickCmd())

	// --- Hijack flow ---

	case runsHijackSessionMsg:
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
			return runsHijackReturnMsg{runID: runID, err: err}
		})

	case runsHijackReturnMsg:
		v.hijacking = false
		v.hijackErr = msg.err
		// Refresh the run list after returning from the hijacked session.
		v.loading = true
		return v, v.loadRunsCmd()

	// --- Quick-action results ---

	case runsApproveResultMsg:
		if msg.err != nil {
			v.actionMsg = fmt.Sprintf("Approve error: %v", msg.err)
		} else {
			v.actionMsg = fmt.Sprintf("Approved run %s", msg.runID)
			// Optimistically update the run status and refresh.
			for i, r := range v.runs {
				if r.RunID == msg.runID {
					v.runs[i].Status = smithers.RunStatusRunning
					break
				}
			}
		}
		return v, nil

	case runsDenyResultMsg:
		if msg.err != nil {
			v.actionMsg = fmt.Sprintf("Deny error: %v", msg.err)
		} else {
			v.actionMsg = fmt.Sprintf("Denied run %s", msg.runID)
			for i, r := range v.runs {
				if r.RunID == msg.runID {
					v.runs[i].Status = smithers.RunStatusFailed
					break
				}
			}
		}
		return v, nil

	case runsCancelResultMsg:
		v.cancelConfirm = false
		if msg.err != nil {
			v.actionMsg = fmt.Sprintf("Cancel error: %v", msg.err)
		} else {
			v.actionMsg = fmt.Sprintf("Cancelled run %s", msg.runID)
			for i, r := range v.runs {
				if r.RunID == msg.runID {
					v.runs[i].Status = smithers.RunStatusCancelled
					break
				}
			}
		}
		return v, nil

	// --- Layout ---

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.searchInput.SetWidth(msg.Width - 4)
		return v, nil

	// --- Keyboard ---

	case tea.KeyPressMsg:
		// When search is active, route most keys to the textinput.
		if v.searchActive {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				if v.searchInput.Value() != "" {
					// First Esc: clear the query but stay in search mode.
					v.searchInput.Reset()
					v.cursor = 0
				} else {
					// Second Esc (query already empty): exit search mode.
					v.searchActive = false
					v.searchInput.Blur()
					v.cursor = 0
				}
				return v, nil

			default:
				// Forward to textinput; reset cursor on any input change.
				prevQuery := v.searchInput.Value()
				var tiCmd tea.Cmd
				v.searchInput, tiCmd = v.searchInput.Update(msg)
				if v.searchInput.Value() != prevQuery {
					v.cursor = 0
				}
				return v, tiCmd
			}
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
			if v.cancel != nil {
				v.cancel()
			}
			if v.pollTicker != nil {
				v.pollTicker.Stop()
			}
			return v, func() tea.Msg { return PopViewMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			// Activate search mode.
			v.searchActive = true
			cmd := v.searchInput.Focus()
			return v, cmd

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			// cursor is a navigable-row index (run rows only, not section headers).
			// visibleRuns() is the correct upper bound to account for active filter.
			if v.cursor < len(v.visibleRuns())-1 {
				v.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			// Hijack the currently selected run.
			if !v.hijacking {
				visible := v.visibleRuns()
				if len(visible) > 0 && v.cursor < len(visible) {
					v.hijacking = true
					v.hijackErr = nil
					v.actionMsg = ""
					runID := visible[v.cursor].RunID
					return v, v.hijackRunCmd(runID)
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			// Approve the selected waiting-approval run.
			run, ok := v.selectedRun()
			if ok && run.Status == smithers.RunStatusWaitingApproval {
				v.actionMsg = ""
				v.cancelConfirm = false
				return v, v.approveRunCmd(run.RunID)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			// Deny the selected waiting-approval run.
			run, ok := v.selectedRun()
			if ok && run.Status == smithers.RunStatusWaitingApproval {
				v.actionMsg = ""
				v.cancelConfirm = false
				return v, v.denyRunCmd(run.RunID)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			// Cancel the selected active run (requires confirmation).
			run, ok := v.selectedRun()
			if ok && !run.Status.IsTerminal() {
				if v.cancelConfirm {
					// Second 'x': execute cancel.
					v.cancelConfirm = false
					v.actionMsg = ""
					return v, v.cancelRunCmd(run.RunID)
				}
				// First 'x': prompt for confirmation.
				v.cancelConfirm = true
				v.actionMsg = ""
			} else {
				// Clear any stale confirmation if the cursor moved to a terminal run.
				v.cancelConfirm = false
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
			// Cycle through status filter modes.
			v.cycleFilter()
			// Trigger a fresh load with the new filter applied server-side.
			v.loading = true
			v.err = nil
			return v, v.loadRunsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("F"))):
			// Clear status filter (reset to All).
			v.clearFilter()
			v.loading = true
			v.err = nil
			return v, v.loadRunsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
			// Cycle through workflow name filter (client-side).
			v.cycleWorkflowFilter()

		case key.Matches(msg, key.NewBinding(key.WithKeys("W"))):
			// Clear workflow name filter.
			v.clearWorkflowFilter()

		case key.Matches(msg, key.NewBinding(key.WithKeys("D"))):
			// Cycle through date range filter (client-side).
			v.cycleDateFilter()

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			v.loading = true
			v.err = nil
			return v, v.loadRunsCmd()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Enter on a collapsed run: expand inline details (first press).
			// Enter on an already-expanded run: collapse and navigate to full
			// inspector (second press).
			// TODO(runs-inspect-summary): once that ticket ships, the second Enter
			// here will push RunInspectView onto the navigation stack.
			run, ok := v.selectedRun()
			if !ok {
				break
			}
			id := run.RunID
			if v.expanded[id] {
				// Second Enter: collapse and navigate to full inspector.
				delete(v.expanded, id)
				return v, func() tea.Msg { return OpenRunInspectMsg{RunID: id} }
			}
			// First Enter: expand inline detail row.
			v.expanded[id] = true
			// Lazily fetch inspection data if not yet cached and run is active.
			if _, cached := v.inspections[id]; !cached && !run.Status.IsTerminal() {
				return v, v.fetchInspection(id)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			// Open live chat for the selected run.
			run, ok := v.selectedRun()
			if !ok {
				break
			}
			runID := run.RunID
			// Resolve the first running task ID from the cached inspection, if available.
			taskID := ""
			if insp, cached := v.inspections[runID]; cached && insp != nil {
				for _, t := range insp.Tasks {
					if t.State == smithers.TaskStateRunning {
						taskID = t.NodeID
						break
					}
				}
			}
			return v, func() tea.Msg {
				return OpenLiveChatMsg{
					RunID:     runID,
					TaskID:    taskID,
					AgentName: "",
				}
			}
		}
	}
	return v, nil
}

// filterLabel returns the human-readable label for the current status filter,
// e.g. "[Running]" or "[All]".
func (v *RunsView) filterLabel() string {
	switch v.statusFilter {
	case smithers.RunStatusRunning:
		return "[Running]"
	case smithers.RunStatusWaitingApproval:
		return "[Waiting]"
	case smithers.RunStatusFinished:
		return "[Completed]"
	case smithers.RunStatusFailed:
		return "[Failed]"
	default:
		return "[All]"
	}
}

// View renders the runs dashboard.
func (v *RunsView) View() string {
	var b strings.Builder

	// Build compound filter indicator from active filters.
	filterParts := []string{v.filterLabel()}
	if v.workflowFilter != "" {
		wfLabel := "[" + truncate(v.workflowFilter, 20) + "]"
		filterParts = append(filterParts, wfLabel)
	}
	if v.dateFilter != dateRangeAll {
		filterParts = append(filterParts, v.dateFilterLabel())
	}
	filterStr := strings.Join(filterParts, " ")

	// Header with filter indicator, mode indicator and right-justified help hint.
	filterIndicator := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(filterStr)
	header := lipgloss.NewStyle().Bold(true).Render("SMITHERS › Runs") + " " + filterIndicator
	helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")

	modeIndicator := ""
	switch v.streamMode {
	case "live":
		modeIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Live")
	case "polling":
		modeIndicator = lipgloss.NewStyle().Faint(true).Render("○ Polling")
	}

	headerLine := header
	if v.width > 0 {
		right := modeIndicator
		if right != "" && helpHint != "" {
			right = right + "  " + helpHint
		} else if right == "" {
			right = helpHint
		}
		gap := v.width - lipgloss.Width(header) - lipgloss.Width(right) - 2
		if gap > 0 {
			headerLine = header + strings.Repeat(" ", gap) + right
		} else {
			// Not enough room for the full gap — just append with one space.
			headerLine = header + " " + right
		}
	}
	b.WriteString(headerLine)
	b.WriteString("\n\n")

	// Search bar: shown when search is active.
	if v.searchActive {
		searchBar := lipgloss.NewStyle().Faint(true).Render("/") + " " + v.searchInput.View()
		b.WriteString(searchBar)
		b.WriteString("\n\n")
	}

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

	// Cancel confirmation prompt.
	if v.cancelConfirm {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render(
			"  Press 'x' again to confirm cancel, or any other key to abort."))
		b.WriteString("\n")
	}

	// Inline action result (success or error).
	if v.actionMsg != "" {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		if strings.HasPrefix(v.actionMsg, "Approve error:") ||
			strings.HasPrefix(v.actionMsg, "Deny error:") ||
			strings.HasPrefix(v.actionMsg, "Cancel error:") {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		}
		b.WriteString(style.Render("  "+v.actionMsg))
		b.WriteString("\n")
	}

	if v.loading {
		b.WriteString("  Loading runs...\n")
		return b.String()
	}

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
		return b.String()
	}

	visible := v.visibleRuns()
	if len(visible) == 0 {
		query := v.searchInput.Value()
		if query != "" {
			b.WriteString(fmt.Sprintf("  No runs matching %q.\n", query))
		} else if v.workflowFilter != "" {
			b.WriteString(fmt.Sprintf("  No runs for workflow %q.\n", v.workflowFilter))
		} else if v.dateFilter != dateRangeAll {
			b.WriteString(fmt.Sprintf("  No runs in %s range.\n", v.dateFilterLabel()))
		} else if v.statusFilter != "" {
			b.WriteString(fmt.Sprintf("  No %s runs found.\n", v.filterLabel()))
		} else {
			b.WriteString("  No runs found.\n")
		}
		return b.String()
	}

	// Render the run table using the filtered slice, passing expand state.
	table := components.RunTable{
		Runs:        visible,
		Cursor:      v.cursor,
		Width:       v.width,
		Expanded:    v.expanded,
		Inspections: v.inspections,
	}
	b.WriteString(table.View())

	return b.String()
}

// Name returns the view name for the router.
func (v *RunsView) Name() string {
	return "runs"
}

// SetSize stores the terminal dimensions for use during rendering.
func (v *RunsView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// --- Exported accessors (used by tests in external packages) ---

// Runs returns the current list of run summaries held by the view.
func (v *RunsView) Runs() []smithers.RunSummary { return v.runs }

// StatusFilter returns the current status filter value ("" means All).
func (v *RunsView) StatusFilter() smithers.RunStatus { return v.statusFilter }

// StreamMode returns the current stream mode: "live", "polling", or "".
func (v *RunsView) StreamMode() string { return v.streamMode }

// Loading reports whether the view is waiting for the initial run list.
func (v *RunsView) Loading() bool { return v.loading }

// Ctx returns the view's context (created in Init; nil before Init is called).
func (v *RunsView) Ctx() context.Context { return v.ctx }

// Expanded returns the current expand state map (runID → bool).
// Exported for tests.
func (v *RunsView) Expanded() map[string]bool { return v.expanded }

// Inspections returns the current inspections cache (runID → *RunInspection).
// Exported for tests.
func (v *RunsView) Inspections() map[string]*smithers.RunInspection { return v.inspections }

// SearchActive reports whether the search input is currently active.
// Exported for tests.
func (v *RunsView) SearchActive() bool { return v.searchActive }

// SearchQuery returns the current search input value.
// Exported for tests.
func (v *RunsView) SearchQuery() string { return v.searchInput.Value() }

// ActionMsg returns the current inline action result message (success or error).
// Exported for tests.
func (v *RunsView) ActionMsg() string { return v.actionMsg }

// CancelConfirm reports whether the view is awaiting cancel confirmation.
// Exported for tests.
func (v *RunsView) CancelConfirm() bool { return v.cancelConfirm }

// WorkflowFilter returns the current workflow name filter ("" means All).
// Exported for tests.
func (v *RunsView) WorkflowFilter() string { return v.workflowFilter }

// DateFilter returns the current date range filter.
// Exported for tests.
func (v *RunsView) DateFilter() dateRangeFilter { return v.dateFilter }

// ShortHelp returns keybinding hints for the help bar.
func (v *RunsView) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "toggle details")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "deny")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel run")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
		key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hijack")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter status")),
		key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "clear filter")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "filter workflow")),
		key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "filter date")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

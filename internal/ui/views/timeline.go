package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"go.opentelemetry.io/otel/attribute"
)

// --- Snapshot kind ---

// snapshotKind classifies a snapshot for visual differentiation.
type snapshotKind int

const (
	snapshotKindAuto   snapshotKind = iota // ordinary auto-checkpoint
	snapshotKindManual                     // user-triggered manual save
	snapshotKindError                      // snapshot captured on an error/failure
	snapshotKindFork                       // snapshot that was forked from another run
)

// classifySnapshot infers the kind of a snapshot from its label, nodeID, and
// parentID.  The Snapshot struct has no explicit kind field, so we derive it
// heuristically from the human-readable label and structural metadata.
func classifySnapshot(snap smithers.Snapshot) snapshotKind {
	if snap.ParentID != nil {
		return snapshotKindFork
	}
	lower := strings.ToLower(snap.Label + " " + snap.NodeID)
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "fail") ||
		strings.Contains(lower, "exception") {
		return snapshotKindError
	}
	if strings.Contains(lower, "manual") ||
		strings.Contains(lower, "save") ||
		strings.Contains(lower, "checkpoint") {
		return snapshotKindManual
	}
	return snapshotKindAuto
}

// snapshotKindStyle returns a lipgloss style for the given snapshot kind.
func snapshotKindStyle(kind snapshotKind) lipgloss.Style {
	switch kind {
	case snapshotKindError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	case snapshotKindManual:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	case snapshotKindFork:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default: // snapshotKindAuto
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	}
}

// snapshotKindLabel returns a short human-readable label for a snapshot kind.
func snapshotKindLabel(kind snapshotKind) string {
	switch kind {
	case snapshotKindError:
		return "error"
	case snapshotKindManual:
		return "manual"
	case snapshotKindFork:
		return "fork"
	default:
		return "auto"
	}
}

// Compile-time interface check.
var _ View = (*TimelineView)(nil)

// --- Pending action kinds ---

// pendingActionKind identifies which action awaits confirmation.
type pendingActionKind int

const (
	pendingNone pendingActionKind = iota
	pendingFork
	pendingReplay
)

// --- Message types ---

// timelineLoadedMsg carries the initial snapshot list.
type timelineLoadedMsg struct {
	snapshots []smithers.Snapshot
}

// timelineErrorMsg signals a fatal error loading the snapshot list.
type timelineErrorMsg struct {
	err error
}

// timelineDiffLoadedMsg carries a computed diff.
type timelineDiffLoadedMsg struct {
	key  string // "fromID:toID"
	diff *smithers.SnapshotDiff
}

// timelineDiffErrorMsg signals a diff computation failure.
type timelineDiffErrorMsg struct {
	key string
	err error
}

// timelineForkDoneMsg signals a successful fork.
type timelineForkDoneMsg struct {
	run *smithers.ForkReplayRun
}

// timelineReplayDoneMsg signals a successful replay.
type timelineReplayDoneMsg struct {
	run *smithers.ForkReplayRun
}

// timelineActionErrorMsg signals a fork or replay failure.
type timelineActionErrorMsg struct {
	err error
}

// timelineRefreshTickMsg is sent by the polling tick when the run is active.
type timelineRefreshTickMsg struct{}

// --- TimelineView ---

// TimelineView renders a split-pane timeline of a run's snapshot history.
// Left pane: scrollable snapshot list with a compact horizontal rail header.
// Right pane: diff/detail for the selected snapshot.
//
// Navigation:
//   - ↑/k         move cursor up in snapshot list
//   - ↓/j         move cursor down
//   - ←/h         focus left pane
//   - →/l         focus right pane (detail)
//   - g           go to first snapshot
//   - G           go to last snapshot
//   - d           load/show diff for selected vs previous
//   - f           fork run from selected snapshot (with confirmation)
//   - r           replay run from selected snapshot (with confirmation)
//   - R           refresh snapshot list
//   - q / Esc     pop back to previous view
type TimelineView struct {
	client *smithers.Client

	// Identity
	runID string

	// Data
	snapshots []smithers.Snapshot
	diffs     map[string]*smithers.SnapshotDiff // key: "fromID:toID"
	diffErrs  map[string]error                  // cached diff errors

	// Selection
	cursor    int // index into snapshots
	focusPane int // 0 = list, 1 = detail/diff

	// Loading / error state
	loading    bool
	loadingErr error

	// Diff loading
	loadingDiff bool

	// Fork / replay confirmation
	pendingAction pendingActionKind
	pendingResult *smithers.ForkReplayRun
	pendingErr    error

	// Follow mode (auto-scroll to latest when run is still active)
	follow bool

	// Detail pane scroll
	detailScroll int
	detailDirty  bool

	// Snapshot inspector (Enter on a snapshot to view full StateJSON)
	inspecting      bool // true when the full-state inspector is open
	inspectorScroll int  // scroll offset within the inspector

	// Viewport
	width  int
	height int
}

// NewTimelineView creates a new timeline view for the given run.
func NewTimelineView(client *smithers.Client, runID string) *TimelineView {
	return &TimelineView{
		client:      client,
		runID:       runID,
		loading:     true,
		follow:      true,
		diffs:       make(map[string]*smithers.SnapshotDiff),
		diffErrs:    make(map[string]error),
		detailDirty: true,
	}
}

// Init fires the snapshot list fetch and starts a 5-second refresh tick.
func (v *TimelineView) Init() tea.Cmd {
	return tea.Batch(
		v.fetchSnapshots(),
		v.refreshTick(),
	)
}

// fetchSnapshots returns a Cmd that loads all snapshots for the run.
func (v *TimelineView) fetchSnapshots() tea.Cmd {
	runID := v.runID
	client := v.client
	return func() tea.Msg {
		start := time.Now()
		snaps, err := client.ListSnapshots(context.Background(), runID)
		attrs := []attribute.KeyValue{attribute.String("crush.run_id", runID)}
		if err == nil {
			attrs = append(attrs, attribute.Int("crush.snapshot.count", len(snaps)))
		}
		observability.RecordSnapshotOperation("load", time.Since(start), err, attrs...)
		if err != nil {
			return timelineErrorMsg{err: err}
		}
		return timelineLoadedMsg{snapshots: snaps}
	}
}

// fetchDiff returns a Cmd that computes the diff between two snapshots.
func (v *TimelineView) fetchDiff(fromSnap, toSnap smithers.Snapshot) tea.Cmd {
	diffKey := fromSnap.ID + ":" + toSnap.ID
	client := v.client
	return func() tea.Msg {
		start := time.Now()
		diff, err := client.DiffSnapshots(context.Background(), fromSnap.ID, toSnap.ID)
		observability.RecordSnapshotOperation("diff", time.Since(start), err,
			attribute.String("crush.run_id", toSnap.RunID),
			attribute.String("crush.snapshot.from_id", fromSnap.ID),
			attribute.String("crush.snapshot.to_id", toSnap.ID),
		)
		if err != nil {
			return timelineDiffErrorMsg{key: diffKey, err: err}
		}
		return timelineDiffLoadedMsg{key: diffKey, diff: diff}
	}
}

// refreshTick returns a command that fires timelineRefreshTickMsg after 5 seconds.
func (v *TimelineView) refreshTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
		return timelineRefreshTickMsg{}
	})
}

// prefetchAdjacentDiff ensures the diff between the selected snapshot and the
// previous one is loaded. Returns a fetch command if not yet in the cache.
func (v *TimelineView) prefetchAdjacentDiff() tea.Cmd {
	if v.cursor <= 0 || v.cursor >= len(v.snapshots) {
		return nil
	}
	from := v.snapshots[v.cursor-1]
	to := v.snapshots[v.cursor]
	diffKey := from.ID + ":" + to.ID
	if _, ok := v.diffs[diffKey]; ok {
		return nil // already cached
	}
	if _, ok := v.diffErrs[diffKey]; ok {
		return nil // error cached, don't retry
	}
	v.loadingDiff = true
	return v.fetchDiff(from, to)
}

// Update handles all messages for the timeline view.
func (v *TimelineView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {

	case timelineLoadedMsg:
		v.snapshots = msg.snapshots
		v.loading = false
		v.detailDirty = true
		if v.follow && len(v.snapshots) > 0 {
			v.cursor = len(v.snapshots) - 1
		}
		return v, v.prefetchAdjacentDiff()

	case timelineErrorMsg:
		v.loadingErr = msg.err
		v.loading = false
		return v, nil

	case timelineDiffLoadedMsg:
		v.diffs[msg.key] = msg.diff
		v.loadingDiff = false
		v.detailDirty = true
		return v, nil

	case timelineDiffErrorMsg:
		v.diffErrs[msg.key] = msg.err
		v.loadingDiff = false
		v.detailDirty = true
		return v, nil

	case timelineForkDoneMsg:
		v.pendingAction = pendingNone
		v.pendingResult = msg.run
		v.pendingErr = nil
		return v, forkSuccessToast(msg.run)

	case timelineReplayDoneMsg:
		v.pendingAction = pendingNone
		v.pendingResult = msg.run
		v.pendingErr = nil
		return v, replaySuccessToast(msg.run)

	case timelineActionErrorMsg:
		v.pendingAction = pendingNone
		v.pendingErr = msg.err
		return v, actionErrorToast(msg.err)

	case timelineRefreshTickMsg:
		// Re-fetch snapshot list and schedule next tick (poll while run is active).
		return v, tea.Batch(v.fetchSnapshots(), v.refreshTick())

	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.detailDirty = true
		return v, nil

	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

// handleKey routes keyboard input based on whether a confirmation prompt is active.
func (v *TimelineView) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	if v.pendingAction != pendingNone {
		return v.handleConfirmKey(msg)
	}

	// Inspector mode: Enter opens/closes the full-state inspector for the
	// selected snapshot.  When the inspector is open, ↑/↓ scroll its content,
	// Enter/Esc/q close it, and all other actions are suppressed.
	if v.inspecting {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "q", "esc", "alt+esc"))):
			v.inspecting = false
			v.inspectorScroll = 0
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.inspectorScroll > 0 {
				v.inspectorScroll--
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			v.inspectorScroll++
		}
		return v, nil
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc", "alt+esc"))):
		return v, func() tea.Msg { return PopViewMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		// Open the full-state inspector for the currently selected snapshot.
		if len(v.snapshots) > 0 {
			v.inspecting = true
			v.inspectorScroll = 0
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if v.focusPane == 1 {
			// Scroll detail pane up.
			if v.detailScroll > 0 {
				v.detailScroll--
			}
		} else {
			v.follow = false
			if v.cursor > 0 {
				v.cursor--
				v.detailDirty = true
				return v, v.prefetchAdjacentDiff()
			}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if v.focusPane == 1 {
			// Scroll detail pane down.
			v.detailScroll++
		} else {
			v.follow = false
			if v.cursor < len(v.snapshots)-1 {
				v.cursor++
				v.detailDirty = true
				return v, v.prefetchAdjacentDiff()
			}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		v.follow = false
		v.cursor = 0
		v.detailDirty = true
		return v, v.prefetchAdjacentDiff()

	case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
		if len(v.snapshots) > 0 {
			v.cursor = len(v.snapshots) - 1
		}
		v.detailDirty = true
		return v, v.prefetchAdjacentDiff()

	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
		v.focusPane = 0

	case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
		if v.width >= 80 {
			v.focusPane = 1
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		return v, v.prefetchAdjacentDiff()

	case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
		if len(v.snapshots) > 0 {
			v.pendingAction = pendingFork
			v.pendingResult = nil
			v.pendingErr = nil
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		if len(v.snapshots) > 0 {
			v.pendingAction = pendingReplay
			v.pendingResult = nil
			v.pendingErr = nil
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("R"))):
		v.loading = true
		return v, v.fetchSnapshots()
	}

	return v, nil
}

// handleConfirmKey handles y/N responses to fork/replay confirmation prompts.
func (v *TimelineView) handleConfirmKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
		action := v.pendingAction
		v.pendingAction = pendingNone
		return v, v.dispatchAction(action)
	default:
		// Any other key cancels.
		v.pendingAction = pendingNone
	}
	return v, nil
}

// dispatchAction fires the fork or replay API call after confirmation.
func (v *TimelineView) dispatchAction(action pendingActionKind) tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.snapshots) {
		return nil
	}
	snap := v.snapshots[v.cursor]
	client := v.client

	switch action {
	case pendingFork:
		label := fmt.Sprintf("fork from snap %d", snap.SnapshotNo)
		return func() tea.Msg {
			start := time.Now()
			run, err := client.ForkRun(context.Background(), snap.ID, smithers.ForkOptions{
				Label: label,
			})
			attrs := []attribute.KeyValue{
				attribute.String("crush.run_id", snap.RunID),
				attribute.String("crush.snapshot.id", snap.ID),
			}
			if err == nil && run != nil {
				attrs = append(attrs, attribute.String("crush.snapshot.result_run_id", run.ID))
			}
			observability.RecordSnapshotOperation("fork", time.Since(start), err, attrs...)
			if err != nil {
				return timelineActionErrorMsg{err: err}
			}
			return timelineForkDoneMsg{run: run}
		}

	case pendingReplay:
		label := fmt.Sprintf("replay from snap %d", snap.SnapshotNo)
		return func() tea.Msg {
			start := time.Now()
			run, err := client.ReplayRun(context.Background(), snap.ID, smithers.ReplayOptions{
				Label: label,
			})
			attrs := []attribute.KeyValue{
				attribute.String("crush.run_id", snap.RunID),
				attribute.String("crush.snapshot.id", snap.ID),
			}
			if err == nil && run != nil {
				attrs = append(attrs, attribute.String("crush.snapshot.result_run_id", run.ID))
			}
			observability.RecordSnapshotOperation("replay", time.Since(start), err, attrs...)
			if err != nil {
				return timelineActionErrorMsg{err: err}
			}
			return timelineReplayDoneMsg{run: run}
		}
	}
	return nil
}

// --- View (rendering) ---

// View renders the full timeline view as a string.
func (v *TimelineView) View() string {
	var b strings.Builder

	b.WriteString(v.renderHeader())
	b.WriteString("\n")
	b.WriteString(v.renderDivider())
	b.WriteString("\n")

	if v.loading {
		b.WriteString("  Loading snapshots...\n")
		return b.String()
	}
	if v.loadingErr != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", v.loadingErr))
		return b.String()
	}
	if len(v.snapshots) == 0 {
		b.WriteString("  No snapshots found for this run.\n")
		return b.String()
	}

	// Inspector overlay: show full-state inspector instead of normal body.
	if v.inspecting {
		b.WriteString(v.renderInspector())
		b.WriteString(v.renderDivider())
		b.WriteString("\n")
		b.WriteString(v.renderFooter())
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(v.renderRail())
	b.WriteString("\n")
	b.WriteString(v.renderDivider())
	b.WriteString("\n")

	b.WriteString(v.renderBody())

	b.WriteString(v.renderDivider())
	b.WriteString("\n")
	b.WriteString(v.renderFooter())
	b.WriteString("\n")

	return b.String()
}

// Name returns the view name for the router.
func (v *TimelineView) Name() string { return "timeline" }

// SetSize stores the terminal dimensions for use during rendering.
// Called by the router when the view is pushed and on resize events.
func (v *TimelineView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.detailDirty = true
}

// ShortHelp returns keybinding hints for the help bar.
func (v *TimelineView) ShortHelp() []key.Binding {
	if v.pendingAction != pendingNone {
		return []key.Binding{
			key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
			key.NewBinding(key.WithKeys("N", "esc"), key.WithHelp("N/Esc", "cancel")),
		}
	}
	if v.inspecting {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "scroll")),
			key.NewBinding(key.WithKeys("enter", "q", "esc"), key.WithHelp("Enter/q/Esc", "close inspector")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "navigate")),
		key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/→", "panes")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "inspect")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fork")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/Esc", "back")),
	}
}

// --- Rendering helpers ---

func (v *TimelineView) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)

	runPart := v.runID
	if len(runPart) > 8 {
		runPart = runPart[:8]
	}

	title := "SMITHERS › Snapshots › " + runPart
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

func (v *TimelineView) renderDivider() string {
	if v.width > 0 {
		return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", v.width))
	}
	return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", 40))
}

// renderRail renders the compact horizontal snapshot strip at the top of the body.
// Encircled numbers for 1–20; bracketed numbers beyond. Markers are color-coded
// by snapshot kind (auto=green, manual=cyan, error=red, fork=yellow). The selected
// snapshot is highlighted with bold+reverse and has a ▲ indicator on the row below.
// Snapshots exceeding the visible budget are shown as "──...+N".
func (v *TimelineView) renderRail() string {
	if len(v.snapshots) == 0 {
		return ""
	}

	// Each marker slot is roughly 5 chars wide plus a 2-char connector = 7 total.
	maxVisible := 20
	if v.width > 0 {
		maxVisible = (v.width - 4) / 7
		if maxVisible < 3 {
			maxVisible = 3
		}
		if maxVisible > 20 {
			maxVisible = 20
		}
	}

	boldRev := lipgloss.NewStyle().Bold(true).Reverse(true)
	faint := lipgloss.NewStyle().Faint(true)

	// Track rendered widths per slot for arrow positioning.
	type slot struct {
		text  string
		width int
	}
	var slots []slot

	total := len(v.snapshots)
	shown := min(total, maxVisible)
	start := 0
	if total > shown {
		start = v.cursor - shown/2
		if start < 0 {
			start = 0
		}
		if maxStart := total - shown; start > maxStart {
			start = maxStart
		}
	}
	end := start + shown

	connector := faint.Render("──")
	connectorWidth := lipgloss.Width(connector)

	for i := start; i < end; i++ {
		snap := v.snapshots[i]
		marker := snapshotMarker(snap.SnapshotNo)
		kind := classifySnapshot(snap)
		kindStyle := snapshotKindStyle(kind)

		var rendered string
		switch {
		case i == v.cursor:
			// Selected: bold+reverse overrides kind color for high contrast.
			rendered = boldRev.Render(marker)
		case snap.ParentID != nil:
			// Fork origin: branch glyph + kind color.
			rendered = kindStyle.Faint(true).Render("⎇" + markerSuffix(marker))
		default:
			// Normal: apply kind color.
			rendered = kindStyle.Render(marker)
		}
		slots = append(slots, slot{text: rendered, width: lipgloss.Width(rendered)})
	}

	// Build the rail line.
	var railParts []string
	for _, s := range slots {
		railParts = append(railParts, s.text)
	}
	rail := "  " + strings.Join(railParts, connector)

	if start > 0 {
		rail = "  " + faint.Render(fmt.Sprintf("...+%d──", start)) + strings.Join(railParts, connector)
	}
	if end < total {
		rail += faint.Render(fmt.Sprintf("──...+%d", total-end))
	}

	// Build the arrow line: a ▲ positioned under the selected marker.
	// Compute the offset of the cursor slot in the rail string.
	arrowLine := ""
	cursorSlot := v.cursor - start
	if cursorSlot >= 0 && cursorSlot < len(slots) {
		// 2 spaces prefix + (slot_index * (connectorWidth + slotWidth_of_each_prior_slot))
		offset := 2 // leading "  "
		if start > 0 {
			offset += lipgloss.Width(faint.Render(fmt.Sprintf("...+%d──", start)))
		}
		for i := 0; i < cursorSlot; i++ {
			offset += slots[i].width + connectorWidth
		}
		// Center the ▲ under the selected marker.
		arrowOffset := offset + slots[cursorSlot].width/2
		if arrowOffset < 0 {
			arrowOffset = 0
		}
		arrowStyle := lipgloss.NewStyle().Bold(true)
		arrowLine = strings.Repeat(" ", arrowOffset) + arrowStyle.Render("▲")
	}

	if arrowLine != "" {
		return rail + "\n" + arrowLine
	}
	return rail
}

// markerSuffix returns everything after the first rune of a marker string.
// For encircled numbers the first rune is the Unicode character itself, so
// this returns an empty string. For bracketed "[N]" it returns "N]".
func markerSuffix(marker string) string {
	runes := []rune(marker)
	if len(runes) <= 1 {
		return ""
	}
	return string(runes[1:])
}

// renderBody chooses between split-pane and compact layouts based on width.
func (v *TimelineView) renderBody() string {
	listWidth := 38
	dividerWidth := 3
	detailWidth := v.width - listWidth - dividerWidth

	if v.width < 80 || detailWidth < 20 {
		return v.renderBodyCompact()
	}

	listContent := v.renderList(listWidth)
	detailContent := v.renderDetail(detailWidth)

	divider := lipgloss.NewStyle().Faint(true).Render(" │ ")

	listLines := strings.Split(listContent, "\n")
	detailLines := strings.Split(detailContent, "\n")

	// Reserve lines for: header (1) + divider (1) + rail (1) + divider (1) +
	// two footer divider+help lines (2) = 6 overhead lines.
	reserved := 6
	maxLines := v.height - reserved
	if maxLines < 4 {
		maxLines = 4
	}

	total := len(listLines)
	if len(detailLines) > total {
		total = len(detailLines)
	}
	if total > maxLines {
		total = maxLines
	}

	var b strings.Builder
	for i := 0; i < total; i++ {
		left := ""
		if i < len(listLines) {
			left = listLines[i]
		}
		right := ""
		if i < len(detailLines) {
			right = detailLines[i]
		}
		left = padRight(left, listWidth)
		b.WriteString(left + divider + right + "\n")
	}
	return b.String()
}

// renderList renders the snapshot list pane constrained to the given width.
func (v *TimelineView) renderList(width int) string {
	var b strings.Builder

	sectionHeader := lipgloss.NewStyle().Bold(true).Faint(true)
	b.WriteString(sectionHeader.Render(fmt.Sprintf("Snapshots (%d)", len(v.snapshots))) + "\n\n")

	for i, snap := range v.snapshots {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			style = style.Bold(true)
		}

		kind := classifySnapshot(snap)
		kindStyle := snapshotKindStyle(kind)
		marker := snapshotMarker(snap.SnapshotNo)
		var markerRendered string
		if snap.ParentID != nil {
			markerRendered = kindStyle.Faint(true).Render("⎇" + markerSuffix(marker))
		} else {
			markerRendered = kindStyle.Render(marker)
		}

		label := snap.Label
		if label == "" {
			label = snap.NodeID
		}
		maxLabelWidth := width - 14
		if maxLabelWidth < 1 {
			maxLabelWidth = 1
		}
		if len(label) > maxLabelWidth {
			label = label[:maxLabelWidth-3] + "..."
		}

		ts := snap.CreatedAt.Format("15:04:05")
		line := fmt.Sprintf("%s%s %s", cursor, markerRendered, style.Render(label))
		tsStr := lipgloss.NewStyle().Faint(true).Render(ts)

		lineWidth := lipgloss.Width(line)
		tsWidth := lipgloss.Width(tsStr)
		gap := width - lineWidth - tsWidth - 1
		if gap > 0 {
			line = line + strings.Repeat(" ", gap) + tsStr
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

// renderDetail renders the detail/diff pane for the selected snapshot.
func (v *TimelineView) renderDetail(width int) string {
	if len(v.snapshots) == 0 {
		return ""
	}

	snap := v.snapshots[v.cursor]
	kind := classifySnapshot(snap)
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)
	kindStyle := snapshotKindStyle(kind)

	// Title line with kind badge.
	title := titleStyle.Render(fmt.Sprintf("Snapshot %s", snapshotMarker(snap.SnapshotNo)))
	kindBadge := kindStyle.Bold(true).Render("[" + snapshotKindLabel(kind) + "]")
	b.WriteString(title + "  " + kindBadge)
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("Node:   ") + snap.NodeID + "\n")
	if snap.Iteration > 0 || snap.Attempt > 0 {
		b.WriteString(labelStyle.Render("Iter:   ") +
			fmt.Sprintf("%d / attempt %d", snap.Iteration, snap.Attempt) + "\n")
	}
	b.WriteString(labelStyle.Render("Time:   ") + snap.CreatedAt.Format("2006-01-02 15:04:05 UTC") + "\n")
	if snap.SizeBytes > 0 {
		b.WriteString(labelStyle.Render("Size:   ") + fmtBytes(snap.SizeBytes) + "\n")
	}
	if snap.ParentID != nil {
		parentRef := *snap.ParentID
		if len(parentRef) > 8 {
			parentRef = parentRef[:8] + "..."
		}
		b.WriteString(labelStyle.Render("Fork:   ") +
			lipgloss.NewStyle().Faint(true).Render("⎇ forked from "+parentRef) + "\n")
	}
	if snap.StateJSON != "" {
		hint := lipgloss.NewStyle().Faint(true).Render("  [Enter] inspect full state")
		b.WriteString(hint + "\n")
	}

	b.WriteString("\n")
	b.WriteString(v.renderDiffSection(snap, width))

	if v.pendingResult != nil {
		doneStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		runID := v.pendingResult.ID
		if len(runID) > 8 {
			runID = runID[:8]
		}
		b.WriteString("\n")
		b.WriteString(doneStyle.Render(fmt.Sprintf("✓ New run: %s (%s)", runID, v.pendingResult.Status)))
		b.WriteString("\n")
	}
	if v.pendingErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		b.WriteString("\n")
		b.WriteString(errStyle.Render(fmt.Sprintf("✗ Error: %v", v.pendingErr)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderDiffSection renders the diff portion of the detail pane.
func (v *TimelineView) renderDiffSection(snap smithers.Snapshot, width int) string {
	if v.cursor == 0 {
		return lipgloss.NewStyle().Faint(true).Render("  (first snapshot — no previous to diff)") + "\n"
	}

	prev := v.snapshots[v.cursor-1]
	diffKey := prev.ID + ":" + snap.ID

	if v.loadingDiff {
		return lipgloss.NewStyle().Faint(true).Render("  computing diff...") + "\n"
	}
	if err, ok := v.diffErrs[diffKey]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).
			Render(fmt.Sprintf("  diff unavailable: %v", err)) + "\n"
	}

	diff, ok := v.diffs[diffKey]
	if !ok {
		return lipgloss.NewStyle().Faint(true).Render("  [press d to load diff]") + "\n"
	}

	return renderSnapshotDiff(diff, prev.SnapshotNo, snap.SnapshotNo, width)
}

// renderBodyCompact renders the list with inline detail below the selected item,
// for terminal widths below 80 columns.
func (v *TimelineView) renderBodyCompact() string {
	var b strings.Builder

	for i, snap := range v.snapshots {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			style = style.Bold(true)
		}

		kind := classifySnapshot(snap)
		kindStyle := snapshotKindStyle(kind)
		marker := snapshotMarker(snap.SnapshotNo)
		var markerRendered string
		if snap.ParentID != nil {
			markerRendered = kindStyle.Faint(true).Render("\u238f" + markerSuffix(marker))
		} else {
			markerRendered = kindStyle.Render(marker)
		}

		label := snap.Label
		if label == "" {
			label = snap.NodeID
		}

		b.WriteString(cursor + markerRendered + " " + style.Render(label) + "\n")

		if i == v.cursor {
			faint := lipgloss.NewStyle().Faint(true)
			b.WriteString(faint.Render("    "+snap.NodeID) + "\n")
			b.WriteString(faint.Render("    "+snap.CreatedAt.Format("15:04:05")) + "\n")
			b.WriteString(faint.Render("    kind: "+snapshotKindLabel(kind)) + "\n")

			if i > 0 {
				prev := v.snapshots[i-1]
				diffKey := prev.ID + ":" + snap.ID
				if diff, ok := v.diffs[diffKey]; ok {
					summary := fmt.Sprintf("    +%d -%d ~%d",
						diff.AddedCount, diff.RemovedCount, diff.ChangedCount)
					b.WriteString(faint.Render(summary) + "\n")
				}
			}
			if snap.StateJSON != "" {
				b.WriteString(faint.Render("    [Enter] inspect state") + "\n")
			}
		}

		if i < len(v.snapshots)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderFooter renders the confirmation prompt or the normal help bar.
func (v *TimelineView) renderFooter() string {
	if v.inspecting {
		faint := lipgloss.NewStyle().Faint(true)
		hints := []string{
			"[↑↓] Scroll",
			"[Enter/q/Esc] Close inspector",
		}
		return faint.Render(strings.Join(hints, "  "))
	}

	if v.pendingAction != pendingNone && len(v.snapshots) > 0 {
		snap := v.snapshots[v.cursor]
		action := "Fork"
		if v.pendingAction == pendingReplay {
			action = "Replay"
		}
		prompt := fmt.Sprintf("  %s from %s? [y/N]: ", action, snapshotMarker(snap.SnapshotNo))
		return lipgloss.NewStyle().Bold(true).Render(prompt)
	}

	faint := lipgloss.NewStyle().Faint(true)
	hints := []string{
		"[↑↓] Navigate",
		"[←→] Panes",
		"[Enter] Inspect",
		"[f] Fork",
		"[r] Replay",
		"[d] Diff",
		"[R] Refresh",
		"[q/Esc] Back",
	}
	return faint.Render(strings.Join(hints, "  "))
}

// renderInspector renders the full-state inspector overlay for the selected
// snapshot.  It shows snapshot metadata and a pretty-printed, colour-coded
// JSON tree of StateJSON with scroll support via inspectorScroll.
func (v *TimelineView) renderInspector() string {
	if v.cursor < 0 || v.cursor >= len(v.snapshots) {
		return ""
	}

	snap := v.snapshots[v.cursor]
	kind := classifySnapshot(snap)
	kindStyle := snapshotKindStyle(kind)

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Faint(true)
	faint := lipgloss.NewStyle().Faint(true)

	// Header line.
	title := titleStyle.Render(
		fmt.Sprintf("Inspector: Snapshot %s", snapshotMarker(snap.SnapshotNo)),
	)
	kindBadge := kindStyle.Bold(true).Render("[" + snapshotKindLabel(kind) + "]")
	b.WriteString(title + "  " + kindBadge + "\n\n")

	// Metadata block.
	b.WriteString(labelStyle.Render("ID:     ") + snap.ID + "\n")
	b.WriteString(labelStyle.Render("Run:    ") + snap.RunID + "\n")
	b.WriteString(labelStyle.Render("Node:   ") + snap.NodeID + "\n")
	if snap.Label != "" {
		b.WriteString(labelStyle.Render("Label:  ") + snap.Label + "\n")
	}
	if snap.Iteration > 0 || snap.Attempt > 0 {
		b.WriteString(labelStyle.Render("Iter:   ") +
			fmt.Sprintf("%d / attempt %d", snap.Iteration, snap.Attempt) + "\n")
	}
	b.WriteString(labelStyle.Render("Time:   ") + snap.CreatedAt.Format("2006-01-02 15:04:05 UTC") + "\n")
	if snap.SizeBytes > 0 {
		b.WriteString(labelStyle.Render("Size:   ") + fmtBytes(snap.SizeBytes) + "\n")
	}
	if snap.ParentID != nil {
		parentRef := *snap.ParentID
		if len(parentRef) > 8 {
			parentRef = parentRef[:8] + "..."
		}
		b.WriteString(labelStyle.Render("Fork:   ") +
			lipgloss.NewStyle().Faint(true).Render("\u238f forked from "+parentRef) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("State JSON:") + "\n")

	// Render StateJSON as a pretty-printed, colour-coded tree.
	stateContent := renderPrettyJSON(snap.StateJSON, v.width)
	stateLines := strings.Split(stateContent, "\n")

	// Clamp scroll offset.
	scroll := v.inspectorScroll
	if scroll < 0 {
		scroll = 0
	}
	if len(stateLines) > 0 && scroll >= len(stateLines) {
		scroll = len(stateLines) - 1
	}

	// Reserve lines for header/footer overhead.
	reserved := 15
	maxStateLines := v.height - reserved
	if maxStateLines < 4 {
		maxStateLines = 4
	}

	end := scroll + maxStateLines
	if end > len(stateLines) {
		end = len(stateLines)
	}
	visible := stateLines[scroll:end]
	b.WriteString(strings.Join(visible, "\n"))
	b.WriteString("\n")

	// Scroll position hint when content exceeds the viewport.
	if len(stateLines) > maxStateLines {
		remaining := len(stateLines) - end
		scrollHint := fmt.Sprintf("  [\u2191/\u2193 scroll] line %d/%d", scroll+1, len(stateLines))
		if remaining > 0 {
			scrollHint += fmt.Sprintf(" (%d more)", remaining)
		}
		b.WriteString(faint.Render(scrollHint) + "\n")
	}

	return b.String()
}

// renderPrettyJSON attempts to pretty-print a JSON string for terminal display.
// Returns colour-coded indented JSON on success; falls back to wrapped raw text.
func renderPrettyJSON(rawJSON string, width int) string {
	if rawJSON == "" {
		return lipgloss.NewStyle().Faint(true).Render("  (empty)")
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return wrapText(rawJSON, width-4)
	}

	pretty, err := json.MarshalIndent(parsed, "  ", "  ")
	if err != nil {
		return wrapText(rawJSON, width-4)
	}

	return colorizeJSON(string(pretty))
}

// colorizeJSON applies colour highlights to a pretty-printed JSON string.
// Object keys are cyan, string values are green, numbers/bools are yellow,
// null is red, and structural punctuation is faint.
func colorizeJSON(pretty string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // cyan
	strStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	nullStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	punctStyle := lipgloss.NewStyle().Faint(true)

	lines := strings.Split(pretty, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, colorizeJSONLine(line, keyStyle, strStyle, numStyle, nullStyle, punctStyle))
	}
	return strings.Join(result, "\n")
}

// colorizeJSONLine applies colour to a single line of pretty-printed JSON.
// This is a best-effort heuristic that handles keys, string values, and
// scalar values without a full JSON tokenizer.
func colorizeJSONLine(
	line string,
	keyStyle, strStyle, numStyle, nullStyle, punctStyle lipgloss.Style,
) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// Key-value line: "key": value
	if strings.HasPrefix(trimmed, `"`) {
		colonIdx := strings.Index(trimmed, `": `)
		if colonIdx > 0 {
			rawKey := trimmed[:colonIdx+1] // includes closing quote
			rest := trimmed[colonIdx+3:]   // everything after \": \"
			return indent + keyStyle.Render(rawKey) + punctStyle.Render(`": `) +
				colorizeJSONValue(rest, strStyle, numStyle, nullStyle, punctStyle)
		}
	}

	return indent + colorizeJSONValue(trimmed, strStyle, numStyle, nullStyle, punctStyle)
}

// colorizeJSONValue applies colour to the value portion of a JSON line.
func colorizeJSONValue(v string, strStyle, numStyle, nullStyle, punctStyle lipgloss.Style) string {
	trailing := ""
	bare := v
	if strings.HasSuffix(v, ",") {
		trailing = ","
		bare = v[:len(v)-1]
	}

	var colorized string
	switch {
	case bare == "null":
		colorized = nullStyle.Render(bare)
	case bare == "true" || bare == "false":
		colorized = numStyle.Render(bare)
	case strings.HasPrefix(bare, `"`):
		colorized = strStyle.Render(bare)
	case bare == "{" || bare == "}" || bare == "[" || bare == "]" ||
		bare == "{}" || bare == "[]":
		colorized = punctStyle.Render(bare)
	default:
		colorized = numStyle.Render(bare)
	}

	if trailing != "" {
		return colorized + punctStyle.Render(trailing)
	}
	return colorized
}

// --- Standalone rendering helpers ---

// renderSnapshotDiff formats a SnapshotDiff for display in the detail pane.
func renderSnapshotDiff(diff *smithers.SnapshotDiff, fromNo, toNo, width int) string {
	if diff == nil {
		return ""
	}

	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Faint(true)
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))    // green
	removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	changeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	pathStyle := lipgloss.NewStyle().Faint(true)

	summary := fmt.Sprintf("Diff %s → %s  (+%d -%d ~%d)",
		snapshotMarker(fromNo), snapshotMarker(toNo),
		diff.AddedCount, diff.RemovedCount, diff.ChangedCount)
	b.WriteString(headerStyle.Render(summary) + "\n\n")

	if len(diff.Entries) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no changes)") + "\n")
		return b.String()
	}

	const maxEntries = 20
	for i, entry := range diff.Entries {
		if i >= maxEntries {
			remaining := len(diff.Entries) - maxEntries
			b.WriteString(lipgloss.NewStyle().Faint(true).
				Render(fmt.Sprintf("  ... +%d more entries", remaining)) + "\n")
			break
		}

		var opStyle lipgloss.Style
		var opSymbol string
		switch entry.Op {
		case "add":
			opStyle = addStyle
			opSymbol = "+"
		case "remove":
			opStyle = removeStyle
			opSymbol = "-"
		default: // "replace"
			opStyle = changeStyle
			opSymbol = "~"
		}

		valWidth := width - 6
		if valWidth < 10 {
			valWidth = 10
		}
		path := truncateMiddle(entry.Path, valWidth)
		b.WriteString("  " + opStyle.Render(opSymbol) + " " + pathStyle.Render(path) + "\n")

		if entry.OldValue != nil {
			old := truncate(fmt.Sprintf("%v", entry.OldValue), valWidth)
			b.WriteString("    " + removeStyle.Render("- "+old) + "\n")
		}
		if entry.NewValue != nil {
			nv := truncate(fmt.Sprintf("%v", entry.NewValue), valWidth)
			b.WriteString("    " + addStyle.Render("+ "+nv) + "\n")
		}
	}

	return b.String()
}

// snapshotMarker returns the display marker for a snapshot number.
// Uses Unicode encircled numbers for 1–20, bracketed numbers beyond that.
func snapshotMarker(n int) string {
	encircled := []string{
		"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩",
		"⑪", "⑫", "⑬", "⑭", "⑮", "⑯", "⑰", "⑱", "⑲", "⑳",
	}
	if n >= 1 && n <= len(encircled) {
		return encircled[n-1]
	}
	return fmt.Sprintf("[%d]", n)
}

// fmtBytes returns a human-readable size string (e.g. "1.2 KiB").
func fmtBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

// truncateMiddle shortens a string from the middle with "…" if it exceeds maxLen.
func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen || maxLen < 5 {
		return s
	}
	half := (maxLen - 1) / 2
	return s[:half] + "…" + s[len(s)-half:]
}

// --- Toast helpers ---

// forkSuccessToast returns a Cmd that emits a success toast after a fork.
// The toast shows the new run ID (truncated to 8 chars) and status.
func forkSuccessToast(run *smithers.ForkReplayRun) tea.Cmd {
	if run == nil {
		return nil
	}
	runID := run.ID
	if len(runID) > 8 {
		runID = runID[:8]
	}
	return func() tea.Msg {
		return components.ShowToastMsg{
			Title: "Fork created",
			Body:  fmt.Sprintf("Run %s  status: %s", runID, run.Status),
			Level: components.ToastLevelSuccess,
		}
	}
}

// replaySuccessToast returns a Cmd that emits a success toast after a replay.
// The toast shows the new run ID (truncated to 8 chars) and status.
func replaySuccessToast(run *smithers.ForkReplayRun) tea.Cmd {
	if run == nil {
		return nil
	}
	runID := run.ID
	if len(runID) > 8 {
		runID = runID[:8]
	}
	return func() tea.Msg {
		return components.ShowToastMsg{
			Title: "Replay started",
			Body:  fmt.Sprintf("Run %s  status: %s", runID, run.Status),
			Level: components.ToastLevelSuccess,
		}
	}
}

// actionErrorToast returns a Cmd that emits an error toast for fork/replay failures.
func actionErrorToast(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return func() tea.Msg {
		return components.ShowToastMsg{
			Title: "Action failed",
			Body:  err.Error(),
			Level: components.ToastLevelError,
		}
	}
}

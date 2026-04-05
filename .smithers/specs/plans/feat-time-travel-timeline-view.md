# Implementation Plan: feat-time-travel-timeline-view

## Goal

Deliver a fully interactive Time-Travel Timeline view (`internal/ui/views/timeline.go`) for the
Smithers TUI. Users navigate a run's snapshot history in a split-pane layout, inspect state
diffs between snapshots, and trigger fork/replay operations — all without leaving the TUI.

This ticket depends on `eng-time-travel-api-and-model` having landed. That ticket delivers
`internal/smithers/timetravel.go`, `internal/smithers/types_timetravel.go`, and the initial
Bubble Tea scaffolding for the timeline model. This ticket completes the view.

---

## Pre-flight Checks

Before writing any code, run:

```bash
go build ./...
go test ./internal/smithers/...
```

Both must pass. If the `eng-time-travel-api-and-model` dependency has not landed, stop and
wait for it. The timeline view cannot be built without `ListSnapshots`, `DiffSnapshots`,
`ForkRun`, `ReplayRun`, and the associated types.

Check that these types exist in `internal/smithers/types_timetravel.go`:
- `Snapshot` (with `ID`, `RunID`, `SnapshotNo`, `NodeID`, `Label`, `CreatedAt`, `StateJSON`,
  `SizeBytes`, `ParentID`)
- `SnapshotDiff` (with `FromID`, `ToID`, `FromNo`, `ToNo`, `Entries`, `AddedCount`,
  `RemovedCount`, `ChangedCount`)
- `DiffEntry` (with `Path`, `Op`, `OldValue`, `NewValue`)
- `ForkOptions`, `ReplayOptions`, `ForkReplayRun`

Check that these methods exist on `*smithers.Client`:
- `ListSnapshots(ctx, runID) ([]Snapshot, error)`
- `GetSnapshot(ctx, snapshotID) (*Snapshot, error)`
- `DiffSnapshots(ctx, fromID, toID) (*SnapshotDiff, error)`
- `ForkRun(ctx, snapshotID, ForkOptions) (*ForkReplayRun, error)`
- `ReplayRun(ctx, snapshotID, ReplayOptions) (*ForkReplayRun, error)`

---

## Step 1: Add `ActionOpenTimelineView` to the command palette

**File**: `internal/ui/dialog/actions.go`

Add after `ActionOpenApprovalsView` (line 96):

```go
// ActionOpenTimelineView is a message to navigate to the timeline view for a run.
ActionOpenTimelineView struct {
    RunID string
}
```

This message carries the run ID so the router knows which run's timeline to open. The `RunID`
field is empty when launched from the command palette (user must then navigate to a run first)
and non-empty when triggered via the `t` key from the Run Dashboard.

**Verification**: `go build ./internal/ui/dialog/...` passes.

---

## Step 2: Add "Timeline" entry to the command palette

**File**: `internal/ui/dialog/commands.go`

In the Smithers command block (near the "Approvals" entry), add a "Timeline" entry:

```go
commands = append(commands,
    NewCommandItem(c.com.Styles, "timeline", "Timeline", "", ActionOpenTimelineView{}),
    // ... existing entries ...
)
```

No keyboard shortcut at the top level — the timeline is a detail view, accessed from the Run
Dashboard via `t` or from the command palette with a run ID argument.

**Verification**: Open the command palette (`/` or `Ctrl+P`), type "timeline" — entry appears.

---

## Step 3: Build the `TimelineView` Bubble Tea model

**File**: `internal/ui/views/timeline.go` (new file, or extend the scaffold from the dependency ticket)

The view struct:

```go
package views

import (
    "context"
    "fmt"
    "strings"
    "time"

    "charm.land/bubbles/v2/key"
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    "github.com/charmbracelet/crush/internal/smithers"
)

// Compile-time interface check.
var _ View = (*TimelineView)(nil)

// pendingActionKind identifies which action awaits confirmation.
type pendingActionKind int

const (
    pendingNone   pendingActionKind = iota
    pendingFork
    pendingReplay
)
```

**Model fields**:

```go
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
    diffErr     error

    // Fork / replay confirmation
    pendingAction pendingActionKind
    pendingResult *smithers.ForkReplayRun
    pendingErr    error

    // Follow mode (auto-scroll to latest when run is still active)
    follow bool

    // Detail pane scroll
    detailScroll int
    detailLines  []string
    detailDirty  bool

    // Viewport
    width  int
    height int
}
```

**Constructor**:

```go
func NewTimelineView(client *smithers.Client, runID string) *TimelineView {
    return &TimelineView{
        client:    client,
        runID:     runID,
        loading:   true,
        follow:    true,
        diffs:     make(map[string]*smithers.SnapshotDiff),
        diffErrs:  make(map[string]error),
        detailDirty: true,
    }
}
```

**Verification**: `go build ./internal/ui/views/...` passes (struct defined, methods stubbed).

---

## Step 4: Implement message types

**File**: `internal/ui/views/timeline.go` (same file)

Define all message types used by the view. These may partially overlap with scaffolding from
the dependency ticket; reconcile or reuse as appropriate:

```go
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
```

---

## Step 5: Implement `Init`

**File**: `internal/ui/views/timeline.go`

`Init` fires two commands in parallel: load the snapshot list and start a refresh ticker if
the run is active. The ticker is a simple 5-second poll (a future ticket can replace with SSE).

```go
func (v *TimelineView) Init() tea.Cmd {
    return tea.Batch(
        v.fetchSnapshots(),
        v.refreshTick(),
    )
}

func (v *TimelineView) fetchSnapshots() tea.Cmd {
    runID := v.runID
    client := v.client
    return func() tea.Msg {
        snaps, err := client.ListSnapshots(context.Background(), runID)
        if err != nil {
            return timelineErrorMsg{err: err}
        }
        return timelineLoadedMsg{snapshots: snaps}
    }
}

func (v *TimelineView) fetchDiff(fromSnap, toSnap smithers.Snapshot) tea.Cmd {
    key := fromSnap.ID + ":" + toSnap.ID
    client := v.client
    return func() tea.Msg {
        diff, err := client.DiffSnapshots(context.Background(), fromSnap.ID, toSnap.ID)
        if err != nil {
            return timelineDiffErrorMsg{key: key, err: err}
        }
        return timelineDiffLoadedMsg{key: key, diff: diff}
    }
}

// refreshTick returns a command that fires timelineRefreshTickMsg after 5 seconds.
// Only called when the run is not terminal (checked by the caller).
func (v *TimelineView) refreshTick() tea.Cmd {
    return tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
        return timelineRefreshTickMsg{}
    })
}
```

**Verification**: `go build ./internal/ui/views/...` passes.

---

## Step 6: Implement `Update`

**File**: `internal/ui/views/timeline.go`

The Update method is the largest part of the implementation. Handle each message type:

```go
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
        return v, nil

    case timelineReplayDoneMsg:
        v.pendingAction = pendingNone
        v.pendingResult = msg.run
        v.pendingErr = nil
        return v, nil

    case timelineActionErrorMsg:
        v.pendingAction = pendingNone
        v.pendingErr = msg.err
        return v, nil

    case timelineRefreshTickMsg:
        // Re-fetch snapshot list if run is not terminal.
        // A future ticket can replace this with SSE.
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
```

Key handling is extracted into `handleKey`:

```go
func (v *TimelineView) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
    // If a confirmation prompt is active, handle only y/n/Esc.
    if v.pendingAction != pendingNone {
        return v.handleConfirmKey(msg)
    }

    switch {
    case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc", "alt+esc"))):
        return v, func() tea.Msg { return PopViewMsg{} }

    case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
        v.follow = false
        if v.cursor > 0 {
            v.cursor--
            v.detailDirty = true
            return v, v.prefetchAdjacentDiff()
        }

    case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
        v.follow = false
        if v.cursor < len(v.snapshots)-1 {
            v.cursor++
            v.detailDirty = true
            return v, v.prefetchAdjacentDiff()
        }

    case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
        // Go to first snapshot.
        v.follow = false
        v.cursor = 0
        v.detailDirty = true
        return v, v.prefetchAdjacentDiff()

    case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
        // Go to last snapshot.
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
        // Diff selected vs. previous (already loaded or trigger load).
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

    // Detail pane scroll when focused on right pane.
    case v.focusPane == 1 && key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
        if v.detailScroll > 0 {
            v.detailScroll--
        }
    case v.focusPane == 1 && key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
        v.detailScroll++
    }

    return v, nil
}

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
            run, err := client.ForkRun(context.Background(), snap.ID, smithers.ForkOptions{
                Label: label,
            })
            if err != nil {
                return timelineActionErrorMsg{err: err}
            }
            return timelineForkDoneMsg{run: run}
        }

    case pendingReplay:
        label := fmt.Sprintf("replay from snap %d", snap.SnapshotNo)
        return func() tea.Msg {
            run, err := client.ReplayRun(context.Background(), snap.ID, smithers.ReplayOptions{
                Label: label,
            })
            if err != nil {
                return timelineActionErrorMsg{err: err}
            }
            return timelineReplayDoneMsg{run: run}
        }
    }
    return nil
}

// prefetchAdjacentDiff ensures the diff between the selected snapshot and the previous
// one is loaded. Returns a fetch command if not yet in the cache.
func (v *TimelineView) prefetchAdjacentDiff() tea.Cmd {
    if v.cursor <= 0 || v.cursor >= len(v.snapshots) {
        return nil
    }
    from := v.snapshots[v.cursor-1]
    to := v.snapshots[v.cursor]
    key := from.ID + ":" + to.ID
    if _, ok := v.diffs[key]; ok {
        return nil // already cached
    }
    if _, ok := v.diffErrs[key]; ok {
        return nil // error cached, don't retry
    }
    v.loadingDiff = true
    return v.fetchDiff(from, to)
}
```

**Verification**: `go build ./internal/ui/views/...` passes.

---

## Step 7: Implement `View` — the rendering method

**File**: `internal/ui/views/timeline.go`

The `View` method composes: header line, divider, rail strip, divider, list+detail, divider,
confirmation prompt or help bar.

```go
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

    b.WriteString(v.renderRail())
    b.WriteString("\n")
    b.WriteString(v.renderDivider())
    b.WriteString("\n")

    body := v.renderBody()
    b.WriteString(body)

    b.WriteString(v.renderDivider())
    b.WriteString("\n")
    b.WriteString(v.renderFooter())
    b.WriteString("\n")

    return b.String()
}
```

Implement each sub-renderer:

### 7.1 Header

```go
func (v *TimelineView) renderHeader() string {
    titleStyle := lipgloss.NewStyle().Bold(true)
    hintStyle := lipgloss.NewStyle().Faint(true)

    runPart := v.runID
    if len(runPart) > 8 {
        runPart = runPart[:8]
    }

    title := "SMITHERS › Timeline › " + runPart
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
```

### 7.2 Rail Strip

The rail renders a compact horizontal strip showing the snapshot numbers. The selected snapshot
is highlighted. Up to 20 snapshots are shown; for more, truncate with `...[N]`.

```go
func (v *TimelineView) renderRail() string {
    if len(v.snapshots) == 0 {
        return ""
    }

    // Determine how many we can fit: each marker is ≤5 chars + "──" connector = ~7 chars.
    maxVisible := 20
    if v.width > 0 {
        maxVisible = (v.width - 4) / 7
        if maxVisible < 3 {
            maxVisible = 3
        }
    }

    var parts []string
    total := len(v.snapshots)
    shown := total
    if shown > maxVisible {
        shown = maxVisible
    }

    boldRev := lipgloss.NewStyle().Bold(true).Reverse(true)
    faint := lipgloss.NewStyle().Faint(true)
    normal := lipgloss.NewStyle()

    for i := 0; i < shown; i++ {
        snap := v.snapshots[i]
        marker := snapshotMarker(snap.SnapshotNo)
        var rendered string
        if i == v.cursor {
            rendered = boldRev.Render(marker)
        } else if snap.ParentID != nil {
            rendered = faint.Render("⎇" + marker[1:]) // strip leading space, add branch
        } else {
            rendered = normal.Render(marker)
        }
        parts = append(parts, rendered)
    }

    connector := faint.Render("──")
    rail := "  " + strings.Join(parts, connector)

    if total > shown {
        rail += faint.Render(fmt.Sprintf("──...+%d", total-shown))
    }

    return rail
}

// snapshotMarker returns the display marker for a snapshot number.
// Uses Unicode encircled numbers for 1–20, bracketed numbers beyond that.
func snapshotMarker(n int) string {
    encircled := []string{
        "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩",
        "⑪", "⑫", "⑬", "⑭", "⑮", "⑯", "⑰", "⑱", "⑲", "⑳",
    }
    if n >= 1 && n <= 20 {
        return encircled[n-1]
    }
    return fmt.Sprintf("[%d]", n)
}
```

### 7.3 Body: Split-Pane List + Detail/Diff

The body follows the `ApprovalsView` split-pane pattern (from `approvals.go:130-176`), with a
list on the left and the diff/detail on the right. Below 80 columns, it falls back to a compact
single-column layout.

```go
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

    // Reserve lines for header (3) + rail (2) + two dividers (2) + footer (2) = 9.
    reserved := 9
    maxLines := v.height - reserved
    if maxLines < 4 {
        maxLines = 4
    }

    // Use the larger of list and detail heights, capped at available height.
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
```

### 7.4 Snapshot List Pane

```go
func (v *TimelineView) renderList(width int) string {
    var b strings.Builder

    header := lipgloss.NewStyle().Bold(true).Faint(true).Render(
        fmt.Sprintf("Snapshots (%d)", len(v.snapshots)))
    b.WriteString(header + "\n\n")

    for i, snap := range v.snapshots {
        cursor := "  "
        style := lipgloss.NewStyle()
        if i == v.cursor {
            cursor = "▸ "
            style = style.Bold(true)
        }

        // Snapshot number marker
        marker := snapshotMarker(snap.SnapshotNo)
        if snap.ParentID != nil {
            marker = "⎇" + marker[1:]
        }

        // Label — truncate to fit
        label := snap.Label
        if label == "" {
            label = snap.NodeID
        }
        maxLabelWidth := width - 14
        if len(label) > maxLabelWidth {
            label = label[:maxLabelWidth-3] + "..."
        }

        // Elapsed time from run start (if available)
        // Use CreatedAt timestamp; relative display would need run start time.
        ts := snap.CreatedAt.Format("15:04:05")

        line := fmt.Sprintf("%s%s %s", cursor, marker, style.Render(label))
        tsStr := lipgloss.NewStyle().Faint(true).Render(ts)

        // Right-align timestamp if it fits.
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
```

### 7.5 Detail/Diff Pane

```go
func (v *TimelineView) renderDetail(width int) string {
    if len(v.snapshots) == 0 {
        return ""
    }

    snap := v.snapshots[v.cursor]
    var b strings.Builder

    titleStyle := lipgloss.NewStyle().Bold(true)
    labelStyle := lipgloss.NewStyle().Faint(true)

    // Snapshot header
    b.WriteString(titleStyle.Render(fmt.Sprintf("Snapshot %s", snapshotMarker(snap.SnapshotNo))))
    b.WriteString("\n")

    // Metadata
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
        b.WriteString(labelStyle.Render("Fork:   ") +
            lipgloss.NewStyle().Faint(true).Render("⎇ forked from "+*snap.ParentID[:8]+"...") + "\n")
    }

    b.WriteString("\n")

    // Diff section
    b.WriteString(v.renderDiffSection(snap, width))

    // Action result
    if v.pendingResult != nil {
        doneStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
        b.WriteString("\n")
        b.WriteString(doneStyle.Render(fmt.Sprintf("✓ New run: %s (%s)",
            v.pendingResult.ID[:8], v.pendingResult.Status)))
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
```

### 7.6 Diff Renderer (standalone function)

Extract diff rendering as a standalone function so it can be tested independently:

```go
// renderSnapshotDiff formats a SnapshotDiff for display in the detail pane.
func renderSnapshotDiff(diff *smithers.SnapshotDiff, fromNo, toNo, width int) string {
    if diff == nil {
        return ""
    }

    var b strings.Builder
    headerStyle := lipgloss.NewStyle().Bold(true).Faint(true)
    addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))   // green
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

    maxEntries := 20 // cap display to avoid overwhelming the pane
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

        path := truncateMiddle(entry.Path, width-6)
        b.WriteString("  " + opStyle.Render(opSymbol) + " " + pathStyle.Render(path) + "\n")

        // Show old/new values for replace, just new for add, just old for remove.
        valWidth := width - 6
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
```

### 7.7 Footer / Help Bar

```go
func (v *TimelineView) renderFooter() string {
    // Confirmation prompt overrides the normal help bar.
    if v.pendingAction != pendingNone {
        snap := v.snapshots[v.cursor]
        action := "fork"
        if v.pendingAction == pendingReplay {
            action = "replay"
        }
        prompt := fmt.Sprintf("  %s from %s? [y/N]: ",
            strings.Title(action), snapshotMarker(snap.SnapshotNo)) //nolint:staticcheck
        return lipgloss.NewStyle().Bold(true).Render(prompt)
    }

    faint := lipgloss.NewStyle().Faint(true)
    hints := []string{
        "[↑↓] Navigate",
        "[←→] Panes",
        "[f] Fork",
        "[r] Replay",
        "[d] Diff",
        "[R] Refresh",
        "[q/Esc] Back",
    }
    return faint.Render(strings.Join(hints, "  "))
}
```

### 7.8 Compact Body (narrow terminals)

```go
func (v *TimelineView) renderBodyCompact() string {
    var b strings.Builder

    for i, snap := range v.snapshots {
        cursor := "  "
        style := lipgloss.NewStyle()
        if i == v.cursor {
            cursor = "▸ "
            style = style.Bold(true)
        }

        marker := snapshotMarker(snap.SnapshotNo)
        label := snap.Label
        if label == "" {
            label = snap.NodeID
        }

        b.WriteString(cursor + marker + " " + style.Render(label) + "\n")

        // For the selected item, show inline detail below.
        if i == v.cursor {
            faint := lipgloss.NewStyle().Faint(true)
            b.WriteString(faint.Render("    "+snap.NodeID) + "\n")
            b.WriteString(faint.Render("    "+snap.CreatedAt.Format("15:04:05")) + "\n")

            // Show diff summary if available.
            if i > 0 {
                prev := v.snapshots[i-1]
                diffKey := prev.ID + ":" + snap.ID
                if diff, ok := v.diffs[diffKey]; ok {
                    summary := fmt.Sprintf("    +%d -%d ~%d",
                        diff.AddedCount, diff.RemovedCount, diff.ChangedCount)
                    b.WriteString(faint.Render(summary) + "\n")
                }
            }
        }

        if i < len(v.snapshots)-1 {
            b.WriteString("\n")
        }
    }

    return b.String()
}
```

### 7.9 Helper functions

Add these to `timeline.go` (some overlap with helpers in `approvals.go` but are local to the
view file to avoid coupling):

```go
func (v *TimelineView) renderDivider() string {
    if v.width > 0 {
        return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", v.width))
    }
    return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", 40))
}

// fmtBytes returns a human-readable file size string.
func fmtBytes(b int64) string {
    const unit = 1024
    if b < unit {
        return fmt.Sprintf("%d B", b)
    }
    div, exp := int64(unit), 0
    for n := b / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// truncateMiddle shortens a string from the middle with "…" if it exceeds maxLen.
func truncateMiddle(s string, maxLen int) string {
    if len(s) <= maxLen || maxLen < 5 {
        return s
    }
    half := (maxLen - 1) / 2
    return s[:half] + "…" + s[len(s)-half:]
}
```

### 7.10 `Name` and `ShortHelp`

```go
func (v *TimelineView) Name() string { return "timeline" }

func (v *TimelineView) ShortHelp() []string {
    if v.pendingAction != pendingNone {
        return []string{"[y] Confirm", "[N/Esc] Cancel"}
    }
    return []string{
        "[↑↓] Navigate",
        "[f] Fork",
        "[r] Replay",
        "[d] Diff",
        "[R] Refresh",
        "[q/Esc] Back",
    }
}
```

**Verification**: `go build ./internal/ui/views/...` passes with no errors.

---

## Step 8: Wire the `TimelineView` into the router

**File**: `internal/ui/model/ui.go`

Find the section that handles `ActionOpenAgentsView`, `ActionOpenApprovalsView`, and
`ActionOpenTicketsView`. Add a case for `ActionOpenTimelineView`:

```go
case dialog.ActionOpenTimelineView:
    runID := msg.RunID
    if runID == "" {
        // Opened from palette without a run ID — show an error toast or no-op.
        // A future ticket can add a run picker here.
        break
    }
    cmd := m.router.Push(views.NewTimelineView(m.smithersClient, runID))
    return m, cmd
```

**File**: `internal/ui/dialog/commands.go`

Ensure the "timeline" palette entry dispatches `ActionOpenTimelineView{}` (added in Step 2).

**Verification**: Launch the TUI, open the command palette, search "timeline", press Enter — no
crash. The view renders "No snapshots found" or the loading state.

---

## Step 9: Write unit tests for `TimelineView`

**File**: `internal/ui/views/timeline_test.go` (new)

Model the tests on `internal/ui/views/livechat_test.go`, which is the most complete view test
in the codebase. Cover:

### 9.1 Interface compliance
```go
func TestTimelineView_ImplementsView(t *testing.T) {
    var _ View = (*TimelineView)(nil)
}
```

### 9.2 Constructor defaults
```go
func TestNewTimelineView_Defaults(t *testing.T) {
    v := newTimelineView("run-001")
    assert.Equal(t, "run-001", v.runID)
    assert.True(t, v.loading)
    assert.True(t, v.follow)
    assert.Equal(t, 0, v.cursor)
    assert.NotNil(t, v.diffs)
    assert.NotNil(t, v.diffErrs)
}
```

### 9.3 Snapshot loading
```go
// TestTimelineView_Update_SnapshotsLoaded
// TestTimelineView_Update_SnapshotsError
// TestTimelineView_Update_SnapshotsEmpty
```

### 9.4 Diff loading
```go
// TestTimelineView_Update_DiffLoaded_CachedByKey
// TestTimelineView_Update_DiffError_CachedByKey
// TestTimelineView_Update_DiffNotRefetched_IfCached
```

### 9.5 Keyboard navigation
```go
// TestTimelineView_Update_DownMoveCursor
// TestTimelineView_Update_UpMoveCursor_NoBelowZero
// TestTimelineView_Update_GGoToFirst
// TestTimelineView_Update_ShiftGGoToLast
// TestTimelineView_Update_EscPopsView
// TestTimelineView_Update_QPopsView
// TestTimelineView_Update_FollowTurnedOffOnManualNav
```

### 9.6 Fork/replay confirmation flow
```go
// TestTimelineView_Update_FSetsPendingFork
// TestTimelineView_Update_RSetsPendingReplay
// TestTimelineView_Update_YConfirmsFork_DispatchesCmd
// TestTimelineView_Update_NOtherKeyCancelsConfirmation
// TestTimelineView_Update_ForkDone_ClearsState
// TestTimelineView_Update_ReplayDone_ClearsState
// TestTimelineView_Update_ActionError_ClearsState
```

### 9.7 View rendering
```go
// TestTimelineView_View_LoadingState
// TestTimelineView_View_ErrorState
// TestTimelineView_View_EmptyState
// TestTimelineView_View_RendersSnapshots
// TestTimelineView_View_ContainsRunID
// TestTimelineView_View_ConfirmationPrompt_Fork
// TestTimelineView_View_ConfirmationPrompt_Replay
// TestTimelineView_View_RailMarkers_CircledNumbers
// TestTimelineView_View_RailMarkers_BeyondTwenty
// TestTimelineView_View_SplitPane_WideTerminal
// TestTimelineView_View_CompactLayout_NarrowTerminal
```

### 9.8 Helper functions
```go
// TestSnapshotMarker_OneToTwenty
// TestSnapshotMarker_BeyondTwenty
// TestRenderSnapshotDiff_EmptyEntries
// TestRenderSnapshotDiff_AddEntry
// TestRenderSnapshotDiff_RemoveEntry
// TestRenderSnapshotDiff_ReplaceEntry
// TestRenderSnapshotDiff_TruncatesAtTwentyEntries
// TestFmtBytes_Sizes
// TestTruncateMiddle_Short
// TestTruncateMiddle_Long
```

### 9.9 Integration-style: client exec wiring
```go
// TestTimelineView_FetchSnapshots_DoesNotPanic
// (mirrors TestLiveChatView_FetchRunCmd_DoesNotPanic in livechat_test.go)
```

Use the same pattern as `livechat_test.go`: create a `smithers.NewClient()` with no server,
drive the model by injecting messages via `Update`, and call `View()` on the result.

**Verification**: `go test ./internal/ui/views/... -run TestTimeline` passes with all new tests.

---

## Step 10: Write the VHS recording test

**File**: `tests/vhs/timeline-happy-path.tape` (new)

```
# Timeline view happy-path smoke recording.
Output tests/vhs/output/timeline-happy-path.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Open timeline via command palette
Type "/"
Sleep 500ms
Type "timeline"
Sleep 500ms
Enter
Sleep 2s

Screenshot tests/vhs/output/timeline-happy-path.png

Ctrl+c
Sleep 1s
```

This tape follows the existing pattern used by `smithers-domain-system-prompt.tape` and
`branding-status.tape` in `tests/vhs/`. The timeline will show the "No snapshots found"
state (since no real Smithers server is running in the test environment), but confirms the
view renders without crashing.

**Verification**: `vhs tests/vhs/timeline-happy-path.tape` produces a `.gif` file without errors.

---

## Step 11: Final integration verification

Run the full test suite:

```bash
go build ./...
go test ./...
```

Both must pass with zero failures. Then do a manual smoke test:

1. Launch the TUI: `go run .`
2. Open command palette (`/` or `Ctrl+P`), type "timeline", press Enter.
3. Confirm view renders with appropriate loading/empty state.
4. Press `Esc` — confirm the router pops back to the chat view.
5. (If a Smithers server is running with a run that has snapshots):
   Navigate to `/timeline <run_id>`, confirm snapshots appear, navigate with `↑`/`↓`,
   confirm diff loads, press `f` + `y` and verify fork confirmation flow.

---

## File Plan

| File | Action |
|------|--------|
| `internal/ui/dialog/actions.go` | Add `ActionOpenTimelineView` struct |
| `internal/ui/dialog/commands.go` | Add "timeline" palette entry |
| `internal/ui/views/timeline.go` | New — full `TimelineView` implementation |
| `internal/ui/views/timeline_test.go` | New — unit tests |
| `internal/ui/model/ui.go` | Wire `ActionOpenTimelineView` to router push |
| `tests/vhs/timeline-happy-path.tape` | New — VHS smoke recording |

No changes are needed to `internal/smithers/` (all client methods provided by the dependency).

---

## Validation

| Check | Command |
|-------|---------|
| Compilation | `go build ./...` |
| Unit tests | `go test ./internal/ui/views/... -run TestTimeline` |
| Full test suite | `go test ./...` |
| VHS recording | `vhs tests/vhs/timeline-happy-path.tape` |
| Manual smoke | `go run .` → command palette → "timeline" → navigate → Esc |

---

## Open Questions

1. **Stub or live `eng-time-travel-api-and-model`**: If the dependency scaffold created
   `timeline.go` with stub `Init`/`Update`/`View` implementations, this ticket should
   overwrite/replace them entirely (do not try to layer on top of stubs). Check the
   dependency state before starting Step 3.

2. **`parseRunJSON` vs `parseForkReplayRunJSON`**: The `timetravel.go` file has both
   `parseRunJSON` and `parseForkReplayRunJSON` (currently `parseRunJSON` is called in
   `ForkRun` and `ReplayRun` — this appears to be using `Run` from `types_timetravel.go`
   rather than `RunSummary` from `types_runs.go`). The timeline view's display of fork results
   uses `ForkReplayRun.ID` and `.Status` which are available in both types. No special handling
   needed, but confirm the return type of `ForkRun`/`ReplayRun` is `*ForkReplayRun` before
   writing the `timelineForkDoneMsg` struct.

3. **`tea.Tick` import path**: The codebase uses `charm.land/bubbletea/v2` (not the standard
   `github.com/charmbracelet/bubbletea`). Verify `tea.Tick` is available in this version and
   check the signature — it should be `tea.Tick(d time.Duration, fn func(time.Time) tea.Msg)`.

4. **`strings.Title` deprecation**: `livechat.go` uses `strings.Title` for display (with a
   staticcheck suppression comment). Follow the same pattern for the confirmation prompt. Do
   not introduce a `golang.org/x/text` dependency.

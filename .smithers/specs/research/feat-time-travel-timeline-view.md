# Research: feat-time-travel-timeline-view

## Summary

This ticket builds the full Time-Travel Timeline view (`internal/ui/views/timeline.go`) on top of
the client methods and Bubble Tea scaffolding delivered by `eng-time-travel-api-and-model`. The
view lets users navigate a run's snapshot history, inspect state at any checkpoint, diff adjacent
snapshots, fork a new run from a snapshot, and replay from any point. It is accessed via
`/timeline <run_id>` from the command palette or by pressing `t` on a run in the Run Dashboard.

---

## 1. Timeline Visualization Approaches in Terminal

### 1.1 Orientation: Vertical vs. Horizontal vs. Compact

Three layouts are common in terminal-based timeline tools. Each has trade-offs given the
Smithers use case.

**Horizontal rail** (used in the design wireframe `02-DESIGN.md §3.6`):
```
①──②──③──④──⑤──⑥──⑦
│  │  │  │  │  │  └─ [now] review-auth attempt 1 complete
│  │  │  │  │  └──── lint-check complete ✓
│  │  │  │  └─────── test-runner complete ✓
│  │  │  └────────── build complete ✓
│  │  └───────────── fetch-deps complete ✓
│  └──────────────── parse-config complete ✓
└─────────────────── workflow started
```
Pros: Familiar "timeline" metaphor; all snapshots visible at once; encircled numbers make
cursor position clear; labels read top-to-bottom matching a narrative sequence.
Cons: Width-limited — a run with 30+ snapshots wraps or truncates; the rail line plus label
indentation consumes ~60 columns leaving little room for detail.

**Vertical scrollable list** (used by most TUI debuggers, e.g., k9s event lists):
```
 ① workflow started                  04/01 12:00:00.000
 ② parse-config complete ✓           04/01 12:00:02.341
▸③ fetch-deps complete ✓             04/01 12:00:14.812
 ④ build complete ✓                  04/01 12:00:45.203
 ⑤ test-runner complete ✓            04/01 12:01:22.110
 ⑥ lint-check complete ✓             04/01 12:01:34.571
 ⑦ review-auth attempt 1 complete    04/01 12:03:48.900
```
Pros: Scales to any snapshot count; easy to scroll; leaves the right column free for a
detail/diff pane; consistent with the existing `ApprovalsView` split-pane pattern already
in this codebase.
Cons: Loses the "rail" metaphor; nodes no longer visually connected.

**Compact horizontal** (fits in a narrow status strip — not suitable as the primary view):
```
①>②>③>[④]>⑤>⑥>⑦
```
Useful only as a secondary navigation strip at the top of a split layout.

**Decision**: Use a **hybrid** — compact horizontal rail at the top of the view for at-a-glance
position (up to ~15 snapshots before truncating to `...[N]`), with a vertical scrollable list
below that expands each snapshot with its label, node, and timestamp. The cursor in the list
drives the selection; the rail auto-advances to reflect the selected position. This matches
the wireframe intent while remaining practical for runs with many snapshots.

### 1.2 Snapshot Marker Symbols

Unicode encircled numbers are defined for 1–20 (①–⑳) in Basic Multilingual Plane. Beyond 20,
fall back to `[N]`. The selected snapshot uses bold+reverse styling. Fork origins (snapshots
with a `ParentID` set) can be marked with a branch glyph `⎇` or `⑂`.

| Condition | Symbol |
|-----------|--------|
| Selected cursor | `▸①` bold+reverse |
| Fork origin | `⎇③` faint |
| Normal | ` ①` |
| Beyond 20 | ` [21]` |
| Current run position (last) | ` [N] ●` with "now" indicator |

### 1.3 Width Responsiveness

The view must handle terminal widths from 60 (minimum usable) to 220+ columns.

| Width range | Layout |
|-------------|--------|
| < 80 cols | Compact: vertical list only, no detail pane; diff shown inline below selected item |
| 80–119 cols | Split: list takes 38 cols, divider 3, detail takes remainder (~40–78 cols) |
| 120+ cols | Split: list takes 40 cols, divider 3, detail takes remainder |

This matches the responsive logic already used in `ApprovalsView.View()` (line 133-139 of
`internal/ui/views/approvals.go`).

---

## 2. Snapshot Rendering: State Diffs and Metadata Display

### 2.1 Snapshot Metadata Panel

For the selected snapshot, the detail pane shows:
- **Header**: `Snapshot ③` bold, with the human-readable label
- **Node**: which workflow node was active (`NodeID`, `Iteration`, `Attempt`)
- **Timestamp**: absolute wall time plus elapsed-since-run-start
- **Size**: storage size of the serialized state (`SizeBytes`)
- **Fork origin**: if `ParentID` is set, show "Forked from snapshot X"
- **State summary**: key counts from `StateJSON` (message count, tool call count) parsed
  lazily from the JSON string — avoid full deserialization on every render

### 2.2 Diff Display

The `SnapshotDiff` returned by `client.DiffSnapshots(ctx, fromID, toID)` contains a slice of
`DiffEntry` values with `Path`, `Op` (add/remove/replace), `OldValue`, and `NewValue`, plus
aggregate counts (`AddedCount`, `RemovedCount`, `ChangedCount`).

**Rendering strategy** for the diff pane:
```
Snapshot ③ → ④  (+2 added, -0 removed, ~1 changed)

  ~ messages[3].content
    - "I'll start by reading the auth..."
    + "I've reviewed the auth module..."

  + toolCalls[4]
    {"name":"edit","input":{"file":"src/auth/middleware.ts",...}}

  ~ nodeState.lint-check
    - "pending"
    + "running"
```

- `~` prefix for replace, `+` for add, `-` for remove (git-style diff convention).
- Values are truncated at `width - 10` characters with ellipsis. Full JSON shown in a
  horizontally scrollable or wrappable region only if terminal is wide enough.
- Empty diff (no changes) shows: `  (no changes between these snapshots)`.
- Diff is loaded lazily: only fetched when a snapshot pair is selected (cursor moves
  from one snapshot to an adjacent one). Diff for the initial selection (snapshot N–1 → N)
  is pre-fetched on `Init`.

### 2.3 State JSON Viewer

Pressing `Enter` on a selected snapshot in the list expands a "snapshot detail" modal or
sub-pane showing the full `StateJSON` pretty-printed. Because `StateJSON` can be very large
(multi-MB for long agent sessions), this is rendered as a scrollable text area with the same
line-based viewport pattern used by `LiveChatView` (`renderedLines` cache with a dirty flag).

For v1, the detail view renders the raw pretty-printed JSON. A future ticket can add JSON tree
folding via the planned `jsontree.go` component referenced in `03-ENGINEERING.md §2.3`.

---

## 3. Navigation UX

### 3.1 Primary Keybindings

The design doc (`02-DESIGN.md §3.6`) specifies `←`/`→` for snapshot navigation. Given the
vertical list layout, up/down are more natural for scrolling the list; left/right become
lateral navigation between the "timeline" and "diff/detail" pane.

Proposed keybindings:

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up in snapshot list |
| `↓` / `j` | Move cursor down in snapshot list |
| `←` / `h` | Focus left pane (snapshot list) if split view |
| `→` / `l` | Focus right pane (diff/detail) if split view |
| `d` | Toggle diff for selected snapshot vs. previous |
| `D` | Diff selected snapshot vs. a user-chosen snapshot (prompt with cursor in list) |
| `f` | Fork run from selected snapshot |
| `r` | Replay run from selected snapshot |
| `Enter` | Inspect snapshot state (full `StateJSON` view) |
| `R` (capital) | Refresh snapshot list |
| `q` / `Esc` | Pop view (return to previous view) |

### 3.2 Cursor Auto-Advance and Follow Mode

When the run is still active (status not terminal), new snapshots arrive as the run progresses.
The view should auto-scroll to the latest snapshot when added (analogous to `follow` mode in
`LiveChatView`). A `follow` bool field on the model enables/disables this. Follow turns off
the moment the user manually moves the cursor.

### 3.3 Jumping Between Snapshots

For runs with many snapshots, adding a `g`/`G` (first/last) shortcut matches standard pager
convention. A `/<number>` or `:N` prompt to jump to snapshot N by number is deferred to a
future ticket; for v1, scrolling is sufficient.

### 3.4 Fork/Replay Confirmation Flow

Forking and replaying are destructive in the sense that they create new runs. To prevent
accidental triggers, pressing `f` or `r` should show an inline confirmation prompt before
dispatching the client call:

```
  Fork from snapshot ③? [y/N]:
```

The prompt is rendered as a single-line overlay at the bottom of the view, replacing the
help bar text. A `y` keypress fires the fork; any other key cancels. The confirmation state
is modeled as a `pendingAction` field on the model struct.

### 3.5 Post-Fork/Replay Navigation

After a successful fork or replay, the response includes the new `ForkReplayRun.ID`. The view
emits a navigation message (equivalent to `ActionOpenRunsView` but for the new run) so the
router can push the Run Dashboard filtered to the new run ID. For v1, a toast notification
`"Forked → run abc123"` with run ID is sufficient; deep navigation is a future enhancement.

---

## 4. Integration with the Time-Travel API Client

### 4.1 Available Client Methods (from `eng-time-travel-api-and-model`)

The dependency ticket delivers these methods on `*smithers.Client`:

```go
ListSnapshots(ctx context.Context, runID string) ([]Snapshot, error)
GetSnapshot(ctx context.Context, snapshotID string) (*Snapshot, error)
DiffSnapshots(ctx context.Context, fromID, toID string) (*SnapshotDiff, error)
ForkRun(ctx context.Context, snapshotID string, opts ForkOptions) (*ForkReplayRun, error)
ReplayRun(ctx context.Context, snapshotID string, opts ReplayOptions) (*ForkReplayRun, error)
```

All three transport tiers are implemented (HTTP → SQLite → exec), matching the pattern used
by `ListPendingApprovals`, `ExecuteSQL`, and all other client methods in this codebase.

### 4.2 Loading Strategy

The view loads data in layers to maintain responsiveness:

1. **`Init`**: Fire `ListSnapshots(ctx, runID)` to populate the list. Also pre-fetch the diff
   between the last two snapshots (most commonly the user's starting point of interest).
2. **On cursor move**: If the cursor moves to a new position, lazy-load the diff between
   `snapshots[cursor-1]` and `snapshots[cursor]`. Cache loaded diffs in a
   `map[string]*SnapshotDiff` keyed on `"fromID:toID"` to avoid repeated fetches.
3. **On `Enter`**: Load the full `Snapshot.StateJSON` for the selected snapshot (it may be
   truncated in the list response depending on the API). Cache separately.
4. **On `f`/`r`**: After confirmation, call `ForkRun` or `ReplayRun`. Show a loading indicator
   ("Forking...") while the operation is in flight.

### 4.3 SSE / Live Updates

When the run is still active, new snapshots can arrive. The existing `RunEventMsg` SSE
infrastructure (`types_runs.go`) delivers run-level events. The timeline view can poll
`ListSnapshots` periodically (every 5 seconds) while the run is non-terminal, using a
`tea.Tick` command. This avoids adding a new SSE endpoint for snapshot creation events.
When a new snapshot appears, append it to the list and auto-scroll if `follow` is true.

### 4.4 Error Handling

- `ListSnapshots` error: show an inline error state (same pattern as `AgentsView`, `TicketsView`).
- `DiffSnapshots` error: show an error message in the diff pane (`"diff unavailable: <err>"`).
  This is a non-fatal error — the user can still navigate and fork/replay.
- `ForkRun` / `ReplayRun` error: show an inline error notification (toast or inline message).
  Cancel the pending confirmation state.

---

## 5. Dependency on `eng-time-travel-api-and-model`

This ticket is a strict superset of `eng-time-travel-api-and-model`. The dependency ticket
provides:
- All four smithers client methods
- `Snapshot`, `SnapshotDiff`, `DiffEntry`, `ForkOptions`, `ReplayOptions`, `ForkReplayRun`
  types in `internal/smithers/types_timetravel.go`
- Bubble Tea scaffolding: `TimelineView` struct, `Init`/`Update`/`View` method shells,
  `timelineLoadedMsg`, `snapshotSelectedMsg`, `replayRequestedMsg` message types

This ticket fleshes out the scaffolding into a fully interactive view.

**Caution for implementation**: The dependency ticket may have used a stub `Run` type in
`internal/smithers/timetravel.go` that differs from `RunSummary` in `types_runs.go`.
The `ForkReplayRun` type in `types_timetravel.go` uses `time.Time` timestamps directly
(not millisecond integers), so care is needed at the boundary where the timeline view
interacts with run state coming from the runs dashboard.

---

## 6. Existing Codebase Patterns to Reuse

| Pattern | Source file | Reuse in timeline |
|---------|-------------|-------------------|
| `View` interface | `internal/ui/views/router.go` | `TimelineView` implements same interface |
| Split-pane rendering | `internal/ui/views/approvals.go:131-175` | List on left, detail/diff on right |
| `padRight` helper | `internal/ui/views/approvals.go:335-341` | Already in package `views` |
| Loading/error/empty states | all existing views | Same three-branch pattern |
| `renderedLines` cache + `linesDirty` | `internal/ui/views/livechat.go:416-478` | Diff pane text wrapping |
| `fmtDuration` | `internal/ui/views/livechat.go:504-510` | Elapsed time display |
| Compact horizontal rail header | (new) | Top N snapshots in the header strip |
| `PopViewMsg` | `internal/ui/views/router.go` | Esc/q to go back |
| `ActionOpenTimeline` message | `internal/ui/dialog/actions.go` | Push timeline view from runs view |

---

## 7. Open Questions

1. **Snapshot count ceiling**: Does the HTTP API return all snapshots for a run or paginate?
   For runs with thousands of snapshots (long-running agents), the initial `ListSnapshots`
   call could be slow. Plan: add a `limit` parameter to `ListSnapshots` in a follow-up ticket;
   for v1 assume reasonable snapshot counts (< 200) where full list load is acceptable.

2. **State JSON size**: `StateJSON` can be multi-MB for long agent sessions. The snapshot list
   response from the HTTP API likely omits `StateJSON` (bandwidth concern) and includes it
   only in `GetSnapshot`. Confirm with the API spec or test — if the list includes full state,
   the `GetSnapshot` on `Enter` is redundant but harmless.

3. **Diff latency**: `DiffSnapshots` involves TypeScript-runtime processing and can be slow
   for large states. Pre-fetching the diff for the last pair on `Init` amortizes the wait for
   the common case. Consider adding a `(computing diff...)` loading indicator in the detail
   pane while the diff is in flight.

4. **Fork modal depth**: Should `f` open a modal form to override workflow path and inputs
   (matching `ForkOptions.WorkflowPath` and `ForkOptions.Inputs`)? For v1: no modal — fork
   with default options (empty `ForkOptions`). Label can be auto-generated as
   `"fork from snap N"`. A richer fork form is a future ticket.

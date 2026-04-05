## Existing Crush Surface

### View system patterns
- The `views.View` interface is defined at `internal/ui/views/router.go:6-20` and requires `Init() tea.Cmd`, `Update(msg tea.Msg) (View, tea.Cmd)`, `View() string`, `Name() string`, `SetSize(width, height int)`, and `ShortHelp() []key.Binding`.
- The `Router` is a slice-backed view stack at `router.go:33-199`. `Push` calls `v.Init()`, appends, and invokes `OnFocus` if the view implements `Focusable`. `Pop` drops the tail and invokes `OnBlur`/`OnFocus`. `PopViewMsg{}` returned from `Update` is caught by `ui.go` around line 1474 to pop the stack.
- `PushView(v View)` is a convenience wrapper that uses the router's stored dimensions (`router.go:161-163`).
- All concrete Smithers views follow a uniform structural pattern: `<domain>LoadedMsg` / `<domain>ErrorMsg` private message types; a struct with `client *smithers.Client`, `loading bool`, `err error`, `cursor int`, `width int`, `height int`; and `Init`/`Update`/`View`/`Name`/`SetSize`/`ShortHelp`. Reference implementations: `agents.go`, `tickets.go`, `approvals.go`, `livechat.go`.

### RunsView (parent, already shipped)
- `RunsView` at `internal/ui/views/runs.go:27-163`: client pointer, `[]smithers.RunSummary` slice, cursor, width, height, loading, err.
- `Init()` calls `client.ListRuns(ctx, RunFilter{Limit: 50})` and dispatches `runsLoadedMsg` or `runsErrorMsg`.
- `Update()` handles loaded/error msgs, `tea.WindowSizeMsg`, and key bindings (`esc`/`alt+esc` → pop, `up`/`k`, `down`/`j`, `r` → reload, `enter` → currently no-op with comment "future: drill into run inspector").
- The `enter` case is the hook point: when `runs-inspect-summary` lands, replace the no-op with a push of `RunInspectView`.
- `components.RunTable` is a stateless rendering component at `internal/ui/components/runtable.go` used by `RunsView.View()`.

### LiveChatView (sibling, canonical scrollable-view reference)
- `LiveChatView` at `internal/ui/views/livechat.go:51-517`: multi-state view with loadingRun, loadingBlocks, follow mode, line cache, scroll position.
- Renders a `renderHeader()` / `renderSubHeader()` / `renderDivider()` / `renderBody()` layered layout pattern.
- `renderedLines()` rebuilds and caches a `[]string` of wrapped lines whenever `linesDirty == true` (set on new data or width change). `scrollToBottom()` clamps to `len(lines) - visibleHeight`.
- `visibleHeight()` at `livechat.go:502` reserves lines for chrome: `height - 5`.
- This line-cache + scroll pattern is the recommended approach for the run inspector's node-list panel.

### ApprovalsView split-pane (reference for two-panel layout)
- `ApprovalsView` at `internal/ui/views/approvals.go:28`: renders a left `renderList` panel and a right `renderDetail` panel when `v.width >= 80`, collapses to single-column on narrow terminals.
- `padRight` helper at `helpers.go:11` and `truncate` at `helpers.go:19` are in the `views` package and available to all views.
- `formatPayload` at `helpers.go:46` pretty-prints JSON payloads; useful for the Input/Output tabs that downstream tickets will add.

### Registry and navigation wiring
- `DefaultRegistry()` at `registry.go:49` pre-loads view factories for "agents", "approvals", "tickets". The `RunInspectView` does not need registry registration — it is pushed directly from `RunsView.Update()` on Enter, not from the command palette.
- `dialog.ActionOpenRunsView` is already wired in `ui.go`. The inspect view needs no new action type — it is a child navigation event internal to the runs group.

---

## Smithers Client Surface

### InspectRun
- `Client.InspectRun(ctx, runID string) (*RunInspection, error)` at `internal/smithers/runs.go:198-216`: fetches `RunSummary` via `GetRunSummary` (HTTP → SQLite → exec), then enriches with `[]RunTask` from `getRunTasks` (SQLite → exec).
- `getRunTasks` at `runs.go:220-225`: prefers SQLite (`sqliteGetRunTasks`), falls back to `execGetRunTasks`.
- `sqliteGetRunTasks` at `runs.go:228-238`: queries `_smithers_nodes WHERE run_id = ?` ordering by `updated_at_ms ASC`.
- `execGetRunTasks` at `runs.go:241-266`: shells out to `smithers inspect <runID> --nodes --format json`. Handles both `{ tasks: [...] }` and bare `[]RunTask` JSON shapes.
- Task enrichment is best-effort: errors are silently swallowed and the caller receives a `RunInspection` with an empty `Tasks` slice rather than an error.

### RunInspection type
- `RunInspection` at `internal/smithers/types_runs.go:67-71`:
  ```go
  type RunInspection struct {
      RunSummary
      Tasks    []RunTask `json:"tasks,omitempty"`
      EventSeq int       `json:"eventSeq,omitempty"`
  }
  ```
- `RunSummary` fields: `RunID`, `WorkflowName`, `WorkflowPath`, `Status RunStatus`, `StartedAtMs *int64`, `FinishedAtMs *int64`, `Summary map[string]int`, `ErrorJSON *string`.
- `RunTask` fields: `NodeID string`, `Label *string`, `Iteration int`, `State TaskState`, `LastAttempt *int`, `UpdatedAtMs *int64`.

### TaskState values
- Defined at `types_runs.go:30-38`: `pending`, `running`, `finished`, `failed`, `cancelled`, `skipped`, `blocked`.
- State → suggested glyph mapping for the node list: `running` → `●`, `finished` → `✓`, `failed` → `✗`, `pending` → `○`, `cancelled` → `–`, `skipped` → `↷`, `blocked` → `⏸`.

### RunStatus.IsTerminal()
- `RunStatus.IsTerminal()` at `types_runs.go:18-25`: `finished | failed | cancelled` → true.
- Used by `LiveChatView` to suppress the streaming indicator. The same check gates the "live" annotation in the run inspector header.

### SSE streaming (available for active runs)
- `Client.StreamRunEvents(ctx, runID)` at `runs.go:312-449`: opens `GET /v1/runs/:id/events?afterSeq=-1` and returns `<-chan interface{}` emitting `RunEventMsg`, `RunEventErrorMsg`, `RunEventDoneMsg`.
- `RunEvent` fields include `Type`, `RunID`, `NodeID`, `Status`, `TimestampMs`, `Seq`.
- The inspector view can subscribe to this channel and update node states in real time for running runs. This is an optional enhancement beyond the base ticket's scope but the channel is ready.

### GetRunSummary
- `Client.GetRunSummary(ctx, runID)` at `runs.go:126-160`: same three-tier transport (HTTP → SQLite → exec) returning `*RunSummary`.
- The inspector calls `InspectRun` which internally calls `GetRunSummary`, so there is no need to call both.

---

## Upstream Smithers Reference

### GUI NodeInspector (source context from ticket)
- `../smithers/gui/src/routes/runs/NodeInspector.tsx` referenced in the ticket's `sourceContext`.
- The GUI's NodeInspector split the page into: (left) node list with state indicators, (right) detail tabs — Input, Output, Config, Chat.
- The TUI counterpart (`runinspect.go`) should mirror this layout: a left node-list panel and a right detail area. On narrow terminals (< 80 cols), collapse to single-column with tab-based switching.

### Run inspect CLI command
- `smithers inspect <runID> --nodes --format json` is the exec fallback path already handled by `execGetRunTasks`.
- `smithers inspect <runID> --format json` (without `--nodes`) returns the run summary.
- Both are already wired in `InspectRun` / `getRunTasks`.

### Node input/output data
- Node-level input and output payloads are stored in the Smithers SQLite database but are not yet surfaced by `RunTask` / `RunInspection`. Downstream tickets (`runs-task-input-tab`, `runs-task-output-tab`) will require a new `GetNodeDetail(ctx, runID, nodeID)` client method.
- For this base ticket, the node list only needs `NodeID`, `Label`, `State`, `Iteration`, `UpdatedAtMs` — all present in `RunTask`.

---

## Layout Research

### Two-panel pattern
- `ApprovalsView` demonstrates width-aware two-panel layout. For the inspector: left panel = ~30% width (min 24 cols) for the node list; right panel = remainder for run metadata and (future) node detail tabs.
- On terminals narrower than 80 cols, render a single-column node list only. The right panel is deferred to downstream tickets.

### Header chrome (consistent with existing views)
- Title pattern (from `RunsView.View()` and `LiveChatView.renderHeader()`): `"SMITHERS › Runs › <runID-truncated>"` left-aligned, `"[Esc] Back"` right-aligned, gap filled with spaces.
- Sub-header pattern (from `LiveChatView.renderSubHeader()`): faint pipe-separated metadata items: workflow name, status, elapsed time.
- Divider: `strings.Repeat("─", v.width)` with faint styling.

### Node list rendering
- Each node row: cursor indicator (2 cols) + state glyph (2 cols) + NodeID or Label (flex) + elapsed time since `UpdatedAtMs` (8 cols).
- Selected row: bold or highlighted background (lipgloss reverse video: `lipgloss.NewStyle().Reverse(true)`).
- Section header for active nodes vs. completed nodes (optional, deferred to downstream tickets).

### Scroll management for node list
- For runs with many nodes (20+), the node list needs scroll. Follow the `LiveChatView` pattern: `scrollLine int`, `visibleHeight() int`, clamp on render.
- For the base ticket, a simple clamped cursor (no scroll) is acceptable since most runs have < 20 nodes.

---

## Navigation and Keybinding Gaps

### From RunsView to RunInspectView
- `RunsView.Update()` has a `case key.Matches(msg, key.NewBinding(key.WithKeys("enter")))` handler that is currently a no-op. The implementation adds: `return v, v.pushInspect()` where `pushInspect` creates a `RunInspectView` for the selected run and emits a `PushViewMsg` or directly calls `viewRouter.Push`.
- The router's `PushView` method is called from `ui.go`, not from view code. Views return `PopViewMsg` to pop themselves but cannot directly push siblings. To push a child, the view must return a message that `ui.go` handles — or a `tea.Cmd` that sends a `PushViewMsg`-style message.
- Existing pattern: `return v, func() tea.Msg { return PopViewMsg{} }`. A parallel `PushViewMsg{View: v}` pattern does not yet exist in the codebase. The inspector can be pushed from `ui.go`'s `Update` handler by adding a new `OpenRunInspectMsg{RunID: string}` message type, analogous to how `ActionOpenRunsView` is handled.

### Within RunInspectView
- `↑`/`k` and `↓`/`j`: navigate node list.
- `c`: push `LiveChatView` for the selected node (calls `NewLiveChatView(client, runID, nodeID, agentName)`).
- `h`: push hijack for the selected node (deferred to downstream hijack ticket).
- `r`: re-fetch inspection data.
- `Esc`/`q`: pop back to `RunsView`.

### LiveChatView integration
- `NewLiveChatView` at `livechat.go:82` accepts `(client, runID, taskID, agentName string)`. The `taskID` parameter filters the chat to a single node.
- For the inspector: pressing `c` on a node passes `selectedTask.NodeID` as `taskID`.
- `LiveChatView` is already fully implemented and uses `PopViewMsg` to return to the caller — no changes needed in that view.

---

## Test Infrastructure

### VHS tapes
- Existing tapes live in `tests/vhs/` with `Output`, `Set`, `Type`, `Enter`, `Ctrl+R`, `Sleep`, `Screenshot`, `Down`, `Up`, `Escape` syntax.
- Fixture config at `tests/vhs/fixtures/crush.json` loaded via `CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures`.
- A new tape `tests/vhs/runs-inspect-summary.tape` should: launch → Ctrl+R → navigate to a run → Enter → verify inspector header → Esc → return to runs list.

### E2E test harness
- The `eng-smithers-client-runs` plan established a PTY test harness pattern at `tests/tui/`. The `runs-dashboard` E2E test (`tests/tui/runs_dashboard_e2e_test.go`) provides a model for the inspector test.
- The inspector E2E test requires a mock server that responds to both `GET /v1/runs` (for RunsView) and `GET /v1/runs/:id` + node data (for RunInspectView).
- `smithers.InspectRun` uses `GetRunSummary` (HTTP) + `getRunTasks` (SQLite or exec). For the E2E test, the mock server only needs to serve `GET /v1/runs/:id`. Node tasks can come from a mock exec or from a pre-seeded in-memory SQLite.

### Unit tests
- `RunInspectView` rendering can be unit-tested without a PTY by calling `v.View()` directly after constructing the view with fixture `RunInspection` data.
- Test cases: loading state, error state, empty tasks, single-task run, multi-task run (cursor at different positions).

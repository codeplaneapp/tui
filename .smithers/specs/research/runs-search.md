Research Report: runs-search

## Existing Crush Surface

### RunsView (`internal/ui/views/runs.go`)
- Implements `View` interface with `Init`, `Update`, `View`, `Name`, `ShortHelp`, `SetSize`.
- Holds `runs []smithers.RunSummary` (the full fetched set) and `statusFilter smithers.RunStatus` (server-side query param AND client-side post-filter applied in `visibleRuns()`).
- `visibleRuns()` already performs a client-side pass over `v.runs` — the text search can slot in as a second predicate in this function without any structural change.
- The existing `f`/`F` key cycle triggers `loadRunsCmd()` (re-fetches from server). Text search does NOT trigger a new network call; it filters the in-memory slice.
- `cursor` is reset to 0 on every filter change (in `cycleFilter` and `clearFilter`). The same reset pattern applies to search query changes.
- SSE updates patch `v.runs` in-place via `applyRunEvent` and `runsEnrichRunMsg` — `visibleRuns()` is called fresh on each `View()` invocation, so live updates work for free.

### RunTable (`internal/ui/components/runtable.go`)
- Stateless renderer. Accepts `Runs []smithers.RunSummary` and `Cursor int` (navigable-row index).
- `partitionRuns` builds the virtual row list for section rendering. Because the table is stateless and `RunsView` passes `v.visibleRuns()` as the `Runs` field, no changes to `RunTable` are needed — it already receives a pre-filtered slice.
- Column structure: ID (8ch), Workflow (fills available width), Status (18ch), optional Nodes (7ch) and Time (9ch) at ≥80 columns.

### RunFilter (`internal/smithers/types_runs.go`)
- `RunFilter` has two fields: `Limit int` and `Status string`.
- There is no `Query` or `Search` field. The server-side API path (`/v1/runs?limit=N&status=S`) also has no text-search parameter.
- Conclusion: text search is purely client-side. No changes to `RunFilter`, `ListRuns`, or the three transport tiers (HTTP, SQLite, exec) are needed for this ticket.

### textinput component (`charm.land/bubbles/v2/textinput`)
- Already used in `internal/ui/dialog/commands.go` (live command palette filter) and `internal/ui/dialog/arguments.go` (argument fields).
- Pattern in `commands.go`: `c.input = textinput.New()`, `c.input.SetVirtualCursor(false)`, `c.input.Placeholder = "..."`, `c.input.Focus()`, then in the Update handler: `c.input, cmd = c.input.Update(msg)` + `value := c.input.Value()`.
- `textinput.Model` is a struct value, not a pointer; it must be stored by value on `RunsView` and reassigned after each `Update` call.
- `SetVirtualCursor(false)` is the standard call in this codebase to disable the virtual cursor that bubbles draws, in favour of the terminal's real cursor position managed by Bubble Tea's renderer.

### Key binding patterns (`internal/ui/views/runs.go`)
- All key checks use `key.Matches(msg, key.NewBinding(key.WithKeys(...)))` inline — no separate keyMap struct in RunsView.
- The `/` character is not bound anywhere in `runs.go`; it is safe to claim.
- `Esc` currently has only one action: pop the view. Adding a two-level Esc (first clears query, second pops) requires changing the `esc` case to inspect `searchQuery` first.
- `j`/`k`/`up`/`down` and `enter` must not be forwarded to `searchInput` when `searchMode == false`. A `searchMode` boolean gate at the top of the key handler block is the correct pattern (mirrors how `commands.go` routes keys only to the active component).

## Gaps

### No text search field on `RunFilter`
The `RunFilter` struct and all three backend tiers (`sqliteListRuns`, HTTP, exec) do not support a text search parameter. Any text search must happen client-side against `v.runs`. This is consistent with the ticket scope ("live filtering as you type") — server round-trips would introduce latency that makes live filtering feel sluggish.

### No search state on `RunsView`
`RunsView` has no `searchMode`, `searchQuery`, or `searchInput` field today. All three must be added.

### `visibleRuns()` applies only status filter
The function contains a single predicate (`r.Status == v.statusFilter`). The text predicate must be added as a second pass.

### `ShortHelp()` does not include a search hint
The `f` filter key is also absent from `ShortHelp()` in the current file (it is an undiscoverable feature). The search hint and the filter hint should both be present.

## Recommended Direction

1. **Add three fields to `RunsView`**: `searchMode bool`, `searchQuery string`, `searchInput textinput.Model`. Initialize `searchInput` in `NewRunsView` following the `commands.go` pattern.

2. **Extend `visibleRuns()`**: Apply the status predicate first (unchanged), then apply the text predicate (case-insensitive `strings.Contains` on `RunID` and `WorkflowName`/`WorkflowPath`). This is a pure in-memory filter with O(n) cost and no allocations when the query is empty.

3. **Update key handler in `Update()`**: Gate on `searchMode`. When true, forward all keys except `Esc` and `Enter` to `searchInput.Update()`, updating `searchQuery` from the input value after each keystroke and resetting `cursor = 0`. `Esc` clears the query and exits search mode. `Enter` commits the query and exits search mode while keeping the filter active. When false, bind `/` to enter search mode.

4. **Update `View()`**: Render a one-line search bar between the header and the run table whenever `searchMode || searchQuery != ""`. Use faint styling when committed (not focused). Replace the "No runs found" message with a query-aware message.

5. **Update `ShortHelp()`**: Add `"/"` → `"search"` binding.

6. **No changes to `RunTable`, `RunFilter`, or any backend code.**

## Files To Touch
- `internal/ui/views/runs.go`
- `internal/ui/views/runs_test.go`
- `tests/vhs/runs-search.tape`

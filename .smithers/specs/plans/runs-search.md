## Goal
Add live text search to the Runs Dashboard so that pressing `/` activates an
inline search bar, typing narrows the visible run list by run ID or workflow
name in real time, and `Esc` dismisses the search and restores the full list —
all client-side with no network calls or changes to the backend filter API.

## Steps

1. **Add search state to `RunsView`** (`internal/ui/views/runs.go`).
   Add three fields: `searchMode bool`, `searchQuery string`,
   `searchInput textinput.Model`. Initialize `searchInput` in `NewRunsView`
   using `textinput.New()`, `SetVirtualCursor(false)`, and a placeholder string
   of `"filter by id or workflow…"`. Import `"charm.land/bubbles/v2/textinput"`.

2. **Extend `visibleRuns()`** (`internal/ui/views/runs.go`).
   Refactor the existing status-filter pass into a `base` slice, then add a
   second pass that applies `strings.Contains(strings.ToLower(...), q)` against
   `RunID` and `WorkflowName` (falling back to `WorkflowPath`) when
   `searchQuery != ""`. Both predicates compose: status filter narrows first,
   text filter narrows further. Return the base slice unchanged when
   `searchQuery == ""` (zero allocations on the hot path).

3. **Update key handler in `Update()`** (`internal/ui/views/runs.go`).
   Gate the existing key block on `!v.searchMode`. When `searchMode == true`,
   forward all keys except `Esc` and `Enter` to `v.searchInput.Update(msg)`,
   update `v.searchQuery` from the input value, and reset `v.cursor = 0`.
   `Esc` in search mode clears the query, resets the input value, blurs the
   input, sets `searchMode = false`, and returns without a Cmd.
   `Enter` in search mode commits the query (keeps filter), blurs the input,
   sets `searchMode = false`, and returns without a Cmd.
   When `searchMode == false`, bind `/` to set `searchMode = true` and call
   `v.searchInput.Focus()`. Modify the existing `Esc` case to first check
   `v.searchQuery != ""`: if so, clear the query and return (do not pop the
   view); only pop on a second `Esc` when the query is already empty.

4. **Update `View()`** (`internal/ui/views/runs.go`).
   Insert the search bar between the header and the run table.
   When `searchMode == true`: render `"/ " + v.searchInput.View()`.
   When `searchMode == false && searchQuery != ""`: render a faint line
   `"/ <query>  (press / to edit, Esc to clear)"`.
   Replace the unconditional `"No runs found."` message with a conditional:
   when `searchQuery != ""`, show `"No runs match \"<query>\"."`.
   Pass `v.visibleRuns()` (not `v.runs`) to `components.RunTable.Runs` — this
   is already correct in the existing code; confirm it is unchanged.

5. **Update `ShortHelp()`** (`internal/ui/views/runs.go`).
   Add `key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))` to the
   returned slice, between the `enter` and `f` bindings.

6. **Add unit tests** (`internal/ui/views/runs_test.go`).
   - `TestVisibleRuns_TextFilter`: three runs, query matching two by workflow
     name and one by run ID.
   - `TestVisibleRuns_TextAndStatusFilter`: query + status filter compose
     correctly.
   - `TestRunsView_SearchActivate`: pressing `/` sets `searchMode = true`.
   - `TestRunsView_SearchEscClears`: `Esc` in search mode clears query, returns
     `nil` Cmd.
   - `TestRunsView_SearchEscPops`: `Esc` with no query returns `PopViewMsg`.

7. **Add VHS recording tape** (`tests/vhs/runs-search.tape`).
   Tape opens the TUI, navigates to the runs dashboard, presses `/`, types
   `deploy`, pauses to show the filtered list, navigates with arrow keys, clears
   with `Esc`, and exits.

## File Plan
1. `internal/ui/views/runs.go` — fields, `visibleRuns`, key handler, `View`, `ShortHelp`
2. `internal/ui/views/runs_test.go` — new search unit tests
3. `tests/vhs/runs-search.tape` — VHS recording tape

## Validation
1. Build: `go build ./...`
2. Unit tests: `go test ./internal/ui/views/ -run TestVisibleRuns -v`
3. Unit tests: `go test ./internal/ui/views/ -run TestRunsView_Search -v`
4. Full suite: `go test ./internal/ui/views/ -v` (no regressions)
5. Full suite: `go test ./internal/ui/components/ -v` (RunTable unaffected)
6. VHS: `vhs validate tests/vhs/runs-search.tape`
7. Manual smoke: open runs dashboard, press `/`, type partial run ID, verify live narrowing; press `Esc` twice (first clears, second pops).

## Open Questions
1. Should `Enter` in search mode keep the search bar visible (committed state)
   or fully hide it? The spec proposes a faint committed bar — confirm this is
   the desired UX before implementation.
2. Should the status filter label (`f` cycle indicator) and the search query
   both be visible in the header simultaneously? Currently the header has limited
   horizontal space; a secondary status line may be needed if both are present.
3. Should pressing `/` when a committed query is already active re-open the
   input with the existing value pre-populated, or start from empty?
   Pre-populating is the more useful behaviour and is the proposed approach.

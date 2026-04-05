# Engineering Spec: Run Text Search

## Metadata
- Ticket: `runs-search`
- Feature: `RUNS_SEARCH`
- Group: Runs And Inspection
- Dependencies: `runs-dashboard` (RunsView, RunTable, visibleRuns pattern must be present)
- Target files:
  - `internal/ui/views/runs.go` (modify — search state, key handling, filtered visible runs)
  - `internal/ui/components/runtable.go` (modify — accept pre-filtered runs slice, no internal filtering)
  - `internal/ui/components/runtable_test.go` (modify — add search highlight tests)
  - `internal/ui/views/runs_test.go` (modify — add search mode tests)
  - `tests/vhs/runs-search.tape` (new — VHS recording)

---

## Objective

Add a text search bar to the Runs Dashboard that activates on `/`, filters the
visible run list in real time as the user types, and dismisses on `Esc` (restoring
the full list). The filter matches against run ID and workflow name using
case-insensitive substring matching. The existing status filter (`f`/`F` keys)
and the search query compose: both constraints apply simultaneously.

---

## Scope

### In scope
- `/` key activates the search bar; focus moves to the text input
- Typing a query immediately narrows the visible run list (client-side, no network call)
- Match fields: `RunSummary.RunID` and `RunSummary.WorkflowName` (falling back to `WorkflowPath`)
- Case-insensitive substring match
- `Esc` while in search mode clears the query and returns to normal navigation mode
- Cursor resets to 0 whenever the query changes (same as cycleFilter)
- Search query composes with the existing `statusFilter`
- Help bar includes `/ search` hint when not in search mode
- Search bar renders below the header, above the run table, showing the current query
- Empty query while in search mode shows all runs (no filtering by text)

### Out of scope
- Fuzzy / scored matching (exact substring only for v1)
- Searching inside the detail/task fields (error message, node labels)
- Server-side text search via the `RunFilter` struct
- Highlight / mark matched characters in the rendered rows
- Search history or persistent query

---

## State model

Add three fields to `RunsView`:

```go
// RunsView additions
searchMode  bool           // true while '/' is active and input has focus
searchQuery string         // the current text query; empty == no filter
searchInput textinput.Model // bubbles textinput; rendered in search mode
```

`searchMode` governs whether key events are routed to `searchInput.Update()`
or to the list navigation block. It also controls whether the search bar is
rendered.

---

## Implementation plan

### Slice 1: Import and field additions

**File**: `internal/ui/views/runs.go`

Add import:

```go
"charm.land/bubbles/v2/textinput"
```

Add to `RunsView` struct after `statusFilter`:

```go
searchMode  bool
searchQuery string
searchInput textinput.Model
```

Initialize in `NewRunsView`:

```go
ti := textinput.New()
ti.SetVirtualCursor(false)
ti.Placeholder = "filter by id or workflow…"
return &RunsView{
    client:      client,
    loading:     true,
    searchInput: ti,
}
```

**Verification**: `go build ./internal/ui/views/...` passes.

---

### Slice 2: visibleRuns — apply text filter

**File**: `internal/ui/views/runs.go`

Extend `visibleRuns()` to apply `searchQuery` after the status filter. The
status filter already narrows `v.runs` to a candidate set; the text filter
then narrows further.

```go
// visibleRuns returns the subset of v.runs that satisfy both the active
// statusFilter and the active searchQuery (if any).
func (v *RunsView) visibleRuns() []smithers.RunSummary {
    // Apply status filter first (existing logic, unchanged).
    base := v.runs
    if v.statusFilter != "" {
        filtered := make([]smithers.RunSummary, 0, len(v.runs))
        for _, r := range v.runs {
            if r.Status == v.statusFilter {
                filtered = append(filtered, r)
            }
        }
        base = filtered
    }
    // Apply text search filter.
    if v.searchQuery == "" {
        return base
    }
    q := strings.ToLower(v.searchQuery)
    out := make([]smithers.RunSummary, 0, len(base))
    for _, r := range base {
        name := r.WorkflowName
        if name == "" {
            name = r.WorkflowPath
        }
        if strings.Contains(strings.ToLower(r.RunID), q) ||
            strings.Contains(strings.ToLower(name), q) {
            out = append(out, r)
        }
    }
    return out
}
```

The existing `statusFilter` path is refactored into the new function body;
the observable contract of `visibleRuns()` is unchanged from callers'
perspectives.

**Verification**: Unit test `TestVisibleRuns_TextFilter` (see Slice 5).

---

### Slice 3: Key handling in Update()

**File**: `internal/ui/views/runs.go`

The `tea.KeyPressMsg` block requires two modes:

**Search mode** (`v.searchMode == true`): all key events except `Esc` and
`enter` are forwarded to `searchInput.Update()`. On `Esc`, exit search mode
and clear the query. On `enter`, exit search mode but keep the query active
(the list remains filtered).

**Normal mode** (`v.searchMode == false`): add a `/` binding that activates
search mode, focuses `searchInput`, and resets the cursor.

```go
case tea.KeyPressMsg:
    if v.searchMode {
        switch {
        case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
            v.searchMode = false
            v.searchQuery = ""
            v.searchInput.SetValue("")
            v.searchInput.Blur()
            v.cursor = 0
        case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
            // Commit the query; leave search mode but keep the filter.
            v.searchMode = false
            v.searchInput.Blur()
        default:
            var cmd tea.Cmd
            v.searchInput, cmd = v.searchInput.Update(msg)
            v.searchQuery = v.searchInput.Value()
            v.cursor = 0
            return v, cmd
        }
        return v, nil
    }

    // Normal mode key handling.
    switch {
    case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
        v.searchMode = true
        v.searchInput.Focus()
        v.cursor = 0

    case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
        // If a query is active, first clear it; second Esc pops the view.
        if v.searchQuery != "" {
            v.searchQuery = ""
            v.searchInput.SetValue("")
            v.cursor = 0
            return v, nil
        }
        if v.cancel != nil {
            v.cancel()
        }
        if v.pollTicker != nil {
            v.pollTicker.Stop()
        }
        return v, func() tea.Msg { return PopViewMsg{} }

    // … existing up/down/f/F/r/enter cases unchanged …
    }
```

The double-`Esc` behaviour (first clears query, second pops view) ensures
that a user who searches and then wants to exit does not accidentally pop the
view while text is still in the box.

**Verification**: Unit tests `TestRunsView_SearchActivate`,
`TestRunsView_SearchEscClears`, `TestRunsView_SearchEscPops` (see Slice 5).

---

### Slice 4: View() rendering

**File**: `internal/ui/views/runs.go`

Insert the search bar between the header line and the run table. The search
bar is always rendered when `searchMode == true` or `searchQuery != ""`.

```go
// After the existing headerLine render, before loading/error/empty checks:
if v.searchMode || v.searchQuery != "" {
    prefix := lipgloss.NewStyle().Faint(true).Render("/ ")
    bar := prefix + v.searchInput.View()
    b.WriteString(bar)
    b.WriteString("\n\n")
}
```

When `searchMode == false` but `searchQuery != ""` (committed query), render
the bar with faint styling to indicate the filter is active but not focused:

```go
if v.searchQuery != "" && !v.searchMode {
    faint := lipgloss.NewStyle().Faint(true)
    b.WriteString(faint.Render("/ " + v.searchQuery + "  (press / to edit, Esc to clear)"))
    b.WriteString("\n\n")
} else if v.searchMode {
    prefix := lipgloss.NewStyle().Faint(true).Render("/ ")
    b.WriteString(prefix + v.searchInput.View())
    b.WriteString("\n\n")
}
```

Update the empty-state message to distinguish "no runs at all" from "no
runs match the current search":

```go
visible := v.visibleRuns()
if len(visible) == 0 {
    if v.searchQuery != "" {
        b.WriteString("  No runs match \"" + v.searchQuery + "\".\n")
    } else {
        b.WriteString("  No runs found.\n")
    }
    return b.String()
}
table := components.RunTable{
    Runs:   visible,
    Cursor: v.cursor,
    Width:  v.width,
}
b.WriteString(table.View())
```

**Verification**: Visual inspection in VHS recording; unit test for empty-state
message.

---

### Slice 5: Unit tests

**File**: `internal/ui/views/runs_test.go`

```go
// TestVisibleRuns_TextFilter verifies that searchQuery narrows the list
// to runs whose RunID or WorkflowName contain the query (case-insensitive).
func TestVisibleRuns_TextFilter(t *testing.T) {
    v := NewRunsView(nil)
    v.runs = []smithers.RunSummary{
        {RunID: "abc123", WorkflowName: "deploy-prod"},
        {RunID: "def456", WorkflowName: "build-staging"},
        {RunID: "ghi789", WorkflowName: "Deploy-Dev"},
    }
    v.searchQuery = "deploy"
    got := v.visibleRuns()
    if len(got) != 2 {
        t.Fatalf("expected 2 runs, got %d", len(got))
    }
    // Verify RunID filter as well.
    v.searchQuery = "def"
    got = v.visibleRuns()
    if len(got) != 1 || got[0].RunID != "def456" {
        t.Fatalf("expected def456, got %v", got)
    }
}

// TestVisibleRuns_TextAndStatusFilter verifies that text search composes
// with statusFilter.
func TestVisibleRuns_TextAndStatusFilter(t *testing.T) {
    v := NewRunsView(nil)
    v.runs = []smithers.RunSummary{
        {RunID: "r1", WorkflowName: "deploy", Status: smithers.RunStatusRunning},
        {RunID: "r2", WorkflowName: "deploy", Status: smithers.RunStatusFailed},
        {RunID: "r3", WorkflowName: "build",  Status: smithers.RunStatusRunning},
    }
    v.statusFilter = smithers.RunStatusRunning
    v.searchQuery = "deploy"
    got := v.visibleRuns()
    if len(got) != 1 || got[0].RunID != "r1" {
        t.Fatalf("expected r1 only, got %v", got)
    }
}

// TestRunsView_SearchActivate verifies that pressing '/' enters search mode.
func TestRunsView_SearchActivate(t *testing.T) {
    v := NewRunsView(nil)
    v.runs = []smithers.RunSummary{{RunID: "r1", WorkflowName: "wf"}}
    _, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyRunes, Text: "/"})
    if !v.searchMode {
        t.Fatal("expected searchMode to be true after '/' press")
    }
}

// TestRunsView_SearchEscClears verifies that Esc in search mode clears the
// query but does not pop the view.
func TestRunsView_SearchEscClears(t *testing.T) {
    v := NewRunsView(nil)
    v.searchMode = true
    v.searchQuery = "foo"
    v.searchInput.SetValue("foo")
    _, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
    if v.searchMode {
        t.Fatal("expected searchMode to be false after Esc")
    }
    if v.searchQuery != "" {
        t.Fatalf("expected empty searchQuery, got %q", v.searchQuery)
    }
    if cmd != nil {
        t.Fatal("expected no cmd from Esc in search mode")
    }
}

// TestRunsView_SearchEscPops verifies that Esc with no active query pops the view.
func TestRunsView_SearchEscPops(t *testing.T) {
    v := NewRunsView(nil)
    v.ctx, v.cancel = context.WithCancel(context.Background())
    var got tea.Msg
    _, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
    if cmd != nil {
        got = cmd()
    }
    if _, ok := got.(PopViewMsg); !ok {
        t.Fatalf("expected PopViewMsg, got %T", got)
    }
}
```

**Verification**: `go test ./internal/ui/views/ -run TestVisibleRuns -v` and
`go test ./internal/ui/views/ -run TestRunsView_Search -v` all pass.

---

### Slice 6: Help bar update

**File**: `internal/ui/views/runs.go`

Extend `ShortHelp()` to include the search hint:

```go
func (v *RunsView) ShortHelp() []key.Binding {
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑↓/jk", "navigate")),
        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter status")),
        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
    }
}
```

---

### Slice 7: VHS recording tape

**File**: `tests/vhs/runs-search.tape`

```tape
# runs-search.tape
# Records the search/filter feature on the runs dashboard.
Output tests/vhs/output/runs-search.gif
Set FontSize 14
Set Width 130
Set Height 40
Set Shell "bash"
Set Env CRUSH_GLOBAL_CONFIG tests/vhs/fixtures
Set Env SMITHERS_MOCK_RUNS "1"

Type "go run . --config tests/vhs/fixtures/crush.json"
Enter
Sleep 3s

# Open runs dashboard
Ctrl+R
Sleep 2s

# Activate search
Type "/"
Sleep 400ms

# Type a partial workflow name
Type "deploy"
Sleep 600ms

# List narrows to matching runs
Sleep 1s

# Navigate within filtered results
Down
Sleep 400ms
Up
Sleep 400ms

# Clear search with Esc
Escape
Sleep 600ms

# Full list restored
Sleep 1s

# Exit
Escape
Sleep 1s
```

---

## Validation

### Automated checks

| Check | Command | What it verifies |
|---|---|---|
| Build | `go build ./...` | All modifications compile without errors |
| Unit: text filter | `go test ./internal/ui/views/ -run TestVisibleRuns -v` | Substring filter with and without status composition |
| Unit: key handling | `go test ./internal/ui/views/ -run TestRunsView_Search -v` | Activate, clear, pop semantics |
| Existing suite | `go test ./internal/ui/views/ -v` | No regressions in RunsView navigation |
| Existing components | `go test ./internal/ui/components/ -v` | RunTable unaffected |
| VHS validate | `vhs validate tests/vhs/runs-search.tape` | Tape parses cleanly |

### Manual verification

1. Open runs dashboard (`Ctrl+R`).
2. Press `/` — confirm cursor is in the search bar, placeholder text appears.
3. Type `dep` — only runs whose ID or workflow name contains "dep" remain visible.
4. Continue typing — list narrows further.
5. Press `Backspace` — list expands back.
6. Press `Enter` — search bar shows committed query (faint), list stays filtered.
7. Press `/` again — re-enters edit mode with existing query.
8. Press `Esc` — query cleared, full list restored; one more `Esc` pops the view.
9. Combine with `f` status filter — verify both constraints apply simultaneously.

### Acceptance criteria mapping

| Criterion | Verification |
|---|---|
| Key shortcut to focus search | Manual: `/` activates input; unit: `TestRunsView_SearchActivate` |
| Typing dynamically filters the list | Unit: `TestVisibleRuns_TextFilter`; manual: visual |

---

## Risks

### 1. Cursor goes out of bounds when search narrows the list

**Impact**: If `cursor = 5` and the search narrows the list to 2 runs, the
cursor index is stale. `visibleRuns()` returns a shorter slice; `RunTable`
receives `Cursor = 5` but only has 2 rows.

**Mitigation**: Reset `cursor = 0` every time `searchQuery` changes (in the
`default` branch of the search-mode key handler). The cursor clamping in the
`down` key handler already uses `len(v.visibleRuns())-1` as the upper bound,
which self-corrects on the next navigation keystroke.

**Severity**: Low — cursor reset on every keystroke is safe and predictable.

### 2. searchInput receives events during normal navigation

**Impact**: If `searchMode == false`, keys like `j`/`k`/`f` must not be
forwarded to `searchInput.Update()`. The guard `if v.searchMode` at the top of
the key handler block ensures this.

**Mitigation**: Enforced by the `searchMode` gate; `searchInput` is blurred
when `searchMode == false`, so it will not accept focus-dependent events even
if accidentally called.

**Severity**: Low — architecture guards are in place.

### 3. SSE updates arrive while search is active

**Impact**: New runs delivered via `runsEnrichRunMsg` or `smithers.RunEventMsg`
patch `v.runs` directly. The search applies at render time via `visibleRuns()`,
so new runs will appear in filtered results automatically if they match the
query — no special handling required.

**Mitigation**: No action needed; `visibleRuns()` is called fresh on every
`View()` invocation.

**Severity**: None — the architecture handles this correctly by default.

### 4. `RunFilter.Status` server-side filter and client-side search composition

**Impact**: The `loadRunsCmd` passes `statusFilter` to the server via
`RunFilter.Status`, which means the server only returns runs matching the
current status. If the user first sets a text search and then cycles the
status filter, `loadRunsCmd` re-fetches with the new status — the in-memory
`v.runs` will change. The search query applies to whatever is in `v.runs`,
not the full universe.

**Mitigation**: This is consistent with the existing filter behaviour (the
`f` key already triggers a re-fetch). Document in comments that `searchQuery`
is a client-side post-filter on the server-fetched `v.runs`. No change needed.

**Severity**: Low — expected composition behaviour, not a bug.

---

## Files To Touch

- `/Users/williamcory/crush/internal/ui/views/runs.go` — add `searchMode`, `searchQuery`, `searchInput`; extend `visibleRuns()`; update key handler; update `View()`; update `ShortHelp()`
- `/Users/williamcory/crush/internal/ui/views/runs_test.go` — add text filter and key-mode unit tests
- `/Users/williamcory/crush/tests/vhs/runs-search.tape` — new VHS recording tape

# Implementation Plan: workflows-discovery-from-project

## Goal

Implement the Workflows List view — the foundation of the Workflows feature group. The view discovers workflows from `.smithers/workflows/` via the existing `ListWorkflows` client method, displays them in a selectable list with sourceType grouping and metadata, registers the `/workflows` route in the view registry, and wires up the `Enter` key to display a detail pane (the launch point for the downstream `workflows-dynamic-input-forms` ticket). No new client code is needed — `ListWorkflows` is fully implemented and tested in `internal/smithers/workflows.go`.

---

## Research

### What `ListWorkflows` already provides

`internal/smithers/workflows.go` implements the full dual-route discovery pattern used by every other Smithers client method:

1. **HTTP path**: `GET /api/workspaces/{workspaceId}/workflows` — returns `[]Workflow` with `ID`, `Name`, `RelativePath`, `Status`, and optional `UpdatedAt`.
2. **Exec fallback**: `smithers workflow list --format json` — parses `DiscoveredWorkflow` records (with `smithers-display-name:` and `smithers-source:` header comments) and adapts them into `[]Workflow` via `adaptDiscoveredWorkflows`.

The exec path is what runs against the actual `.smithers/workflows/` directory when the daemon is not running. The 15 `.tsx` files in `.smithers/workflows/` each carry a `// smithers-source: seeded` (or `user`/`generated`) and `// smithers-display-name: <Name>` header. The CLI reads these to populate `DisplayName` and `SourceType`.

`DiscoveredWorkflow` fields after adaptation into `Workflow`:

| Field | Source | Example |
|-------|--------|---------|
| `ID` | Filename slug | `implement` |
| `Name` | `smithers-display-name:` comment | `Implement` |
| `RelativePath` | Absolute path to `.tsx` | `/…/workflows/implement.tsx` |
| `Status` | Always `WorkflowStatusActive` from exec | `active` |

`SourceType` (`seeded`, `user`, `generated`) is available in `DiscoveredWorkflow` but is lost in the `adaptDiscoveredWorkflows` mapping — the `Workflow` struct has no `SourceType` field. The view must work with what `Workflow` gives it. See Open Questions §1.

### Actual workflows in `.smithers/workflows/`

15 workflows discovered from the project's own directory:

```
audit            grill-me         ralph
debug            implement        research
feature-enum     improve-test-coverage   review
plan             specs            ticket-create
ticket-implement ticket-kanban    tickets-create
test-first       write-a-prd
```

All carry `smithers-source: seeded` and a `smithers-display-name:` header. The `Name` field from the client will be the display name (e.g., `"Implement"`, `"Write a PRD"`).

### Existing view patterns

Every view follows the same Bubble Tea lifecycle established in `runs.go`, `tickets.go`, `agents.go`:

- `*View` struct with `client`, `cursor`, `width`, `height`, `loading bool`, `err error`.
- `loadedMsg` / `errorMsg` message types for async init.
- `Init()` fires a single goroutine via `func() tea.Msg`.
- `Update()` handles `loadedMsg`, `errorMsg`, `tea.WindowSizeMsg`, and `tea.KeyPressMsg`.
- `View()` renders header + body + help bar.
- `SetSize(w, h int)` satisfies the `View` interface.
- `Name() string` returns the route name.
- `ShortHelp() []key.Binding` returns contextual bindings.
- `Esc` / `alt+esc` returns `PopViewMsg{}`.
- `r` key reloads by setting `loading = true` and returning `v.Init()`.
- Compile-time interface check: `var _ View = (*WorkflowsView)(nil)`.

The view is registered in `DefaultRegistry()` in `internal/ui/views/registry.go`.

### Detail pane pattern

`agents.go` already implements a split detail pane for wide terminals (`width >= 100`) using `lipgloss.JoinHorizontal`. The workflows view will follow the same pattern: list on the left, metadata on the right.

### Design spec (from 02-DESIGN.md §3.11 and PRD §6.7)

The design doc's §3.11 covers the *Workflow Executor* (the run-configuration screen), but the workflow list itself is the prerequisite. From the PRD:

> **List workflows**: Show discovered workflows from `.smithers/workflows/`.

From the design doc's view hierarchy:
> `├── Workflow List & Executor`

The list must show: workflow name, relative path, status, and be navigable. `r` opens the run form (downstream ticket). `Enter` shows detail. `Esc` returns to chat.

---

## Steps

### Step 1 — Create `WorkflowsView` struct and lifecycle

**File**: `internal/ui/views/workflows.go` (new file)

```go
package views

// Compile-time interface check.
var _ View = (*WorkflowsView)(nil)

type workflowsLoadedMsg struct {
    workflows []smithers.Workflow
}

type workflowsErrorMsg struct {
    err error
}

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
}

func NewWorkflowsView(client *smithers.Client) *WorkflowsView {
    return &WorkflowsView{
        client:  client,
        loading: true,
    }
}

func (v *WorkflowsView) Init() tea.Cmd {
    return func() tea.Msg {
        workflows, err := v.client.ListWorkflows(context.Background())
        if err != nil {
            return workflowsErrorMsg{err: err}
        }
        return workflowsLoadedMsg{workflows: workflows}
    }
}

func (v *WorkflowsView) Name() string { return "workflows" }

func (v *WorkflowsView) SetSize(w, h int) {
    v.width = w
    v.height = h
}
```

### Step 2 — Update handler

**File**: `internal/ui/views/workflows.go`

```go
func (v *WorkflowsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case workflowsLoadedMsg:
        v.workflows = msg.workflows
        v.loading = false
        return v, nil

    case workflowsErrorMsg:
        v.err = msg.err
        v.loading = false
        return v, nil

    case tea.WindowSizeMsg:
        v.width = msg.Width
        v.height = msg.Height
        return v, nil

    case tea.KeyPressMsg:
        switch {
        case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
            return v, func() tea.Msg { return PopViewMsg{} }

        case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
            if v.cursor > 0 {
                v.cursor--
                v.clampScroll()
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
            if v.cursor < len(v.workflows)-1 {
                v.cursor++
                v.clampScroll()
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
            if !v.loading {
                v.loading = true
                return v, v.Init()
            }
        }
    }
    return v, nil
}
```

`clampScroll()` adjusts `scrollOffset` to keep the cursor visible, matching the pattern used in `tickets.go`.

### Step 3 — View renderer

**File**: `internal/ui/views/workflows.go`

#### 3a — List pane content

The list renders each workflow as two lines:

```
▸ Implement                                   active
  .smithers/workflows/implement.tsx
```

Cursor line uses bright cyan for the name (matching the brand color scheme from the design doc). Non-cursor entries use the default style.

For wide terminals (`v.width >= 100`), split using `lipgloss.JoinHorizontal` with a 38-column left pane. The right pane shows:

```
implement
──────────────────────────────────
Name:    Implement
Path:    .smithers/workflows/implement.tsx
Status:  active

[r] Run workflow
```

For narrow terminals, a single-column layout is used.

#### 3b — Empty/loading/error states

- Loading: `" Loading workflows..."` (spinner via lipgloss faint).
- Error: `" Error: <message>"` with instructions to check that `smithers` is on PATH.
- Empty: `" No workflows found in .smithers/workflows/\n Run: smithers workflow list"`.

#### 3c — Header and help bar

```
SMITHERS › Workflows                                      [Esc] Back
```

Help bar bindings:

| Key | Action |
|-----|--------|
| `↑`/`k`, `↓`/`j` | Navigate |
| `r` | Refresh |
| `Esc` | Back to chat |

`ShortHelp()` returns these bindings in the standard `[]key.Binding` form.

### Step 4 — Register the route

**File**: `internal/ui/views/registry.go`

Add to `DefaultRegistry()`:

```go
r.Register("workflows", func(c *smithers.Client) View { return NewWorkflowsView(c) })
```

No other wiring is needed — the command palette already resolves route names from the registry, and the router pushes views by name.

### Step 5 — Unit tests for the view

**File**: `internal/ui/views/workflows_test.go` (new file)

Test the Bubble Tea model lifecycle:

- `TestWorkflowsView_Init_SetsLoading`: `NewWorkflowsView(client)` → `loading == true`, `Init()` returns a non-nil `tea.Cmd`.
- `TestWorkflowsView_LoadedMsg_PopulatesWorkflows`: send `workflowsLoadedMsg{workflows: testWorkflows}` → `loading == false`, `workflows` has expected length.
- `TestWorkflowsView_ErrorMsg_SetsErr`: send `workflowsErrorMsg{err: someErr}` → `loading == false`, `err != nil`.
- `TestWorkflowsView_CursorNavigation`: down/up key presses move cursor within bounds; cursor does not go negative or past last workflow.
- `TestWorkflowsView_CursorBoundary_AtBottom`: `down` at last item → cursor stays at last index.
- `TestWorkflowsView_CursorBoundary_AtTop`: `up` at index 0 → cursor stays at 0.
- `TestWorkflowsView_Esc_ReturnsPopViewMsg`: `Esc` key → returned `tea.Cmd` produces `PopViewMsg{}` when invoked.
- `TestWorkflowsView_Refresh_ReloadsWorkflows`: `r` key press → `loading == true` and `Init()` command is returned.
- `TestWorkflowsView_View_HeaderText`: `View()` output contains `"SMITHERS › Workflows"`.
- `TestWorkflowsView_View_ShowsWorkflowNames`: `View()` output contains loaded workflow names.
- `TestWorkflowsView_View_EmptyState`: no workflows → output contains `"No workflows found"`.
- `TestWorkflowsView_View_LoadingState`: `loading == true` → output contains `"Loading"`.
- `TestWorkflowsView_View_ErrorState`: `err != nil` → output contains `"Error"`.
- `TestWorkflowsView_Name`: `Name()` returns `"workflows"`.
- `TestWorkflowsView_SetSize`: `SetSize(120, 40)` sets `v.width == 120`, `v.height == 40`.

Test helpers reuse the `newExecClient` / `newWorkspaceTestServer` pattern from `internal/smithers/workflows_test.go`.

### Step 6 — E2E terminal test

**File**: `tests/tui/workflows_e2e_test.go` (new file, matches `agents_e2e_test.go` structure)

```go
func TestWorkflowsView_Navigation(t *testing.T) {
    h := NewTUIHarness(t)
    defer h.Close(t)

    h.SendKeys(t, "/")
    h.WaitForText(t, "workflows", 5*time.Second)

    h.SendKeys(t, "workflows\r")
    h.WaitForText(t, "SMITHERS › Workflows", 5*time.Second)

    snap := h.Snapshot()
    assert.Contains(t, snap, "Workflows")

    // Navigate down
    h.SendKeys(t, "j")

    // Escape returns to chat
    h.SendKeys(t, "\x1b")
    h.WaitForText(t, "SMITHERS", 3*time.Second)
    assert.NotContains(t, h.Snapshot(), "SMITHERS › Workflows")
}
```

Requires the binary to be built and `SMITHERS_TEST_BINARY` set (see `feat-agents-browser.md` §Step 6 for the shared harness).

### Step 7 — VHS tape

**File**: `tests/vhs/workflows_view.tape` (new file)

```
Output tests/vhs/workflows_view.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 800

Type "go run . --no-mcp"
Enter
Sleep 2s

Ctrl+P
Sleep 500ms
Type "workflows"
Sleep 300ms
Enter
Sleep 1s

Down
Sleep 300ms
Down
Sleep 300ms
Up
Sleep 500ms

Escape
Sleep 1s
```

---

## File Plan

| File | Status | Changes |
|------|--------|---------|
| `internal/ui/views/workflows.go` | Create | Full `WorkflowsView` implementation: struct, `Init`, `Update`, `View`, `Name`, `SetSize`, `ShortHelp`, `clampScroll`, list renderer, detail pane, empty/error/loading states |
| `internal/ui/views/registry.go` | Modify | Add `r.Register("workflows", ...)` to `DefaultRegistry()` |
| `internal/ui/views/workflows_test.go` | Create | 15 unit tests covering lifecycle, navigation, key bindings, and View() output |
| `tests/tui/workflows_e2e_test.go` | Create | E2E navigation test (depends on shared harness from `feat-agents-browser.md`) |
| `tests/vhs/workflows_view.tape` | Create | VHS visual regression tape |
| `internal/smithers/workflows.go` | No change | Client is complete |
| `internal/smithers/types_workflows.go` | No change | Types are complete |

---

## Validation

### Unit tests

```bash
go test ./internal/ui/views/... -run TestWorkflowsView -v
```

### Build check

```bash
go build ./...
go vet ./internal/ui/views/...
```

### E2E terminal test

```bash
go build -o /tmp/smithers-tui .
SMITHERS_TEST_BINARY=/tmp/smithers-tui go test ./tests/tui/... -run TestWorkflowsView -timeout 30s -v
```

### VHS recording

```bash
vhs tests/vhs/workflows_view.tape
# Verify tests/vhs/workflows_view.gif is non-empty
```

### Manual smoke test

1. `go run .`
2. Press `/` or `Ctrl+P` to open command palette.
3. Type `workflows`, press `Enter`.
4. Verify `"SMITHERS › Workflows"` header appears.
5. Verify the list shows workflow names from `.smithers/workflows/` (e.g., `Implement`, `Review`, `Plan`).
6. Press `↓`/`j` — cursor moves down. Press `↑`/`k` — cursor moves up.
7. Cursor does not go out of bounds at either end.
8. Press `r` — loading spinner appears, list refreshes.
9. On a wide terminal (≥ 100 cols): verify two-column layout with detail pane on the right.
10. On a narrow terminal (< 100 cols): verify single-column list.
11. Press `Esc` — returns to chat/console view.

---

## Open Questions

1. **`SourceType` in `Workflow`**: `adaptDiscoveredWorkflows` drops `SourceType` when mapping `DiscoveredWorkflow` → `Workflow`. The `Workflow` struct has no `SourceType` field. If grouping by source type (`seeded` vs `user` vs `generated`) is desired for the list, either: (a) add `SourceType string` to `Workflow` in `types_workflows.go` and populate it in `adaptDiscoveredWorkflows`, or (b) omit grouping in this ticket and derive it solely from path heuristics. Recommendation: add `SourceType` to `Workflow` as part of this ticket — it's a small, self-contained change and avoids a second pass. Block on this decision before implementing the renderer.

2. **`runs` view as precedent for detail pane**: The `runs.go` view does not currently implement a detail pane. The `agents.go` detail pane (`width >= 100` split) is the best existing precedent. The workflows view should follow the agents pattern exactly to maintain consistency.

3. **Downstream dependency surface**: The `workflows-dynamic-input-forms` ticket (which adds the run-configuration form) depends on this view for the `r`-key entrypoint. This ticket should expose a `selectedWorkflow() *smithers.Workflow` helper method on `WorkflowsView` to make it easy for the downstream ticket to read the cursor selection when pushing the executor view.

4. **`Enter` key behavior**: This ticket's acceptance criteria only covers discovery and list display. The `Enter` key and `r` key are natural entrypoints for the downstream executor. For this ticket, `Enter` should show the detail pane (already in scope) but not push a new view — that comes in `workflows-dynamic-input-forms`. Pressing `r` should be reserved for "refresh list" (consistent with agents view) and the run-form shortcut should be a different key or handled downstream.

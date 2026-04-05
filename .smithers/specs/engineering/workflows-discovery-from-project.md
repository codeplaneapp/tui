# Engineering Spec: workflows-discovery-from-project

## Ticket Summary
- **ID**: workflows-discovery-from-project
- **Group**: Workflows
- **Feature**: WORKFLOWS_DISCOVERY_FROM_PROJECT
- **Dependencies**: eng-smithers-workflows-client (complete)
- **Goal**: Create `WorkflowsView` — the Bubble Tea view that calls `ListWorkflows`, renders the result as a navigable list, and registers the `/workflows` route so the command palette can open it.

## Acceptance Criteria
1. `client.ListWorkflows(ctx)` is called on `Init()` and populates the view.
2. Workflow metadata (`ID`, `Name`, `RelativePath`, `Status`) is correctly displayed in the list.
3. `↑`/`↓` (or `k`/`j`) navigate the cursor.
4. `r` refreshes the list.
5. `Esc` returns to the previous view (chat/console) via `PopViewMsg{}`.
6. The view is reachable via the command palette (`/workflows`).
7. Missing or unparseable workflows do not crash the view — errors are shown inline.
8. An empty list renders a helpful empty-state message.

## Interface Contract

`WorkflowsView` must satisfy `views.View`:

```go
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Name() string
    SetSize(width, height int)
    ShortHelp() []key.Binding
}
```

## Data Flow

```
WorkflowsView.Init()
  └─▶ goroutine: client.ListWorkflows(ctx)
        ├─▶ HTTP GET /api/workspaces/{id}/workflows   (daemon available)
        └─▶ exec smithers workflow list --format json  (fallback)
              └─▶ parseDiscoveredWorkflowsJSON → adaptDiscoveredWorkflows
  └─▶ workflowsLoadedMsg{workflows} | workflowsErrorMsg{err}
        └─▶ Update() sets v.workflows / v.err, clears v.loading
```

## File Inventory

| File | Action | Notes |
|------|--------|-------|
| `internal/ui/views/workflows.go` | Create | Full view implementation |
| `internal/ui/views/registry.go` | Modify | Add `"workflows"` route |
| `internal/ui/views/workflows_test.go` | Create | 15 unit tests |
| `tests/tui/workflows_e2e_test.go` | Create | Navigation E2E test |
| `tests/vhs/workflows_view.tape` | Create | VHS visual regression |

## Struct Definition

```go
// internal/ui/views/workflows.go

var _ View = (*WorkflowsView)(nil)

type workflowsLoadedMsg struct{ workflows []smithers.Workflow }
type workflowsErrorMsg  struct{ err error }

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
```

## Key Behaviours

### Cursor + Scroll

- `cursor` stays within `[0, len(workflows)-1]`.
- `clampScroll()` keeps `scrollOffset` so the cursor row is always visible.
- `pageSize()` uses `(height - headerLines) / linesPerWorkflow`, minimum 1.

### Renderer Layout

Narrow (`width < 100`) — single column:

```
SMITHERS › Workflows                                    [Esc] Back
───────────────────────────────────────────────────────────────────

▸ Implement                                              active
  .smithers/workflows/implement.tsx

  Review                                                 active
  .smithers/workflows/review.tsx
  ...

[↑/↓] Navigate   [r] Refresh   [Esc] Back
```

Wide (`width >= 100`) — two columns (list 38 chars, remainder detail):

```
SMITHERS › Workflows                                    [Esc] Back
───────────────────────────────────────────────────────────────────

▸ Implement         │ implement
  Review            │ ─────────────────────────────────────────────
  Plan              │ Name:    Implement
  ...               │ Path:    .smithers/workflows/implement.tsx
                    │ Status:  active
                    │
                    │ [r] Run workflow

[↑/↓] Navigate   [r] Refresh   [Esc] Back
```

The detail pane uses `lipgloss.JoinHorizontal` following the pattern in `agents.go`.

### States

| State | Rendered output |
|-------|-----------------|
| `loading == true` | `" Loading workflows..."` (faint) |
| `err != nil` | `" Error: <msg>\n Check that smithers is on PATH."` |
| `len(workflows) == 0` | `" No workflows found in .smithers/workflows/"` |
| Normal | List with cursor |

### Key Bindings

| Key | Behaviour |
|-----|-----------|
| `↑`, `k` | Move cursor up (clamped) |
| `↓`, `j` | Move cursor down (clamped) |
| `r` | Set `loading = true`, return `Init()` |
| `Esc`, `Alt+Esc` | Return `func() tea.Msg { return PopViewMsg{} }` |

### Registry Change

```go
// internal/ui/views/registry.go — DefaultRegistry()
r.Register("workflows", func(c *smithers.Client) View { return NewWorkflowsView(c) })
```

## Test Plan

### Unit tests (`internal/ui/views/workflows_test.go`)

1. `TestWorkflowsView_Init_SetsLoading`
2. `TestWorkflowsView_LoadedMsg_PopulatesWorkflows`
3. `TestWorkflowsView_ErrorMsg_SetsErr`
4. `TestWorkflowsView_CursorNavigation_Down`
5. `TestWorkflowsView_CursorNavigation_Up`
6. `TestWorkflowsView_CursorBoundary_AtBottom`
7. `TestWorkflowsView_CursorBoundary_AtTop`
8. `TestWorkflowsView_Esc_ReturnsPopViewMsg`
9. `TestWorkflowsView_Refresh_ReloadsWorkflows`
10. `TestWorkflowsView_View_HeaderText`
11. `TestWorkflowsView_View_ShowsWorkflowNames`
12. `TestWorkflowsView_View_EmptyState`
13. `TestWorkflowsView_View_LoadingState`
14. `TestWorkflowsView_View_ErrorState`
15. `TestWorkflowsView_Name`
16. `TestWorkflowsView_SetSize`

### E2E (`tests/tui/workflows_e2e_test.go`)

`TestWorkflowsView_Navigation` — open `/workflows` via palette, verify header, navigate, escape.

### VHS (`tests/vhs/workflows_view.tape`)

Records `workflows_view.gif` of open, navigate, escape sequence.

## Validation Commands

```bash
# Unit
go test ./internal/ui/views/... -run TestWorkflowsView -v

# Build
go build ./... && go vet ./internal/ui/views/...

# E2E (requires built binary)
go build -o /tmp/smithers-tui .
SMITHERS_TEST_BINARY=/tmp/smithers-tui go test ./tests/tui/... -run TestWorkflowsView -timeout 30s -v

# VHS
vhs tests/vhs/workflows_view.tape
```

## Open Questions

1. **`SourceType` on `Workflow`**: `adaptDiscoveredWorkflows` discards `DiscoveredWorkflow.SourceType`. If grouping (`seeded` / `user` / `generated`) is desired, add `SourceType string` to `Workflow` in `types_workflows.go` and populate it. This change is backward-compatible (zero value `""` for HTTP path). Decision needed before the renderer is built.

2. **`r` key conflict**: `r` currently means "refresh" in all other views. If the downstream executor also uses `r` to launch a run, consider reserving `Enter` for "open run form" on the executor push and keeping `r` = "refresh" on the discovery list. Confirm this key allocation with the `workflows-dynamic-input-forms` ticket.

3. **`selectedWorkflow()` helper**: Expose a `selectedWorkflow() *smithers.Workflow` method on `WorkflowsView` so the downstream executor push can read the selection cleanly without re-indexing.

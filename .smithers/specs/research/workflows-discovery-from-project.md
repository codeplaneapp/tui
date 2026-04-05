# Research: workflows-discovery-from-project

## What Already Exists

### `ListWorkflows` — Fully Implemented

`internal/smithers/workflows.go` contains the complete, tested dual-route client method:

- **HTTP path**: `GET /api/workspaces/{workspaceId}/workflows` returns `[]Workflow` directly.
- **Exec fallback**: `smithers workflow list --format json` parses `DiscoveredWorkflow` records, handles both `{ "workflows": [...] }` wrapped and bare `[...]` array output shapes, and adapts entries into `[]Workflow` via `adaptDiscoveredWorkflows`.

`internal/smithers/workflows_test.go` has 20+ passing tests covering all branches (HTTP success, HTTP server error, exec wrapped, exec bare array, exec error, no-workspace-ID fallback, server-down fallback, bearer token propagation, parse helpers, and `adaptDiscoveredWorkflows`).

There is no stub to replace and no client work to do. This is a pure view-layer ticket.

### Workflow Types

`internal/smithers/types_workflows.go` defines:

| Type | Purpose |
|------|---------|
| `Workflow` | Canonical record: `ID`, `WorkspaceID`, `Name`, `RelativePath`, `Status`, `UpdatedAt` |
| `WorkflowStatus` | Enum: `draft`, `active`, `hot`, `archived` |
| `WorkflowDefinition` | Extends `Workflow` with `Source` (raw TSX) |
| `DAGDefinition` | Launch fields for the run form |
| `WorkflowTask` | A single input field with `Key`, `Label`, `Type` |
| `DiscoveredWorkflow` | CLI-only type: `ID`, `DisplayName`, `EntryFile`, `SourceType` |

After `adaptDiscoveredWorkflows`, `Workflow.Name` contains the display name from the `// smithers-display-name:` header comment in the `.tsx` file. `SourceType` is not carried through — see Open Questions.

### Actual `.tsx` Files in `.smithers/workflows/`

16 workflow files discovered in the project's own directory:

```
audit.tsx             grill-me.tsx          ralph.tsx
debug.tsx             implement.tsx         research.tsx
feature-enum.tsx      improve-test-coverage.tsx  review.tsx
plan.tsx              specs.tsx             ticket-create.tsx
ticket-implement.tsx  ticket-kanban.tsx     tickets-create.tsx
test-first.tsx        write-a-prd.tsx
```

Each carries:
- `// smithers-source: seeded`
- `// smithers-display-name: <Human Name>`

Example from `implement.tsx`:
```tsx
// smithers-source: seeded
// smithers-display-name: Implement
```

These map to `DiscoveredWorkflow.SourceType = "seeded"` and `DisplayName = "Implement"` from the CLI. After adaptation: `Workflow.Name = "Implement"`, `Workflow.Status = WorkflowStatusActive`.

### Existing View Patterns

All Smithers views in `internal/ui/views/` follow a common lifecycle established across `runs.go`, `tickets.go`, `agents.go`, `prompts.go`:

1. Struct fields: `client`, `cursor int`, `scrollOffset int` (when needed), `width`, `height`, `loading bool`, `err error`, domain slice.
2. `loadedMsg` / `errorMsg` unexported message types.
3. `Init()` fires a single goroutine returning the loaded/error msg.
4. `Update()` dispatches on `loadedMsg`, `errorMsg`, `tea.WindowSizeMsg`, `tea.KeyPressMsg`.
5. `View()` renders: header `SMITHERS › <Name>` with `[Esc] Back`, body (loading/error/empty/list), help bar.
6. `SetSize(w, h int)` and `Name() string` satisfy the `View` interface.
7. `ShortHelp() []key.Binding` returns contextual bindings.
8. `r` refreshes by re-running `Init()`.
9. `Esc` returns `PopViewMsg{}`.
10. Compile-time interface check: `var _ View = (*XxxView)(nil)`.

### View Registry

`internal/ui/views/registry.go`:

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register("agents",    func(c *smithers.Client) View { return NewAgentsView(c) })
    r.Register("approvals", func(c *smithers.Client) View { return NewApprovalsView(c) })
    r.Register("tickets",   func(c *smithers.Client) View { return NewTicketsView(c) })
    return r
}
```

The `/workflows` route is absent. A single `r.Register("workflows", ...)` line is all that's needed to make the view reachable from the command palette.

### Detail Pane Precedent

`agents.go` implements a two-column layout when `v.width >= 100` using `lipgloss.JoinHorizontal` with a fixed 36-character left pane. The workflows view can reuse this exact approach.

### Design Spec

From PRD §6.7:
> **List workflows**: Show discovered workflows from `.smithers/workflows/`.

From Design doc §2 view hierarchy:
> `├── Workflow List & Executor`

From Design doc §3.11:
> "When selecting a workflow in the Workflow List and pressing `r` to run..."

The list is the entry point. The executor (run-configuration form) is a downstream view.

## Gap Analysis

| Item | Status |
|------|--------|
| `ListWorkflows` client method | Complete |
| `Workflow` / `DiscoveredWorkflow` types | Complete |
| `WorkflowsView` struct | Missing — file does not exist |
| `/workflows` route registration | Missing — not in `DefaultRegistry()` |
| `SourceType` field on `Workflow` | Missing — `adaptDiscoveredWorkflows` drops it |
| Unit tests for the view | Missing |
| E2E test for workflows navigation | Missing |
| VHS tape for workflows view | Missing |

No dependency on any other in-flight ticket. The `eng-smithers-workflows-client` ticket is already done.

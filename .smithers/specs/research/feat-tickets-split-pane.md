# Research: feat-tickets-split-pane

**Ticket**: `feat-tickets-split-pane`
**Feature**: `TICKETS_SPLIT_PANE_LAYOUT`
**Date**: 2026-04-05

---

## Objective

Integrate the completed `SplitPane` component into `TicketsView` so the tickets
list renders on the left and a detail/markdown-preview pane renders on the right.
Tab switches keyboard focus between panes. Ticket content loads lazily (right
pane remains empty until a ticket is selected).

---

## Dependency Status

### `eng-split-pane-component` — Complete

The `SplitPane` component was implemented on branch `worktree-agent-a76a2b3f`
(commit `ec49f53a`). The file exists at:

```
.claude/worktrees/agent-a76a2b3f/internal/ui/components/splitpane.go
```

It provides:

- `Pane` interface: `Init() tea.Cmd`, `Update(msg tea.Msg) (Pane, tea.Cmd)`,
  `View() string`, `SetSize(width, height int)`.
- `FocusSide` (`FocusLeft` / `FocusRight`) and `SplitPaneOpts` (LeftWidth,
  DividerWidth, CompactBreakpoint, FocusedBorderColor, DividerColor).
- `NewSplitPane(left, right Pane, opts SplitPaneOpts) *SplitPane`.
- `Update` intercepts `Tab` / `Shift+Tab` to toggle focus; all other messages
  route exclusively to the focused pane.
- `SetSize` distributes widths; enters compact (single-pane) mode when total
  width falls below `CompactBreakpoint` (default 80).
- Focus indicator: `lipgloss.ThickBorder()` accent on focused pane left edge;
  border consumes 1 column (inner width reduced by 1 to prevent overflow).
- `ShortHelp() []key.Binding` returns the Tab binding for help bar use.
- 17 unit tests + 2 example tests, all passing.

The component is **not yet merged to main**. The implementation ticket
(`feat-tickets-split-pane`) must either be built against the worktree branch or
wait for the component to land. See "Merge Strategy" below.

### `feat-tickets-list` — Complete (main branch)

`internal/ui/views/tickets.go` on main implements a fully functional
`TicketsView`:

- Loads tickets via `smithers.Client.ListTickets()`.
- `cursor` + `scrollOffset` with viewport clipping.
- Navigation: `↑/↓`, `j/k`, `g/G`, `Ctrl+U/D`, `PgUp/PgDn`, `r` refresh,
  `Esc` back.
- `ticketSnippet()` extracts a summary from markdown content.
- `ShortHelp()` returns key binding hints.
- 12 unit tests passing on main.

The current `TicketsView` does NOT use `SplitPane` — it is a flat single-pane
list. This ticket adds the split pane wrapper.

---

## Existing Code Landscape

### `internal/ui/views/tickets.go`

Key struct fields on `TicketsView`:

```go
type TicketsView struct {
    client       *smithers.Client
    tickets      []smithers.Ticket
    cursor       int
    scrollOffset int
    width        int
    height       int
    loading      bool
    err          error
}
```

The `View()` method does all rendering inline. Navigation key handling is
directly in `Update()`. There are no sub-pane types; everything is flat.

The `enter` key currently has a `// No-op for now; future: detail view.`
comment — this ticket activates it.

### `internal/ui/views/helpers.go`

Already exists on main with: `padRight`, `truncate`, `formatStatus`,
`formatPayload`, `wrapText`. The `ticketSnippet` and `metadataLine` helpers
remain in `tickets.go` (not yet moved to helpers). The plan below moves them.

### `internal/ui/views/approvals.go`

The `ApprovalsView` is a reference design for the refactor pattern this ticket
follows: it has a `cursor`-driven list and a detail section. However, it is
also not yet using `SplitPane` — both tickets and approvals are listed in
`platform-split-pane` as the two views to be refactored. This ticket's scope
is `TicketsView` only; `ApprovalsView` is covered by the `platform-split-pane`
ticket.

### `internal/ui/views/tickets_test.go`

12 existing tests cover: `Init`, `ticketsLoadedMsg`, error, empty list, cursor
navigation, page navigation, home/end, refresh, Esc, cursor indicator, header
count, scroll offset, `ticketSnippet`, and `metadataLine`. The refactor must
keep all these green.

---

## GUI Reference: `TicketsList.tsx`

The upstream Electrobun GUI (`../smithers/gui/src/ui/TicketsList.tsx`) shows:

- Two-column layout: `w-64` (≈32 terminal cols) sidebar left, remaining right.
- Left: scrollable ticket list; clicking a ticket updates the right panel.
- Right: ticket title + markdown body rendered via a markdown component.
- No editing in the list view itself; editing is a separate mode.
- No lazy loading — the GUI loads all tickets on mount and caches the list.

The TUI equivalent: left pane = navigable ticket list, right pane = markdown
detail of the currently focused ticket. Detail updates immediately as cursor
moves (no explicit "enter to load" step); the content is already in memory from
`ListTickets`.

---

## Lazy Content Loading Analysis

The PRD describes "lazy content loading" for the detail pane. Given the
current data model:

- `smithers.Ticket` has both `ID` and `Content` fields.
- `Client.ListTickets()` returns full `Ticket` structs including content.
- There is no separate `GetTicket(id)` API call in the current client.

**Implication**: All ticket content is available in memory after the initial
list load. "Lazy" in this context means the right pane renders nothing until
a ticket is selected (cursor ≥ 0 and `len(tickets) > 0`), not that it makes
an additional network request per ticket.

If a future API revision returns only IDs + summaries from `ListTickets` and
requires a separate `GetTicket(id)` call for full content, the `ticketDetailPane`
would issue that fetch command from its `Update` when the cursor changes. The
architecture proposed here already accommodates this: the detail pane's `Update`
can fire a `tea.Cmd` in response to a cursor-change message.

For v1, lazy = show placeholder until cursor is set; render `ticket.Content`
immediately from in-memory data once a ticket is focused.

---

## Focus Model

`SplitPane` intercepts `Tab` / `Shift+Tab` at its own `Update` level. When
focus is on the left pane, cursor navigation (`j/k`, arrows) moves the ticket
list. When focus shifts to the right pane, those same keys route to the detail
pane (which can use them for scrolling in a future iteration).

The parent `TicketsView.Update` continues to own `Esc` (pop view) and `r`
(refresh) — these are intercepted before delegation to `splitPane.Update`.

The `enter` key while left-focused moves focus to the right pane (or the
`SplitPane` Tab toggle suffices; `enter` on a ticket as a "view detail"
shortcut is additive and handled in `ticketListPane.Update` by emitting a
focus-right command).

---

## Width Budget

| Terminal width | SplitPane behavior |
|---------------|-------------------|
| >= 80 cols | Two-pane mode. Left: 30 cols + 1-col border = 31 cols consumed. Right: `width - 31` cols. |
| < 80 cols | Compact mode. Only focused pane visible (full width). Tab swaps. |

The header row rendered by `TicketsView.View()` itself occupies 2 lines
(header + blank). The split pane receives `height - 2` to fill the rest.

Left pane width 30 (matching `SplitPaneOpts` default and Crush's
`sidebarWidth`) shows ~8–10 ticket IDs without truncation for typical IDs
like `ticket-001` or `PROJ-123`. The right pane at 80-col terminal gets
`80 - 30 - 1 = 49` columns for detail rendering — sufficient for markdown.

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| `eng-split-pane-component` not yet merged to main | Build on the worktree branch, or merge the component first. The plan notes the prerequisite clearly. |
| Existing `tickets_test.go` cursor tests access `v.cursor` directly | After refactor, `cursor` lives in `ticketListPane`, not `TicketsView`. Tests must be updated to access `v.listPane.cursor`. Helper `loadedView()` must also update `listPane.tickets`. |
| `ticketSnippet` / `metadataLine` tested in `tickets_test.go` | Moving them to `helpers.go` requires updating test file package references — they stay in `package views`, so the move is transparent to tests. |
| `scrollOffset` field relied on by `TestTicketsView_ScrollOffset` | `scrollOffset` moves to `ticketListPane`; the test's assertion on `v.scrollOffset` must become `v.listPane.scrollOffset`. |
| SplitPane height excludes header rows | `TicketsView.SetSize` must pass `height - 2` (not `height`) to `splitPane.SetSize`. Passing wrong height causes blank rows or clipping. |
| Compact mode at < 80 cols hides divider and right pane | Correct and expected. `TestTicketsView_LoadedMsg` renders at `width=80` which is exactly the breakpoint. Use `width=81` in tests to guarantee two-pane mode; or set `CompactBreakpoint` slightly lower (e.g., 78) to make 80-col tests unambiguous. |

---

## Files to Read Before Implementation

1. `/Users/williamcory/crush/.claude/worktrees/agent-a76a2b3f/internal/ui/components/splitpane.go` — authoritative component source
2. `/Users/williamcory/crush/.claude/worktrees/agent-a76a2b3f/internal/ui/components/splitpane_test.go` — component tests (usage patterns)
3. `/Users/williamcory/crush/internal/ui/views/tickets.go` — current flat view (to refactor)
4. `/Users/williamcory/crush/internal/ui/views/tickets_test.go` — tests to keep green
5. `/Users/williamcory/crush/internal/ui/views/helpers.go` — shared utilities already extracted
6. `/Users/williamcory/crush/internal/ui/views/approvals.go` — reference pattern for a view with list + detail sections

---

## Merge Strategy

The `eng-split-pane-component` implementation lives in a worktree branch.
Before this ticket can land on main, one of the following must occur:

**Option A (recommended)**: Merge `eng-split-pane-component` to main first
(the component is complete with 19 passing tests), then implement this ticket
on main against the merged component.

**Option B**: Build this ticket on the same worktree branch (`worktree-agent-a76a2b3f`),
add the tickets split-pane as a second commit on that branch, and merge both together.

Option A produces a cleaner history. The implementation plan below assumes
Option A.

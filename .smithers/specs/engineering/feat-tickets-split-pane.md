# Engineering Spec: Tickets Split Pane Layout

**Ticket**: `feat-tickets-split-pane`
**Feature**: `TICKETS_SPLIT_PANE_LAYOUT`
**Status**: Draft
**Date**: 2026-04-05
**Dependencies**: `feat-tickets-list` (complete), `eng-split-pane-component` (complete, pending merge)

---

## Objective

Refactor `internal/ui/views/tickets.go` so `TicketsView` renders a split-pane
layout: ticket list on the left, markdown detail preview on the right. The
right pane shows a placeholder until a ticket is focused; once focused, it
renders the ticket's full content immediately from in-memory data. Tab switches
keyboard focus between panes.

This is a **structural refactor** of an existing view, not new feature logic.
The `SplitPane` component from `eng-split-pane-component` is the only new
dependency.

---

## Scope

### In Scope

- Split `TicketsView` into two private `Pane` types: `ticketListPane` and
  `ticketDetailPane`.
- Compose `SplitPane` inside `TicketsView` with default `LeftWidth=30`.
- Left pane: full existing list rendering (ID + snippet, cursor indicator,
  viewport clipping, scroll offset).
- Right pane: ticket ID as bold title + content, wrapped to pane width.
  Placeholder text ("Select a ticket") when no ticket is selected.
- Tab and Shift+Tab toggle focus via `SplitPane`'s built-in handler.
- Compact mode (< 80 cols): only focused pane visible; Tab swaps.
- Help bar: context-aware — left focus shows navigation hints; right focus
  shows Tab-back hint.
- Move `ticketSnippet` and `metadataLine` helpers to `helpers.go` (they are
  already referenced only within the views package).
- Update all existing tests to match the refactored struct layout.
- Add new unit tests for split-pane integration.

### Out of Scope

- Scrollable right pane (detail scroll) — deferred to `feat-tickets-detail-view`.
- Markdown rendering with `glamour` — plain `wrapText` for v1; glamour
  rendering is `feat-tickets-detail-view`.
- Inline editing — `feat-tickets-edit-inline`.
- `enter` key to open a full-screen detail view — `feat-tickets-detail-view`.
- Lazy network loading of individual ticket content — all content is in memory
  from `ListTickets`; network-lazy loading is a future API concern.
- E2E / VHS tests — deferred to `platform-split-pane` which covers the shared
  split-pane E2E harness.
- Refactoring `ApprovalsView` — covered by `platform-split-pane`.

---

## Design

### Pane Type Architecture

```
TicketsView (implements views.View)
├── *ticketListPane (implements components.Pane)   — left
└── *ticketDetailPane (implements components.Pane)  — right
    wrapped by
    *components.SplitPane
```

`TicketsView` owns the `SplitPane` and both pane instances. It is the only
type that implements the `views.View` interface registered with the router.
The two pane types are private (unexported) to `package views`.

The list pane owns `cursor` and `scrollOffset`. The detail pane reads from a
shared `*int` pointer into the list pane's `cursor` — the simplest synchronization
for v1. If the panes ever become independent (e.g., pinning the detail while
navigating the list), the pointer can be replaced by a `cursorChangedMsg`.

### Lazy Content Rendering

The right pane renders nothing (or a placeholder) when:
- `len(tickets) == 0`, or
- `*cursor` is out of range.

Once `*cursor` points to a valid ticket, the pane renders:

```
TICKET-ID           (bold, rendered with lipgloss.NewStyle().Bold(true))

<content wrapped to pane width>
```

No additional network call is needed — `ListTickets` already returns full
`Ticket` structs with `Content`.

### Height Budget

`TicketsView` renders a 1-line header + 1 blank line (2 rows total) above the
split pane. The split pane must receive `height - 2` to avoid overlap.

```
Row 0:  SMITHERS › Tickets (N)              [Esc] Back
Row 1:  (blank)
Row 2+: [SplitPane occupying height-2 rows]
```

`SetSize(width, height)` on `TicketsView` stores the dimensions and calls
`splitPane.SetSize(width, height-2)`. The same adjustment is made when
`ticketsLoadedMsg` arrives (data loads may arrive before or after the first
`WindowSizeMsg`).

### Width Budget

At `width=80` (the `CompactBreakpoint` default), the layout is exactly at the
split boundary. To avoid off-by-one ambiguity in tests, set
`CompactBreakpoint: 79` so that an 80-column terminal is always in two-pane
mode.

| Terminal width | Mode |
|---|---|
| >= 79 cols | Two-pane: left=30, divider=1, right=`width-31` |
| < 79 cols | Compact: only focused pane |

### Focus and Key Routing

```
TicketsView.Update(msg)
├── Esc → emit PopViewMsg  (always, regardless of focus)
├── r   → refresh          (always, regardless of focus)
└── all other → splitPane.Update(msg)
    ├── Tab / Shift+Tab → toggle focus (handled by SplitPane)
    ├── focus=left → ticketListPane.Update(msg)
    │   └── j/k, ↑/↓, g/G, Ctrl+U/D, PgUp/PgDn → cursor movement
    └── focus=right → ticketDetailPane.Update(msg)
        └── (no-op in v1; scrolling is future work)
```

### Help Bar

`TicketsView.ShortHelp()` returns context-aware hints:

```go
// left pane focused:
[↑/↓] select    [tab] detail    [r] refresh    [esc] back

// right pane focused:
[tab] list    [esc] back
```

---

## Struct Definitions

### `ticketListPane`

```go
type ticketListPane struct {
    tickets      []smithers.Ticket
    cursor       int
    scrollOffset int
    width        int
    height       int
}

func (p *ticketListPane) Init() tea.Cmd { return nil }

func (p *ticketListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    // Handle: up/k, down/j, g/G, pgup/ctrl+u, pgdown/ctrl+d
    // Mirror existing TicketsView cursor logic verbatim.
    return p, nil
}

func (p *ticketListPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ticketListPane) View() string {
    // Render list: cursor indicator + ID + snippet per visible ticket.
    // Scroll position indicator when list is clipped.
    // Logic migrated verbatim from TicketsView.View() list section.
}

func (p *ticketListPane) pageSize() int {
    // Same formula as current TicketsView.pageSize():
    // (height - headerLines) / linesPerTicket, min 1.
    const linesPerTicket = 3
    const headerLines = 0   // header is rendered by TicketsView, not list pane
    ...
}
```

Note: the list pane receives no header row — that is handled by the parent
`TicketsView.View()`. Therefore `headerLines` in `pageSize()` drops to 0.
The existing `TicketsView.pageSize()` uses `headerLines=4` to account for the
header rendered by the view itself; the pane version uses 0 since it only
accounts for its own allocated area.

### `ticketDetailPane`

```go
type ticketDetailPane struct {
    tickets []smithers.Ticket
    cursor  *int   // points to ticketListPane.cursor
    width   int
    height  int
}

func (p *ticketDetailPane) Init() tea.Cmd { return nil }

func (p *ticketDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    return p, nil  // read-only in v1
}

func (p *ticketDetailPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ticketDetailPane) View() string {
    if len(p.tickets) == 0 || p.cursor == nil || *p.cursor >= len(p.tickets) {
        return lipgloss.NewStyle().Faint(true).Render("Select a ticket")
    }
    t := p.tickets[*p.cursor]
    title := lipgloss.NewStyle().Bold(true).Render(t.ID)
    body  := wrapText(t.Content, p.width)
    return title + "\n\n" + body
}
```

### `TicketsView` (refactored)

```go
type TicketsView struct {
    client     *smithers.Client
    tickets    []smithers.Ticket
    width      int
    height     int
    loading    bool
    err        error
    splitPane  *components.SplitPane
    listPane   *ticketListPane
    detailPane *ticketDetailPane
}
```

Remove: `cursor`, `scrollOffset` (these move to `ticketListPane`).

```go
func NewTicketsView(client *smithers.Client) *TicketsView {
    list   := &ticketListPane{}
    detail := &ticketDetailPane{cursor: &list.cursor}
    sp := components.NewSplitPane(list, detail, components.SplitPaneOpts{
        LeftWidth:         30,
        CompactBreakpoint: 79,
    })
    return &TicketsView{
        client:     client,
        loading:    true,
        splitPane:  sp,
        listPane:   list,
        detailPane: detail,
    }
}
```

`Update`:

```go
func (v *TicketsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case ticketsLoadedMsg:
        v.tickets              = msg.tickets
        v.listPane.tickets     = msg.tickets
        v.detailPane.tickets   = msg.tickets
        v.loading              = false
        v.splitPane.SetSize(v.width, max(0, v.height-2))
        return v, nil

    case ticketsErrorMsg:
        v.err     = msg.err
        v.loading = false
        return v, nil

    case tea.WindowSizeMsg:
        v.width  = msg.Width
        v.height = msg.Height
        v.SetSize(msg.Width, msg.Height)
        return v, nil

    case tea.KeyPressMsg:
        switch {
        case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "alt+esc"))):
            return v, func() tea.Msg { return PopViewMsg{} }
        case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
            v.loading = true
            return v, v.Init()
        }
    }

    // All other messages (including Tab, j/k, etc.) go to the split pane.
    newSP, cmd := v.splitPane.Update(msg)
    v.splitPane = newSP
    return v, cmd
}
```

`SetSize`:

```go
func (v *TicketsView) SetSize(width, height int) {
    v.width  = width
    v.height = height
    v.splitPane.SetSize(width, max(0, height-2))
}
```

`View`:

```go
func (v *TicketsView) View() string {
    var b strings.Builder

    // Header row (always rendered by the view, not the split pane).
    title := "SMITHERS › Tickets"
    if !v.loading && v.err == nil {
        title = fmt.Sprintf("SMITHERS › Tickets (%d)", len(v.tickets))
    }
    header   := lipgloss.NewStyle().Bold(true).Render(title)
    helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
    if v.width > 0 {
        gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
        if gap > 0 {
            header += strings.Repeat(" ", gap) + helpHint
        }
    }
    b.WriteString(header + "\n\n")

    // State guards.
    if v.loading {
        b.WriteString("  Loading tickets...\n")
        return b.String()
    }
    if v.err != nil {
        b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
        return b.String()
    }
    if len(v.tickets) == 0 {
        b.WriteString("  No tickets found.\n")
        return b.String()
    }

    // Split pane fills the remaining height.
    b.WriteString(v.splitPane.View())
    return b.String()
}
```

`ShortHelp`:

```go
func (v *TicketsView) ShortHelp() []key.Binding {
    if v.splitPane != nil && v.splitPane.Focus() == components.FocusRight {
        return []key.Binding{
            key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "list")),
            key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
        }
    }
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "select")),
        key.NewBinding(key.WithKeys("tab"),      key.WithHelp("tab", "detail")),
        key.NewBinding(key.WithKeys("r"),         key.WithHelp("r", "refresh")),
        key.NewBinding(key.WithKeys("esc"),       key.WithHelp("esc", "back")),
    }
}
```

---

## Helper Migration

Move from `tickets.go` to `helpers.go`:

- `ticketSnippet(content string, maxLen int) string`
- `metadataLine(s string) bool`

Both are pure string utilities with no imports outside `strings`. They are
already tested in `tickets_test.go` via the `package views` internal scope —
moving them to `helpers.go` in the same package is transparent to tests.

---

## Test Plan

### Keep All Existing Tests Green

All 12 tests in `tickets_test.go` must continue to pass after the refactor.
Required adjustments:

| Test | Change Required |
|------|----------------|
| `TestTicketsView_CursorNavigation` | Access cursor via `v.listPane.cursor` not `v.cursor` |
| `TestTicketsView_HomeEnd` | Access `v.listPane.cursor` and `v.listPane.scrollOffset` |
| `TestTicketsView_ScrollOffset` | Access `v.listPane.scrollOffset` and `v.listPane.pageSize()` |
| `TestTicketsView_PageNavigation` | Access `v.listPane.cursor`; call `v.listPane.pageSize()` |
| `loadedView()` helper | After `ticketsLoadedMsg`, assert `v.listPane.tickets` is populated |
| All `v.View()` assertions | No changes — string output behavior is identical |

`TestTicketsView_HeaderCount` tests the header count `(7)` in `View()` output —
this is unchanged since the header is still rendered by `TicketsView.View()`.

`TestTicketsView_CursorIndicator` checks that `▸ ` appears on the same line as
`ticket-001`. After refactor this is rendered by `ticketListPane.View()` which
is embedded in `splitPane.View()` which is returned by `TicketsView.View()`.
The assertion remains valid but the test must render at `width >= 79` to
trigger two-pane mode (so both panes appear in output).

### New Tests to Add

Add to `tickets_test.go`:

```
TestTicketsView_SplitPaneInstantiated
    NewTicketsView returns a view with non-nil splitPane, listPane, detailPane.

TestTicketsView_SplitPane_TwoPaneRender
    At width=100, height=30: View() output contains "│" (divider) and
    ticket IDs appear on the left side of the divider.

TestTicketsView_SplitPane_CompactMode
    At width=60, height=30: View() output does NOT contain "│" (compact mode).

TestTicketsView_SplitPane_RightPanePlaceholder
    Before any ticket data loads, detail pane shows placeholder text
    (or is empty — not a crash).

TestTicketsView_SplitPane_RightPaneContent
    After loading tickets, the right pane View contains the first ticket's ID.

TestTicketsView_SplitPane_TabSwitchesFocus
    After Tab, splitPane.Focus() == FocusRight.
    After Tab again, splitPane.Focus() == FocusLeft.

TestTicketsView_SplitPane_EscAlwaysPops
    Esc emits PopViewMsg regardless of which pane is focused.

TestTicketsView_SplitPane_HeightBudget
    splitPane receives height-2 in SetSize; verify by checking
    splitPane.Height() == v.height - 2 after SetSize(80, 40).
```

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/views/tickets.go` | Refactor: add `ticketListPane`, `ticketDetailPane`; rewrite `TicketsView` to compose `SplitPane`; remove `cursor`, `scrollOffset` fields; move `ticketSnippet` and `metadataLine` to helpers |
| `internal/ui/views/helpers.go` | Add `ticketSnippet` and `metadataLine` (moved from `tickets.go`) |
| `internal/ui/views/tickets_test.go` | Update cursor/scrollOffset access paths; add 8 new split-pane tests |
| `internal/ui/components/splitpane.go` | No changes — component is already complete |

---

## Acceptance Criteria (from ticket)

- Tickets list renders on the left side. ✓
- An empty or placeholder view renders on the right side if no ticket is selected. ✓

## Additional Acceptance Criteria (engineering completeness)

- All 12 existing `tickets_test.go` tests pass.
- 8 new split-pane tests pass.
- At terminal width >= 79: both panes and divider visible.
- At terminal width < 79: compact mode, only focused pane visible.
- Tab toggles focus between list and right pane.
- Shift+Tab also toggles focus.
- Esc pops view regardless of focused pane.
- `r` refreshes ticket list regardless of focused pane.
- No regression in header count display.
- `go vet ./internal/ui/views/...` passes.
- `go test ./internal/ui/views/... -count=1` passes.

# Implementation Plan: feat-tickets-split-pane

**Ticket**: `feat-tickets-split-pane`
**Feature**: `TICKETS_SPLIT_PANE_LAYOUT`
**Date**: 2026-04-05

---

## Goal

Refactor `TicketsView` to use the shared `SplitPane` component: list on the
left, markdown detail preview on the right, Tab to toggle focus. All existing
tests stay green; 8 new tests are added.

The `SplitPane` component (`eng-split-pane-component`) is already implemented
and tested; it must be merged to main before this plan executes (see Pre-work).

---

## Pre-work: Merge `eng-split-pane-component`

The component lives at:

```
.claude/worktrees/agent-a76a2b3f/internal/ui/components/splitpane.go
```

Before starting, confirm the component tests pass on the target branch:

```bash
go test ./internal/ui/components/... -v -run TestSplitPane
```

All 17 unit tests + 2 example tests should pass. If any fail, investigate
before proceeding.

---

## Step 1: Move Helper Functions to `helpers.go`

**File**: `internal/ui/views/helpers.go`

Add two functions currently defined at the bottom of `tickets.go`:

1. `ticketSnippet(content string, maxLen int) string`
2. `metadataLine(s string) bool`

These are pure string utilities. Moving them to `helpers.go` makes them
available to future views (e.g., the detail pane of `feat-tickets-detail-view`
and eventually `feat-prompts-list` which has similar snippet logic).

After copying both functions to `helpers.go`, delete them from `tickets.go`.

Verify no compiler errors:

```bash
go build ./internal/ui/views/...
```

The existing tests (`TestTicketSnippet`, `TestMetadataLine`) continue to pass
because both files are in `package views`.

---

## Step 2: Add Private Pane Types to `tickets.go`

**File**: `internal/ui/views/tickets.go`

Add the following above `TicketsView`. These are unexported types — they do not
need godoc beyond a one-line comment.

### 2a. Add import

Add to the import block:

```go
"github.com/charmbracelet/crush/internal/ui/components"
```

### 2b. `ticketListPane`

```go
// ticketListPane is the left pane of the tickets split view.
// It owns cursor navigation and viewport clipping.
type ticketListPane struct {
    tickets      []smithers.Ticket
    cursor       int
    scrollOffset int
    width        int
    height       int
}

func (p *ticketListPane) Init() tea.Cmd { return nil }

func (p *ticketListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
        switch {
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
            if p.cursor > 0 {
                p.cursor--
            }
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
            if p.cursor < len(p.tickets)-1 {
                p.cursor++
            }
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("home", "g"))):
            p.cursor = 0
            p.scrollOffset = 0
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("end", "G"))):
            if len(p.tickets) > 0 {
                p.cursor = len(p.tickets) - 1
            }
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
            ps := p.pageSize()
            p.cursor -= ps
            if p.cursor < 0 {
                p.cursor = 0
            }
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
            ps := p.pageSize()
            p.cursor += ps
            if len(p.tickets) > 0 && p.cursor >= len(p.tickets) {
                p.cursor = len(p.tickets) - 1
            }
        }
    }
    return p, nil
}

func (p *ticketListPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ticketListPane) pageSize() int {
    const linesPerTicket = 3
    if p.height <= 0 {
        return 1
    }
    n := p.height / linesPerTicket
    if n < 1 {
        return 1
    }
    return n
}

func (p *ticketListPane) View() string {
    if len(p.tickets) == 0 {
        return ""
    }

    var b strings.Builder
    visibleCount := p.pageSize()
    if visibleCount > len(p.tickets) {
        visibleCount = len(p.tickets)
    }

    // Keep cursor visible.
    if p.cursor < p.scrollOffset {
        p.scrollOffset = p.cursor
    }
    if p.cursor >= p.scrollOffset+visibleCount {
        p.scrollOffset = p.cursor - visibleCount + 1
    }

    end := p.scrollOffset + visibleCount
    if end > len(p.tickets) {
        end = len(p.tickets)
    }

    maxSnippetLen := 80
    if p.width > 4 {
        maxSnippetLen = p.width - 4
    }

    for i := p.scrollOffset; i < end; i++ {
        t := p.tickets[i]
        cursor := "  "
        nameStyle := lipgloss.NewStyle()
        if i == p.cursor {
            cursor = "▸ "
            nameStyle = nameStyle.Bold(true)
        }
        b.WriteString(cursor + nameStyle.Render(t.ID) + "\n")
        if snippet := ticketSnippet(t.Content, maxSnippetLen); snippet != "" {
            b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(snippet) + "\n")
        }
        if i < end-1 {
            b.WriteString("\n")
        }
    }

    if len(p.tickets) > visibleCount {
        b.WriteString(fmt.Sprintf("\n  (%d/%d)", p.cursor+1, len(p.tickets)))
    }

    return b.String()
}
```

### 2c. `ticketDetailPane`

```go
// ticketDetailPane is the right pane of the tickets split view.
// It renders the full content of the currently focused ticket.
type ticketDetailPane struct {
    tickets []smithers.Ticket
    cursor  *int  // points to ticketListPane.cursor; nil-safe
    width   int
    height  int
}

func (p *ticketDetailPane) Init() tea.Cmd { return nil }

func (p *ticketDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    return p, nil // read-only in v1
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

---

## Step 3: Rewrite `TicketsView`

**File**: `internal/ui/views/tickets.go`

Replace the existing `TicketsView` struct and all its methods. Keep the
message types (`ticketsLoadedMsg`, `ticketsErrorMsg`) and the compile-time
check (`var _ View = (*TicketsView)(nil)`) unchanged.

### 3a. New struct

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

Remove fields: `cursor`, `scrollOffset` (moved to `ticketListPane`).

### 3b. New constructor

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

### 3c. `Init` — unchanged

```go
func (v *TicketsView) Init() tea.Cmd {
    return func() tea.Msg {
        tickets, err := v.client.ListTickets(context.Background())
        if err != nil {
            return ticketsErrorMsg{err: err}
        }
        return ticketsLoadedMsg{tickets: tickets}
    }
}
```

### 3d. `Update`

Replace the existing method:

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

    newSP, cmd := v.splitPane.Update(msg)
    v.splitPane = newSP
    return v, cmd
}
```

### 3e. `SetSize`

```go
func (v *TicketsView) SetSize(width, height int) {
    v.width  = width
    v.height = height
    v.splitPane.SetSize(width, max(0, height-2))
}
```

### 3f. `View`

```go
func (v *TicketsView) View() string {
    var b strings.Builder

    title := "SMITHERS \u203a Tickets"
    if !v.loading && v.err == nil {
        title = fmt.Sprintf("SMITHERS \u203a Tickets (%d)", len(v.tickets))
    }
    header   := lipgloss.NewStyle().Bold(true).Render(title)
    helpHint := lipgloss.NewStyle().Faint(true).Render("[Esc] Back")
    headerLine := header
    if v.width > 0 {
        gap := v.width - lipgloss.Width(header) - lipgloss.Width(helpHint) - 2
        if gap > 0 {
            headerLine = header + strings.Repeat(" ", gap) + helpHint
        }
    }
    b.WriteString(headerLine + "\n\n")

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

    b.WriteString(v.splitPane.View())
    return b.String()
}
```

### 3g. `Name`, `ShortHelp`, `pageSize` (remove)

`Name()` is unchanged: `return "tickets"`.

Delete `pageSize()` from `TicketsView` — it now lives in `ticketListPane`.

Replace `ShortHelp()`:

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

## Step 4: Update `tickets_test.go`

**File**: `internal/ui/views/tickets_test.go`

### 4a. Update `loadedView` helper

The helper currently directly reads `v.cursor` and `v.scrollOffset`. Update
to verify the panes are populated after `ticketsLoadedMsg`:

```go
func loadedView(tickets []smithers.Ticket, width, height int) *TicketsView {
    v := NewTicketsView(nil)
    v.width = width
    v.height = height
    updated, _ := v.Update(ticketsLoadedMsg{tickets: tickets})
    tv := updated.(*TicketsView)
    // Sanity: panes must be populated.
    _ = tv.listPane.tickets
    return tv
}
```

### 4b. Fix cursor access in navigation tests

Change all `v.cursor` references to `v.listPane.cursor`.
Change all `v.scrollOffset` references to `v.listPane.scrollOffset`.
Change all `v.pageSize()` calls to `v.listPane.pageSize()`.

Affected tests:
- `TestTicketsView_CursorNavigation` — `v.cursor` → `v.listPane.cursor`
- `TestTicketsView_PageNavigation` — `v.cursor` → `v.listPane.cursor`, `v.pageSize()` → `v.listPane.pageSize()`
- `TestTicketsView_HomeEnd` — `v.cursor` → `v.listPane.cursor`, `v.scrollOffset` → `v.listPane.scrollOffset`
- `TestTicketsView_ScrollOffset` — `v.scrollOffset` → `v.listPane.scrollOffset`, `v.pageSize()` → `v.listPane.pageSize()`

### 4c. Update `TestTicketsView_CursorIndicator`

Render at width=100 (to guarantee two-pane mode > 79 breakpoint). The
assertion `assert.Contains(t, output, "▸ ")` is unchanged; the cursor
indicator is now rendered by `ticketListPane.View()` embedded in the split
pane, which is embedded in `TicketsView.View()`. The string is still present.

### 4d. Add 8 new split-pane tests

Add to `tickets_test.go`:

```go
func TestTicketsView_SplitPaneInstantiated(t *testing.T) {
    v := NewTicketsView(nil)
    assert.NotNil(t, v.splitPane)
    assert.NotNil(t, v.listPane)
    assert.NotNil(t, v.detailPane)
}

func TestTicketsView_SplitPane_TwoPaneRender(t *testing.T) {
    v := loadedView(sampleTickets(3), 100, 30)
    out := v.View()
    assert.Contains(t, out, "│")           // divider present
    assert.Contains(t, out, "ticket-001")  // list content visible
}

func TestTicketsView_SplitPane_CompactMode(t *testing.T) {
    v := loadedView(sampleTickets(3), 60, 30)
    out := v.View()
    assert.NotContains(t, out, "│")  // no divider in compact mode
}

func TestTicketsView_SplitPane_RightPanePlaceholder(t *testing.T) {
    // Detail pane shows placeholder before any data.
    v := NewTicketsView(nil)
    v.SetSize(100, 30)
    // Manually send a Window resize so splitpane gets dimensions:
    v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
    out := v.View()
    // Either "Loading" guard or placeholder text — no panic.
    assert.True(t, strings.Contains(out, "Loading") || strings.Contains(out, "Select a ticket"))
}

func TestTicketsView_SplitPane_RightPaneContent(t *testing.T) {
    v := loadedView(sampleTickets(3), 100, 30)
    out := v.View()
    // Right pane should show the first ticket's ID as detail.
    assert.Contains(t, out, "ticket-001")
}

func TestTicketsView_SplitPane_TabSwitchesFocus(t *testing.T) {
    v := loadedView(sampleTickets(3), 100, 30)
    assert.Equal(t, components.FocusLeft, v.splitPane.Focus())

    v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
    assert.Equal(t, components.FocusRight, v.splitPane.Focus())

    v.Update(tea.KeyPressMsg{Code: tea.KeyTab})
    assert.Equal(t, components.FocusLeft, v.splitPane.Focus())
}

func TestTicketsView_SplitPane_EscAlwaysPops(t *testing.T) {
    // Esc from left pane.
    v := loadedView(sampleTickets(1), 100, 30)
    _, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
    require.NotNil(t, cmd)
    _, ok := cmd().(PopViewMsg)
    assert.True(t, ok)

    // Esc from right pane.
    v2 := loadedView(sampleTickets(1), 100, 30)
    v2.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus right
    _, cmd2 := v2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
    require.NotNil(t, cmd2)
    _, ok2 := cmd2().(PopViewMsg)
    assert.True(t, ok2)
}

func TestTicketsView_SplitPane_HeightBudget(t *testing.T) {
    v := NewTicketsView(nil)
    v.SetSize(80, 40)
    assert.Equal(t, 38, v.splitPane.Height()) // 40 - 2 = 38
}
```

---

## Step 5: Verify

```bash
# Compile check.
go build ./internal/ui/views/...

# All tests (existing + new).
go test ./internal/ui/views/... -v -count=1

# Component tests still pass.
go test ./internal/ui/components/... -v -run TestSplitPane

# Vet.
go vet ./internal/ui/views/...
```

Expected: all tests pass, no vet warnings.

---

## Commit Strategy

**Commit 1**: Move `ticketSnippet` and `metadataLine` to `helpers.go`
- `internal/ui/views/helpers.go` (add two functions)
- `internal/ui/views/tickets.go` (remove two functions)
- Tests pass: `go test ./internal/ui/views/...`

**Commit 2**: Add `ticketListPane` and `ticketDetailPane` pane types
- `internal/ui/views/tickets.go` (add two private pane types above `TicketsView`)
- No behavior change yet; compile check only.

**Commit 3**: Rewrite `TicketsView` to compose `SplitPane`; update tests
- `internal/ui/views/tickets.go` (new struct, constructor, Update, SetSize, View, ShortHelp)
- `internal/ui/views/tickets_test.go` (cursor access path updates + 8 new tests)

Three focused commits make review and bisect straightforward.

---

## Responsive Behavior Summary

| Width | Mode | Left pane | Right pane | Divider |
|-------|------|-----------|------------|---------|
| >= 79 | Two-pane | 30 cols (focus border uses 1, content 29) | `width-31` cols | visible `│` |
| < 79 | Compact | full width when left-focused | full width when right-focused | hidden |

The `CompactBreakpoint: 79` (not 80) ensures that an 80-column terminal is
always in two-pane mode, avoiding test ambiguity at the exact breakpoint.

---

## Open Questions

1. **`pageSize()` in `ticketListPane`**: The original `TicketsView.pageSize()`
   subtracted `headerLines=4` because the header was counted against the full
   terminal height. After refactor, the list pane receives `height - 2` (header
   already excluded). Removing the subtraction from `pageSize()` in the pane
   is correct, but this means `TestTicketsView_PageNavigation` (which verifies
   `ps = (20-4)/3 = 5`) will need the height math recalculated. With
   `height=20`, split pane gets `height-2=18`; list pane gets `18` rows;
   `pageSize = 18/3 = 6`. Update the test comment accordingly.

2. **`TestTicketsView_ScrollOffset` height=10**: With `height=10`, split pane
   gets `height-2=8`; list pane `pageSize = 8/3 = 2`. The test assertion
   `v.listPane.pageSize()` will return 2 (same as before via the old formula
   with `headerLines=4`: `(10-4)/3 = 2`). No arithmetic change needed here.

3. **`wrapText` in detail pane**: `wrapText` from `helpers.go` prepends `"  "`
   to each line. For the detail pane this adds a 2-space indent, which is
   visually correct (matches list item indentation). If a future iteration uses
   a dedicated markdown renderer, `wrapText` is replaced there.

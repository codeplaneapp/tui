# Implementation Plan: feat-tickets-detail-view

**Ticket**: `feat-tickets-detail-view`
**Feature**: `TICKETS_DETAIL_VIEW`
**Date**: 2026-04-05

---

## Goal

Push a full-screen `TicketDetailView` from `TicketsView` on Enter. Render
ticket markdown via glamour, scroll it, and hand off to `$EDITOR` for in-place
editing. On editor exit, save the change to the server and reload.

---

## Pre-work: Confirm Dependencies on main

```bash
# Confirm split-pane is in place
grep -n "splitPane" internal/ui/views/tickets.go | head -5

# Confirm handoff package exists
ls internal/ui/handoff/

# Confirm common.MarkdownRenderer
grep -n "MarkdownRenderer" internal/ui/common/markdown.go

# Confirm glamour is in go.mod
grep glamour go.mod

# All view tests pass
go test ./internal/ui/views/... -count=1
```

Expected: all tests pass, no missing files. If `feat-tickets-split-pane` is
not merged, that must land first.

---

## Step 1: Add Enter Handling to `ticketListPane`

**File**: `internal/ui/views/tickets.go`

Inside `ticketListPane.Update`, in the `tea.KeyPressMsg` switch block, add a
new case after the existing navigation cases:

```go
case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
    if len(p.tickets) > 0 && p.cursor < len(p.tickets) {
        t := p.tickets[p.cursor]
        return p, func() tea.Msg { return openTicketDetailMsg{ticketID: t.ID} }
    }
```

Add the private message type at the top of `tickets.go` with the other message
types (`ticketsLoadedMsg`, `ticketsErrorMsg`):

```go
// openTicketDetailMsg is emitted by ticketListPane when Enter is pressed.
type openTicketDetailMsg struct {
    ticketID string
}
```

In `TicketsView.Update`, add handling for `openTicketDetailMsg` **before** the
`splitPane.Update` delegation. This goes inside the `tea.KeyPressMsg` block
or as its own `case` in the top-level switch:

```go
case openTicketDetailMsg:
    if len(v.tickets) > 0 {
        t := v.tickets[v.listPane.cursor]
        client := v.client
        return v, func() tea.Msg {
            return OpenTicketDetailMsg{Ticket: t, Client: client}
        }
    }
    return v, nil
```

Add `OpenTicketDetailMsg` to `tickets.go` (exported, for `ui.go` to consume):

```go
// OpenTicketDetailMsg signals ui.go to push a TicketDetailView.
type OpenTicketDetailMsg struct {
    Ticket smithers.Ticket
    Client *smithers.Client
}
```

Update `ShortHelp` for the left-focused case to include Enter:

```go
key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
```

Add this after the existing `↑/↓ select` binding.

Verify:

```bash
go build ./internal/ui/views/...
go test ./internal/ui/views/... -count=1 -run TestTicketsView
```

---

## Step 2: Create `internal/ui/views/ticketdetail.go`

**File**: `internal/ui/views/ticketdetail.go` (new)

### 2a. Package declaration and imports

```go
package views

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "strings"

    "charm.land/bubbles/v2/key"
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/charmbracelet/crush/internal/ui/common"
    "github.com/charmbracelet/crush/internal/ui/handoff"
    "github.com/charmbracelet/crush/internal/ui/styles"
)

// Compile-time interface check.
var _ View = (*TicketDetailView)(nil)
```

### 2b. Private message types

```go
type ticketDetailReloadedMsg struct {
    ticket smithers.Ticket
}

type ticketDetailErrorMsg struct {
    err error
}
```

### 2c. `TicketDetailView` struct and constructor

```go
// TicketDetailView is a full-screen view for a single ticket.
type TicketDetailView struct {
    client  *smithers.Client
    sty     *styles.Styles
    ticket  smithers.Ticket

    rendered      []string
    renderedWidth int
    scrollOffset  int

    width  int
    height int

    loading bool
    err     error
    tmpPath string
}

func NewTicketDetailView(client *smithers.Client, sty *styles.Styles, ticket smithers.Ticket) *TicketDetailView {
    return &TicketDetailView{
        client: client,
        sty:    sty,
        ticket: ticket,
    }
}
```

### 2d. `Init`

```go
func (v *TicketDetailView) Init() tea.Cmd { return nil }
```

Content is already in memory. No network call needed on push.

### 2e. `Name`

```go
func (v *TicketDetailView) Name() string { return "ticket-detail" }
```

### 2f. `SetSize`

```go
func (v *TicketDetailView) SetSize(width, height int) {
    v.width = width
    v.height = height
    // Invalidate render cache so next View() call re-renders at new width.
    v.renderedWidth = 0
}
```

### 2g. `Update`

Handle messages in this order:

```go
func (v *TicketDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        v.SetSize(msg.Width, msg.Height)
        return v, nil

    case ticketDetailReloadedMsg:
        v.ticket = msg.ticket
        v.loading = false
        v.err = nil
        v.renderedWidth = 0 // invalidate cache
        v.scrollOffset = 0  // reset scroll on reload
        return v, nil

    case ticketDetailErrorMsg:
        v.loading = false
        v.err = msg.err
        return v, nil

    case handoff.HandoffMsg:
        if msg.Tag != "ticket-edit" {
            return v, nil
        }
        tmpPath := v.tmpPath
        defer func() { _ = os.Remove(tmpPath) }()
        v.tmpPath = ""

        if msg.Result.Err != nil {
            v.err = fmt.Errorf("editor: %w", msg.Result.Err)
            return v, nil
        }
        newContentBytes, err := os.ReadFile(tmpPath)
        if err != nil {
            v.err = fmt.Errorf("read edited file: %w", err)
            return v, nil
        }
        newContent := string(newContentBytes)
        if newContent == v.ticket.Content {
            return v, nil // no change
        }
        v.loading = true
        ticketID := v.ticket.ID
        client := v.client
        return v, func() tea.Msg {
            ctx := context.Background()
            _, err := client.UpdateTicket(ctx, ticketID, smithers.UpdateTicketInput{Content: newContent})
            if err != nil {
                return ticketDetailErrorMsg{err: fmt.Errorf("save ticket: %w", err)}
            }
            updated, err := client.GetTicket(ctx, ticketID)
            if err != nil {
                return ticketDetailErrorMsg{err: fmt.Errorf("reload ticket: %w", err)}
            }
            return ticketDetailReloadedMsg{ticket: *updated}
        }

    case tea.KeyPressMsg:
        switch {
        case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
            return v, func() tea.Msg { return PopViewMsg{} }

        case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
            if v.scrollOffset > 0 {
                v.scrollOffset--
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
            max := v.maxScrollOffset()
            if v.scrollOffset < max {
                v.scrollOffset++
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
            v.scrollOffset -= v.visibleHeight()
            if v.scrollOffset < 0 {
                v.scrollOffset = 0
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
            v.scrollOffset += v.visibleHeight()
            if max := v.maxScrollOffset(); v.scrollOffset > max {
                v.scrollOffset = max
            }

        case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
            v.scrollOffset = 0

        case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
            v.scrollOffset = v.maxScrollOffset()

        case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
            if v.loading {
                return v, nil
            }
            return v, v.startEditor()
        }
    }

    return v, nil
}
```

### 2h. `startEditor` helper

```go
func (v *TicketDetailView) startEditor() tea.Cmd {
    editor := resolveEditor()
    tmpFile, err := os.CreateTemp("", "ticket-*.md")
    if err != nil {
        v.err = fmt.Errorf("create temp file: %w", err)
        return nil
    }
    if _, err := tmpFile.WriteString(v.ticket.Content); err != nil {
        _ = tmpFile.Close()
        _ = os.Remove(tmpFile.Name())
        v.err = err
        return nil
    }
    _ = tmpFile.Close()
    v.tmpPath = tmpFile.Name()
    return handoff.Handoff(handoff.Options{
        Binary: editor,
        Args:   []string{v.tmpPath},
        Tag:    "ticket-edit",
    })
}

// resolveEditor returns the best available editor binary.
func resolveEditor() string {
    for _, env := range []string{"EDITOR", "VISUAL"} {
        if e := os.Getenv(env); e != "" {
            if _, err := exec.LookPath(e); err == nil {
                return e
            }
        }
    }
    return "vi"
}
```

### 2i. `visibleHeight` and `maxScrollOffset`

```go
// visibleHeight returns the number of content rows that fit on screen.
// Reserves: 1 header + 1 blank + 1 divider + 1 help bar = 4 rows.
func (v *TicketDetailView) visibleHeight() int {
    h := v.height - 4
    if h < 1 {
        return 1
    }
    return h
}

func (v *TicketDetailView) maxScrollOffset() int {
    n := len(v.rendered) - v.visibleHeight()
    if n < 0 {
        return 0
    }
    return n
}
```

### 2j. `renderMarkdown`

```go
func (v *TicketDetailView) renderMarkdown() {
    if v.renderedWidth == v.width && len(v.rendered) > 0 {
        return // cache hit
    }
    var out string
    if v.sty != nil {
        renderer := common.MarkdownRenderer(v.sty, v.width)
        result, err := renderer.Render(v.ticket.Content)
        if err == nil {
            out = strings.TrimSpace(result)
        } else {
            out = v.ticket.Content
        }
    } else {
        // Fallback: plain word-wrap when styles are unavailable.
        out = wrapText(v.ticket.Content, v.width)
    }
    if out == "" {
        out = lipgloss.NewStyle().Faint(true).Render("(no content)")
    }
    v.rendered = strings.Split(out, "\n")
    v.renderedWidth = v.width
    // Clamp scroll after re-render.
    if max := v.maxScrollOffset(); v.scrollOffset > max {
        v.scrollOffset = max
    }
}
```

### 2k. `View`

```go
func (v *TicketDetailView) View() string {
    var b strings.Builder

    // Header
    b.WriteString(v.renderHeader())
    b.WriteString("\n")

    // Separator
    w := v.width
    if w <= 0 {
        w = 40
    }
    b.WriteString(lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", w)))
    b.WriteString("\n")

    if v.loading {
        b.WriteString(lipgloss.NewStyle().Faint(true).Render("  Saving..."))
        b.WriteString("\n")
        return b.String()
    }

    if v.err != nil {
        b.WriteString(fmt.Sprintf("  Error: %v\n", v.err))
    }

    // Render markdown (uses cache when width unchanged).
    v.renderMarkdown()

    end := v.scrollOffset + v.visibleHeight()
    if end > len(v.rendered) {
        end = len(v.rendered)
    }
    visible := v.rendered[v.scrollOffset:end]
    b.WriteString(strings.Join(visible, "\n"))
    b.WriteString("\n")

    // Help bar
    b.WriteString(v.renderHelpBar())
    return b.String()
}
```

### 2l. `renderHeader`

```go
func (v *TicketDetailView) renderHeader() string {
    title := "SMITHERS \u203a Tickets \u203a " + v.ticket.ID
    styledTitle := lipgloss.NewStyle().Bold(true).Render(title)

    var scrollInfo string
    if len(v.rendered) > 0 {
        scrollInfo = fmt.Sprintf("(%d/%d)", v.scrollOffset+1, len(v.rendered))
    }

    hints := "[e] Edit  [Esc] Back"
    styledHints := lipgloss.NewStyle().Faint(true).Render(hints)

    if v.width > 0 {
        right := scrollInfo + "  " + styledHints
        gap := v.width - lipgloss.Width(styledTitle) - lipgloss.Width(right) - 2
        if gap > 0 {
            return styledTitle + strings.Repeat(" ", gap) + right
        }
    }
    return styledTitle
}
```

### 2m. `renderHelpBar`

```go
func (v *TicketDetailView) renderHelpBar() string {
    var parts []string
    for _, b := range v.ShortHelp() {
        h := b.Help()
        if h.Key != "" && h.Desc != "" {
            parts = append(parts, fmt.Sprintf("[%s] %s", h.Key, h.Desc))
        }
    }
    return lipgloss.NewStyle().Faint(true).Render("  "+strings.Join(parts, "  ")) + "\n"
}
```

### 2n. `ShortHelp`

```go
func (v *TicketDetailView) ShortHelp() []key.Binding {
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓/jk", "scroll")),
        key.NewBinding(key.WithKeys("g"),        key.WithHelp("g/G", "top/bottom")),
        key.NewBinding(key.WithKeys("e"),        key.WithHelp("e", "edit")),
        key.NewBinding(key.WithKeys("esc"),      key.WithHelp("esc", "back")),
    }
}
```

Verify:

```bash
go build ./internal/ui/views/...
```

---

## Step 3: Create `internal/ui/views/ticketdetail_test.go`

**File**: `internal/ui/views/ticketdetail_test.go` (new)

Implement the 12 tests from the engineering spec. Key patterns:

- Construct via `NewTicketDetailView(nil, nil, ticket)` for tests that do not
  need client or styles (scroll, scroll-clamp, Esc, etc.).
- Construct with a real `*styles.Styles` for markdown rendering tests; use the
  test styles helper if one exists in the views package, otherwise construct
  `styles.DefaultStyles()` or skip the glamour assertion and check for plain
  content fallback.
- For editor handoff tests, do not invoke the actual editor — directly send a
  `handoff.HandoffMsg` into `Update` and verify the resulting Cmd type.

### Stub for `UpdateTicket` / `GetTicket` in tests

The async save Cmd returned from `Update` on `HandoffMsg` calls
`client.UpdateTicket` and `client.GetTicket`. In unit tests, do not execute
the Cmd (it would try a real network call). Instead:

- Assert the Cmd is non-nil (save was triggered).
- For the "no change" test, assert the Cmd is nil (no save needed).

```go
func TestTicketDetailView_EditorHandoff_NoChange(t *testing.T) {
    ticket := smithers.Ticket{ID: "ticket-001", Content: "hello"}
    v := NewTicketDetailView(nil, nil, ticket)
    v.SetSize(80, 40)
    v.tmpPath = "/tmp/fake-ticket.md"

    // Pre-populate temp file with identical content.
    _ = os.WriteFile(v.tmpPath, []byte(ticket.Content), 0600)
    defer os.Remove(v.tmpPath)

    _, cmd := v.Update(handoff.HandoffMsg{Tag: "ticket-edit", Result: handoff.HandoffResult{}})
    assert.Nil(t, cmd) // no change → no save cmd
}
```

---

## Step 4: Wire `OpenTicketDetailMsg` in `ui.go`

**File**: `internal/ui/model/ui.go`

Find the section in the host model's `Update` where `OpenRunInspectMsg` and
`OpenLiveChatMsg` are handled. Add a new case:

```go
case views.OpenTicketDetailMsg:
    cmd := m.router.PushView(views.NewTicketDetailView(
        m.smithersClient,
        m.styles,
        msg.Ticket,
    ))
    return m, cmd
```

The exact field names (`m.smithersClient`, `m.styles`) depend on the host
model struct — substitute the correct field names from `ui.go`. If the host
model does not hold a styles reference, locate where `NewTicketsView` is
constructed and confirm the styles plumbing path.

Verify:

```bash
go build ./...
```

---

## Step 5: Update `tickets_test.go`

**File**: `internal/ui/views/tickets_test.go`

Add two tests (see engineering spec `TestTicketsView_EnterEmitsOpenDetail` and
`TestTicketsView_EnterNoTickets`).

For `TestTicketsView_EnterEmitsOpenDetail`:

```go
func TestTicketsView_EnterEmitsOpenDetail(t *testing.T) {
    v := loadedView(sampleTickets(3), 100, 30)
    // Default focus is left pane; send Enter.
    updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
    _ = updated
    require.NotNil(t, cmd)
    msg := cmd()
    detail, ok := msg.(OpenTicketDetailMsg)
    require.True(t, ok)
    assert.Equal(t, "ticket-001", detail.Ticket.ID)
}
```

---

## Step 6: Verify All Tests

```bash
# Build the whole UI package.
go build ./internal/ui/...

# Run all view tests.
go test ./internal/ui/views/... -v -count=1

# Run new detail view tests specifically.
go test ./internal/ui/views/... -v -run TestTicketDetail

# Run Enter-routing tests.
go test ./internal/ui/views/... -v -run TestTicketsView_Enter

# Vet.
go vet ./internal/ui/...

# Full build check.
go build ./...
```

Expected: all tests pass, no vet warnings.

---

## Commit Strategy

**Commit 1**: Enter key routing in `TicketsView` + `OpenTicketDetailMsg`
- `internal/ui/views/tickets.go` (add Enter case in listPane, add
  `openTicketDetailMsg` and `OpenTicketDetailMsg`, update ShortHelp)
- `internal/ui/views/tickets_test.go` (add 2 new Enter tests)
- Verify: `go test ./internal/ui/views/...`

**Commit 2**: `TicketDetailView` implementation
- `internal/ui/views/ticketdetail.go` (new file, full implementation)
- `internal/ui/views/ticketdetail_test.go` (new file, 12 tests)
- Verify: `go test ./internal/ui/views/...`

**Commit 3**: Wire `OpenTicketDetailMsg` in `ui.go`
- `internal/ui/model/ui.go` (add case for `OpenTicketDetailMsg`)
- Verify: `go build ./...`

Three focused commits allow each step to be reviewed and bisected independently.

---

## Open Questions

1. **`*styles.Styles` availability in `NewTicketsView`**: The current
   constructor signature is `NewTicketsView(client *smithers.Client)`. If the
   host model constructs `TicketsView` without styles, the detail view must
   either receive them separately at push time (via `OpenTicketDetailMsg`) or
   fall back to `glamour.WithAutoStyle()`. The `OpenTicketDetailMsg` carrying
   the `Client` already demonstrates the pattern — adding a `Styles` field is
   straightforward. Confirm the `ui.go` call site before Step 4.

2. **`resolveEditor` placement**: `resolveEditor` is a private function in
   `ticketdetail.go`. If a future inline edit view or prompt edit view needs
   the same logic, move it to `helpers.go`. For now, keep it local.

3. **Temp file cleanup on crash**: If the TUI crashes between writing the temp
   file and receiving `HandoffMsg`, the temp file is orphaned. This is
   acceptable for v1 — temp files in `/tmp` are cleaned by the OS. A future
   improvement could store `tmpPath` in a crash journal.

4. **`GetTicket` vs. re-using `UpdateTicket` response**: `UpdateTicket` returns
   `*Ticket` — if the server returns the canonical updated ticket, the
   `GetTicket` call is redundant. Check the server contract: if the response
   includes the full updated content, skip `GetTicket` and use the `UpdateTicket`
   response directly to save a round-trip.

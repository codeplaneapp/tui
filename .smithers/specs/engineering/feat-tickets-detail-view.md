# Engineering Spec: Tickets Detail View

**Ticket**: `feat-tickets-detail-view`
**Feature**: `TICKETS_DETAIL_VIEW`
**Status**: Draft
**Date**: 2026-04-05
**Dependencies**: `feat-tickets-split-pane` (complete), `eng-tickets-api-client` (complete), `eng-hijack-handoff-util` (complete)

---

## Objective

Add a full-screen `TicketDetailView` that is pushed onto the router stack when
the user presses Enter on a ticket in `TicketsView`. The view renders the
ticket's markdown content using glamour, supports keyboard scroll, and can
suspend the TUI to open the ticket in `$EDITOR`. After the editor exits the
view reloads the ticket content from the server.

---

## Scope

### In Scope

- New exported `TicketDetailView` type in `internal/ui/views/ticketdetail.go`
  that implements `views.View`.
- New exported `OpenTicketDetailMsg` struct that `ui.go` catches to push the
  view.
- Enter key handling in `ticketListPane.Update` emitting a private
  `openTicketDetailMsg`, which `TicketsView.Update` converts to the exported
  message.
- Glamour markdown rendering via `common.MarkdownRenderer` with width-keyed
  caching.
- Keyboard scroll: `↑/k`, `↓/j`, `PgUp/Ctrl+U`, `PgDn/Ctrl+D`, `g`/`G`
  (top/bottom).
- External editor handoff: `e` key → write temp file → `handoff.Handoff` →
  on return read temp file → `UpdateTicket` → `GetTicket` → re-render.
- Inline error display when `UpdateTicket` or `GetTicket` fails.
- `[e] edit  [esc] back` help bar with full `ShortHelp()` bindings.
- Unit tests for: Enter routing, scroll logic, width-change invalidation,
  editor handoff message flow (stubbed), error states.
- Plumbing `*styles.Styles` through `NewTicketsView` if not already present.

### Out of Scope

- Inline editing within the TUI — deferred to `feat-tickets-edit-inline`.
- Creating new tickets from the detail view.
- Mouse click scroll.
- Syntax highlighting of code blocks beyond what glamour provides natively.
- E2E / VHS tests — deferred.

---

## Design

### View Type Architecture

```
TicketsView (existing, views.View)
  ticketListPane  ← handles Enter → openTicketDetailMsg
  ↓
  TicketsView.Update → OpenTicketDetailMsg (exported)
  ↓
  ui.go → router.PushView(NewTicketDetailView(...))

TicketDetailView (new, views.View)
  ticket    smithers.Ticket   (in-memory, passed at construction)
  client    *smithers.Client  (for UpdateTicket + GetTicket)
  sty       *styles.Styles
  rendered  []string          (glamour output split into lines)
  renderedWidth int           (width at which rendered was computed)
  scrollOffset  int
  width, height int
  loading   bool              (true during reload after editor save)
  err       error
```

### Message Flow

```
[Enter in ticketListPane]
  → openTicketDetailMsg{ticketID: id} (private, within views package)
  → TicketsView.Update catches it
  → returns tea.Cmd emitting OpenTicketDetailMsg{TicketID: id, Ticket: &t}

[OpenTicketDetailMsg in ui.go]
  → router.PushView(NewTicketDetailView(client, sty, ticket))

[e key in TicketDetailView]
  → write ticket.Content to os.CreateTemp
  → handoff.Handoff(Options{Binary: editor, Args: [tmpPath], Tag: "ticket-edit"})
  → TUI suspended; editor runs full-screen

[handoff.HandoffMsg{Tag: "ticket-edit"}]
  → read tmpPath → os.Remove(tmpPath)
  → if content changed: tea.Cmd → UpdateTicket → GetTicket → ticketDetailReloadedMsg

[ticketDetailReloadedMsg]
  → update ticket.Content, invalidate cache
  → re-render markdown

[Esc in TicketDetailView]
  → tea.Cmd returning PopViewMsg
```

### Enter Key in `ticketListPane`

Add to `ticketListPane.Update` inside the `tea.KeyPressMsg` switch:

```go
case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
    if len(p.tickets) > 0 && p.cursor < len(p.tickets) {
        t := p.tickets[p.cursor]
        return p, func() tea.Msg { return openTicketDetailMsg{ticketID: t.ID} }
    }
```

Add to `TicketsView.Update` before the splitPane delegation:

```go
case openTicketDetailMsg:
    if len(v.tickets) > 0 {
        t := v.tickets[v.listPane.cursor]
        return v, func() tea.Msg {
            return OpenTicketDetailMsg{Ticket: t, Client: v.client}
        }
    }
```

`OpenTicketDetailMsg` carries the full `smithers.Ticket` (already in memory)
and the `*smithers.Client`. The host model (`ui.go`) constructs
`NewTicketDetailView` from these.

```go
// OpenTicketDetailMsg signals ui.go to push a TicketDetailView.
type OpenTicketDetailMsg struct {
    Ticket smithers.Ticket
    Client *smithers.Client
}
```

### Styles Plumbing

`TicketDetailView` needs `*styles.Styles` to call `common.MarkdownRenderer`.
If `NewTicketsView` currently does not accept styles, it must be updated:

```go
func NewTicketsView(client *smithers.Client, sty *styles.Styles) *TicketsView
```

`TicketsView` stores `sty *styles.Styles` and passes it via `OpenTicketDetailMsg`
or the constructor. If updating `NewTicketsView`'s signature is too disruptive,
`TicketDetailView` may fall back to constructing a renderer with
`glamour.WithAutoStyle()` — acceptable for v1 if styles are not available.

### Markdown Rendering and Cache

```go
func (v *TicketDetailView) renderMarkdown() {
    if v.renderedWidth == v.width && len(v.rendered) > 0 {
        return // cache hit
    }
    renderer := common.MarkdownRenderer(v.sty, v.width)
    out, err := renderer.Render(v.ticket.Content)
    if err != nil {
        out = v.ticket.Content
    }
    out = strings.TrimSpace(out)
    v.rendered = strings.Split(out, "\n")
    v.renderedWidth = v.width
}
```

Called at the top of `View()` and when `SetSize` changes the width.
Scroll offset is clamped to `max(0, len(v.rendered)-visibleHeight())` after
re-render to prevent rendering past end.

### Scroll Model

```
visibleHeight() = v.height - headerRows - footerRows
  where headerRows = 2 (title line + blank separator)
        footerRows = 1 (help bar)

View() renders v.rendered[v.scrollOffset : v.scrollOffset+visibleHeight()]
Scroll up:   scrollOffset = max(0, scrollOffset-1)
Scroll down: scrollOffset = min(max(0, len(rendered)-visibleHeight()), scrollOffset+1)
PgUp:        scrollOffset = max(0, scrollOffset-visibleHeight())
PgDn:        scrollOffset = min(max(0, len(rendered)-visibleHeight()), scrollOffset+visibleHeight())
g (top):     scrollOffset = 0
G (bottom):  scrollOffset = max(0, len(rendered)-visibleHeight())
```

Scroll indicator in the title bar: `(N/M)` where N = scrollOffset+1, M = len(rendered).

### External Editor Handoff

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
    if v.loading {
        return v, nil
    }
    editor := resolveEditor() // $EDITOR, $VISUAL, "vi"
    tmpFile, err := os.CreateTemp("", "ticket-*.md")
    if err != nil {
        v.err = fmt.Errorf("create temp file: %w", err)
        return v, nil
    }
    if _, err := tmpFile.WriteString(v.ticket.Content); err != nil {
        _ = tmpFile.Close()
        _ = os.Remove(tmpFile.Name())
        v.err = err
        return v, nil
    }
    _ = tmpFile.Close()
    v.tmpPath = tmpFile.Name()
    return v, handoff.Handoff(handoff.Options{
        Binary: editor,
        Args:   []string{v.tmpPath},
        Tag:    "ticket-edit",
    })
```

`resolveEditor()` is a private helper:

```go
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

On `handoff.HandoffMsg{Tag: "ticket-edit"}`:

```go
case handoff.HandoffMsg:
    if msg.Tag != "ticket-edit" {
        return v, nil
    }
    defer os.Remove(v.tmpPath)
    if msg.Result.Err != nil {
        // Editor exited non-zero or failed to start.
        v.err = fmt.Errorf("editor: %w", msg.Result.Err)
        return v, nil
    }
    newContent, err := os.ReadFile(v.tmpPath)
    if err != nil {
        v.err = fmt.Errorf("read temp file: %w", err)
        return v, nil
    }
    if string(newContent) == v.ticket.Content {
        return v, nil // no change; skip server round-trip
    }
    v.loading = true
    ticketID := v.ticket.ID
    client := v.client
    content := string(newContent)
    return v, func() tea.Msg {
        ctx := context.Background()
        _, err := client.UpdateTicket(ctx, ticketID, smithers.UpdateTicketInput{Content: content})
        if err != nil {
            return ticketDetailErrorMsg{err: fmt.Errorf("save ticket: %w", err)}
        }
        updated, err := client.GetTicket(ctx, ticketID)
        if err != nil {
            return ticketDetailErrorMsg{err: fmt.Errorf("reload ticket: %w", err)}
        }
        return ticketDetailReloadedMsg{ticket: *updated}
    }
```

### View Rendering

```
SMITHERS › Tickets › feat-tickets-detail-view  (12/47)      [e] Edit  [Esc] Back
─────────────────────────────────────────────────────────────────────────────────
<glamour-rendered markdown lines, clipped to visibleHeight()>
...
[↑↓/jk] scroll  [g/G] top/bottom  [e] edit  [esc] back
```

Header line: breadcrumb + scroll indicator right-aligned.
Separator: `─` repeated to full width (same as `RunInspectView.renderDivider()`).
Body: rendered markdown lines, scrolled.
Footer: help bar from `ShortHelp()`.

### Error Display

When `v.err != nil`, render a one-line error below the separator in place of
the body:

```
  Error: <message>
```

Content is not cleared on error — if the ticket was loaded before the error,
the pre-error content remains visible in `v.ticket.Content` and the error line
appears at the top of the body area.

---

## Struct Definition

```go
// TicketDetailView is a full-screen view that renders a single ticket's
// markdown content with scroll and external editor handoff.
type TicketDetailView struct {
    client  *smithers.Client
    sty     *styles.Styles
    ticket  smithers.Ticket

    // Rendered output
    rendered      []string
    renderedWidth int

    // Scroll
    scrollOffset int

    // Terminal dimensions
    width  int
    height int

    // Loading / error
    loading bool
    err     error

    // Editor handoff temp file path
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

Implements `views.View`: `Init`, `Update`, `View`, `Name`, `SetSize`, `ShortHelp`.

`Init` returns `nil` — content is already in memory from `ListTickets`. No
async load needed on first push. (Reload after editor save is driven by a
Cmd returned from `Update`.)

---

## Message Types (private)

```go
// openTicketDetailMsg is emitted by ticketListPane when Enter is pressed.
type openTicketDetailMsg struct {
    ticketID string
}

// ticketDetailReloadedMsg carries the updated ticket after an editor save.
type ticketDetailReloadedMsg struct {
    ticket smithers.Ticket
}

// ticketDetailErrorMsg carries an error from an async operation.
type ticketDetailErrorMsg struct {
    err error
}
```

---

## ShortHelp

```go
func (v *TicketDetailView) ShortHelp() []key.Binding {
    return []key.Binding{
        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓/jk", "scroll")),
        key.NewBinding(key.WithKeys("g"), key.WithHelp("g/G", "top/bottom")),
        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
    }
}
```

---

## Test Plan

### `internal/ui/views/ticketdetail_test.go` (new file)

```
TestTicketDetailView_ViewRendersTitle
    NewTicketDetailView; SetSize(120, 40); View() contains ticket.ID.

TestTicketDetailView_GlamourMarkdown
    Content "# Hello\n\nWorld"; View() contains "Hello" (heading rendered).

TestTicketDetailView_ScrollDown
    Set rendered to 50 lines, height=20; send j key 3 times; scrollOffset == 3.

TestTicketDetailView_ScrollUpClamped
    scrollOffset=0; send k key; scrollOffset remains 0 (no negative scroll).

TestTicketDetailView_GotoBottom
    Send G key; scrollOffset == max(0, len(rendered)-visibleHeight()).

TestTicketDetailView_GotoTop
    Set scrollOffset=10; send g key; scrollOffset == 0.

TestTicketDetailView_EscPopsView
    Send Esc; returned Cmd produces PopViewMsg.

TestTicketDetailView_WidthChangeInvalidatesCache
    Render at width=80; SetSize(100, 40); renderedWidth != 80 after invalidation.

TestTicketDetailView_LoadingState
    Set loading=true; View() contains "Saving...".

TestTicketDetailView_ErrorDisplay
    Set err=errors.New("timeout"); View() contains "Error: timeout".

TestTicketDetailView_EditorHandoff_NoChange
    Simulate HandoffMsg with Tag="ticket-edit", ExitCode=0, but temp file
    content identical to ticket.Content; no UpdateTicket cmd emitted.

TestTicketDetailView_EditorHandoff_Error
    Simulate HandoffMsg with Tag="ticket-edit" and non-nil Err;
    v.err is set; no UpdateTicket cmd emitted.
```

### Updates to `internal/ui/views/tickets_test.go`

```
TestTicketsView_EnterEmitsOpenDetail
    Build loadedView with 3 tickets; focus left pane (default); send Enter;
    returned Cmd produces OpenTicketDetailMsg{} with correct TicketID.

TestTicketsView_EnterNoTickets
    Build NewTicketsView (no tickets loaded); send Enter; no cmd returned (no crash).
```

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/views/ticketdetail.go` | New file: `TicketDetailView`, `OpenTicketDetailMsg`, private message types, `resolveEditor` helper |
| `internal/ui/views/ticketdetail_test.go` | New file: 12 unit tests |
| `internal/ui/views/tickets.go` | Add `openTicketDetailMsg` handling in `TicketsView.Update`; add Enter handling in `ticketListPane.Update`; optionally add `sty` field to `TicketsView` |
| `internal/ui/views/tickets_test.go` | Add 2 new Enter-routing tests |
| `internal/ui/model/ui.go` | Handle `OpenTicketDetailMsg` — call `router.PushView(NewTicketDetailView(...))` |

---

## Acceptance Criteria

- Enter on a focused ticket pushes `TicketDetailView` via the router.
- Ticket content renders as styled markdown (headings, bold, lists, code blocks
  rendered by glamour).
- `↑/↓/j/k/PgUp/PgDn/g/G` scroll the rendered content.
- `e` suspends the TUI, opens `$EDITOR` with the ticket's markdown in a temp
  file. On exit, if the content changed, the server is updated and the view
  reloads the ticket.
- `Esc` pops the detail view and returns to `TicketsView`.
- No crash when the ticket has no content.
- No crash when `$EDITOR` is not set (falls back to `vi`).
- `go vet ./internal/ui/views/...` passes.
- `go test ./internal/ui/views/... -count=1` passes (all existing + new tests).

# Research: feat-tickets-detail-view

**Ticket**: `feat-tickets-detail-view`
**Feature**: `TICKETS_DETAIL_VIEW`
**Date**: 2026-04-05

---

## Objective

Push a full-screen detail view when the user presses Enter on a ticket in
`TicketsView`. The detail view renders the ticket's markdown content using
glamour, supports scrolling through long tickets, and can hand off the terminal
to an external editor (`$EDITOR`) so the user can modify the ticket in place.
On return from the editor the detail view reloads the ticket content.

---

## Dependency Status

### `feat-tickets-split-pane` — Complete (main branch)

`TicketsView` already renders a split-pane layout with `ticketListPane` on the
left and a read-only `ticketDetailPane` on the right. The `ticketDetailPane`
currently uses plain `wrapText` with no glamour rendering and no scroll. The
Enter key is not yet handled on the list pane.

Key struct fields available:
- `TicketsView.listPane.cursor` — integer index into `TicketsView.tickets`.
- `TicketsView.tickets[cursor].ID` — ticket ID string.
- `TicketsView.tickets[cursor].Content` — full markdown content (in-memory).
- `TicketsView.client` — `*smithers.Client` with `GetTicket(ctx, id)`.

### `eng-tickets-api-client` — Complete

`smithers.Client.GetTicket(ctx, ticketID string) (*Ticket, error)` is
implemented in `internal/smithers/tickets.go`. It routes: HTTP GET first, exec
fallback. Returns `ErrTicketNotFound` on 404. The `Ticket` struct has two
fields: `ID string` and `Content string`. `Content` is full markdown.

### `internal/ui/handoff` — Complete

`handoff.Handoff(opts Options) tea.Cmd` in `internal/ui/handoff/handoff.go`
suspends the TUI, execs an external binary via `tea.ExecProcess`, and returns a
`HandoffMsg` when the process exits. It resolves the binary via `exec.LookPath`
and merges `Env` overrides. Used by `LiveChatView` for hijack sessions via a
raw `tea.ExecProcess` call; for the editor handoff pattern `handoff.Handoff`
is a better fit since it handles the binary-not-found error path.

### `internal/ui/common.MarkdownRenderer` — Complete

`common.MarkdownRenderer(sty *styles.Styles, width int) *glamour.TermRenderer`
in `internal/ui/common/markdown.go` returns a glamour renderer configured with
the project's style sheet and word-wrap width. Used extensively by
`AssistantMessageItem.renderMarkdown` in `internal/ui/chat/assistant.go`.

### Router / View Stack — Complete

`internal/ui/views/router.go` implements a push/pop stack. Pushing a view calls
`v.SetSize(width, height)` then `v.Init()`. Popping returns focus to the
previous top-of-stack. To push a new view from `TicketsView.Update`, the view
emits a `tea.Cmd` that returns a domain message (e.g., `OpenTicketDetailMsg`),
and the host model (`ui.go`) catches that message and calls
`router.PushView(NewTicketDetailView(...))`. This is the same pattern used by
`RunInspectView` → `OpenRunInspectMsg` and `LiveChatView` → `OpenLiveChatMsg`.

---

## Existing Code Landscape

### `internal/ui/views/tickets.go` — current state

`ticketListPane.Update` handles `j/k`, arrows, `g/G`, `Ctrl+U/D`, `PgUp/PgDn`.
It does **not** handle `enter`. The `ticketDetailPane.Update` is a no-op stub
(`return p, nil`).

`TicketsView.Update` intercepts `esc` (pop) and `r` (refresh) before delegating
to `splitPane.Update`. Everything else routes to the focused pane.

To add Enter → open detail:
1. Handle `enter` in `ticketListPane.Update` by returning a `tea.Cmd` that
   emits a new `openTicketDetailMsg{ticketID: p.tickets[p.cursor].ID}`.
2. `TicketsView.Update` intercepts that message and returns a `tea.Cmd` that
   emits `OpenTicketDetailMsg{...}` (exported, for `ui.go` to handle).

### `internal/ui/views/router.go`

`Router.PushView(v View) tea.Cmd` pushes with stored dimensions. The host
model reads `PopViewMsg` to call `router.Pop()`. No changes to router needed.

### `internal/ui/views/helpers.go`

Has `wrapText`, `padRight`, `truncate`, `formatStatus`, `formatPayload`.
Glamour rendering does not belong in helpers — it belongs in the detail view
itself (or a dedicated `renderMarkdown` private method).

### `internal/ui/views/runinspect.go`

`RunInspectView` is the closest structural analogue: a full-screen view pushed
from a list view, displays structured data with a header, sub-header, scroll
navigation. Does not use glamour or external editor. Good reference for
View/Init/Update/SetSize/ShortHelp pattern.

### `internal/ui/views/livechat.go`

`LiveChatView` demonstrates external process handoff via `tea.ExecProcess` for
the hijack flow. It validates the binary via `exec.LookPath` before calling
`tea.ExecProcess(cmd, callback)`. The `handoff` package wraps this more cleanly
and is preferable for the editor case.

---

## Glamour Markdown Rendering

`charm.land/glamour/v2` is already a project dependency. The call site is:

```go
import "github.com/charmbracelet/crush/internal/ui/common"

renderer := common.MarkdownRenderer(sty, width)
rendered, err := renderer.Render(content)
if err != nil {
    rendered = content // plain fallback
}
rendered = strings.TrimSpace(rendered)
```

`MarkdownRenderer` creates a new renderer on each call — it is cheap (no
network, just struct init) and the rendered output is width-specific. For the
detail view, re-render when `width` changes (i.e., on `WindowSizeMsg`). Cache
the result at a given width to avoid re-rendering on every `View()` call.

The detail view does not have access to `*styles.Styles` directly — it must
either receive it at construction or use a styles singleton. Looking at how
`AssistantMessageItem` receives styles: via its `NewAssistantMessageItem(sty,
msg)` constructor. The `TicketDetailView` should similarly accept `*styles.Styles`
at construction time from `TicketsView`, which itself must carry `sty`. The
`TicketsView` constructor `NewTicketsView(client)` does not currently take
styles; this must be extended or the detail view must construct its own glamour
renderer using `glamour.WithAutoStyle()` as a fallback.

**Recommended approach**: pass `*styles.Styles` through `NewTicketsView` and
`NewTicketDetailView`. This keeps styling consistent with the rest of the app.
If `TicketsView` is currently constructed without styles in the host model,
update the call site.

Alternatively, construct the glamour renderer with `glamour.WithAutoStyle()`
which auto-detects dark/light terminal. This is a valid fallback but may not
match the rest of the app's palette.

---

## Viewport / Scroll Model

Glamour produces a multi-line ANSI string. To scroll it:

**Option A — Manual offset (current pattern in `ticketListPane`)**: store
`scrollOffset int` and `renderedLines []string`. `View()` slices
`renderedLines[scrollOffset:scrollOffset+height]` and joins them.
Simple, no new dependencies. This is what `LiveChatView` effectively does via
its own `lines []string` field.

**Option B — `bubbles/v2/viewport`**: the bubbles library has a `viewport`
component that manages scroll state, supports `Up`/`Down`/`GotoTop`/`GotoBottom`,
and handles `PgUp`/`PgDn`. It is used elsewhere in crush (the chat list uses
a custom list component; the viewport would be introduced here for the first
time in views/).

**Recommendation**: Option A for v1. The scroll logic is ~30 lines, requires
no new dependency, and matches the existing scroll pattern in the codebase.
If bubbles viewport is already a transitive dependency the case for Option B
is stronger, but Option A is safer for this ticket's scope.

---

## External Editor Handoff

When the user presses `e` in `TicketDetailView`:

1. Write `ticket.Content` to a temp file (`os.CreateTemp("", "ticket-*.md")`).
2. Read `$EDITOR` from the environment (fallback: `"vi"`).
3. Call `handoff.Handoff(handoff.Options{Binary: editor, Args: []string{tmpPath}})`.
4. On `handoff.HandoffMsg` return: read the temp file, call
   `client.UpdateTicket(ctx, id, UpdateTicketInput{Content: newContent})`.
5. On success: update the in-memory `ticket.Content`, re-render markdown,
   clear cache. On error: show an error overlay (reuse toast or inline error).

The temp file approach is necessary because `$EDITOR` expects a file path.
`os.CreateTemp` returns a `*os.File`; close it before handing the path to the
editor (some editors refuse to open a file that is still open by another
process). Remove the temp file after reading it back.

**Tag value** on the `HandoffMsg`: use a sentinel string `"ticket-edit"` so
the detail view's `Update` can detect the return and trigger the reload.

---

## GetTicket vs. In-Memory Content

`TicketsView` loads all tickets via `ListTickets()` which returns full `Ticket`
structs including `Content`. The detail view can therefore receive the ticket
struct directly at push time — no network call needed to display the initial
content.

However, after an editor handoff + `UpdateTicket`, the in-memory content must
be refreshed. Options:
1. After `UpdateTicket` succeeds, call `GetTicket(ctx, id)` to get the
   canonical server-side version (handles any normalisation the server applies).
2. Reuse the content written to the temp file directly.

Option 1 is safer. Implement it as a `ticketDetailReloadMsg` async command.

---

## UX Flow

```
TicketsView (split-pane)
  [Enter on focused ticket]
  ↓ emits OpenTicketDetailMsg{ticketID}
  ↓ host model calls router.PushView(NewTicketDetailView(client, sty, ticket))

TicketDetailView (full-screen)
  ┌─────────────────────────────────────────────────────┐
  │ SMITHERS › Tickets › feat-tickets-detail-view   [e] Edit  [Esc] Back │
  │ ─────────────────────────────────────────────────── │
  │ <glamour-rendered markdown content>                  │
  │ ...                                                  │
  │ (scroll indicator)                                   │
  │                                                      │
  │ [↑↓/jk] scroll  [g/G] top/bottom  [e] edit  [esc] back │
  └─────────────────────────────────────────────────────┘

  [e] → write temp file → handoff.Handoff($EDITOR, tmpPath)
  TUI suspends; editor opens in full terminal
  Editor exits → HandoffMsg received
  → read temp file → UpdateTicket → GetTicket → re-render markdown
```

---

## Msg Routing Summary

| Message | Produced by | Consumed by |
|---------|-------------|-------------|
| `openTicketDetailMsg` (private) | `ticketListPane.Update` on enter | `TicketsView.Update` (converts to exported msg) |
| `OpenTicketDetailMsg` (exported) | `TicketsView.Update` | `ui.go` host model → `router.PushView` |
| `ticketDetailLoadedMsg` (private) | async `GetTicket` cmd | `TicketDetailView.Update` |
| `ticketDetailSavedMsg` (private) | async `UpdateTicket` + `GetTicket` chain | `TicketDetailView.Update` |
| `handoff.HandoffMsg` | `handoff.Handoff` cmd | `TicketDetailView.Update` |
| `PopViewMsg` | `TicketDetailView.Update` on esc | `ui.go` host → `router.Pop()` |

---

## Files to Read Before Implementation

1. `/Users/williamcory/crush/internal/ui/views/tickets.go` — current TicketsView, pane types
2. `/Users/williamcory/crush/internal/smithers/tickets.go` — GetTicket, UpdateTicket signatures
3. `/Users/williamcory/crush/internal/smithers/types.go` — Ticket struct
4. `/Users/williamcory/crush/internal/ui/common/markdown.go` — MarkdownRenderer
5. `/Users/williamcory/crush/internal/ui/handoff/handoff.go` — Handoff, HandoffMsg, Options
6. `/Users/williamcory/crush/internal/ui/views/runinspect.go` — structural reference for full-screen view pattern
7. `/Users/williamcory/crush/internal/ui/views/livechat.go` — external process handoff pattern
8. `/Users/williamcory/crush/internal/ui/views/router.go` — Push/Pop/PushView
9. `/Users/williamcory/crush/internal/ui/model/ui.go` — how OpenRunInspectMsg and OpenLiveChatMsg are handled, to replicate for OpenTicketDetailMsg

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| `*styles.Styles` not plumbed into `NewTicketsView` | Check the `TicketsView` construction call in `ui.go`; add `sty` parameter if absent. If not feasible, use `glamour.WithAutoStyle()` as a fallback. |
| Temp file left on disk if editor crashes | Defer `os.Remove(tmpPath)` in the HandoffMsg handler unconditionally. |
| `$EDITOR` not set | Ordered fallback: `$EDITOR`, `$VISUAL`, `"vi"`. Use `exec.LookPath` on each; if none found, show error toast rather than crashing. |
| `UpdateTicket` fails (network/server error) | Show inline error below the content; keep the pre-edit content in memory so the view is not blank. |
| Glamour render width changes on resize | Invalidate the cached render in `SetSize`. Re-render on next `View()` call. |
| Scroll state becomes invalid after re-render (content length changes) | Clamp `scrollOffset` to `max(0, len(renderedLines)-height)` in `View()`. |
| `Enter` in right pane of split view vs. Enter in list pane | `Enter` is only handled in `ticketListPane.Update` (left pane). When focus is on the right pane, Enter is a no-op (or ignored) — the split view is for preview only. |

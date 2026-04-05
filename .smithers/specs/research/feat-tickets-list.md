# Research: feat-tickets-list

**Ticket**: `feat-tickets-list`
**Feature**: `TICKETS_LIST`
**Dependency**: `eng-tickets-api-client` (already implemented)
**Date**: 2026-04-05

---

## 1. Existing Scaffold Audit

### 1.1 TicketsView (`internal/ui/views/tickets.go`) — What Works

The file is 179 lines and satisfies the `View` interface at compile-time (`var _ View = (*TicketsView)(nil)`).

**Struct**: `TicketsView` holds `client *smithers.Client`, `tickets []smithers.Ticket`, `cursor int`, `width/height int`, `loading bool`, `err error`. All fields needed for a basic list are present.

**Init**: Issues an async command that calls `client.ListTickets(ctx)` and returns either `ticketsLoadedMsg` or `ticketsErrorMsg`. This is the correct Bubble Tea pattern (side-effect-free model, async command for I/O).

**Update**: Handles all expected message types:
- `ticketsLoadedMsg` — sets `v.tickets`, clears `v.loading`
- `ticketsErrorMsg` — sets `v.err`, clears `v.loading`
- `tea.WindowSizeMsg` — captures `width`/`height`
- `tea.KeyPressMsg` for `esc` (pop view), `up/k`, `down/j` (cursor), `r` (refresh), `enter` (no-op placeholder)

**View**: Renders a `SMITHERS › Tickets` header with right-aligned `[Esc] Back`, a `▸` cursor against the selected row, per-ticket ID + snippet lines separated by blank lines. Loading and error states produce their own outputs.

**ShortHelp**: Returns `["[Enter] View", "[r] Refresh", "[Esc] Back"]`.

**Router integration**: `internal/ui/model/ui.go:1465–1470` handles `ActionOpenTicketsView`, creates a `NewTicketsView(m.smithersClient)`, and pushes it onto the router. This is already wired and functional.

**Command palette**: `dialog/commands.go:528` has the "tickets" entry. `dialog/actions.go:90–91` defines `ActionOpenTicketsView`.

### 1.2 TicketsView — What Is Missing

1. **No viewport clipping**: `View()` (line 100) renders all tickets unconditionally using `for i, ticket := range v.tickets`. With 100+ `.smithers/tickets/*.md` files in this repo, the output overflows the terminal height without any scroll offset tracking.

2. **Naive snippet extraction**: `ticketSnippet()` (line 166–178) skips blank lines, `#` headings, and `---` separators, but returns the first remaining line. In this project's ticket files the first remaining content line is always a metadata list item like `- ID: feat-tickets-list` or `- Group: Content And Prompts`. The function never finds the actual summary text under `## Summary`.

3. **Header shows no count**: The header hardcodes `"SMITHERS › Tickets"`. The Design doc (§3.8) and the engineering spec both call for `"SMITHERS › Tickets (N)"` to show the loaded count.

4. **No page navigation**: Only `up/k` and `down/j` single-step navigation is implemented. Missing: `g`/`Home` (jump to top), `G`/`End` (jump to bottom), `PgUp`/`Ctrl+U` (half-page up), `PgDn`/`Ctrl+D` (half-page down).

5. **No footer help bar**: The view renders no footer. The ShortHelp data is defined but never rendered inline in the view body. The Design doc wireframe (§3.8) shows a dedicated `[↑/↓] Select  [Enter] View  [e] Edit  [n] New ticket  [Esc] Back` footer.

6. **Enter is a no-op**: The enter handler has the comment `// No-op for now; future: detail view.` No `ActionOpenTicketDetailView` dispatch or preview pane population occurs.

7. **No split-pane layout**: The Design wireframe (§3.8) shows a two-column layout with the ticket list on the left and ticket content on the right. The current implementation is single-column only, matching a pre-split-pane stage. This is tracked as a separate ticket (`feat-tickets-split-pane`), but the list view should be designed to accommodate the eventual split.

8. **No unit tests**: `internal/ui/views/tickets_test.go` does not exist. No tests cover `TicketsView` states, cursor navigation, or `ticketSnippet()` behavior.

---

## 2. Client Analysis: `ListTickets`

### 2.1 Method Implementation (`internal/smithers/client.go:516–532`)

```
Transport tier 1: HTTP GET /ticket/list → unmarshal []Ticket
Transport tier 2: exec smithers ticket list --format json → parseTicketsJSON
```

The method follows the identical two-tier pattern used for `ListCrons` (line 415), `ListApprovals` (line 372), etc. The `parseTicketsJSON` helper at line 831–838 does a straightforward `json.Unmarshal` into `[]Ticket`.

There is **no third tier** (direct filesystem access). Tickets in Smithers are stored as `.smithers/tickets/*.md` files, but `ListTickets` has no path that reads them directly with `os.ReadDir` + `os.ReadFile`. This means the method requires either the HTTP server (`smithers up --serve`) or the `smithers` CLI binary on `$PATH`. Without either, callers receive an error.

### 2.2 Ticket Type (`internal/smithers/types.go:78–81`)

```go
type Ticket struct {
    ID      string `json:"id"`      // Filename without .md extension, e.g. "feat-tickets-list"
    Content string `json:"content"` // Full markdown content of the ticket file
}
```

The type is minimal and complete for the list view's needs. The `Content` field contains the full raw markdown, which is what `ticketSnippet()` parses.

### 2.3 Missing Client Tests

`internal/smithers/client_test.go` has thorough tests for `ExecuteSQL`, `GetScores`, `RecallMemory`, `ListCrons`, `CreateCron`, `ToggleCron`, and `DeleteCron`. There are **zero tests for `ListTickets`**, `CreateTicket`, or `UpdateTicket`. This is an obvious gap given the established test patterns.

The `newTestServer` and `newExecClient` helpers (lines 18–51) provide everything needed to add HTTP and exec coverage with minimal boilerplate.

---

## 3. Data Flow and Rendering Requirements

### 3.1 Data Flow

```
TicketsView.Init()
    └── goroutine: client.ListTickets(ctx)
         ├── HTTP GET /ticket/list → []Ticket (server path)
         └── exec smithers ticket list --format json → []Ticket (exec path)
              └── parseTicketsJSON(out) → []Ticket

ticketsLoadedMsg{tickets} → TicketsView.Update()
    └── v.tickets = msg.tickets; v.loading = false

tea.WindowSizeMsg{Width, Height} → TicketsView.Update()
    └── v.width, v.height updated (used for viewport clipping)

TicketsView.View()
    ├── header: "SMITHERS › Tickets (N)" + right-aligned "[Esc] Back"
    ├── loading spinner / error message (early returns)
    ├── visible ticket window [scrollOffset : scrollOffset+visibleCount]
    │    └── for each ticket: cursor indicator + ID (bold if selected) + snippet (faint)
    └── footer: "[↑/↓] Select  [r] Refresh  [Esc] Back"
```

### 3.2 Ticket File Format in This Project

Every `.smithers/tickets/*.md` file in this repo follows a consistent structure:

```markdown
# <Title>

## Metadata
- ID: <ticket-id>
- Group: <group-name>
- Type: feature | task | bug
- Feature: <FEATURE_CONSTANT>
- Dependencies: <dep-1>, <dep-2>

## Summary

<actual summary text, 1-3 sentences>

## Acceptance Criteria

- ...

## Source Context

- ...

## Implementation Notes

- ...
```

The current `ticketSnippet()` returns the first non-blank, non-heading, non-separator line, which will always be `- ID: feat-tickets-list` for these files. The function needs to:
1. Detect the `## Summary` heading and capture content after it.
2. Skip `- Key: value` metadata list items even when not under `## Metadata`.
3. Fall back gracefully for plain markdown without the `## Summary` heading.

### 3.3 Rendering Requirements

**List item**: Each ticket requires 2–3 rendered lines:
- Line 1: `▸ <ticket-id>` (cursor selected, bold) or `  <ticket-id>` (unselected)
- Line 2: `  <snippet>` (faint, only if snippet is non-empty)
- Line 3: blank separator (except after last item)

**Viewport**: `linesPerTicket = 3` (worst case; 2 if snippet is empty). `headerLines = 4` (header + blank + optional border). `visibleCount = (height - headerLines) / linesPerTicket`. Scroll offset must be clamped so the cursor is always visible.

**Width**: The snippet should be truncated to `v.width - 4` characters (2 for cursor indent, 2 for padding) rather than the hardcoded 80 characters currently in `ticketSnippet()`.

---

## 4. Gap Analysis

### 4.1 Critical Gaps (Block Usability)

| Gap | Impact | Location |
|-----|--------|----------|
| No viewport clipping | List of 100+ tickets overflows terminal; unusable with this repo's ticket directory | `tickets.go:View()` |
| Snippet shows metadata not summary | Every ticket displays `- ID: feat-tickets-list` instead of meaningful text | `tickets.go:ticketSnippet()` |

### 4.2 Quality Gaps (Degrade UX)

| Gap | Impact | Location |
|-----|--------|----------|
| No ticket count in header | Operator doesn't know how many tickets loaded | `tickets.go:View()` |
| No page/home/end keys | Long lists require many keystrokes to navigate | `tickets.go:Update()` |
| No inline footer help | User must guess available keys (inconsistent with other views) | `tickets.go:View()` |
| Width-agnostic snippet truncation | Hardcoded 80-char limit clips on narrow terminals, wastes space on wide | `tickets.go:ticketSnippet()` |

### 4.3 Testing Gaps

| Gap | Impact | Location |
|-----|--------|----------|
| No `TicketsView` unit tests | Navigation bugs, rendering regressions undetected | Missing `views/tickets_test.go` |
| No `ListTickets` client tests | HTTP/exec transport paths uncovered | `client_test.go` (absent) |
| No E2E test | No automated verification that the view opens, loads, and navigates | Missing `tests/e2e/tickets_test.go` |
| No VHS tape | No visual regression record | Missing `tests/vhs/tickets-list.tape` |

### 4.4 Out-of-Scope Gaps (Tracked Elsewhere)

| Gap | Downstream Ticket |
|-----|-------------------|
| Ticket detail/preview panel | `feat-tickets-detail-view` |
| Split-pane list + detail layout | `feat-tickets-split-pane` |
| Inline text editing | `feat-tickets-edit-inline` |
| New ticket creation | `feat-tickets-create` |
| `$EDITOR` handoff | Separate downstream ticket |
| Direct filesystem transport tier | Future enhancement |

---

## 5. Keyboard Navigation Design

### 5.1 Current Keys

| Key | Action |
|-----|--------|
| `↑` / `k` | cursor up |
| `↓` / `j` | cursor down |
| `r` | refresh (re-run Init) |
| `Enter` | no-op |
| `Esc` / `Alt+Esc` | pop view (return to chat) |

### 5.2 Required Additions

| Key | Action | Notes |
|-----|--------|-------|
| `g` / `Home` | jump to first ticket | vim convention |
| `G` / `End` | jump to last ticket | vim convention |
| `PgUp` / `Ctrl+U` | half-page up | matches typical TUI conventions |
| `PgDn` / `Ctrl+D` | half-page down | matches typical TUI conventions |

Page size is computed as `(height - headerLines) / linesPerTicket` — the same formula used for the scroll offset window. This keeps page jumps consistent with the visible region.

### 5.3 Key Conflict Check

None of the new keys (`g`, `G`, `PgUp`, `Ctrl+U`, `PgDn`, `Ctrl+D`, `Home`, `End`) conflict with the existing bindings or with the global Crush keybindings in `internal/ui/model/keys.go`.

---

## 6. Design Specification Alignment

The Design doc wireframe (§3.8) specifies:

**View layout (split-pane target state)**:
```
│ Tickets              │ PROJ-123: Auth module security review        │
│ ──────────           │ ────────────────────────────────────         │
│ ▸ PROJ-123           │ ## Description                               │
│   PROJ-124           │ ...                                          │
│ [n] New  [e] Edit    │                                              │
│─────────────────────────────────────────────────────────────────────│
│ [↑/↓] Select  [Enter] View  [e] Edit  [n] New ticket  [Esc] Back  │
```

For `feat-tickets-list` scope (no split-pane, no detail, no edit/create), the deliverable is the **left panel only**: the navigable ticket list with ID + snippet, full viewport clipping, and the footer help bar. The `[Enter] View`, `[e] Edit`, and `[n] New` hints in the footer are placeholders until `feat-tickets-detail-view`, `feat-tickets-edit-inline`, and `feat-tickets-create` land.

---

## 7. Files Relevant to This Ticket

| File | Relevance | Current Status |
|------|-----------|----------------|
| `internal/ui/views/tickets.go` | Primary view — all changes land here | 179 lines, functional scaffold |
| `internal/smithers/client.go` | `ListTickets` method (lines 516–532) | Implemented, no tests |
| `internal/smithers/types.go` | `Ticket{ID, Content}` struct (lines 78–81) | Complete, no changes needed |
| `internal/smithers/client_test.go` | Client tests | No ticket coverage |
| `internal/ui/views/router.go` | View stack (Pop/Push/PopViewMsg) | Fully functional, no changes |
| `internal/ui/model/ui.go` | `ActionOpenTicketsView` handler (line 1465) | Wired and functional |
| `internal/ui/dialog/commands.go` | Command palette entry (line 528) | Registered |
| `internal/ui/dialog/actions.go` | `ActionOpenTicketsView` (line 90–91) | Defined |
| `tests/vhs/smithers-domain-system-prompt.tape` | Reference pattern for new VHS tape | Reference only |
| `.smithers/tickets/feat-tickets-list.md` | Ticket definition | Source of truth |

---

## 8. Related Research Documents

- `.smithers/specs/research/eng-tickets-api-client.md` — API client transport and type design
- `.smithers/specs/research/feat-agents-browser.md` — Structural reference; identical view architecture (`AgentsView` ≈ `TicketsView`)
- `.smithers/specs/research/platform-view-model.md` — View router and stack design
- `.smithers/specs/research/eng-split-pane-component.md` — Future split-pane dependency for `feat-tickets-split-pane`

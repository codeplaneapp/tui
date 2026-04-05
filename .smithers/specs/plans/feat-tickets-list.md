# Plan: feat-tickets-list — Tickets List View

**Ticket**: `feat-tickets-list`
**Feature**: `TICKETS_LIST`
**Dependency**: `eng-tickets-api-client` (already shipped)
**Date**: 2026-04-05

---

## Goal

Harden the existing `TicketsView` scaffold from a functional-but-rough prototype to a production-quality navigable list view. The deliverable is the **list panel only** as described in Design §3.8 — no detail view, no split-pane, no create/edit. Those are downstream tickets.

The two critical defects are: (1) the list overflows the terminal with 100+ tickets and (2) every ticket shows its `- ID:` metadata line instead of its actual summary. Both must be fixed. Beyond those, the gap list from the research doc drives the rest of the work.

---

## Slices

### Slice 1 — Viewport Clipping and Scroll Offset

**File**: `internal/ui/views/tickets.go`

**Problem**: `View()` iterates `v.tickets` unconditionally. On a 40-line terminal with 3 lines per ticket, 14 visible items is the maximum before overflow. This repo has 100+ tickets.

**Change**: Add `scrollOffset int` to the `TicketsView` struct. In `View()`, after the empty-list early return, compute a visible window and render only tickets in that window.

```go
// Add to TicketsView struct:
scrollOffset int

// In View(), after the empty-list check:
linesPerTicket := 3 // cursor+ID line, snippet line, blank separator
headerLines := 4    // header + blank + footer + blank
visibleCount := (v.height - headerLines) / linesPerTicket
if visibleCount < 1 {
    visibleCount = len(v.tickets)
}

// Keep cursor visible (scroll follows cursor).
if v.cursor < v.scrollOffset {
    v.scrollOffset = v.cursor
}
if v.cursor >= v.scrollOffset+visibleCount {
    v.scrollOffset = v.cursor - visibleCount + 1
}

end := v.scrollOffset + visibleCount
if end > len(v.tickets) {
    end = len(v.tickets)
}

for i := v.scrollOffset; i < end; i++ {
    // ... existing rendering logic using v.tickets[i]
}
```

The cursor bounds checks in `Update()` (lines 78–84) clamp `cursor` to `[0, len(tickets)-1]` and need no change — the scroll offset is a pure rendering concern derived from cursor position.

Add a scroll position indicator (e.g. `  (14/107)`) appended to the header when `len(v.tickets) > visibleCount` to signal that the list is truncated.

---

### Slice 2 — Enhanced Snippet Extraction

**File**: `internal/ui/views/tickets.go` (`ticketSnippet` function, line 166)

**Problem**: Files in `.smithers/tickets/` start with a heading and then a `## Metadata` block whose list items (`- ID: ...`, `- Group: ...`) are the first non-heading, non-separator lines. The current function returns these metadata items as the snippet.

**Change**: Replace the current implementation with one that:
1. Tracks whether the parser is positioned after a `## Summary` or `## Description` heading.
2. Skips lines matching `^- \w+:` (metadata key-value patterns).
3. Returns the first non-empty content line encountered after a Summary/Description heading, or falls back to the first non-metadata, non-heading content line.
4. Accepts a `maxLen int` parameter (replacing the hardcoded 80) so callers can pass `v.width - 4`.

```go
func ticketSnippet(content string, maxLen int) string {
    if maxLen <= 0 {
        maxLen = 80
    }
    lines := strings.Split(content, "\n")
    afterSummary := false
    var fallback string

    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            continue
        }
        if strings.HasPrefix(trimmed, "#") {
            lower := strings.ToLower(trimmed)
            afterSummary = strings.Contains(lower, "summary") ||
                strings.Contains(lower, "description")
            continue
        }
        if strings.HasPrefix(trimmed, "---") {
            continue
        }
        // Skip YAML-style metadata list items.
        if metadataLine(trimmed) {
            continue
        }
        if afterSummary {
            return truncate(trimmed, maxLen)
        }
        // Store as fallback in case no Summary heading is found.
        if fallback == "" && !strings.HasPrefix(trimmed, "- ") {
            fallback = trimmed
        }
    }
    return truncate(fallback, maxLen)
}

// metadataLine returns true for lines like "- ID: foo", "- Group: bar".
func metadataLine(s string) bool {
    if !strings.HasPrefix(s, "- ") {
        return false
    }
    rest := s[2:]
    colon := strings.Index(rest, ":")
    if colon <= 0 {
        return false
    }
    key := rest[:colon]
    return !strings.Contains(key, " ") // single-word key = metadata
}

func truncate(s string, maxLen int) string {
    if len(s) > maxLen {
        return s[:maxLen-3] + "..."
    }
    return s
}
```

Update the two call sites in `View()` to pass `v.width - 4` (or `80` if `v.width == 0`).

---

### Slice 3 — Header Ticket Count and Footer Help Bar

**File**: `internal/ui/views/tickets.go`

**Header count**: Replace the hardcoded header string with a format that includes the count after loading completes:

```go
title := "SMITHERS › Tickets"
if !v.loading && v.err == nil {
    title = fmt.Sprintf("SMITHERS › Tickets (%d)", len(v.tickets))
}
header := lipgloss.NewStyle().Bold(true).Render(title)
```

**Footer help bar**: After the ticket list rendering loop, append a footer below a blank line:

```go
b.WriteString("\n")
footerStyle := lipgloss.NewStyle().Faint(true)
b.WriteString(footerStyle.Render(strings.Join(v.ShortHelp(), "  ")))
b.WriteString("\n")
```

**Update `ShortHelp()`** to match the design wireframe (current return is `["[Enter] View", "[r] Refresh", "[Esc] Back"]` — keep `[Enter] View` as a forward-looking hint even before the detail view lands):

```go
func (v *TicketsView) ShortHelp() []string {
    return []string{"[↑/↓] Select", "[Enter] View", "[r] Refresh", "[Esc] Back"}
}
```

---

### Slice 4 — Extended Keyboard Navigation

**File**: `internal/ui/views/tickets.go` (`Update` method, `tea.KeyPressMsg` switch)

Add page/home/end navigation cases after the existing `down/j` case:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("home", "g"))):
    v.cursor = 0
    v.scrollOffset = 0

case key.Matches(msg, key.NewBinding(key.WithKeys("end", "G"))):
    if len(v.tickets) > 0 {
        v.cursor = len(v.tickets) - 1
    }

case key.Matches(msg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
    pageSize := v.pageSize()
    v.cursor -= pageSize
    if v.cursor < 0 {
        v.cursor = 0
    }

case key.Matches(msg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
    pageSize := v.pageSize()
    v.cursor += pageSize
    if len(v.tickets) > 0 && v.cursor >= len(v.tickets) {
        v.cursor = len(v.tickets) - 1
    }
```

Extract the page size calculation into a helper to avoid duplication between the `Update` handler and the `View` renderer:

```go
func (v *TicketsView) pageSize() int {
    const linesPerTicket = 3
    const headerLines = 4
    if v.height <= headerLines {
        return 1
    }
    n := (v.height - headerLines) / linesPerTicket
    if n < 1 {
        return 1
    }
    return n
}
```

---

### Slice 5 — ListTickets Client Unit Tests

**File**: `internal/smithers/client_test.go`

Add three tests following the `TestListCrons_*` pattern (lines 261–292):

```go
// TestListTickets_HTTP verifies the HTTP transport path.
func TestListTickets_HTTP(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/ticket/list", r.URL.Path)
        assert.Equal(t, "GET", r.Method)
        writeEnvelope(t, w, []Ticket{
            {ID: "auth-bug", Content: "# Auth Bug\n\n## Summary\n\nFix the auth module."},
            {ID: "deploy-fix", Content: "# Deploy Fix\n\n## Summary\n\nFix deploys."},
        })
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    require.Len(t, tickets, 2)
    assert.Equal(t, "auth-bug", tickets[0].ID)
    assert.Contains(t, tickets[0].Content, "Auth Bug")
}

// TestListTickets_Exec verifies the exec fallback path.
func TestListTickets_Exec(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        assert.Equal(t, []string{"ticket", "list", "--format", "json"}, args)
        return json.Marshal([]Ticket{
            {ID: "test-ticket", Content: "# Test\n\nContent here."},
        })
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    require.Len(t, tickets, 1)
    assert.Equal(t, "test-ticket", tickets[0].ID)
}

// TestListTickets_Empty verifies empty list handling.
func TestListTickets_Empty(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        return json.Marshal([]Ticket{})
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    assert.Empty(t, tickets)
}
```

---

### Slice 6 — TicketsView Unit Tests

**File**: `internal/ui/views/tickets_test.go` (new)

The test file lives in `package views` (not `package views_test`) so it can access internal message types (`ticketsLoadedMsg`, `ticketsErrorMsg`) and the unexported `ticketSnippet` function.

**Test list**:

| Test | What It Covers |
|------|----------------|
| `TestTicketsView_Init` | `Init()` returns a non-nil command, `loading` is true on construction |
| `TestTicketsView_LoadedMsg` | `ticketsLoadedMsg` clears loading, populates tickets, IDs appear in View output |
| `TestTicketsView_ErrorMsg` | `ticketsErrorMsg` clears loading, error text appears in View output |
| `TestTicketsView_EmptyList` | Empty `ticketsLoadedMsg` shows "No tickets found" |
| `TestTicketsView_CursorNavigation` | `j`/`k` moves cursor, clamped at boundaries |
| `TestTicketsView_PageNavigation` | `PgDn`/`PgUp` jumps by page, clamped at boundaries |
| `TestTicketsView_HomeEnd` | `g`/`G` jumps to first/last |
| `TestTicketsView_Refresh` | `r` sets `loading = true`, returns a non-nil command |
| `TestTicketsView_Escape` | `Esc` returns a command that yields `PopViewMsg{}` |
| `TestTicketsView_CursorIndicator` | View output contains `▸ ` and the first ticket's ID on the same line |
| `TestTicketsView_HeaderCount` | After load, header contains `(N)` count |
| `TestTicketsView_ScrollOffset` | With a short terminal height and many tickets, cursor stays in visible window |
| `TestTicketSnippet` | Table-driven: normal markdown, metadata-heavy, Summary heading, plain list items, empty, long line truncation |

**Sample helper**:

```go
package views

import (
    "fmt"
    "strings"
    "testing"

    tea "charm.land/bubbletea/v2"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func sampleTickets(n int) []smithers.Ticket {
    t := make([]smithers.Ticket, n)
    for i := range n {
        t[i] = smithers.Ticket{
            ID: fmt.Sprintf("ticket-%03d", i+1),
            Content: fmt.Sprintf(
                "# Ticket %d\n\n## Metadata\n- ID: ticket-%03d\n\n## Summary\n\nSummary for ticket %d.",
                i+1, i+1, i+1,
            ),
        }
    }
    return t
}
```

**Key assertions for `TestTicketSnippet`**:

```go
func TestTicketSnippet(t *testing.T) {
    tests := []struct {
        name    string
        content string
        want    string
    }{
        {
            name:    "summary heading preferred",
            content: "# Title\n\n## Metadata\n- ID: foo\n- Group: bar\n\n## Summary\n\nActual summary here.",
            want:    "Actual summary here.",
        },
        {
            name:    "plain paragraph fallback",
            content: "# Title\n\nThis is the first paragraph.",
            want:    "This is the first paragraph.",
        },
        {
            name:    "metadata only skips all",
            content: "# Title\n\n## Metadata\n- ID: foo\n- Group: bar\n",
            want:    "",
        },
        {
            name:    "long line truncated",
            content: "# T\n\n## Summary\n\n" + strings.Repeat("x", 100),
            want:    strings.Repeat("x", 77) + "...",
        },
        {
            name:    "empty content",
            content: "",
            want:    "",
        },
        {
            name:    "description heading also works",
            content: "# T\n\n## Description\n\nSome description text.",
            want:    "Some description text.",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.want, ticketSnippet(tt.content, 80))
        })
    }
}
```

---

### Slice 7 — VHS Happy-Path Recording Test

**File**: `tests/vhs/tickets-list.tape` (new)

Follow the existing pattern in `tests/vhs/smithers-domain-system-prompt.tape`:

```tape
# Tickets list view happy-path smoke recording.
Output tests/vhs/output/tickets-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with VHS fixtures
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs-tickets go run ."
Enter
Sleep 3s

# Open command palette and navigate to tickets
Ctrl+p
Sleep 500ms
Type "tickets"
Sleep 500ms
Enter
Sleep 2s

# Ticket list should be visible
Screenshot tests/vhs/output/tickets-list-loaded.png

# Navigate down through a few tickets
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms

Screenshot tests/vhs/output/tickets-list-navigated.png

# Jump to end (G key)
Type "G"
Sleep 500ms

Screenshot tests/vhs/output/tickets-list-end.png

# Jump back to top (g key)
Type "g"
Sleep 500ms

# Return to chat
Escape
Sleep 1s

Screenshot tests/vhs/output/tickets-list-back.png

Ctrl+c
Sleep 1s
```

**Required fixtures**: Seed `tests/vhs/fixtures/.smithers/tickets/` with 3–5 representative `.md` files committed to the repo. Use real-looking ticket IDs and content with a `## Summary` section so the snippet extraction is visually verified.

The fixtures directory already has `tests/vhs/fixtures/crush.json` — verify that the smithers client can discover tickets from the fixtures directory or that the fixture config points to the right path.

---

## Testing Strategy

### Automated

| Check | Command | Pass Criteria |
|-------|---------|---------------|
| View unit tests | `go test ./internal/ui/views/ -run TestTickets -v` | All `TestTicketsView_*` and `TestTicketSnippet` pass |
| Client unit tests | `go test ./internal/smithers/ -run TestListTickets -v` | `TestListTickets_HTTP`, `_Exec`, `_Empty` pass |
| Full suite | `go test ./...` | Zero regressions across all packages |
| VHS recording | `vhs tests/vhs/tickets-list.tape` | Generates output files without error |
| Lint | `golangci-lint run ./internal/ui/views/... ./internal/smithers/...` | No new warnings |

### Manual Verification Checklist

1. **Launch** with a project containing `.smithers/tickets/` files: `go run .`
2. **Open command palette** (`Ctrl+P`), type `tickets`, press Enter
3. **Header** reads `SMITHERS › Tickets (N)` with correct count
4. **Snippets** show summary text, not `- ID:` or `- Group:` lines
5. **Navigate** with `j`/`k`, `↑`/`↓`, `g`/`G`, `PgUp`/`PgDn` — cursor moves, viewport scrolls
6. **Scroll indicator** (`14/107`) appears when list exceeds terminal height
7. **Cursor always visible** — selecting the last item scrolls the viewport
8. **Footer** shows `[↑/↓] Select  [Enter] View  [r] Refresh  [Esc] Back`
9. **Refresh** (`r`) — "Loading tickets..." flashes briefly, list reloads
10. **Escape** — returns to chat/landing view, no error
11. **Empty state** — remove all `.smithers/tickets/` files, reopen, see "No tickets found."
12. **Error state** — misconfigure server/CLI, verify error message is displayed

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/views/tickets.go` | Add `scrollOffset`, `pageSize()`, update `View()` with viewport clipping + count + footer, replace `ticketSnippet()`, add `metadataLine()` and `truncate()` helpers, add page/home/end key handlers |
| `internal/ui/views/tickets_test.go` | New — all `TestTicketsView_*` and `TestTicketSnippet` tests |
| `internal/smithers/client_test.go` | Add `TestListTickets_HTTP`, `TestListTickets_Exec`, `TestListTickets_Empty` |
| `tests/vhs/tickets-list.tape` | New — VHS happy-path recording |
| `tests/vhs/fixtures/.smithers/tickets/*.md` | New — 3–5 fixture ticket files |

**No other files need changes.** The router, model, dialog actions, and command palette entries are all already wired from `eng-tickets-api-client`.

---

## Risks and Mitigations

### Risk 1: `ticketSnippet` still misses edge cases

**Risk**: The metadata detection heuristic (`metadataLine`) works for `- Key: Value` patterns but may produce wrong results for ticket files that use other formats (e.g., plain list items that start with `- ` but aren't metadata).

**Mitigation**: The table-driven `TestTicketSnippet` covers all known file formats in this repo. The function degrades gracefully — worst case it shows a non-metadata list item, which is functional. Add a `description` heading fallback so files without a `## Summary` heading still produce useful snippets.

### Risk 2: Width-adaptive truncation with zero width

**Risk**: `ticketSnippet(content, v.width-4)` is called before any `tea.WindowSizeMsg` arrives. `v.width` starts at `0`, producing a `maxLen` of `-4`.

**Mitigation**: Guard in the `ticketSnippet` function: `if maxLen <= 0 { maxLen = 80 }`. This is already in the proposed implementation.

### Risk 3: `ListTickets` requires server or CLI

**Risk**: Running `go run .` in an environment without the `smithers` binary or HTTP server causes `ListTickets` to return an error, and users see the error state immediately.

**Mitigation**: The error state UX already handles this gracefully. Not blocking for this ticket. A future enhancement (filesystem-direct tier) would add a fourth transport tier using `os.ReadDir` + `os.ReadFile`. For now, the error message should be informative: `"Error: no smithers transport available — run 'smithers up --serve' or install the smithers CLI"`.

### Risk 4: `G` key conflicts with lipgloss shift detection

**Risk**: The `key.NewBinding(key.WithKeys("G"))` case-sensitive uppercase key binding may be tricky in the Bubble Tea v2 key system. Some terminal emulators send different byte sequences for uppercase vs lowercase shift.

**Mitigation**: Test explicitly in the VHS recording (the `Type "G"` instruction) and in a unit test that sends a `tea.KeyPressMsg{Code: 'G'}`. If uppercase is unreliable, fall back to `End` key only for bottom-of-list navigation.

### Risk 5: VHS fixtures need smithers client configuration

**Risk**: The VHS tape uses `CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures` but the fixtures `crush.json` may not include smithers API configuration, so `ListTickets` will attempt to exec `smithers` and fail.

**Mitigation**: Two options: (1) Add `.smithers/tickets/` fixture files to `tests/vhs/fixtures/` and add a filesystem-direct transport tier so the view works without the CLI. (2) Use a mock/stub server. Option 1 is preferred but is a scope extension — gate the VHS test on having the smithers CLI available (add a skip comment), similar to how the `eng-tickets-api-client` plan handles this.

---

## Downstream Ticket Dependencies

This ticket's output (the list view) is consumed by:

| Ticket | What It Needs from This Ticket |
|--------|-------------------------------|
| `feat-tickets-detail-view` | Selected ticket's `ID` and `Content` from `v.tickets[v.cursor]`; wires the `Enter` key handler |
| `feat-tickets-split-pane` | The list panel as the left pane; needs `v.width` to be set to half-terminal width |
| `feat-tickets-edit-inline` | Depends on detail view landing first |
| `feat-tickets-create` | Wires the `n` key; needs the view to accept a `PushNewTicketMsg` after creation |

The `Enter` key handler placeholder in `Update()` should remain a no-op for this ticket — do not add a stub `ActionOpenTicketDetailView` dispatch here to avoid a false dependency.

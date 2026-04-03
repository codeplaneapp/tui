# Engineering Spec: feat-tickets-list — Tickets List View

**Ticket**: `feat-tickets-list`
**Feature**: `TICKETS_LIST`
**Group**: Content And Prompts
**Dependency**: `eng-tickets-api-client` (already implemented)
**Date**: 2026-04-03

---

## Objective

Ship a production-quality navigable ticket list view in the Crush TUI that mirrors the left-panel behavior of the upstream Smithers GUI's `TicketsList.tsx` (`smithers_tmp/gui-src/ui/TicketsList.tsx`). The view fetches tickets from the Smithers backend via `smithers.Client.ListTickets()`, renders each ticket as a selectable list item showing its ID and a content snippet, and supports keyboard navigation with proper viewport scrolling.

Much of the scaffolding already exists:
- **View struct**: `internal/ui/views/tickets.go` (179 lines) — functional but lacks viewport clipping, enhanced snippet extraction, and tests.
- **Client method**: `internal/smithers/client.go:482-498` — `ListTickets()` with HTTP → exec fallback is implemented.
- **Types**: `internal/smithers/types.go:78-81` — `Ticket{ID, Content}` struct exists.
- **Router integration**: `internal/ui/model/ui.go:1443-1448` — `ActionOpenTicketsView` handler wired.
- **Command palette**: `internal/ui/dialog/commands.go:528` — "tickets" entry registered.
- **Action type**: `internal/ui/dialog/actions.go:90-91` — `ActionOpenTicketsView` defined.

This spec focuses on hardening the existing scaffold to production quality, adding viewport clipping for long lists, improving snippet extraction for `.smithers/tickets/` file formats, and building out the full test suite.

---

## Scope

### In scope

1. **Viewport clipping** — Add scroll offset tracking to `TicketsView` so lists taller than the terminal are scrollable, keeping the cursor always visible.
2. **Snippet extraction improvement** — The existing `ticketSnippet()` function (`tickets.go:166-178`) skips headings and separators but not YAML-like metadata lines (`- ID:`, `- Group:`, etc.) common in `.smithers/tickets/` files. Enhance it to prefer the first line under `## Summary` or `## Description`.
3. **Ticket count in header** — Show `SMITHERS › Tickets (N)` for information density (Design §1.4).
4. **Page Up/Down, Home/End keys** — Standard terminal navigation: `g`/`Home` to top, `G`/`End` to bottom, `ctrl+u`/`ctrl+d` for half-page jumps.
5. **Footer help bar** — Render `ShortHelp()` output inline in the view's footer, matching the design wireframe.
6. **Unit tests** for `TicketsView` — All states, navigation edge cases, snippet extraction.
7. **Client unit tests** for `ListTickets` — HTTP and exec paths (currently absent from `client_test.go`).
8. **Terminal E2E test** — Go-based harness modeled on the upstream `tui-helpers.ts` `TUITestInstance` pattern.
9. **VHS happy-path recording test** — `.tape` file following `tests/vhs/smithers-domain-system-prompt.tape`.

### Out of scope

- Ticket detail view / markdown rendering → `feat-tickets-detail-view`
- Ticket create / edit → `feat-tickets-create`, `feat-tickets-edit-inline`
- Split-pane layout → `feat-tickets-split-pane`
- External editor handoff → separate downstream ticket

---

## Implementation Plan

### Slice 1: Viewport clipping and scroll offset

**File**: `internal/ui/views/tickets.go`

The current `View()` method at line 100 renders all tickets unconditionally. For projects with many tickets (e.g., this repo has 100+ `.smithers/tickets/*.md` files), the output overflows the terminal.

Add a `scrollOffset int` field to `TicketsView` and compute a visible window in `View()`:

```go
// Add to TicketsView struct (line 27):
scrollOffset int

// In View(), after the empty-list check (line 130):
// Each ticket takes ~3 lines (cursor+ID, snippet, separator blank).
linesPerTicket := 3
headerLines := 4 // header + blank + footer
visibleCount := (v.height - headerLines) / linesPerTicket
if visibleCount < 1 {
    visibleCount = len(v.tickets)
}
// Keep cursor visible
if v.cursor < v.scrollOffset {
    v.scrollOffset = v.cursor
}
if v.cursor >= v.scrollOffset+visibleCount {
    v.scrollOffset = v.cursor - visibleCount + 1
}
// Render only tickets[v.scrollOffset : end]
end := v.scrollOffset + visibleCount
if end > len(v.tickets) {
    end = len(v.tickets)
}
for i := v.scrollOffset; i < end; i++ {
    // ... existing rendering logic, using v.tickets[i]
}
```

Update the cursor bounds check in `Update()` (lines 78-84) to work with the new scroll offset — no logic change needed since `cursor` is already clamped to `[0, len(tickets)-1]`.

### Slice 2: Enhanced snippet extraction

**File**: `internal/ui/views/tickets.go` (function `ticketSnippet` at line 166)

Current behavior: skips blank lines, `#` headings, and `---` separators. Returns first remaining line, truncated to 80 chars.

Problem: `.smithers/tickets/` files in this repo start with a heading, then have metadata blocks like:
```
## Metadata
- ID: feat-tickets-list
- Group: Content And Prompts (content-and-prompts)
- Type: feature
```

The current function returns `"- ID: feat-tickets-list"` — not useful.

Enhancement:
1. Skip lines matching `^- \w+:` (metadata key-value pairs).
2. If a `## Summary` or `## Description` heading is found, return the first non-empty line after it.
3. Fall back to the first content line that isn't metadata or a heading.

```go
func ticketSnippet(content string) string {
    lines := strings.Split(content, "\n")
    afterSummary := false
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            continue
        }
        if strings.HasPrefix(trimmed, "#") {
            // Check if this is a Summary/Description heading
            lower := strings.ToLower(trimmed)
            afterSummary = strings.Contains(lower, "summary") || strings.Contains(lower, "description")
            continue
        }
        if strings.HasPrefix(trimmed, "---") {
            continue
        }
        // Skip YAML-like metadata lines
        if matched, _ := regexp.MatchString(`^- \w+:`, trimmed); matched {
            continue
        }
        if afterSummary || !strings.HasPrefix(trimmed, "- ") {
            if len(trimmed) > 80 {
                return trimmed[:77] + "..."
            }
            return trimmed
        }
    }
    return ""
}
```

### Slice 3: Header ticket count and footer help bar

**File**: `internal/ui/views/tickets.go`

1. **Header**: Change line 104 from `"SMITHERS › Tickets"` to include the count:
   ```go
   header := lipgloss.NewStyle().Bold(true).Render(
       fmt.Sprintf("SMITHERS › Tickets (%d)", len(v.tickets)),
   )
   ```
   Only show count after loading completes (when `v.loading` is false and `v.err` is nil).

2. **Footer**: After the ticket list rendering, append a footer bar:
   ```go
   // Footer
   b.WriteString("\n")
   footerStyle := lipgloss.NewStyle().Faint(true)
   b.WriteString(footerStyle.Render(strings.Join(v.ShortHelp(), "  ")))
   b.WriteString("\n")
   ```

3. **Update `ShortHelp()`** to match the design wireframe at Design §3.8:
   ```go
   func (v *TicketsView) ShortHelp() []string {
       return []string{"[↑/↓] Select", "[r] Refresh", "[Esc] Back"}
   }
   ```
   The `[Enter] View` hint will be added by `feat-tickets-detail-view`.

### Slice 4: Extended keyboard navigation

**File**: `internal/ui/views/tickets.go` (Update method, line 73)

Add page/home/end navigation in the `tea.KeyPressMsg` switch:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("home", "g"))):
    v.cursor = 0

case key.Matches(msg, key.NewBinding(key.WithKeys("end", "G"))):
    if len(v.tickets) > 0 {
        v.cursor = len(v.tickets) - 1
    }

case key.Matches(msg, key.NewBinding(key.WithKeys("pgup", "ctrl+u"))):
    linesPerTicket := 3
    headerLines := 4
    pageSize := (v.height - headerLines) / linesPerTicket
    if pageSize < 1 { pageSize = 1 }
    v.cursor -= pageSize
    if v.cursor < 0 { v.cursor = 0 }

case key.Matches(msg, key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))):
    linesPerTicket := 3
    headerLines := 4
    pageSize := (v.height - headerLines) / linesPerTicket
    if pageSize < 1 { pageSize = 1 }
    v.cursor += pageSize
    if v.cursor >= len(v.tickets) {
        v.cursor = len(v.tickets) - 1
    }
```

### Slice 5: ListTickets client unit tests

**File**: `internal/smithers/client_test.go`

Add tests for the `ListTickets` method using the established test patterns (see `TestListCrons_HTTP` at line 261, `TestListCrons_Exec` at line 279):

```go
func TestListTickets_HTTP(t *testing.T) {
    _, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/ticket/list", r.URL.Path)
        assert.Equal(t, "GET", r.Method)
        writeEnvelope(t, w, []Ticket{
            {ID: "auth-bug", Content: "# Auth Bug\n\nFix the auth module."},
            {ID: "deploy-fix", Content: "# Deploy Fix\n\nFix deploys."},
        })
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    require.Len(t, tickets, 2)
    assert.Equal(t, "auth-bug", tickets[0].ID)
    assert.Contains(t, tickets[0].Content, "Auth Bug")
}

func TestListTickets_Exec(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        assert.Equal(t, []string{"ticket", "list", "--format", "json"}, args)
        return json.Marshal([]Ticket{
            {ID: "test-ticket", Content: "Test content"},
        })
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    require.Len(t, tickets, 1)
    assert.Equal(t, "test-ticket", tickets[0].ID)
}

func TestListTickets_Empty(t *testing.T) {
    c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
        return json.Marshal([]Ticket{})
    })
    tickets, err := c.ListTickets(context.Background())
    require.NoError(t, err)
    assert.Empty(t, tickets)
}
```

### Slice 6: TicketsView unit tests

**File**: `internal/ui/views/tickets_test.go` (new)

```go
package views

import (
    "errors"
    "strings"
    "testing"

    tea "charm.land/bubbletea/v2"
    "github.com/charmbracelet/crush/internal/smithers"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func sampleTickets(n int) []smithers.Ticket {
    tickets := make([]smithers.Ticket, n)
    for i := range n {
        tickets[i] = smithers.Ticket{
            ID:      fmt.Sprintf("ticket-%03d", i+1),
            Content: fmt.Sprintf("# Ticket %d\n\n## Summary\n\nSummary for ticket %d.", i+1, i+1),
        }
    }
    return tickets
}

func TestTicketsView_Init(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    cmd := v.Init()
    assert.NotNil(t, cmd)
    assert.True(t, v.loading)
}

func TestTicketsView_LoadedMsg(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.width = 80
    v.height = 40
    tickets := sampleTickets(3)
    updated, _ := v.Update(ticketsLoadedMsg{tickets: tickets})
    tv := updated.(*TicketsView)
    assert.False(t, tv.loading)
    assert.Len(t, tv.tickets, 3)
    output := tv.View()
    assert.Contains(t, output, "ticket-001")
    assert.Contains(t, output, "ticket-002")
    assert.Contains(t, output, "ticket-003")
}

func TestTicketsView_ErrorMsg(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.width = 80
    v.height = 40
    updated, _ := v.Update(ticketsErrorMsg{err: errors.New("connection refused")})
    tv := updated.(*TicketsView)
    assert.False(t, tv.loading)
    assert.Contains(t, tv.View(), "Error:")
    assert.Contains(t, tv.View(), "connection refused")
}

func TestTicketsView_EmptyList(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.width = 80
    v.height = 40
    updated, _ := v.Update(ticketsLoadedMsg{tickets: []smithers.Ticket{}})
    tv := updated.(*TicketsView)
    assert.Contains(t, tv.View(), "No tickets found")
}

func TestTicketsView_CursorNavigation(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.Update(ticketsLoadedMsg{tickets: sampleTickets(5)})
    assert.Equal(t, 0, v.cursor)

    // Down 3 times
    for i := 0; i < 3; i++ {
        v.Update(tea.KeyPressMsg{Code: 'j'})
    }
    assert.Equal(t, 3, v.cursor)

    // Up once
    v.Update(tea.KeyPressMsg{Code: 'k'})
    assert.Equal(t, 2, v.cursor)

    // Up 5 times — clamped at 0
    for i := 0; i < 5; i++ {
        v.Update(tea.KeyPressMsg{Code: 'k'})
    }
    assert.Equal(t, 0, v.cursor)

    // Down past end — clamped at len-1
    for i := 0; i < 10; i++ {
        v.Update(tea.KeyPressMsg{Code: 'j'})
    }
    assert.Equal(t, 4, v.cursor)
}

func TestTicketsView_Refresh(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.Update(ticketsLoadedMsg{tickets: sampleTickets(2)})
    assert.False(t, v.loading)

    updated, cmd := v.Update(tea.KeyPressMsg{Code: 'r'})
    tv := updated.(*TicketsView)
    assert.True(t, tv.loading)
    assert.NotNil(t, cmd)
}

func TestTicketsView_Escape(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.Update(ticketsLoadedMsg{tickets: sampleTickets(1)})

    _, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
    require.NotNil(t, cmd)
    msg := cmd()
    _, ok := msg.(PopViewMsg)
    assert.True(t, ok)
}

func TestTicketsView_CursorIndicator(t *testing.T) {
    v := NewTicketsView(smithers.NewClient())
    v.width = 80
    v.height = 40
    v.Update(ticketsLoadedMsg{tickets: sampleTickets(3)})
    output := v.View()
    assert.Contains(t, output, "▸ ")
    // First item should have the cursor
    lines := strings.Split(output, "\n")
    found := false
    for _, line := range lines {
        if strings.Contains(line, "▸") && strings.Contains(line, "ticket-001") {
            found = true
            break
        }
    }
    assert.True(t, found, "cursor should be on first ticket")
}

func TestTicketSnippet(t *testing.T) {
    tests := []struct {
        name    string
        content string
        want    string
    }{
        {
            name:    "normal content",
            content: "# Title\n\nThis is the first paragraph.",
            want:    "This is the first paragraph.",
        },
        {
            name:    "metadata heavy",
            content: "# Ticket\n\n## Metadata\n- ID: foo\n- Group: bar\n\n## Summary\n\nActual summary here.",
            want:    "Actual summary here.",
        },
        {
            name:    "headings only",
            content: "# Title\n## Section\n---",
            want:    "",
        },
        {
            name:    "long line truncation",
            content: "# Title\n\n" + strings.Repeat("x", 100),
            want:    strings.Repeat("x", 77) + "...",
        },
        {
            name:    "empty",
            content: "",
            want:    "",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.want, ticketSnippet(tt.content))
        })
    }
}
```

### Slice 7: Terminal E2E test (tui-test harness pattern)

**File**: `tests/e2e/tickets_test.go` (new), `tests/e2e/helpers.go` (new if not existing)

Model the harness on the upstream `smithers_tmp/tests/tui-helpers.ts` (lines 1-113). The Go equivalent:

```go
// tests/e2e/helpers.go
package e2e

type TUIInstance struct {
    cmd    *exec.Cmd
    stdin  io.Writer
    buf    *syncBuffer // goroutine-safe buffer collecting stdout+stderr
}

// launchTUI compiles and spawns the TUI binary, returning a test instance.
func launchTUI(t *testing.T, projectDir string) *TUIInstance { ... }

// waitForText polls the output buffer for substring (ANSI stripped), up to timeout.
// Mirrors tui-helpers.ts:55-61.
func (ti *TUIInstance) waitForText(text string, timeout time.Duration) error { ... }

// waitForNoText polls until substring disappears. Mirrors tui-helpers.ts:64-70.
func (ti *TUIInstance) waitForNoText(text string, timeout time.Duration) error { ... }

// sendKeys writes raw bytes to stdin. Mirrors tui-helpers.ts:73-83.
func (ti *TUIInstance) sendKeys(text string) { ... }

// snapshot returns the current buffer with ANSI stripped. Mirrors tui-helpers.ts:85-87.
func (ti *TUIInstance) snapshot() string { ... }

// terminate kills the process. Mirrors tui-helpers.ts:89-91.
func (ti *TUIInstance) terminate() { ... }
```

**Test case** (mirrors the structure of `tui.e2e.test.ts:18-72`):

```go
func TestE2E_TicketsListNavigation(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // 1. Setup: seed .smithers/tickets/ with fixture files
    dir := t.TempDir()
    ticketsDir := filepath.Join(dir, ".smithers", "tickets")
    os.MkdirAll(ticketsDir, 0o755)
    os.WriteFile(filepath.Join(ticketsDir, "alpha-ticket.md"),
        []byte("# Alpha\n\n## Summary\n\nFirst test ticket."), 0o644)
    os.WriteFile(filepath.Join(ticketsDir, "beta-ticket.md"),
        []byte("# Beta\n\n## Summary\n\nSecond test ticket."), 0o644)

    // 2. Launch TUI
    tui := launchTUI(t, dir)
    defer tui.terminate()

    // 3. Open tickets view
    // Send Ctrl+P to open command palette (matching tui.e2e.test.ts pattern)
    tui.sendKeys("\x10") // Ctrl+P
    require.NoError(t, tui.waitForText("Tickets", 5*time.Second))
    tui.sendKeys("tickets\r")

    // 4. Verify view opened
    require.NoError(t, tui.waitForText("SMITHERS", 5*time.Second))
    require.NoError(t, tui.waitForText("alpha-ticket", 5*time.Second))
    require.NoError(t, tui.waitForText("beta-ticket", 5*time.Second))

    // 5. Navigate down
    tui.sendKeys("j") // down
    time.Sleep(200 * time.Millisecond)

    // 6. Esc back
    tui.sendKeys("\x1b") // Escape
    require.NoError(t, tui.waitForNoText("SMITHERS › Tickets", 5*time.Second))
}
```

This directly follows the upstream E2E pattern:
- `beforeAll` (setup) → seed fixture data, launch TUI
- Send keys → verify text appears (using `waitForText`)
- Navigate back → verify previous view restored
- `finally` (cleanup) → `terminate()`
- On failure → dump `snapshot()` for debugging (as in `tui.e2e.test.ts:67`)

### Slice 8: VHS happy-path recording test

**File**: `tests/vhs/tickets-list.tape` (new)

Following the existing pattern at `tests/vhs/smithers-domain-system-prompt.tape`:

```tape
# Tickets list view happy-path smoke recording.
Output tests/vhs/output/tickets-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Launch TUI with test fixtures that include seeded tickets
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

# Verify tickets list is displayed
Screenshot tests/vhs/output/tickets-list-loaded.png

# Navigate down through tickets
Down
Sleep 500ms
Down
Sleep 500ms

Screenshot tests/vhs/output/tickets-list-navigated.png

# Return to chat
Escape
Sleep 1s

Screenshot tests/vhs/output/tickets-list-back.png

Ctrl+c
Sleep 1s
```

**Required fixtures**: Create `tests/vhs/fixtures/.smithers/tickets/` with 3-5 sample `.md` ticket files committed to the repo.

---

## Validation

### Automated checks

| Check | Command | Pass criteria |
|-------|---------|---------------|
| View unit tests | `go test ./internal/ui/views/ -run TestTickets -v` | All `TestTicketsView_*` and `TestTicketSnippet` pass |
| Client unit tests | `go test ./internal/smithers/ -run TestListTickets -v` | `TestListTickets_HTTP`, `TestListTickets_Exec`, `TestListTickets_Empty` pass |
| Full test suite | `go test ./...` | Zero regressions across all packages |
| Terminal E2E | `go test ./tests/e2e/ -run TestE2E_TicketsListNavigation -timeout 30s -v` | TUI launches, tickets view opens, seeded tickets visible, navigation works, Esc returns to chat |
| VHS recording | `vhs tests/vhs/tickets-list.tape` | Generates `tests/vhs/output/tickets-list.gif` and `.png` files without error |
| Lint | `golangci-lint run ./internal/ui/views/... ./internal/smithers/...` | No new warnings |

### Terminal E2E coverage (modeled on upstream @microsoft/tui-test harness)

The Go E2E test helper (`tests/e2e/helpers.go`) mirrors the upstream `TUITestInstance` interface from `smithers_tmp/tests/tui-helpers.ts:10-16`:

| Upstream method (TS) | Go equivalent | Behavior |
|----------------------|---------------|----------|
| `waitForText(text, timeout)` | `waitForText(text string, timeout time.Duration) error` | Polls stdout buffer (ANSI-stripped) for substring match, returns error on timeout. Matches `tui-helpers.ts:55-61`. |
| `waitForNoText(text, timeout)` | `waitForNoText(text string, timeout time.Duration) error` | Polls until substring disappears. Matches `tui-helpers.ts:64-70`. |
| `sendKeys(text)` | `sendKeys(text string)` | Writes raw bytes to stdin pipe. Matches `tui-helpers.ts:73-83`. |
| `snapshot()` | `snapshot() string` | Returns ANSI-stripped buffer. Matches `tui-helpers.ts:85-87`. |
| `terminate()` | `terminate()` | Kills process. Matches `tui-helpers.ts:89-91`. |

The test structure follows `tui.e2e.test.ts:18-72`:
1. `beforeAll` → seed fixture data (cf. `tui.e2e.test.ts:6-16` spawning a background workflow)
2. Navigate to view → `waitForText` for expected content (cf. lines 23-24)
3. Send keys → verify state transitions (cf. lines 30-38)
4. Navigate back → verify previous view (cf. lines 50-59)
5. Error handling → dump `snapshot()` on failure (cf. lines 66-68)
6. Cleanup → `terminate()` (cf. line 70)

### VHS happy-path recording test

The `.tape` file follows the established pattern from `tests/vhs/smithers-domain-system-prompt.tape`:
- Uses `CRUSH_GLOBAL_CONFIG` and `CRUSH_GLOBAL_DATA` env vars for fixture isolation
- Takes screenshots at key moments for visual regression
- Generates a GIF for human review of the full flow

### Manual verification

1. **Launch** with a project containing `.smithers/tickets/` files → `go run .`
2. **Open command palette** (`Ctrl+P`), type "tickets", press Enter
3. **Verify** header reads "SMITHERS › Tickets (N)" with correct count
4. **Verify** ticket IDs appear with content snippets (not metadata lines)
5. **Navigate** with `j`/`k`, `↑`/`↓`, `g`/`G`, `PgUp`/`PgDn` — cursor moves, viewport scrolls
6. **Press `r`** — "Loading tickets..." flashes, list refreshes
7. **Press `Esc`** — returns to chat/landing view
8. **Empty state** — remove all ticket files, reopen view, verify "No tickets found" message
9. **Error state** — misconfigure API, verify error is displayed

---

## Risks

### 1. No existing Go E2E test infrastructure

**Risk**: Crush has no Go-based terminal E2E test helpers. The upstream harness (`tui-helpers.ts`) uses Bun to spawn a TS process; we need to spawn a compiled Go binary.

**Mitigation**: Build a minimal `tests/e2e/helpers.go` that mirrors `TUITestInstance`. Use `go build -o` in `TestMain` to pre-compile the binary once, then spawn it per test. The key challenge is that Bubble Tea uses the alternate screen buffer — ANSI stripping (as `tui-helpers.ts:44` does with `/\x1B\[[0-9;]*[a-zA-Z]/g`) is a reasonable first pass. The VHS test provides complementary visual verification.

### 2. Ticket snippet extraction for varied file formats

**Risk**: `.smithers/tickets/` files have inconsistent formats. Some have `## Metadata` blocks with `- ID:`, `- Group:` lines, some are plain markdown, some have frontmatter. The enhanced `ticketSnippet()` may still miss edge cases.

**Mitigation**: Table-driven tests in `TestTicketSnippet` cover the known variants. The function degrades gracefully — worst case it shows a metadata line, which is functional if not ideal.

### 3. `ListTickets` requires Smithers CLI or server

**Risk**: Without `smithers` on `$PATH` or an HTTP server running, `ListTickets` returns an error. Tickets are just markdown files on disk, so this is an unnecessary dependency for read-only listing.

**Crush vs Smithers mismatch**: The upstream GUI always has the daemon running (`smithers up --serve`). The Crush TUI may run standalone. The 3-tier transport (HTTP → exec → error) has no filesystem-direct path.

**Mitigation**: Not blocking for this ticket. A future enhancement could add a 4th transport tier that uses `os.ReadDir` + `os.ReadFile` on `.smithers/tickets/` directly. The error state UX already handles this gracefully.

### 4. View file placement vs ticket suggestion

**Risk**: The ticket's implementation notes say "Create `internal/ui/tickets/tickets.go`" (new package), but the established pattern puts views in `internal/ui/views/` (see `agents.go`, `router.go`). The file already exists at `internal/ui/views/tickets.go`.

**Mitigation**: Keep the file at `internal/ui/views/tickets.go`. The engineering doc (`03-ENGINEERING.md:108`) confirms this location. The ticket's suggestion is superseded by the actual architecture.

### 5. No `bubbles/list` used (divergence from ticket notes)

**Risk**: The ticket suggests using `bubbles/list`, but the existing `AgentsView` and `TicketsView` scaffold both use manual cursor rendering. Using `bubbles/list` for tickets only would create inconsistency.

**Mitigation**: Keep the manual cursor pattern for consistency with `AgentsView`. The manual approach is simpler, gives full layout control, and avoids a new dependency. If a future refactor adopts `bubbles/list` across all list views, that's a separate concern.

### 6. VHS binary availability in CI

**Risk**: `vhs` must be installed to run `.tape` tests. CI environments may not have it.

**Mitigation**: Gate VHS tests behind a build tag or skip condition. The Go E2E tests provide automated regression coverage; VHS is supplementary for visual verification and can run locally or in dedicated CI jobs.

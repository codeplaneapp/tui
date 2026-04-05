# Implementation Plan: platform-split-pane

## Goal

The `SplitPane` component is already built and tested. This ticket delivers **platform-level integration**: wiring the component into the two existing views that need it (`ApprovalsView`, `TicketsView`), fixing the three root model issues that block proper split-pane behavior, and establishing the shared E2E test infrastructure.

The component itself (`internal/ui/components/splitpane.go`) and its unit tests (`splitpane_test.go`) require no changes.

---

## Pre-work: Verify the Component Tests Pass

Before touching any integration code, confirm the existing unit tests compile and pass:

```bash
go test ./internal/ui/components/... -v -run TestSplitPane
```

All 14 test cases should pass. If any fail, investigate before proceeding — the integration work assumes the component is correct.

---

## Step 1: Fix the Root Model — Three Blockers

**File**: `internal/ui/model/ui.go`

All three fixes are small and self-contained. Land them in a single commit before the view rewrites.

### 1a. Add `uiSmithersView` case to `generateLayout()`

In `generateLayout()` (~line 2668), add a case alongside `uiChat`. The Smithers view layout is simple: a 1-row header at the top, the full remaining area for the active view's content.

```go
case uiSmithersView:
    // Layout:
    //   header (1 row)
    //   ─────────────
    //   main (remaining)
    const smithersHeaderHeight = 1
    headerRect, mainRect := layout.SplitVertical(appRect, layout.Fixed(smithersHeaderHeight))
    uiLayout.header = headerRect
    uiLayout.main = mainRect
```

Place this case before the `default:` / `uiChat` group. This fixes the zero-rect draw bug — the current view's `View()` output is drawn into a properly-sized `layout.main`.

### 1b. Remove duplicate Smithers view dispatch

The `default:` case of the message-type switch (~line 917) forwards all messages to the current Smithers view:

```go
// This block — REMOVE it (lines ~917–929):
if m.state == uiSmithersView {
    if current := m.viewRouter.Current(); current != nil {
        updated, cmd := current.Update(msg)
        ...
    }
}
```

Delete this block. The canonical forwarding already happens in the `uiSmithersView` arm of the state-switch at line ~1823. Having both causes every key message to be processed twice, which neutralizes Tab focus toggles.

After deletion, the `uiSmithersView` state-switch arm at ~1808 remains the single dispatch point.

### 1c. Forward size to the router on `WindowSizeMsg`

In the `case tea.WindowSizeMsg:` handler (~line 664), after `m.updateLayoutAndSize()`, add:

```go
if m.state == uiSmithersView {
    m.viewRouter.SetSize(m.width, m.height)
}
```

`Router.SetSize` does not exist yet. Add it to `internal/ui/views/router.go`:

```go
// SetSize propagates the current terminal dimensions to the active view.
// Call this whenever the terminal is resized while a Smithers view is active.
func (r *Router) SetSize(width, height int) {
    r.width = width
    r.height = height
    if current := r.Current(); current != nil {
        // Views that have a SetSize method get it directly; others receive
        // the WindowSizeMsg via normal Update forwarding.
        type sizer interface{ SetSize(int, int) }
        if s, ok := current.(sizer); ok {
            s.SetSize(width, height)
        }
    }
}
```

Also add `width` and `height` fields to the `Router` struct so they are remembered for when a new view is pushed.

---

## Step 2: Refactor `ApprovalsView` to Use `SplitPane`

**File**: `internal/ui/views/approvals.go`

### 2a. Define private pane types

Add two unexported types above `ApprovalsView`. They implement `components.Pane`.

**`approvalListPane`** — the navigable left pane:

```go
type approvalListPane struct {
    approvals []smithers.Approval
    cursor    int
    width     int
    height    int
}

func (p *approvalListPane) Init() tea.Cmd { return nil }

func (p *approvalListPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
        switch {
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
            if p.cursor > 0 {
                p.cursor--
            }
        case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
            if p.cursor < len(p.approvals)-1 {
                p.cursor++
            }
        }
    }
    return p, nil
}

func (p *approvalListPane) SetSize(width, height int) {
    p.width = width
    p.height = height
}

func (p *approvalListPane) View() string {
    // Same rendering logic as the existing renderList() / renderListItem() methods.
    // Move that logic here verbatim; the width comes from p.width.
    ...
}
```

**`approvalDetailPane`** — the passive right pane:

```go
type approvalDetailPane struct {
    approvals []smithers.Approval
    cursor    *int   // points to the list pane's cursor so detail stays in sync
    width     int
    height    int
}

func (p *approvalDetailPane) Init() tea.Cmd { return nil }

func (p *approvalDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    return p, nil  // detail pane is read-only in v1
}

func (p *approvalDetailPane) SetSize(width, height int) {
    p.width = width
    p.height = height
}

func (p *approvalDetailPane) View() string {
    // Same rendering logic as the existing renderDetail() method.
    ...
}
```

Sharing the cursor via a pointer (or by updating both panes when the cursor changes) is the simplest approach. An alternative is to emit a custom `approvalCursorChangedMsg` from the list pane and handle it in `ApprovalsView.Update` to refresh both panes, but for v1 the pointer approach is sufficient.

### 2b. Rewrite `ApprovalsView`

Replace the flat struct with one that owns a `*components.SplitPane`:

```go
type ApprovalsView struct {
    client    *smithers.Client
    approvals []smithers.Approval
    cursor    int
    width     int
    height    int
    loading   bool
    err       error
    splitPane *components.SplitPane
    listPane  *approvalListPane
    detailPane *approvalDetailPane
}
```

In `NewApprovalsView`, construct the split pane:

```go
func NewApprovalsView(client *smithers.Client) *ApprovalsView {
    listPane := &approvalListPane{}
    detailPane := &approvalDetailPane{cursor: &listPane.cursor}
    sp := components.NewSplitPane(listPane, detailPane, components.SplitPaneOpts{
        LeftWidth:         30,
        CompactBreakpoint: 80,
    })
    return &ApprovalsView{
        client:     client,
        loading:    true,
        splitPane:  sp,
        listPane:   listPane,
        detailPane: detailPane,
    }
}
```

In `Update`:

1. On `approvalsLoadedMsg`: set `v.approvals`, update `v.listPane.approvals` and `v.detailPane.approvals`, call `v.splitPane.SetSize(v.width, v.height)`.
2. On `tea.WindowSizeMsg`: set `v.width`, `v.height`, call `v.splitPane.SetSize(msg.Width, msg.Height)`.
3. On `tea.KeyPressMsg` with Esc: return `PopViewMsg{}` (unchanged).
4. On `tea.KeyPressMsg` with `r`: refresh (unchanged).
5. All other messages: delegate to `v.splitPane.Update(msg)`.

Remove the `case tea.KeyPressMsg` branches for `up`/`down`/`j`/`k` from `ApprovalsView.Update` — those now live inside `approvalListPane.Update` and are routed there by `SplitPane` when the list pane is focused.

In `View()`:

Replace the entire manual split logic with:

```go
func (v *ApprovalsView) View() string {
    header := v.renderHeader()
    if v.loading {
        return header + "\n\n  Loading approvals...\n"
    }
    if v.err != nil {
        return header + fmt.Sprintf("\n\n  Error: %v\n", v.err)
    }
    if len(v.approvals) == 0 {
        return header + "\n\n  No pending approvals.\n"
    }
    // SplitPane renders both panes, divider, focus indicator, and compact fallback.
    return header + "\n" + v.splitPane.View()
}
```

The header is still rendered by `ApprovalsView.View()` itself (1 line), then the split pane fills the rest. The height passed to `SetSize` should be `v.height - 2` (subtract header row + blank line) so the panes fill the available space accurately.

### 2c. Delete the now-redundant private helpers

Remove from `approvals.go`:
- `renderList(width int) string`
- `renderListItem(idx, width int) string`
- `renderListCompact() string`
- `renderDetail(width int) string`
- `padRight(s string, width int) string` — move to `internal/ui/views/helpers.go`
- `truncate(s string, maxLen int) string` — move to helpers
- `formatStatus(status string) string` — move to helpers
- `formatPayload(payload string, width int) string` — move to helpers
- `wrapText(s string, width int) string` — move to helpers

These helpers are also needed by the Tickets detail pane and future views. Centralizing them now pays off immediately.

### 2d. Update `ShortHelp()` to be context-aware

The help bar should show different hints depending on which pane is focused:

```go
func (v *ApprovalsView) ShortHelp() []string {
    if v.splitPane != nil && v.splitPane.Focus() == components.FocusLeft {
        return []string{"[↑↓] Navigate", "[tab] Detail", "[r] Refresh", "[Esc] Back"}
    }
    return []string{"[tab] List", "[Esc] Back"}
}
```

---

## Step 3: Refactor `TicketsView` to Use `SplitPane`

**File**: `internal/ui/views/tickets.go`

### 3a. Define private pane types

**`ticketListPane`** — mirrors the existing rendering logic:

```go
type ticketListPane struct {
    tickets []smithers.Ticket
    cursor  int
    width   int
    height  int
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
        }
    }
    return p, nil
}

func (p *ticketListPane) SetSize(width, height int) { p.width = width; p.height = height }

func (p *ticketListPane) View() string {
    // Render ticket list: ID + snippet per item, cursor indicator.
    // Same logic as existing TicketsView.View() list section.
    ...
}
```

**`ticketDetailPane`** — displays the full ticket content:

```go
type ticketDetailPane struct {
    tickets []smithers.Ticket
    cursor  *int
    width   int
    height  int
}

func (p *ticketDetailPane) Init() tea.Cmd { return nil }

func (p *ticketDetailPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) {
    return p, nil  // read-only in v1; scrolling is a future enhancement
}

func (p *ticketDetailPane) SetSize(width, height int) { p.width = width; p.height = height }

func (p *ticketDetailPane) View() string {
    if len(p.tickets) == 0 || *p.cursor >= len(p.tickets) {
        return ""
    }
    ticket := p.tickets[*p.cursor]
    titleStyle := lipgloss.NewStyle().Bold(true)
    // Render: ticket ID as title, then markdown content wrapped to p.width.
    return titleStyle.Render(ticket.ID) + "\n\n" + wrapText(ticket.Content, p.width)
}
```

### 3b. Rewrite `TicketsView`

Replace the flat struct:

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

Constructor:

```go
func NewTicketsView(client *smithers.Client) *TicketsView {
    listPane := &ticketListPane{}
    detailPane := &ticketDetailPane{cursor: &listPane.cursor}
    sp := components.NewSplitPane(listPane, detailPane, components.SplitPaneOpts{
        LeftWidth:         30,
        CompactBreakpoint: 80,
    })
    return &TicketsView{
        client:     client,
        loading:    true,
        splitPane:  sp,
        listPane:   listPane,
        detailPane: detailPane,
    }
}
```

`Update`: same pattern as ApprovalsView. On `ticketsLoadedMsg`, populate `listPane.tickets` and `detailPane.tickets`. Forward all other non-Esc/non-r messages to `v.splitPane.Update(msg)`.

`View()`: same pattern — header line + `v.splitPane.View()`.

Remove `ticketSnippet` from `tickets.go` and move it to `helpers.go`.

### 3c. Adjust split pane height

When building the `SplitPane.View()` string to return from `TicketsView.View()`, the split pane should occupy `v.height - 2` rows (header + separator = 2 rows). Call `v.splitPane.SetSize(v.width, v.height-2)` whenever the view is resized or data is loaded, not just in `WindowSizeMsg`.

---

## Step 4: Add `internal/ui/views/helpers.go`

**File**: `internal/ui/views/helpers.go` (new)

Collect the string utilities that are used by multiple views:

```go
package views

import (
    "encoding/json"
    "strings"

    "charm.land/lipgloss/v2"
)

// padRight pads s to the given visual width using lipgloss.Width for ANSI safety.
func padRight(s string, width int) string { ... }

// truncate shortens s to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string { ... }

// wrapText wraps plain text to fit within the given column width.
func wrapText(s string, width int) string { ... }

// ticketSnippet returns the first non-heading, non-empty line of markdown content.
func ticketSnippet(content string) string { ... }

// formatStatus returns a styled status string for approval states.
func formatStatus(status string) string { ... }

// formatPayload pretty-prints a JSON payload string, falling back to wrapped text.
func formatPayload(payload string, width int) string { ... }
```

Move the implementations verbatim from `approvals.go` and `tickets.go`.

---

## Step 5: Add Demo View for E2E Testing

**File**: `internal/ui/views/splitpane_demo.go` (new, build-tag guarded)

The E2E tests (Step 6) need a deterministic view that shows a split pane without requiring a live Smithers server. Create a demo view:

```go
//go:build testview

package views

import (
    "charm.land/bubbletea/v2"
    "github.com/charmbracelet/crush/internal/ui/components"
)

// SplitPaneDemoView is a test-only view that renders a split pane with
// static content, used by the E2E test harness and VHS recordings.
type SplitPaneDemoView struct {
    splitPane *components.SplitPane
    width     int
    height    int
}

func NewSplitPaneDemoView() *SplitPaneDemoView {
    left := &staticPane{content: "LEFT PANE\n\nItem 1\nItem 2\nItem 3"}
    right := &staticPane{content: "RIGHT PANE\n\nDetail content here."}
    sp := components.NewSplitPane(left, right, components.SplitPaneOpts{})
    return &SplitPaneDemoView{splitPane: sp}
}

// staticPane is a minimal Pane that renders fixed content.
type staticPane struct {
    content       string
    width, height int
}

func (p *staticPane) Init() tea.Cmd                           { return nil }
func (p *staticPane) Update(msg tea.Msg) (components.Pane, tea.Cmd) { return p, nil }
func (p *staticPane) View() string                            { return p.content }
func (p *staticPane) SetSize(w, h int)                       { p.width = w; p.height = h }

func (v *SplitPaneDemoView) Init() tea.Cmd { return v.splitPane.Init() }
func (v *SplitPaneDemoView) Update(msg tea.Msg) (View, tea.Cmd) {
    if ws, ok := msg.(tea.WindowSizeMsg); ok {
        v.width, v.height = ws.Width, ws.Height
        v.splitPane.SetSize(ws.Width, ws.Height)
        return v, nil
    }
    newSP, cmd := v.splitPane.Update(msg)
    v.splitPane = newSP
    return v, cmd
}
func (v *SplitPaneDemoView) View() string          { return v.splitPane.View() }
func (v *SplitPaneDemoView) Name() string           { return "splitpane-demo" }
func (v *SplitPaneDemoView) ShortHelp() []string    { return []string{"[tab] Switch pane", "[Esc] Back"} }
```

Wire it into the command-line argument handling in `internal/cmd/root.go`: when `--test-view splitpane-demo` is passed, push this view onto the router immediately after startup.

---

## Step 6: E2E Test Infrastructure

**Files**:
- `tests/e2e/tui_helpers_test.go` (new)
- `tests/e2e/splitpane_test.go` (new)

### 6a. `tui_helpers_test.go` — shared test harness

Model on upstream `tui-helpers.ts`. The harness spawns the TUI binary, writes to stdin, and polls ANSI-stripped stdout:

```go
package e2e_test

import (
    "bytes"
    "io"
    "os/exec"
    "regexp"
    "strings"
    "sync"
    "testing"
    "time"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type TUIHarness struct {
    t      *testing.T
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    mu     sync.Mutex
    buf    strings.Builder
}

// LaunchTUI builds the binary (once per test run, cached), spawns it with
// TERM=xterm-256color, and starts reading stdout into an internal buffer.
func LaunchTUI(t *testing.T, args ...string) *TUIHarness { ... }

// WaitForText polls the stdout buffer until text appears or timeout elapses.
// Strips ANSI before comparison. Fails the test on timeout.
func (h *TUIHarness) WaitForText(text string, timeout time.Duration) { ... }

// WaitForNoText polls until text is absent or timeout elapses.
func (h *TUIHarness) WaitForNoText(text string, timeout time.Duration) { ... }

// SendKeys writes raw bytes to stdin (supports \t, \x1b, \r).
func (h *TUIHarness) SendKeys(s string) { ... }

// Snapshot returns the current ANSI-stripped buffer for debugging.
func (h *TUIHarness) Snapshot() string { ... }

// Close kills the process and cleans up.
func (h *TUIHarness) Close() { ... }
```

Build caching: use `sync.Once` + `os.MkdirTemp` to build the binary once per `go test` invocation and reuse across tests in the package.

### 6b. `splitpane_test.go` — split-pane E2E test

```go
func TestSplitPane_E2E_TwoPane(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E in short mode")
    }
    h := LaunchTUI(t, "--test-view", "splitpane-demo")
    defer h.Close()

    // Both panes and divider must render.
    h.WaitForText("LEFT PANE", 10*time.Second)
    h.WaitForText("│", 5*time.Second)
    h.WaitForText("RIGHT PANE", 5*time.Second)

    // Tab switches focus (visual focus indicator moves but content stays).
    h.SendKeys("\t")
    time.Sleep(200 * time.Millisecond)
    h.WaitForText("RIGHT PANE", 2*time.Second)

    // Tab back.
    h.SendKeys("\t")
    time.Sleep(200 * time.Millisecond)
    h.WaitForText("LEFT PANE", 2*time.Second)

    // Esc pops back to the chat/landing view.
    h.SendKeys("\x1b")
    h.WaitForNoText("LEFT PANE", 5*time.Second)
}

func TestSplitPane_E2E_CompactMode(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E in short mode")
    }
    // Launch with a narrow terminal to force compact mode.
    h := LaunchTUI(t, "--test-view", "splitpane-demo", "--width", "60")
    defer h.Close()

    // In compact mode only one pane is visible; the divider should not appear.
    h.WaitForText("LEFT PANE", 10*time.Second)
    h.WaitForNoText("│", 2*time.Second)
    h.WaitForNoText("RIGHT PANE", 2*time.Second)

    // Tab shows the right pane.
    h.SendKeys("\t")
    time.Sleep(200 * time.Millisecond)
    h.WaitForText("RIGHT PANE", 2*time.Second)
    h.WaitForNoText("LEFT PANE", 2*time.Second)
}
```

Run with: `go test ./tests/e2e/... -v -run TestSplitPane_E2E -timeout 60s`

Skip in short mode: `go test ./tests/e2e/... -short` exits immediately.

---

## Step 7: VHS Tape

**File**: `tests/vhs/splitpane.tape` (new)

```tape
# Split Pane — Happy Path
Output tests/vhs/splitpane.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Shell "bash"
Set Theme "Dracula"

Type "go build -o /tmp/crush-splitpane-test . && /tmp/crush-splitpane-test --test-view splitpane-demo"
Enter
Sleep 2s
Screenshot tests/vhs/splitpane_initial.png

Down
Down
Sleep 300ms
Screenshot tests/vhs/splitpane_navigate.png

Tab
Sleep 300ms
Screenshot tests/vhs/splitpane_focus_right.png

Tab
Sleep 300ms
Screenshot tests/vhs/splitpane_focus_left.png

Escape
Sleep 500ms
Screenshot tests/vhs/splitpane_back.png
```

Add a line to `tests/vhs/README.md` documenting the new tape.

---

## Responsive Behavior at Different Terminal Widths

| Terminal width | SplitPane behavior |
|---------------|-------------------|
| >= 80 cols | Normal two-pane mode. Left pane: 30 cols. Right pane: `width - 30 - 1` cols. Divider visible. Both panes rendered. |
| < 80 cols | Compact mode. Only focused pane visible (full width). Divider hidden. Tab swaps which pane is shown. |
| < ~34 cols | Edge case: `clampLeftWidth` caps left at `width / 2`. Right pane may be very narrow. The component handles this without panic; content may truncate. |

The 80-column `CompactBreakpoint` is consistent across both `ApprovalsView` and `TicketsView`. Consumer views should not override this without a good reason, so the behavior is predictable for users who resize their terminals.

The header line (1 row) is always rendered by the parent view outside the split pane. The split pane receives `height - 2` to account for the header and its trailing newline.

---

## Component Enhancements Needed

None. The `SplitPane` component is complete as-is for this ticket's requirements.

The only enhancement considered was a scrollable `approvalDetailPane` (for large payloads), but this is deferred to a future iteration. The `Pane` interface's `Update(msg tea.Msg) (Pane, tea.Cmd)` method is already in place to accept scroll key events when that feature is added.

---

## Testing Strategy

| Test type | Command | Coverage |
|-----------|---------|---------|
| Component unit tests | `go test ./internal/ui/components/... -v -run TestSplitPane` | 14 cases: layout math, compact mode, Tab routing, focus accessors, border rendering |
| View unit tests | `go test ./internal/ui/views/... -v` | `approvalsLoadedMsg` populates panes; `WindowSizeMsg` propagates to split pane; Esc emits `PopViewMsg`; `View()` renders split output; compact mode renders single pane |
| E2E terminal tests | `go test ./tests/e2e/... -v -run TestSplitPane_E2E -timeout 60s` | Two-pane render, Tab focus toggle, compact mode at narrow width, Esc back-navigation |
| VHS recording | `vhs tests/vhs/splitpane.tape` | Visual confirmation of all states |

---

## File Plan

| File | Change |
|------|--------|
| `internal/ui/model/ui.go` | Add `uiSmithersView` layout case; remove duplicate dispatch block; add `viewRouter.SetSize` call on `WindowSizeMsg` |
| `internal/ui/views/router.go` | Add `width`/`height` fields to `Router`; add `SetSize(width, height int)` method |
| `internal/ui/views/helpers.go` | New — `padRight`, `truncate`, `wrapText`, `ticketSnippet`, `formatStatus`, `formatPayload` |
| `internal/ui/views/approvals.go` | Add `approvalListPane` and `approvalDetailPane`; rewrite `ApprovalsView` to compose `SplitPane`; delete manual split logic and moved helpers |
| `internal/ui/views/tickets.go` | Add `ticketListPane` and `ticketDetailPane`; rewrite `TicketsView` to compose `SplitPane`; delete moved helpers |
| `internal/ui/views/splitpane_demo.go` | New (build tag `testview`) — demo view for E2E/VHS testing |
| `internal/cmd/root.go` | Add `--test-view <name>` flag to push a named test view at startup |
| `tests/e2e/tui_helpers_test.go` | New — `TUIHarness`, `LaunchTUI`, `WaitForText`, `WaitForNoText`, `SendKeys`, `Snapshot`, `Close` |
| `tests/e2e/splitpane_test.go` | New — E2E tests: two-pane render, Tab focus, compact mode, Esc back |
| `tests/vhs/splitpane.tape` | New — VHS happy-path tape |
| `tests/vhs/README.md` | Update — add splitpane entry |
| `internal/ui/components/splitpane.go` | No changes needed |
| `internal/ui/components/splitpane_test.go` | No changes needed |

---

## Open Questions

1. **`View` interface `ShortHelp` type**: Currently `ShortHelp() []string`. The `platform-view-model` plan upgrades it to `[]key.Binding`. If that plan lands first, the new `ShortHelp` implementations here should return `[]key.Binding`. If this ticket lands first, use `[]string` and plan a follow-up migration.

2. **Cursor sharing via pointer vs. message**: The shared `cursor *int` between list and detail pane is simple but couples the two pane types. If the panes ever become independent of each other (e.g., the detail pane shows a fixed item while the list navigates), a message-passing approach is cleaner. For v1 the pointer is fine.

3. **Demo view build tag**: Using `//go:build testview` keeps the demo out of production builds. An alternative is a `--debug` flag that is always compiled in but gated at runtime. The build-tag approach is cleaner for binary size but requires building with `-tags testview` for E2E tests. Ensure the E2E harness's `go build` invocation includes `-tags testview`.

4. **`--width` flag for compact-mode E2E test**: Forcing the terminal to a specific width via a CLI flag requires the TUI to accept an initial size override rather than reading from the TTY. This is optional — if not implemented, the compact-mode E2E test can be skipped and validated manually instead.

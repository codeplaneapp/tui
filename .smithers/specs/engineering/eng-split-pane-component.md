# Engineering Spec: Shared Split Pane Layout Component

**Ticket**: `eng-split-pane-component`
**Feature**: `PLATFORM_SPLIT_PANE_LAYOUTS`
**Status**: Draft
**Date**: 2026-04-02

---

## Objective

Implement a reusable Bubble Tea component (`internal/ui/components/splitpane.go`) that renders two arbitrary child views side-by-side in a fixed-left / responsive-right layout. This component is a shared building block consumed by at least four Smithers TUI views: Tickets (list + detail), Prompts (list + source + preview), SQL Browser (table sidebar + query/results), and Node Inspector (node list + task tabs). It must integrate cleanly with Crush's existing ultraviolet-based rectangle layout system and the `views.View` interface in `internal/ui/views/router.go`.

---

## Scope

### In Scope

- A `SplitPane` struct in `internal/ui/components/splitpane.go` that composes two child `tea.Model`-compatible panes.
- Fixed-width left pane (configurable at construction, default 30 columns) with a responsive right pane that fills remaining width.
- Focus management: track which pane is active, route key/mouse events to the focused pane, allow `Tab` to toggle focus.
- A vertical divider gutter (1-column wide, rendered as `│` characters) between panes.
- Terminal resize handling: recalculate pane widths on `tea.WindowSizeMsg`.
- Compact-mode fallback: when terminal width is below a configurable breakpoint, collapse to single-pane mode showing only the focused pane (mirroring how `internal/ui/model/ui.go` switches to compact mode at `compactModeWidthBreakpoint = 120`).
- Proper integration with Crush's ultraviolet `Draw(scr uv.Screen, area uv.Rectangle)` rendering pattern.

### Out of Scope

- Draggable resize handle (upstream Smithers GUI uses static CSS widths with no drag resize; we follow suit).
- Vertical (top/bottom) split — only horizontal (left/right) split for v1.
- Three-pane layouts — the Prompts view's three-column layout will compose two nested `SplitPane` instances.

---

## Implementation Plan

### Slice 1: Core `SplitPane` struct and constructor

**File**: `internal/ui/components/splitpane.go` (new)

Create the package `internal/ui/components` and the `SplitPane` type.

```go
package components

import (
    tea "charm.land/bubbletea/v2"
    uv "github.com/charmbracelet/ultraviolet"
    "github.com/charmbracelet/ultraviolet/layout"
    "charm.land/lipgloss/v2"
)

// Pane is the interface child views must satisfy.
// It extends tea.Model with Draw for ultraviolet rendering
// and SetSize for resize propagation.
type Pane interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    SetSize(width, height int)
}

// FocusSide indicates which pane is focused.
type FocusSide int

const (
    FocusLeft  FocusSide = iota
    FocusRight
)

// SplitPaneOpts configures a SplitPane.
type SplitPaneOpts struct {
    LeftWidth         int  // Fixed left pane width in columns (default: 30)
    DividerWidth      int  // Divider gutter width (default: 1)
    CompactBreakpoint int  // Below this total width, collapse to single pane (default: 80)
}

// SplitPane renders two panes side-by-side.
type SplitPane struct {
    left          Pane
    right         Pane
    focus         FocusSide
    opts          SplitPaneOpts
    width, height int
    compact       bool // true when collapsed to single pane
}
```

**Key design decisions**:

1. **`Pane` interface vs `views.View`**: The `Pane` interface is intentionally more minimal than `views.View`. Views like `TicketsView` will implement both `View` (for the router) and internally compose a `SplitPane` whose children implement `Pane`. This avoids coupling the split-pane to the router.

2. **Fixed-left width mirrors GUI**: The upstream Smithers GUI uses `w-64` (256px, ~32 terminal columns at standard font) and `w-72` (288px, ~36 columns) for sidebars. We default to 30 columns, matching Crush's existing `sidebarWidth` constant in `internal/ui/model/ui.go:2534`.

3. **Constructor**:

```go
func NewSplitPane(left, right Pane, opts SplitPaneOpts) *SplitPane {
    if opts.LeftWidth == 0 {
        opts.LeftWidth = 30
    }
    if opts.DividerWidth == 0 {
        opts.DividerWidth = 1
    }
    if opts.CompactBreakpoint == 0 {
        opts.CompactBreakpoint = 80
    }
    return &SplitPane{
        left:  left,
        right: right,
        focus: FocusLeft,
        opts:  opts,
    }
}
```

### Slice 2: Layout calculation and `SetSize`

Implement size propagation using ultraviolet's `layout.SplitHorizontal`, following the exact pattern used in `internal/ui/model/ui.go:2642`:

```go
func (sp *SplitPane) SetSize(width, height int) {
    sp.width = width
    sp.height = height
    sp.compact = width < sp.opts.CompactBreakpoint

    if sp.compact {
        // Single-pane mode: give all space to focused pane
        switch sp.focus {
        case FocusLeft:
            sp.left.SetSize(width, height)
        case FocusRight:
            sp.right.SetSize(width, height)
        }
        return
    }

    leftWidth := min(sp.opts.LeftWidth, width/2) // Never exceed half width
    dividerWidth := sp.opts.DividerWidth
    rightWidth := width - leftWidth - dividerWidth

    sp.left.SetSize(leftWidth, height)
    sp.right.SetSize(rightWidth, height)
}
```

**Compact mode**: When the terminal is narrower than `CompactBreakpoint`, collapse to show only the focused pane. This mirrors how `internal/ui/model/ui.go` hides the sidebar in compact mode (below `compactModeWidthBreakpoint = 120`). The user toggles between panes with Tab.

### Slice 3: `Update` with focus routing and Tab toggle

```go
func (sp *SplitPane) Init() tea.Cmd {
    return tea.Batch(sp.left.Init(), sp.right.Init())
}

func (sp *SplitPane) Update(msg tea.Msg) (*SplitPane, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        sp.SetSize(msg.Width, msg.Height)
        return sp, nil

    case tea.KeyPressMsg:
        if key.Matches(msg, key.NewBinding(key.WithKeys("tab"))) {
            sp.toggleFocus()
            return sp, nil
        }
    }

    // Route message to focused pane
    var cmd tea.Cmd
    switch sp.focus {
    case FocusLeft:
        newLeft, c := sp.left.Update(msg)
        sp.left = newLeft
        cmd = c
    case FocusRight:
        newRight, c := sp.right.Update(msg)
        sp.right = newRight
        cmd = c
    }
    return sp, cmd
}

func (sp *SplitPane) toggleFocus() {
    if sp.focus == FocusLeft {
        sp.focus = FocusRight
    } else {
        sp.focus = FocusLeft
    }
    // Re-propagate sizes in compact mode (swaps which pane gets space)
    if sp.compact {
        sp.SetSize(sp.width, sp.height)
    }
}
```

### Slice 4: `View` rendering with divider

Two rendering paths depending on whether the host view uses ultraviolet `Draw` or string-based `View()`:

**String-based `View()` (for initial integration)**:

```go
func (sp *SplitPane) View() string {
    if sp.compact {
        switch sp.focus {
        case FocusLeft:
            return sp.left.View()
        case FocusRight:
            return sp.right.View()
        }
    }

    leftWidth := min(sp.opts.LeftWidth, sp.width/2)
    dividerWidth := sp.opts.DividerWidth

    leftContent := sp.left.View()
    rightContent := sp.right.View()

    // Constrain each pane's rendered output to its allocated width
    leftStyled := lipgloss.NewStyle().
        Width(leftWidth).
        MaxWidth(leftWidth).
        Height(sp.height).
        Render(leftContent)

    divider := sp.renderDivider()

    rightWidth := sp.width - leftWidth - dividerWidth
    rightStyled := lipgloss.NewStyle().
        Width(rightWidth).
        MaxWidth(rightWidth).
        Height(sp.height).
        Render(rightContent)

    return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, divider, rightStyled)
}

func (sp *SplitPane) renderDivider() string {
    style := lipgloss.NewStyle().
        Foreground(lipgloss.Color("240")).
        Width(sp.opts.DividerWidth).
        Height(sp.height)
    return style.Render(strings.Repeat("│\n", sp.height))
}
```

**Ultraviolet `Draw` (for views that use the screen-based renderer)**:

```go
func (sp *SplitPane) Draw(scr uv.Screen, area uv.Rectangle) {
    if sp.compact {
        switch sp.focus {
        case FocusLeft:
            sp.drawPane(scr, area, sp.left)
        case FocusRight:
            sp.drawPane(scr, area, sp.right)
        }
        return
    }

    leftWidth := min(sp.opts.LeftWidth, area.Dx()/2)
    leftRect, remainder := layout.SplitHorizontal(area, layout.Fixed(leftWidth))
    dividerRect, rightRect := layout.SplitHorizontal(remainder, layout.Fixed(sp.opts.DividerWidth))

    sp.drawPane(scr, leftRect, sp.left)
    sp.drawDivider(scr, dividerRect)
    sp.drawPane(scr, rightRect, sp.right)
}
```

### Slice 5: Focus indicator styling

Add visual focus indicators so users know which pane is active:

- The focused pane's divider border uses the accent color (from `internal/ui/styles/styles.go`).
- In compact mode, render a small breadcrumb at the top: `[List] | Detail` with the focused side highlighted.

```go
func (sp *SplitPane) renderDividerStyled(styles *styles.Styles) string {
    color := lipgloss.Color("240") // dim
    if sp.focus == FocusLeft {
        // Divider adjacent to focused pane uses accent
    }
    // The divider itself is a neutral separator; focus is indicated
    // by bold/highlight on the focused pane's header, not the divider.
    _ = color
    return sp.renderDivider()
}
```

### Slice 6: Unit tests

**File**: `internal/ui/components/splitpane_test.go` (new)

```go
package components_test

// Test cases:
// 1. NewSplitPane defaults: LeftWidth=30, DividerWidth=1, CompactBreakpoint=80
// 2. SetSize propagates correct widths to children
// 3. SetSize with width < CompactBreakpoint enters compact mode
// 4. Tab key toggles focus between left and right
// 5. Key events route only to the focused pane
// 6. WindowSizeMsg recalculates layout
// 7. View() output has correct column widths (measure with lipgloss.Width)
// 8. Compact mode View() renders only the focused pane
// 9. LeftWidth clamped to max width/2 (prevents left pane exceeding half)
// 10. Init() calls both children's Init()
```

Each test uses mock `Pane` implementations that record `SetSize` calls and `Update` messages for assertion.

### Slice 7: Integration with existing view system

The `SplitPane` is consumed by views, not by the router directly. Example integration pattern for a future `TicketsView`:

```go
// internal/ui/views/tickets.go (future, not part of this ticket)
type TicketsView struct {
    splitPane *components.SplitPane
    // ...
}

func NewTicketsView(client *smithers.Client) *TicketsView {
    list := newTicketListPane(client)
    detail := newTicketDetailPane()
    sp := components.NewSplitPane(list, detail, components.SplitPaneOpts{
        LeftWidth: 36, // matches GUI's w-72 ≈ 36 terminal columns
    })
    return &TicketsView{splitPane: sp}
}

func (v *TicketsView) Update(msg tea.Msg) (views.View, tea.Cmd) {
    newSP, cmd := v.splitPane.Update(msg)
    v.splitPane = newSP
    return v, cmd
}

func (v *TicketsView) View() string {
    return v.splitPane.View()
}
```

This pattern ensures each consumer can customize `LeftWidth` (30 for SQL browser, 36 for tickets/prompts) and provide its own `Pane` implementations.

---

## Validation

### Unit Tests

Run from the repo root:

```bash
go test ./internal/ui/components/... -v -run TestSplitPane
```

Expected test cases (see Slice 6):
- `TestSplitPane_Defaults` — verifies constructor defaults
- `TestSplitPane_SetSize_Normal` — child panes receive correct widths
- `TestSplitPane_SetSize_Compact` — compact mode activates below breakpoint
- `TestSplitPane_TabTogglesFocus` — Tab routes focus and re-propagates size in compact mode
- `TestSplitPane_KeyRouting` — only focused pane receives key events
- `TestSplitPane_WindowResize` — `tea.WindowSizeMsg` triggers `SetSize`
- `TestSplitPane_ViewOutput` — rendered string has expected column structure
- `TestSplitPane_LeftWidthClamped` — left pane never exceeds half of total width

### Terminal E2E Tests (modeled on upstream `@microsoft/tui-test` harness)

The upstream Smithers E2E harness (`../smithers/tests/tui.e2e.test.ts` + `../smithers/tests/tui-helpers.ts`) uses a `BunSpawnBackend` that:
1. Spawns the TUI process with `stdin: "pipe"`, `stdout: "pipe"`
2. Polls stdout for text via `waitForText(string, timeoutMs?)` (100ms poll interval, 10s default timeout)
3. Sends keystrokes via `sendKeys(text)` (supports escape sequences: `\x1b` for Esc, `\r` for Enter)
4. Strips ANSI with regex before text matching
5. Normalizes whitespace for tolerance against reflow

**Crush Go equivalent**: Create `tests/e2e/splitpane_e2e_test.go` that follows the same pattern using Go's `os/exec` to spawn the Crush binary and `bufio.Scanner` to read stdout:

```go
// tests/e2e/splitpane_e2e_test.go
func TestSplitPane_E2E(t *testing.T) {
    tui := launchTUI(t, []string{"--view", "tickets"})  // or a test harness view
    defer tui.Terminate()

    // 1. Verify split pane renders both panes
    tui.WaitForText("Tickets", 10*time.Second)      // Left pane header
    tui.WaitForText("│", 5*time.Second)              // Divider visible
    // Right pane content depends on selected item

    // 2. Tab toggles focus
    tui.SendKeys("\t")
    // Verify focus shifted (e.g., detail pane now accepts input)

    // 3. Resize: send SIGWINCH or use reduced terminal size
    // Verify compact mode by checking divider disappears at small width

    // 4. Esc pops back to chat
    tui.SendKeys("\x1b")
    tui.WaitForText("Ready...", 5*time.Second)
}
```

The Go E2E helpers should mirror the key patterns from `tui-helpers.ts`:
- `WaitForText(text string, timeout time.Duration)` — polls ANSI-stripped stdout buffer every 100ms
- `WaitForNoText(text string, timeout time.Duration)` — polls until text absent
- `SendKeys(text string)` — writes to stdin pipe
- `Snapshot() string` — returns current stdout buffer with ANSI stripped
- `Terminate()` — kills process

**File**: `tests/e2e/helpers_test.go` (shared Go E2E test helpers)

### VHS Happy-Path Recording Test

Create a VHS tape file that exercises the split-pane layout end-to-end:

**File**: `tests/vhs/splitpane.tape`

```
# Split Pane Component — Happy Path
Output tests/vhs/splitpane.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Shell "bash"
Set Theme "Dracula"

# Launch Smithers TUI and navigate to tickets (split-pane view)
Type "smithers-tui"
Enter
Sleep 2s

# Navigate to tickets view
Type "/tickets"
Enter
Sleep 1s

# Verify split pane: list on left, detail on right
Screenshot tests/vhs/splitpane_initial.png

# Select a ticket
Down
Down
Enter
Sleep 500ms
Screenshot tests/vhs/splitpane_selected.png

# Tab to right pane
Tab
Sleep 300ms
Screenshot tests/vhs/splitpane_focus_right.png

# Tab back to left pane
Tab
Sleep 300ms

# Back to chat
Escape
Sleep 500ms
Screenshot tests/vhs/splitpane_back.png
```

Run with:

```bash
vhs tests/vhs/splitpane.tape
```

Verify the generated GIF and screenshots show:
1. Two-pane layout with visible divider
2. Left pane showing a list, right pane showing detail content
3. Focus visually shifts when Tab is pressed
4. Navigation back to chat works

### Manual Verification

1. `go build -o smithers-tui . && ./smithers-tui` — verify startup
2. Navigate to any view using the split pane (tickets, SQL)
3. Resize terminal window — verify panes reflow; at narrow width (<80 cols), only one pane visible
4. Press Tab — verify focus toggles; in compact mode, verify pane swap
5. Press Esc — verify back navigation works from within split-pane views

---

## Risks

### 1. `Pane` interface compatibility with existing `views.View`

**Risk**: The `Pane` interface defines `Update(msg) (Pane, tea.Cmd)` which returns `Pane`, while `views.View` returns `View`. Child panes that also implement `View` need adapter glue.

**Mitigation**: Keep `Pane` deliberately minimal and separate from `View`. Views that contain a `SplitPane` (like `TicketsView`) implement `View` themselves and delegate internally to the split pane. The `Pane` implementations are private to each view package, not registered with the router.

### 2. Ultraviolet `Draw` vs string-based `View()` duality

**Risk**: Crush uses ultraviolet's screen-based `Draw(scr, rect)` for the main model but existing Smithers views (like `AgentsView`) use string-based `View()`. The split pane must support both paths.

**Mitigation**: Implement both `View() string` (using `lipgloss.JoinHorizontal`) and `Draw(scr uv.Screen, area uv.Rectangle)` (using `layout.SplitHorizontal`). String-based rendering is the initial default; views can migrate to `Draw` as needed. The engineering doc's architecture (`internal/ui/model/ui.go:2041`) shows the main model using `Draw`; view content rendered via `View()` is drawn into an `uv.StyledString` within the allocated rectangle.

### 3. Compact mode breakpoint conflicts

**Risk**: The root `UI` model already has its own compact breakpoint at 120 columns. If the split-pane's compact breakpoint (80) triggers while the root model is in normal mode, layout could be inconsistent.

**Mitigation**: The split pane's `CompactBreakpoint` applies to its own allocated rectangle width (not the full terminal width). Since the split pane is rendered inside a view's allocated area (which is already reduced by the root model's sidebar, editor, etc.), the effective breakpoint is relative. Views should set the `CompactBreakpoint` based on their own minimum viable two-pane width, typically `leftWidth * 2 + dividerWidth`.

### 4. No `internal/ui/components/` directory exists yet

**Risk**: This is the first file in a new package. Import cycles could arise if components need styles or common utilities.

**Mitigation**: The `components` package should depend only on external libraries (`lipgloss`, `bubbletea`, `ultraviolet`) and optionally `internal/ui/styles` for theme colors. It must NOT import `internal/ui/model` or `internal/ui/views` to avoid cycles. Views import components, not the reverse.

### 5. Mismatch: Crush's existing sidebar layout vs split-pane

**Risk**: Crush already has a fixed-width sidebar (30 cols, right-aligned) in its chat layout (`internal/ui/model/ui.go:2642`). The new `SplitPane` component has a fixed-width left pane. These are different layout patterns that could confuse contributors.

**Mitigation**: The split pane is a self-contained component used only within Smithers views (tickets, SQL, prompts, etc.), not as a replacement for the root-level chat sidebar. Document this clearly in the component's godoc: "SplitPane is for view-level master-detail layouts, not for the root UI sidebar."
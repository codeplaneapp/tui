# Platform: Split Pane Layouts — Engineering Specification

## Metadata
- ID: platform-split-pane
- Group: Platform And Navigation (platform-and-navigation)
- Type: feature
- Feature: PLATFORM_SPLIT_PANE_LAYOUTS
- Dependencies: none

---

## Objective

Implement a reusable Bubble Tea split-pane component (`internal/ui/components/splitpane.go`) that renders two arbitrary child views side-by-side with a configurable fixed-width left pane and a responsive right pane. This component is a shared building block consumed by at least four Smithers TUI views: Tickets (list + detail), Prompts (list + source/preview), SQL Browser (table sidebar + query/results), and Node Inspector (node list + task tabs). It must integrate with Crush's existing ultraviolet-based rectangle layout system used in `internal/ui/model/ui.go` and the `views.View` interface in `internal/ui/views/router.go`.

---

## Scope

### In Scope

- A `SplitPane` struct in `internal/ui/components/splitpane.go` that composes two child panes implementing a `Pane` interface.
- Fixed-width left pane (configurable, default 30 columns) with a responsive right pane filling remaining width.
- Focus management: track which pane is active, route key/mouse events to the focused pane, `Tab` toggles focus.
- A 1-column vertical divider gutter (`│`) between panes.
- Terminal resize handling: recalculate pane widths on `tea.WindowSizeMsg`.
- Compact-mode fallback: when the component's allocated width falls below a configurable breakpoint, collapse to single-pane mode showing only the focused pane.
- Both `View() string` rendering (via `lipgloss.JoinHorizontal`) and `Draw(scr uv.Screen, area uv.Rectangle)` rendering (via `layout.SplitHorizontal`) for integration with Crush's ultraviolet screen buffer.
- Unit tests with mock panes.
- Terminal E2E tests modeled on the upstream `@microsoft/tui-test` harness.
- A VHS happy-path recording test.

### Out of Scope

- Draggable resize handle (upstream Smithers GUI uses static CSS widths; we follow suit).
- Vertical (top/bottom) split — only horizontal (left/right) for v1.
- Three-pane layouts — the Prompts view will compose two nested `SplitPane` instances.
- Consumer views (Tickets, SQL, etc.) — they are separate tickets that depend on this component.

---

## Implementation Plan

### Slice 1: `Pane` interface and `SplitPane` struct

**File**: `internal/ui/components/splitpane.go` (new)

Create the `internal/ui/components` package. Define the `Pane` interface that child views must satisfy, and the `SplitPane` struct with its constructor.

```go
package components

import (
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    uv "github.com/charmbracelet/ultraviolet"
    "github.com/charmbracelet/ultraviolet/layout"
)

// Pane is the interface child views must satisfy.
type Pane interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    SetSize(width, height int)
}

type FocusSide int

const (
    FocusLeft  FocusSide = iota
    FocusRight
)

type SplitPaneOpts struct {
    LeftWidth         int // Fixed left pane width (default: 30)
    DividerWidth      int // Divider gutter width (default: 1)
    CompactBreakpoint int // Collapse threshold (default: 80)
}

type SplitPane struct {
    left, right   Pane
    focus         FocusSide
    opts          SplitPaneOpts
    width, height int
    compact       bool
}
```

**Design decisions**:

1. **`Pane` vs `views.View`**: The `Pane` interface is intentionally more minimal than `views.View` (no `Name()`, no `ShortHelp()`). Views like `TicketsView` implement `views.View` for the router and internally compose a `SplitPane` whose children implement `Pane`. This avoids coupling the layout component to the router.

2. **Fixed-left width mirrors GUI**: The upstream Smithers GUI uses `w-64` / `w-72` Tailwind classes for sidebars (~30-36 terminal columns). Crush's existing sidebar in `internal/ui/model/ui.go:2534` uses `sidebarWidth := 30`. Default to 30.

3. **Import constraints**: The `components` package depends only on `lipgloss`, `bubbletea`, `ultraviolet`, and `charm.land/bubbles/v2/key`. It must NOT import `internal/ui/model`, `internal/ui/views`, or `internal/ui/styles` to prevent import cycles. Style colors are passed in via opts or by the consumer at render time.

**Constructor**:

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

### Slice 2: `SetSize` and layout calculation

The `SetSize` method propagates dimensions to children, following the same `layout.SplitHorizontal` pattern used at `internal/ui/model/ui.go:2642`.

```go
func (sp *SplitPane) SetSize(width, height int) {
    sp.width = width
    sp.height = height
    sp.compact = width < sp.opts.CompactBreakpoint

    if sp.compact {
        // Single-pane mode: all space to focused pane
        switch sp.focus {
        case FocusLeft:
            sp.left.SetSize(width, height)
        case FocusRight:
            sp.right.SetSize(width, height)
        }
        return
    }

    leftWidth := min(sp.opts.LeftWidth, width/2)
    rightWidth := width - leftWidth - sp.opts.DividerWidth
    sp.left.SetSize(leftWidth, height)
    sp.right.SetSize(rightWidth, height)
}
```

The left pane width is clamped to `width/2` so it never exceeds half the available space. This prevents degenerate layouts on very narrow terminals that are still above the compact breakpoint.

### Slice 3: `Init` and `Update` with focus routing

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
            sp.ToggleFocus()
            return sp, nil
        }
    }

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

func (sp *SplitPane) ToggleFocus() {
    if sp.focus == FocusLeft {
        sp.focus = FocusRight
    } else {
        sp.focus = FocusLeft
    }
    if sp.compact {
        sp.SetSize(sp.width, sp.height) // swap which pane gets space
    }
}
```

Key messages route only to the focused pane. `tea.WindowSizeMsg` is handled by the split pane itself and NOT forwarded to children (they receive sizes via `SetSize`).

### Slice 4: `View()` string-based rendering

The primary rendering path for initial integration. Uses `lipgloss.JoinHorizontal`, consistent with patterns at `internal/ui/model/landing.go:41` and `internal/ui/model/pills.go:263`.

```go
func (sp *SplitPane) View() string {
    if sp.compact {
        switch sp.focus {
        case FocusLeft:
            return sp.left.View()
        default:
            return sp.right.View()
        }
    }

    leftWidth := min(sp.opts.LeftWidth, sp.width/2)
    rightWidth := sp.width - leftWidth - sp.opts.DividerWidth

    leftStyled := lipgloss.NewStyle().
        Width(leftWidth).MaxWidth(leftWidth).Height(sp.height).
        Render(sp.left.View())

    divider := sp.renderDivider()

    rightStyled := lipgloss.NewStyle().
        Width(rightWidth).MaxWidth(rightWidth).Height(sp.height).
        Render(sp.right.View())

    return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, divider, rightStyled)
}

func (sp *SplitPane) renderDivider() string {
    divider := strings.Repeat("│\n", max(0, sp.height-1)) + "│"
    return lipgloss.NewStyle().
        Foreground(lipgloss.Color("240")).
        Width(sp.opts.DividerWidth).
        Render(divider)
}
```

### Slice 5: `Draw()` ultraviolet-based rendering

For views that use Crush's screen-buffer renderer (the path taken in `internal/ui/model/ui.go:2097-2102` for `uiSmithersView`). Currently the `uiSmithersView` case calls `current.View()` and wraps it in `uv.NewStyledString`, so the `View()` path is the initial integration point. The `Draw` method is provided for future migration when views need direct screen-buffer control.

```go
func (sp *SplitPane) Draw(scr uv.Screen, area uv.Rectangle) {
    if sp.compact {
        switch sp.focus {
        case FocusLeft:
            uv.NewStyledString(sp.left.View()).Draw(scr, area)
        case FocusRight:
            uv.NewStyledString(sp.right.View()).Draw(scr, area)
        }
        return
    }

    leftWidth := min(sp.opts.LeftWidth, area.Dx()/2)
    leftRect, remainder := layout.SplitHorizontal(area, layout.Fixed(leftWidth))
    dividerRect, rightRect := layout.SplitHorizontal(remainder, layout.Fixed(sp.opts.DividerWidth))

    uv.NewStyledString(sp.left.View()).Draw(scr, leftRect)
    sp.drawDivider(scr, dividerRect)
    uv.NewStyledString(sp.right.View()).Draw(scr, rightRect)
}
```

### Slice 6: Public accessors

Expose read-only accessors so consumer views can query state for conditional rendering:

```go
func (sp *SplitPane) Focus() FocusSide    { return sp.focus }
func (sp *SplitPane) IsCompact() bool      { return sp.compact }
func (sp *SplitPane) Left() Pane           { return sp.left }
func (sp *SplitPane) Right() Pane          { return sp.right }
func (sp *SplitPane) SetFocus(f FocusSide) { sp.focus = f }
```

These allow consumer views to render focus-dependent help text in `ShortHelp()` (e.g., showing different key hints depending on which pane is active).

### Slice 7: Integration with the view router

The `SplitPane` is consumed by views, not the router. The pattern is:

```go
// Example: internal/ui/views/tickets.go (future ticket, not built here)
type TicketsView struct {
    splitPane *components.SplitPane
    width, height int
}

func (v *TicketsView) Update(msg tea.Msg) (View, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        v.width, v.height = msg.Width, msg.Height
        v.splitPane.SetSize(msg.Width, msg.Height)
        return v, nil
    }
    newSP, cmd := v.splitPane.Update(msg)
    v.splitPane = newSP
    return v, cmd
}

func (v *TicketsView) View() string {
    return v.splitPane.View()
}
```

The view receives `tea.WindowSizeMsg` from the root model (at `internal/ui/model/ui.go:2097-2102`, where `uiSmithersView` delegates to `current.View()`), calls `SetSize` on the split pane, and the split pane propagates to children. The root model's `generateLayout` function at line 2559 does NOT currently have a `uiSmithersView` layout case — it falls through with only `header` and `status` set. This means views rendered via the router get the full `appRect` minus margins. The split pane's `SetSize` should be called with the dimensions from the `WindowSizeMsg` that the root model forwards.

**Mismatch to address**: The root `UI.generateLayout()` method (line 2564) has cases for `uiOnboarding`, `uiInitialize`, `uiLanding`, and `uiChat`, but no explicit `uiSmithersView` case. This means `uiSmithersView` gets only `area` and `status` set in the layout, and the Draw path at line 2097 uses `layout.header` and `layout.main` which are zero-valued since no layout case sets them. This needs a `uiSmithersView` case added to `generateLayout()`. This is outside the scope of this ticket but is a prerequisite — tracked in Risks.

### Slice 8: Unit tests

**File**: `internal/ui/components/splitpane_test.go` (new)

Tests use mock `Pane` implementations that record calls:

```go
type mockPane struct {
    initCalled   bool
    updateMsgs   []tea.Msg
    sizeW, sizeH int
    viewContent  string
}

func (p *mockPane) Init() tea.Cmd                      { p.initCalled = true; return nil }
func (p *mockPane) Update(msg tea.Msg) (Pane, tea.Cmd) { p.updateMsgs = append(p.updateMsgs, msg); return p, nil }
func (p *mockPane) View() string                       { return p.viewContent }
func (p *mockPane) SetSize(w, h int)                   { p.sizeW, p.sizeH = w, h }
```

**Test cases**:

| Test | Asserts |
|------|--------|
| `TestSplitPane_Defaults` | Constructor sets LeftWidth=30, DividerWidth=1, CompactBreakpoint=80, FocusLeft |
| `TestSplitPane_SetSize_Normal` | At width=120: left gets 30, right gets 89 (120 - 30 - 1) |
| `TestSplitPane_SetSize_Compact` | At width=70 (<80): only focused pane gets SetSize(70, h) |
| `TestSplitPane_LeftWidthClamped` | At width=50: left gets 25 (50/2), not 30 |
| `TestSplitPane_TabTogglesFocus` | Send Tab KeyPressMsg: focus flips; in compact mode, SetSize re-called on new focused pane |
| `TestSplitPane_KeyRouting` | Key events route only to focused pane; non-focused pane's updateMsgs is empty |
| `TestSplitPane_WindowResize` | `tea.WindowSizeMsg` triggers SetSize on both children |
| `TestSplitPane_ViewOutput_Normal` | `lipgloss.Width(sp.View())` equals configured total width |
| `TestSplitPane_ViewOutput_Compact` | Compact mode View() returns only focused pane content |
| `TestSplitPane_Init` | Init() calls both children's Init() |

Run: `go test ./internal/ui/components/... -v -run TestSplitPane`

---

## Validation

### Unit Tests

```bash
go test ./internal/ui/components/... -v -run TestSplitPane
```

All 10 test cases listed in Slice 8 must pass. Each test uses mock `Pane` implementations that record `SetSize` dimensions and `Update` messages for assertion.

### Terminal E2E Tests (modeled on upstream `@microsoft/tui-test` harness)

The upstream Smithers E2E harness (`smithers_tmp/tests/tui.e2e.test.ts` + `smithers_tmp/tests/tui-helpers.ts`) uses a `BunSpawnBackend` that:

1. Spawns the TUI with `stdin: "pipe"`, `stdout: "pipe"`, `stderr: "pipe"`
2. Reads stdout into a buffer, strips ANSI via `\x1B\[[0-9;]*[a-zA-Z]` regex
3. `waitForText(text, timeout)` — polls buffer every 100ms for up to 10s (default)
4. `waitForNoText(text, timeout)` — polls until text absent
5. `sendKeys(text)` — writes raw bytes to stdin pipe (supports `\x1b` for Esc, `\r` for Enter)
6. `snapshot()` — returns ANSI-stripped buffer for debugging
7. Normalizes whitespace for tolerance against reflow (`compact()` helper collapses `\s+`)

Create a Go equivalent in `tests/e2e/helpers_test.go`:

```go
package e2e_test

import (
    "io"
    "os/exec"
    "regexp"
    "strings"
    "sync"
    "time"
    "testing"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type TUITestInstance struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    mu     sync.Mutex
    buffer strings.Builder
    t      *testing.T
}

func launchTUI(t *testing.T, args []string) *TUITestInstance { ... }
func (tui *TUITestInstance) WaitForText(text string, timeout time.Duration) { ... }
func (tui *TUITestInstance) WaitForNoText(text string, timeout time.Duration) { ... }
func (tui *TUITestInstance) SendKeys(text string) { ... }
func (tui *TUITestInstance) Snapshot() string { ... }
func (tui *TUITestInstance) Terminate() { ... }
```

Key implementation details:
- `launchTUI` builds the binary via `go build -o` into a temp dir, spawns it with `TERM=xterm-256color`
- Background goroutine reads stdout/stderr into a shared `strings.Builder` (mutex-protected)
- `WaitForText` polls every 100ms, strips ANSI, checks `strings.Contains` and whitespace-collapsed match
- `SendKeys` writes to stdin pipe
- `Terminate` calls `cmd.Process.Kill()`

**E2E test file**: `tests/e2e/splitpane_e2e_test.go`

```go
func TestSplitPane_E2E_TwoPane(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E in short mode")
    }
    tui := launchTUI(t, []string{"--test-view", "splitpane-demo"})
    defer tui.Terminate()

    // Verify both panes render
    tui.WaitForText("LEFT PANE", 10*time.Second)
    tui.WaitForText("│", 5*time.Second)  // divider visible
    tui.WaitForText("RIGHT PANE", 5*time.Second)

    // Tab toggles focus
    tui.SendKeys("\t")
    time.Sleep(200 * time.Millisecond)

    // Tab back
    tui.SendKeys("\t")
    time.Sleep(200 * time.Millisecond)

    // Esc pops back to chat
    tui.SendKeys("\x1b")
    tui.WaitForText("Ready", 5*time.Second)
}
```

This requires a `--test-view splitpane-demo` flag that pushes a demo split-pane view onto the router for E2E testing. The demo view uses two simple static-content panes labeled "LEFT PANE" and "RIGHT PANE".

### VHS Happy-Path Recording Test

**File**: `tests/vhs/splitpane.tape`

```tape
# Split Pane Component — Happy Path
Output tests/vhs/splitpane.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Shell "bash"
Set Theme "Dracula"

# Build and launch
Type "go build -o /tmp/crush-test . && /tmp/crush-test --test-view splitpane-demo"
Enter
Sleep 2s

# Verify split pane renders with both panes and divider
Screenshot tests/vhs/splitpane_initial.png

# Navigate in left pane
Down
Down
Sleep 300ms
Screenshot tests/vhs/splitpane_left_nav.png

# Tab to right pane
Tab
Sleep 300ms
Screenshot tests/vhs/splitpane_focus_right.png

# Tab back to left pane
Tab
Sleep 300ms
Screenshot tests/vhs/splitpane_focus_left.png

# Back to chat
Escape
Sleep 500ms
Screenshot tests/vhs/splitpane_back.png
```

Run:

```bash
vhs tests/vhs/splitpane.tape
```

Verify the generated GIF and screenshots show:
1. Two-pane layout with visible `│` divider
2. Left pane content and right pane content side-by-side
3. Visual focus indicator shifts when Tab is pressed
4. Navigation back to chat on Esc

### Manual Verification

1. `go build -o /tmp/crush-test . && /tmp/crush-test` — verify startup
2. Push a split-pane view via command palette or `--test-view` flag
3. Verify two panes render with divider at expected widths
4. Resize terminal: at width > 80, both panes visible; at width < 80, only focused pane visible
5. Press Tab: verify focus toggles; in compact mode, verify pane swap
6. Press Esc: verify pop back to chat
7. Narrow terminal to < 80 columns, verify compact mode activates and only one pane renders
8. Widen terminal back to > 80 columns, verify two-pane layout restores

---

## Risks

### 1. `uiSmithersView` layout case is missing from `generateLayout()`

**Risk**: The root `UI.generateLayout()` at `internal/ui/model/ui.go:2564` has cases for `uiOnboarding`, `uiInitialize`, `uiLanding`, and `uiChat`, but no `uiSmithersView` case. This means `layout.main` and `layout.header` are zero-valued `uv.Rectangle` when a Smithers view is active. The Draw path at line 2097 draws into `layout.header` and `layout.main`, which will produce no visible output or render at position (0,0).

**Mitigation**: Before this ticket can produce visible output, add a `uiSmithersView` case to `generateLayout()` that allocates `header` (1 row) and `main` (remaining space) from `appRect`. This is a small addition (~10 lines) that can be done as the first commit of this ticket or as a blocking prerequisite. The layout should mirror the compact chat layout: a 1-line header, then the full remaining area for the view.

### 2. `Pane` interface vs `views.View` interface mismatch

**Risk**: `Pane.Update()` returns `(Pane, tea.Cmd)` while `views.View.Update()` returns `(View, tea.Cmd)`. Types implementing both interfaces need careful handling to avoid type assertion issues.

**Mitigation**: Keep `Pane` deliberately separate from `views.View`. Pane implementations are private types within each consumer view (e.g., `ticketListPane`, `ticketDetailPane`). The consumer view itself implements `views.View` and delegates to the `SplitPane` internally. No type needs to implement both interfaces.

### 3. Compact breakpoint vs root model compact mode

**Risk**: The root `UI` model has its own compact breakpoint at 120 columns (`compactModeWidthBreakpoint`). The split pane has a separate breakpoint (default 80). These are independent, which could create confusing states where the root is in normal mode but the split pane collapses.

**Mitigation**: The split pane's `CompactBreakpoint` applies to its own allocated width (from `SetSize`), not the terminal width. Since Smithers views receive the full `appRect` width minus margins (~terminal width - 4), the effective breakpoints are close enough. Consumer views should set `CompactBreakpoint` to `LeftWidth * 2 + DividerWidth` (minimum viable two-pane width) so the pane collapses only when there is genuinely not enough space for both sides.

### 4. No `internal/ui/components/` package exists yet

**Risk**: This is the first file in a new package. Import cycles could arise if components need Crush internals.

**Mitigation**: The `components` package depends only on external libraries: `lipgloss`, `bubbletea`, `ultraviolet`, `charm.land/bubbles/v2/key`. It does NOT import `internal/ui/model`, `internal/ui/views`, or `internal/ui/styles`. Views import components, never the reverse.

### 5. `tea.WindowSizeMsg` propagation to views

**Risk**: The root model handles `tea.WindowSizeMsg` at `internal/ui/model/ui.go:664` and calls `updateLayoutAndSize()`. When in `uiSmithersView` state, the current view also needs to receive the size message so it can call `splitPane.SetSize()`. The existing `AgentsView` handles `tea.WindowSizeMsg` in its own `Update` (`internal/ui/views/agents.go:68-71`), which suggests the root model does forward it.

**Mitigation**: Verify that the root model's `Update` forwards messages to the active Smithers view when in `uiSmithersView` state. If it does not, add forwarding in the root model's Update. The `AgentsView` already handles `tea.WindowSizeMsg`, confirming this forwarding exists.

### 6. E2E test infrastructure does not exist yet

**Risk**: There is no existing Go E2E test harness or VHS tape infrastructure in the Crush repo. Both need to be created from scratch.

**Mitigation**: The Go E2E harness (`tests/e2e/helpers_test.go`) is ~100 lines of straightforward `os/exec` + buffer polling, modeled directly on the proven `tui-helpers.ts` pattern from upstream Smithers. VHS is an external tool (`brew install vhs`) with a simple declarative tape format. Both are low-risk to implement. The E2E harness is a reusable investment that all future view tickets will benefit from.
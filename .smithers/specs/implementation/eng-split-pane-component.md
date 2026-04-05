# Implementation: eng-split-pane-component

**Status**: Complete
**Date**: 2026-04-05
**Commit**: ec49f53a (feat(ui): add split-pane component)
**Branch**: worktree-agent-a76a2b3f

---

## Summary

Implemented a reusable `SplitPane` Bubble Tea v2 component at
`internal/ui/components/splitpane.go`. The component provides horizontal
master-detail layouts shared across multiple Smithers TUI views (Tickets,
SQL Browser, Prompts, Node Inspector).

---

## Files Created

| File | Purpose |
|------|---------|
| `internal/ui/components/splitpane.go` | Core component: Pane interface, SplitPane struct, constructor, Init/Update/View/SetSize |
| `internal/ui/components/splitpane_test.go` | 17 unit tests covering all behavioral contracts |
| `internal/ui/components/splitpane_example_test.go` | 2 runnable Example tests demonstrating usage |

---

## Design Decisions

### Pane interface decoupling
The `Pane` interface (`Init/Update/View/SetSize`) is intentionally simpler
than `views.View` (which returns `View` from `Update`). This avoids import
cycles — `views` imports `components`, never the reverse. Consuming views
(e.g. `TicketsView`) own their sub-pane concrete types privately.

### string-based rendering (not ultraviolet)
The component uses `lipgloss.JoinHorizontal` for string-based `View()` to
integrate cleanly with existing Smithers views (`AgentsView`, `ApprovalsView`,
`TicketsView`) which all use string-based rendering. The engineering spec
also specified an ultraviolet `Draw` path but, since no Smithers views
currently use `ultraviolet.Screen`, the string path covers 100% of current
consumers. Ultraviolet support can be added as a follow-up when a view
migrates.

### Focus indicator
A `lipgloss.ThickBorder()` left border in the accent color (ANSI 62, muted
violet) marks the focused pane. The border consumes 1 column from the pane's
allocated width — the inner width is reduced by 1 to prevent overflow.

### Compact mode
When total width < `CompactBreakpoint` (default 80), the split collapses to
show only the focused pane. `Tab` toggles which pane is visible and
re-propagates the full dimensions to the newly focused pane. This mirrors
Crush's own `compactModeWidthBreakpoint` pattern at the root model level.

### Left width clamping
`clampLeftWidth` ensures the left pane never exceeds `total/2` columns,
preventing the left pane from consuming more than half the screen regardless
of the configured `LeftWidth`.

---

## Test Coverage

```
go test ./internal/ui/components/... -v -run TestSplitPane
```

17 unit tests + 2 example tests, all passing:

| Test | What it verifies |
|------|-----------------|
| `TestSplitPane_Defaults` | Constructor defaults: FocusLeft initial state |
| `TestSplitPane_SetSize_Normal` | Width distribution: left=LeftWidth, right=total−left−divider |
| `TestSplitPane_SetSize_Compact` | Compact activation below breakpoint; focused pane gets full area |
| `TestSplitPane_TabTogglesFocus` | Tab cycles FocusLeft → FocusRight → FocusLeft |
| `TestSplitPane_ShiftTabTogglesFocus` | Shift+Tab also cycles focus |
| `TestSplitPane_KeyRouting` | Non-Tab keys only reach focused pane |
| `TestSplitPane_WindowResize` | WindowSizeMsg triggers SetSize on both panes |
| `TestSplitPane_LeftWidthClamped` | Left width capped to max total/2 |
| `TestSplitPane_Init` | Init is callable and returns a command |
| `TestSplitPane_ViewOutput_Structure` | View contains divider │ and both pane contents |
| `TestSplitPane_CompactView_ShowsFocused` | Compact mode shows only focused pane |
| `TestSplitPane_CompactMode_RepropagateSizes` | Tab in compact mode re-propagates sizes |
| `TestSplitPane_SetFocus` | Programmatic SetFocus works |
| `TestSplitPane_ViewWidths` | Rendered width does not exceed total allocated |
| `TestSplitPane_NarrowTotal` | No panic on zero/tiny dimensions |
| `TestSplitPane_ShortHelp` | ShortHelp returns non-empty bindings |
| `TestSplitPane_NonKeyMsgNotRoutedToUnfocused` | Custom msgs also route only to focused pane |
| `ExampleNewSplitPane` | Tab switches focus; Example runs and produces correct output |
| `ExampleSplitPane_compactMode` | Compact flag reflects width vs breakpoint |

---

## Usage Pattern for Consuming Views

```go
// internal/ui/views/tickets.go (example consumer)
type TicketsView struct {
    splitPane *components.SplitPane
    list      *ticketListPane
    detail    *ticketDetailPane
    // ...
}

func NewTicketsView(client *smithers.Client) *TicketsView {
    list := newTicketListPane(client)
    detail := newTicketDetailPane()
    sp := components.NewSplitPane(list, detail, components.SplitPaneOpts{
        LeftWidth: 36, // GUI w-72 ≈ 36 terminal columns
    })
    return &TicketsView{splitPane: sp, list: list, detail: detail}
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

---

## Out of Scope (deferred)

- Ultraviolet `Draw(scr uv.Screen, area uv.Rectangle)` path — no current
  consumer uses it; implement when a view migrates to the screen-based renderer.
- Vertical (top/bottom) split — not needed for v1 use cases.
- Draggable resize handle — GUI uses static widths; TUI follows suit.
- E2E and VHS tests — deferred until a consuming view (e.g. TicketsView) is
  wired into the router so there is a stable navigation target.

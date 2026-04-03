# Research: eng-split-pane-component

## Existing Crush Surface

- **`internal/ui/model/ui.go`**: Implements terminal layouts utilizing `ultraviolet` rendering (`Draw(scr uv.Screen, area uv.Rectangle)`). It uses `layout.SplitHorizontal` and `layout.SplitVertical` to statically allocate rects (e.g., `mainRect, sideRect := layout.SplitHorizontal(appRect, layout.Fixed(appRect.Dx()-sidebarWidth))`). It also tracks terminal window size events and collapses the sidebar below a global threshold (`compactModeWidthBreakpoint = 120`). However, these layout semantics are hardcoded in the root model, rather than extracted into a generalized pane component.
- **`internal/ui/views/router.go`**: Contains Crush's `views.View` interface (`Init()`, `Update(msg tea.Msg) (View, tea.Cmd)`, `View() string`, `Name()`, `ShortHelp()`). The router manages a stack of these views. Existing views output styled string components via Lipgloss (`View() string`), whereas `internal/ui/model/ui.go` uses screen-based `Draw(scr, area)` arrays. There is no abstraction inside `views.View` for routing focus or keystrokes to isolated child panes.

## Upstream Smithers Reference

- **`docs/smithers-tui/02-DESIGN.md`**: Outlines UI topologies that rely on split layouts. Notably, the **Run Dashboard/Node Inspector** has task tabs alongside a node list, the **SQL Browser** features a table sidebar and query editor, and the **Tickets View** requires a left-list/right-detail design. 
- **`docs/smithers-tui/features.ts`**: Highlights `PLATFORM_SPLIT_PANE_LAYOUTS` and `TICKETS_SPLIT_PANE_LAYOUT` as explicitly designed components within the Smithers inventory.
- **`../smithers/tests/tui.e2e.test.ts` & `../smithers/tests/tui-helpers.ts`**: Employs a Node/Bun-based `BunSpawnBackend` to launch the Smithers TUI process (`stdin: "pipe"`, `stdout: "pipe"`), reads stdout while stripping ANSI sequences, and makes polling assertions like `waitForText("fan-out-fan-in")` and `sendKeys("\t")`.
- **`../smithers/gui/src` (GUI Width Equivalency)**: Though local `.tsx` components could not be resolved directly, the engineering spec emphasizes a "static CSS widths with no drag resize" behavior mimicking upstream `w-64` (~32 terminal cols) and `w-72` (~36 cols) tailwind classes.

## Gaps

- **Interface Mismatch**: Crush's existing `views.View` lacks recursive dimension propagation (`SetSize`) distinct from window messages. A split pane must negotiate sizes relative to its sub-allocation, not just the terminal root. A `Pane` interface with `SetSize(width, height int)` is proposed in the engineering spec to wrap `tea.Model`, separating internal layouts from top-level routable views.
- **Duality of Rendering Paths**: The root UI in Crush (`ui.go`) leverages `ultraviolet` screen buffers, whereas standard views use string-based Lipgloss interpolation (`View() string`). A robust `SplitPane` needs to be able to render via `lipgloss.JoinHorizontal` to integrate seamlessly into string-based views (like `TicketsView` or `AgentsView`) while remaining compatible with `layout.SplitHorizontal` for `ultraviolet` migrations.
- **Compact Breakpoint Scoping**: The root chat model collapses the right sidebar when window width is `< 120`. A generic split pane nested inside a view has a much smaller available canvas, meaning its breakpoint (e.g., `80` columns) must be calculated relative to its allocated width, rather than the global terminal width.
- **Focus State Tracking**: `Router` dispatches `tea.Msg` uniformly. A composite `SplitPane` needs an internal `FocusSide` state (`FocusLeft` / `FocusRight`) that toggles via the `Tab` key, selectively routing updates (especially `tea.KeyMsg`) to the active child pane.

## Recommended Direction

- Implement a generic struct `SplitPane` in a new `internal/ui/components` package. This component will orchestrate two child `Pane` implementations side-by-side. 
- Provide an internal `Pane` interface extending `tea.Model` with `View() string` and `SetSize(width, height int)` methods. It intentionally decouples from `views.View` to let views privately orchestrate layout without imposing router bloat.
- Instantiate `SplitPaneOpts` that supports fixed left-widths (default `30` to match Crush's `sidebarWidth`), a configurable 1-column `│` string divider, and a customizable internal `CompactBreakpoint` (e.g., `80`).
- During `tea.WindowSizeMsg` updates, call `SetSize` to allocate fixed space to the left pane (clamped to max `width/2`), placing the remaining dimension on the responsive right pane. 
- If the layout's width dips below `CompactBreakpoint`, transition `SplitPane` into single-pane mode, displaying only the child associated with the active `focus` state, enabling the user to swap contexts with `Tab`.
- Create Go-equivalent E2E testing helpers replicating `BunSpawnBackend`. Use `os/exec` to spawn the `smithers-tui` binary with stdio pipes, build a `bufio.Scanner` loop to strip ANSI codes, and expose polling methods (`WaitForText`, `SendKeys`) to establish rigorous, timing-tolerant layout validation.
- Accompany the E2E verification with a VHS test for visual layout regressions.

## Files To Touch

- `internal/ui/components/splitpane.go` (new)
- `internal/ui/components/splitpane_test.go` (new)
- `tests/e2e/helpers_test.go` (new)
- `tests/e2e/splitpane_e2e_test.go` (new)
- `tests/vhs/splitpane.tape` (new)
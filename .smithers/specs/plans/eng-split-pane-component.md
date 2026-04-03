## Goal
Implement a reusable Bubble Tea component (`SplitPane`) for side-by-side layouts, supporting a fixed-width left pane and a responsive right pane. This component will be consumed by multiple Smithers TUI views (Tickets, SQL Browser, Node Inspector) to provide a consistent, dynamic split layout. The implementation must support terminal resizing, dynamic focus management, a compact fallback mode, and integration with both string-based (`View() string`) and screen-based (`Draw()`) rendering paths, mirroring the upstream Smithers GUI behaviors.

## Steps
1. **Define Core Interfaces and Structs**:
   - Scaffold the `internal/ui/components` package.
   - Define the internal `Pane` interface extending `tea.Model` with `View() string` and `SetSize(width, height int)`. This intentionally decouples from `views.View` so that host views can orchestrate layouts privately.
   - Define `FocusSide`, `SplitPaneOpts`, and the primary `SplitPane` struct.
   - Implement the `NewSplitPane` constructor with defaults (LeftWidth: 30, DividerWidth: 1, CompactBreakpoint: 80).

2. **Implement Layout Calculation & Resize Handling**:
   - Implement `SetSize(width, height int)` to allocate space dynamically. 
   - Clamp the left pane width to a maximum of half the total width.
   - Calculate remaining dimensions for the right pane.
   - Enable single-pane mode rendering if the total width dips below `CompactBreakpoint`.

3. **Implement Focus Routing & Update Logic**:
   - Implement `Init()` to recursively initialize both child panes.
   - Implement `Update(msg tea.Msg)` to capture `tea.WindowSizeMsg` and trigger `SetSize`.
   - Intercept the `Tab` key to toggle the `focus` state between `FocusLeft` and `FocusRight`. Re-propagate sizes if currently in compact mode.
   - Route all non-intercepted messages exclusively to the currently focused child pane.

4. **Implement Rendering Paths**:
   - **String-based (`View() string`)**: Use `lipgloss.JoinHorizontal` to stitch views with a 1-column `│` divider. Apply exact width constraints using lipgloss styles.
   - **Screen-based (`Draw(scr uv.Screen, area uv.Rectangle)`)**: Utilize ultraviolet's `layout.SplitHorizontal` to accurately project the left pane, divider, and right pane onto the screen buffer.
   - Apply styling changes (such as highlighting the active pane header or divider) to visually indicate focus.

5. **Unit Testing & E2E Foundation**:
   - Write comprehensive unit tests for `SplitPane` covering defaults, layout clamping, compact mode activation, key routing, and output rendering.
   - Build Go-based terminal E2E test helpers (`tests/e2e/helpers_test.go`) utilizing `os/exec` to spawn the `smithers-tui` binary. Replicate upstream `@microsoft/tui-test` functionality with `WaitForText`, `WaitForNoText`, and `SendKeys` by polling an ANSI-stripped stdout buffer.
   - Create an E2E test (`tests/e2e/splitpane_e2e_test.go`) that validates split pane structure, Tab-focus toggling, and layout reflows.

6. **VHS Recording Integration**:
   - Create a VHS tape (`tests/vhs/splitpane.tape`) for visual and empirical validation of the `SplitPane` behavior across different view states (initial load, focus change, compact collapse, navigation exit).

## File Plan
- `internal/ui/components/splitpane.go`: Core component logic including the `Pane` interface, `SplitPaneOpts`, focus routing, and `View()`/`Draw()` implementations.
- `internal/ui/components/splitpane_test.go`: Granular unit tests for `SplitPane` behavior.
- `tests/e2e/helpers_test.go`: Shared E2E testing utilities mapping to the upstream `tui-helpers.ts` behaviors (e.g., `WaitForText`, `SendKeys`).
- `tests/e2e/splitpane_e2e_test.go`: Terminal E2E test verifying structural rendering, focus toggling, and compact mode transitions.
- `tests/vhs/splitpane.tape`: VHS script for a happy-path recording test of the layout.

## Validation

- **Unit Testing**:
  Run from the repo root to verify localized layout calculations and state updates:
  ```bash
  go test ./internal/ui/components/... -v -run TestSplitPane
  ```
  Expected to assert correctly on defaults, dimension distribution, compact mode triggers, and constrained key routing.

- **Terminal E2E Coverage**:
  Run E2E tests utilizing the new custom Go test harness:
  ```bash
  go test ./tests/e2e/... -v -run TestSplitPane_E2E
  ```
  This will use `os/exec` to spawn the TUI, send keystrokes like `\t` (Tab) via stdin, and poll stdout using `WaitForText` to verify that the divider (`│`) appears in regular mode, and that focus transitions correctly without layout breakage.

- **VHS Happy-Path Recording**:
  Generate visual evidence of the layout using VHS:
  ```bash
  vhs tests/vhs/splitpane.tape
  ```
  Verify the generated `tests/vhs/splitpane.gif` visually confirms:
  1. The two-pane layout with a vertical divider.
  2. Tab key toggling focus cleanly between panes.
  3. Proper layout reflow on interaction.
  4. Successful `Esc` back-navigation.

- **Manual Verification**:
  1. Build the application: `go build -o smithers-tui .`
  2. Run the application: `./smithers-tui`
  3. Navigate to a target split view.
  4. Resize the terminal window horizontally to confirm the responsive right pane reflows and compact mode activates correctly below the threshold width.
  5. Press `Tab` to ensure focus smoothly alternates.
  6. Press `Esc` to verify un-mounting and return to the main chat router.

## Open Questions
- **Interface Flexibility**: Should the `Pane` interface strictly enforce returning `Pane` from `Update(msg)`, or should it align with standard Bubble Tea behavior (`tea.Model`, `tea.Cmd`) requiring dynamic type assertions in the router?
- **Standardized Widths**: The upstream GUI uses `w-64` (32 cols) and `w-72` (36 cols) for sidebars while `SplitPaneOpts` currently defaults to `30`. Should we export constant aliases (e.g., `components.WidthW64`) to standardize these width assignments across disparate consuming views?
- **E2E Demo Target**: How should we target a view to run `tests/e2e/splitpane_e2e_test.go` and `tests/vhs/splitpane.tape` if an implementing view like `TicketsView` is not fully wired yet? Should we temporarily create a hidden `/debug-splitpane` route specifically for test harness targets?
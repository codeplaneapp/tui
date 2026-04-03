## Existing Crush Surface

- **Component Directory**: The planned `internal/ui/components/` directory does not yet exist in the Crush codebase, as confirmed by inspecting the `internal/ui` folder.
- **Existing Notifications**: Crush currently has two notification mechanisms that are distinct from this requirement:
  1. OS-level desktop notifications located in `internal/ui/notification/` (which use native OS APIs rather than terminal overlays).
  2. A status bar `InfoMsg` rendered via `internal/ui/model/status.go`, which draws a single-line message at the bottom of the screen.
- **Positioning Helpers**: `internal/ui/common/common.go` provides `CenterRect` and `BottomLeftRect` functions for positioning UI elements, but lacks a bottom-right helper.
- **Overlay Rendering Pattern**: The `internal/ui/dialog/dialog.go` component provides a working reference for overlay rendering using `func (d *Overlay) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor`, demonstrating how to draw elements positionally over the main view.
- **Styling**: `internal/ui/styles/styles.go` defines the global `Styles` struct using lipgloss, but currently lacks any styling definitions for rich toasts.

## Upstream Smithers Reference

- **Testing Infrastructure**: The upstream Smithers TUI utilizes a Playwright/Bun-based terminal testing framework. As seen in `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`, it uses a `TUITestInstance` (implemented by `BunSpawnBackend`) to spawn the TUI process, pipe input, wait for specific text (`waitForText`), and match stdout snapshots.
- **Agent Handoff & UX**: `../smithers/docs/guides/smithers-tui-v2-agent-handoff.md` establishes that the TUI is a chat-first Control Plane, prioritizing real data binding and E2E TDD implementation for all components.

## Gaps

- **Missing Package & Models**: There is no `internal/ui/components` package. The `Toast` data model, `ToastLevel` enum, and `ToastManager` needed to handle bounded stacks of active toasts and TTL auto-dismissal via `tea.Tick` do not exist.
- **Positioning**: A `BottomRightRect` function is required to position the toast correctly on the `uv.Screen`, consistent with the existing helper patterns.
- **Styling**: The global `Styles` struct requires a new `Toast` sub-struct with initialized lipgloss definitions for the container, title, body, action hints, and per-level border colors.
- **E2E & VHS Tests**: There is no Go equivalent of the `TUITestInstance` pattern yet, nor are there terminal E2E tests or VHS tapes covering the toast auto-dismissal lifecycle.

## Recommended Direction

1. **Create the Component Package**: Instantiate the `internal/ui/components/` directory. Implement `notification.go` containing the `Toast` struct, `ToastLevel` enum, and `ToastManager`.
2. **Implement Rendering & TTL**: Ensure the `ToastManager` handles `tea.Tick` commands for TTL expiry. Its `Draw` method should iterate through active toasts and render them positionally using `uv.NewStyledString(...).Draw(scr, rect)`. 
3. **Extend Positioning Helpers**: Add a `BottomRightRect` function to `internal/ui/common/common.go`.
4. **Add Toast Styles**: Inject a `Toast` struct into the global `Styles` struct in `internal/ui/styles/styles.go`, configuring it with rounded borders, specific level colors, and appropriate padding/margins.
5. **Validation Gate**: Implement comprehensive unit tests in `notification_test.go` targeting `ToastManager` state changes and `Draw` logic. Stub an E2E terminal test modeled after the Smithers upstream pattern and write a VHS tape (`toast_notification.tape`) to capture the visual regression flow.

## Files To Touch

- `internal/ui/common/common.go`
- `internal/ui/common/common_test.go`
- `internal/ui/styles/styles.go`
- `internal/ui/components/notification.go` (new)
- `internal/ui/components/notification_test.go` (new)
- `tests/e2e/toast_notification_test.go` (new)
- `tests/vhs/toast_notification.tape` (new)
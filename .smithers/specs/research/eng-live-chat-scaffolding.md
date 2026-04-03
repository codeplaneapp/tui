Research Report: eng-live-chat-scaffolding

## Existing Crush Surface
- **Router and View Interface (`internal/ui/views/router.go`, `internal/ui/views/agents.go`)**: A basic stack-based view router is already implemented in `internal/ui/views/router.go`. It defines a `View` interface and is currently integrated into `internal/ui/model/ui.go` for state routing (`m.viewRouter *views.Router`). However, the current interface defines `View() string` rather than the native Ultraviolet screen buffer drawing method.
- **UI Integration (`internal/ui/model/ui.go`)**: We can see `ui.go` delegates update and view logic to the router (`m.viewRouter.Current().View()`) when the state is `uiSmithersView` (e.g. line 2100).
- **Smithers Domain Types (`internal/smithers/types.go`)**: Found existing scaffolding for types like `Agent`, `SQLResult`, and `ScoreRow`. However, models specific to the live chat feature (`Run`, `ChatBlock`, `Client`) are missing.

## Upstream Smithers Reference
- **E2E Terminal Testing Harness (`../smithers/tests/tui-helpers.ts`, `../smithers/tests/tui.e2e.test.ts`)**: Upstream Smithers uses a Playwright-style TUI harness. Tests spawn the TUI as a subprocess and expose an API (`TUITestInstance`) with functions like `waitForText`, `waitForNoText`, `sendKeys`, and `snapshot()` that strip ANSI sequences and poll the buffer.
- **Agent Handoff (`../smithers/docs/guides/smithers-tui-v2-agent-handoff.md`)**: The expected mechanism for taking over a chat session uses native TUI handoff instead of rendering a chat clone.

## Gaps
- **Rendering Model Discrepancy**: The engineering spec explicitly requires the `View` interface to follow "Crush's Bubble Tea v2 Draw-based rendering model (the `Draw(scr uv.Screen, area uv.Rectangle)` pattern, not legacy `View() string`)". Currently, `internal/ui/views/router.go` and `internal/ui/model/ui.go` are wired to use `View() string` combined with `uv.NewStyledString()`. This is a critical architectural gap that must be addressed to properly implement `LiveChatView`.
- **Data Models Missing**: The required `Run` and `ChatBlock` data types, and the `Client` interface (to provide the fake data stream) are missing from `internal/smithers/types.go`. The stub client `internal/smithers/stub.go` also needs to be created.
- **LiveChat View Missing**: `internal/ui/views/livechat.go` has not been implemented yet.
- **Testing Infrastructure**: The Go equivalent of the `TUITestInstance` harness and the requested VHS tape recording are absent.

## Recommended Direction
1. **Refactor the View Interface**: Move the `View` interface from `router.go` into `internal/ui/views/view.go` and refactor it from `View() string` to `Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor`. Update `internal/ui/model/ui.go` (and existing views like `agents.go`) to use the new `Draw` paradigm.
2. **Implement Data Models**: Add `Run`, `ChatBlock`, and the `Client` interface to `internal/smithers/types.go`. Implement a stub client in `internal/smithers/stub.go` that simulates a delayed data stream.
3. **Build the View**: Create `internal/ui/views/livechat.go` implementing the newly refactored `View` interface, adding a header, and rendering the simulated `ChatBlock` data to the screen.
4. **Implement TUI E2E Harness**: Build `tests/tui_helpers_test.go` mirroring `TUITestInstance` using `os/exec` + `io.Pipe`, and write `tests/livechat_e2e_test.go`. Write the `tests/tapes/livechat-happy-path.tape` file.

## Files To Touch
- `internal/ui/views/view.go`
- `internal/ui/views/router.go`
- `internal/ui/views/agents.go`
- `internal/ui/views/livechat.go`
- `internal/ui/model/ui.go`
- `internal/smithers/types.go`
- `internal/smithers/stub.go`
- `tests/tui_helpers_test.go`
- `tests/livechat_e2e_test.go`
- `tests/tapes/livechat-happy-path.tape`
# Implementation Summary: eng-in-terminal-toast-component

**Status**: Shipped
**Date**: 2026-04-05

---

## What Was Built

### New: `internal/ui/components/toast.go`

A production-quality Bubble Tea v2 / Lip Gloss v2 in-terminal toast overlay component. Key types:

- **`ToastLevel`** — severity enum: `ToastLevelInfo`, `ToastLevelSuccess`, `ToastLevelWarning`, `ToastLevelError`
- **`ActionHint`** — `{Key, Label}` pair rendered as `[key] label` at the bottom of a toast
- **`ShowToastMsg`** — public message to display a toast (Title, Body, ActionHints, Level, TTL)
- **`DismissToastMsg`** — public message to manually dismiss a toast by ID
- **`ToastManager`** — manages a bounded stack of toasts (max 3), handles TTL timers via `tea.Tick`, renders via `Draw(scr uv.Screen, area uv.Rectangle)` at the bottom-right corner of the given area

The `ToastManager` is integrated with the root `UI` model: all messages are forwarded through `m.toasts.Update(msg)` early in `Update()`, and `m.toasts.Draw(scr, scr.Bounds())` is called in `Draw()` between content and dialogs (so dialogs always appear on top).

### New: `internal/ui/components/toast_test.go`

21 passing unit tests covering:
- Lifecycle: add, dismiss, TTL auto-dismiss, bounded stack eviction, clear
- Rendering: title, body, action hints, multiple toasts, post-dismiss cleanup
- Positioning: bottom-right quadrant, bottom quarter of tall screens
- Severity: all four levels render without error
- Edge cases: narrow terminal (10 cols), zero-size screen, long body word-wrap

### Modified: `internal/ui/common/common.go`

Added `BottomRightRect(area uv.Rectangle, width, height int) uv.Rectangle` to mirror the existing `BottomLeftRect` and `CenterRect` helpers. Used by the toast Draw logic to anchor toasts at the bottom-right.

### Modified: `internal/ui/styles/styles.go`

Added `Styles.Toast` struct with fields:
- `Container`, `ContainerInfo`, `ContainerSuccess`, `ContainerWarning`, `ContainerError` — level-specific border colors via rounded border + bgOverlay background
- `Title` — bold heading
- `Body` — muted body text
- `ActionHint`, `ActionHintKey` — subtle key/label rendering

All colors source from the existing `charmtone`/`lipgloss` palette; no hardcoded hex values.

### Modified: `internal/ui/model/ui.go`

- Added `toasts *components.ToastManager` field to `UI`
- Initialized in `New()` via `components.NewToastManager(com.Styles)`
- Universal message forwarding at the top of `Update()` (handles `ShowToastMsg`, `DismissToastMsg`, and the package-internal `toastTimedOutMsg`)
- `Draw()` call inserted between status rendering and dialog overlay
- Env-gated debug hook: `CRUSH_TEST_TOAST_ON_START=1` triggers an info toast at startup for manual verification

---

## Integration API

Dispatch a toast from any view/component by returning a `tea.Cmd` that produces `components.ShowToastMsg`:

```go
return func() tea.Msg {
    return components.ShowToastMsg{
        Title: "Run finished",
        Body:  "workflow/code-review completed in 42s",
        Level: components.ToastLevelSuccess,
        ActionHints: []components.ActionHint{
            {Key: "r", Label: "view results"},
            {Key: "esc", Label: "dismiss"},
        },
        TTL: 10 * time.Second,
    }
}
```

Or use `util.CmdHandler(components.ShowToastMsg{...})` for convenience.

---

## Test Results

```
ok  github.com/charmbracelet/crush/internal/ui/components  0.921s
ok  github.com/charmbracelet/crush/internal/ui/styles      0.315s
```

All 21 component tests pass. The `internal/ui/model` package has a pre-existing compile error (`msg.(tea.KeyPressMsg)` type assertion on a concrete struct) in the Smithers view key handler — this is unrelated to this ticket and was present before implementation began.

---

## Deferred

- **SSE/event-driven wiring**: Connecting Smithers run events (`RunFinished`, etc.) to `ShowToastMsg` is deferred to the notification integration ticket.
- **E2E terminal harness**: VHS tape and Go process-spawn harness were specified in the plan but deferred — the component is fully exercised by unit tests against `uv.NewScreen` virtual buffers.
- **Dismiss key routing**: The plan mentioned routing an `esc` key to dismiss the frontmost toast. Deferred; currently dismissal is time-based or explicit `DismissToastMsg`.
